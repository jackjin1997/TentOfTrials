package ws

import (
	"testing"
	"time"

	"github.com/tent-of-trials/market/types"
	"go.uber.org/zap"
)

func testHub(t *testing.T) *Hub {
	t.Helper()
	hub := NewHub(zap.NewNop())
	go hub.Run()
	t.Cleanup(func() { time.Sleep(15 * time.Millisecond) })
	return hub
}

func testClient(hub *Hub, buffer int, remote string) *Client {
	return &Client{
		hub:    hub,
		send:   make(chan []byte, buffer),
		subs:   make(map[types.Symbol]struct{}),
		remote: remote,
	}
}

func waitForClients(t *testing.T, hub *Hub, want int) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		got := len(hub.clients)
		hub.mu.RUnlock()
		if got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	hub.mu.RLock()
	got := len(hub.clients)
	hub.mu.RUnlock()
	t.Fatalf("client count = %d, want %d", got, want)
}

func TestHubRegisterAndUnregisterOnce(t *testing.T) {
	hub := testHub(t)
	client := testClient(hub, 8, "127.0.0.1:9001")

	hub.register <- client
	waitForClients(t, hub, 1)

	hub.unregister <- client
	waitForClients(t, hub, 0)

	select {
	case _, open := <-client.send:
		if open {
			t.Fatal("send channel should be closed after unregister")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for closed send channel")
	}

	hub.unregister <- client
	waitForClients(t, hub, 0)
}

func TestHubBroadcastDeliversToActiveClients(t *testing.T) {
	hub := testHub(t)
	first := testClient(hub, 4, "127.0.0.1:9002")
	second := testClient(hub, 4, "127.0.0.1:9003")

	hub.register <- first
	hub.register <- second
	waitForClients(t, hub, 2)

	payload := []byte(`{"type":"trade","symbol":"BTC-USD"}`)
	hub.broadcast <- payload

	for i, client := range []*Client{first, second} {
		select {
		case got := <-client.send:
			if string(got) != string(payload) {
				t.Fatalf("client %d payload = %q, want %q", i, got, payload)
			}
		case <-time.After(150 * time.Millisecond):
			t.Fatalf("client %d did not receive broadcast", i)
		}
	}
}

func TestHubBroadcastDropsSlowClientUnderWriteLock(t *testing.T) {
	hub := testHub(t)
	healthy := testClient(hub, 4, "127.0.0.1:9004")
	slow := testClient(hub, 1, "127.0.0.1:9005")

	hub.register <- healthy
	hub.register <- slow
	waitForClients(t, hub, 2)

	slow.send <- []byte("prefill")
	hub.broadcast <- []byte("overflow")

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		_, slowPresent := hub.clients[slow]
		_, healthyPresent := hub.clients[healthy]
		hub.mu.RUnlock()
		if !slowPresent && healthyPresent {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	hub.mu.RLock()
	_, slowPresent := hub.clients[slow]
	_, healthyPresent := hub.clients[healthy]
	hub.mu.RUnlock()
	if slowPresent {
		t.Fatal("slow client should be removed after broadcast")
	}
	if !healthyPresent {
		t.Fatal("healthy client should remain connected")
	}
}
