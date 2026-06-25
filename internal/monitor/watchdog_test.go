package monitor

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"xkeen-panel/internal/geoip"
	"xkeen-panel/internal/models"
	"xkeen-panel/internal/xkeen"
)

func newWatchdog(t *testing.T, cfg *models.Config) *Watchdog {
	t.Helper()
	sub := xkeen.NewSubscriptionManager(t.TempDir())
	return NewWatchdog(cfg, sub)
}

func TestIsAvoidedCountry(t *testing.T) {
	w := newWatchdog(t, &models.Config{AutoSwitchAvoidCountries: []string{"RU", "BY"}})

	cases := []struct {
		cc   string
		want bool
	}{
		{"RU", true},
		{"ru", true},
		{"BY", true},
		{"NL", false},
		{"", false},
	}
	for _, c := range cases {
		if got := w.isAvoidedCountry(c.cc); got != c.want {
			t.Errorf("isAvoidedCountry(%q) = %v, want %v", c.cc, got, c.want)
		}
	}
}

func TestBlacklist(t *testing.T) {
	w := newWatchdog(t, &models.Config{BlacklistTTLSec: 300})

	w.blacklistServer("uriA")

	if !w.isBlacklisted("uriA") {
		t.Fatal("uriA должен быть в чёрном списке")
	}
	if w.isBlacklisted("other") {
		t.Fatal("other не должен быть в чёрном списке")
	}

	w.ClearBlacklist("uriA")
	if w.isBlacklisted("uriA") {
		t.Fatal("uriA должен быть очищен из чёрного списка")
	}
}

func TestBlacklistExpiry(t *testing.T) {
	w := newWatchdog(t, &models.Config{BlacklistTTLSec: 300})

	w.blacklist["x"] = time.Now().Add(-time.Second)

	if w.isBlacklisted("x") {
		t.Fatal("просроченная запись не должна считаться активной")
	}

	w.mu.Lock()
	_, exists := w.blacklist["x"]
	w.mu.Unlock()
	if exists {
		t.Fatal("просроченная запись должна быть удалена из map")
	}
}

func TestBlacklistTTLZero(t *testing.T) {
	w := newWatchdog(t, &models.Config{BlacklistTTLSec: 0})

	w.blacklistServer("uriZ")
	if w.isBlacklisted("uriZ") {
		t.Fatal("при TTL=0 blacklistServer должен быть no-op")
	}
}

func TestIsServerAllowedNilGeoIP(t *testing.T) {
	w := newWatchdog(t, &models.Config{AutoSwitchAvoidCountries: []string{"RU", "BY"}})

	cases := []struct {
		name string
		s    models.Server
		want bool
	}{
		{"allowed country", models.Server{Country: "NL"}, true},
		{"avoided country", models.Server{Country: "RU"}, false},
		{"unknown country strict", models.Server{Country: ""}, false},
		{"override wins", models.Server{Country: "RU", CountryOverride: "NL"}, true},
		{"override avoided", models.Server{Country: "NL", CountryOverride: "RU"}, false},
	}
	for _, c := range cases {
		if got := w.isServerAllowed(c.s); got != c.want {
			t.Errorf("%s: isServerAllowed = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsServerAllowedWithGeoIP(t *testing.T) {
	path := buildGeoIP(t)
	matcher, err := geoip.Load(path, []string{"RU"})
	if err != nil {
		t.Fatalf("geoip.Load: %v", err)
	}

	w := newWatchdog(t, &models.Config{AutoSwitchAvoidCountries: []string{"RU", "BY"}})
	w.SetGeoIP(matcher)

	cases := []struct {
		name string
		s    models.Server
		want bool
	}{
		{"ip in RU range", models.Server{Address: "1.2.3.5"}, false},
		{"ip resolved not avoided", models.Server{Address: "8.8.8.8"}, true},
		{"geoip authoritative over name", models.Server{Address: "1.2.3.5", Country: "NL"}, false},
	}
	for _, c := range cases {
		if got := w.isServerAllowed(c.s); got != c.want {
			t.Errorf("%s: isServerAllowed = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRotateLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "watchdog.log")

	var buf bytes.Buffer
	for i := 0; buf.Len() < 2<<20; i++ {
		fmt.Fprintf(&buf, "log line number %08d with padding text to make lines longer zzzzz\n", i)
	}
	original := append([]byte(nil), buf.Bytes()...)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	rotateLog(path, 1<<20, 256<<10)

	rotated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(rotated)) > 256<<10 {
		t.Fatalf("размер после ротации %d > %d", len(rotated), 256<<10)
	}
	if len(rotated) == 0 || len(rotated) >= len(original) {
		t.Fatalf("неожиданный размер после ротации %d (исходный %d)", len(rotated), len(original))
	}
	if !bytes.HasSuffix(original, rotated) {
		t.Fatal("содержимое после ротации должно быть суффиксом исходного")
	}
	if before := original[len(original)-len(rotated)-1]; before != '\n' {
		t.Fatalf("ротация не на границе строки, предыдущий байт = %q", before)
	}

	smallPath := filepath.Join(dir, "small.log")
	small := []byte("just a few bytes\nsecond line\n")
	if err := os.WriteFile(smallPath, small, 0644); err != nil {
		t.Fatal(err)
	}
	rotateLog(smallPath, 1<<20, 256<<10)
	got, err := os.ReadFile(smallPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, small) {
		t.Fatal("маленький файл не должен изменяться")
	}
}

// --- protobuf-энкодер для сборки geoip.dat формата Xray ---

func uvarint(v uint64) []byte {
	b := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(b, v)
	return b[:n]
}

func pbTag(field, wire int) []byte { return uvarint(uint64(field<<3 | wire)) }

func pbLen(field int, p []byte) []byte {
	var b bytes.Buffer
	b.Write(pbTag(field, 2))
	b.Write(uvarint(uint64(len(p))))
	b.Write(p)
	return b.Bytes()
}

func pbVarint(field int, v uint64) []byte {
	var b bytes.Buffer
	b.Write(pbTag(field, 0))
	b.Write(uvarint(v))
	return b.Bytes()
}

func pbCIDR(ip []byte, prefix uint64) []byte {
	var b bytes.Buffer
	b.Write(pbLen(1, ip))
	b.Write(pbVarint(2, prefix))
	return b.Bytes()
}

func pbGeo(code string, cidrs ...[]byte) []byte {
	var b bytes.Buffer
	b.Write(pbLen(1, []byte(code)))
	for _, c := range cidrs {
		b.Write(pbLen(2, c))
	}
	return b.Bytes()
}

func buildGeoIP(t *testing.T) string {
	t.Helper()
	entry := pbGeo("RU", pbCIDR([]byte{1, 2, 3, 0}, 24))
	data := pbLen(1, entry)
	path := filepath.Join(t.TempDir(), "geoip.dat")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
