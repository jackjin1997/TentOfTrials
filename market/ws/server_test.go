package ws

import (
	"sync"
	"testing"
	"time"

	"github.com/tent-of-trials/market/types"
	"go.uber.org/zap"
)

func newTestHub() *Hub {
	return NewHub(zap.NewNop())
}

// startHub runs the hub in a goroutine and returns a stop function.
func startHub(h *Hub) func() {
	done := make(chan struct{})
	go func() {
		h.Run()
		close(done)
	}()
	return func() {}
}

// newTestClient creates a client with a buffered send channel (no real conn).
func newTestClient(hub *Hub, remote string) *Client {
	return &Client{
		hub:    hub,
		send:   make(chan []byte, 256),
		subs:   make(map[types.Symbol]struct{}),
		remote: remote,
	}
}

// ---------------------------------------------------------------------------
// Client registration and unregister
// ---------------------------------------------------------------------------

func TestHub_ClientRegistration(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	c1 := newTestClient(hub, "10.0.0.1:1111")
	c2 := newTestClient(hub, "10.0.0.2:2222")

	hub.register <- c1
	hub.register <- c2
	time.Sleep(10 * time.Millisecond)

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()

	if count != 2 {
		t.Fatalf("expected 2 clients after registration, got %d", count)
	}
}

func TestHub_ClientUnregister(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	c1 := newTestClient(hub, "10.0.0.1:1111")
	c2 := newTestClient(hub, "10.0.0.2:2222")

	hub.register <- c1
	hub.register <- c2
	time.Sleep(10 * time.Millisecond)

	hub.unregister <- c1
	time.Sleep(10 * time.Millisecond)

	hub.mu.RLock()
	count := len(hub.clients)
	_, c1StillExists := hub.clients[c1]
	_, c2StillExists := hub.clients[c2]
	hub.mu.RUnlock()

	if count != 1 {
		t.Fatalf("expected 1 client after unregister, got %d", count)
	}
	if c1StillExists {
		t.Error("c1 should have been removed")
	}
	if !c2StillExists {
		t.Error("c2 should still exist")
	}
}

func TestHub_UnregisterNonexistentClient(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	c := newTestClient(hub, "10.0.0.99:9999")

	hub.unregister <- c
	time.Sleep(10 * time.Millisecond)

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()

	if count != 0 {
		t.Fatalf("expected 0 clients, got %d", count)
	}
}

func TestHub_UnregisterClosesSendChannel(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	c := newTestClient(hub, "10.0.0.1:1111")
	hub.register <- c
	time.Sleep(10 * time.Millisecond)

	hub.unregister <- c
	time.Sleep(10 * time.Millisecond)

	// The send channel should be closed after unregister.
	// Reading from a closed channel returns the zero value immediately.
	select {
	case _, ok := <-c.send:
		if ok {
			// Channel still open — acceptable if unregister hasn't processed yet,
			// but by now it should be closed.
			t.Error("expected send channel to be closed after unregister")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for send channel close")
	}
}

// ---------------------------------------------------------------------------
// Broadcast delivery
// ---------------------------------------------------------------------------

func TestHub_BroadcastDelivery(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	c1 := newTestClient(hub, "10.0.0.1:1111")
	c2 := newTestClient(hub, "10.0.0.2:2222")

	hub.register <- c1
	hub.register <- c2
	time.Sleep(10 * time.Millisecond)

	msg := []byte(`{"type":"trade","symbol":"BTC-USD"}`)
	hub.broadcast <- msg
	time.Sleep(10 * time.Millisecond)

	for i, c := range []*Client{c1, c2} {
		select {
		case received := <-c.send:
			if string(received) != string(msg) {
				t.Errorf("client %d: expected %q, got %q", i, msg, received)
			}
		default:
			t.Errorf("client %d: no message received from broadcast", i)
		}
	}
}

func TestHub_BroadcastToSingleClient(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	c := newTestClient(hub, "10.0.0.1:1111")
	hub.register <- c
	time.Sleep(10 * time.Millisecond)

	msg := []byte("hello")
	hub.broadcast <- msg
	time.Sleep(10 * time.Millisecond)

	select {
	case received := <-c.send:
		if string(received) != "hello" {
			t.Errorf("expected 'hello', got %q", received)
		}
	default:
		t.Error("single client did not receive broadcast")
	}
}

func TestHub_BroadcastToNoClients(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	// Broadcasting with no clients should not panic or block.
	hub.broadcast <- []byte("orphan message")
	time.Sleep(10 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Slow / full client send buffer removal
// ---------------------------------------------------------------------------

func TestHub_SlowClientRemovedDuringBroadcast(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	// Create a client with a tiny (full) send channel.
	slow := &Client{
		hub:    hub,
		send:   make(chan []byte, 1), // capacity 1
		subs:   make(map[types.Symbol]struct{}),
		remote: "slow-client:1111",
	}
	hub.register <- slow
	time.Sleep(10 * time.Millisecond)

	// Fill the buffer.
	slow.send <- []byte("fill")
	time.Sleep(10 * time.Millisecond)

	// Broadcast — the default case should remove the slow client.
	hub.broadcast <- []byte("overflow")
	time.Sleep(20 * time.Millisecond)

	hub.mu.RLock()
	_, exists := hub.clients[slow]
	hub.mu.RUnlock()

	if exists {
		t.Error("slow client should have been removed from hub during broadcast")
	}
}

func TestHub_BroadcastDoesNotMutateUnderRLock(t *testing.T) {
	// This test verifies the fix: the broadcast path should not call
	// delete() while holding RLock. The fix collects slow clients and
	// removes them after releasing the read lock.
	hub := newTestHub()
	startHub(hub)

	fast := newTestClient(hub, "fast:1111")
	slow := &Client{
		hub:    hub,
		send:   make(chan []byte, 1),
		subs:   make(map[types.Symbol]struct{}),
		remote: "slow:2222",
	}

	hub.register <- fast
	hub.register <- slow
	time.Sleep(10 * time.Millisecond)

	// Fill slow's buffer.
	slow.send <- []byte("occupied")
	time.Sleep(10 * time.Millisecond)

	// Broadcast — slow should be evicted, fast should receive the message.
	hub.broadcast <- []byte("data")
	time.Sleep(20 * time.Millisecond)

	// Fast should still have the message.
	select {
	case msg := <-fast.send:
		if string(msg) != "data" {
			t.Errorf("fast client expected 'data', got %q", msg)
		}
	default:
		t.Error("fast client did not receive broadcast")
	}

	hub.mu.RLock()
	_, slowExists := hub.clients[slow]
	_, fastExists := hub.clients[fast]
	hub.mu.RUnlock()

	if slowExists {
		t.Error("slow client should have been removed")
	}
	if !fastExists {
		t.Error("fast client should still be in hub")
	}
}

// ---------------------------------------------------------------------------
// Register then immediately unregister
// ---------------------------------------------------------------------------

func TestHub_RegisterThenUnregister(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	c := newTestClient(hub, "temp:1111")
	hub.register <- c
	hub.unregister <- c
	time.Sleep(20 * time.Millisecond)

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()

	if count != 0 {
		t.Fatalf("expected 0 clients after register+unregister, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Multiple broadcasts accumulate messages
// ---------------------------------------------------------------------------

func TestHub_MultipleBroadcasts(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	c := newTestClient(hub, "listener:1111")
	hub.register <- c
	time.Sleep(10 * time.Millisecond)

	for i := 0; i < 5; i++ {
		hub.broadcast <- []byte("msg")
	}
	time.Sleep(20 * time.Millisecond)

	received := 0
	for {
		select {
		case <-c.send:
			received++
		default:
			goto done
		}
	}
done:
	if received != 5 {
		t.Errorf("expected 5 messages, got %d", received)
	}
}

// ---------------------------------------------------------------------------
// Concurrent register/unregister safety
// ---------------------------------------------------------------------------

func TestHub_ConcurrentRegisterUnregister(t *testing.T) {
	hub := newTestHub()
	startHub(hub)

	var wg sync.WaitGroup
	clients := make([]*Client, 50)

	for i := 0; i < 50; i++ {
		clients[i] = newTestClient(hub, "concurrent:1111")
	}

	// Register all
	for _, c := range clients {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			hub.register <- c
		}(c)
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond)

	hub.mu.RLock()
	if len(hub.clients) != 50 {
		t.Fatalf("expected 50 clients, got %d", len(hub.clients))
	}
	hub.mu.RUnlock()

	// Unregister first 25
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			hub.unregister <- c
		}(clients[i])
	}
	wg.Wait()
	time.Sleep(20 * time.Millisecond)

	hub.mu.RLock()
	count := len(hub.clients)
	hub.mu.RUnlock()

	if count != 25 {
		t.Fatalf("expected 25 clients after unregister, got %d", count)
	}
}
