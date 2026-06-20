package ws

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHub_Lifecycle_Register_Unregister(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	
	// Start hub in background
	go hub.Run()

	client := &Client{
		hub:    hub,
		send:   make(chan []byte, 10),
		remote: "1.2.3.4:1234",
	}

	// 1. Register client
	hub.register <- client
	
	// Give loop time to process
	time.Sleep(10 * time.Millisecond)

	hub.mu.RLock()
	_, exists := hub.clients[client]
	hub.mu.RUnlock()
	if !exists {
		t.Fatalf("expected client to be registered")
	}

	// 2. Unregister client
	hub.unregister <- client
	
	// Give loop time to process
	time.Sleep(10 * time.Millisecond)

	hub.mu.RLock()
	_, exists = hub.clients[client]
	hub.mu.RUnlock()
	if exists {
		t.Fatalf("expected client to be unregistered")
	}

	// Verify channel is closed
	_, open := <-client.send
	if open {
		t.Fatalf("expected client.send channel to be closed after unregister")
	}
}

func TestHub_Broadcast(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	client1 := &Client{
		hub:    hub,
		send:   make(chan []byte, 10),
		remote: "1.2.3.4:1001",
	}
	client2 := &Client{
		hub:    hub,
		send:   make(chan []byte, 10),
		remote: "1.2.3.4:1002",
	}

	hub.register <- client1
	hub.register <- client2
	time.Sleep(10 * time.Millisecond)

	// Broadcast a message
	msg := []byte("hello market")
	hub.broadcast <- msg

	// Verify both clients receive the broadcast message
	select {
	case received := <-client1.send:
		if string(received) != "hello market" {
			t.Errorf("client 1 received incorrect message: %s", string(received))
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("client 1 did not receive broadcast")
	}

	select {
	case received := <-client2.send:
		if string(received) != "hello market" {
			t.Errorf("client 2 received incorrect message: %s", string(received))
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("client 2 did not receive broadcast")
	}
}

func TestHub_SlowClientCleanup(t *testing.T) {
	logger := zap.NewNop()
	hub := NewHub(logger)
	go hub.Run()

	// Client 1 has a very small buffer of 1 and we fill it up
	client1 := &Client{
		hub:    hub,
		send:   make(chan []byte, 1),
		remote: "1.2.3.4:1001",
	}
	// Client 2 has standard buffer
	client2 := &Client{
		hub:    hub,
		send:   make(chan []byte, 10),
		remote: "1.2.3.4:1002",
	}

	hub.register <- client1
	hub.register <- client2
	time.Sleep(10 * time.Millisecond)

	// Fill client1's buffer
	client1.send <- []byte("fill")

	// Broadcast should trigger default case (client1's buffer is full)
	hub.broadcast <- []byte("broadcast-message")
	time.Sleep(10 * time.Millisecond)

	// Client 1 should be removed from hub and its send channel closed
	hub.mu.RLock()
	_, exists1 := hub.clients[client1]
	_, exists2 := hub.clients[client2]
	hub.mu.RUnlock()

	if exists1 {
		t.Errorf("expected client1 (slow) to be removed from clients map")
	}
	if !exists2 {
		t.Errorf("expected client2 (active) to remain in clients map")
	}

	// Verify client1's channel is closed
	_, open := <-client1.send
	// Since we manually filled it with "fill", the first read is "fill", next is close
	if open {
		// Read second time (which should be close or broadcast-message if race, but channel should be closed)
		_, open = <-client1.send
		if open {
			t.Errorf("expected client1 channel to be closed")
		}
	}
}
