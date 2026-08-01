package xkeen

import (
	"os"
	"path/filepath"
	"testing"

	"xkeen-panel/internal/models"
)

// A rotated server leaves the pool pointing at an endpoint that no longer
// answers, which is exactly how a pool takes the whole VPN down.
func TestRefreshPoolPicksUpRotatedServer(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	rotated := []models.Server{
		{Name: "NL", Protocol: "vless", RawURI: "vless://99999999-8888-7777-6666-555555555555@new.example:443?type=raw&security=reality&pbk=KEY"},
		{Name: "DE", Protocol: "vless", RawURI: wsURI},
	}

	// No dispatcher in the fixture, so the live path is unavailable and the
	// restart is a no-op — the file content is what matters here
	result, err := RefreshPool(rt, outboundsPath, "", rotated, state)
	if err != nil {
		t.Fatalf("RefreshPool: %v", err)
	}
	if !result.Changed {
		t.Fatal("rotation not detected — the pool would keep the dead endpoint")
	}

	matches, err := PoolMatchesSubscription(outboundsPath, rotated, DefaultPoolSelector)
	if err != nil {
		t.Fatalf("PoolMatchesSubscription: %v", err)
	}
	if !matches {
		t.Error("pool still differs from the subscription after a refresh")
	}
}

// Rewriting an unchanged pool would mean a restart for nothing.
func TestRefreshPoolNoopWhenUnchanged(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	before, _ := os.ReadFile(outboundsPath)

	result, err := RefreshPool(rt, outboundsPath, "", poolServers(), state)
	if err != nil {
		t.Fatalf("RefreshPool: %v", err)
	}
	if result.Changed || result.Restarted {
		t.Errorf("result = %+v, want an untouched pool", result)
	}

	after, _ := os.ReadFile(outboundsPath)
	if string(before) != string(after) {
		t.Error("config was rewritten despite no change")
	}
}

func TestRefreshPoolReportsAddedAndRemoved(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	result, err := RefreshPool(rt, outboundsPath, "", poolServers()[:1], state)
	if err != nil {
		t.Fatalf("RefreshPool: %v", err)
	}

	if len(result.Removed) != 1 || result.Removed[0] != "sub-2" {
		t.Errorf("Removed = %v, want [sub-2]", result.Removed)
	}
	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want none", result.Added)
	}
}

// Pinning needs RoutingService and hot pool updates need HandlerService; a pool
// created before HandlerService existed must be upgraded in place.
func TestEnsureHandlerServiceUpgradesOldAPIBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), apiConfigFile)
	old := `{"api":{"tag":"api","services":["RoutingService"]},"inbounds":[]}`
	if err := os.WriteFile(path, []byte(old), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	upgraded, err := ensureHandlerService(path)
	if err != nil {
		t.Fatalf("ensureHandlerService: %v", err)
	}
	if !upgraded {
		t.Fatal("upgrade not reported")
	}

	var cfg map[string]interface{}
	if err := ReadJSONC(path, &cfg); err != nil {
		t.Fatalf("read: %v", err)
	}
	services := cfg["api"].(map[string]interface{})["services"].([]interface{})
	if len(services) != 2 || services[0] != "RoutingService" || services[1] != "HandlerService" {
		t.Errorf("services = %v, want both", services)
	}

	// Second call must be a no-op
	if again, _ := ensureHandlerService(path); again {
		t.Error("already-upgraded block reported as changed")
	}
}

func TestEnsureHandlerServiceIgnoresForeignBlock(t *testing.T) {
	if upgraded, err := ensureHandlerService(""); upgraded || err != nil {
		t.Errorf("upgraded=%v err=%v, want a no-op when the panel does not own the file", upgraded, err)
	}
}

func TestNewPoolWritesHandlerService(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{APIAddr: "127.0.0.1:10085"})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	var cfg map[string]interface{}
	if err := ReadJSONC(state.APIFile, &cfg); err != nil {
		t.Fatalf("read api config: %v", err)
	}

	services := cfg["api"].(map[string]interface{})["services"].([]interface{})
	found := map[string]bool{}
	for _, s := range services {
		found[s.(string)] = true
	}
	if !found["RoutingService"] || !found["HandlerService"] {
		t.Errorf("services = %v, want RoutingService and HandlerService", services)
	}
}
