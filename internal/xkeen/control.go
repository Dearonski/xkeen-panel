package xkeen

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	restartMu            sync.Mutex
	restarting           bool
	OnRestartStateChange func(restarting bool)
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

// IsRunning проверяет, запущен ли процесс xray
func IsRunning() bool {
	cmd := exec.Command("sh", "-c", "busybox ps | grep -v grep | grep 'xray run'")
	err := cmd.Run()
	return err == nil
}
