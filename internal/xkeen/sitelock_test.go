package xkeen

import (
	"reflect"
	"strings"
	"testing"
)

func lockedSite(t *testing.T, opts SiteApply, blocked bool) Site {
	t.Helper()
	pool, err := poolNodesInFile(opts.OutboundsPath, DefaultPoolSelector)
	if err != nil || len(pool) == 0 {
		t.Fatalf("pool: %v", err)
	}
	outbound := deepCopy(pool[1])
	delete(outbound, "tag")
	return Site{
		ID:      1,
		Domains: []string{"anthropic.com", "claude.ai"},
		IPs:     []string{"160.79.104.0/23"},
		// Nothing passes, which must not hand a locked site the pool
		Results: []SiteResult{{Node: NodeKeyOfServer(poolServers()[0])}},
		Lock: &SiteLock{
			Node:     NodeKeyOfServer(poolServers()[1]),
			Outbound: outbound,
			ExitIP:   "203.0.113.7",
			Country:  "SE",
			Blocked:  blocked,
		},
	}
}

func TestApplySitesLockUsesOnePinOutbound(t *testing.T) {
	rt, opts := sitePool(t)
	site := lockedSite(t, opts, false)

	restart, err := ApplySites(rt, opts, []Site{site})
	if err != nil {
		t.Fatalf("ApplySites: %v", err)
	}
	if !restart {
		t.Error("a new lock changes routing and must ask for a restart")
	}

	nodes := readSiteNodes(t, rt)
	if len(nodes) != 1 || nodes[0]["tag"] != "site1-pin" {
		t.Fatalf("site nodes = %v, want only site1-pin", nodes)
	}
	if ep, _ := endpointOfOutbound(nodes[0]); ep.Key() != site.Lock.Node {
		t.Errorf("pin points at %s, want the locked node %s", ep.Key(), site.Lock.Node)
	}

	doc, _ := findRoutingDoc(rt, ruleTargetsBalancer(DefaultBalancerTag))
	for _, raw := range asSlice(doc.routing["balancers"]) {
		if raw.(map[string]interface{})["tag"] == "site1" {
			t.Error("a locked site must not get a balancer")
		}
	}

	rules := asSlice(doc.routing["rules"])
	first := rules[0].(map[string]interface{})
	if !isLockProbeRule(first) || first["outboundTag"] != "site1-pin" {
		t.Fatalf("first rule = %v, want the lock probe", first)
	}

	var domainRule, ipRule map[string]interface{}
	for _, raw := range rules {
		rule := raw.(map[string]interface{})
		if rule["outboundTag"] != "site1-pin" || isProbeRule(rule) {
			continue
		}
		if rule["domain"] != nil {
			domainRule = rule
		}
		if rule["ip"] != nil {
			ipRule = rule
		}
	}
	if domainRule == nil || ipRule == nil {
		t.Fatalf("rules = %v, want separate domain and ip rules to site1-pin", rules)
	}
	if domainRule["ip"] != nil || ipRule["domain"] != nil {
		t.Error("domain and ip in one rule would have to match together")
	}
	if !reflect.DeepEqual(ipRule["inboundTag"], []interface{}{"redirect", "tproxy"}) {
		t.Errorf("ip rule inbounds = %v, want the catch-all's", ipRule["inboundTag"])
	}

	observatory := doc.config["observatory"].(map[string]interface{})
	if got := stringsOf(observatory["subjectSelector"]); !reflect.DeepEqual(got, []string{DefaultPoolSelector}) {
		t.Errorf("observatory selectors = %v, a lock has no balancer to probe for", got)
	}

	for _, tag := range ReadTopology(rt).ProxyTags {
		if IsSiteTag(tag) {
			t.Errorf("pin %s counted as a proxy node", tag)
		}
	}
}

func TestApplySitesBlockedLockDropsThePin(t *testing.T) {
	rt, opts := sitePool(t)

	if _, err := ApplySites(rt, opts, []Site{lockedSite(t, opts, false)}); err != nil {
		t.Fatalf("ApplySites: %v", err)
	}
	before, _ := findRoutingDoc(rt, ruleTargetsBalancer(DefaultBalancerTag))

	if _, err := ApplySites(rt, opts, []Site{lockedSite(t, opts, true)}); err != nil {
		t.Fatalf("ApplySites blocked: %v", err)
	}

	if nodes := readSiteNodes(t, rt); len(nodes) != 0 {
		t.Errorf("site nodes = %v, a blocked lock must leave nothing to route to", nodes)
	}
	after, _ := findRoutingDoc(rt, ruleTargetsBalancer(DefaultBalancerTag))
	if !sameJSON(before.config, after.config) {
		t.Error("blocking changed routing, so it could not go through the core API")
	}
}

func TestSetProbeRuleKeepsLockProbesFirst(t *testing.T) {
	rt, opts := sitePool(t)
	doc, _ := findRoutingDoc(rt, ruleTargetsBalancer(DefaultBalancerTag))
	setProbeRule(doc.routing, "balancerTag", DefaultBalancerTag)
	doc.config["routing"] = doc.routing
	if err := WriteOutboundsConfig(doc.path, doc.config); err != nil {
		t.Fatalf("write routing: %v", err)
	}

	if _, err := ApplySites(rt, opts, []Site{lockedSite(t, opts, false)}); err != nil {
		t.Fatalf("ApplySites: %v", err)
	}

	doc, _ = findRoutingDoc(rt, ruleTargetsBalancer(DefaultBalancerTag))
	if setProbeRule(doc.routing, "balancerTag", DefaultBalancerTag) {
		t.Fatal("the probe rule fought the lock probe for first place")
	}
	rules := asSlice(doc.routing["rules"])
	if !isLockProbeRule(rules[0].(map[string]interface{})) {
		t.Errorf("rules[0] = %v, want the lock probe", rules[0])
	}
	second := rules[1].(map[string]interface{})
	if !isProbeRule(second) || isLockProbeRule(second) || second["balancerTag"] != DefaultBalancerTag {
		t.Errorf("rules[1] = %v, want the probe rule", second)
	}
}

func TestOutboundDiffTouchesOnlyChangedTags(t *testing.T) {
	pin := map[string]interface{}{"tag": "site1-pin", "protocol": "vless"}
	a := map[string]interface{}{"tag": "site2-1", "protocol": "vless", "settings": map[string]interface{}{"address": "a"}}
	b := map[string]interface{}{"tag": "site2-1", "protocol": "vless", "settings": map[string]interface{}{"address": "b"}}

	remove, add := outboundDiff([]interface{}{pin, a}, []interface{}{deepCopy(pin), b})
	if !reflect.DeepEqual(remove, []string{"site2-1"}) || len(add) != 1 || add[0].(map[string]interface{})["tag"] != "site2-1" {
		t.Errorf("diff = -%v +%v, want only site2-1 swapped and the pin left alone", remove, add)
	}

	remove, add = outboundDiff([]interface{}{pin}, nil)
	if !reflect.DeepEqual(remove, []string{"site1-pin"}) || len(add) != 0 {
		t.Errorf("blocking diff = -%v +%v, want just the pin removed", remove, add)
	}
}

func TestParseTrace(t *testing.T) {
	body := "fl=123\nh=api.anthropic.com\nip=203.0.113.7\nts=1\nloc=se\nwarp=off\n"
	if got := parseTrace(body); got != (Exit{IP: "203.0.113.7", Country: "SE"}) {
		t.Errorf("parseTrace = %+v", got)
	}
	if got := parseTrace("<html>blocked</html>"); got.IP != "" {
		t.Errorf("a non-trace page gave %+v", got)
	}
}

func TestParseIPs(t *testing.T) {
	got, err := ParseIPs([]string{"160.79.104.5/23, 2607:6bc0::/48\n1.2.3.4 1.2.3.4"})
	if err != nil {
		t.Fatalf("ParseIPs: %v", err)
	}
	if want := []string{"160.79.104.0/23", "2607:6bc0::/48", "1.2.3.4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ParseIPs = %v, want %v", got, want)
	}
	if _, err := ParseIPs([]string{"anthropic.com"}); err == nil {
		t.Error("a domain accepted as an IP")
	}
}

func TestHostCovered(t *testing.T) {
	domains := []string{"anthropic.com", "claude.ai"}
	for host, want := range map[string]bool{"api.anthropic.com": true, "claude.ai": true, "notanthropic.com": false, "claude.ai.evil": false} {
		if hostCovered(host, domains) != want {
			t.Errorf("hostCovered(%q) = %v", host, !want)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	for v, want := range map[[3]int]bool{{26, 3, 27}: true, {25, 9, 11}: true, {25, 9, 10}: false, {25, 8, 31}: false, {24, 12, 31}: false} {
		if versionAtLeast(v, lockMinXray) != want {
			t.Errorf("versionAtLeast(%v) = %v", v, !want)
		}
	}
}

func TestKillSwitchCoversOnlyLockedSites(t *testing.T) {
	sites := []Site{
		{ID: 1, IPs: []string{"160.79.104.0/23", "2607:6bc0::/48"}, Lock: &SiteLock{}},
		{ID: 2, IPs: []string{"198.51.100.0/24"}},
	}
	v4, v6 := killSwitchIPs(sites)
	if !reflect.DeepEqual(v4, []string{"160.79.104.0/23"}) || !reflect.DeepEqual(v6, []string{"2607:6bc0::/48"}) {
		t.Errorf("kill switch = %v %v, want only the locked site's ranges split by family", v4, v6)
	}

	script := killSwitchScript(v4, v6)
	for _, want := range []string{`iptables) nets="160.79.104.0/23"`, `ip6tables) nets="2607:6bc0::/48"`, "-I FORWARD 1 -j XKEEN_PANEL_LOCK"} {
		if !strings.Contains(script, want) {
			t.Errorf("script lacks %q:\n%s", want, script)
		}
	}
}
