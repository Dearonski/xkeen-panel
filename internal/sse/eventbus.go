package sse

import (
	"encoding/json"
	"fmt"
	"sync"
)

const (
	maxClients    = 4
	chanBufferSize = 16
)

// Event — SSE-событие с типом и данными
type Event struct {
	Type string
	Data interface{}
}

// EventBus — pub/sub брокер с fan-out по каналам клиентов
type EventBus struct {
	mu      sync.RWMutex
	clients map[chan Event]struct{}
}

func NewEventBus() *EventBus {
	return &EventBus{
		clients: make(map[chan Event]struct{}),
	}
}

// Subscribe регистрирует нового клиента. Возвращает nil если лимит превышен.
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

// Unsubscribe удаляет клиента и закрывает канал
func (b *EventBus) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.clients[ch]; ok {
		delete(b.clients, ch)
		close(ch)
	}
}

// Publish отправляет событие всем подписчикам (non-blocking)
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Медленный клиент — пропускаем, не блокируем продюсера
		}
	}
}

// FormatSSE сериализует событие в формат SSE
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
