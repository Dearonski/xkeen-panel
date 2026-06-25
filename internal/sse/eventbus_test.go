package sse

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestSubscribeLimit(t *testing.T) {
	bus := NewEventBus()

	subs := make([]chan Event, 0, maxClients)
	for i := 0; i < maxClients; i++ {
		ch := bus.Subscribe()
		if ch == nil {
			t.Fatalf("Subscribe #%d вернул nil, ожидался канал", i+1)
		}
		subs = append(subs, ch)
	}

	if extra := bus.Subscribe(); extra != nil {
		t.Fatalf("Subscribe сверх лимита (%d) должен вернуть nil", maxClients)
	}

	bus.Unsubscribe(subs[0])

	if ch := bus.Subscribe(); ch == nil {
		t.Fatal("после Unsubscribe Subscribe должен снова вернуть канал")
	}
}

func TestPublishDelivers(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe вернул nil")
	}

	want := Event{Type: "status", Data: "x"}
	bus.Publish(want)

	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("получено %+v, ожидалось %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("событие не доставлено в течение таймаута")
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe вернул nil")
	}

	bus.Unsubscribe(ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("канал должен быть закрыт после Unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("чтение из закрытого канала зависло")
	}

	// Повторный Subscribe подтверждает, что клиент удалён и слот свободен.
	for i := 0; i < maxClients; i++ {
		if bus.Subscribe() == nil {
			t.Fatalf("слот #%d занят, хотя клиент был удалён", i+1)
		}
	}
}

func TestPublishSlowClientNonBlocking(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe вернул nil")
	}

	for i := 0; i < chanBufferSize+5; i++ {
		bus.Publish(Event{Type: "log", Data: "x"})
	}

	count := 0
drain:
	for {
		select {
		case <-ch:
			count++
		default:
			break drain
		}
	}

	if count != chanBufferSize {
		t.Fatalf("в буфере %d событий, ожидалось %d", count, chanBufferSize)
	}
}

func TestFormatSSE(t *testing.T) {
	got, err := FormatSSE(Event{Type: "log", Data: "hello"})
	if err != nil {
		t.Fatalf("FormatSSE вернул ошибку: %v", err)
	}
	want := []byte("event: log\ndata: hello\n\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("FormatSSE = %q, ожидалось %q", got, want)
	}

	got, err = FormatSSE(Event{Type: "status", Data: map[string]int{"a": 1}})
	if err != nil {
		t.Fatalf("FormatSSE вернул ошибку: %v", err)
	}
	if !bytes.Contains(got, []byte("event: status\n")) {
		t.Fatalf("вывод %q не содержит заголовок события", got)
	}

	jsonData, _ := json.Marshal(map[string]int{"a": 1})
	wantData := append([]byte("data: "), jsonData...)
	wantData = append(wantData, '\n')
	if !bytes.Contains(got, wantData) {
		t.Fatalf("вывод %q не содержит data %q", got, wantData)
	}
}
