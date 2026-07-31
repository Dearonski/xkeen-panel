package xkeen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"xkeen-panel/internal/models"
)

// DefaultBalancerTag and DefaultPoolSelector are what the panel writes when it
// builds a pool itself. An existing balancer keeps its own tag and selector.
const (
	DefaultBalancerTag  = "balancer"
	DefaultPoolSelector = "sub-"
	defaultProbeURL     = "https://www.google.com/generate_204"
	defaultProbeEvery   = "5m"
)

// BuildPoolOutbounds renders one outbound per server, tagged <selector><N>.
//
// The template is the outbound being replaced: its streamSettings.sockopt is
// copied onto every node, because XKeen validates sockopt.mark on each of them
// and a pool built without it would fail the check for the whole pool at once.
func BuildPoolOutbounds(servers []models.Server, selector string, template map[string]interface{}) ([]interface{}, error) {
	if selector == "" {
		selector = DefaultPoolSelector
	}

	format := detectOutboundFormat(template)

	var nodes []interface{}
	for _, server := range servers {
		if server.RawURI == "" || (server.Protocol != "" && server.Protocol != "vless") {
			continue
		}

		params, err := ParseVLESS(server.RawURI)
		if err != nil {
			continue
		}

		tag := selector + strconv.Itoa(len(nodes)+1)
		nodes = append(nodes, mergeOutbound(template, buildOutboundFromURI(params, tag, format)))
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("нет VLESS-серверов для пула")
	}

	return nodes, nil
}

// PoolMatchesSubscription reports whether the pool in the config already covers
// exactly the subscription's VLESS servers, in the same order.
//
// Callers check this before syncing: writing the pool means restarting the core,
// which drops live connections, so an unchanged subscription must be a no-op.
func PoolMatchesSubscription(outboundsPath string, servers []models.Server, selector string) (bool, error) {
	if selector == "" {
		selector = DefaultPoolSelector
	}

	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return false, err
	}

	var current [][3]interface{}
	for _, raw := range asSlice(config["outbounds"]) {
		ob, ok := raw.(map[string]interface{})
		if !ok || isServiceOutbound(ob) {
			continue
		}
		if tag, _ := ob["tag"].(string); !strings.Contains(tag, selector) {
			continue
		}
		address, port, uuid, ok := readProxyEndpoint(ob)
		if !ok {
			continue
		}
		current = append(current, [3]interface{}{address, port, uuid})
	}

	var wanted [][3]interface{}
	for _, server := range servers {
		if server.RawURI == "" || (server.Protocol != "" && server.Protocol != "vless") {
			continue
		}
		params, err := ParseVLESS(server.RawURI)
		if err != nil {
			continue
		}
		wanted = append(wanted, [3]interface{}{params.Address, params.Port, params.UUID})
	}

	if len(current) != len(wanted) {
		return false, nil
	}
	for i := range current {
		if current[i] != wanted[i] {
			return false, nil
		}
	}

	return true, nil
}

// PoolNodeTags returns the tags BuildPoolOutbounds produced.
func PoolNodeTags(nodes []interface{}) []string {
	tags := make([]string, 0, len(nodes))
	for _, raw := range nodes {
		if ob, ok := raw.(map[string]interface{}); ok {
			if tag, _ := ob["tag"].(string); tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}

// balancerBlock is the routing.balancers entry the panel writes. leastPing is
// the only strategy that resolves to a single node, which both the failover
// story and XKeen's own speed balancer depend on.
func balancerBlock(tag, selector string) map[string]interface{} {
	return map[string]interface{}{
		"tag":      tag,
		"selector": []interface{}{selector},
		"strategy": map[string]interface{}{"type": "leastPing"},
	}
}

// apiConfigDoc builds the gRPC API config that OverrideBalancerTarget needs.
//
// In the fork this block is installed by `xkeen -sb on`, which only exists on
// the Beta channel — on Stable the panel has to provide it itself, or pinning a
// node by hand is impossible.
//
// The inbound deliberately omits `settings.followRedirect`: XKeen scans
// dokodemo-door inbounds that have it to derive the transparent proxy ports, so
// an api inbound carrying it would be mistaken for a redirect entry point.
func apiConfigDoc(apiAddr string) (map[string]interface{}, error) {
	host, port, err := splitAPIAddr(apiAddr)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"api": map[string]interface{}{
			"tag":      "api",
			"services": []interface{}{"RoutingService"},
		},
		"inbounds": []interface{}{
			map[string]interface{}{
				"tag":      "api-in",
				"listen":   host,
				"port":     port,
				"protocol": "dokodemo-door",
				"settings": map[string]interface{}{"address": host},
			},
		},
		"routing": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"type":        "field",
					"inboundTag":  []interface{}{"api-in"},
					"outboundTag": "api",
				},
			},
		},
	}, nil
}

func splitAPIAddr(addr string) (string, int, error) {
	host, portText, found := strings.Cut(addr, ":")
	if !found || host == "" {
		return "", 0, fmt.Errorf("некорректный адрес api %q, ожидается host:port", addr)
	}

	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("некорректный порт api в %q", addr)
	}

	return host, port, nil
}

// observatoryBlock enables the probing leastPing needs — without it the strategy
// has no latency data to rank nodes by.
func observatoryBlock(selector string) map[string]interface{} {
	return map[string]interface{}{
		"subjectSelector": []interface{}{selector},
		"probeURL":        defaultProbeURL,
		"probeInterval":   defaultProbeEvery,
	}
}

// OverrideBalancerTarget pins the balancer to one node through the gRPC API.
//
// The override has no TTL and does not survive an Xray restart, so callers that
// want a durable choice must re-apply it after one.
func OverrideBalancerTarget(rt Runtime, apiAddr, balancerTag, outboundTag string) error {
	if _, err := xrayAPI(rt, "bo", "-s", apiAddr, "-b", balancerTag, outboundTag); err != nil {
		return fmt.Errorf("не удалось закрепить ноду %s: %w", outboundTag, err)
	}
	return nil
}

// CurrentBalancerTarget reports the node traffic actually goes through.
func CurrentBalancerTarget(rt Runtime, apiAddr, balancerTag string) (string, error) {
	out, err := xrayAPI(rt, "bi", "-s", apiAddr, balancerTag)
	if err != nil {
		return "", fmt.Errorf("api балансировщика недоступен: %w", err)
	}
	return parseBalancerInfo(out), nil
}

// parseBalancerInfo reads `xray api bi` output. It prints two sections and the
// override wins: after `bo` the traffic goes through the forced node, not the
// leastPing pick, so reporting Selects would name the wrong one.
//
//	Selecting Override:
//	    1   sub-2
//	Selects:
//	    1   sub-1
func parseBalancerInfo(out string) string {
	const (
		sectionNone = iota
		sectionOverride
		sectionSelects
	)

	section := sectionNone
	override, selects := "", ""

	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.Contains(line, "Selecting Override:"):
			section = sectionOverride
			continue
		case strings.Contains(line, "Selects:"):
			section = sectionSelects
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		tag := fields[1]

		if section == sectionOverride && override == "" {
			override = tag
		}
		if section == sectionSelects && selects == "" {
			selects = tag
		}
	}

	if override != "" {
		return override
	}

	return selects
}

// BalancerAPIAvailable reports whether the gRPC API answers. XKeen only adds the
// api block when its speed balancer is switched on, so it is usually absent.
func BalancerAPIAvailable(rt Runtime, apiAddr, balancerTag string) bool {
	_, err := xrayAPI(rt, "bi", "-s", apiAddr, balancerTag)
	return err == nil
}

func xrayAPI(rt Runtime, args ...string) (string, error) {
	if rt.Core != CoreXray {
		return "", fmt.Errorf("api балансировщика доступно только для xray, активно ядро %s", rt.Core)
	}
	if _, err := os.Stat(rt.CoreBin); err != nil {
		return "", fmt.Errorf("бинарь xray не найден (%s)", rt.CoreBin)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, rt.CoreBin, append([]string{"api"}, args...)...).CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("%w (%s)", err, TailLines(text, 2))
	}

	return text, nil
}
