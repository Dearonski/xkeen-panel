package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	"xkeen-panel/internal/auth"
	"xkeen-panel/internal/models"
	"xkeen-panel/internal/monitor"
	"xkeen-panel/internal/server"
	"xkeen-panel/internal/sse"
	"xkeen-panel/internal/xkeen"

	"gopkg.in/yaml.v3"
)

func main() {
	configPath := flag.String("config", "config.yaml", "путь к конфигурационному файлу")
	flag.Parse()

	// Чтение конфигурации
	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Ошибка загрузки конфига: %v", err)
	}

	// Инициализация менеджера пользователей
	userManager := auth.NewUserManager(cfg.DataDir)
	if err := userManager.Load(); err != nil {
		log.Fatalf("Ошибка загрузки пользователя: %v", err)
	}

	// Инициализация менеджера подписок
	subManager := xkeen.NewSubscriptionManager(cfg.DataDir)
	if err := subManager.Load(); err != nil {
		log.Printf("Предупреждение: ошибка загрузки подписки: %v", err)
	}

	// Инициализация watchdog и SSE
	watchdog := monitor.NewWatchdog(cfg, subManager)
	eventBus := sse.NewEventBus()
	watchdog.SetEventBus(eventBus)

	// Публикация событий рестарта через SSE
	xkeen.OnRestartStateChange = func(restarting bool) {
		eventBus.Publish(sse.Event{
			Type: "restart",
			Data: map[string]bool{"restarting": restarting},
		})
		eventBus.Publish(sse.Event{
			Type: "status",
			Data: watchdog.GetStatus(),
		})
	}

	// Контекст для graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Запуск watchdog в горутине
	go watchdog.Start(ctx)

	// Подготовка фронтенда
	var frontendFS fs.FS
	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Printf("Предупреждение: встроенный фронтенд недоступен: %v", err)
	} else {
		frontendFS = distFS
	}

	// HTTP-сервер
	srv := server.New(cfg, userManager, subManager, watchdog, eventBus, frontendFS)
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: srv.Handler(),
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Получен сигнал завершения, останавливаем сервер...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Ошибка при остановке сервера: %v", err)
		}
	}()

	log.Printf("XKeen Panel v2 запущена на порту %d (xkeen=%s, outbounds=%s)", cfg.Port, cfg.XKeenPath, cfg.OutboundsFile)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Ошибка сервера: %v", err)
	}

	log.Println("Сервер остановлен")
}

func loadConfig(path string) (*models.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &models.Config{
		Port:          3000,
		DataDir:       "data",
		XKeenPath:     "/opt/sbin/xkeen",
		OutboundsFile: "/opt/etc/xray/configs/04_outbounds.json",
		InitScript:    "/opt/etc/init.d/S24xray",
		CheckInterval: 120,
		CheckURL:      "https://www.google.com",
		MaxFails:      3,
		LogFile:       "xkeen-panel.log",
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

