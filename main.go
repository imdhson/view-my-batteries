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
	staleAfter     = 30 * time.Second
	cleanupEvery   = 10 * time.Second
	subChannelSize = 4
)

// Device represents the latest known state of a single connected device.
type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Level     *float64  `json:"level"`
	Charging  *bool     `json:"charging"`
	Supported bool      `json:"supported"`
	UserAgent string    `json:"userAgent"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type deviceUpdate struct {
	RoomID    string  `json:"roomId"`
	DeviceID  string  `json:"deviceId"`
	Name      string  `json:"name"`
	Level     *float64 `json:"level"`
	Charging  *bool    `json:"charging"`
	Supported bool     `json:"supported"`
	UserAgent string   `json:"userAgent"`
}

type leaveRequest struct {
	RoomID   string `json:"roomId"`
	DeviceID string `json:"deviceId"`
}

// Store holds all in-memory room/device state. Nothing here is persisted to disk.
type Store struct {
	mu   sync.Mutex
	rooms map[string]map[string]*Device      // roomID -> deviceID -> device
	subs  map[string]map[chan []byte]struct{} // roomID -> set of subscriber channels
}

func newStore() *Store {
	return &Store{
		rooms: make(map[string]map[string]*Device),
		subs:  make(map[string]map[chan []byte]struct{}),
	}
}

func (s *Store) upsert(u deviceUpdate) {
	s.mu.Lock()
	room, ok := s.rooms[u.RoomID]
	if !ok {
		room = make(map[string]*Device)
		s.rooms[u.RoomID] = room
	}
	room[u.DeviceID] = &Device{
		ID:        u.DeviceID,
		Name:      u.Name,
		Level:     u.Level,
		Charging:  u.Charging,
		Supported: u.Supported,
		UserAgent: u.UserAgent,
		UpdatedAt: time.Now(),
	}
	s.mu.Unlock()
	s.broadcast(u.RoomID)
}

func (s *Store) remove(roomID, deviceID string) {
	s.mu.Lock()
	if room, ok := s.rooms[roomID]; ok {
		delete(room, deviceID)
	}
	s.mu.Unlock()
	s.broadcast(roomID)
}

func (s *Store) snapshot(roomID string) []*Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	room := s.rooms[roomID]
	list := make([]*Device, 0, len(room))
	for _, d := range room {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Name != list[j].Name {
			return list[i].Name < list[j].Name
		}
		return list[i].ID < list[j].ID
	})
	return list
}

func (s *Store) subscribe(roomID string) chan []byte {
	ch := make(chan []byte, subChannelSize)
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

func (s *Store) broadcast(roomID string) {
	payload, err := json.Marshal(s.snapshot(roomID))
	if err != nil {
		return
	}
	s.mu.Lock()
	set := s.subs[roomID]
	chans := make([]chan []byte, 0, len(set))
	for ch := range set {
		chans = append(chans, ch)
	}
	s.mu.Unlock()

	for _, ch := range chans {
		select {
		case ch <- payload:
		default:
			// Slow subscriber: drop the stale message, keep only the latest.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- payload:
			default:
			}
		}
	}
}

// cleanupStale periodically drops devices that stopped sending heartbeats.
func (s *Store) cleanupStale() {
	ticker := time.NewTicker(cleanupEvery)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		var affected []string
		s.mu.Lock()
		for roomID, room := range s.rooms {
			for deviceID, d := range room {
				if now.Sub(d.UpdatedAt) > staleAfter {
					delete(room, deviceID)
					affected = append(affected, roomID)
				}
			}
			if len(room) == 0 {
				delete(s.rooms, roomID)
			}
		}
		s.mu.Unlock()
		for _, roomID := range affected {
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
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
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
		if len(u.Name) > 80 {
			u.Name = u.Name[:80]
		}
		if len(u.UserAgent) > 200 {
			u.UserAgent = u.UserAgent[:200]
		}
		store.upsert(u)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("/api/leave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req leaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
