package xkeen

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type ruMatcher struct{}

func (ruMatcher) Match(ip net.IP) (string, bool) {
	if ip.Equal(net.ParseIP("89.19.223.190")) {
		return "RU", true
	}
	return "", false
}

func TestParseEcho(t *testing.T) {
	cases := []struct {
		body, want string
	}{
		{"fl=123\nh=1.1.1.1\nip=194.9.118.50\nts=1\nloc=LV\n", "194.9.118.50"},
		{"89.19.223.190\n", "89.19.223.190"},
		{"  2001:db8::1 ", "2001:db8::1"},
	}
	for _, c := range cases {
		if got, err := parseEcho(c.body); err != nil || got != c.want {
			t.Errorf("parseEcho(%q) = %q, %v; want %q", c.body, got, err, c.want)
		}
	}

	if _, err := parseEcho("<html>blocked</html>"); err == nil {
		t.Error("a page without an IP must be an error")
	}
}

func TestEchoIPRejectsErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/denied" {
			http.Error(w, "203.0.113.7", http.StatusForbidden)
			return
		}
		w.Write([]byte("203.0.113.7\n"))
	}))
	defer srv.Close()

	if ip, err := echoIP(nil, srv.URL+"/ip", time.Second); err != nil || ip != "203.0.113.7" {
		t.Errorf("echoIP = %q, %v", ip, err)
	}
	if _, err := echoIP(nil, srv.URL+"/denied", time.Second); err == nil {
		t.Error("an error page must not count as an exit")
	}
}

// The first exit can be clean while a cloud-bound one leaves from Russia
func TestAvoidedFindsSecondExit(t *testing.T) {
	exits := NodeExits{Exits: []NodeExit{
		{Via: "Cloudflare", IP: "194.9.118.50"},
		{Via: "Google Cloud", IP: "89.19.223.190"},
	}}

	got, bad := exits.Avoided(ruMatcher{})
	if !bad || got.Via != "Google Cloud" || got.Country != "RU" || got.IP != "89.19.223.190" {
		t.Errorf("Avoided = %+v, %v", got, bad)
	}

	if _, bad := exits.Avoided(nil); bad {
		t.Error("without a geo database nothing is avoided")
	}
}

func TestExitStoreStaleness(t *testing.T) {
	a, b, c := "a.example:443:u", "b.example:443:u", "c.example:443:u"
	both := map[string]bool{a: true, b: true}
	dir := t.TempDir()
	store := NewExitStore(dir)

	if got := store.Stale(both); len(got) != 2 {
		t.Fatalf("Stale = %v, want both nodes of an empty store", got)
	}

	now := time.Now()
	clean := NodeExits{Exits: []NodeExit{{Via: "Cloudflare", IP: "203.0.113.1"}}, CheckedAt: now}
	if err := store.Update(both, map[string]NodeExits{a: clean, b: clean}); err != nil {
		t.Fatal(err)
	}
	if got := store.Stale(both); len(got) != 0 {
		t.Fatalf("Stale = %v after a fresh scan", got)
	}

	// b is down: it keeps its last exits and is not retried at once
	if err := store.Update(both, map[string]NodeExits{b: {CheckedAt: now}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(b); len(got.Exits) != 1 || got.FailedAt.IsZero() {
		t.Errorf("down node = %+v, want its old exits and a failure mark", got)
	}
	if got := store.Stale(both); len(got) != 0 {
		t.Errorf("Stale = %v, a node that just failed must wait before a rescan", got)
	}

	if got := store.Stale(map[string]bool{a: true, c: true}); len(got) != 1 || got[0] != c {
		t.Errorf("Stale = %v, want only the new node", got)
	}

	if err := store.Update(map[string]bool{a: true}, nil); err != nil {
		t.Fatal(err)
	}
	reloaded := NewExitStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Get(b); ok {
		t.Error("a node that left the subscription must be forgotten")
	}
	if _, ok := reloaded.Get(a); !ok {
		t.Error("results must survive a restart")
	}
}
