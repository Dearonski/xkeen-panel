package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
	"xkeen-panel/internal/geoip"
	"xkeen-panel/internal/mihomo"
	"xkeen-panel/internal/models"
	"xkeen-panel/internal/monitor"
	"xkeen-panel/internal/xkeen"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	config       *models.Config
	subscription *xkeen.SubscriptionManager
	watchdog     *monitor.Watchdog
	detector     *xkeen.Detector
	pool         *xkeen.PoolStore
	geoip        *geoip.Matcher
}

func NewHandlers(cfg *models.Config, sub *xkeen.SubscriptionManager, wd *monitor.Watchdog, det *xkeen.Detector, pool *xkeen.PoolStore, matcher *geoip.Matcher) *Handlers {
	return &Handlers{config: cfg, subscription: sub, watchdog: wd, detector: det, pool: pool, geoip: matcher}
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

	resp := map[string]interface{}{
		"server_count": len(servers),
		"servers":      servers,
	}
	if sync, err := h.refreshPool(servers); err != nil {
		resp["pool_error"] = err.Error()
	} else if sync.Changed {
		resp["pool"] = sync
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleRefreshSubscription — POST /api/subscription/refresh
func (h *Handlers) HandleRefreshSubscription(w http.ResponseWriter, r *http.Request) {
	servers, err := h.subscription.Refresh()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	resp := map[string]interface{}{
		"server_count": len(servers),
		"servers":      servers,
	}
	if sync, err := h.refreshPool(servers); err != nil {
		resp["pool_error"] = err.Error()
	} else if sync.Changed {
		resp["pool"] = sync
	}

	writeJSON(w, http.StatusOK, resp)
}

// refreshPool приводит пул к обновлённой подписке. Пул генерируется из URI
// подписки, поэтому смена сервера у провайдера оставляет в нём мёртвый адрес —
// и если протухли все ноды, балансировщику не из чего выбирать.
func (h *Handlers) refreshPool(servers []models.Server) (xkeen.SyncResult, error) {
	top := h.detector.Topology()
	if top.Mode != xkeen.TopologyPool {
		return xkeen.SyncResult{}, nil
	}

	state := h.pool.Get()
	state.BalancerTag = top.BalancerTag
	if len(top.Selectors) > 0 {
		state.Selector = top.Selectors[0]
	}

	result, err := xkeen.RefreshPool(h.detector.Runtime(), h.config.OutboundsFile, h.config.XrayAPIAddr, servers, state,
		xkeen.PoolSelectionFromConfig(h.config, h.geoip))
	if err != nil {
		log.Printf("[POOL] Синхронизация с подпиской не выполнена: %v", err)
		return result, err
	}
	if result.Changed {
		h.detector.InvalidateTopology()
		h.watchdog.Log("[POOL] Пул синхронизирован: +%d, -%d, заменено %d, без перезапуска=%v", len(result.Added), len(result.Removed), len(result.Replaced), result.Live)
	}

	return result, nil
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

	// A manual pick clears the server from the failover blacklist
	h.watchdog.ClearBlacklist(server.RawURI)

	rt := h.detector.Runtime()

	// Mihomo has no Xray outbounds: the panel brings the proxy list in line with
	// the subscription, and the core's own proxy-group picks the node
	if rt.Core == xkeen.CoreMihomo {
		if err := h.syncMihomo(rt); err != nil {
			log.Printf("[SELECT] Ошибка конфига Mihomo: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		go func() {
			if _, err := xkeen.Restart(rt.Dispatcher); err != nil {
				log.Printf("[SELECT] Ошибка рестарта: %v", err)
			}
		}()

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":    true,
			"server":     server,
			"restarting": true,
			"core":       rt.Core,
			"note":       "конфигурация Mihomo синхронизирована с подпиской; активную ноду выбирает proxy-group",
		})
		return
	}

	// In pool mode the balancer picks the node, so a manual choice is an override
	// through the core API: instant, no restart
	if top := h.detector.Topology(); top.Mode == xkeen.TopologyPool {
		if err := h.pinPoolNode(rt, top, server); err != nil {
			log.Printf("[SELECT] Ошибка закрепления ноды: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":    true,
			"server":     server,
			"restarting": false,
			"pinned":     true,
		})
		return
	}

	// Write the core config and validate it before restarting — a broken config
	// would otherwise leave XKeen unable to start
	if err := xkeen.ApplyServer(rt, h.config.OutboundsFile, server); err != nil {
		log.Printf("[SELECT] Ошибка применения конфига: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("[SELECT] Конфиг обновлён и проверен, запускаем рестарт...")

	// Restart the core without blocking the response
	go func() {
		if _, err := xkeen.Restart(rt.Dispatcher); err != nil {
			log.Printf("[SELECT] Ошибка рестарта: %v", err)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"server":     server,
		"restarting": true,
	})
}

// pinPoolNode pins the pool node that carries the chosen server.
//
// The node is matched by endpoint, not by position: the pool holds only the best
// `pool_max_nodes` of the subscription, so a subscription index says nothing
// about which node — if any — serves that server.
func (h *Handlers) pinPoolNode(rt xkeen.Runtime, top xkeen.Topology, server *models.Server) error {
	selector := xkeen.DefaultPoolSelector
	if len(top.Selectors) > 0 {
		selector = top.Selectors[0]
	}

	nodes, err := xkeen.PoolNodes(h.config.OutboundsFile, selector, []models.Server{*server})
	if err != nil {
		return err
	}

	tag := ""
	for _, node := range nodes {
		if node.Server != nil {
			tag = node.Tag
			break
		}
	}
	if tag == "" {
		return fmt.Errorf("сервер %q не входит в пул (в нём %d лучших нод) — выберите один из них или пересоберите пул",
			server.Name, len(nodes))
	}

	if err := xkeen.OverrideBalancerTarget(rt, h.config.XrayAPIAddr, top.BalancerTag, tag); err != nil {
		return fmt.Errorf("%w. Закрепление ноды требует блок api в конфиге Xray — его добавляет `xkeen -sb on`", err)
	}

	// The override lives only in the core's memory — store it to reapply after a restart
	if err := h.pool.SetPinned(tag); err != nil {
		log.Printf("[SELECT] Не удалось сохранить закреплённую ноду: %v", err)
	}

	return nil
}

// syncMihomo brings the proxies in config.yaml in line with the subscription and
// validates the result with the core's own parser (`xkeen -mtest`), rolling the
// file back on failure.
func (h *Handlers) syncMihomo(rt xkeen.Runtime) error {
	cfg, err := mihomo.Read(rt.MihomoConf)
	if err != nil {
		return err
	}

	if _, err := cfg.SyncProxies(h.subscription.GetServers()); err != nil {
		return err
	}
	if err := cfg.Write(); err != nil {
		return err
	}

	if !rt.Installed || rt.Dispatcher == "" {
		return nil
	}

	output, err := xkeen.TestConfig(rt.Dispatcher, rt.Core)
	if err == nil {
		return nil
	}

	if rbErr := mihomo.RestoreBackup(rt.MihomoConf); rbErr != nil {
		return fmt.Errorf("конфигурация mihomo не прошла проверку, откат не удался: %v (%s)", rbErr, output)
	}

	return fmt.Errorf("конфигурация mihomo не прошла проверку, изменения отменены: %s", xkeen.TailLines(output, 4))
}

// HandleMihomoSync — POST /api/mihomo/sync
func (h *Handlers) HandleMihomoSync(w http.ResponseWriter, r *http.Request) {
	rt := h.detector.Runtime()
	if rt.Core != xkeen.CoreMihomo {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "активное ядро — " + rt.Core + "; переключите ядро командой `xkeen -mihomo`",
		})
		return
	}

	if err := h.syncMihomo(rt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	go func() {
		if _, err := xkeen.Restart(rt.Dispatcher); err != nil {
			log.Printf("[MIHOMO] Ошибка рестарта: %v", err)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "restarting": true})
}

// HandleCheckServers — POST /api/servers/check
func (h *Handlers) HandleCheckServers(w http.ResponseWriter, r *http.Request) {
	servers := h.subscription.GetServers()
	timeout := time.Duration(h.config.ProbeTimeoutMs) * time.Millisecond
	checked := xkeen.CheckAllLatencies(servers, timeout, h.config.ProbeConcurrency)
	h.subscription.UpdateLatencies(checked)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"servers": checked,
	})
}

// HandleSetCountry — POST /api/servers/country (manual country override)
func (h *Handlers) HandleSetCountry(w http.ResponseWriter, r *http.Request) {
	var req models.SetCountryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	if err := h.subscription.SetCountryOverride(req.ID, req.Country); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// HandleRestart — POST /api/xkeen/restart
func (h *Handlers) HandleRestart(w http.ResponseWriter, r *http.Request) {
	rt := h.detector.Runtime()
	log.Printf("[RESTART-API] Кнопка рестарта нажата, xkeen=%s", rt.Dispatcher)

	output, err := xkeen.Restart(rt.Dispatcher)
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

// HandleStart — POST /api/xkeen/start
func (h *Handlers) HandleStart(w http.ResponseWriter, r *http.Request) {
	output, err := xkeen.Start(h.detector.Runtime().Dispatcher)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "не удалось запустить: " + err.Error(),
			"output": output,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "output": output})
}

// HandleStop — POST /api/xkeen/stop
func (h *Handlers) HandleStop(w http.ResponseWriter, r *http.Request) {
	output, err := xkeen.Stop(h.detector.Runtime().Dispatcher)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":  "не удалось остановить: " + err.Error(),
			"output": output,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "output": output})
}

// HandleSelfTest — POST /api/xkeen/selftest. Replaces the old CLI update: XKeen
// 2.0 has no `-u` flag, and `-uk`/`-ux` are interactive, so validating the core
// config is both more useful and safer for a web button.
func (h *Handlers) HandleSelfTest(w http.ResponseWriter, r *http.Request) {
	rt := h.detector.Runtime()

	// The core's validator logs a line per config file it reads and prints the
	// verdict last, so the UI gets the tail rather than the whole log
	output, err := xkeen.TestConfig(rt.Dispatcher, rt.Core)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"core":    rt.Core,
			"output":  xkeen.TailLines(output, 4),
			"error":   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"core":    rt.Core,
		"output":  xkeen.TailLines(output, 1),
	})
}

// HandlePoolStatus — GET /api/pool
func (h *Handlers) HandlePoolStatus(w http.ResponseWriter, r *http.Request) {
	rt := h.detector.Runtime()
	top := h.detector.Topology()
	state := h.pool.Get()

	resp := map[string]interface{}{
		"mode":         top.Mode,
		"balancer_tag": top.BalancerTag,
		"pool_tags":    top.PoolTags,
		"proxy_tags":   top.ProxyTags,
		"pinned_tag":   state.PinnedTag,
		"core":         rt.Core,
	}

	// The current node is only knowable through the core API, which may be absent
	if top.Mode == xkeen.TopologyPool {
		if target, err := xkeen.CurrentBalancerTarget(rt, h.config.XrayAPIAddr, top.BalancerTag); err == nil {
			resp["current_tag"] = target
			resp["api_available"] = true
		} else {
			resp["api_available"] = false
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandlePoolEnable — POST /api/pool/enable. Builds a pool from the subscription
// and moves the routing rules onto the balancer.
func (h *Handlers) HandlePoolEnable(w http.ResponseWriter, r *http.Request) {
	servers := h.subscription.GetServers()
	if len(servers) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "подписка пуста — нечего добавлять в пул"})
		return
	}

	rt := h.detector.Runtime()
	state, err := xkeen.EnablePool(rt, h.config.OutboundsFile, servers, xkeen.PoolOptions{
		APIAddr:   h.config.XrayAPIAddr,
		Selection: xkeen.PoolSelectionFromConfig(h.config, h.geoip),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if err := h.pool.Set(state); err != nil {
		log.Printf("[POOL] Не удалось сохранить состояние пула: %v", err)
	}
	h.detector.InvalidateTopology()

	go func() {
		if _, err := xkeen.Restart(rt.Dispatcher); err != nil {
			log.Printf("[POOL] Ошибка рестарта: %v", err)
			return
		}
		h.pinAfterRestart(rt)
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"balancer_tag": state.BalancerTag,
		"restarting":   true,
	})
}

// pinAfterRestart waits for the core to come back and pins one node.
//
// A fresh pool selects an outbound per connection, so the outgoing IP moves
// between nodes and anything IP-bound — Telegram sessions, CDN anti-abuse —
// breaks. Pinning right after the restart avoids a window of that behaviour
// before the watchdog's next tick.
func (h *Handlers) pinAfterRestart(rt xkeen.Runtime) {
	deadline := time.Now().Add(90 * time.Second)
	for xkeen.IsRestarting() && time.Now().Before(deadline) {
		time.Sleep(time.Second)
	}
	// The core needs a moment past process start before its API answers
	time.Sleep(3 * time.Second)

	h.detector.InvalidateTopology()
	top := h.detector.Topology()
	if top.Mode != xkeen.TopologyPool {
		return
	}

	tag, err := xkeen.PinBestNode(rt, h.config.XrayAPIAddr, h.config.OutboundsFile, top,
		h.subscription.GetServers(), nil,
		time.Duration(h.config.ProbeTimeoutMs)*time.Millisecond, h.config.ProbeConcurrency)
	if err != nil {
		h.watchdog.Log("[PIN] Не удалось закрепить ноду: %v", err)
		return
	}

	if err := h.pool.SetPinned(tag); err != nil {
		log.Printf("[PIN] Не удалось сохранить закрепление: %v", err)
	}
	h.watchdog.Log("[PIN] Трафик закреплён за нодой %s", tag)
}

// HandlePoolDisable — POST /api/pool/disable. Returns the config to a single
// outbound carrying the active server.
func (h *Handlers) HandlePoolDisable(w http.ResponseWriter, r *http.Request) {
	server := h.subscription.GetActiveServer()
	if server == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "нет активного сервера, к которому можно вернуться"})
		return
	}

	rt := h.detector.Runtime()
	if err := xkeen.DisablePool(rt, h.config.OutboundsFile, server, h.pool.Get()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if err := h.pool.Set(xkeen.PoolState{}); err != nil {
		log.Printf("[POOL] Не удалось очистить состояние пула: %v", err)
	}
	h.detector.InvalidateTopology()

	go func() {
		if _, err := xkeen.Restart(rt.Dispatcher); err != nil {
			log.Printf("[POOL] Ошибка рестарта: %v", err)
		}
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "restarting": true})
}

// HandlePoolSync — POST /api/pool/sync. Brings pool membership in line with the subscription.
func (h *Handlers) HandlePoolSync(w http.ResponseWriter, r *http.Request) {
	rt := h.detector.Runtime()
	state := h.pool.Get()
	if state.Selector == "" {
		state.Selector = xkeen.DefaultPoolSelector
	}

	result, err := xkeen.RefreshPool(rt, h.config.OutboundsFile, h.config.XrayAPIAddr, h.subscription.GetServers(), state,
		xkeen.PoolSelectionFromConfig(h.config, h.geoip))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if result.Changed {
		h.detector.InvalidateTopology()
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"changed":    result.Changed,
		"added":      result.Added,
		"removed":    result.Removed,
		"replaced":   result.Replaced,
		"live":       result.Live,
		"restarting": result.Restarted,
	})
}

// HandleGetSettings — GET /api/xkeen/settings (contents of xkeen.json)
func (h *Handlers) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	rt := h.detector.Runtime()

	cfg, err := xkeen.ReadSettings(rt.XkeenJSON)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":     rt.XkeenJSON,
		"settings": cfg,
	})
}

// HandleUpdateSettings — PUT /api/xkeen/settings
func (h *Handlers) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Settings map[string]interface{} `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}
	if req.Settings == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "поле settings обязательно"})
		return
	}

	if err := xkeen.WriteSettings(h.detector.Runtime().XkeenJSON, req.Settings); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// HandleGetList — GET /api/xkeen/lists/{name} (ports and IP exclusions)
func (h *Handlers) HandleGetList(w http.ResponseWriter, r *http.Request) {
	path, kind, err := xkeen.ListPath(h.detector.Runtime(), chi.URLParam(r, "name"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":    path,
		"kind":    kind,
		"content": string(content),
	})
}

// HandleUpdateList — PUT /api/xkeen/lists/{name}
func (h *Handlers) HandleUpdateList(w http.ResponseWriter, r *http.Request) {
	path, kind, err := xkeen.ListPath(h.detector.Runtime(), chi.URLParam(r, "name"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	if err := xkeen.ValidateList(req.Content, kind); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := xkeen.WriteList(path, req.Content); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// The init script reads these lists on start — without a restart nothing changes
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success":        true,
		"restart_needed": true,
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
