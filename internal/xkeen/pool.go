package xkeen

import (
	"context"
	"encoding/json"
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
	// A dead node stays selectable until the next probe, so this is how long a
	// pool can keep sending traffic into a hole
	defaultProbeEvery = "1m"
)

// BuildPoolOutbounds renders one outbound per server, tagged <selector><N>.
//
// The template is the outbound being replaced: its streamSettings.sockopt is
// copied onto every node, because XKeen validates sockopt.mark on each of them
// and a pool built without it would fail the check for the whole pool at once.
//
// existing is the pool already in the config; servers found there keep the tag
// they already have, so a refresh only touches what genuinely changed.
func BuildPoolOutbounds(servers []models.Server, selector string, template map[string]interface{}, existing PoolLayout) ([]interface{}, error) {
	if selector == "" {
		selector = DefaultPoolSelector
	}

	format := detectOutboundFormat(template)

	var nodes []interface{}
	for _, entry := range assignTags(servers, selector, existing) {
		params, err := ParseVLESS(entry.Server.RawURI)
		if err != nil {
			continue
		}
		nodes = append(nodes, mergeOutbound(template, buildOutboundFromURI(params, entry.Tag, format)))
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("нет VLESS-серверов для пула")
	}

	return nodes, nil
}

// PoolMatchesSubscription reports whether the pool already covers exactly the
// wanted servers, as a SET.
//
// Order deliberately does not count. Candidates are ranked by measured latency,
// which jitters between runs, so comparing sequences reported a drift on nearly
// every refresh — and each such "drift" rewrote the whole pool and tore down
// every live connection through it.
func PoolMatchesSubscription(outboundsPath string, servers []models.Server, selector string) (bool, error) {
	layout, err := ReadPoolLayout(outboundsPath, selector)
	if err != nil {
		return false, err
	}

	current := layout.Endpoints()

	wanted := map[endpoint]bool{}
	for _, server := range servers {
		if ep, ok := endpointOfServer(server); ok {
			wanted[ep] = true
		}
	}

	if len(current) != len(wanted) {
		return false, nil
	}
	for ep := range wanted {
		if !current[ep] {
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

// apiServices are the gRPC services the panel needs: RoutingService pins a
// balancer node, HandlerService swaps outbounds in a running core so a pool can
// be refreshed without dropping connections.
var apiServices = []interface{}{"RoutingService", "HandlerService"}

// apiConfigDoc builds the gRPC API config that OverrideBalancerTarget needs.
//
// In the fork this block is installed by `xkeen -sb on`, which only exists on
// the Beta channel — on Stable the panel has to provide it itself, or pinning a
// node by hand is impossible.
//
// `api.listen` makes Xray bind the port itself. The older recipe — a
// dokodemo-door inbound plus a routing rule pointing at the api tag — depends on
// rule ordering across merged config files and adds an inbound that XKeen scans
// when deriving the transparent proxy ports. Xray reports the outcome as
// "API server listening on …", so a failure is visible in its log.
func apiConfigDoc(apiAddr string) (map[string]interface{}, error) {
	if _, _, err := splitAPIAddr(apiAddr); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"api": map[string]interface{}{
			"tag":      "api",
			"listen":   apiAddr,
			"services": apiServices,
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
//
// The interval scales with pool size: observatory probes every node separately,
// so a minute-long cycle over a large pool means constant handshakes on the
// router.
func observatoryBlock(selector string, nodes int) map[string]interface{} {
	return map[string]interface{}{
		"subjectSelector": []interface{}{selector},
		"probeURL":        defaultProbeURL,
		"probeInterval":   probeIntervalFor(nodes),
	}
}

func probeIntervalFor(nodes int) string {
	switch {
	case nodes <= 20:
		return defaultProbeEvery
	case nodes <= 40:
		return "3m"
	default:
		return "5m"
	}
}

// AddOutbounds installs outbounds into the running core through the gRPC API,
// so a pool can be updated without a restart. Requires HandlerService in the api
// block; callers must be ready to fall back to a restart.
func AddOutbounds(rt Runtime, apiAddr string, outbounds []interface{}) error {
	if len(outbounds) == 0 {
		return nil
	}

	data, err := json.MarshalIndent(map[string]interface{}{"outbounds": outbounds}, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка сериализации outbound'ов: %w", err)
	}

	file, err := os.CreateTemp("", "xkeen-panel-outbounds-*.json")
	if err != nil {
		return fmt.Errorf("не удалось создать временный файл: %w", err)
	}
	defer os.Remove(file.Name())

	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("ошибка записи %s: %w", file.Name(), err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("ошибка закрытия %s: %w", file.Name(), err)
	}

	if _, err := xrayAPI(rt, "ado", "-s", apiAddr, file.Name()); err != nil {
		return fmt.Errorf("не удалось добавить outbound'ы через api: %w", err)
	}

	return nil
}

// RemoveOutbounds drops outbounds from the running core by tag.
func RemoveOutbounds(rt Runtime, apiAddr string, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	if _, err := xrayAPI(rt, append([]string{"rmo", "-s", apiAddr}, tags...)...); err != nil {
		return fmt.Errorf("не удалось удалить outbound'ы через api: %w", err)
	}

	return nil
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

// BalancerInfo separates the forced override from what the strategy would pick.
//
// The distinction is load-bearing: "leastPing currently ranks sub-2 first" and
// "traffic is pinned to sub-2" look identical from the outside but behave
// completely differently — without an override the balancer still chooses per
// connection, so the exit IP keeps moving.
type BalancerInfo struct {
	Override string // tag forced through `bo`, empty when none is set
	Selects  string // what the strategy ranks first
}

// Effective is the tag traffic actually goes through.
func (b BalancerInfo) Effective() string {
	if b.Override != "" {
		return b.Override
	}
	return b.Selects
}

// BalancerStatus reads the balancer's current state.
func BalancerStatus(rt Runtime, apiAddr, balancerTag string) (BalancerInfo, error) {
	out, err := xrayAPI(rt, "bi", "-s", apiAddr, balancerTag)
	if err != nil {
		return BalancerInfo{}, fmt.Errorf("api балансировщика недоступен: %w", err)
	}
	return parseBalancerInfo(out), nil
}

// CurrentBalancerTarget reports the node traffic actually goes through.
func CurrentBalancerTarget(rt Runtime, apiAddr, balancerTag string) (string, error) {
	info, err := BalancerStatus(rt, apiAddr, balancerTag)
	if err != nil {
		return "", err
	}
	return info.Effective(), nil
}

// parseBalancerInfo reads `xray api bi` output, keeping the two sections apart:
//
//	Selecting Override:
//	    1   sub-2
//	Selects:
//	    1   sub-1
//
// An empty Override section means nothing is pinned, however plausible the
// Selects entry looks.
func parseBalancerInfo(out string) BalancerInfo {
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

	return BalancerInfo{Override: override, Selects: selects}
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
