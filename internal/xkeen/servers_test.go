package xkeen

import (
	"encoding/base64"
	"strings"
	"testing"
)

func vmessURI() string {
	j := `{"v":"2","ps":"Frankfurt DE","add":"5.6.7.8","port":"443","id":"uuid-x","net":"ws","tls":"tls"}`
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(j))
}

func sampleSub() string {
	return strings.Join([]string{
		"vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=tcp&security=reality&sni=example.com&fp=chrome#NL-Amsterdam",
		vmessURI(),
		"trojan://secret@9.10.11.12:443?security=tls#Istanbul-TR",
		"ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ=@13.14.15.16:8388#Finland",
	}, "\n")
}

func TestParseSubscriptionPlaintext(t *testing.T) {
	servers, err := ParseSubscription(sampleSub())
	if err != nil {
		t.Fatalf("ParseSubscription: %v", err)
	}
	if len(servers) != 4 {
		t.Fatalf("серверов = %d, want 4", len(servers))
	}

	want := []struct {
		proto, addr, country string
		port                 int
	}{
		{"vless", "1.2.3.4", "NL", 443},
		{"vmess", "5.6.7.8", "DE", 443},
		{"trojan", "9.10.11.12", "TR", 443},
		{"shadowsocks", "13.14.15.16", "FI", 8388},
	}
	for i, w := range want {
		s := servers[i]
		if s.ID != i {
			t.Errorf("server[%d].ID = %d, want %d", i, s.ID, i)
		}
		if s.Protocol != w.proto {
			t.Errorf("server[%d].Protocol = %q, want %q", i, s.Protocol, w.proto)
		}
		if s.Address != w.addr {
			t.Errorf("server[%d].Address = %q, want %q", i, s.Address, w.addr)
		}
		if s.Port != w.port {
			t.Errorf("server[%d].Port = %d, want %d", i, s.Port, w.port)
		}
		if s.Country != w.country {
			t.Errorf("server[%d].Country = %q, want %q", i, s.Country, w.country)
		}
		if s.RawURI == "" {
			t.Errorf("server[%d].RawURI пуст", i)
		}
	}
}

func TestParseSubscriptionBase64(t *testing.T) {
	wrapped := base64.StdEncoding.EncodeToString([]byte(sampleSub()))
	servers, err := ParseSubscription(wrapped)
	if err != nil {
		t.Fatalf("ParseSubscription(base64): %v", err)
	}
	if len(servers) != 4 {
		t.Fatalf("серверов = %d, want 4", len(servers))
	}
}

func TestParseSubscriptionEmpty(t *testing.T) {
	if _, err := ParseSubscription("no valid links here"); err == nil {
		t.Error("ожидалась ошибка при отсутствии серверов")
	}
}
