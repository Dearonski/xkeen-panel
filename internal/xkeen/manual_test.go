package xkeen

import (
	"strings"
	"testing"

	"xkeen-panel/internal/models"
)

const finlandURI = "vless://33333333-4444-5555-6666-777777777777@fi.example:443?type=tcp&security=reality&sni=fi.com&fp=chrome&pbk=KEY&sid=cd#FI"

func pooledConfDir(t *testing.T) (Runtime, string) {
	t.Helper()
	rt, outboundsPath := liveConfDir(t)
	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}
	return rt, outboundsPath
}

func outboundTags(t *testing.T, path string) []string {
	t.Helper()
	cfg, err := ReadOutboundsConfig(path)
	if err != nil {
		t.Fatalf("read outbounds: %v", err)
	}
	var tags []string
	for _, raw := range asSlice(cfg["outbounds"]) {
		tags = append(tags, outboundTag(raw.(map[string]interface{})))
	}
	return tags
}

func TestSetManualOutbound(t *testing.T) {
	rt, outboundsPath := pooledConfDir(t)
	fi := models.Server{Name: "FI", Protocol: "vless", RawURI: finlandURI}

	if err := SetManualOutbound(rt, outboundsPath, fi); err != nil {
		t.Fatalf("SetManualOutbound: %v", err)
	}

	// Pool first (the first outbound is Xray's default route), services last
	if got := strings.Join(outboundTags(t, outboundsPath), ","); got != "sub-1,sub-2,manual,direct,block" {
		t.Errorf("outbounds = %s", got)
	}

	cfg, _ := ReadOutboundsConfig(outboundsPath)
	slot := manualOutbound(asSlice(cfg["outbounds"]))
	sockopt := slot["streamSettings"].(map[string]interface{})["sockopt"].(map[string]interface{})
	if sockopt["mark"] != "0xffffaaa" {
		t.Errorf("slot lost the mark XKeen validates: %v", sockopt)
	}

	if key := NodeKeyForTag(outboundsPath, DefaultPoolSelector, ManualTag); key != "fi.example:443:33333333-4444-5555-6666-777777777777" {
		t.Errorf("NodeKeyForTag(manual) = %q", key)
	}

	top := ReadTopology(rt)
	for _, tag := range top.PoolTags {
		if tag == ManualTag {
			t.Error("the balancer must not adopt the manual slot")
		}
	}

	// Picking another server replaces the slot instead of adding a second one
	de := models.Server{Name: "DE", Protocol: "vless", RawURI: wsURI}
	if err := SetManualOutbound(rt, outboundsPath, de); err != nil {
		t.Fatalf("second SetManualOutbound: %v", err)
	}
	if got := strings.Count(strings.Join(outboundTags(t, outboundsPath), ","), ManualTag); got != 1 {
		t.Errorf("manual slots = %d, want 1", got)
	}
}

func TestSyncPoolKeepsManualSlot(t *testing.T) {
	rt, outboundsPath := pooledConfDir(t)
	if err := SetManualOutbound(rt, outboundsPath, models.Server{Name: "FI", Protocol: "vless", RawURI: finlandURI}); err != nil {
		t.Fatalf("SetManualOutbound: %v", err)
	}

	state := PoolState{BalancerTag: DefaultBalancerTag, Selector: DefaultPoolSelector}
	if err := SyncPool(rt, outboundsPath, poolServers()[:1], state, PoolSelection{MaxNodes: 1}); err != nil {
		t.Fatalf("SyncPool: %v", err)
	}

	if got := strings.Join(outboundTags(t, outboundsPath), ","); got != "sub-1,manual,direct,block" {
		t.Errorf("outbounds after sync = %s — traffic may be pinned to the slot", got)
	}
}

func TestSetManualOutboundRejectsNonVLESS(t *testing.T) {
	rt, outboundsPath := pooledConfDir(t)
	err := SetManualOutbound(rt, outboundsPath, models.Server{Name: "SS", Protocol: "ss", RawURI: "ss://x@y:1"})
	if err == nil {
		t.Fatal("a non-VLESS server cannot be rendered into an Xray outbound")
	}
}
