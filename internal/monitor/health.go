package monitor

import (
	"fmt"
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
var DefaultHealthURLs = []string{
	"https://soundcloud.com",   // CloudFront
	"https://web.telegram.org", // Telegram
	"https://www.youtube.com",
	"https://chatgpt.com", // usually the first to block datacentre ranges
}

// healthVerdict is the outcome of one probe.
type healthVerdict struct {
	URL     string
	OK      bool
	Blocked bool // answered, but with a block rather than content
	Status  int
	Err     error
}

// HealthChecker probes one service per round, cycling through the list.
//
// One request per round on purpose: these are real public services, and probing
// all of them on every watchdog tick would be both pointless and a good way to
// collect rate limits.
type HealthChecker struct {
	urls      []string
	threshold int

	mu       sync.Mutex
	next     int
	failures map[string]int
}

func NewHealthChecker(urls []string, threshold int) *HealthChecker {
	if len(urls) == 0 {
		urls = DefaultHealthURLs
	}
	if threshold < 1 {
		threshold = 2
	}

	return &HealthChecker{
		urls:      urls,
		threshold: threshold,
		failures:  make(map[string]int),
	}
}

// Probe checks the next service in the rotation.
func (h *HealthChecker) Probe(timeout time.Duration) healthVerdict {
	h.mu.Lock()
	url := h.urls[h.next%len(h.urls)]
	h.next++
	h.mu.Unlock()

	verdict := probeURL(url, timeout)

	h.mu.Lock()
	defer h.mu.Unlock()

	if verdict.OK {
		delete(h.failures, url)
	} else {
		h.failures[url]++
	}

	return verdict
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

	return bad >= 2
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
func probeURL(url string, timeout time.Duration) healthVerdict {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
		// Redirects are normal here; the status of the first hop is enough
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return healthVerdict{URL: url, Err: err}
	}
	// A default Go user agent is itself a reason for some CDNs to answer 403
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; xkeen-panel health check)")

	resp, err := client.Do(req)
	if err != nil {
		return healthVerdict{URL: url, Err: err}
	}
	defer resp.Body.Close()

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
