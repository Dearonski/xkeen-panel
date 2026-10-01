package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"xkeen-panel/internal/models"
	"xkeen-panel/internal/xkeen"
)

func subWith(t *testing.T, servers []models.Server, activeID int) *xkeen.SubscriptionManager {
	t.Helper()
	dir := t.TempDir()
	data := models.SubscriptionData{Servers: servers, ActiveID: activeID}
	b, err := json.MarshalIndent(&data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "subscription.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	sm := xkeen.NewSubscriptionManager(dir)
	if err := sm.Load(); err != nil {
		t.Fatal(err)
	}
	return sm
}

func TestAllowedActiveOrBest(t *testing.T) {
	cfg := &models.Config{AutoSwitchAvoidCountries: []string{"RU", "BY"}}

	// The active server is allowed: it is returned as is, with no probing.
	subOk := subWith(t, []models.Server{
		{ID: 0, Name: "nl", Country: "NL", Protocol: "vless", RawURI: "u-nl", Active: true},
	}, 0)
	if got := NewWatchdog(cfg, subOk, xkeen.NewDetector(t.TempDir(), "", "", "", "", "", "")).AllowedActiveOrBest(); got == nil || got.Country != "NL" {
		t.Errorf("got %+v, want the active NL", got)
	}

	// Active sits in an avoided country and there is no allowed replacement.
	subRu := subWith(t, []models.Server{
		{ID: 0, Name: "ru", Country: "RU", Protocol: "vless", RawURI: "u-ru", Active: true},
	}, 0)
	if got := NewWatchdog(cfg, subRu, xkeen.NewDetector(t.TempDir(), "", "", "", "", "", "")).AllowedActiveOrBest(); got == nil || got.Country != "RU" {
		t.Errorf("got %+v, want a fallback to the current RU", got)
	}
}
