package sse

import (
	"encoding/json"
	"fmt"
	"sync"
)

const (
	maxClients     = 4
	chanBufferSize = 16
)

// Event is one SSE message: a type and its payload.
type Event struct {
	Type string
	Data interface{}
}

// EventBus is a pub/sub broker fanning out to per-client channels.
type EventBus struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

func NewEventBus() *EventBus {
	return &EventBus{
		clients: make(map[chan Event]struct{}),
	}
}

// Subscribe registers a client. Returns nil when the client limit is reached.
func (b *EventBus) Subscribe() chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.clients) >= maxClients {
		return nil
	}

	ch := make(chan Event, chanBufferSize)
	b.clients[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a client and closes its channel.
func (b *EventBus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
}

// Publish delivers an event to every subscriber without blocking.
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Slow client: skip it rather than block the producer
		}
	}
}

// FormatSSE renders an event in SSE wire format.
func FormatSSE(event Event) ([]byte, error) {
	var dataStr string

	switch v := event.Data.(type) {
	case string:
		dataStr = v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		dataStr = string(b)
	}

	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, dataStr)), nil
}
