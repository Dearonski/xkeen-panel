package xkeen

import (
	"os"
	"path/filepath"
	"strings"

	"xkeen-panel/internal/models"
)

// entwareMark is the literal mark XKeen 2.x validates on every outbound when
// it proxies the router's own traffic — not the Keenetic policy fwmark.
const entwareMark = 255

// Networks that never leave through the proxy. 198.51.100.0/24 is where KeenDNS
// hairpins the router's own domain, and it is not a private range.
var directNetworks = []interface{}{
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8",
	"169.254.0.0/16", "100.64.0.0/10", "198.51.100.0/24", "fc00::/7", "fe80::/10",
}

// NeedsBootstrap reports a config with no proxy outbound at all, which is what
// a fresh XKeen install ships: 04_outbounds.json holds only a comment.
func NeedsBootstrap(rt Runtime, top Topology) bool {
	return rt.Core == CoreXray && len(top.ProxyTags) == 0
}

// Bootstrap writes a working config from scratch: a pool built from the
// subscription, direct and block outbounds, and routing that sends Russian
// traffic direct and everything else through the pool.
//
// It replaces the routing file wholesale, so it must only run when NeedsBootstrap
// holds — rules written for proxy outbounds that do not exist route nowhere.
func Bootstrap(rt Runtime, outboundsPath string, servers []models.Server, opts PoolOptions) (PoolState, error) {
	balancerTag := opts.BalancerTag
	if balancerTag == "" {
		balancerTag = DefaultBalancerTag
	}
	selector := opts.Selector
	if selector == "" {
		selector = DefaultPoolSelector
	}

	template := map[string]interface{}{
		"streamSettings": map[string]interface{}{"sockopt": map[string]interface{}{"mark": entwareMark}},
	}
	nodes, err := BuildPoolOutbounds(SelectPoolServers(servers, opts.Selection), selector, template, nil)
	if err != nil {
		return PoolState{}, err
	}

	// Anything else in the file is kept; a placeholder that does not parse is not worth keeping
	outbounds := map[string]interface{}{}
	if err := ReadJSONC(outboundsPath, &outbounds); err != nil {
		outbounds = map[string]interface{}{}
	}
	outbounds["outbounds"] = append(nodes,
		map[string]interface{}{
			"tag":            "direct",
			"protocol":       "freedom",
			"streamSettings": map[string]interface{}{"sockopt": map[string]interface{}{"mark": entwareMark}},
		},
		map[string]interface{}{"tag": "block", "protocol": "blackhole"},
	)

	routingPath := rt.RoutingFile
	if routingPath == "" {
		routingPath = filepath.Join(rt.XrayConfDir, "05_routing.json")
	}

	writes := map[string]map[string]interface{}{
		outboundsPath: outbounds,
		routingPath: {
			"routing":     bootstrapRouting(rt, balancerTag, selector),
			"observatory": observatoryBlock(selector, len(nodes)),
		},
	}

	apiPath := filepath.Join(rt.XrayConfDir, apiConfigFile)
	apiCreated := false
	if opts.APIAddr != "" {
		if _, err := os.Stat(apiPath); os.IsNotExist(err) {
			apiDoc, err := apiConfigDoc(opts.APIAddr)
			if err != nil {
				return PoolState{}, err
			}
			writes[apiPath] = apiDoc
			apiCreated = true
		}
	}

	if err := applyConfigs(rt, writes); err != nil {
		if apiCreated {
			os.Remove(apiPath)
		}
		return PoolState{}, err
	}

	return PoolState{
		Enabled:     true,
		BalancerTag: balancerTag,
		Selector:    selector,
		OriginalTag: defaultProxyTag,
		RoutingFile: routingPath,
		APIFile:     apiFileIfCreated(apiPath, apiCreated),
	}, nil
}

func bootstrapRouting(rt Runtime, balancerTag, selector string) map[string]interface{} {
	clients, forced := transparentInboundTags(rt)

	rule := func(fields map[string]interface{}) map[string]interface{} {
		fields["type"] = "field"
		if len(clients) > 0 {
			fields["inboundTag"] = clients
		}
		return fields
	}

	var rules []interface{}

	// The full-proxy policy skips the Russian exceptions by design
	if len(forced) > 0 {
		rules = append(rules, map[string]interface{}{
			"type": "field", "inboundTag": forced, "balancerTag": balancerTag,
		})
	}

	rules = append(rules, rule(map[string]interface{}{"ip": directNetworks, "outboundTag": "direct"}))

	ruDomains := []interface{}{"domain:ru", "domain:su", "domain:xn--p1ai"}
	if datExists(rt, "geosite_v2fly.dat") {
		ruDomains = append(ruDomains, "ext:geosite_v2fly.dat:category-ru")
	}
	rules = append(rules, rule(map[string]interface{}{"domain": ruDomains, "outboundTag": "direct"}))

	if datExists(rt, "geoip_v2fly.dat") {
		rules = append(rules, rule(map[string]interface{}{"ip": []interface{}{"ext:geoip_v2fly.dat:ru"}, "outboundTag": "direct"}))
	} else {
		Log("[BOOTSTRAP] geoip_v2fly.dat не найден — российские IP без домена пойдут через прокси")
	}

	rules = append(rules, rule(map[string]interface{}{"balancerTag": balancerTag}))

	return map[string]interface{}{
		"domainStrategy": "AsIs",
		"balancers":      []interface{}{balancerBlock(balancerTag, selector)},
		"rules":          rules,
	}
}

// transparentInboundTags splits XKeen's transparent inbounds into the regular
// policy ones and the force-proxy ones of the full-proxy policy.
func transparentInboundTags(rt Runtime) (clients, forced []interface{}) {
	for _, path := range ConfigFiles(rt) {
		var cfg map[string]interface{}
		if err := ReadJSONC(path, &cfg); err != nil {
			continue
		}
		for _, raw := range asSlice(cfg["inbounds"]) {
			ib, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			settings, _ := ib["settings"].(map[string]interface{})
			if follow, _ := settings["followRedirect"].(bool); !follow {
				continue
			}
			tag, _ := ib["tag"].(string)
			if tag == "" {
				continue
			}
			if strings.HasPrefix(tag, "force-proxy") {
				forced = appendUnique(forced, tag)
			} else {
				clients = appendUnique(clients, tag)
			}
		}
	}
	return clients, forced
}

func appendUnique(list []interface{}, tag string) []interface{} {
	for _, existing := range list {
		if existing == tag {
			return list
		}
	}
	return append(list, tag)
}

// datExists looks for a geo database where XKeen installs them, next to the
// config directory.
func datExists(rt Runtime, name string) bool {
	_, err := os.Stat(filepath.Join(filepath.Dir(rt.XrayConfDir), "dat", name))
	return err == nil
}
