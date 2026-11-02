package xkeen

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Providers route traffic for big clouds through a second exit, so a single echo service misses it
var exitProbes = []struct{ Via, URL string }{
	{"Cloudflare", "https://1.1.1.1/cdn-cgi/trace"},
	{"Google Cloud", "https://ifconfig.me/ip"},
	{"AWS", "https://checkip.amazonaws.com/"},
}

// A provider can reroute a node's exits without changing anything a subscription refresh would see
const exitRescanEvery = 6 * time.Hour

const exitRetryAfter = 30 * time.Minute

type NodeExit struct {
	Via string `json:"via"`
	IP  string `json:"ip"`
}

type NodeExits struct {
	Exits     []NodeExit `json:"exits"`
	CheckedAt time.Time  `json:"checked_at"`
	FailedAt  time.Time  `json:"failed_at,omitempty"`
}

type IPMatcher interface {
	Match(ip net.IP) (string, bool)
}

type AvoidedExit struct {
	Via     string `json:"via"`
	IP      string `json:"ip"`
	Country string `json:"country"`
}

func (e NodeExits) Avoided(m IPMatcher) (AvoidedExit, bool) {
	if m == nil {
		return AvoidedExit{}, false
	}
	for _, exit := range e.Exits {
		ip := net.ParseIP(exit.IP)
		if ip == nil {
			continue
		}
		if cc, ok := m.Match(ip); ok {
			return AvoidedExit{Via: exit.Via, IP: exit.IP, Country: cc}, true
		}
	}
	return AvoidedExit{}, false
}

func ScanExits(rt Runtime, nodes []map[string]interface{}, timeout time.Duration) (map[string]NodeExits, error) {
	results := make([]NodeExits, len(nodes))
	err := throughThrowaway(rt, nodes, func(i int, proxy *url.URL) {
		results[i] = probeExits(proxy, timeout)
	})
	if err != nil {
		return nil, err
	}

	scanned := make(map[string]NodeExits, len(nodes))
	for i, node := range nodes {
		if ep, ok := endpointOfOutbound(node); ok {
			scanned[ep.Key()] = results[i]
		}
	}
	return scanned, nil
}

func probeExits(proxy *url.URL, timeout time.Duration) NodeExits {
	result := NodeExits{CheckedAt: time.Now()}
	for _, probe := range exitProbes {
		if ip, err := echoIP(proxy, probe.URL, timeout); err == nil {
			result.Exits = append(result.Exits, NodeExit{Via: probe.Via, IP: ip})
		}
	}
	return result
}

func echoIP(proxy *url.URL, rawURL string, timeout time.Duration) (string, error) {
	transport := &http.Transport{DisableKeepAlives: true}
	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
	}
	client := &http.Client{Timeout: timeout, Transport: transport}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", siteCheckUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s", shortNetError(err))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s ответил %d", rawURL, resp.StatusCode)
	}

	return parseEcho(string(body))
}

func parseEcho(body string) (string, error) {
	if strings.Contains(body, "ip=") {
		if exit := parseTrace(body); exit.IP != "" {
			return exit.IP, nil
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(body)); ip != nil {
		return ip.String(), nil
	}
	return "", fmt.Errorf("в ответе нет IP")
}

func ScanPoolExits(rt Runtime, outboundsPath, selector string, store *ExitStore, timeout time.Duration) (bool, error) {
	layout, err := ReadPoolLayout(outboundsPath, selector)
	if err != nil {
		return false, err
	}
	if !store.Stale(layout) {
		return false, nil
	}

	nodes, err := poolNodesInFile(outboundsPath, selector)
	if err != nil {
		return false, err
	}
	scanned, err := ScanExits(rt, nodes, timeout)
	if err != nil {
		return false, err
	}

	return true, store.Replace(layout, scanned)
}

// ExitStore keeps scan results across panel restarts, so auto-pick knows the bad nodes before the first scan ends.
type ExitStore struct {
	path  string
	mu    sync.RWMutex
	nodes map[string]NodeExits
}

func NewExitStore(dataDir string) *ExitStore {
	return &ExitStore{path: filepath.Join(dataDir, "exits.json"), nodes: map[string]NodeExits{}}
}

func (s *ExitStore) Load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	nodes := map[string]NodeExits{}
	if err := json.Unmarshal(data, &nodes); err != nil {
		return err
	}

	s.mu.Lock()
	s.nodes = nodes
	s.mu.Unlock()
	return nil
}

func (s *ExitStore) Get(node string) (NodeExits, bool) {
	if s == nil {
		return NodeExits{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	exits, ok := s.nodes[node]
	return exits, ok
}

func (s *ExitStore) Stale(layout PoolLayout) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ep := range layout {
		exits, ok := s.nodes[ep.Key()]
		switch {
		case !ok:
			return true
		case time.Since(exits.FailedAt) < exitRetryAfter:
		case len(exits.Exits) == 0 || time.Since(exits.CheckedAt) > exitRescanEvery:
			return true
		}
	}
	return false
}

func (s *ExitStore) Replace(layout PoolLayout, scanned map[string]NodeExits) error {
	s.mu.Lock()
	next := make(map[string]NodeExits, len(layout))
	for _, ep := range layout {
		key := ep.Key()
		exits, ok := scanned[key]
		if !ok {
			exits, ok = s.nodes[key]
		} else if len(exits.Exits) == 0 {
			// A node that is down right now keeps its last known exits
			failed := s.nodes[key]
			failed.FailedAt = exits.CheckedAt
			exits = failed
		}
		if ok {
			next[key] = exits
		}
	}
	s.nodes = next
	data, err := json.MarshalIndent(next, "", "  ")
	s.mu.Unlock()

	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}
