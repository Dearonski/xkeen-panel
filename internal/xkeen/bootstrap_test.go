package xkeen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// freshInstallDir reproduces what `xkeen -i` leaves behind: the stock inbounds,
// a comment-only outbounds file and the example routing aimed at vless-reality.
func freshInstallDir(t *testing.T, withGeo bool) (Runtime, string) {
	t.Helper()
	root := t.TempDir()
	confDir := filepath.Join(root, "configs")

	writeFile(t, filepath.Join(confDir, "03_inbounds.json"), `{"inbounds":[
		{"protocol":"tunnel","port":1181,"settings":{"allowedNetwork":"tcp","followRedirect":true},"tag":"redirect"},
		{"protocol":"tunnel","port":1181,"settings":{"allowedNetwork":"udp","followRedirect":true},"streamSettings":{"sockopt":{"tproxy":"tproxy"}},"tag":"tproxy"},
		{"protocol":"tunnel","port":1191,"settings":{"allowedNetwork":"tcp","followRedirect":true},"tag":"force-proxy-redirect"},
		{"protocol":"tunnel","port":1191,"settings":{"allowedNetwork":"udp","followRedirect":true},"streamSettings":{"sockopt":{"tproxy":"tproxy"}},"tag":"force-proxy-tproxy"}]}`)
	writeFile(t, filepath.Join(confDir, "04_outbounds.json"), "{\n// Создайте файл по ссылке https://zxc-rv.github.io/XKeen-UI/Outbound_Generator/\n}")
	writeFile(t, filepath.Join(confDir, "05_routing.json"), `// example
	{"routing":{"rules":[
		{"inboundTag":["force-proxy-redirect","force-proxy-tproxy"],"outboundTag":"vless-reality"},
		{"inboundTag":["redirect","tproxy"],"outboundTag":"direct"}]}}`)
	if withGeo {
		writeFile(t, filepath.Join(root, "dat", "geoip_v2fly.dat"), "")
		writeFile(t, filepath.Join(root, "dat", "geosite_v2fly.dat"), "")
	}

	rt := Runtime{XrayConfDir: confDir, RoutingFile: filepath.Join(confDir, "05_routing.json"), Core: CoreXray}
	return rt, filepath.Join(confDir, "04_outbounds.json")
}

func TestBootstrapFreshInstall(t *testing.T) {
	rt, outboundsPath := freshInstallDir(t, true)

	if !NeedsBootstrap(rt, ReadTopology(rt)) {
		t.Fatal("a comment-only outbounds file must need bootstrapping")
	}

	state, err := Bootstrap(rt, outboundsPath, poolServers(), PoolOptions{APIAddr: "127.0.0.1:10085"})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if !state.Enabled || state.APIFile == "" {
		t.Errorf("state = %+v, want an enabled pool with the panel's api block", state)
	}

	top := ReadTopology(rt)
	if top.Mode != TopologyPool || len(top.PoolTags) != 2 {
		t.Fatalf("topology = %+v, want a two-node pool", top)
	}
	if NeedsBootstrap(rt, top) {
		t.Error("a bootstrapped config must not be bootstrapped again")
	}

	cfg, _ := ReadOutboundsConfig(outboundsPath)
	for _, raw := range asSlice(cfg["outbounds"]) {
		ob := raw.(map[string]interface{})
		if ob["protocol"] == "blackhole" {
			continue
		}
		ss, _ := ob["streamSettings"].(map[string]interface{})
		sockopt, _ := ss["sockopt"].(map[string]interface{})
		if sockopt["mark"] != float64(255) {
			t.Errorf("%v: sockopt.mark = %v, want the integer 255 XKeen validates", ob["tag"], sockopt["mark"])
		}
	}

	rules := routingRules(t, rt)
	want := []string{"balancer", "direct", "direct", "direct", "balancer"}
	if len(rules) != len(want) {
		t.Fatalf("rules = %d, want %d: %v", len(rules), len(want), rules)
	}
	for i, raw := range rules {
		rule := raw.(map[string]interface{})
		got, _ := rule["outboundTag"].(string)
		if got == "" {
			got, _ = rule["balancerTag"].(string)
		}
		if got != want[i] {
			t.Errorf("rule %d goes to %q, want %q: %v", i, got, want[i], rule)
		}
	}

	forced := stringsOf(rules[0].(map[string]interface{})["inboundTag"])
	if len(forced) != 2 || forced[0] != "force-proxy-redirect" {
		t.Errorf("first rule inbounds = %v, want the force-proxy pair", forced)
	}
	ru := rules[2].(map[string]interface{})["domain"].([]interface{})
	if ru[len(ru)-1] != "ext:geosite_v2fly.dat:category-ru" {
		t.Errorf("Russian domains = %v, want the geosite category", ru)
	}

	ready, changed, err := EnsureProbeRoute(rt, "127.0.0.1:10086", top)
	if err != nil || !ready || !changed {
		t.Errorf("probe on a bootstrapped config: ready=%v changed=%v err=%v", ready, changed, err)
	}
}

// Without geo databases the rules must not reference them, or Xray refuses the
// whole config.
func TestBootstrapWithoutGeoFiles(t *testing.T) {
	rt, outboundsPath := freshInstallDir(t, false)

	if _, err := Bootstrap(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	data, err := readFileString(rt.RoutingFile)
	if err != nil {
		t.Fatal(err)
	}
	if containsAny(data, "geoip_v2fly", "geosite_v2fly") {
		t.Errorf("routing references missing geo files:\n%s", data)
	}
}

func readFileString(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
