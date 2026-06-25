package monitor

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"xkeen-panel/internal/geoip"
	"xkeen-panel/internal/models"
	"xkeen-panel/internal/sse"
	"xkeen-panel/internal/xkeen"
)

type Watchdog struct {
	config       *models.Config
	subscription *xkeen.SubscriptionManager
	mu           sync.RWMutex
	active       bool
	failCount    int
	latencyHigh  int
	lastCheck    time.Time
	lastLatency  int
	connected    bool
	startTime    time.Time
	logs         []string
	logFile      *os.File
	eventBus     *sse.EventBus
	geoip        *geoip.Matcher
	blacklist    map[string]time.Time // ключ — RawURI, устойчив к переиндексации
}

func NewWatchdog(cfg *models.Config, sub *xkeen.SubscriptionManager) *Watchdog {
	return &Watchdog{
		config:       cfg,
		subscription: sub,
		active:       false, // По умолчанию выключен — включается вручную из UI
		startTime:    time.Now(),
		lastLatency:  -1,
		blacklist:    make(map[string]time.Time),
	}
}

// SetEventBus устанавливает шину событий для SSE-уведомлений
func (w *Watchdog) SetEventBus(bus *sse.EventBus) {
	w.eventBus = bus
}

// SetGeoIP устанавливает GeoIP-матчер для гео-фильтрации при автопереключении.
func (w *Watchdog) SetGeoIP(m *geoip.Matcher) {
	w.geoip = m
}

// publishStatus отправляет текущий статус в SSE
func (w *Watchdog) publishStatus() {
	if w.eventBus != nil {
		w.eventBus.Publish(sse.Event{Type: "status", Data: w.GetStatus()})
	}
}

// Start запускает watchdog в горутине
func (w *Watchdog) Start(ctx context.Context) {
	// Открыть лог-файл
	if w.config.LogFile != "" {
		f, err := os.OpenFile(w.config.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("Не удалось открыть лог-файл: %v", err)
		} else {
			w.logFile = f
		}
	}

	interval := time.Duration(w.config.CheckInterval) * time.Second
	if interval < 10*time.Second {
		interval = 120 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	w.writeLog("Watchdog запущен (интервал: %s)", interval)

	// Первая проверка сразу
	w.check()

	for {
		select {
		case <-ctx.Done():
			w.writeLog("Watchdog остановлен")
			if w.logFile != nil {
				w.logFile.Close()
			}
			return
		case <-ticker.C:
			w.mu.RLock()
			active := w.active
			w.mu.RUnlock()

			if active {
				w.check()
			}
		}
	}
}

func (w *Watchdog) check() {
	start := time.Now()

	// Прямой HTTP-запрос — tproxy прозрачно проксирует трафик
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(w.config.CheckURL)

	w.mu.Lock()
	w.lastCheck = time.Now()

	if err != nil {
		w.connected = false
		w.lastLatency = -1
		w.latencyHigh = 0
		w.failCount++
		w.mu.Unlock()

		w.writeLog("[FAIL] Соединение недоступно (%v), попытка %d/%d", err, w.failCount, w.config.MaxFails)
		w.publishStatus()

		if w.failCount >= w.config.MaxFails {
			w.handleFailover("нет соединения")
		}
		return
	}

	resp.Body.Close()
	latency := int(time.Since(start).Milliseconds())

	w.connected = true
	w.lastLatency = latency
	w.failCount = 0

	// Проактивное переключение: соединение живо, но пинг стабильно высокий
	proactive := false
	if w.config.LatencyAutoSwitch && w.config.LatencyThresholdMs > 0 && latency > w.config.LatencyThresholdMs {
		w.latencyHigh++
		if w.latencyHigh >= w.config.LatencySwitchCount {
			w.latencyHigh = 0
			proactive = true
		}
	} else {
		w.latencyHigh = 0
	}
	w.mu.Unlock()

	w.writeLog("[OK] Соединение активно (%dms)", latency)
	w.publishStatus()

	if proactive {
		w.handleFailover(fmt.Sprintf("высокий пинг %dms подряд", latency))
	}
}

func (w *Watchdog) handleFailover(reason string) {
	w.writeLog("[FAILOVER] %s — подбор лучшего сервера...", reason)

	// Обновить подписку (активный сохраняется по RawURI)
	if _, err := w.subscription.Refresh(); err != nil {
		w.writeLog("[WARN] Не удалось обновить подписку: %v", err)
	}

	prevURI := ""
	if prev := w.subscription.GetActiveServer(); prev != nil {
		prevURI = prev.RawURI
	}

	server, err := w.selectBest()
	if err != nil {
		w.writeLog("[FAILOVER] %v — переключение не выполнено", err)
		return
	}

	// Исключить прежний сервер на время TTL, чтобы не метаться по кругу
	w.blacklistServer(prevURI)

	w.writeLog("[FAILOVER] Выбран сервер: %s (%s:%d, %dms)", server.Name, server.Address, server.Port, server.Latency)

	if err := xkeen.UpdateOutbound(w.config.OutboundsFile, server); err != nil {
		w.writeLog("[ERROR] Ошибка обновления конфига Xray: %v", err)
		return
	}

	if output, err := xkeen.Restart(w.config.XKeenPath); err != nil {
		w.writeLog("[ERROR] Ошибка перезапуска: %v (%s)", err, output)
		return
	}

	w.mu.Lock()
	w.failCount = 0
	w.latencyHigh = 0
	w.mu.Unlock()

	w.writeLog("[FAILOVER] Перезапуск выполнен, ожидание следующей проверки")
}

// selectBest подбирает живой сервер с минимальным пингом, избегая текущего,
// серверов из чёрного списка, не-VLESS и серверов в заблокированных странах.
func (w *Watchdog) selectBest() (*models.Server, error) {
	data := w.subscription.GetData()
	if len(data.Servers) == 0 {
		return nil, fmt.Errorf("нет доступных серверов")
	}
	currentID := data.ActiveID

	var candidates []models.Server
	skippedGeo := 0
	for _, s := range data.Servers {
		if s.ID == currentID {
			continue
		}
		if w.isBlacklisted(s.RawURI) {
			continue
		}
		if s.Protocol != "" && s.Protocol != "vless" {
			continue
		}
		if !w.isServerAllowed(s) {
			skippedGeo++
			continue
		}
		candidates = append(candidates, s)
	}

	if skippedGeo > 0 {
		w.writeLog("[GEO] Пропущено %d сервер(ов) из заблокированных стран / нераспознанных", skippedGeo)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("нет разрешённых серверов для авто-переключения")
	}

	timeout := time.Duration(w.config.ProbeTimeoutMs) * time.Millisecond
	checked := xkeen.CheckAllLatencies(candidates, timeout, w.config.ProbeConcurrency)
	w.subscription.UpdateLatencies(checked)

	best := -1
	bestLat := int(^uint(0) >> 1)
	for i := range checked {
		if checked[i].Latency >= 0 && checked[i].Latency < bestLat {
			bestLat = checked[i].Latency
			best = i
		}
	}
	if best < 0 {
		return nil, fmt.Errorf("ни один разрешённый сервер не ответил")
	}

	return w.subscription.SetActive(checked[best].ID)
}

// isServerAllowed решает, можно ли авто-переключиться на сервер. GeoIP — основной
// сигнал (доказывает «не в заблокированной стране»), имя — запасной.
func (w *Watchdog) isServerAllowed(s models.Server) bool {
	// Ручной override и распознанная по имени страна — если она в списке избегаемых, сразу нет
	effCountry := s.CountryOverride
	if effCountry == "" {
		effCountry = s.Country
	}
	if effCountry != "" && w.isAvoidedCountry(effCountry) {
		return false
	}

	// GeoIP — авторитетная проверка по реальному IP
	if w.geoip != nil {
		avoidCC, resolved := w.geoip.Inspect(s.Address)
		if resolved {
			return avoidCC == "" // резолвится и не в избегаемой стране → разрешён
		}
		// не резолвится → откат на имя ниже
	}

	// Запасной вариант (GeoIP недоступен / не резолвится): строгий режим —
	// разрешаем только при известной и разрешённой стране по имени
	return effCountry != ""
}

func (w *Watchdog) isAvoidedCountry(cc string) bool {
	cc = strings.ToUpper(cc)
	for _, a := range w.config.AutoSwitchAvoidCountries {
		if strings.ToUpper(strings.TrimSpace(a)) == cc {
			return true
		}
	}
	return false
}

func (w *Watchdog) blacklistServer(uri string) {
	ttl := time.Duration(w.config.BlacklistTTLSec) * time.Second
	if ttl <= 0 || uri == "" {
		return
	}
	w.mu.Lock()
	w.blacklist[uri] = time.Now().Add(ttl)
	w.mu.Unlock()
}

func (w *Watchdog) isBlacklisted(uri string) bool {
	if uri == "" {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	until, ok := w.blacklist[uri]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(w.blacklist, uri)
		return false
	}
	return true
}

// ClearBlacklist убирает сервер из чёрного списка (например, при ручном выборе).
func (w *Watchdog) ClearBlacklist(uri string) {
	if uri == "" {
		return
	}
	w.mu.Lock()
	delete(w.blacklist, uri)
	w.mu.Unlock()
}

func (w *Watchdog) writeLog(format string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf("%s %s", timestamp, fmt.Sprintf(format, args...))

	log.Println(msg)

	w.mu.Lock()
	w.logs = append(w.logs, msg)
	// Хранить максимум 500 строк в памяти
	if len(w.logs) > 500 {
		w.logs = w.logs[len(w.logs)-500:]
	}
	w.mu.Unlock()

	if w.logFile != nil {
		w.logFile.WriteString(msg + "\n")
	}

	if w.eventBus != nil {
		w.eventBus.Publish(sse.Event{Type: "log", Data: msg})
	}
}

// GetStatus возвращает текущий статус
func (w *Watchdog) GetStatus() models.Status {
	w.mu.RLock()
	defer w.mu.RUnlock()

	restarting := xkeen.IsRestarting()
	xrayRunning := xkeen.IsRunning()

	// Во время рестарта состояние xray ненадёжно — процесс может быть
	// ещё жив или уже убит, не показываем "connected" чтобы не вводить в заблуждение
	if restarting {
		xrayRunning = false
	}

	status := models.Status{
		Connected:      w.connected && !restarting,
		XrayRunning:    xrayRunning,
		Restarting:     restarting,
		Latency:        w.lastLatency,
		LastCheck:      w.lastCheck,
		WatchdogActive: w.active,
	}

	// Uptime
	if w.connected {
		uptime := time.Since(w.startTime)
		hours := int(uptime.Hours())
		minutes := int(uptime.Minutes()) % 60
		if hours > 0 {
			status.Uptime = fmt.Sprintf("%dh %dm", hours, minutes)
		} else {
			status.Uptime = fmt.Sprintf("%dm", minutes)
		}
	}

	// Текущий сервер
	if server := w.subscription.GetActiveServer(); server != nil {
		status.CurrentServer = server.Name
		status.Protocol = server.Protocol
	}

	return status
}

// GetLogs возвращает последние N строк лога
func (w *Watchdog) GetLogs(n int) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if n <= 0 || n > len(w.logs) {
		n = len(w.logs)
	}

	// Читать также из файла если лог в памяти короче
	if n > len(w.logs) && w.config.LogFile != "" {
		return w.readLogFile(n)
	}

	result := make([]string, n)
	copy(result, w.logs[len(w.logs)-n:])
	return result
}

func (w *Watchdog) readLogFile(n int) []string {
	data, err := os.ReadFile(w.config.LogFile)
	if err != nil {
		return w.logs
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if n > len(lines) {
		n = len(lines)
	}
	return lines[len(lines)-n:]
}

// SetActive включает/выключает watchdog
func (w *Watchdog) SetActive(active bool) {
	w.mu.Lock()
	w.active = active
	w.mu.Unlock()

	if active {
		w.writeLog("Watchdog включён")
	} else {
		w.writeLog("Watchdog выключен")
	}

	w.publishStatus()
}

// IsActive проверяет, активен ли watchdog
func (w *Watchdog) IsActive() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.active
}
