package xkeen

import (
	"os"
	"path/filepath"
	"testing"
	"xkeen-panel/internal/models"
)

const realityURI = "vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=tcp&security=reality&sni=example.com&fp=chrome&pbk=KEY&sid=ab&flow=xtls-rprx-vision"

const wsURI = "vless://22222222-3333-4444-5555-666666666666@host.example:443?type=ws&host=h.com&path=/p&security=tls&sni=s.com"

func TestParseVLESSURIReality(t *testing.T) {
	p, err := parseVLESSURI(realityURI)
	if err != nil {
		t.Fatalf("parseVLESSURI: %v", err)
	}

	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"UUID", p.UUID, "11111111-2222-3333-4444-555555555555"},
		{"Address", p.Address, "1.2.3.4"},
		{"Security", p.Security, "reality"},
		{"Network", p.Network, "tcp"},
		{"SNI", p.SNI, "example.com"},
		{"Fingerprint", p.Fingerprint, "chrome"},
		{"PublicKey", p.PublicKey, "KEY"},
		{"ShortID", p.ShortID, "ab"},
		{"Flow", p.Flow, "xtls-rprx-vision"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
	if p.Port != 443 {
		t.Errorf("Port = %d, want 443", p.Port)
	}
}

func TestBuildOutboundReality(t *testing.T) {
	p, err := parseVLESSURI(realityURI)
	if err != nil {
		t.Fatalf("parseVLESSURI: %v", err)
	}

	ob := buildOutboundFromURI(p, "vless-reality")

	if ob["protocol"] != "vless" {
		t.Errorf("protocol = %v, want vless", ob["protocol"])
	}
	if ob["tag"] != "vless-reality" {
		t.Errorf("tag = %v, want vless-reality", ob["tag"])
	}

	v0 := vnextEntry(t, ob)
	if v0["address"] != "1.2.3.4" {
		t.Errorf("vnext[0].address = %v, want 1.2.3.4", v0["address"])
	}

	ss := ob["streamSettings"].(map[string]interface{})
	if ss["security"] != "reality" {
		t.Errorf("streamSettings.security = %v, want reality", ss["security"])
	}
	rs := ss["realitySettings"].(map[string]interface{})
	if rs["serverName"] != "example.com" {
		t.Errorf("realitySettings.serverName = %v, want example.com", rs["serverName"])
	}
}

func TestBuildOutboundWS(t *testing.T) {
	p, err := parseVLESSURI(wsURI)
	if err != nil {
		t.Fatalf("parseVLESSURI: %v", err)
	}

	ob := buildOutboundFromURI(p, "vless-ws")
	ss := ob["streamSettings"].(map[string]interface{})

	if ss["network"] != "ws" {
		t.Errorf("network = %v, want ws", ss["network"])
	}

	ws := ss["wsSettings"].(map[string]interface{})
	if ws["path"] != "/p" {
		t.Errorf("wsSettings.path = %v, want /p", ws["path"])
	}
	headers := ws["headers"].(map[string]interface{})
	if headers["Host"] != "h.com" {
		t.Errorf("wsSettings.headers.Host = %v, want h.com", headers["Host"])
	}

	ts := ss["tlsSettings"].(map[string]interface{})
	if ts["serverName"] != "s.com" {
		t.Errorf("tlsSettings.serverName = %v, want s.com", ts["serverName"])
	}
}

func TestUpdateOutbound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "04_outbounds.json")
	initial := `{"outbounds":[{"protocol":"vless","tag":"vless-reality"},{"protocol":"freedom","tag":"direct"}]}`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := UpdateOutbound(path, &models.Server{Protocol: "vless", RawURI: realityURI}); err != nil {
		t.Fatalf("UpdateOutbound: %v", err)
	}

	cfg, err := ReadOutboundsConfig(path)
	if err != nil {
		t.Fatalf("ReadOutboundsConfig: %v", err)
	}

	outbounds := cfg["outbounds"].([]interface{})
	ob0 := outbounds[0].(map[string]interface{})
	if ob0["tag"] != "vless-reality" {
		t.Errorf("outbounds[0].tag = %v, want vless-reality", ob0["tag"])
	}
	v0 := vnextEntry(t, ob0)
	if v0["address"] != "1.2.3.4" {
		t.Errorf("outbounds[0] address = %v, want 1.2.3.4", v0["address"])
	}
	if _, ok := cfg["routing"]; ok {
		t.Error("в конфиге не должно быть ключа routing")
	}
}

func TestUpdateOutboundGuards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "04_outbounds.json")
	initial := `{"outbounds":[{"protocol":"vless","tag":"vless-reality"},{"protocol":"freedom","tag":"direct"}]}`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := UpdateOutbound(path, &models.Server{Protocol: "vmess", RawURI: "vmess://abc"}); err == nil {
		t.Error("ожидалась ошибка для протокола vmess")
	}
	if err := UpdateOutbound(path, &models.Server{Protocol: "vless", RawURI: ""}); err == nil {
		t.Error("ожидалась ошибка при пустом RawURI")
	}
}

func TestOutboundsConfigRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rt.json")
	cfg := map[string]interface{}{
		"log": map[string]interface{}{"loglevel": "warning"},
		"outbounds": []interface{}{
			map[string]interface{}{"protocol": "freedom", "tag": "direct"},
		},
	}

	if err := WriteOutboundsConfig(path, cfg); err != nil {
		t.Fatalf("WriteOutboundsConfig: %v", err)
	}

	got, err := ReadOutboundsConfig(path)
	if err != nil {
		t.Fatalf("ReadOutboundsConfig: %v", err)
	}

	logMap := got["log"].(map[string]interface{})
	if logMap["loglevel"] != "warning" {
		t.Errorf("log.loglevel = %v, want warning", logMap["loglevel"])
	}
	outbounds := got["outbounds"].([]interface{})
	if len(outbounds) != 1 {
		t.Fatalf("outbounds len = %d, want 1", len(outbounds))
	}
	ob0 := outbounds[0].(map[string]interface{})
	if ob0["tag"] != "direct" {
		t.Errorf("outbounds[0].tag = %v, want direct", ob0["tag"])
	}
}

func vnextEntry(t *testing.T, outbound map[string]interface{}) map[string]interface{} {
	t.Helper()
	settings := outbound["settings"].(map[string]interface{})
	vnext := settings["vnext"].([]interface{})
	return vnext[0].(map[string]interface{})
}
