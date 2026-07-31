package geoip

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestMatchIPv6(t *testing.T) {
	v6 := net.ParseIP("2001:db8::").To16()
	path := writeDat(t, geoEntry("RU", cidr(v6, 32)))

	m, err := Load(path, []string{"RU"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cc, ok := m.Match(net.ParseIP("2001:db8::1")); !ok || cc != "RU" {
		t.Errorf("Match(in-range v6) = (%q, %v), want (RU, true)", cc, ok)
	}
	if cc, ok := m.Match(net.ParseIP("2001:dead::1")); ok || cc != "" {
		t.Errorf("Match(out-of-range v6) = (%q, %v), want (\"\", false)", cc, ok)
	}
}

func TestMultipleAvoidCountries(t *testing.T) {
	path := writeDat(t,
		geoEntry("RU", cidr([]byte{1, 2, 3, 0}, 24)),
		geoEntry("BY", cidr([]byte{178, 124, 0, 0}, 16)),
		geoEntry("NL", cidr([]byte{5, 6, 7, 0}, 24)),
	)

	m, err := Load(path, []string{"RU", "BY"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cc, ok := m.Match(net.ParseIP("1.2.3.7")); !ok || cc != "RU" {
		t.Errorf("Match(RU ip) = (%q, %v), want (RU, true)", cc, ok)
	}
	if cc, ok := m.Match(net.ParseIP("178.124.9.9")); !ok || cc != "BY" {
		t.Errorf("Match(BY ip) = (%q, %v), want (BY, true)", cc, ok)
	}
	// NL is in the file but not in the avoid list, so it must not match.
	if cc, ok := m.Match(net.ParseIP("5.6.7.7")); ok || cc != "" {
		t.Errorf("Match(NL ip) = (%q, %v), want (\"\", false)", cc, ok)
	}
}

func TestFindDatVariants(t *testing.T) {
	dir := t.TempDir()
	v2fly := filepath.Join(dir, "geoip_v2fly.dat")
	if err := os.WriteFile(v2fly, []byte{0}, 0644); err != nil {
		t.Fatal(err)
	}

	if got := FindDat(v2fly); got != v2fly {
		t.Errorf("FindDat(existing) = %q, want %q", got, v2fly)
	}

	// The configured file is missing but geoip_v2fly.dat sits next to it.
	if got := FindDat(filepath.Join(dir, "nope.dat")); got != v2fly {
		t.Errorf("FindDat(sibling fallback) = %q, want %q", got, v2fly)
	}

	// Must not panic; returns a string, usually "" when no standard path exists.
	_ = FindDat("")
}

func TestLoadMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.dat")
	if err := os.WriteFile(path, []byte{0xff, 0xff, 0xff}, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, []string{"RU"}); err == nil {
		t.Error("Load(malformed) = nil error, want error")
	}

	if _, err := Load(filepath.Join(t.TempDir(), "missing.dat"), []string{"RU"}); err == nil {
		t.Error("Load(nonexistent) = nil error, want error")
	}
}
