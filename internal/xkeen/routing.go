package xkeen

import (
	"fmt"
)

// routingDoc is a config file that carries a routing section.
type routingDoc struct {
	path    string
	config  map[string]interface{}
	routing map[string]interface{}
}

// findRoutingDoc locates the file whose rules match. Xray merges the whole
// directory, so the rules are not necessarily in 05_routing.json — the panel
// edits wherever they actually live.
//
// The fallback prefers the configured routing file: several files carry a
// routing section (the api block has one of its own), and picking the first one
// alphabetically would land on that.
func findRoutingDoc(rt Runtime, matches func(rule map[string]interface{}) bool) (*routingDoc, error) {
	var fallback *routingDoc

	for _, path := range ConfigFiles(rt) {
		var cfg map[string]interface{}
		if err := ReadJSONC(path, &cfg); err != nil {
			continue
		}

		routing, ok := cfg["routing"].(map[string]interface{})
		if !ok {
			continue
		}

		doc := &routingDoc{path: path, config: cfg, routing: routing}
		if fallback == nil || path == rt.RoutingFile {
			fallback = doc
		}

		for _, raw := range asSlice(routing["rules"]) {
			if rule, ok := raw.(map[string]interface{}); ok && matches(rule) {
				return doc, nil
			}
		}
	}

	if fallback != nil {
		return fallback, nil
	}

	return nil, fmt.Errorf("файл маршрутизации не найден в %s", rt.XrayConfDir)
}

// ruleTargetsOutbound matches the rules that send traffic to a proxy outbound.
func ruleTargetsOutbound(tags []string) func(map[string]interface{}) bool {
	wanted := make(map[string]bool, len(tags))
	for _, tag := range tags {
		wanted[tag] = true
	}

	return func(rule map[string]interface{}) bool {
		tag, _ := rule["outboundTag"].(string)
		return wanted[tag]
	}
}

// ruleTargetsBalancer matches the rules a pool installed.
func ruleTargetsBalancer(balancerTag string) func(map[string]interface{}) bool {
	return func(rule map[string]interface{}) bool {
		tag, _ := rule["balancerTag"].(string)
		return tag == balancerTag
	}
}

// retargetRules swaps the outbound a rule points at. Direction is decided by
// toBalancer: outboundTag -> balancerTag when enabling a pool, and back when
// leaving it. Returns how many rules changed.
func retargetRules(routing map[string]interface{}, from, to string, toBalancer bool) int {
	changed := 0

	fromKey, toKey := "outboundTag", "balancerTag"
	if !toBalancer {
		fromKey, toKey = "balancerTag", "outboundTag"
	}

	for _, raw := range asSlice(routing["rules"]) {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if tag, _ := rule[fromKey].(string); tag != from {
			continue
		}

		delete(rule, fromKey)
		rule[toKey] = to
		changed++
	}

	return changed
}

// enableBalancerRouting installs the balancer and points the proxy rules at it.
func enableBalancerRouting(doc *routingDoc, balancerTag, selector string, proxyTags []string) error {
	changed := 0
	for _, tag := range proxyTags {
		changed += retargetRules(doc.routing, tag, balancerTag, true)
	}
	if changed == 0 {
		return fmt.Errorf("в %s нет правил, ведущих на прокси-outbound — маршрутизацию нужно настроить вручную", doc.path)
	}

	balancers := asSlice(doc.routing["balancers"])
	replaced := false
	for i, raw := range balancers {
		if b, ok := raw.(map[string]interface{}); ok {
			if tag, _ := b["tag"].(string); tag == balancerTag {
				balancers[i] = balancerBlock(balancerTag, selector)
				replaced = true
				break
			}
		}
	}
	if !replaced {
		balancers = append(balancers, balancerBlock(balancerTag, selector))
	}
	doc.routing["balancers"] = balancers

	// observatory is a top-level key, not part of routing
	if _, exists := doc.config["observatory"]; !exists {
		doc.config["observatory"] = observatoryBlock(selector)
	}

	doc.config["routing"] = doc.routing

	return nil
}

// disableBalancerRouting removes the balancer and sends the rules back to a
// single outbound.
func disableBalancerRouting(doc *routingDoc, balancerTag, outboundTag string) error {
	if changed := retargetRules(doc.routing, balancerTag, outboundTag, false); changed == 0 {
		return fmt.Errorf("в %s нет правил, ведущих на балансировщик %q", doc.path, balancerTag)
	}

	var kept []interface{}
	for _, raw := range asSlice(doc.routing["balancers"]) {
		if b, ok := raw.(map[string]interface{}); ok {
			if tag, _ := b["tag"].(string); tag == balancerTag {
				continue
			}
		}
		kept = append(kept, raw)
	}
	if len(kept) == 0 {
		delete(doc.routing, "balancers")
		delete(doc.config, "observatory")
	} else {
		doc.routing["balancers"] = kept
	}

	doc.config["routing"] = doc.routing

	return nil
}
