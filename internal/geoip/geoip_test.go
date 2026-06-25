package geoip

import (
	"bytes"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func uvarint(v uint64) []byte {
	buf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(buf, v)
	return buf[:n]
}

func tag(field, wire int) []byte {
	return uvarint(uint64(field<<3 | wire))
}

func lenDelim(field int, payload []byte) []byte {
	var b bytes.Buffer
	b.Write(tag(field, 2))
	b.Write(uvarint(uint64(len(payload))))
	b.Write(payload)
	return b.Bytes()
}

func varintField(field int, v uint64) []byte {
	var b bytes.Buffer
	b.Write(tag(field, 0))
	b.Write(uvarint(v))
	return b.Bytes()
}

func cidr(ip []byte, prefix uint64) []byte {
	var b bytes.Buffer
	b.Write(lenDelim(1, ip))
	b.Write(varintField(2, prefix))
	return b.Bytes()
}

func geoEntry(code string, cidrs ...[]byte) []byte {
	var b bytes.Buffer
	b.Write(lenDelim(1, []byte(code)))
	for _, c := range cidrs {
		b.Write(lenDelim(2, c))
	}
	return b.Bytes()
}

func writeDat(t *testing.T, entries ...[]byte) string {
	t.Helper()
	var b bytes.Buffer
	for _, e := range entries {
		b.Write(lenDelim(1, e))
	}
	path := filepath.Join(t.TempDir(), "geoip.dat")
	if err := os.WriteFile(path, b.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAndMatch(t *testing.T) {
	path := writeDat(t,
		geoEntry("RU", cidr([]byte{1, 2, 3, 0}, 24), cidr([]byte{95, 0, 0, 0}, 8)),
		geoEntry("BY", cidr([]byte{178, 124, 0, 0}, 16)),
		geoEntry("NL", cidr([]byte{5, 6, 7, 0}, 24)),
	)

	m, err := Load(path, []string{"RU", "BY"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := []struct {
		ip       string
		wantCC   string
		wantHave bool
	}{
		{"1.2.3.5", "RU", true},
		{"95.100.200.1", "RU", true},
		{"178.124.5.5", "BY", true},
		{"5.6.7.5", "", false},   // NL не загружали — не избегается
		{"8.8.8.8", "", false},   // вне всех диапазонов
		{"1.2.4.0", "", false},   // соседний /24, вне диапазона
	}
	for _, c := range cases {
		cc, ok := m.Match(net.ParseIP(c.ip))
		if ok != c.wantHave || cc != c.wantCC {
			t.Errorf("Match(%s) = (%q, %v), want (%q, %v)", c.ip, cc, ok, c.wantCC, c.wantHave)
		}
	}
}

func TestInspectIPLiteral(t *testing.T) {
	path := writeDat(t, geoEntry("RU", cidr([]byte{1, 2, 3, 0}, 24)))
	m, err := Load(path, []string{"RU"})
	if err != nil {
		t.Fatal(err)
	}

	if cc, resolved := m.Inspect("1.2.3.9"); !resolved || cc != "RU" {
		t.Errorf("Inspect(RU ip) = (%q, %v), want (RU, true)", cc, resolved)
	}
	if cc, resolved := m.Inspect("8.8.8.8"); !resolved || cc != "" {
		t.Errorf("Inspect(clean ip) = (%q, %v), want (\"\", true)", cc, resolved)
	}
}

func TestNilMatcher(t *testing.T) {
	var m *Matcher
	if cc, ok := m.Match(net.ParseIP("1.2.3.4")); ok || cc != "" {
		t.Errorf("nil Match = (%q, %v), want empty", cc, ok)
	}
	if cc, resolved := m.Inspect("1.2.3.4"); resolved || cc != "" {
		t.Errorf("nil Inspect = (%q, %v), want empty/false", cc, resolved)
	}
}
