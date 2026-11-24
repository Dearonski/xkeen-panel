package xkeen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"xkeen-panel/internal/models"
)

func loadSub(t *testing.T, servers []models.Server, activeID int) *SubscriptionManager {
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
	sm := NewSubscriptionManager(dir)
	if err := sm.Load(); err != nil {
		t.Fatal(err)
	}
	return sm
}

func TestSetActiveByRawURI(t *testing.T) {
	sm := loadSub(t, []models.Server{
		{ID: 0, RawURI: "vless://a@1.1.1.1:443#A", Active: true},
		{ID: 1, RawURI: "vless://b@2.2.2.2:443#B"},
	}, 0)

	s, err := sm.SetActiveByRawURI("vless://b@2.2.2.2:443#B")
	if err != nil {
		t.Fatalf("SetActiveByRawURI: %v", err)
	}
	if s.RawURI != "vless://b@2.2.2.2:443#B" {
		t.Errorf("wrong server activated: %q", s.RawURI)
	}
	if got := sm.GetActiveServer(); got == nil || got.RawURI != "vless://b@2.2.2.2:443#B" {
		t.Error("the active server was not set")
	}
	if _, err := sm.SetActiveByRawURI("vless://missing"); err == nil {
		t.Error("want an error for an unknown RawURI")
	}
}

// Regression: GetData must not hand out an alias of the internal server slice.
func TestGetDataDeepCopy(t *testing.T) {
	sm := loadSub(t, []models.Server{
		{ID: 0, RawURI: "u0", Country: "NL"},
		{ID: 1, RawURI: "u1"},
	}, 0)

	d := sm.GetData()
	d.Servers[0].Country = "MUTATED"

	if got := sm.GetServers(); got[0].Country != "NL" {
		t.Errorf("GetData returned an alias: internal Country = %q, want NL", got[0].Country)
	}
}

func TestRefreshKeepsMeasuredLatency(t *testing.T) {
	checked := time.Now().Add(-time.Minute).Truncate(time.Second)
	old := []models.Server{
		{RawURI: "vless://a", Latency: 87, LastChecked: checked, CountryOverride: "SE"},
		{RawURI: "vless://gone", Latency: 40},
	}
	fresh := []models.Server{{RawURI: "vless://new"}, {RawURI: "vless://a"}}

	carryOverrides(old, fresh)

	if fresh[1].Latency != 87 || !fresh[1].LastChecked.Equal(checked) || fresh[1].CountryOverride != "SE" {
		t.Errorf("server = %+v, want its ping and country carried over the refresh", fresh[1])
	}
	if fresh[0].Latency != 0 {
		t.Errorf("a new server got latency %d from nowhere", fresh[0].Latency)
	}
}
