package xkeen

import "strings"

// Xray accepts two shapes for a VLESS outbound:
//
//	classic: settings.vnext[0].{address,port,users[0].{id,flow,encryption}}
//	flat:    settings.{address,port,id,flow,encryption}   (Xray 25.x and newer)
//
// Both are live in the wild — the fork's own docs point at generators that emit
// the flat one. The panel keeps whichever shape the file already uses instead of
// rewriting the owner's config into the other one.
type outboundFormat int

const (
	formatFlat outboundFormat = iota
	formatVNext
)

const defaultProxyTag = "vless-reality"

// serviceProtocols never carry a subscription server.
var serviceProtocols = map[string]bool{
	"freedom":   true,
	"blackhole": true,
	"loopback":  true,
	"dns":       true,
}

// isServiceOutbound reports whether the outbound is infrastructure rather than a
// proxy node. Tags are checked too because XKeen templates label them that way.
func isServiceOutbound(ob map[string]interface{}) bool {
	protocol, _ := ob["protocol"].(string)
	if serviceProtocols[protocol] {
		return true
	}
	tag, _ := ob["tag"].(string)
	return tag == "direct" || tag == "block"
}

// detectOutboundFormat inspects an existing outbound. Flat is the default for a
// config that has no proxy outbound yet — it is what current Xray generators emit.
func detectOutboundFormat(ob map[string]interface{}) outboundFormat {
	settings, _ := ob["settings"].(map[string]interface{})
	if settings == nil {
		return formatFlat
	}
	if _, ok := settings["vnext"]; ok {
		return formatVNext
	}
	return formatFlat
}

// canonicalNetwork folds Xray's transport aliases. "raw" is the current name of
// what used to be "tcp"; a subscription URI may use either.
func canonicalNetwork(n string) string {
	switch n {
	case "", "tcp", "raw":
		return "tcp"
	case "h2", "http":
		return "h2"
	default:
		return n
	}
}

// buildVLESSSettings renders the credentials in the requested shape.
func buildVLESSSettings(p *vlessParams, format outboundFormat) map[string]interface{} {
	encryption := p.Encryption
	if encryption == "" {
		encryption = "none"
	}

	if format == formatFlat {
		settings := map[string]interface{}{
			"address":    p.Address,
			"port":       p.Port,
			"id":         p.UUID,
			"encryption": encryption,
		}
		if p.Flow != "" {
			settings["flow"] = p.Flow
		}
		return settings
	}

	user := map[string]interface{}{
		"id":         p.UUID,
		"encryption": encryption,
		"level":      0,
	}
	if p.Flow != "" {
		user["flow"] = p.Flow
	}

	return map[string]interface{}{
		"vnext": []interface{}{
			map[string]interface{}{
				"address": p.Address,
				"port":    p.Port,
				"users":   []interface{}{user},
			},
		},
	}
}

// readProxyEndpoint extracts address/port/uuid from either shape.
func readProxyEndpoint(ob map[string]interface{}) (address string, port int, uuid string, ok bool) {
	settings, _ := ob["settings"].(map[string]interface{})
	if settings == nil {
		return "", 0, "", false
	}

	if vnext, isVNext := settings["vnext"].([]interface{}); isVNext {
		if len(vnext) == 0 {
			return "", 0, "", false
		}
		entry, _ := vnext[0].(map[string]interface{})
		if entry == nil {
			return "", 0, "", false
		}
		address, _ = entry["address"].(string)
		port = toInt(entry["port"])
		if users, _ := entry["users"].([]interface{}); len(users) > 0 {
			if user, _ := users[0].(map[string]interface{}); user != nil {
				uuid, _ = user["id"].(string)
			}
		}
		return address, port, uuid, address != ""
	}

	address, _ = settings["address"].(string)
	port = toInt(settings["port"])
	uuid, _ = settings["id"].(string)

	return address, port, uuid, address != ""
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// mergeOutbound overlays the generated outbound onto the existing one.
//
// The existing outbound may carry settings the panel knows nothing about, and
// one of them is load-bearing: XKeen validates streamSettings.sockopt.mark on
// every real outbound (strict PBR refuses to start without it, Entware proxying
// warns), so blindly replacing the outbound can leave XKeen unable to start.
func mergeOutbound(existing, generated map[string]interface{}) map[string]interface{} {
	if existing == nil {
		return generated
	}

	merged := make(map[string]interface{}, len(generated)+len(existing))
	for k, v := range generated {
		merged[k] = v
	}
	for k, v := range existing {
		if _, taken := merged[k]; !taken {
			merged[k] = v
		}
	}

	merged["streamSettings"] = mergeStreamSettings(
		mapOf(existing["streamSettings"]),
		mapOf(generated["streamSettings"]),
	)

	return merged
}

func mergeStreamSettings(existing, generated map[string]interface{}) map[string]interface{} {
	if generated == nil {
		generated = map[string]interface{}{}
	}
	if existing == nil {
		return generated
	}

	merged := make(map[string]interface{}, len(generated)+len(existing))
	for k, v := range generated {
		merged[k] = v
	}

	// sockopt carries the Keenetic policy mark — never regenerated, always kept
	if sockopt, ok := existing["sockopt"]; ok {
		merged["sockopt"] = sockopt
	}

	// Preserve the owner's transport spelling ("raw" vs "tcp") when equivalent
	if oldNet, ok := existing["network"].(string); ok {
		newNet, _ := merged["network"].(string)
		if canonicalNetwork(oldNet) == canonicalNetwork(newNet) {
			merged["network"] = oldNet
		}
	}

	return merged
}

func mapOf(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

// findProxyOutbound returns the index of the first non-service outbound.
func findProxyOutbound(outbounds []interface{}) (int, map[string]interface{}) {
	for i, raw := range outbounds {
		ob, ok := raw.(map[string]interface{})
		if !ok || isServiceOutbound(ob) {
			continue
		}
		return i, ob
	}
	return -1, nil
}

// countProxyOutbounds counts non-service outbounds; more than one means a
// balancer pool rather than a single upstream.
func countProxyOutbounds(outbounds []interface{}) int {
	n := 0
	for _, raw := range outbounds {
		if ob, ok := raw.(map[string]interface{}); ok && !isServiceOutbound(ob) {
			n++
		}
	}
	return n
}

// outboundTag returns the tag of an outbound, falling back to the XKeen default.
func outboundTag(ob map[string]interface{}) string {
	if ob == nil {
		return defaultProxyTag
	}
	if tag, _ := ob["tag"].(string); strings.TrimSpace(tag) != "" {
		return tag
	}
	return defaultProxyTag
}
