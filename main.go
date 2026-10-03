package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
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

func (s *Store) upsert(u deviceUpdate) {
	s.mu.Lock()
	room, ok := s.rooms[u.RoomID]
	if !ok {
		room = make(map[string]*Device)
		s.rooms[u.RoomID] = room
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
	s.mu.Unlock()
	s.broadcast(u.RoomID)
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
func (s *Store) attach(roomID, deviceID string) {
	now := time.Now()
	s.mu.Lock()
	if s.presence[roomID] == nil {
		s.presence[roomID] = make(map[string]*presence)
	}
	p := s.presence[roomID][deviceID]
	if p == nil {
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
	defer s.mu.Unlock()
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
	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return list[i].Name < list[j].Name
		}
		return list[i].ID < list[j].ID
	})
	return roomSnapshot{ServerTime: now, Devices: list}
}

func (s *Store) subscribe(roomID string) chan []byte {
	ch := make(chan []byte, 1)
	s.mu.Lock()
	if s.subs[roomID] == nil {
		s.subs[roomID] = make(map[chan []byte]struct{})
	}
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

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
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

	mux.HandleFunc("/api/battery", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		store.upsert(u)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/leave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		ch := store.subscribe(roomID)
		defer store.unsubscribe(roomID, ch)

		if deviceID != "" {
			store.attach(roomID, deviceID)
			defer store.detach(roomID, deviceID)
		}

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
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("view-my-batteries listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
