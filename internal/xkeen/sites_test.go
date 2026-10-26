package xkeen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParseDomains(t *testing.T) {
	got, err := ParseDomains([]string{"https://Sentry.io/auth/login/, *.sentry-cdn.com\nsentry.dev:443 sentry.io"})
	if err != nil {
		t.Fatalf("ParseDomains: %v", err)
	}
	want := []string{"sentry.io", "sentry-cdn.com", "sentry.dev"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseDomains = %v, want %v", got, want)
	}

	for _, bad := range []string{"sentry", "", "foo bar_baz.com"} {
		if _, err := ParseDomains([]string{bad}); err == nil {
			t.Errorf("ParseDomains(%q) accepted", bad)
		}
	}
}

func TestNormalizeCheckURL(t *testing.T) {
	if got, _ := NormalizeCheckURL("", []string{"sentry.io"}); got != "https://sentry.io/" {
		t.Errorf("default check URL = %q", got)
	}
	if got, _ := NormalizeCheckURL("sentry.io/auth/login/", []string{"sentry.io"}); got != "https://sentry.io/auth/login/" {
		t.Errorf("schemeless check URL = %q", got)
	}
	if _, err := NormalizeCheckURL("ftp://sentry.io", []string{"sentry.io"}); err == nil {
		t.Error("ftp check URL accepted")
	}
}

// sitePool builds a two-node pool whose routing also carries a hand-written sentry rule.
func sitePool(t *testing.T) (Runtime, SiteApply) {
	t.Helper()
	rt, outboundsPath := liveConfDir(t)

	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	doc, err := findRoutingDoc(rt, ruleTargetsBalancer(DefaultBalancerTag))
	if err != nil {
		t.Fatalf("findRoutingDoc: %v", err)
	}
	manual := map[string]interface{}{
		"type":        "field",
		"inboundTag":  []interface{}{"redirect", "tproxy"},
		"domain":      []interface{}{"domain:sentry.io", "domain:other.example"},
		"balancerTag": DefaultBalancerTag,
	}
	doc.routing["rules"] = append([]interface{}{manual}, asSlice(doc.routing["rules"])...)
	doc.config["routing"] = doc.routing
	if err := WriteOutboundsConfig(doc.path, doc.config); err != nil {
		t.Fatalf("write routing: %v", err)
	}

	return rt, SiteApply{OutboundsPath: outboundsPath, Selector: DefaultPoolSelector, PoolBalancer: DefaultBalancerTag}
}

func readSiteNodes(t *testing.T, rt Runtime) []map[string]interface{} {
	t.Helper()
	var doc map[string]interface{}
	if err := ReadJSONC(filepath.Join(rt.XrayConfDir, sitesConfigFile), &doc); err != nil {
		t.Fatalf("read sites file: %v", err)
	}
	var nodes []map[string]interface{}
	for _, raw := range asSlice(doc["outbounds"]) {
		nodes = append(nodes, raw.(map[string]interface{}))
	}
	return nodes
}

func TestApplySitesRoutesThroughPassingNodes(t *testing.T) {
	rt, opts := sitePool(t)
	passing := NodeKeyOfServer(poolServers()[1])

	sites := []Site{{
		ID:      1,
		Domains: []string{"sentry.io"},
		Results: []SiteResult{
			{Node: NodeKeyOfServer(poolServers()[0]), OK: false, Status: 403},
			{Node: passing, OK: true, Status: 200},
		},
	}}

	restart, err := ApplySites(rt, opts, sites)
	if err != nil {
		t.Fatalf("ApplySites: %v", err)
	}
	if !restart {
		t.Error("a new site changes routing and must ask for a restart")
	}

	nodes := readSiteNodes(t, rt)
	if len(nodes) != 1 || nodes[0]["tag"] != "site1-1" {
		t.Fatalf("site nodes = %v, want one node tagged site1-1", nodes)
	}
	if ep, _ := endpointOfOutbound(nodes[0]); ep.Key() != passing {
		t.Errorf("site node points at %s, want the passing one %s", ep.Key(), passing)
	}

	doc, _ := findRoutingDoc(rt, ruleTargetsBalancer(DefaultBalancerTag))
	rules := asSlice(doc.routing["rules"])

	first := rules[0].(map[string]interface{})
	if first["balancerTag"] != "site1" {
		t.Fatalf("site rule not placed first: %v", rules[0])
	}
	if !reflect.DeepEqual(first["inboundTag"], []interface{}{"redirect", "tproxy"}) {
		t.Errorf("site rule inbounds = %v, want the catch-all's", first["inboundTag"])
	}

	for _, raw := range rules {
		rule := raw.(map[string]interface{})
		if rule["balancerTag"] != DefaultBalancerTag {
			continue
		}
		for _, d := range asSlice(rule["domain"]) {
			if d == "domain:sentry.io" {
				t.Error("the hand-written rule still claims sentry.io")
			}
		}
	}

	top := ReadTopology(rt)
	if top.BalancerTag != DefaultBalancerTag {
		t.Errorf("topology picked balancer %q, want the pool's", top.BalancerTag)
	}
	for _, tag := range top.ProxyTags {
		if IsSiteTag(tag) {
			t.Errorf("site node %s counted as a proxy node", tag)
		}
	}

	observatory := doc.config["observatory"].(map[string]interface{})
	if !reflect.DeepEqual(stringsOf(observatory["subjectSelector"]), []string{DefaultPoolSelector, "site"}) {
		t.Errorf("observatory selectors = %v, site nodes need probing for leastPing", observatory["subjectSelector"])
	}

	if restart, err := ApplySites(rt, opts, sites); err != nil || restart {
		t.Errorf("re-applying the same sites: restart=%v err=%v, want a no-op", restart, err)
	}
}

func TestApplySitesKeepsWholePoolWhenNothingPasses(t *testing.T) {
	rt, opts := sitePool(t)

	sites := []Site{{ID: 2, Domains: []string{"sentry.io"}, Results: []SiteResult{
		{Node: NodeKeyOfServer(poolServers()[0])},
		{Node: NodeKeyOfServer(poolServers()[1])},
	}}}

	if _, err := ApplySites(rt, opts, sites); err != nil {
		t.Fatalf("ApplySites: %v", err)
	}

	if nodes := readSiteNodes(t, rt); len(nodes) != 2 {
		t.Errorf("site nodes = %d, want the whole pool when no node passes", len(nodes))
	}
}

func TestApplySitesRemovesLastSite(t *testing.T) {
	rt, opts := sitePool(t)
	sites := []Site{{ID: 1, Domains: []string{"sentry.io"}}}

	if _, err := ApplySites(rt, opts, sites); err != nil {
		t.Fatalf("ApplySites: %v", err)
	}
	if _, err := ApplySites(rt, opts, nil); err != nil {
		t.Fatalf("ApplySites(nil): %v", err)
	}

	if _, err := os.Stat(filepath.Join(rt.XrayConfDir, sitesConfigFile)); !os.IsNotExist(err) {
		t.Errorf("sites file left behind: %v", err)
	}

	doc, _ := findRoutingDoc(rt, ruleTargetsBalancer(DefaultBalancerTag))
	for _, raw := range asSlice(doc.routing["balancers"]) {
		if tag := raw.(map[string]interface{})["tag"].(string); IsSiteTag(tag) {
			t.Errorf("site balancer %s left behind", tag)
		}
	}
	for _, raw := range asSlice(doc.routing["rules"]) {
		if tag, _ := raw.(map[string]interface{})["balancerTag"].(string); IsSiteTag(tag) {
			t.Errorf("site rule for %s left behind", tag)
		}
	}
	observatory := doc.config["observatory"].(map[string]interface{})
	if got := stringsOf(observatory["subjectSelector"]); !reflect.DeepEqual(got, []string{DefaultPoolSelector}) {
		t.Errorf("observatory selectors = %v after the last site is gone", got)
	}
}

func TestSiteTags(t *testing.T) {
	for tag, want := range map[string]bool{"site1": true, "site12-3": true, "sub-1": false, "website": false, "site": false} {
		if IsSiteTag(tag) != want {
			t.Errorf("IsSiteTag(%q) = %v", tag, !want)
		}
	}
}

func TestSiteStale(t *testing.T) {
	a := endpoint{Address: "a", Port: 443, UUID: "u"}
	b := endpoint{Address: "b", Port: 443, UUID: "u"}
	site := Site{CheckedAt: time.Now(), Results: []SiteResult{{Node: a.Key()}}}

	if stale(site, PoolLayout{"sub-1": a}) {
		t.Error("a fresh check of the same pool is not stale")
	}
	if !stale(site, PoolLayout{"sub-1": a, "sub-2": b}) {
		t.Error("a node the check never saw makes the site stale")
	}
	site.CheckedAt = time.Now().Add(-siteRecheckEvery - time.Minute)
	if !stale(site, PoolLayout{"sub-1": a}) {
		t.Error("an old check is stale")
	}
}
