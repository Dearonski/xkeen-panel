package models

import (
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// Config is the application configuration (config.yaml).
type Config struct {
	Port          int    `yaml:"port"`
	DataDir       string `yaml:"data_dir"`
	XKeenPath     string `yaml:"xkeen_path"`
	OutboundsFile string `yaml:"outbounds_file"`
	CheckInterval int    `yaml:"check_interval"`
	CheckURL      string `yaml:"check_url"`
	MaxFails      int    `yaml:"max_fails"`
	LogFile       string `yaml:"log_file"`

	// XKeen layout. Every field is optional — the panel detects the layout of
	// both the 1.x (S24xray) and 2.x (S05xkeen) installs on startup.
	InitScript    string `yaml:"init_script"` // deprecated: kept so old config.yaml still loads
	XrayConfigDir string `yaml:"xray_config_dir"`
	RoutingFile   string `yaml:"routing_file"`
	MihomoConfig  string `yaml:"mihomo_config"`
	XkeenJSON     string `yaml:"xkeen_json"`
	XrayAPIAddr   string `yaml:"xray_api_addr"`

	// Autopilot: latency probing and server switching
	ProbeTimeoutMs     int  `yaml:"probe_timeout_ms"`
	ProbeConcurrency   int  `yaml:"probe_concurrency"`
	LatencyAutoSwitch  bool `yaml:"latency_auto_switch"`
	LatencyThresholdMs int  `yaml:"latency_threshold_ms"`
	LatencySwitchCount int  `yaml:"latency_switch_count"`
	BlacklistTTLSec    int  `yaml:"blacklist_ttl_sec"`
	WatchdogAutoStart  bool `yaml:"watchdog_auto_start"`

	// Automatic subscription refresh
	SubscriptionRefreshInterval int `yaml:"subscription_refresh_interval"`

	// Cap on pool size: every node is probed by observatory separately
	PoolMaxNodes int `yaml:"pool_max_nodes"`

	// Health probes of real services, used to catch an exit IP a CDN blocks
	// while plain connectivity still works. One service per round, in rotation.
	HealthCheckURLs     []string `yaml:"health_check_urls"`
	HealthCheckEvery    int      `yaml:"health_check_every"`    // раз в N циклов watchdog
	HealthFailThreshold int      `yaml:"health_fail_threshold"` // неудач одного сервиса подряд
	HealthQuorum        int      `yaml:"health_quorum"`         // сколько сервисов должны отвалиться

	// Countries to avoid when switching automatically
	GeoIPPath                string   `yaml:"geoip_path"`
	AutoSwitchAvoidCountries []string `yaml:"auto_switch_avoid_countries"`

	// Trust proxy headers (X-Forwarded-For/Host/Proto). Enable ONLY behind a
	// trusted proxy that rewrites them — otherwise they can be spoofed on the
	// direct :3000 socket.
	TrustProxyHeaders bool `yaml:"trust_proxy_headers"`

	// WebAuthn (passkey). Prefer pinning RPID/origins explicitly; deriving them
	// from request headers is allowed only with trust_proxy_headers: true.
	WebAuthnRPID    string   `yaml:"webauthn_rp_id"`
	WebAuthnRPName  string   `yaml:"webauthn_rp_name"`
	WebAuthnOrigins []string `yaml:"webauthn_origins"`
}

// User is the panel account (data/user.json).
type User struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	TOTPSecret   string    `json:"totp_secret"`
	JWTSecret    string    `json:"jwt_secret"`
	CreatedAt    time.Time `json:"created_at"`

	WebAuthnID  []byte                `json:"webauthn_id,omitempty"`
	Credentials []webauthn.Credential `json:"credentials,omitempty"`
}

// Server is one entry of the subscription.
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

// SubscriptionData is the stored subscription (data/subscription.json).
type SubscriptionData struct {
	URL         string    `json:"url"`
	LastUpdated time.Time `json:"last_updated"`
	Servers     []Server  `json:"servers"`
	ActiveID    int       `json:"active_id"`
}

// Status is the connection status reported to the UI.
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

	// XKeen runtime: proxy core (xray/mihomo), proxying mode (TProxy/Hybrid/…),
	// version and layout generation.
	Core         string `json:"core"`
	Mode         string `json:"mode"`
	XKeenVersion string `json:"xkeen_version"`
	Generation   int    `json:"generation"`
}

// SetupRequest starts the initial account setup.
type SetupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// SetupConfirmRequest confirms the TOTP enrolment.
type SetupConfirmRequest struct {
	Code string `json:"code"`
}

// LoginRequest is a login attempt.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

// SelectServerRequest selects a server.
type SelectServerRequest struct {
	ID int `json:"id"`
}

// SetCountryRequest overrides a server's country by hand.
type SetCountryRequest struct {
	ID      int    `json:"id"`
	Country string `json:"country"`
}

// UpdateSubscriptionRequest sets the subscription URL.
type UpdateSubscriptionRequest struct {
	URL string `json:"url"`
}
