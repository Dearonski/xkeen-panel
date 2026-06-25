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

	// Автопилот: проверка задержек и переключение серверов
	ProbeTimeoutMs     int  `yaml:"probe_timeout_ms"`
	ProbeConcurrency   int  `yaml:"probe_concurrency"`
	LatencyAutoSwitch  bool `yaml:"latency_auto_switch"`
	LatencyThresholdMs int  `yaml:"latency_threshold_ms"`
	LatencySwitchCount int  `yaml:"latency_switch_count"`
	BlacklistTTLSec    int  `yaml:"blacklist_ttl_sec"`
	WatchdogAutoStart  bool `yaml:"watchdog_auto_start"`

	// Автообновление подписки
	SubscriptionRefreshInterval int `yaml:"subscription_refresh_interval"`

	// Гео-избегание при автопереключении
	GeoIPPath                string   `yaml:"geoip_path"`
	AutoSwitchAvoidCountries []string `yaml:"auto_switch_avoid_countries"`
}

// User — пользователь (data/user.json)
type User struct {
	Username      string    `json:"username"`
	PasswordHash  string    `json:"password_hash"`
	TOTPSecret    string    `json:"totp_secret"`
	JWTSecret     string    `json:"jwt_secret"`
	CreatedAt     time.Time `json:"created_at"`
	AccessKeyHash string    `json:"access_key_hash,omitempty"`
	AccessKeyHint string    `json:"access_key_hint,omitempty"`
}

// Server — сервер из подписки
type Server struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Address         string    `json:"address"`
	Port            int       `json:"port"`
	Protocol        string    `json:"protocol"`
	Active          bool      `json:"active"`
	Latency         int       `json:"latency_ms"`
	RawURI          string    `json:"raw_uri,omitempty"`
	LastChecked     time.Time `json:"last_checked,omitempty"`
	Country         string    `json:"country,omitempty"`
	CountryOverride string    `json:"country_override,omitempty"`
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

// AccessKeyLoginRequest — вход по ключу доступа (без TOTP)
type AccessKeyLoginRequest struct {
	AccessKey string `json:"access_key"`
}

// SelectServerRequest — выбор сервера
type SelectServerRequest struct {
	ID int `json:"id"`
}

// SetCountryRequest — ручной override страны сервера
type SetCountryRequest struct {
	ID      int    `json:"id"`
	Country string `json:"country"`
}

// UpdateSubscriptionRequest — обновление подписки
type UpdateSubscriptionRequest struct {
	URL string `json:"url"`
}
