package ws

import (
	"testing"
	"time"
)

// SlowClient simulates a client that blocks on send
type SlowClient struct {
	id   string
	send chan []byte
}

func NewSlowClient(id string) *SlowClient {
	return &SlowClient{
		id:   id,
		send: make(chan []byte), // unbuffered - will block
	}
}

func (c *SlowClient) Send(data []byte) bool {
	select {
	case c.send <- data:
		return true
	default:
		return false
	}
}

func TestHubSlowClientCleanup(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	slow := NewSlowClient("slow")
	fast := NewMockClient("fast")

	hub.Register(slow)
	hub.Register(fast)
	time.Sleep(10 * time.Millisecond)

	// Broadcast should not deadlock due to slow client
	hub.Broadcast([]byte("test"))
	time.Sleep(50 * time.Millisecond)

	// Slow client should be removed after failed send
	if hub.HasClient(slow) {
		t.Error("slow client should be cleaned up")
	}
	if !hub.HasClient(fast) {
		t.Error("fast client should still be registered")
	}
}