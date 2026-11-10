package xkeen

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"xkeen-panel/internal/models"
)

// SiteSource narrows a site to servers of the whole subscription; nil keeps it on the pool.
type SiteSource struct {
	Countries []string `json:"countries,omitempty"`
	Nodes     []string `json:"nodes,omitempty"`
}

func (s *SiteSource) IsPool() bool {
	return s == nil || (len(s.Countries) == 0 && len(s.Nodes) == 0)
}

// An unknown exit country passes: the trace only works on Cloudflare-fronted sites, and the scan already filtered by country
func (s *SiteSource) allowsCountry(cc string) bool {
	if s == nil || len(s.Countries) == 0 || cc == "" {
		return true
	}
	for _, c := range s.Countries {
		if c == cc {
			return true
		}
	}
	return false
}

var countryCodeRe = regexp.MustCompile(`^[A-Z]{2}$`)

func normalizeSource(src SiteSource) (*SiteSource, error) {
	if len(src.Countries) > 0 && len(src.Nodes) > 0 {
		return nil, fmt.Errorf("выберите либо страны, либо серверы")
	}

	out := SiteSource{Countries: []string{}, Nodes: []string{}}
	seen := map[string]bool{}
	for _, c := range src.Countries {
		c = strings.ToUpper(strings.TrimSpace(c))
		if !countryCodeRe.MatchString(c) {
			return nil, fmt.Errorf("неверный код страны %q", c)
		}
		if !seen[c] {
			seen[c] = true
			out.Countries = append(out.Countries, c)
		}
	}
	for _, n := range src.Nodes {
		n = strings.TrimSpace(n)
		if n != "" && !seen[n] {
			seen[n] = true
			out.Nodes = append(out.Nodes, n)
		}
	}

	if out.IsPool() {
		return nil, nil
	}
	sort.Strings(out.Countries)
	sort.Strings(out.Nodes)
	return &out, nil
}

func vlessServers(servers []models.Server) []models.Server {
	var vless []models.Server
	for _, server := range servers {
		if server.RawURI != "" && (server.Protocol == "" || server.Protocol == "vless") {
			vless = append(vless, server)
		}
	}
	return uniqueEndpoints(vless)
}

func sourceServers(src *SiteSource, servers []models.Server, exits *ExitStore) []models.Server {
	if src.IsPool() {
		return nil
	}

	nodes := map[string]bool{}
	for _, n := range src.Nodes {
		nodes[n] = true
	}
	countries := map[string]bool{}
	for _, c := range src.Countries {
		countries[c] = true
	}

	var picked []models.Server
	for _, server := range vlessServers(servers) {
		key := NodeKeyOfServer(server)
		if key == "" {
			continue
		}
		if len(nodes) > 0 {
			if nodes[key] {
				picked = append(picked, server)
			}
			continue
		}
		if found, ok := exits.Get(key); ok && countries[found.Country()] {
			picked = append(picked, server)
		}
	}
	return picked
}

// The template carries the sockopt mark XKeen validates on every outbound
func outboundsForServers(outboundsPath string, servers []models.Server) ([]map[string]interface{}, error) {
	if len(servers) == 0 {
		return nil, nil
	}

	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return nil, err
	}
	outbounds, _ := config["outbounds"].([]interface{})
	_, template := findProxyOutbound(outbounds)

	built, err := BuildPoolOutbounds(servers, "scan-", template, nil)
	if err != nil {
		return nil, err
	}
	nodes := make([]map[string]interface{}, 0, len(built))
	for _, raw := range built {
		if node, ok := raw.(map[string]interface{}); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes, nil
}

// siteNodes is what a site is checked against and routed through; an empty result for a non-pool source means nothing matched
func (m *SiteManager) siteNodes(site Site, pool []map[string]interface{}) ([]map[string]interface{}, error) {
	if site.Source.IsPool() {
		return pool, nil
	}
	return outboundsForServers(m.outboundsPath, sourceServers(site.Source, m.subscription.GetServers(), m.Exits))
}

func (m *SiteManager) siteNodeKeys(site Site, layout PoolLayout) []string {
	var keys []string
	if site.Source.IsPool() {
		for _, ep := range layout {
			keys = append(keys, ep.Key())
		}
		return keys
	}
	for _, server := range sourceServers(site.Source, m.subscription.GetServers(), m.Exits) {
		keys = append(keys, NodeKeyOfServer(server))
	}
	return keys
}

func (m *SiteManager) SetSource(id int, src SiteSource) error {
	if reason := m.Unavailable(); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	source, err := normalizeSource(src)
	if err != nil {
		return err
	}

	m.work.Lock()
	site, ok := m.store.Get(id)
	if !ok {
		m.work.Unlock()
		return fmt.Errorf("сайт не найден")
	}
	site.Source = source
	err = m.store.Put(site)
	m.work.Unlock()
	if err != nil {
		return err
	}

	switch {
	case source == nil:
		m.Logf("[SITES] %s снова ходит через пул", site.Domains[0])
	case len(source.Countries) > 0:
		m.Logf("[SITES] %s: только серверы с выходом из %s", site.Domains[0], strings.Join(source.Countries, ", "))
	default:
		m.Logf("[SITES] %s: только выбранные серверы (%d)", site.Domains[0], len(source.Nodes))
	}

	m.CheckAsync(id)
	return nil
}

type SourceCountry struct {
	Code    string `json:"code"`
	Servers int    `json:"servers"`
}

type SourceServer struct {
	Key         string `json:"key"`
	ServerID    int    `json:"server_id"`
	Name        string `json:"name"`
	Country     string `json:"country,omitempty"`
	ExitCountry string `json:"exit_country,omitempty"`
	Latency     int    `json:"latency_ms"`
}

type SourceOptions struct {
	Countries []SourceCountry `json:"countries"`
	Servers   []SourceServer  `json:"servers"`
}

// Countries come from scanned exits only, so a server whose name lies about its country lands where it really exits
func (m *SiteManager) SourceOptions() SourceOptions {
	opts := SourceOptions{Countries: []SourceCountry{}, Servers: []SourceServer{}}
	counts := map[string]int{}

	for _, server := range vlessServers(m.subscription.GetServers()) {
		key := NodeKeyOfServer(server)
		if key == "" {
			continue
		}
		view := SourceServer{Key: key, ServerID: server.ID, Name: server.Name, Country: ServerCountry(server), Latency: server.Latency}
		if found, ok := m.Exits.Get(key); ok {
			view.ExitCountry = found.Country()
		}
		if view.ExitCountry != "" {
			counts[view.ExitCountry]++
		}
		opts.Servers = append(opts.Servers, view)
	}

	for code, n := range counts {
		opts.Countries = append(opts.Countries, SourceCountry{Code: code, Servers: n})
	}
	sort.Slice(opts.Countries, func(i, j int) bool {
		if opts.Countries[i].Servers != opts.Countries[j].Servers {
			return opts.Countries[i].Servers > opts.Countries[j].Servers
		}
		return opts.Countries[i].Code < opts.Countries[j].Code
	})

	return opts
}
