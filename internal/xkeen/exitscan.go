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
	"sort"
	"strings"
	"sync"
	"time"

	"xkeen-panel/internal/models"
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

// The whole subscription is scanned, not just the pool, so a pool can be built from clean nodes only
func ScanSubscriptionExits(rt Runtime, outboundsPath, selector string, servers []models.Server, sel PoolSelection, store *ExitStore, timeout time.Duration) (int, error) {
	candidates := poolCandidates(servers, sel)
	keep := map[string]bool{}
	byKey := map[string]models.Server{}
	for _, server := range candidates {
		if key := NodeKeyOfServer(server); key != "" {
			keep[key] = true
			byKey[key] = server
		}
	}
	// The pool can still hold a node the subscription dropped, and its row keeps the flag until the next refresh
	if layout, err := ReadPoolLayout(outboundsPath, selector); err == nil {
		for _, ep := range layout {
			keep[ep.Key()] = true
		}
	}

	var stale []models.Server
	for _, key := range store.Stale(keep) {
		if server, ok := byKey[key]; ok {
			stale = append(stale, server)
		}
	}
	if len(stale) == 0 {
		return 0, nil
	}

	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return 0, err
	}
	outbounds, _ := config["outbounds"].([]interface{})
	_, template := findProxyOutbound(outbounds)
	built, err := BuildPoolOutbounds(stale, "scan-", template, nil)
	if err != nil {
		return 0, err
	}
	nodes := make([]map[string]interface{}, 0, len(built))
	for _, raw := range built {
		if node, ok := raw.(map[string]interface{}); ok {
			nodes = append(nodes, node)
		}
	}

	scanned, err := ScanExits(rt, nodes, timeout)
	if err != nil {
		return 0, err
	}
	return len(nodes), store.Update(keep, scanned)
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

func (s *ExitStore) Stale(nodes map[string]bool) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stale []string
	for key := range nodes {
		exits, ok := s.nodes[key]
		switch {
		case !ok:
			stale = append(stale, key)
		case time.Since(exits.FailedAt) < exitRetryAfter:
		case len(exits.Exits) == 0 || time.Since(exits.CheckedAt) > exitRescanEvery:
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	return stale
}

func (s *ExitStore) Update(keep map[string]bool, scanned map[string]NodeExits) error {
	s.mu.Lock()
	next := make(map[string]NodeExits, len(keep))
	for key := range keep {
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
