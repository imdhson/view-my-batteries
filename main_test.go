package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStoreDeadlockOnBroadcastWithSubscribers(t *testing.T) {
	s := newStore()
	roomID := "test-room"
	deviceID := "test-dev"

	ch := s.subscribe(roomID)
	if ch == nil {
		t.Fatal("expected sub channel")
	}

	done := make(chan bool, 1)
	go func() {
		// When a device attaches to a room with subscribers, s.broadcast is triggered
		s.attach(roomID, deviceID)

		// Then an upsert is done
		level := 0.85
		s.upsert(deviceUpdate{
			RoomID:   roomID,
			DeviceID: deviceID,
			Name:     "Test Device",
			Level:    &level,
		})

		// And a snapshot is taken
		snap := s.snapshot(roomID)
		if len(snap.Devices) != 1 {
			t.Errorf("expected 1 device, got %d", len(snap.Devices))
		}
		done <- true
	}()

	select {
	case <-done:
		// Success!
	case <-time.After(2 * time.Second):
		t.Fatal("Deadlock detected in store operations!")
	}
}

func TestHTTPBatteryUpdateAndStream(t *testing.T) {
	s := newStore()

	// 1. Subscribe to stream
	roomID := "room123"
	deviceID := "dev123"

	ch := s.subscribe(roomID)
	if ch == nil {
		t.Fatal("expected subscribe channel")
	}
	defer s.unsubscribe(roomID, ch)

	if !s.attach(roomID, deviceID) {
		t.Fatal("expected attach to succeed")
	}
	defer s.detach(roomID, deviceID)

	// 2. Perform POST /api/battery
	level := 0.77
	reqBody, _ := json.Marshal(deviceUpdate{
		RoomID:    roomID,
		DeviceID:  deviceID,
		Name:      "Laptop",
		Supported: true,
		Level:     &level,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/battery", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleBattery(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	// 3. Receive broadcast messages on channel until we see our device
	timeout := time.After(2 * time.Second)
	found := false
	for !found {
		select {
		case msg := <-ch:
			var snap roomSnapshot
			if err := json.Unmarshal(msg, &snap); err != nil {
				t.Fatalf("failed to unmarshal snapshot: %v", err)
			}
			if len(snap.Devices) > 0 {
				if snap.Devices[0].ID != deviceID {
					t.Fatalf("expected device id %s, got %s", deviceID, snap.Devices[0].ID)
				}
				if snap.Devices[0].Level == nil || *snap.Devices[0].Level != level {
					t.Fatalf("expected level %v, got %v", level, snap.Devices[0].Level)
				}
				found = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for broadcast message with device on SSE subscriber channel")
		}
	}
}
