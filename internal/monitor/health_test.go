package monitor

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func serverWith(t *testing.T, status int, headers map[string]string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

// A 403 from a CDN is the signature of a blocked exit IP — the very thing a
// plain connectivity check misses.
func TestProbeURLDetectsBlocks(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		headers     map[string]string
		wantOK      bool
		wantBlocked bool
	}{
		{"ok", 200, nil, true, false},
		{"redirect", 302, nil, true, false},
		{"not found is still a working exit", 404, nil, true, false},
		{"cloudfront 403", 403, map[string]string{"Via": "1.1 cloudfront.net"}, false, true},
		{"plain 403", 403, nil, false, true},
		{"legal block", 451, nil, false, true},
		{"cdn 503", 503, map[string]string{"Server": "cloudflare"}, false, true},
		{"origin 500 is not a block", 500, nil, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := probeURL(serverWith(t, c.status, c.headers), 5*time.Second)

			if got.OK != c.wantOK {
				t.Errorf("OK = %v, want %v", got.OK, c.wantOK)
			}
			if got.Blocked != c.wantBlocked {
				t.Errorf("Blocked = %v, want %v", got.Blocked, c.wantBlocked)
			}
		})
	}
}

func TestProbeURLUnreachable(t *testing.T) {
	got := probeURL("http://127.0.0.1:1", 500*time.Millisecond)

	if got.OK || got.Err == nil {
		t.Errorf("got %+v, want a failure", got)
	}
}

// One service per round: these are real public endpoints, and probing all of
// them every tick would collect rate limits for nothing.
func TestHealthCheckerRotatesOneServicePerRound(t *testing.T) {
	a := serverWith(t, 200, nil)
	b := serverWith(t, 200, nil)
	c := serverWith(t, 200, nil)

	checker := NewHealthChecker([]string{a, b, c}, 2)

	seen := map[string]int{}
	for range 6 {
		seen[checker.Probe(2*time.Second).URL]++
	}

	for _, url := range []string{a, b, c} {
		if seen[url] != 2 {
			t.Errorf("%s probed %d times over 6 rounds, want 2", url, seen[url])
		}
	}
}

// A single failing service proves nothing — sites go down on their own. Two
// different ones failing repeatedly is what points at the exit.
func TestExitLooksBlockedNeedsTwoServices(t *testing.T) {
	good := serverWith(t, 200, nil)
	bad1 := serverWith(t, 403, nil)
	bad2 := serverWith(t, 403, nil)

	checker := NewHealthChecker([]string{bad1, good}, 2)

	// bad1 fails twice, good keeps passing: one service is not enough
	for range 4 {
		checker.Probe(2 * time.Second)
	}
	if checker.ExitLooksBlocked() {
		t.Error("one failing service must not condemn the exit")
	}

	checker = NewHealthChecker([]string{bad1, bad2}, 2)
	for range 4 {
		checker.Probe(2 * time.Second)
	}
	if !checker.ExitLooksBlocked() {
		t.Error("two services failing repeatedly should condemn the exit")
	}
}

// A service that recovers must stop counting against the exit.
func TestHealthCheckerForgetsRecoveredService(t *testing.T) {
	flaky := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flaky++
		if flaky <= 2 {
			w.WriteHeader(403)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bad := serverWith(t, 403, nil)
	checker := NewHealthChecker([]string{srv.URL, bad}, 2)

	for range 4 {
		checker.Probe(2 * time.Second)
	}
	if !checker.ExitLooksBlocked() {
		t.Fatal("both services failing should condemn the exit")
	}

	// The flaky one recovers
	for range 2 {
		checker.Probe(2 * time.Second)
	}
	if checker.ExitLooksBlocked() {
		t.Error("a recovered service must stop counting")
	}
}

func TestHealthCheckerResetClearsState(t *testing.T) {
	bad1 := serverWith(t, 403, nil)
	bad2 := serverWith(t, 403, nil)

	checker := NewHealthChecker([]string{bad1, bad2}, 2)
	for range 4 {
		checker.Probe(2 * time.Second)
	}
	if !checker.ExitLooksBlocked() {
		t.Fatal("setup: exit should look blocked")
	}

	checker.Reset()

	if checker.ExitLooksBlocked() {
		t.Error("Reset must clear the verdict — the exit node just changed")
	}
	if got := checker.Failing(); len(got) != 0 {
		t.Errorf("Failing = %v, want empty", got)
	}
}

func TestNewHealthCheckerDefaults(t *testing.T) {
	checker := NewHealthChecker(nil, 0)

	if len(checker.urls) == 0 {
		t.Error("no default services configured")
	}
	if checker.threshold < 1 {
		t.Errorf("threshold = %d, want at least 1", checker.threshold)
	}
}

// HEAD is what broke the first version: SoundCloud never answers it and the
// probe timed out on a perfectly healthy exit.
func TestProbeUsesGET(t *testing.T) {
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	probeURL(srv.URL, 5*time.Second)

	if method != http.MethodGet {
		t.Errorf("method = %q, want GET", method)
	}
}

// A timeout says far less than a 403 does, so a single slow answer must not
// count against the exit.
func TestProbeRetriesNetworkErrorOnce(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			// Hijack and drop the connection to force a network error
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	got := probeURL(srv.URL, 5*time.Second)

	if attempts != 2 {
		t.Errorf("attempts = %d, want 2 — a network error should be retried", attempts)
	}
	if !got.OK {
		t.Errorf("verdict = %+v, want the retry to succeed", got)
	}
}

// A 403 is decisive; retrying it would only double the request rate.
func TestProbeDoesNotRetryBlocks(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(403)
	}))
	defer srv.Close()

	if got := probeURL(srv.URL, 5*time.Second); !got.Blocked {
		t.Errorf("verdict = %+v, want blocked", got)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 — a block needs no second opinion", attempts)
	}
}
