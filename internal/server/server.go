package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"
	"xkeen-panel/internal/api"
	"xkeen-panel/internal/auth"
	"xkeen-panel/internal/models"
	"xkeen-panel/internal/monitor"
	"xkeen-panel/internal/sse"
	"xkeen-panel/internal/xkeen"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	config       *models.Config
	userManager  *auth.UserManager
	subscription *xkeen.SubscriptionManager
	watchdog     *monitor.Watchdog
	eventBus     *sse.EventBus
	frontendFS   fs.FS
}

func New(cfg *models.Config, um *auth.UserManager, sub *xkeen.SubscriptionManager, wd *monitor.Watchdog, bus *sse.EventBus, frontendFS fs.FS) *Server {
	return &Server{
		config:       cfg,
		userManager:  um,
		subscription: sub,
		watchdog:     wd,
		eventBus:     bus,
		frontendFS:   frontendFS,
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Rate limiter для login — 5 попыток в минуту
	rateLimiter := api.NewRateLimiter(5, time.Minute)

	// Хендлеры
	authHandler := api.NewAuthHandler(s.userManager, rateLimiter)
	handlers := api.NewHandlers(s.config, s.subscription, s.watchdog)

	// API-маршруты
	r.Route("/api", func(r chi.Router) {
		// Аутентификация — без JWT
		r.Route("/auth", func(r chi.Router) {
			r.Get("/status", authHandler.HandleAuthStatus)
			r.Post("/setup", authHandler.HandleSetup)
			r.Post("/setup/confirm", authHandler.HandleSetupConfirm)
			r.With(api.RateLimitMiddleware(rateLimiter)).Post("/login", authHandler.HandleLogin)
		})

		// SSE-маршруты — с JWT, без таймаута
		r.Group(func(r chi.Router) {
			r.Use(api.AuthMiddleware(s.userManager))

			r.Get("/events", sse.HandleEvents(s.eventBus, s.watchdog))
			r.Get("/servers/check", sse.HandleStreamLatency(s.subscription))
		})

		// Защищённые REST-маршруты — с JWT и таймаутом
		r.Group(func(r chi.Router) {
			r.Use(api.AuthMiddleware(s.userManager))
			r.Use(middleware.Timeout(30 * time.Second))

			r.Get("/status", handlers.HandleStatus)

			r.Get("/subscription", handlers.HandleGetSubscription)
			r.Post("/subscription", handlers.HandleUpdateSubscription)
			r.Post("/subscription/refresh", handlers.HandleRefreshSubscription)

			r.Get("/servers", handlers.HandleGetServers)
			r.Post("/servers/select", handlers.HandleSelectServer)
			r.Post("/servers/check", handlers.HandleCheckServers)

			r.Post("/xkeen/restart", handlers.HandleRestart)
			r.Post("/xkeen/update", handlers.HandleUpdate)

			r.Get("/logs", handlers.HandleLogs)

			// Watchdog toggle
			r.Post("/watchdog/toggle", func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Active bool `json:"active"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{"error": "неверный формат"})
					return
				}
				s.watchdog.SetActive(req.Active)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]bool{"active": req.Active})
			})
		})
	})

	// SPA fallback — отдаём фронтенд
	if s.frontendFS != nil {
		fileServer := http.FileServer(http.FS(s.frontendFS))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			// Попробовать отдать статический файл
			path := r.URL.Path
			if path == "/" {
				path = "/index.html"
			}

			// Проверить существование файла
			f, err := s.frontendFS.Open(path[1:]) // убрать /
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}

			// SPA fallback — отдать index.html
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	}

	return r
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Port)
	log.Printf("Сервер запущен на %s", addr)
	return http.ListenAndServe(addr, s.Handler())
}
