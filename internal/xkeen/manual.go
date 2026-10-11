package xkeen

import (
	"fmt"

	"xkeen-panel/internal/models"
)

// ManualTag is the slot for a server picked by hand from outside the pool. The
// balancer override accepts any outbound tag, so pinning it needs no restart.
// It must never contain the pool selector: Xray matches selectors by substring,
// and the balancer and observatory would adopt it.
const ManualTag = "manual"

// SetManualOutbound writes server into the manual slot and validates the config.
func SetManualOutbound(rt Runtime, outboundsPath string, server models.Server) error {
	if server.Protocol != "" && server.Protocol != "vless" {
		return fmt.Errorf("вручную можно выбрать только VLESS-сервер, у %q протокол %s", server.Name, server.Protocol)
	}
	params, err := ParseVLESS(server.RawURI)
	if err != nil {
		return fmt.Errorf("ошибка парсинга URI: %w", err)
	}

	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return err
	}
	outbounds, ok := config["outbounds"].([]interface{})
	if !ok {
		return fmt.Errorf("outbounds не найдены в %s", outboundsPath)
	}

	// A pool node is the template: it carries the sockopt mark XKeen validates
	_, template := findProxyOutbound(outbounds)
	slot := mergeOutbound(template, buildOutboundFromURI(params, ManualTag, detectOutboundFormat(template)))

	var nodes []interface{}
	for _, raw := range outbounds {
		if ob, ok := raw.(map[string]interface{}); ok && !isServiceOutbound(ob) && outboundTag(ob) != ManualTag {
			nodes = append(nodes, raw)
		}
	}

	// The first outbound is Xray's default route, so the slot goes after the pool
	config["outbounds"] = append(append(nodes, slot), serviceOutbounds(outbounds)...)

	return applyConfigs(rt, map[string]map[string]interface{}{outboundsPath: config})
}

// HasManualOutbound reports whether the manual slot is in the config.
func HasManualOutbound(outboundsPath string) bool {
	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return false
	}
	return manualOutbound(asSlice(config["outbounds"])) != nil
}

// ApplyManualLive swaps the manual slot in the running core. A failure leaves
// the slot missing from the core, so the caller has to restart it.
func ApplyManualLive(rt Runtime, apiAddr, outboundsPath string) error {
	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return err
	}
	slot := manualOutbound(asSlice(config["outbounds"]))
	if slot == nil {
		return fmt.Errorf("в %s нет outbound %q", outboundsPath, ManualTag)
	}

	// The core may not know the tag yet, and `rmo` on an unknown tag fails
	RemoveOutbounds(rt, apiAddr, []string{ManualTag})

	return AddOutbounds(rt, apiAddr, []interface{}{slot})
}

func manualOutbound(outbounds []interface{}) map[string]interface{} {
	for _, raw := range outbounds {
		if ob, ok := raw.(map[string]interface{}); ok {
			if tag, _ := ob["tag"].(string); tag == ManualTag {
				return ob
			}
		}
	}
	return nil
}

// manualOutbounds keeps the slot when the pool around it is rebuilt.
func manualOutbounds(outbounds []interface{}) []interface{} {
	if slot := manualOutbound(outbounds); slot != nil {
		return []interface{}{slot}
	}
	return nil
}
