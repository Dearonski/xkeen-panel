package xkeen

import (
	"os"
	"path/filepath"
	"testing"
)

// `xkeen -sb on` installs the api block, but that flag only exists on the Beta
// channel — on Stable the panel writes it, or pinning a node is impossible.
func TestEnablePoolWritesAPIConfig(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{APIAddr: "127.0.0.1:10085"})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	apiPath := filepath.Join(rt.XrayConfDir, apiConfigFile)
	if state.APIFile != apiPath {
		t.Errorf("APIFile = %q, want %q", state.APIFile, apiPath)
	}

	var cfg map[string]interface{}
	if err := ReadJSONC(apiPath, &cfg); err != nil {
		t.Fatalf("read api config: %v", err)
	}

	api := cfg["api"].(map[string]interface{})

	// Xray binds the port itself; the older recipe went through a dokodemo-door
	// inbound and a routing rule, and never actually listened
	if api["listen"] != "127.0.0.1:10085" {
		t.Errorf("listen = %v, want 127.0.0.1:10085", api["listen"])
	}
	if api["tag"] != "api" {
		t.Errorf("tag = %v, want api", api["tag"])
	}
	if _, present := cfg["inbounds"]; present {
		t.Error("a listening api block needs no inbound — XKeen scans inbounds for transparent ports")
	}
	if _, present := cfg["routing"]; present {
		t.Error("a listening api block needs no routing rule")
	}
}

// A block installed by `xkeen -sb on` must survive both enabling and leaving
// pool mode, or the fork's speed balancer breaks.
func TestEnablePoolKeepsForeignAPIConfig(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	apiPath := filepath.Join(rt.XrayConfDir, apiConfigFile)
	foreign := `{"api":{"tag":"api","services":["RoutingService","StatsService"]}}`
	if err := os.WriteFile(apiPath, []byte(foreign), 0644); err != nil {
		t.Fatalf("write foreign api config: %v", err)
	}

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{APIAddr: "127.0.0.1:10085"})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}
	if state.APIFile != "" {
		t.Error("a foreign api config must not be recorded as panel-owned")
	}

	body, _ := os.ReadFile(apiPath)
	if string(body) != foreign {
		t.Error("foreign api config was overwritten")
	}

	if err := DisablePool(rt, outboundsPath, &poolServers()[0], state); err != nil {
		t.Fatalf("DisablePool: %v", err)
	}
	if _, err := os.Stat(apiPath); err != nil {
		t.Error("foreign api config was deleted when leaving pool mode")
	}
}

func TestDisablePoolRemovesOwnAPIConfig(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{APIAddr: "127.0.0.1:10085"})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	if err := DisablePool(rt, outboundsPath, &poolServers()[0], state); err != nil {
		t.Fatalf("DisablePool: %v", err)
	}

	if _, err := os.Stat(state.APIFile); !os.IsNotExist(err) {
		t.Error("panel-created api config should be removed when leaving pool mode")
	}
}

func TestSplitAPIAddrRejectsGarbage(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1", ":10085", "127.0.0.1:abc", "127.0.0.1:0"} {
		if _, _, err := splitAPIAddr(addr); err == nil {
			t.Errorf("splitAPIAddr(%q) accepted an invalid address", addr)
		}
	}
}
