package xkeen

import (
	"os"
	"path/filepath"
	"testing"

	"xkeen-panel/internal/models"
)

func poolServers() []models.Server {
	return []models.Server{
		{Name: "NL", Protocol: "vless", RawURI: realityURI},
		{Name: "DE", Protocol: "vless", RawURI: wsURI},
	}
}

// liveConfDir reproduces the router's config directory: flat outbounds with a
// mark, and the commented routing whose catch-all rule points at vless-reality.
func liveConfDir(t *testing.T) (Runtime, string) {
	t.Helper()
	dir := t.TempDir()

	outbounds := `{"outbounds":[
		{"tag":"vless-reality","protocol":"vless",
		 "settings":{"address":"old.example.com","port":443,"id":"old-uuid"},
		 "streamSettings":{"network":"raw","sockopt":{"mark":"0xffffaaa"}}},
		{"protocol":"freedom","tag":"direct"},
		{"protocol":"blackhole","tag":"block"}]}`
	routing := `// routing
	{"routing":{"rules":[
		{"inboundTag":["redirect","tproxy"],"outboundTag":"block","type":"field","domain":["ads.example"]},
		{"inboundTag":["redirect","tproxy"],"outboundTag":"vless-reality","type":"field"}]}}`

	outboundsPath := filepath.Join(dir, "04_outbounds.json")
	if err := os.WriteFile(outboundsPath, []byte(outbounds), 0644); err != nil {
		t.Fatalf("write outbounds: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "05_routing.json"), []byte(routing), 0644); err != nil {
		t.Fatalf("write routing: %v", err)
	}

	return Runtime{XrayConfDir: dir, RoutingFile: filepath.Join(dir, "05_routing.json"), Core: CoreXray}, outboundsPath
}

func TestEnablePoolBuildsBalancer(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	if state.OriginalTag != "vless-reality" {
		t.Errorf("OriginalTag = %q, want vless-reality — needed to undo the change", state.OriginalTag)
	}

	top := ReadTopology(rt)
	if top.Mode != TopologyPool {
		t.Fatalf("Mode = %q, want pool", top.Mode)
	}
	if len(top.PoolTags) != 2 {
		t.Errorf("PoolTags = %v, want two nodes", top.PoolTags)
	}

	// Every node must carry the policy mark, or XKeen rejects the whole pool
	cfg, _ := ReadOutboundsConfig(outboundsPath)
	nodes := 0
	for _, raw := range cfg["outbounds"].([]interface{}) {
		ob := raw.(map[string]interface{})
		if isServiceOutbound(ob) {
			continue
		}
		nodes++
		ss, _ := ob["streamSettings"].(map[string]interface{})
		sockopt, ok := ss["sockopt"].(map[string]interface{})
		if !ok || sockopt["mark"] != "0xffffaaa" {
			tag, _ := ob["tag"].(string)
			t.Errorf("node %s lost sockopt.mark", tag)
		}
	}
	if nodes != 2 {
		t.Errorf("proxy outbounds = %d, want 2", nodes)
	}

	// direct/block must survive
	if len(serviceOutbounds(cfg["outbounds"].([]interface{}))) != 2 {
		t.Error("service outbounds were dropped")
	}
}

func TestEnablePoolRetargetsOnlyProxyRules(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	var cfg map[string]interface{}
	if err := ReadJSONC(filepath.Join(rt.XrayConfDir, "05_routing.json"), &cfg); err != nil {
		t.Fatalf("read routing: %v", err)
	}
	rules := cfg["routing"].(map[string]interface{})["rules"].([]interface{})

	blockRule := rules[0].(map[string]interface{})
	if blockRule["outboundTag"] != "block" {
		t.Errorf("block rule was rewritten: %v", blockRule)
	}

	proxyRule := rules[1].(map[string]interface{})
	if proxyRule["balancerTag"] != DefaultBalancerTag {
		t.Errorf("balancerTag = %v, want %s", proxyRule["balancerTag"], DefaultBalancerTag)
	}
	if _, still := proxyRule["outboundTag"]; still {
		t.Error("outboundTag must be removed when a rule moves to a balancer")
	}

	// leastPing needs observatory probing to rank nodes
	if _, ok := cfg["observatory"]; !ok {
		t.Error("observatory block missing — leastPing would have no latency data")
	}
}

func TestDisablePoolRestoresSingleOutbound(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	if err := DisablePool(rt, outboundsPath, &models.Server{Protocol: "vless", RawURI: realityURI}, state); err != nil {
		t.Fatalf("DisablePool: %v", err)
	}

	top := ReadTopology(rt)
	if top.Mode != TopologySingle {
		t.Errorf("Mode = %q, want single", top.Mode)
	}
	if len(top.ProxyTags) != 1 || top.ProxyTags[0] != "vless-reality" {
		t.Errorf("ProxyTags = %v, want the original [vless-reality]", top.ProxyTags)
	}

	var cfg map[string]interface{}
	if err := ReadJSONC(filepath.Join(rt.XrayConfDir, "05_routing.json"), &cfg); err != nil {
		t.Fatalf("read routing: %v", err)
	}
	routing := cfg["routing"].(map[string]interface{})
	if _, still := routing["balancers"]; still {
		t.Error("balancers block left behind")
	}
	rule := routing["rules"].([]interface{})[1].(map[string]interface{})
	if rule["outboundTag"] != "vless-reality" {
		t.Errorf("rule outboundTag = %v, want vless-reality", rule["outboundTag"])
	}
}

func TestSyncPoolTracksSubscription(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	state, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	if err := SyncPool(rt, outboundsPath, poolServers()[:1], state, PoolSelection{}); err != nil {
		t.Fatalf("SyncPool: %v", err)
	}

	if got := ReadTopology(rt).PoolTags; len(got) != 1 {
		t.Errorf("PoolTags = %v, want one node after the subscription shrank", got)
	}
}

// Without a rule pointing at the proxy outbound the panel cannot know where
// traffic enters, so it must refuse rather than write a pool nothing uses.
func TestEnablePoolRefusesWithoutProxyRule(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	routing := `{"routing":{"rules":[{"inboundTag":["tproxy"],"outboundTag":"direct","type":"field"}]}}`
	if err := os.WriteFile(filepath.Join(rt.XrayConfDir, "05_routing.json"), []byte(routing), 0644); err != nil {
		t.Fatalf("write routing: %v", err)
	}

	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err == nil {
		t.Fatal("expected an error when no rule targets the proxy outbound")
	}

	if ReadTopology(rt).Mode != TopologySingle {
		t.Error("config was modified despite the failure")
	}
}

func TestParseBalancerInfoPrefersOverride(t *testing.T) {
	out := `Balancer balancer
Selecting Override:
    1   sub-2
Selects:
    1   sub-1
    2   sub-2`

	if got := parseBalancerInfo(out); got != "sub-2" {
		t.Errorf("parseBalancerInfo = %q, want sub-2 (override wins over leastPing)", got)
	}
}

func TestParseBalancerInfoFallsBackToSelects(t *testing.T) {
	out := `Balancer balancer
Selecting Override:
Selects:
    1   sub-3`

	if got := parseBalancerInfo(out); got != "sub-3" {
		t.Errorf("parseBalancerInfo = %q, want sub-3", got)
	}
}
