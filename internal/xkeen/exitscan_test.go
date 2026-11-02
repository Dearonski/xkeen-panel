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
	a := endpoint{Address: "a.example", Port: 443, UUID: "u"}
	b := endpoint{Address: "b.example", Port: 443, UUID: "u"}
	layout := PoolLayout{"sub-1": a, "sub-2": b}
	dir := t.TempDir()
	store := NewExitStore(dir)

	if !store.Stale(layout) {
		t.Fatal("an empty store must be stale")
	}

	now := time.Now()
	clean := NodeExits{Exits: []NodeExit{{Via: "Cloudflare", IP: "203.0.113.1"}}, CheckedAt: now}
	if err := store.Replace(layout, map[string]NodeExits{a.Key(): clean, b.Key(): clean}); err != nil {
		t.Fatal(err)
	}
	if store.Stale(layout) {
		t.Fatal("a fresh scan of every node must not be stale")
	}

	// b is down: it keeps its last exits and is not retried at once
	if err := store.Replace(layout, map[string]NodeExits{a.Key(): clean, b.Key(): {CheckedAt: now}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(b.Key()); len(got.Exits) != 1 || got.FailedAt.IsZero() {
		t.Errorf("down node = %+v, want its old exits and a failure mark", got)
	}
	if store.Stale(layout) {
		t.Error("a node that just failed must wait before a rescan")
	}

	c := endpoint{Address: "c.example", Port: 443, UUID: "u"}
	if !store.Stale(PoolLayout{"sub-1": a, "sub-3": c}) {
		t.Error("a node new to the pool must make the store stale")
	}

	if err := store.Replace(PoolLayout{"sub-1": a}, map[string]NodeExits{a.Key(): clean}); err != nil {
		t.Fatal(err)
	}
	reloaded := NewExitStore(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Get(b.Key()); ok {
		t.Error("a node that left the pool must be forgotten")
	}
	if _, ok := reloaded.Get(a.Key()); !ok {
		t.Error("results must survive a restart")
	}
}
