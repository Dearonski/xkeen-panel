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
	lastCheck    time.Time
	lastLatency  int
	connected    bool
	startTime    time.Time
	logs         []string
	logFile      *os.File
	eventBus     *sse.EventBus
}

func NewWatchdog(cfg *models.Config, sub *xkeen.SubscriptionManager) *Watchdog {
	return &Watchdog{
		config:       cfg,
		subscription: sub,
		active:       false, // По умолчанию выключен — включается вручную из UI
		startTime:    time.Now(),
		lastLatency:  -1,
	}
}

// SetEventBus устанавливает шину событий для SSE-уведомлений
func (w *Watchdog) SetEventBus(bus *sse.EventBus) {
	w.eventBus = bus
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
		w.failCount++
		w.mu.Unlock()

		w.writeLog("[FAIL] Соединение недоступно (%v), попытка %d/%d", err, w.failCount, w.config.MaxFails)
		w.publishStatus()

		if w.failCount >= w.config.MaxFails {
			w.handleFailover()
		}
		return
	}

	resp.Body.Close()
	latency := int(time.Since(start).Milliseconds())

	w.connected = true
	w.lastLatency = latency
	w.failCount = 0
	w.mu.Unlock()

	w.writeLog("[OK] Соединение активно (%dms)", latency)
	w.publishStatus()
}

func (w *Watchdog) handleFailover() {
	w.writeLog("[FAILOVER] Переключение на следующий сервер...")

	// Обновить подписку
	if _, err := w.subscription.Refresh(); err != nil {
		w.writeLog("[ERROR] Ошибка обновления подписки: %v", err)
	}

	// Выбрать следующий сервер
	server, err := w.subscription.SelectNext()
	if err != nil {
		w.writeLog("[ERROR] Ошибка выбора сервера: %v", err)
		return
	}

	w.writeLog("[FAILOVER] Выбран сервер: %s (%s:%d)", server.Name, server.Address, server.Port)

	// Обновить конфиг Xray (04_outbounds.json)
	if err := xkeen.UpdateOutbound(w.config.OutboundsFile, server); err != nil {
		w.writeLog("[ERROR] Ошибка обновления конфига Xray: %v", err)
		return
	}

	// Перезапустить Xray через xkeen -restart
	if output, err := xkeen.Restart(w.config.XKeenPath); err != nil {
		w.writeLog("[ERROR] Ошибка перезапуска: %v (%s)", err, output)
		return
	}

	w.mu.Lock()
	w.failCount = 0
	w.mu.Unlock()

	w.writeLog("[FAILOVER] Перезапуск выполнен, ожидание следующей проверки")
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
