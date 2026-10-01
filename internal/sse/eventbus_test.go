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
			t.Fatalf("Subscribe #%d returned nil, want a channel", i+1)
		}
		subs = append(subs, ch)
	}

	if extra := bus.Subscribe(); extra != nil {
		t.Fatalf("Subscribe past the limit (%d) must return nil", maxClients)
	}

	bus.Unsubscribe(subs[0])

	if ch := bus.Subscribe(); ch == nil {
		t.Fatal("after Unsubscribe, Subscribe must return a channel again")
	}
}

func TestPublishDelivers(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe returned nil")
	}

	want := Event{Type: "status", Data: "x"}
	bus.Publish(want)

	select {
	case got := <-ch:
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("event not delivered within the timeout")
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe returned nil")
	}

	bus.Unsubscribe(ch)

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("channel must be closed after Unsubscribe")
		}
	case <-time.After(time.Second):
		t.Fatal("reading from the closed channel hung")
	}

	// Subscribing again proves the client was removed and its slot freed.
	for i := 0; i < maxClients; i++ {
		if bus.Subscribe() == nil {
			t.Fatalf("slot #%d is taken although the client was removed", i+1)
		}
	}
}

func TestPublishSlowClientNonBlocking(t *testing.T) {
	bus := NewEventBus()
	ch := bus.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe returned nil")
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
		t.Fatalf("buffer holds %d events, want %d", count, chanBufferSize)
	}
}

func TestFormatSSE(t *testing.T) {
	got, err := FormatSSE(Event{Type: "log", Data: "hello"})
	if err != nil {
		t.Fatalf("FormatSSE returned an error: %v", err)
	}
	want := []byte("event: log\ndata: hello\n\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("FormatSSE = %q, want %q", got, want)
	}

	got, err = FormatSSE(Event{Type: "status", Data: map[string]int{"a": 1}})
	if err != nil {
		t.Fatalf("FormatSSE returned an error: %v", err)
	}
	if !bytes.Contains(got, []byte("event: status\n")) {
		t.Fatalf("output %q has no event header", got)
	}

	jsonData, _ := json.Marshal(map[string]int{"a": 1})
	wantData := append([]byte("data: "), jsonData...)
	wantData = append(wantData, '\n')
	if !bytes.Contains(got, wantData) {
		t.Fatalf("output %q has no data %q", got, wantData)
	}
}
