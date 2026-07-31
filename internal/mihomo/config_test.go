package mihomo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"xkeen-panel/internal/models"
)

const (
	realityURI = "vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=raw&security=reality&sni=example.com&fp=chrome&pbk=KEY&sid=ab&flow=xtls-rprx-vision#NL"
	wsURI      = "vless://22222222-3333-4444-5555-666666666666@host.example:443?type=ws&host=h.com&path=/p&security=tls&sni=s.com#DE"
)

func servers() []models.Server {
	return []models.Server{
		{Name: "NL", Protocol: "vless", RawURI: realityURI},
		{Name: "DE", Protocol: "vless", RawURI: wsURI},
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// The file XKeen ships is two settings and a comment.
func TestSyncProxiesOnShippedConfig(t *testing.T) {
	path := writeConfig(t, "tproxy-port: 1181\nredir-port: 1182\n# Руководство по конфигурации Mihomo\n")

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	names, err := cfg.SyncProxies(servers())
	if err != nil {
		t.Fatalf("SyncProxies: %v", err)
	}
	if err := cfg.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("names = %v, want two", names)
	}

	body, _ := os.ReadFile(path)
	text := string(body)

	if !strings.Contains(text, "tproxy-port: 1181") {
		t.Error("existing settings were dropped")
	}
	if !strings.Contains(text, "Руководство по конфигурации Mihomo") {
		t.Error("comment was lost — yaml.Node is used precisely to keep it")
	}

	var parsed struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("result does not parse: %v", err)
	}
	if len(parsed.Proxies) != 2 {
		t.Fatalf("proxies = %d, want 2", len(parsed.Proxies))
	}

	reality := parsed.Proxies[0]
	if reality["type"] != "vless" || reality["server"] != "1.2.3.4" {
		t.Errorf("unexpected proxy: %v", reality)
	}
	if reality["tls"] != true {
		t.Error("reality must imply tls: true in Mihomo")
	}
	opts, ok := reality["reality-opts"].(map[string]interface{})
	if !ok || opts["public-key"] != "KEY" || opts["short-id"] != "ab" {
		t.Errorf("reality-opts = %v", reality["reality-opts"])
	}
	// Xray's "raw" is Mihomo's default transport, written by omitting the key
	if _, present := reality["network"]; present {
		t.Errorf("network = %v, want it omitted for raw/tcp", reality["network"])
	}

	ws := parsed.Proxies[1]
	if ws["network"] != "ws" {
		t.Errorf("network = %v, want ws", ws["network"])
	}
	wsOpts := ws["ws-opts"].(map[string]interface{})
	if wsOpts["path"] != "/p" {
		t.Errorf("ws path = %v", wsOpts["path"])
	}
	headers := wsOpts["headers"].(map[string]interface{})
	if headers["Host"] != "h.com" {
		t.Errorf("ws Host = %v", headers["Host"])
	}
}

// XKeen checks routing-mark 255 on every proxy when Entware proxying is on;
// regenerating the list must not silently drop it.
func TestSyncProxiesKeepsRoutingMark(t *testing.T) {
	path := writeConfig(t, `proxies:
  - name: old
    type: vless
    server: old.example
    port: 443
    uuid: old-uuid
    routing-mark: 255
`)

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := cfg.RoutingMark(); got != EntwareBypassMark {
		t.Fatalf("RoutingMark = %d, want %d", got, EntwareBypassMark)
	}
	if _, err := cfg.SyncProxies(servers()); err != nil {
		t.Fatalf("SyncProxies: %v", err)
	}
	if err := cfg.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var parsed struct {
		Proxies []map[string]interface{} `yaml:"proxies"`
	}
	body, _ := os.ReadFile(path)
	yaml.Unmarshal(body, &parsed)

	for _, proxy := range parsed.Proxies {
		if proxy["routing-mark"] != EntwareBypassMark {
			t.Errorf("proxy %v lost routing-mark", proxy["name"])
		}
	}
}

func TestRoutingMarkFromGlobalKey(t *testing.T) {
	path := writeConfig(t, "routing-mark: 255\nproxies: []\n")

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := cfg.RoutingMark(); got != EntwareBypassMark {
		t.Errorf("RoutingMark = %d, want %d", got, EntwareBypassMark)
	}
}

func TestSyncProxiesRetargetsGroups(t *testing.T) {
	path := writeConfig(t, `proxy-groups:
  - name: PROXY
    type: url-test
    proxies: [old-1, old-2, DIRECT]
  - name: FALLBACK
    type: select
    proxies: [PROXY, DIRECT]
proxies:
  - name: old-1
    type: vless
    server: a
    port: 443
    uuid: u
`)

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	names, err := cfg.SyncProxies(servers())
	if err != nil {
		t.Fatalf("SyncProxies: %v", err)
	}
	if err := cfg.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}

	var parsed struct {
		Groups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
	}
	body, _ := os.ReadFile(path)
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}

	proxyGroup := parsed.Groups[0]
	if proxyGroup.Proxies[0] != "DIRECT" {
		t.Errorf("DIRECT must survive: %v", proxyGroup.Proxies)
	}
	for _, stale := range []string{"old-1", "old-2"} {
		for _, got := range proxyGroup.Proxies {
			if got == stale {
				t.Errorf("stale proxy %q left in the group", stale)
			}
		}
	}
	if len(proxyGroup.Proxies) != 1+len(names) {
		t.Errorf("group proxies = %v, want DIRECT plus the new names", proxyGroup.Proxies)
	}

	// A nested group reference is not a proxy name and must be kept
	fallback := parsed.Groups[1]
	if fallback.Proxies[0] != "PROXY" {
		t.Errorf("nested group reference lost: %v", fallback.Proxies)
	}
}

func TestSyncProxiesRejectsEmptySubscription(t *testing.T) {
	path := writeConfig(t, "tproxy-port: 1181\n")

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := cfg.SyncProxies([]models.Server{{Protocol: "shadowsocks", RawURI: "ss://x@h:1"}}); err == nil {
		t.Error("expected an error when no VLESS server is available")
	}
}

func TestSyncProxiesDeduplicatesNames(t *testing.T) {
	path := writeConfig(t, "tproxy-port: 1181\n")

	cfg, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	same := []models.Server{
		{Name: "NL", Protocol: "vless", RawURI: realityURI},
		{Name: "NL", Protocol: "vless", RawURI: wsURI},
	}
	names, err := cfg.SyncProxies(same)
	if err != nil {
		t.Fatalf("SyncProxies: %v", err)
	}

	if names[0] == names[1] {
		t.Errorf("names = %v, want unique — Mihomo rejects duplicates", names)
	}
}

func TestWriteKeepsBackup(t *testing.T) {
	original := "tproxy-port: 1181\n"
	path := writeConfig(t, original)

	cfg, _ := Read(path)
	if _, err := cfg.SyncProxies(servers()); err != nil {
		t.Fatalf("SyncProxies: %v", err)
	}
	if err := cfg.Write(); err != nil {
		t.Fatalf("Write: %v", err)
	}

	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(backup) != original {
		t.Error("backup does not hold the pre-write content")
	}

	if err := RestoreBackup(path); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	restored, _ := os.ReadFile(path)
	if string(restored) != original {
		t.Error("restore did not bring the original back")
	}
}
