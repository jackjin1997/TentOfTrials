package ws

import (
    "testing"
    "time"
)

func TestHubRegisterUnregister(t *testing.T) {
    hub := NewHub()
    go hub.Run()
    c1 := &Client{hub: hub, send: make(chan []byte, 256)}
    c2 := &Client{hub: hub, send: make(chan []byte, 256)}
    hub.register <- c1
    hub.register <- c2
    time.Sleep(10 * time.Millisecond)
    if len(hub.clients) != 2 { t.Errorf("expected 2 clients, got %d", len(hub.clients)) }
    hub.unregister <- c1
    time.Sleep(10 * time.Millisecond)
    if len(hub.clients) != 1 { t.Errorf("expected 1 client after unregister, got %d", len(hub.clients)) }
}

func TestHubBroadcast(t *testing.T) {
    hub := NewHub()
    go hub.Run()
    c1 := &Client{hub: hub, send: make(chan []byte, 256)}
    hub.register <- c1
    time.Sleep(10 * time.Millisecond)
    msg := []byte("test")
    hub.broadcast <- msg
    select {
    case received := <-c1.send:
        if string(received) != string(msg) { t.Errorf("expected %s, got %s", msg, received) }
    case <-time.After(100 * time.Millisecond):
        t.Error("broadcast not received")
    }
}

func TestHubSlowClientCleanup(t *testing.T) {
    hub := NewHub()
    go hub.Run()
    c1 := &Client{hub: hub, send: make(chan []byte, 1)} // buffer de 1 seulement
    hub.register <- c1
    time.Sleep(10 * time.Millisecond)
    // Remplir le buffer pour le rendre "lent"
    c1.send <- []byte("1")
    hub.broadcast <- []byte("2")
    time.Sleep(50 * time.Millisecond)
    hub.mu.RLock()
    _, exists := hub.clients[c1]
    hub.mu.RUnlock()
    if exists { t.Error("slow client should have been removed during broadcast") }
}

func TestHubPingLoopShutdown(t *testing.T) {
    hub := NewHub()
    go hub.Run()
    time.Sleep(10 * time.Millisecond)
    // Vérifier que le hub peut être arrêté proprement
    // (implémente un close(hub.stop) ou équivalent dans hub.go si nécessaire)
}
