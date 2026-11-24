package xkeen

import (
	"testing"
	"time"
)

func stickyManager(t *testing.T) *SiteManager {
	t.Helper()
	dir := t.TempDir()
	return NewSiteManager(NewSiteStore(dir), nil, NewSubscriptionManager(dir), "", "", time.Second)
}

func stickyNodes() []map[string]interface{} {
	var nodes []map[string]interface{}
	for _, host := range []string{"a.example", "b.example", "c.example"} {
		nodes = append(nodes, map[string]interface{}{"tag": "sub-" + host, "protocol": "vless", "settings": vnextSettings(host)})
	}
	return nodes
}

func stickyKey(host string) string {
	return endpoint{Address: host, Port: 443, UUID: "u"}.Key()
}

func TestResolveStickyKeepsTheCurrentNode(t *testing.T) {
	m := stickyManager(t)
	m.Preferred = func() string { return stickyKey("a.example") }
	site := Site{ID: 1, Sticky: stickyKey("c.example"), Results: []SiteResult{
		{Node: stickyKey("a.example"), OK: true, Ms: 100},
		{Node: stickyKey("b.example"), OK: true, Ms: 50},
		{Node: stickyKey("c.example"), OK: true, Ms: 900},
	}}

	key, node := m.resolveSticky(site, stickyNodes())
	if key != site.Sticky || outboundHost(node) != "c.example" {
		t.Errorf("sticky moved to %s, want it kept: a faster node is no reason to change the exit IP", key)
	}
}

func TestResolveStickyLeavesAFailedNode(t *testing.T) {
	m := stickyManager(t)
	site := Site{ID: 1, Sticky: stickyKey("c.example"), Results: []SiteResult{
		{Node: stickyKey("a.example"), OK: true, Ms: 100},
		{Node: stickyKey("b.example"), OK: true, Ms: 50},
		{Node: stickyKey("c.example"), OK: false, Status: 403},
	}}

	if key, _ := m.resolveSticky(site, stickyNodes()); key != stickyKey("b.example") {
		t.Errorf("sticky = %s, want the fastest passing node", key)
	}

	m.Preferred = func() string { return stickyKey("a.example") }
	if key, _ := m.resolveSticky(site, stickyNodes()); key != stickyKey("a.example") {
		t.Errorf("sticky = %s, want the node the rest of the traffic leaves through", key)
	}

	m.Preferred = func() string { return stickyKey("c.example") }
	if key, _ := m.resolveSticky(site, stickyNodes()); key != stickyKey("b.example") {
		t.Errorf("sticky = %s, the main node does not open the site and must be skipped", key)
	}
}

func TestResolveStickyNothingPasses(t *testing.T) {
	m := stickyManager(t)
	site := Site{ID: 1, Sticky: stickyKey("a.example"), Results: []SiteResult{{Node: stickyKey("a.example")}}}

	if key, node := m.resolveSticky(site, stickyNodes()); key != "" || node != nil {
		t.Errorf("sticky = %s, want none so the site falls back to all its nodes", key)
	}
}

func TestSiteOutboundsStickyIsTheOnlyNode(t *testing.T) {
	nodes := stickyNodes()
	sites := []Site{{ID: 7, Results: []SiteResult{
		{Node: stickyKey("a.example"), OK: true},
		{Node: stickyKey("b.example"), OK: true},
	}}}

	out := siteOutbounds(sites, nodes, nil, map[int]map[string]interface{}{7: nodes[1]})
	if len(out) != 1 {
		t.Fatalf("got %d outbounds, want the sticky node alone", len(out))
	}
	only := out[0].(map[string]interface{})
	if only["tag"] != "site7-1" || outboundHost(only) != "b.example" {
		t.Errorf("site outbound = %v, want b.example as site7-1", only)
	}
}

func TestResolveStickyPrefersOneExit(t *testing.T) {
	m := stickyManager(t)
	m.Preferred = func() string { return stickyKey("a.example") }
	m.Exits = NewExitStore(t.TempDir())
	keep := map[string]bool{stickyKey("a.example"): true, stickyKey("b.example"): true, stickyKey("c.example"): true}
	if err := m.Exits.Update(keep, map[string]NodeExits{
		stickyKey("a.example"): {Exits: []NodeExit{{Via: "Cloudflare", IP: "203.0.113.1"}, {Via: "Google Cloud", IP: "104.28.0.1"}}},
		stickyKey("b.example"): {Exits: []NodeExit{{Via: "Cloudflare", IP: "203.0.113.2"}}},
		stickyKey("c.example"): {Exits: []NodeExit{{Via: "Cloudflare", IP: "203.0.113.3"}, {Via: "Google Cloud", IP: "203.0.113.3"}}},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	site := Site{ID: 1, Results: []SiteResult{
		{Node: stickyKey("a.example"), OK: true, Ms: 100},
		{Node: stickyKey("b.example"), OK: true, Ms: 50},
		{Node: stickyKey("c.example"), OK: true, Ms: 900},
	}}

	if key, _ := m.resolveSticky(site, stickyNodes()); key != stickyKey("c.example") {
		t.Errorf("sticky = %s, want the only node every network sees from one address", key)
	}

	site.Results[2].OK = false
	if key, _ := m.resolveSticky(site, stickyNodes()); key != stickyKey("a.example") {
		t.Errorf("sticky = %s, with no one-exit node left the main node comes first", key)
	}
}
