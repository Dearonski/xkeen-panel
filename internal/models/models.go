package models

import "time"

// Config — конфигурация приложения (config.yaml)
type Config struct {
	Port          int    `yaml:"port"`
	DataDir       string `yaml:"data_dir"`
	XKeenPath     string `yaml:"xkeen_path"`
	OutboundsFile string `yaml:"outbounds_file"`
	InitScript    string `yaml:"init_script"`
	CheckInterval int    `yaml:"check_interval"`
	CheckURL      string `yaml:"check_url"`
	MaxFails      int    `yaml:"max_fails"`
	LogFile       string `yaml:"log_file"`
}

// User — пользователь (data/user.json)
type User struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	TOTPSecret   string    `json:"totp_secret"`
	JWTSecret    string    `json:"jwt_secret"`
	CreatedAt    time.Time `json:"created_at"`
}

// Server — сервер из подписки
type Server struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Active   bool   `json:"active"`
	Latency  int    `json:"latency_ms"`
	RawURI   string `json:"raw_uri,omitempty"`
}

// SubscriptionData — подписка (data/subscription.json)
type SubscriptionData struct {
	URL         string    `json:"url"`
	LastUpdated time.Time `json:"last_updated"`
	Servers     []Server  `json:"servers"`
	ActiveID    int       `json:"active_id"`
}

// Status — статус соединения
type Status struct {
	Connected      bool      `json:"connected"`
	XrayRunning    bool      `json:"xray_running"`
	Restarting     bool      `json:"restarting"`
	CurrentServer  string    `json:"current_server"`
	Protocol       string    `json:"protocol"`
	Latency        int       `json:"latency_ms"`
	Uptime         string    `json:"uptime"`
	LastCheck      time.Time `json:"last_check"`
	WatchdogActive bool      `json:"watchdog_active"`
}

// SetupRequest — запрос на первичную регистрацию
type SetupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// SetupConfirmRequest — подтверждение TOTP
type SetupConfirmRequest struct {
	Code string `json:"code"`
}

// LoginRequest — запрос на вход
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

// SelectServerRequest — выбор сервера
type SelectServerRequest struct {
	ID int `json:"id"`
}

// UpdateSubscriptionRequest — обновление подписки
type UpdateSubscriptionRequest struct {
	URL string `json:"url"`
}
