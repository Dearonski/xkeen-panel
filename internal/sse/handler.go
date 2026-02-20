package sse

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
	"xkeen-panel/internal/models"
	"xkeen-panel/internal/xkeen"
)

// StatusProvider — интерфейс для получения текущего статуса (watchdog)
type StatusProvider interface {
	GetStatus() models.Status
}

// HandleEvents — SSE-поток статуса, логов и рестарт-событий
func HandleEvents(bus *EventBus, sp StatusProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		ch := bus.Subscribe()
		if ch == nil {
			http.Error(w, `{"error":"too many SSE clients"}`, http.StatusServiceUnavailable)
			return
		}
		defer bus.Unsubscribe(ch)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Отправить текущий статус при подключении
		initial := Event{Type: "status", Data: sp.GetStatus()}
		if data, err := FormatSSE(initial); err == nil {
			w.Write(data)
			flusher.Flush()
		}

		// Стримить события из EventBus
		for {
			select {
			case <-r.Context().Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				data, err := FormatSSE(evt)
				if err != nil {
					log.Printf("[SSE] Ошибка сериализации: %v", err)
					continue
				}
				if _, err := w.Write(data); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// HandleStreamLatency — SSE-поток проверки латенси серверов
func HandleStreamLatency(sub *xkeen.SubscriptionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		servers := sub.GetServers()
		if len(servers) == 0 {
			evt := Event{Type: "done", Data: map[string]bool{"complete": true}}
			if data, err := FormatSSE(evt); err == nil {
				w.Write(data)
				flusher.Flush()
			}
			return
		}

		type result struct {
			ID      int `json:"id"`
			Latency int `json:"latency_ms"`
		}

		results := make(chan result, len(servers))
		var wg sync.WaitGroup

		for _, s := range servers {
			wg.Add(1)
			go func(srv models.Server) {
				defer wg.Done()
				latency := xkeen.CheckLatency(srv.Address, srv.Port, 3*time.Second)
				results <- result{ID: srv.ID, Latency: latency}
			}(s)
		}

		// Закрыть канал после завершения всех горутин
		go func() {
			wg.Wait()
			close(results)
		}()

		for res := range results {
			if r.Context().Err() != nil {
				return
			}

			evt := Event{Type: "latency", Data: res}
			data, err := FormatSSE(evt)
			if err != nil {
				continue
			}
			if _, err := w.Write(data); err != nil {
				return
			}
			flusher.Flush()
		}

		// Финальное событие
		done := Event{Type: "done", Data: map[string]bool{"complete": true}}
		if data, err := FormatSSE(done); err == nil {
			w.Write(data)
			flusher.Flush()
		}

		// Дать браузеру время прочитать финальное событие
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}

		fmt.Fprint(w, "event: close\ndata: {}\n\n")
		flusher.Flush()
	}
}
