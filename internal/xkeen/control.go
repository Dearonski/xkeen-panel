package xkeen

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	restartMu            sync.Mutex
	restarting           bool
	OnRestartStateChange func(restarting bool)

	runningMu        sync.Mutex
	runningCached    bool
	runningCheckedAt time.Time
)

// Restart перезапускает Xray через xkeen -restart
// Защита от одновременных рестартов — если уже идёт, пропускаем
func Restart(xkeenPath string) (string, error) {
	restartMu.Lock()
	if restarting {
		restartMu.Unlock()
		log.Printf("[RESTART] Пропущен — уже идёт рестарт")
		return "restart already in progress", nil
	}
	restarting = true
	restartMu.Unlock()

	if OnRestartStateChange != nil {
		OnRestartStateChange(true)
	}

	log.Printf("[RESTART] Запуск: %s -restart", xkeenPath)

	cmd := exec.Command(xkeenPath, "-restart")
	// Не захватываем stdout/stderr — xray наследует pipe и cmd.Wait() зависнет
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		restartMu.Lock()
		restarting = false
		restartMu.Unlock()
		if OnRestartStateChange != nil {
			OnRestartStateChange(false)
		}
		log.Printf("[RESTART] Ошибка запуска: %v", err)
		return "", fmt.Errorf("ошибка запуска перезапуска: %w", err)
	}

	log.Printf("[RESTART] Процесс запущен (PID %d)", cmd.Process.Pid)

	go func() {
		err := cmd.Wait()

		restartMu.Lock()
		restarting = false
		restartMu.Unlock()

		if OnRestartStateChange != nil {
			OnRestartStateChange(false)
		}

		if err != nil {
			log.Printf("[RESTART] Ошибка (exit): %v", err)
		} else {
			log.Printf("[RESTART] Завершён успешно")
		}
	}()

	return "restart initiated", nil
}

// Update выполняет обновление через XKeen CLI (xkeen -u)
func Update(xkeenPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, xkeenPath, "-u")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("ошибка обновления: %w (%s)", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// IsRestarting возвращает true если сейчас идёт рестарт
func IsRestarting() bool {
	restartMu.Lock()
	defer restartMu.Unlock()
	return restarting
}

// IsRunning проверяет, запущен ли процесс xray. Результат кэшируется на 2с —
// проверка на каждый SSE-пуш/опрос статуса иначе дорога на роутере.
func IsRunning() bool {
	runningMu.Lock()
	defer runningMu.Unlock()

	if time.Since(runningCheckedAt) < 2*time.Second {
		return runningCached
	}

	runningCached = xrayProcessRunning()
	runningCheckedAt = time.Now()
	return runningCached
}

// xrayProcessRunning ищет процесс xray через /proc — надёжнее, чем grep по выводу
// ps (формат busybox ps на разных роутерах отличается и обрезает аргументы).
func xrayProcessRunning() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		// Нет /proc (например, macOS при разработке) — запасной путь через ps
		out, _ := exec.Command("sh", "-c", "ps | grep -v grep | grep xray").Output()
		return len(bytes.TrimSpace(out)) > 0
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		cmd := string(bytes.ReplaceAll(data, []byte{0}, []byte{' '}))
		if strings.Contains(cmd, "xkeen-panel") {
			continue // не считать саму панель
		}
		if strings.Contains(cmd, "xray") {
			return true
		}
	}
	return false
}
