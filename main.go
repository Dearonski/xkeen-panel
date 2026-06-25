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
	"xkeen-panel/internal/geoip"
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

	// GeoIP — используем geoip.dat, который уже стоит для Xray
	if geoPath := geoip.FindDat(cfg.GeoIPPath); geoPath == "" {
		log.Printf("GeoIP: geoip.dat не найден (%s) — гео-фильтр по IP отключён, используется определение по имени", cfg.GeoIPPath)
	} else if matcher, err := geoip.Load(geoPath, cfg.AutoSwitchAvoidCountries); err != nil {
		log.Printf("GeoIP: ошибка загрузки %s: %v — гео-фильтр по IP отключён", geoPath, err)
	} else {
		watchdog.SetGeoIP(matcher)
		log.Printf("GeoIP: загружен %s (избегаемые страны: %v)", geoPath, cfg.AutoSwitchAvoidCountries)
	}

	// Автостарт watchdog для unattended-режима
	if cfg.WatchdogAutoStart && len(subManager.GetServers()) > 0 {
		watchdog.SetActive(true)
		log.Printf("Watchdog включён автоматически (watchdog_auto_start)")
	}

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

	// Периодическое автообновление подписки
	if cfg.SubscriptionRefreshInterval > 0 {
		go runSubscriptionRefresh(ctx, cfg, subManager, watchdog, eventBus)
	}

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

// runSubscriptionRefresh периодически обновляет подписку. Xray перезапускается
// только если активный сервер реально заменился (исчез из подписки) — иначе
// обновление не должно рвать соединение.
func runSubscriptionRefresh(ctx context.Context, cfg *models.Config, sm *xkeen.SubscriptionManager, wd *monitor.Watchdog, bus *sse.EventBus) {
	interval := time.Duration(cfg.SubscriptionRefreshInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prevURI := ""
			if a := sm.GetActiveServer(); a != nil {
				prevURI = a.RawURI
			}

			var err error
			for attempt := 0; attempt < 2; attempt++ {
				if _, err = sm.Refresh(); err == nil {
					break
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(30 * time.Second):
				}
			}
			if err != nil {
				log.Printf("[AUTO-UPDATE] Подписка не обновилась: %v", err)
				bus.Publish(sse.Event{Type: "log", Data: tsLog("[AUTO-UPDATE] ошибка обновления подписки")})
				continue
			}

			active := sm.GetActiveServer()
			newURI := ""
			if active != nil {
				newURI = active.RawURI
			}

			// Активный сервер сменился (старый исчез из подписки). Не перезапускаемся
			// вслепую на servers[0] — он может оказаться в избегаемой стране; отдаём
			// выбор гео-фильтру watchdog.
			if active != nil && newURI != prevURI {
				if target := wd.AllowedActiveOrBest(); target != nil {
					if err := xkeen.UpdateOutbound(cfg.OutboundsFile, target); err != nil {
						log.Printf("[AUTO-UPDATE] Ошибка конфига: %v", err)
					} else {
						xkeen.Restart(cfg.XKeenPath)
						log.Printf("[AUTO-UPDATE] Активный сервер заменён на %s, xray перезапущен", target.Name)
					}
				}
			}

			bus.Publish(sse.Event{Type: "subscription", Data: map[string]bool{"updated": true}})
			bus.Publish(sse.Event{Type: "log", Data: tsLog("[AUTO-UPDATE] подписка обновлена")})
		}
	}
}

func tsLog(msg string) string {
	return time.Now().Format("2006-01-02 15:04:05") + " " + msg
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

		ProbeTimeoutMs:     2000,
		ProbeConcurrency:   20,
		LatencyAutoSwitch:  true,
		LatencyThresholdMs: 1000,
		LatencySwitchCount: 3,
		BlacklistTTLSec:    300,
		WatchdogAutoStart:  true,

		SubscriptionRefreshInterval: 21600,

		GeoIPPath:                "/opt/etc/xray/dat/geoip_v2fly.dat",
		AutoSwitchAvoidCountries: []string{"RU", "BY"},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

