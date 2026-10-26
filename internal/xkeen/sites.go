package xkeen

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Site routes a set of domains through the pool nodes that actually open them.
type Site struct {
	ID        int          `json:"id"`
	Domains   []string     `json:"domains"`
	CheckURL  string       `json:"check_url"`
	Results   []SiteResult `json:"results,omitempty"`
	CheckedAt time.Time    `json:"checked_at"`
}

type SiteResult struct {
	Node   string `json:"node"`
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	Ms     int    `json:"ms,omitempty"`
}

func (s Site) Passing() map[string]bool {
	passing := map[string]bool{}
	for _, r := range s.Results {
		if r.OK {
			passing[r.Node] = true
		}
	}
	return passing
}

func (s Site) CheckedNodes() map[string]bool {
	checked := make(map[string]bool, len(s.Results))
	for _, r := range s.Results {
		checked[r.Node] = true
	}
	return checked
}

const sitesConfigFile = "04_sites.json"

// observatory matches selectors by substring, so this one covers every site node
const siteObservatorySelector = "site"

var siteTagRe = regexp.MustCompile(`^site\d+(-\d+)?$`)

func IsSiteTag(tag string) bool {
	return siteTagRe.MatchString(tag)
}

func siteBalancerTag(id int) string { return fmt.Sprintf("site%d", id) }

// The trailing dash keeps site1 from selecting the nodes of site10
func siteSelector(id int) string { return fmt.Sprintf("site%d-", id) }

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z0-9-]{2,}$`)

// Owners paste URLs and wildcards as often as bare domains
func ParseDomains(input []string) ([]string, error) {
	seen := map[string]bool{}
	var domains []string

	for _, raw := range input {
		for _, field := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
		}) {
			d := strings.ToLower(strings.TrimSpace(field))
			if i := strings.Index(d, "://"); i >= 0 {
				d = d[i+3:]
			}
			d = strings.TrimPrefix(d, "domain:")
			d = strings.TrimPrefix(d, "*.")
			if i := strings.IndexAny(d, "/?#"); i >= 0 {
				d = d[:i]
			}
			if i := strings.LastIndex(d, ":"); i >= 0 {
				d = d[:i]
			}
			d = strings.TrimSuffix(d, ".")
			if d == "" {
				continue
			}
			if !domainRe.MatchString(d) {
				return nil, fmt.Errorf("%q не похоже на домен", field)
			}
			if !seen[d] {
				seen[d] = true
				domains = append(domains, d)
			}
		}
	}

	if len(domains) == 0 {
		return nil, fmt.Errorf("укажите хотя бы один домен")
	}

	return domains, nil
}

func NormalizeCheckURL(raw string, domains []string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "https://" + domains[0] + "/", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("адрес проверки должен быть ссылкой http(s)")
	}

	return u.String(), nil
}

type SiteStore struct {
	path string
	mu   sync.RWMutex
	data siteData
}

type siteData struct {
	NextID int    `json:"next_id"`
	Sites  []Site `json:"sites"`
}

func NewSiteStore(dataDir string) *SiteStore {
	return &SiteStore{path: filepath.Join(dataDir, "sites.json")}
}

func (s *SiteStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.data)
}

func (s *SiteStore) List() []Site {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sites := make([]Site, len(s.data.Sites))
	copy(sites, s.data.Sites)
	return sites
}

func (s *SiteStore) Get(id int) (Site, bool) {
	for _, site := range s.List() {
		if site.ID == id {
			return site, true
		}
	}
	return Site{}, false
}

func (s *SiteStore) Add(site Site) (Site, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.NextID < 1 {
		s.data.NextID = 1
	}
	site.ID = s.data.NextID
	s.data.NextID++
	s.data.Sites = append(s.data.Sites, site)

	return site, s.saveLocked()
}

// A site deleted while its check ran stays deleted
func (s *SiteStore) Put(site Site) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.data.Sites {
		if s.data.Sites[i].ID == site.ID {
			s.data.Sites[i] = site
			return s.saveLocked()
		}
	}
	return nil
}

func (s *SiteStore) Remove(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	kept := s.data.Sites[:0]
	for _, site := range s.data.Sites {
		if site.ID != id {
			kept = append(kept, site)
		}
	}
	s.data.Sites = kept

	return s.saveLocked()
}

func (s *SiteStore) saveLocked() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path, data, 0600)
}

// A site with no passing node gets the whole pool, so a failed check never cuts it off
func siteOutbounds(sites []Site, pool []map[string]interface{}) []interface{} {
	var outbounds []interface{}

	for _, site := range sites {
		passing := site.Passing()

		var chosen []map[string]interface{}
		for _, node := range pool {
			if ep, ok := endpointOfOutbound(node); ok && passing[ep.Key()] {
				chosen = append(chosen, node)
			}
		}
		if len(chosen) == 0 {
			chosen = pool
		}

		for i, node := range chosen {
			copied := deepCopy(node)
			copied["tag"] = fmt.Sprintf("%s%d", siteSelector(site.ID), i+1)
			outbounds = append(outbounds, copied)
		}
	}

	return outbounds
}

func deepCopy(m map[string]interface{}) map[string]interface{} {
	data, _ := json.Marshal(m)
	var out map[string]interface{}
	_ = json.Unmarshal(data, &out)
	return out
}

// Site domains are dropped from hand-written pool rules, which is how a manual rule turns into a site
func applySiteRouting(doc *routingDoc, sites []Site, poolBalancer string) bool {
	before := deepCopy(doc.config)
	routing := doc.routing

	var balancers []interface{}
	for _, raw := range asSlice(routing["balancers"]) {
		if b, ok := raw.(map[string]interface{}); ok {
			if tag, _ := b["tag"].(string); IsSiteTag(tag) {
				continue
			}
		}
		balancers = append(balancers, raw)
	}
	for _, site := range sites {
		balancers = append(balancers, balancerBlock(siteBalancerTag(site.ID), siteSelector(site.ID)))
	}
	if len(balancers) > 0 {
		routing["balancers"] = balancers
	} else {
		delete(routing, "balancers")
	}

	claimed := map[string]bool{}
	for _, site := range sites {
		for _, d := range site.Domains {
			claimed["domain:"+d] = true
		}
	}

	inboundTags := catchAllInbounds(routing, poolBalancer)

	var head, tail []interface{}
	leading := true
	for _, raw := range asSlice(routing["rules"]) {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			tail = append(tail, raw)
			leading = false
			continue
		}
		if tag, _ := rule["balancerTag"].(string); IsSiteTag(tag) {
			continue
		}
		if leading && isProbeRule(rule) {
			head = append(head, raw)
			continue
		}
		leading = false

		if tag, _ := rule["balancerTag"].(string); tag == poolBalancer && rule["domain"] != nil {
			var domains []interface{}
			for _, d := range asSlice(rule["domain"]) {
				if s, _ := d.(string); !claimed[s] {
					domains = append(domains, d)
				}
			}
			if len(domains) == 0 {
				continue
			}
			rule["domain"] = domains
		}
		tail = append(tail, raw)
	}

	for _, site := range sites {
		domains := make([]interface{}, len(site.Domains))
		for i, d := range site.Domains {
			domains[i] = "domain:" + d
		}
		rule := map[string]interface{}{
			"type":        "field",
			"domain":      domains,
			"balancerTag": siteBalancerTag(site.ID),
		}
		if len(inboundTags) > 0 {
			rule["inboundTag"] = inboundTags
		}
		head = append(head, rule)
	}
	routing["rules"] = append(head, tail...)
	doc.config["routing"] = routing

	if observatory, ok := doc.config["observatory"].(map[string]interface{}); ok {
		var selectors []interface{}
		for _, s := range stringsOf(observatory["subjectSelector"]) {
			if s != siteObservatorySelector {
				selectors = append(selectors, s)
			}
		}
		if len(sites) > 0 {
			selectors = append(selectors, siteObservatorySelector)
		}
		observatory["subjectSelector"] = selectors
	}

	return !sameJSON(before, doc.config)
}

// catchAllInbounds borrows the inbounds of the rule sending everything else to the pool.
func catchAllInbounds(routing map[string]interface{}, poolBalancer string) []interface{} {
	matchers := []string{"domain", "ip", "port", "sourcePort", "source", "protocol", "user", "attrs"}

rules:
	for _, raw := range asSlice(routing["rules"]) {
		rule, ok := raw.(map[string]interface{})
		if !ok || isProbeRule(rule) {
			continue
		}
		if tag, _ := rule["balancerTag"].(string); tag != poolBalancer {
			continue
		}
		for _, key := range matchers {
			if _, present := rule[key]; present {
				continue rules
			}
		}
		tags := asSlice(rule["inboundTag"])
		out := make([]interface{}, len(tags))
		copy(out, tags)
		return out
	}

	return nil
}

type SiteApply struct {
	OutboundsPath string
	Selector      string
	PoolBalancer  string
	APIAddr       string
}

// Only a change in routing needs a restart; swapping a site's nodes goes through the core API
func ApplySites(rt Runtime, opts SiteApply, sites []Site) (restart bool, err error) {
	if rt.Core != CoreXray {
		return false, fmt.Errorf("сайты работают только с ядром xray")
	}

	pool, err := poolNodesInFile(opts.OutboundsPath, opts.Selector)
	if err != nil {
		return false, err
	}
	if len(pool) == 0 && len(sites) > 0 {
		return false, fmt.Errorf("в пуле нет нод")
	}

	sitesPath := filepath.Join(rt.XrayConfDir, sitesConfigFile)
	var current map[string]interface{}
	existed := ReadJSONC(sitesPath, &current) == nil
	oldTags := PoolNodeTags(asSlice(current["outbounds"]))

	outbounds := siteOutbounds(sites, pool)
	sitesDoc := map[string]interface{}{"outbounds": outbounds}
	outboundsChanged := existed != (len(sites) > 0) || (existed && !sameJSON(current, sitesDoc))

	doc, err := findRoutingDoc(rt, ruleTargetsBalancer(opts.PoolBalancer))
	if err != nil {
		return false, err
	}
	routingChanged := applySiteRouting(doc, sites, opts.PoolBalancer)

	if !routingChanged && !outboundsChanged {
		return false, nil
	}

	if !routingChanged && existed && len(sites) > 0 && opts.APIAddr != "" {
		if err := WriteOutboundsConfig(sitesPath, sitesDoc); err != nil {
			return false, err
		}
		if err := swapOutboundsLive(rt, opts.APIAddr, oldTags, outbounds); err == nil {
			return false, nil
		} else {
			Log("[SITES] Горячее обновление недоступно (%v) — нужен перезапуск ядра", err)
		}
		return true, nil
	}

	writes := map[string]map[string]interface{}{doc.path: doc.config}
	if len(sites) > 0 {
		writes[sitesPath] = sitesDoc
	}

	if err := applyConfigs(rt, writes); err != nil {
		if !existed {
			os.Remove(sitesPath)
		}
		return false, err
	}

	if len(sites) == 0 && existed {
		if err := os.Remove(sitesPath); err != nil {
			return false, err
		}
	}

	return true, nil
}

func swapOutboundsLive(rt Runtime, apiAddr string, oldTags []string, outbounds []interface{}) error {
	sort.Strings(oldTags)
	if err := RemoveOutbounds(rt, apiAddr, oldTags); err != nil {
		return err
	}
	return AddOutbounds(rt, apiAddr, outbounds)
}
