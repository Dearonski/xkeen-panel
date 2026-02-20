package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
	"xkeen-panel/internal/models"
	"xkeen-panel/internal/monitor"
	"xkeen-panel/internal/xkeen"
)

type Handlers struct {
	config       *models.Config
	subscription *xkeen.SubscriptionManager
	watchdog     *monitor.Watchdog
}

func NewHandlers(cfg *models.Config, sub *xkeen.SubscriptionManager, wd *monitor.Watchdog) *Handlers {
	return &Handlers{config: cfg, subscription: sub, watchdog: wd}
}

// HandleStatus — GET /api/status
func (h *Handlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	status := h.watchdog.GetStatus()
	writeJSON(w, http.StatusOK, status)
}

// HandleGetSubscription — GET /api/subscription
func (h *Handlers) HandleGetSubscription(w http.ResponseWriter, r *http.Request) {
	data := h.subscription.GetData()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"url":          data.URL,
		"last_updated": data.LastUpdated,
		"server_count": len(data.Servers),
	})
}

// HandleUpdateSubscription — POST /api/subscription
func (h *Handlers) HandleUpdateSubscription(w http.ResponseWriter, r *http.Request) {
	var req models.UpdateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "URL обязателен"})
		return
	}

	servers, err := h.subscription.UpdateURL(req.URL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"server_count": len(servers),
		"servers":      servers,
	})
}

// HandleRefreshSubscription — POST /api/subscription/refresh
func (h *Handlers) HandleRefreshSubscription(w http.ResponseWriter, r *http.Request) {
	servers, err := h.subscription.Refresh()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"server_count": len(servers),
		"servers":      servers,
	})
}

// HandleGetServers — GET /api/servers
func (h *Handlers) HandleGetServers(w http.ResponseWriter, r *http.Request) {
	servers := h.subscription.GetServers()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"servers": servers,
	})
}

// HandleSelectServer — POST /api/servers/select
func (h *Handlers) HandleSelectServer(w http.ResponseWriter, r *http.Request) {
	var req models.SelectServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	log.Printf("[SELECT] Запрос выбора сервера ID=%d", req.ID)

	server, err := h.subscription.SetActive(req.ID)
	if err != nil {
		log.Printf("[SELECT] Ошибка SetActive: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("[SELECT] Сервер: %s (%s:%d), RawURI len=%d", server.Name, server.Address, server.Port, len(server.RawURI))

	// Обновить конфиг Xray (04_outbounds.json) — генерирует полный outbound из URI
	if err := xkeen.UpdateOutbound(h.config.OutboundsFile, server); err != nil {
		log.Printf("[SELECT] Ошибка UpdateOutbound: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "ошибка обновления конфига: " + err.Error()})
		return
	}

	log.Printf("[SELECT] Конфиг обновлён, запускаем рестарт...")

	// Перезапустить Xray — не блокируем ответ
	go func() {
		if _, err := xkeen.Restart(h.config.XKeenPath); err != nil {
			log.Printf("[SELECT] Ошибка рестарта: %v", err)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"server":     server,
		"restarting": true,
	})
}

// HandleCheckServers — POST /api/servers/check
func (h *Handlers) HandleCheckServers(w http.ResponseWriter, r *http.Request) {
	servers := h.subscription.GetServers()
	checked := xkeen.CheckAllLatencies(servers, 3*time.Second)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"servers": checked,
	})
}

// HandleRestart — POST /api/xkeen/restart
func (h *Handlers) HandleRestart(w http.ResponseWriter, r *http.Request) {
	log.Printf("[RESTART-API] Кнопка рестарта нажата, xkeen_path=%s", h.config.XKeenPath)

	output, err := xkeen.Restart(h.config.XKeenPath)
	if err != nil {
		log.Printf("[RESTART-API] Ошибка: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  err.Error(),
			"output": output,
		})
		return
	}

	log.Printf("[RESTART-API] Успешно: %s", output)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "restarted",
		"output":  output,
	})
}

// HandleUpdate — POST /api/xkeen/update
func (h *Handlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	output, err := xkeen.Update(h.config.XKeenPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  err.Error(),
			"output": output,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"output":  output,
	})
}

// HandleLogs — GET /api/logs
func (h *Handlers) HandleLogs(w http.ResponseWriter, r *http.Request) {
	linesStr := r.URL.Query().Get("lines")
	lines := 50
	if linesStr != "" {
		if n, err := strconv.Atoi(linesStr); err == nil && n > 0 {
			lines = n
		}
	}

	logLines := h.watchdog.GetLogs(lines)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lines": logLines,
	})
}
