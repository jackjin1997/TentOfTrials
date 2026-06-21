package ws

import (
	"sync"
	"testing"
	"time"
)

func TestHubClientRegistration(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	client := NewMockClient("test-1")
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)

	if !hub.HasClient(client) {
		t.Error("client should be registered")
	}
}

func TestHubClientUnregistration(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	client := NewMockClient("test-2")
	hub.Register(client)
	time.Sleep(10 * time.Millisecond)
	hub.Unregister(client)
	time.Sleep(10 * time.Millisecond)

	if hub.HasClient(client) {
		t.Error("client should be unregistered")
	}
}

func TestHubBroadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer hub.Stop()

	c1 := NewMockClient("c1")
	c2 := NewMockClient("c2")
	hub.Register(c1)
	hub.Register(c2)
	time.Sleep(10 * time.Millisecond)

	hub.Broadcast([]byte("hello"))
	time.Sleep(10 * time.Millisecond)

	if len(c1.received) != 1 || len(c2.received) != 1 {
		t.Error("both clients should receive broadcast")
	}
}