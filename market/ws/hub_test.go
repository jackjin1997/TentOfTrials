package ws

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestHub() *Hub {
	return NewHub(zap.NewNop())
}

func newTestClient(hub *Hub, bufSize int) *Client {
	return &Client{
		hub:  hub,
		send: make(chan []byte, bufSize),
	}
}

func waitForHubState(t *testing.T, hub *Hub, expected int, timeout time.Duration) []*Client {
	t.Helper()
	deadline := time.After(timeout)
	for {
		hub.mu.RLock()
		count := len(hub.clients)
		if count == expected {
			// Build a snapshot of clients
			snapshot := make([]*Client, 0, count)
			for c := range hub.clients {
				snapshot = append(snapshot, c)
			}
			hub.mu.RUnlock()
			return snapshot
		}
		hub.mu.RUnlock()
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for hub to have %d clients, got %d", expected, count)
			return nil
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func TestHub_RegisterClient(t *testing.T) {
	hub := newTestHub()
	go hub.Run()

	client := newTestClient(hub, 256)
	hub.register <- client

	snapshot := waitForHubState(t, hub, 1, time.Second)
	if snapshot[0] != client {
		t.Error("registered client not found in hub")
	}
}

func TestHub_RegisterMultipleClients(t *testing.T) {
	hub := newTestHub()
	go hub.Run()

	const count = 5
	for i := 0; i < count; i++ {
		client := newTestClient(hub, 256)
		hub.register <- client
	}

	waitForHubState(t, hub, count, time.Second)
}

// ---------------------------------------------------------------------------
// Unregister
// ---------------------------------------------------------------------------

func TestHub_UnregisterClient(t *testing.T) {
	hub := newTestHub()
	go hub.Run()

	client := newTestClient(hub, 256)
	hub.register <- client
	waitForHubState(t, hub, 1, time.Second)

	hub.unregister <- client
	waitForHubState(t, hub, 0, time.Second)
}

func TestHub_UnregisterClosesSendChannel(t *testing.T) {
	hub := newTestHub()
	go hub.Run()

	client := newTestClient(hub, 256)
	hub.register <- client
	waitForHubState(t, hub, 1, time.Second)

	hub.unregister <- client
	waitForHubState(t, hub, 0, time.Second)

	// Send channel should be closed after unregister
	_, ok := <-client.send
	if ok {
		t.Error("send channel should be closed after unregister")
	}
}

func TestHub_UnregisterTwice_NoPanic(t *testing.T) {
	hub := newTestHub()
	go hub.Run()

	client := newTestClient(hub, 256)
	hub.register <- client
	waitForHubState(t, hub, 1, time.Second)

	hub.unregister <- client
	waitForHubState(t, hub, 0, time.Second)

	// Second unregister should not panic
	hub.unregister <- client
	// Give hub time to process — it should just be a no-op
	time.Sleep(50 * time.Millisecond)
	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 clients after second unregister, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Broadcast
// ---------------------------------------------------------------------------

func TestHub_BroadcastDeliversToActiveClients(t *testing.T) {
	hub := newTestHub()
	go hub.Run()

	// Register 3 clients
	clients := make([]*Client, 3)
	for i := range clients {
		clients[i] = newTestClient(hub, 256)
		hub.register <- clients[i]
	}
	waitForHubState(t, hub, 3, time.Second)

	// Broadcast a message
	msg := []byte("hello")
	hub.broadcast <- msg

	// All clients should receive the message
	for i, c := range clients {
		select {
		case received := <-c.send:
			if string(received) != string(msg) {
				t.Errorf("client %d: expected %q, got %q", i, string(msg), string(received))
			}
		case <-time.After(time.Second):
			t.Errorf("client %d: timed out waiting for broadcast message", i)
		}
	}
}

func TestHub_BroadcastDoesNotDeliverToUnregisteredClients(t *testing.T) {
	hub := newTestHub()
	go hub.Run()

	client := newTestClient(hub, 256)
	hub.register <- client
	waitForHubState(t, hub, 1, time.Second)

	hub.unregister <- client
	waitForHubState(t, hub, 0, time.Second)

	// After unregister, the client is no longer in the hub (count = 0).
	// Broadcast should not panic or side-effect.
	hub.broadcast <- []byte("should be safe")

	// Verify the hub state is still clean after broadcast
	waitForHubState(t, hub, 0, 2*time.Second)
}

// ---------------------------------------------------------------------------
// Slow client removal (the locking race)
// ---------------------------------------------------------------------------

func TestHub_SlowClientRemovedOnBroadcast(t *testing.T) {
	hub := newTestHub()
	go hub.Run()

	// Create a client with a tiny send buffer (1)
	slowClient := newTestClient(hub, 1)
	hub.register <- slowClient

	// Create a normal client
	normalClient := newTestClient(hub, 256)
	hub.register <- normalClient

	waitForHubState(t, hub, 2, time.Second)

	// Fill the slow client's send buffer
	slowClient.send <- []byte("fill")

	// Broadcast should cause the slow client to be removed
	hub.broadcast <- []byte("test message")

	// Normal client should still receive
	select {
	case <-normalClient.send:
		// OK
	case <-time.After(time.Second):
		t.Error("normal client did not receive broadcast")
	}

	// Slow client should have been removed
	waitForHubState(t, hub, 1, time.Second)

	hub.mu.RLock()
	found := false
	for c := range hub.clients {
		if c == slowClient {
			found = true
			break
		}
	}
	hub.mu.RUnlock()
	if found {
		t.Error("slow client should have been removed from hub after broadcast")
	}
}

func TestHub_SlowClientSendChannelClosed(t *testing.T) {
	hub := newTestHub()
	go hub.Run()

	client := newTestClient(hub, 1)
	hub.register <- client
	waitForHubState(t, hub, 1, time.Second)

	// Fill buffer and broadcast to trigger removal
	client.send <- []byte("fill")
	hub.broadcast <- []byte("trigger removal")

	waitForHubState(t, hub, 0, time.Second)

	// Drain any remaining buffered messages first
	<-client.send // drain "fill"

	// Now reading from closed (and drained) channel should give ok=false
	_, ok := <-client.send
	if ok {
		t.Error("slow client's send channel should be closed after forced removal")
	}
}
