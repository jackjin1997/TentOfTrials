package ws

import (
	"testing"
	"time"

	"github.com/tent-of-trials/market/types"
	"go.uber.org/zap"
)

func newTestHub(t *testing.T) *Hub {
	t.Helper()

	hub := NewHub(zap.NewNop())
	go hub.Run()
	return hub
}

func newTestClient(hub *Hub, sendCapacity int) *Client {
	return &Client{
		hub:    hub,
		send:   make(chan []byte, sendCapacity),
		subs:   make(map[types.Symbol]struct{}),
		remote: "test-client",
	}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition was not met before timeout")
}

func hubClientCount(hub *Hub) int {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.clients)
}

func hubHasClient(hub *Hub, client *Client) bool {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	_, ok := hub.clients[client]
	return ok
}

func TestHubRegisterAndUnregisterRemoveClientOnce(t *testing.T) {
	hub := newTestHub(t)
	client := newTestClient(hub, 1)

	hub.register <- client
	eventually(t, func() bool { return hubClientCount(hub) == 1 })

	hub.unregister <- client
	eventually(t, func() bool { return hubClientCount(hub) == 0 })

	select {
	case _, ok := <-client.send:
		if ok {
			t.Fatalf("expected unregister to close client send channel")
		}
	default:
		t.Fatalf("expected unregister to close client send channel")
	}

	hub.unregister <- client
	eventually(t, func() bool { return hubClientCount(hub) == 0 })
}

func TestHubBroadcastDeliversToActiveClient(t *testing.T) {
	hub := newTestHub(t)
	client := newTestClient(hub, 1)
	message := []byte("price-update")

	hub.register <- client
	eventually(t, func() bool { return hubHasClient(hub, client) })

	hub.broadcast <- message

	select {
	case got := <-client.send:
		if string(got) != string(message) {
			t.Fatalf("broadcast message = %q, want %q", got, message)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("timed out waiting for broadcast delivery")
	}
}

func TestHubBroadcastRemovesSlowClient(t *testing.T) {
	hub := newTestHub(t)
	client := newTestClient(hub, 1)
	client.send <- []byte("backlog")

	hub.register <- client
	eventually(t, func() bool { return hubHasClient(hub, client) })

	hub.broadcast <- []byte("new-message")

	eventually(t, func() bool { return !hubHasClient(hub, client) })

	if _, ok := <-client.send; !ok {
		t.Fatalf("expected buffered backlog before closed channel is observed")
	}
	if _, ok := <-client.send; ok {
		t.Fatalf("expected slow client channel to be closed after cleanup")
	}
}
