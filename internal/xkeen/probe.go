package xkeen

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// ProbeInboundTag marks the local socks inbound the panel checks the proxy through.
//
// The router's own requests never enter XKeen's chains — it hooks PREROUTING
// only — so a check without this inbound measures the direct uplink and stays
// green while every client behind the proxy is cut off.
const ProbeInboundTag = "panel-probe"

// The inbound lives in its own file because inbounds merge across files by tag;
// its rule cannot, since a later file's routing section replaces earlier ones.
const probeInboundFile = "00_panel_probe.json"

// EnsureProbeRoute installs the probe inbound and a first-place rule sending it
// wherever the proxy rules send client traffic. ready reports that checks can
// go through it; changed means the core has to be restarted to pick it up.
func EnsureProbeRoute(rt Runtime, probeAddr string, top Topology) (ready, changed bool, err error) {
	if rt.Core != CoreXray || probeAddr == "" {
		return false, false, nil
	}

	host, port, err := splitAPIAddr(probeAddr)
	if err != nil {
		return false, false, err
	}

	targetKey, target, ok := probeTarget(top)
	if !ok {
		return false, false, nil
	}

	doc, err := findRoutingDoc(rt, ruleTargets(targetKey, target))
	if err != nil {
		return false, false, err
	}

	inboundPath := filepath.Join(rt.XrayConfDir, probeInboundFile)
	inboundDoc := map[string]interface{}{
		"inbounds": []interface{}{map[string]interface{}{
			"tag":      ProbeInboundTag,
			"listen":   host,
			"port":     port,
			"protocol": "socks",
			"settings": map[string]interface{}{"auth": "noauth", "udp": false},
		}},
	}

	writes := map[string]map[string]interface{}{}

	var current map[string]interface{}
	inboundExisted := ReadJSONC(inboundPath, &current) == nil
	if !inboundExisted || !sameJSON(current, inboundDoc) {
		writes[inboundPath] = inboundDoc
	}

	if setProbeRule(doc.routing, targetKey, target) {
		doc.config["routing"] = doc.routing
		writes[doc.path] = doc.config
	}

	if len(writes) == 0 {
		return true, false, nil
	}

	if err := applyConfigs(rt, writes); err != nil {
		if !inboundExisted {
			os.Remove(inboundPath)
		}
		return false, false, err
	}

	return true, true, nil
}

// probeTarget is where client traffic goes: the balancer in a pool, otherwise
// the single proxy outbound.
func probeTarget(top Topology) (key, target string, ok bool) {
	if top.Mode == TopologyPool && top.BalancerTag != "" {
		return "balancerTag", top.BalancerTag, true
	}
	if len(top.ProxyTags) > 0 {
		return "outboundTag", top.ProxyTags[0], true
	}
	return "", "", false
}

func ruleTargets(key, target string) func(map[string]interface{}) bool {
	return func(rule map[string]interface{}) bool {
		tag, _ := rule[key].(string)
		return tag == target
	}
}

// setProbeRule puts the probe rule first, where nothing else can claim its
// traffic. Returns true when the rules changed.
func setProbeRule(routing map[string]interface{}, key, target string) bool {
	want := map[string]interface{}{
		"type":       "field",
		"inboundTag": []interface{}{ProbeInboundTag},
		key:          target,
	}

	rules := asSlice(routing["rules"])
	kept := make([]interface{}, 0, len(rules)+1)
	kept = append(kept, want)

	for i, raw := range rules {
		rule, ok := raw.(map[string]interface{})
		if ok && isProbeRule(rule) {
			if i == 0 && sameJSON(rule, want) {
				return false
			}
			continue
		}
		kept = append(kept, raw)
	}

	routing["rules"] = kept
	return true
}

func isProbeRule(rule map[string]interface{}) bool {
	tags := stringsOf(rule["inboundTag"])
	return len(tags) == 1 && tags[0] == ProbeInboundTag
}

// sameJSON compares parsed JSON with freshly built maps, whose numbers are ints
// rather than the float64 the decoder produces.
func sameJSON(a, b map[string]interface{}) bool {
	return reflect.DeepEqual(normalizeJSON(a), normalizeJSON(b))
}

func normalizeJSON(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = normalizeJSON(val)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, val := range t {
			out[i] = normalizeJSON(val)
		}
		return out
	case int:
		return float64(t)
	default:
		return v
	}
}

// ProbeProxyURL is the address an HTTP client uses to send a check through the
// core. Empty when the core is not Xray, which has no such inbound.
func ProbeProxyURL(probeAddr string) string {
	if probeAddr == "" {
		return ""
	}
	return fmt.Sprintf("socks5://%s", probeAddr)
}
