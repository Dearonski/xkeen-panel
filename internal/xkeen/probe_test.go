package xkeen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureProbeRouteSingle(t *testing.T) {
	rt, _ := liveConfDir(t)

	ready, changed, err := EnsureProbeRoute(rt, "127.0.0.1:10086", ReadTopology(rt))
	if err != nil {
		t.Fatalf("EnsureProbeRoute: %v", err)
	}
	if !ready || !changed {
		t.Fatal("first run must report a change so the core gets restarted")
	}

	var inbound map[string]interface{}
	if err := ReadJSONC(filepath.Join(rt.XrayConfDir, probeInboundFile), &inbound); err != nil {
		t.Fatalf("read probe inbound: %v", err)
	}
	if _, present := inbound["routing"]; present {
		t.Error("probe file must not carry routing — it would replace the owner's rules")
	}
	ib := asSlice(inbound["inbounds"])[0].(map[string]interface{})
	if ib["protocol"] != "socks" || ib["listen"] != "127.0.0.1" || ib["port"] != float64(10086) {
		t.Errorf("unexpected probe inbound: %v", ib)
	}

	rules := routingRules(t, rt)
	first := rules[0].(map[string]interface{})
	if !isProbeRule(first) || first["outboundTag"] != "vless-reality" {
		t.Errorf("probe rule must come first and follow the proxy outbound, got %v", first)
	}

	ready, changed, err = EnsureProbeRoute(rt, "127.0.0.1:10086", ReadTopology(rt))
	if err != nil {
		t.Fatalf("second EnsureProbeRoute: %v", err)
	}
	if !ready || changed {
		t.Error("an installed probe must not restart the core again")
	}
	if got := len(routingRules(t, rt)); got != len(rules) {
		t.Errorf("rules = %d after rerun, want %d", got, len(rules))
	}
}

// Enabling the pool retargets every rule aimed at the proxy outbound, the probe
// rule included, so checks keep following client traffic.
func TestProbeRuleFollowsPool(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	if _, _, err := EnsureProbeRoute(rt, "127.0.0.1:10086", ReadTopology(rt)); err != nil {
		t.Fatalf("EnsureProbeRoute: %v", err)
	}
	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	first := routingRules(t, rt)[0].(map[string]interface{})
	if !isProbeRule(first) || first["balancerTag"] != DefaultBalancerTag {
		t.Errorf("probe rule = %v, want it on the balancer", first)
	}

	_, changed, err := EnsureProbeRoute(rt, "127.0.0.1:10086", ReadTopology(rt))
	if err != nil {
		t.Fatalf("EnsureProbeRoute after pool: %v", err)
	}
	if changed {
		t.Error("the retargeted rule already matches — nothing to rewrite")
	}
}

func TestEnsureProbeRouteSkipsEmptyConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "04_outbounds.json"), []byte("{\n// generator link\n}"), 0644)
	rt := Runtime{XrayConfDir: dir, Core: CoreXray}

	ready, changed, err := EnsureProbeRoute(rt, "127.0.0.1:10086", ReadTopology(rt))
	if err != nil || ready || changed {
		t.Errorf("no proxy outbound yet: ready=%v changed=%v err=%v, want a silent no-op", ready, changed, err)
	}
}

func routingRules(t *testing.T, rt Runtime) []interface{} {
	t.Helper()
	var cfg map[string]interface{}
	if err := ReadJSONC(rt.RoutingFile, &cfg); err != nil {
		t.Fatalf("read routing: %v", err)
	}
	return asSlice(cfg["routing"].(map[string]interface{})["rules"])
}
