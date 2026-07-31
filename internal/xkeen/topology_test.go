package xkeen

import (
	"os"
	"path/filepath"
	"testing"
)

// confDirFromFixtures builds a config directory out of the live router files
// plus any extra files the test needs.
func confDirFromFixtures(t *testing.T, fixtures map[string]string, extra map[string]string) Runtime {
	t.Helper()
	dir := t.TempDir()

	for name, fixture := range fixtures {
		data, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			t.Fatalf("read fixture %s: %v", fixture, err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	for name, body := range extra {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	return Runtime{XrayConfDir: dir, RoutingFile: filepath.Join(dir, "05_routing.json")}
}

// The live config: one proxy outbound, no balancers, comments everywhere.
func TestReadTopologyLiveSingle(t *testing.T) {
	rt := confDirFromFixtures(t, map[string]string{
		"04_outbounds.json": "outbounds_flat.json",
		"05_routing.json":   "routing_single.json",
	}, nil)

	top := ReadTopology(rt)

	if top.Mode != TopologySingle {
		t.Errorf("Mode = %q, want single", top.Mode)
	}
	if len(top.ProxyTags) != 1 || top.ProxyTags[0] != "vless-reality" {
		t.Errorf("ProxyTags = %v, want [vless-reality]", top.ProxyTags)
	}
}

func TestReadTopologyPool(t *testing.T) {
	rt := confDirFromFixtures(t, nil, map[string]string{
		"04_outbounds.json": `{"outbounds":[
			{"tag":"sub-1","protocol":"vless"},
			{"tag":"sub-2","protocol":"vless"},
			{"tag":"other","protocol":"vless"},
			{"tag":"direct","protocol":"freedom"}]}`,
		"05_routing.json": `{"routing":{"balancers":[
			{"tag":"balancer","selector":["sub-"],"strategy":{"type":"leastPing"}}],
			"rules":[{"inboundTag":["tproxy"],"balancerTag":"balancer"}]}}`,
	})

	top := ReadTopology(rt)

	if top.Mode != TopologyPool {
		t.Fatalf("Mode = %q, want pool", top.Mode)
	}
	if top.BalancerTag != "balancer" {
		t.Errorf("BalancerTag = %q, want balancer", top.BalancerTag)
	}
	if len(top.PoolTags) != 2 {
		t.Errorf("PoolTags = %v, want the two sub- tags only", top.PoolTags)
	}
	if len(top.ProxyTags) != 3 {
		t.Errorf("ProxyTags = %v, want all three proxy outbounds", top.ProxyTags)
	}
}

// A balancer defined in a file other than the routing one still counts:
// Xray merges the whole directory.
func TestReadTopologyBalancerInAnotherFile(t *testing.T) {
	rt := confDirFromFixtures(t, nil, map[string]string{
		"04_outbounds.json": `{"outbounds":[{"tag":"sub-1","protocol":"vless"}]}`,
		"06_balancer.json":  `{"routing":{"balancers":[{"tag":"b","selector":["sub-"]}]}}`,
	})

	if got := ReadTopology(rt); got.Mode != TopologyPool || got.BalancerTag != "b" {
		t.Errorf("topology = %+v, want pool with tag b", got)
	}
}

// A balancer without a selector cannot address anything — not a pool.
func TestReadTopologyIgnoresSelectorlessBalancer(t *testing.T) {
	rt := confDirFromFixtures(t, nil, map[string]string{
		"05_routing.json": `{"routing":{"balancers":[{"tag":"b"}]}}`,
	})

	if got := ReadTopology(rt).Mode; got != TopologySingle {
		t.Errorf("Mode = %q, want single", got)
	}
}

func TestReadTopologyMissingDir(t *testing.T) {
	rt := Runtime{XrayConfDir: filepath.Join(t.TempDir(), "absent")}

	if got := ReadTopology(rt).Mode; got != TopologySingle {
		t.Errorf("Mode = %q, want single for a missing config dir", got)
	}
}
