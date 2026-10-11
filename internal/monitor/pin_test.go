package monitor

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xkeen-panel/internal/models"
	"xkeen-panel/internal/xkeen"
)

const (
	nlURI = "vless://11111111-2222-3333-4444-555555555555@nl.example:443?type=tcp&security=reality&sni=a.com&pbk=K#NL"
	fiURI = "vless://33333333-4444-5555-6666-777777777777@fi.example:443?type=tcp&security=reality&sni=b.com&pbk=K#FI"
)

// routerRoot lays out an XKeen 2.x install whose xray binary only records the
// API calls it receives, and a config with a one-node pool.
func routerRoot(t *testing.T) (root, calls string) {
	t.Helper()
	root = t.TempDir()
	calls = filepath.Join(root, "calls.log")

	write := func(rel, body string, mode os.FileMode) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}

	write("opt/etc/init.d/S05xkeen", "#!/bin/sh\nname_client=\"xray\"\n", 0755)
	write("opt/sbin/xkeen", "#!/bin/sh\nexit 0\n", 0755)
	write("opt/sbin/xray", "#!/bin/sh\necho \"$@\" >> "+calls+"\n", 0755)
	write("opt/etc/xray/configs/04_outbounds.json", `{"outbounds":[
		{"tag":"sub-1","protocol":"vless","settings":{"address":"nl.example","port":443,"id":"11111111-2222-3333-4444-555555555555"},
		 "streamSettings":{"network":"tcp","sockopt":{"mark":255}}},
		{"tag":"direct","protocol":"freedom"}]}`, 0644)
	write("opt/etc/xray/configs/05_routing.json", `{"routing":{
		"balancers":[{"tag":"balancer","selector":["sub-"]}],
		"rules":[{"inboundTag":["redirect"],"balancerTag":"balancer"}]}}`, 0644)

	return root, calls
}

func routerWatchdog(t *testing.T, servers []models.Server) (*Watchdog, *xkeen.PoolStore, string) {
	t.Helper()
	root, calls := routerRoot(t)

	sub := subWith(t, servers, 0)

	cfg := &models.Config{
		OutboundsFile:   filepath.Join(root, "opt/etc/xray/configs/04_outbounds.json"),
		XrayAPIAddr:     "127.0.0.1:10085",
		ProbeTimeoutMs:  500,
		BlacklistTTLSec: 300,
	}
	w := NewWatchdog(cfg, sub, xkeen.NewDetector(root, "", "", "", "", "", ""))
	store := xkeen.NewPoolStore(t.TempDir())
	w.SetPoolStore(store)

	return w, store, calls
}

func readCalls(t *testing.T, path string) string {
	t.Helper()
	data, _ := os.ReadFile(path)
	return string(data)
}

func TestPinServerOutsidePool(t *testing.T) {
	fi := models.Server{Name: "FI", Protocol: "vless", RawURI: fiURI, Country: "FI"}
	w, store, calls := routerWatchdog(t, []models.Server{{Name: "NL", Protocol: "vless", RawURI: nlURI}, fi})

	tag, err := w.PinServer(fi, true, "")
	if err != nil {
		t.Fatalf("PinServer: %v", err)
	}
	if tag != xkeen.ManualTag {
		t.Errorf("tag = %q, want the manual slot", tag)
	}

	log := readCalls(t, calls)
	for _, want := range []string{"api ado", "api bo -s 127.0.0.1:10085 -b balancer manual"} {
		if !strings.Contains(log, want) {
			t.Errorf("xray calls lack %q:\n%s", want, log)
		}
	}

	got := store.Get()
	if got.PinnedTag != xkeen.ManualTag || !got.PinManual || got.PinCountry != "FI" {
		t.Errorf("state = %+v, want a manual pin on FI", got)
	}
	if server, ok := xkeen.ServerForNode([]models.Server{fi}, got.PinnedNode); !ok || server.Name != "FI" {
		t.Errorf("PinnedNode %q does not lead back to FI", got.PinnedNode)
	}
}

func TestPinServerInsidePoolUsesItsTag(t *testing.T) {
	nl := models.Server{Name: "NL", Protocol: "vless", RawURI: nlURI}
	w, store, calls := routerWatchdog(t, []models.Server{nl})

	tag, err := w.PinServer(nl, true, "")
	if err != nil {
		t.Fatalf("PinServer: %v", err)
	}
	if tag != "sub-1" || store.Get().PinnedTag != "sub-1" {
		t.Errorf("tag = %q, want sub-1", tag)
	}
	if log := readCalls(t, calls); strings.Contains(log, "ado") {
		t.Errorf("a pool member needs no new outbound:\n%s", log)
	}
}

func listener(t *testing.T) (string, int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	addr := l.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

func TestSameCountryReplacement(t *testing.T) {
	host, port := listener(t)

	servers := []models.Server{
		{Name: "FI-failed", Protocol: "vless", RawURI: "failed", Address: host, Port: port, Country: "FI"},
		{Name: "FI-dead", Protocol: "vless", RawURI: "dead", Address: "127.0.0.1", Port: 1, Country: "FI"},
		{Name: "FI-live", Protocol: "vless", RawURI: "live", Address: host, Port: port, Country: "FI"},
		{Name: "NL-live", Protocol: "vless", RawURI: "nl", Address: host, Port: port, Country: "NL"},
	}
	w, _, _ := routerWatchdog(t, servers)

	next, err := w.sameCountryReplacement("FI", "failed")
	if err != nil {
		t.Fatalf("sameCountryReplacement: %v", err)
	}
	if next.Name != "FI-live" {
		t.Errorf("picked %s, want FI-live", next.Name)
	}

	if _, err := w.sameCountryReplacement("DE", ""); err == nil {
		t.Error("a country absent from the subscription has no replacement")
	}
}

// A hand-picked country stays picked: the failed server is swapped for another
// one of the same country, and the owner sees why.
func TestRotateExitKeepsManualCountry(t *testing.T) {
	host, port := listener(t)
	uri := func(uuid string) string {
		return fmt.Sprintf("vless://%s@%s:%d?type=tcp&security=reality&sni=b.com&pbk=K", uuid, host, port)
	}
	failed := models.Server{Name: "FI-1", Protocol: "vless", RawURI: fiURI, Address: "fi.example", Port: 443, Country: "FI"}
	backup := models.Server{Name: "FI-2", Protocol: "vless", RawURI: uri("44444444-5555-6666-7777-888888888888"), Address: host, Port: port, Country: "FI"}

	w, store, _ := routerWatchdog(t, []models.Server{{Name: "NL", Protocol: "vless", RawURI: nlURI}, failed, backup})
	if _, err := w.PinServer(failed, true, ""); err != nil {
		t.Fatalf("PinServer: %v", err)
	}

	w.rotateExit(w.detector.Runtime(), w.detector.Topology())

	got := store.Get()
	if !got.PinManual || got.PinCountry != "FI" {
		t.Fatalf("state = %+v, want a manual FI pin", got)
	}
	if server, ok := xkeen.ServerForNode([]models.Server{backup}, got.PinnedNode); !ok || server.Name != "FI-2" {
		t.Errorf("pinned node %q, want FI-2", got.PinnedNode)
	}
	if !strings.Contains(got.PinNote, "FI-1") || !strings.Contains(got.PinNote, "FI-2") {
		t.Errorf("note = %q, want it to name both servers", got.PinNote)
	}
	if !w.isBlacklisted(fiURI) {
		t.Error("the failed server must sit out before it can be picked again")
	}
}
