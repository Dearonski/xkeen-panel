package xkeen

import (
	"os"
	"path/filepath"
	"testing"

	"xkeen-panel/internal/models"
)

// liveOutbounds is the real 04_outbounds.json from the router: flat settings,
// "network": "raw", single proxy node.
func liveOutbounds(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "outbounds_flat.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	path := filepath.Join(t.TempDir(), "04_outbounds.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	return path
}

func TestDetectOutboundFormat(t *testing.T) {
	flat := map[string]interface{}{
		"settings": map[string]interface{}{"address": "a", "id": "u"},
	}
	vnext := map[string]interface{}{
		"settings": map[string]interface{}{"vnext": []interface{}{}},
	}

	if got := detectOutboundFormat(flat); got != formatFlat {
		t.Errorf("flat settings detected as %v, want formatFlat", got)
	}
	if got := detectOutboundFormat(vnext); got != formatVNext {
		t.Errorf("vnext settings detected as %v, want formatVNext", got)
	}
	if got := detectOutboundFormat(nil); got != formatFlat {
		t.Errorf("missing outbound detected as %v, want formatFlat", got)
	}
}

func TestReadProxyEndpointBothFormats(t *testing.T) {
	flat := map[string]interface{}{
		"settings": map[string]interface{}{
			"address": "se3.example.com",
			"port":    float64(443),
			"id":      "uuid-flat",
		},
	}
	vnext := map[string]interface{}{
		"settings": map[string]interface{}{
			"vnext": []interface{}{map[string]interface{}{
				"address": "se3.example.com",
				"port":    float64(443),
				"users":   []interface{}{map[string]interface{}{"id": "uuid-vnext"}},
			}},
		},
	}

	for name, tc := range map[string]struct {
		ob   map[string]interface{}
		uuid string
	}{
		"flat":  {flat, "uuid-flat"},
		"vnext": {vnext, "uuid-vnext"},
	} {
		t.Run(name, func(t *testing.T) {
			address, port, uuid, ok := readProxyEndpoint(tc.ob)
			if !ok {
				t.Fatal("endpoint not found")
			}
			if address != "se3.example.com" || port != 443 || uuid != tc.uuid {
				t.Errorf("got (%q, %d, %q), want (se3.example.com, 443, %q)", address, port, uuid, tc.uuid)
			}
		})
	}
}

// The live config is flat; writing a server must not convert it to vnext.
func TestUpdateOutboundKeepsFlatFormat(t *testing.T) {
	path := liveOutbounds(t)

	if err := UpdateOutbound(path, &models.Server{Protocol: "vless", RawURI: realityURI}); err != nil {
		t.Fatalf("UpdateOutbound: %v", err)
	}

	cfg, err := ReadOutboundsConfig(path)
	if err != nil {
		t.Fatalf("ReadOutboundsConfig: %v", err)
	}
	_, ob := findProxyOutbound(cfg["outbounds"].([]interface{}))

	if got := detectOutboundFormat(ob); got != formatFlat {
		t.Errorf("format = %v, want formatFlat — the owner's shape must survive", got)
	}
	address, _, uuid, _ := readProxyEndpoint(ob)
	if address != "1.2.3.4" {
		t.Errorf("address = %q, want 1.2.3.4", address)
	}
	if uuid != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("uuid = %q, want the URI uuid", uuid)
	}
}

// "raw" is the current spelling of the "tcp" transport; a URI saying type=tcp
// must not rewrite it.
func TestUpdateOutboundKeepsNetworkSpelling(t *testing.T) {
	path := liveOutbounds(t)

	if err := UpdateOutbound(path, &models.Server{Protocol: "vless", RawURI: realityURI}); err != nil {
		t.Fatalf("UpdateOutbound: %v", err)
	}

	cfg, _ := ReadOutboundsConfig(path)
	_, ob := findProxyOutbound(cfg["outbounds"].([]interface{}))
	ss := ob["streamSettings"].(map[string]interface{})

	if ss["network"] != "raw" {
		t.Errorf("network = %v, want raw", ss["network"])
	}
}

// XKeen validates streamSettings.sockopt.mark on every real outbound: strict PBR
// refuses to start without it. Regenerating an outbound must not drop it.
func TestUpdateOutboundPreservesSockoptMark(t *testing.T) {
	path := filepath.Join(t.TempDir(), "04_outbounds.json")
	initial := `{"outbounds":[
		{"tag":"vless-reality","protocol":"vless",
		 "settings":{"address":"old.example.com","port":443,"id":"old-uuid"},
		 "streamSettings":{"network":"raw","sockopt":{"mark":"0xffffaaa","tcpFastOpen":true}},
		 "mux":{"enabled":false}},
		{"protocol":"freedom","tag":"direct"}]}`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := UpdateOutbound(path, &models.Server{Protocol: "vless", RawURI: realityURI}); err != nil {
		t.Fatalf("UpdateOutbound: %v", err)
	}

	cfg, _ := ReadOutboundsConfig(path)
	_, ob := findProxyOutbound(cfg["outbounds"].([]interface{}))
	ss := ob["streamSettings"].(map[string]interface{})

	sockopt, ok := ss["sockopt"].(map[string]interface{})
	if !ok {
		t.Fatal("sockopt dropped — XKeen would refuse to start under strict PBR")
	}
	if sockopt["mark"] != "0xffffaaa" {
		t.Errorf("sockopt.mark = %v, want 0xffffaaa", sockopt["mark"])
	}
	if sockopt["tcpFastOpen"] != true {
		t.Errorf("sockopt.tcpFastOpen = %v, want true", sockopt["tcpFastOpen"])
	}
	if _, ok := ob["mux"]; !ok {
		t.Error("unknown top-level key mux was dropped")
	}
	if ss["security"] != "reality" {
		t.Errorf("security = %v, want reality — generated fields must still win", ss["security"])
	}
}

// A balancer pool must not be silently mangled by single-node replacement.
func TestUpdateOutboundRefusesPool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "04_outbounds.json")
	initial := `{"outbounds":[
		{"tag":"sub-1","protocol":"vless","settings":{"address":"a","port":443,"id":"u1"}},
		{"tag":"sub-2","protocol":"vless","settings":{"address":"b","port":443,"id":"u2"}},
		{"protocol":"freedom","tag":"direct"}]}`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := UpdateOutbound(path, &models.Server{Protocol: "vless", RawURI: realityURI})
	if err == nil {
		t.Fatal("expected an error for a balancer pool")
	}

	cfg, _ := ReadOutboundsConfig(path)
	if n := countProxyOutbounds(cfg["outbounds"].([]interface{})); n != 2 {
		t.Errorf("pool size = %d, want 2 — the pool must be left untouched", n)
	}
}

// Comments are legal in Xray configs; the panel must read through them.
func TestUpdateOutboundReadsCommentedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "04_outbounds.json")
	initial := "// generated by hand\n{\"outbounds\":[{\"tag\":\"vless-reality\",\"protocol\":\"vless\"}]}"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := UpdateOutbound(path, &models.Server{Protocol: "vless", RawURI: realityURI}); err != nil {
		t.Fatalf("UpdateOutbound: %v", err)
	}
}

func TestWriteOutboundsConfigBackupAndRestore(t *testing.T) {
	path := liveOutbounds(t)

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if err := UpdateOutbound(path, &models.Server{Protocol: "vless", RawURI: realityURI}); err != nil {
		t.Fatalf("UpdateOutbound: %v", err)
	}

	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(backup) != string(before) {
		t.Error("backup does not match the pre-write content")
	}

	if err := RestoreBackup(path); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	restored, _ := os.ReadFile(path)
	if string(restored) != string(before) {
		t.Error("restore did not bring the original config back")
	}
}
