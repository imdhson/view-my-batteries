package main

import (
	"cmp"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

const (
	// heartbeatGrace: a device that never opened a stream still counts as
	// online if it sent an update within this window.
	heartbeatGrace = 25 * time.Second
	// offlineRetention: how long an offline device stays listed (as offline)
	// before it is dropped from the room.
	offlineRetention = 10 * time.Minute
	cleanupEvery     = 5 * time.Second
	maxInfoString    = 64

	// Capacity limits to prevent Resource Exhaustion (DoS)
	maxRooms        = 10000
	maxDevices      = 100
	maxConnsPerRoom = 1000
)

// DeviceInfo holds optional, best-effort details the browser could expose.
// Every field may be empty when the browser does not support the relevant API.
type DeviceInfo struct {
	OS             string  `json:"os,omitempty"`
	OSVersion      string  `json:"osVersion,omitempty"`
	Browser        string  `json:"browser,omitempty"`
	Model          string  `json:"model,omitempty"`
	DeviceType     string  `json:"deviceType,omitempty"`
	ScreenW        int     `json:"screenW,omitempty"`
	ScreenH        int     `json:"screenH,omitempty"`
	PixelRatio     float64 `json:"pixelRatio,omitempty"`
	Cores          int     `json:"cores,omitempty"`
	MemoryGB       float64 `json:"memoryGB,omitempty"`
	MaxTouchPoints int     `json:"maxTouchPoints,omitempty"`
	Language       string  `json:"language,omitempty"`
	Timezone       string  `json:"timezone,omitempty"`
	NetType        string  `json:"netType,omitempty"`
	Downlink       float64 `json:"downlink,omitempty"`
	RTT            int     `json:"rtt,omitempty"`
	SaveData       bool    `json:"saveData,omitempty"`
	Visibility     string  `json:"visibility,omitempty"`
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (i *DeviceInfo) sanitize() {
	for _, p := range []*string{&i.OS, &i.OSVersion, &i.Browser, &i.Model, &i.DeviceType,
		&i.Language, &i.Timezone, &i.NetType, &i.Visibility} {
		*p = clip(*p, maxInfoString)
	}
}

// Device represents the latest known state of a single device.
type Device struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Level           *float64   `json:"level"`
	Charging        *bool      `json:"charging"`
	ChargingTime    *float64   `json:"chargingTime"`
	DischargingTime *float64   `json:"dischargingTime"`
	Supported       bool       `json:"supported"`
	UserAgent       string     `json:"userAgent"`
	Info            DeviceInfo `json:"info"`
	Online          bool       `json:"online"`
	ConnectedAt     *time.Time `json:"connectedAt,omitempty"`
	LastSeen        time.Time  `json:"lastSeen"`
	UpdatedAt       time.Time  `json:"updatedAt"`

	lastOnline bool // online state as of the last snapshot
}

// presence tracks the open SSE connections of a device, which is the
// real-time signal for "is this device connected right now".
type presence struct {
	conns    int
	since    time.Time // start of the current connected session
	lastSeen time.Time // last time a connection was open
}

type deviceUpdate struct {
	RoomID          string     `json:"roomId"`
	DeviceID        string     `json:"deviceId"`
	Name            string     `json:"name"`
	Level           *float64   `json:"level"`
	Charging        *bool      `json:"charging"`
	ChargingTime    *float64   `json:"chargingTime"`
	DischargingTime *float64   `json:"dischargingTime"`
	Supported       bool       `json:"supported"`
	UserAgent       string     `json:"userAgent"`
	Info            DeviceInfo `json:"info"`
}

type leaveRequest struct {
	RoomID   string `json:"roomId"`
	DeviceID string `json:"deviceId"`
}

type roomSnapshot struct {
	ServerTime time.Time `json:"serverTime"`
	Devices    []Device  `json:"devices"`
}

// Store holds all in-memory room/device state. Nothing here is persisted to disk.
type Store struct {
	mu          sync.Mutex
	rooms       map[string]map[string]*Device       // roomID -> deviceID -> device
	presence    map[string]map[string]*presence     // roomID -> deviceID -> presence
	subs        map[string]map[chan []byte]struct{} // roomID -> set of subscriber channels
	broadcastCh map[string]chan struct{}            // roomID -> channel to trigger a room broadcast
}

func newStore() *Store {
	return &Store{
		rooms:       make(map[string]map[string]*Device),
		presence:    make(map[string]map[string]*presence),
		subs:        make(map[string]map[chan []byte]struct{}),
		broadcastCh: make(map[string]chan struct{}),
	}
}

// isOnline must be called with s.mu held. A device that has ever opened a
// stream is online exactly while a stream is open; otherwise (e.g. a proxy
// that breaks SSE) fall back to how recent its last heartbeat was.
func (s *Store) isOnline(roomID string, d *Device, now time.Time) bool {
	if p := s.presence[roomID][d.ID]; p != nil {
		return p.conns > 0
	}
	return now.Sub(d.UpdatedAt) < heartbeatGrace
}

// lastSeen must be called with s.mu held.
func (s *Store) lastSeen(roomID string, d *Device) time.Time {
	t := d.UpdatedAt
	if p := s.presence[roomID][d.ID]; p != nil && p.lastSeen.After(t) {
		t = p.lastSeen
	}
	return t
}

func (s *Store) upsert(u deviceUpdate) bool {
	s.mu.Lock()
	room, ok := s.rooms[u.RoomID]
	if !ok {
		if len(s.rooms) >= maxRooms {
			s.mu.Unlock()
			return false
		}
		room = make(map[string]*Device)
		s.rooms[u.RoomID] = room
	}

	// ⚡ Bolt: Update existing device struct in-place to avoid heap
	// allocations on every heartbeat, reducing GC pressure.
	if dev, exists := room[u.DeviceID]; exists {
		dev.Name = u.Name
		dev.Level = u.Level
		dev.Charging = u.Charging
		dev.ChargingTime = u.ChargingTime
		dev.DischargingTime = u.DischargingTime
		dev.Supported = u.Supported
		dev.UserAgent = u.UserAgent
		dev.Info = u.Info
		dev.UpdatedAt = time.Now()
	} else {
		if len(room) >= maxDevices {
			s.mu.Unlock()
			return false
		}
		room[u.DeviceID] = &Device{
			ID:              u.DeviceID,
			Name:            u.Name,
			Level:           u.Level,
			Charging:        u.Charging,
			ChargingTime:    u.ChargingTime,
			DischargingTime: u.DischargingTime,
			Supported:       u.Supported,
			UserAgent:       u.UserAgent,
			Info:            u.Info,
			UpdatedAt:       time.Now(),
		}
	}

	s.mu.Unlock()
	s.broadcast(u.RoomID)
	return true
}

func (s *Store) remove(roomID, deviceID string) {
	s.mu.Lock()
	if room, ok := s.rooms[roomID]; ok {
		delete(room, deviceID)
	}
	if p := s.presence[roomID][deviceID]; p != nil && p.conns == 0 {
		delete(s.presence[roomID], deviceID)
	}
	s.mu.Unlock()
	s.broadcast(roomID)
}

// attach records a newly opened stream for a device.
func (s *Store) attach(roomID, deviceID string) bool {
	now := time.Now()
	s.mu.Lock()
	if s.presence[roomID] == nil {
		if len(s.presence) >= maxRooms {
			s.mu.Unlock()
			return false
		}
		s.presence[roomID] = make(map[string]*presence)
	}
	p := s.presence[roomID][deviceID]
	if p == nil {
		if len(s.presence[roomID]) >= maxDevices {
			s.mu.Unlock()
			return false
		}
		p = &presence{}
		s.presence[roomID][deviceID] = p
	}
	if p.conns == 0 {
		p.since = now
	}
	p.conns++
	p.lastSeen = now
	s.mu.Unlock()
	s.broadcast(roomID)
	return true
}

// detach records a closed stream for a device.
func (s *Store) detach(roomID, deviceID string) {
	s.mu.Lock()
	if p := s.presence[roomID][deviceID]; p != nil {
		p.conns--
		p.lastSeen = time.Now()
		if p.conns <= 0 {
			p.conns = 0
			if _, known := s.rooms[roomID][deviceID]; !known {
				delete(s.presence[roomID], deviceID)
			}
		}
		if len(s.presence[roomID]) == 0 {
			delete(s.presence, roomID)
		}
	}
	s.mu.Unlock()
	s.broadcast(roomID)
}

func (s *Store) snapshot(roomID string) roomSnapshot {
	now := time.Now()
	s.mu.Lock()
	room := s.rooms[roomID]
	list := make([]Device, 0, len(room))
	for _, d := range room {
		online := s.isOnline(roomID, d, now)
		d.lastOnline = online

		c := *d
		c.Online = online
		c.LastSeen = s.lastSeen(roomID, d)
		if p := s.presence[roomID][d.ID]; p != nil && p.conns > 0 {
			since := p.since
			c.ConnectedAt = &since
			c.LastSeen = now
		}
		list = append(list, c)
	}
	// ⚡ Bolt: Unlock before sorting to reduce global lock contention
	s.mu.Unlock()

	slices.SortFunc(list, func(a, b Device) int {
		if a.Name != b.Name {
			return cmp.Compare(a.Name, b.Name)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return roomSnapshot{ServerTime: now, Devices: list}
}

func (s *Store) subscribe(roomID string) chan []byte {
	s.mu.Lock()
	if s.subs[roomID] == nil {
		if len(s.subs) >= maxRooms {
			s.mu.Unlock()
			return nil
		}
		s.subs[roomID] = make(map[chan []byte]struct{})
	}
	if len(s.subs[roomID]) >= maxConnsPerRoom {
		s.mu.Unlock()
		return nil
	}
	ch := make(chan []byte, 1)
	s.subs[roomID][ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Store) unsubscribe(roomID string, ch chan []byte) {
	s.mu.Lock()
	if set, ok := s.subs[roomID]; ok {
		delete(set, ch)
		if len(set) == 0 {
			delete(s.subs, roomID)
		}
	}
	s.mu.Unlock()
	s.broadcast(roomID)
}

func (s *Store) broadcaster(roomID string, ch chan struct{}) {
	for range ch {
		payload, err := json.Marshal(s.snapshot(roomID))
		if err != nil {
			log.Printf("failed to marshal snapshot during broadcast: %v", err)
			continue
		}

		s.mu.Lock()
		set := s.subs[roomID]
		chans := make([]chan []byte, 0, len(set))
		for sub := range set {
			chans = append(chans, sub)
		}
		s.mu.Unlock()

		for _, sub := range chans {
			select {
			case sub <- payload:
			default:
				// Channel full: drain the old state and replace with the newest state
				select {
				case <-sub:
				default:
				}
				select {
				case sub <- payload:
				default:
				}
			}
		}
	}
}

func (s *Store) broadcast(roomID string) {
	s.mu.Lock()
	// ⚡ Bolt: Cleanup empty rooms to prevent goroutine and memory leaks
	if len(s.rooms[roomID]) == 0 && len(s.subs[roomID]) == 0 {
		if ch, ok := s.broadcastCh[roomID]; ok {
			close(ch)
			delete(s.broadcastCh, roomID)
		}
		delete(s.rooms, roomID)
		delete(s.presence, roomID)
		s.mu.Unlock()
		return
	}

	// ⚡ Bolt: Skip expensive JSON serialization if no one is listening.
	// But first, we must sync lastOnline to prevent cleanupStale from
	// infinitely triggering broadcasts.
	if len(s.subs[roomID]) == 0 {
		now := time.Now()
		for _, d := range s.rooms[roomID] {
			d.lastOnline = s.isOnline(roomID, d, now)
		}
		s.mu.Unlock()
		return
	}

	ch, ok := s.broadcastCh[roomID]
	if !ok {
		ch = make(chan struct{}, 1)
		s.broadcastCh[roomID] = ch
		go s.broadcaster(roomID, ch)
	}
	s.mu.Unlock()

	select {
	case ch <- struct{}{}:
	default:
	}
}

// cleanupStale periodically notices devices whose online state changed
// because their heartbeat stopped, and drops devices offline for too long.
func (s *Store) cleanupStale() {
	ticker := time.NewTicker(cleanupEvery)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		affected := make(map[string]struct{})
		s.mu.Lock()
		for roomID, room := range s.rooms {
			for deviceID, d := range room {
				online := s.isOnline(roomID, d, now)
				if !online && now.Sub(s.lastSeen(roomID, d)) > offlineRetention {
					delete(room, deviceID)
					if p := s.presence[roomID][deviceID]; p != nil && p.conns == 0 {
						delete(s.presence[roomID], deviceID)
					}
					affected[roomID] = struct{}{}
				} else if online != d.lastOnline {
					affected[roomID] = struct{}{}
				}
			}
			if len(room) == 0 {
				delete(s.rooms, roomID)
			}
		}
		s.mu.Unlock()
		for roomID := range affected {
			s.broadcast(roomID)
		}
	}
}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ⚡ Bolt: Pre-allocate static JSON response to avoid heap allocations
// and reflection overhead on high-frequency endpoints.
var okResponse = []byte("{\"ok\":true}\n")

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(okResponse)
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' https://static.cloudflareinsights.com; connect-src 'self' https://cloudflareinsights.com")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Store) handleBattery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	var u deviceUpdate
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&u); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if !validID.MatchString(u.RoomID) || !validID.MatchString(u.DeviceID) {
		http.Error(w, "invalid roomId or deviceId", http.StatusBadRequest)
		return
	}
	if u.Name == "" {
		u.Name = "Unknown device"
	}
	u.Name = clip(u.Name, 80)
	u.UserAgent = clip(u.UserAgent, 200)
	u.Info.sanitize()
	if !s.upsert(u) {
		http.Error(w, "server at capacity", http.StatusServiceUnavailable)
		return
	}
	writeOK(w)
}

func main() {
	store := newStore()
	go store.cleanupStale()

	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	mux := http.NewServeMux()
	mux.Handle("/", fileServer)

	mux.HandleFunc("/api/battery", store.handleBattery)

	mux.HandleFunc("/api/leave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
			return
		}
		var req leaveRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if validID.MatchString(req.RoomID) && validID.MatchString(req.DeviceID) {
			store.remove(req.RoomID, req.DeviceID)
		}
		writeOK(w)
	})

	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			http.Error(w, "unacceptable", http.StatusNotAcceptable)
			return
		}
		roomID := r.URL.Query().Get("roomId")
		if !validID.MatchString(roomID) {
			http.Error(w, "invalid roomId", http.StatusBadRequest)
			return
		}
		// deviceId is optional; when present, the open stream marks that
		// device as connected in real time.
		deviceID := r.URL.Query().Get("deviceId")
		if deviceID != "" && !validID.MatchString(deviceID) {
			http.Error(w, "invalid deviceId", http.StatusBadRequest)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store") // 🛡️ Sentinel: prevent browser from caching sensitive SSE data
		w.Header().Set("Connection", "keep-alive")

		ch := store.subscribe(roomID)
		if ch == nil {
			http.Error(w, "server at capacity", http.StatusServiceUnavailable)
			return
		}

		if deviceID != "" {
			if !store.attach(roomID, deviceID) {
				store.unsubscribe(roomID, ch)
				http.Error(w, "server at capacity", http.StatusServiceUnavailable)
				return
			}
			defer store.detach(roomID, deviceID)
		}

		w.WriteHeader(http.StatusOK)

		defer store.unsubscribe(roomID, ch)

		// Send the current snapshot immediately.
		initial, _ := json.Marshal(store.snapshot(roomID))
		w.Write([]byte("data: "))
		w.Write(initial)
		w.Write([]byte("\n\n"))
		flusher.Flush()

		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case payload := <-ch:
				w.Write([]byte("data: "))
				w.Write(payload)
				w.Write([]byte("\n\n"))
				flusher.Flush()
			case <-keepalive.C:
				w.Write([]byte(": ping\n\n"))
				flusher.Flush()
			}
		}
	})

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           secureHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("view-my-batteries listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
