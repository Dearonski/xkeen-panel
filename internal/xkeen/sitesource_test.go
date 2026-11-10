package xkeen

import (
	"testing"
	"time"

	"xkeen-panel/internal/models"
)

func exitsFor(t *testing.T, countries map[models.Server]string) *ExitStore {
	t.Helper()
	store := NewExitStore(t.TempDir())
	keep := map[string]bool{}
	scanned := map[string]NodeExits{}
	for server, cc := range countries {
		key := NodeKeyOfServer(server)
		keep[key] = true
		scanned[key] = NodeExits{CheckedAt: time.Now(), Exits: []NodeExit{{Via: "Cloudflare", IP: "203.0.113.1", Country: cc}}}
	}
	if err := store.Update(keep, scanned); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestNormalizeSource(t *testing.T) {
	if src, err := normalizeSource(SiteSource{}); err != nil || src != nil {
		t.Errorf("empty source = %v, %v; want the pool", src, err)
	}
	if _, err := normalizeSource(SiteSource{Countries: []string{"DE"}, Nodes: []string{"x"}}); err == nil {
		t.Error("countries and nodes together must be rejected")
	}
	if _, err := normalizeSource(SiteSource{Countries: []string{"Germany"}}); err == nil {
		t.Error("a country name instead of a code must be rejected")
	}
	src, err := normalizeSource(SiteSource{Countries: []string{" de", "NL", "DE"}})
	if err != nil || len(src.Countries) != 2 || src.Countries[0] != "DE" || src.Countries[1] != "NL" {
		t.Errorf("normalized = %+v, %v; want [DE NL]", src, err)
	}
}

// The provider's label is not where traffic exits, so a server named LV that exits in RU must not count as Latvia
func TestSourceServersByExitCountry(t *testing.T) {
	lv := vlessAt("LV-1", "LV", "lv.example", 443)
	fake := vlessAt("LV-2", "LV", "lv2.example", 443)
	de := vlessAt("DE-1", "DE", "de.example", 443)
	unscanned := vlessAt("LV-3", "LV", "lv3.example", 443)
	exits := exitsFor(t, map[models.Server]string{lv: "LV", fake: "RU", de: "DE"})

	got := sourceServers(&SiteSource{Countries: []string{"LV"}}, []models.Server{lv, fake, de, unscanned}, exits)

	if len(got) != 1 || got[0].Name != "LV-1" {
		t.Errorf("picked %v, want only LV-1", got)
	}
}

func TestSourceServersByNodes(t *testing.T) {
	a := vlessAt("A", "NL", "a.example", 443)
	b := vlessAt("B", "NL", "b.example", 443)

	got := sourceServers(&SiteSource{Nodes: []string{NodeKeyOfServer(b)}}, []models.Server{a, b}, nil)

	if len(got) != 1 || got[0].Name != "B" {
		t.Errorf("picked %v, want only B", got)
	}
	if got := sourceServers(nil, []models.Server{a, b}, nil); got != nil {
		t.Errorf("a pool source picked %v, want nothing", got)
	}
}

func TestPassingRespectsSourceCountry(t *testing.T) {
	site := Site{
		Source: &SiteSource{Countries: []string{"DE"}},
		Results: []SiteResult{
			{Node: "de", OK: true, ExitCountry: "DE"},
			{Node: "warp", OK: true, ExitCountry: "US"},
			{Node: "untraced", OK: true},
		},
	}

	passing := site.Passing()
	if !passing["de"] || passing["warp"] || !passing["untraced"] {
		t.Errorf("passing = %v, want de and untraced, not the node the site sees from US", passing)
	}
}

func TestSiteOutboundsUsesSourcedNodes(t *testing.T) {
	poolNode := map[string]interface{}{"tag": "sub-1", "protocol": "vless", "settings": vnextSettings("pool.example")}
	own := map[string]interface{}{"tag": "scan-1", "protocol": "vless", "settings": vnextSettings("own.example")}
	sites := []Site{{ID: 1, Source: &SiteSource{Countries: []string{"DE"}}}, {ID: 2, Source: &SiteSource{Countries: []string{"JP"}}}}

	out := siteOutbounds(sites, []map[string]interface{}{poolNode}, map[int][]map[string]interface{}{1: {own}})

	if len(out) != 2 {
		t.Fatalf("got %d outbounds, want one per site", len(out))
	}
	if host := outboundHost(out[0]); host != "own.example" {
		t.Errorf("site 1 routes through %s, want its own node", host)
	}
	if host := outboundHost(out[1]); host != "pool.example" {
		t.Errorf("site 2 matched nothing and routes through %s, want the pool", host)
	}
}

func vnextSettings(host string) map[string]interface{} {
	return map[string]interface{}{"vnext": []interface{}{map[string]interface{}{
		"address": host, "port": float64(443),
		"users": []interface{}{map[string]interface{}{"id": "u"}},
	}}}
}

func outboundHost(raw interface{}) string {
	ep, _ := endpointOfOutbound(raw.(map[string]interface{}))
	return ep.Address
}
