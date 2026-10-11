package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xkeen-panel/internal/models"
)

func putSettings(h *PanelSettingsHandler, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.HandlePut(rec, httptest.NewRequest(http.MethodPut, "/api/panel/settings", strings.NewReader(body)))
	return rec
}

func TestPanelSettingsApplyAndPersist(t *testing.T) {
	cfg := &models.Config{DataDir: t.TempDir(), AutoSwitchAvoidCountries: []string{"RU"}, SubscriptionRefreshInterval: 1800}
	reset := make(chan struct{}, 1)
	h := NewPanelSettingsHandler(cfg, nil, "", reset)

	rec := putSettings(h, `{"avoid_countries":["ru","by","RU"],"subscription_refresh_minutes":60}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body)
	}

	if got := cfg.AvoidCountries(); strings.Join(got, ",") != "RU,BY" {
		t.Errorf("avoid countries = %v, want RU,BY normalised and deduplicated", got)
	}
	if cfg.RefreshInterval() != time.Hour {
		t.Errorf("refresh = %v, want 1h", cfg.RefreshInterval())
	}
	select {
	case <-reset:
	default:
		t.Error("the refresh timer was not told to pick up the new interval")
	}

	fresh := &models.Config{DataDir: cfg.DataDir, AutoSwitchAvoidCountries: []string{"RU"}, SubscriptionRefreshInterval: 1800}
	if err := LoadPanelSettings(fresh); err != nil {
		t.Fatalf("LoadPanelSettings: %v", err)
	}
	if fresh.RefreshInterval() != time.Hour || len(fresh.AvoidCountries()) != 2 {
		t.Errorf("after restart: refresh %v, countries %v — saved settings were not applied", fresh.RefreshInterval(), fresh.AvoidCountries())
	}
}

func TestPanelSettingsRejectsBadInput(t *testing.T) {
	cfg := &models.Config{DataDir: t.TempDir(), SubscriptionRefreshInterval: 1800}
	h := NewPanelSettingsHandler(cfg, nil, "", make(chan struct{}, 1))

	for _, body := range []string{
		`{"avoid_countries":["RUS"],"subscription_refresh_minutes":30}`,
		`{"avoid_countries":["R1"],"subscription_refresh_minutes":30}`,
		`{"avoid_countries":[],"subscription_refresh_minutes":2}`,
		`{"avoid_countries":[],"subscription_refresh_minutes":5000}`,
	} {
		if rec := putSettings(h, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code %d, want 400", body, rec.Code)
		}
	}
	if cfg.RefreshInterval() != 30*time.Minute {
		t.Error("a rejected request must not change the live settings")
	}
}

func TestPanelSettingsZeroTurnsRefreshOff(t *testing.T) {
	cfg := &models.Config{DataDir: t.TempDir(), SubscriptionRefreshInterval: 1800}
	h := NewPanelSettingsHandler(cfg, nil, "", make(chan struct{}, 1))

	if rec := putSettings(h, `{"avoid_countries":[],"subscription_refresh_minutes":0}`); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d", rec.Code)
	}
	if cfg.RefreshInterval() != 0 {
		t.Errorf("refresh = %v, want off", cfg.RefreshInterval())
	}
}
