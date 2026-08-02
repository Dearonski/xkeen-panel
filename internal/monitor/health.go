package monitor

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultHealthURLs are the services worth watching: they are the ones that
// break first when an exit IP has a bad reputation, while plain connectivity
// checks keep passing.
//
// A CDN that fronts many sites (CloudFront, Cloudflare) answers 403 to blocked
// IPs, and Telegram simply stops responding — neither shows up in a
// generate_204 probe.
//
// The endpoints are deliberately small and quick: a probe travels through the
// VPN, and a heavy page turns a healthy exit into a timeout.
var DefaultHealthURLs = []string{
	"https://www.cloudflare.com/cdn-cgi/trace", // Cloudflare edge, a few lines of text
	"https://web.telegram.org/",                // Telegram
	"https://www.youtube.com/generate_204",     // Google, empty 204
	"https://soundcloud.com/",                  // CloudFront
}

// healthVerdict is the outcome of one probe.
type healthVerdict struct {
	URL     string
	OK      bool
	Blocked bool // answered, but with a block rather than content
	Status  int
	Err     error
}

// HealthChecker probes the configured services once per health round.
//
// A round is deliberately rare — one every few watchdog ticks — rather than
// small: checking a single service per round meant each one was sampled only
// every 40 minutes, so two consecutive failures took over an hour to establish
// and an outage went unnoticed. A handful of small requests every ten minutes
// is not a rate-limit risk.
type HealthChecker struct {
	urls      []string
	threshold int
	quorum    int

	mu       sync.Mutex
	failures map[string]int
}

func NewHealthChecker(urls []string, threshold, quorum int) *HealthChecker {
	if len(urls) == 0 {
		urls = DefaultHealthURLs
	}
	if threshold < 1 {
		threshold = 2
	}
	if quorum < 1 {
		quorum = 2
	}

	return &HealthChecker{
		urls:      urls,
		threshold: threshold,
		quorum:    quorum,
		failures:  make(map[string]int),
	}
}

// Probe checks every service and returns the verdicts.
func (h *HealthChecker) Probe(timeout time.Duration) []healthVerdict {
	verdicts := make([]healthVerdict, len(h.urls))

	var wg sync.WaitGroup
	for i, url := range h.urls {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			verdicts[i] = probeURL(url, timeout)
		}(i, url)
	}
	wg.Wait()

	h.mu.Lock()
	defer h.mu.Unlock()

	// A leaky bucket rather than a reset: one lucky success used to wipe the
	// evidence of an outage that was still going on.
	for _, verdict := range verdicts {
		if verdict.OK {
			if h.failures[verdict.URL] > 0 {
				h.failures[verdict.URL]--
			}
			continue
		}
		h.failures[verdict.URL]++
	}

	return verdicts
}

// ExitLooksBlocked reports whether the current exit node should be replaced.
//
// A single failing service proves nothing — sites go down, and some answer 403
// to a datacentre IP regardless of which one it is. Two different services
// failing repeatedly is what points at the exit rather than at the service.
func (h *HealthChecker) ExitLooksBlocked() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	bad := 0
	for _, count := range h.failures {
		if count >= h.threshold {
			bad++
		}
	}

	return bad >= h.quorum
}

// Reset clears the counters after the exit node changed.
func (h *HealthChecker) Reset() {
	h.mu.Lock()
	h.failures = make(map[string]int)
	h.mu.Unlock()
}

// Failing lists the services currently considered failing.
func (h *HealthChecker) Failing() []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	var failing []string
	for url, count := range h.failures {
		if count >= h.threshold {
			failing = append(failing, url)
		}
	}

	return failing
}

// probeURL fetches a URL and decides whether the answer looks like a block.
//
// A network error is retried once: a timeout says far less than a 403 does, and
// one slow response should not count against the exit node.
func probeURL(url string, timeout time.Duration) healthVerdict {
	verdict := probeURLOnce(url, timeout)
	if verdict.Err != nil {
		verdict = probeURLOnce(url, timeout)
	}
	return verdict
}

func probeURLOnce(url string, timeout time.Duration) healthVerdict {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
		// Redirects are normal here; the status of the first hop is enough
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// GET rather than HEAD: plenty of sites answer HEAD slowly or not at all,
	// and SoundCloud simply hangs on it. The body is drained and dropped.
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return healthVerdict{URL: url, Err: err}
	}
	// A default Go user agent is itself a reason for some CDNs to answer 403
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; xkeen-panel health check)")
	req.Header.Set("Accept", "*/*")

	resp, err := client.Do(req)
	if err != nil {
		return healthVerdict{URL: url, Err: err}
	}
	defer func() {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
	}()

	verdict := healthVerdict{URL: url, Status: resp.StatusCode}

	switch {
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusUnavailableForLegalReasons:
		verdict.Blocked = true
	case resp.StatusCode == http.StatusServiceUnavailable && isCDN(resp):
		verdict.Blocked = true
	case resp.StatusCode < 500:
		verdict.OK = true
	}

	return verdict
}

// isCDN reports whether a response came from a CDN edge rather than the origin.
func isCDN(resp *http.Response) bool {
	for _, header := range []string{"X-Cache", "Via", "Server", "Cf-Ray"} {
		if value := resp.Header.Get(header); value != "" {
			lowered := strings.ToLower(value)
			if strings.Contains(lowered, "cloudfront") || strings.Contains(lowered, "cloudflare") {
				return true
			}
		}
	}
	return false
}

func (v healthVerdict) String() string {
	switch {
	case v.OK:
		return fmt.Sprintf("%s ok (%d)", v.URL, v.Status)
	case v.Blocked:
		return fmt.Sprintf("%s заблокирован (%d)", v.URL, v.Status)
	case v.Err != nil:
		return fmt.Sprintf("%s недоступен (%v)", v.URL, v.Err)
	default:
		return fmt.Sprintf("%s ошибка (%d)", v.URL, v.Status)
	}
}
