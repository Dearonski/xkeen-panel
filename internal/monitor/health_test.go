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

// Every service is sampled each round. Round-robin meant a given service was
// checked only every 40 minutes, so establishing two consecutive failures took
// over an hour — far too slow to catch an outage.
func TestHealthCheckerProbesEveryServicePerRound(t *testing.T) {
	a := serverWith(t, 200, nil)
	b := serverWith(t, 200, nil)
	c := serverWith(t, 200, nil)

	checker := NewHealthChecker([]string{a, b, c}, 2, 2)

	got := checker.Probe(2 * time.Second)

	if len(got) != 3 {
		t.Fatalf("verdicts = %d, want one per service", len(got))
	}
	seen := map[string]bool{}
	for _, v := range got {
		seen[v.URL] = true
	}
	for _, url := range []string{a, b, c} {
		if !seen[url] {
			t.Errorf("%s was not probed", url)
		}
	}
}

// A single failing service proves nothing — sites go down on their own. Two
// different ones failing repeatedly is what points at the exit.
func TestExitLooksBlockedNeedsTwoServices(t *testing.T) {
	good := serverWith(t, 200, nil)
	bad1 := serverWith(t, 403, nil)
	bad2 := serverWith(t, 403, nil)

	checker := NewHealthChecker([]string{bad1, good}, 2, 2)

	// bad1 fails twice, good keeps passing: one service is not enough
	for range 2 {
		checker.Probe(2 * time.Second)
	}
	if checker.ExitLooksBlocked() {
		t.Error("one failing service must not condemn the exit")
	}

	checker = NewHealthChecker([]string{bad1, bad2}, 2, 2)
	for range 2 {
		checker.Probe(2 * time.Second)
	}
	if !checker.ExitLooksBlocked() {
		t.Error("two services failing repeatedly should condemn the exit")
	}
}

// A quorum of one lets a single critical service speak for the exit.
func TestExitLooksBlockedHonoursQuorum(t *testing.T) {
	bad := serverWith(t, 403, nil)
	good := serverWith(t, 200, nil)

	checker := NewHealthChecker([]string{bad, good}, 2, 1)
	for range 2 {
		checker.Probe(2 * time.Second)
	}

	if !checker.ExitLooksBlocked() {
		t.Error("with quorum 1 a single failing service should condemn the exit")
	}
}

// A service that recovers stops counting — but it takes as many successes as it
// took failures. One lucky response used to wipe the record of an outage that
// was still going on.
func TestHealthCheckerDrainsFailuresGradually(t *testing.T) {
	flaky := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flaky++
		if flaky <= 3 {
			w.WriteHeader(403)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	bad := serverWith(t, 403, nil)
	checker := NewHealthChecker([]string{srv.URL, bad}, 2, 2)

	for range 3 {
		checker.Probe(2 * time.Second)
	}
	if !checker.ExitLooksBlocked() {
		t.Fatal("both services failing should condemn the exit")
	}

	// One success is not enough to clear three failures
	checker.Probe(2 * time.Second)
	if !checker.ExitLooksBlocked() {
		t.Error("a single success wiped the evidence")
	}

	for range 3 {
		checker.Probe(2 * time.Second)
	}
	if checker.ExitLooksBlocked() {
		t.Error("a sustained recovery must clear the verdict")
	}
}

func TestHealthCheckerResetClearsState(t *testing.T) {
	bad1 := serverWith(t, 403, nil)
	bad2 := serverWith(t, 403, nil)

	checker := NewHealthChecker([]string{bad1, bad2}, 2, 2)
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
	checker := NewHealthChecker(nil, 0, 0)

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
