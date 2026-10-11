package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"xkeen-panel/internal/geoip"
	"xkeen-panel/internal/models"
)

// Kept in data/settings.json so config.yaml, comments and all, is never rewritten
type PanelSettings struct {
	AvoidCountries []string `json:"avoid_countries"`
	RefreshMinutes int      `json:"subscription_refresh_minutes"`
}

func panelSettingsPath(dataDir string) string {
	return filepath.Join(dataDir, "settings.json")
}

// LoadPanelSettings applies settings saved from the panel over config.yaml.
func LoadPanelSettings(cfg *models.Config) error {
	data, err := os.ReadFile(panelSettingsPath(cfg.DataDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var s PanelSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	cfg.SetRuntime(s.AvoidCountries, s.RefreshMinutes*60)
	return nil
}

type PanelSettingsHandler struct {
	config  *models.Config
	geoip   *geoip.Matcher
	geoPath string
	reset   chan<- struct{}
	mu      sync.Mutex
}

func NewPanelSettingsHandler(cfg *models.Config, matcher *geoip.Matcher, geoPath string, reset chan<- struct{}) *PanelSettingsHandler {
	return &PanelSettingsHandler{config: cfg, geoip: matcher, geoPath: geoPath, reset: reset}
}

func (h *PanelSettingsHandler) current() PanelSettings {
	return PanelSettings{
		AvoidCountries: h.config.AvoidCountries(),
		RefreshMinutes: int(h.config.RefreshInterval() / time.Minute),
	}
}

// HandleGet — GET /api/panel/settings
func (h *PanelSettingsHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.current())
}

var countryCodeRe = regexp.MustCompile(`^[A-Z]{2}$`)

// HandlePut — PUT /api/panel/settings
func (h *PanelSettingsHandler) HandlePut(w http.ResponseWriter, r *http.Request) {
	var req PanelSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	seen := map[string]bool{}
	countries := []string{}
	for _, c := range req.AvoidCountries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if !countryCodeRe.MatchString(c) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "код страны должен состоять из двух латинских букв: " + c})
			return
		}
		if !seen[c] {
			seen[c] = true
			countries = append(countries, c)
		}
	}
	if req.RefreshMinutes != 0 && (req.RefreshMinutes < 5 || req.RefreshMinutes > 1440) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "интервал обновления — от 5 минут до суток, или 0 чтобы выключить"})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	next := PanelSettings{AvoidCountries: countries, RefreshMinutes: req.RefreshMinutes}
	data, err := json.MarshalIndent(next, "", "  ")
	if err == nil {
		err = os.WriteFile(panelSettingsPath(h.config.DataDir), data, 0600)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "не удалось сохранить настройки: " + err.Error()})
		return
	}

	h.config.SetRuntime(next.AvoidCountries, next.RefreshMinutes*60)

	if h.geoip != nil && h.geoPath != "" {
		if err := h.geoip.Reload(h.geoPath, next.AvoidCountries); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "настройки сохранены, но GeoIP не перечитан: " + err.Error()})
			return
		}
	}

	select {
	case h.reset <- struct{}{}:
	default:
	}

	writeJSON(w, http.StatusOK, h.current())
}
