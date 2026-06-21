package ws

import (
	"sync"
	"testing"
	"time"
)

// MockClient implements a test client for hub tests
type MockClient struct {
	id       string
	send     chan []byte
	closed   bool
	closeMu  sync.Mutex
	received [][]byte
}

func NewMockClient(id string) *MockClient {
	return &MockClient{
		id:   id,
		send: make(chan []byte, 10),
	}
}

func (c *MockClient) Send(data []byte) bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- data:
		c.received = append(c.received, data)
		return true
	default:
		return false
	}
}

func (c *MockClient) Close() {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	c.closed = true
	close(c.send)
}