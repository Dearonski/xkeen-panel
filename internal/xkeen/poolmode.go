package xkeen

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"xkeen-panel/internal/models"
)

// apiConfigFile is read before the routing files, so the api rule lands first in
// the merged rule list — where it has to be to catch its own inbound.
const apiConfigFile = "00_api.json"

// PoolOptions tunes pool creation. Zero values mean the panel's defaults, and an
// empty APIAddr skips installing the gRPC api block.
type PoolOptions struct {
	BalancerTag string
	Selector    string
	APIAddr     string
}

// EnablePool converts a single-upstream config into a balancer pool built from
// the subscription: one outbound per server, a leastPing balancer over them, and
// the routing rules retargeted from the outbound to the balancer.
//
// Xray then picks the node itself and drops a dead one without a restart, which
// is what the watchdog otherwise emulates by rewriting the config.
//
// Both files are rewritten from the parsed tree, so comments in them are lost —
// callers must confirm with the owner first. Backups are kept next to each file.
func EnablePool(rt Runtime, outboundsPath string, servers []models.Server, opts PoolOptions) (PoolState, error) {
	balancerTag := opts.BalancerTag
	if balancerTag == "" {
		balancerTag = DefaultBalancerTag
	}
	selector := opts.Selector
	if selector == "" {
		selector = DefaultPoolSelector
	}

	state := PoolState{}

	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return state, err
	}

	outbounds, ok := config["outbounds"].([]interface{})
	if !ok {
		return state, fmt.Errorf("outbounds не найдены в %s", outboundsPath)
	}

	_, template := findProxyOutbound(outbounds)
	if template == nil {
		return state, fmt.Errorf("в конфиге нет прокси-outbound, не с чего начинать пул")
	}
	originalTag := outboundTag(template)

	nodes, err := BuildPoolOutbounds(servers, selector, template)
	if err != nil {
		return state, err
	}

	proxyTags := proxyOutboundTags(outbounds)
	doc, err := findRoutingDoc(rt, ruleTargetsOutbound(proxyTags))
	if err != nil {
		return state, err
	}
	if err := enableBalancerRouting(doc, balancerTag, selector, proxyTags); err != nil {
		return state, err
	}

	config["outbounds"] = append(nodes, serviceOutbounds(outbounds)...)

	writes := map[string]map[string]interface{}{
		outboundsPath: config,
		doc.path:      doc.config,
	}

	// Пул без api живёт своей жизнью: ноду выбирает leastPing, а закрепить её
	// вручную нельзя. Чужой блок не трогаем — его мог поставить `xkeen -sb on`.
	apiPath := filepath.Join(rt.XrayConfDir, apiConfigFile)
	apiCreated := false
	if opts.APIAddr != "" {
		if _, err := os.Stat(apiPath); os.IsNotExist(err) {
			apiDoc, err := apiConfigDoc(opts.APIAddr)
			if err != nil {
				return state, err
			}
			writes[apiPath] = apiDoc
			apiCreated = true
		}
	}

	if err := applyConfigs(rt, writes); err != nil {
		if apiCreated {
			os.Remove(apiPath)
		}
		return state, err
	}

	return PoolState{
		Enabled:     true,
		BalancerTag: balancerTag,
		Selector:    selector,
		OriginalTag: originalTag,
		RoutingFile: doc.path,
		APIFile:     apiFileIfCreated(apiPath, apiCreated),
	}, nil
}

// apiFileIfCreated records the api config only when the panel wrote it, so
// leaving pool mode never deletes a block the owner or XKeen installed.
func apiFileIfCreated(path string, created bool) string {
	if created {
		return path
	}
	return ""
}

// DisablePool collapses the pool back to a single outbound carrying server.
func DisablePool(rt Runtime, outboundsPath string, server *models.Server, state PoolState) error {
	if server == nil || server.RawURI == "" {
		return fmt.Errorf("нужен активный сервер, чтобы вернуться к одиночному outbound")
	}

	balancerTag := state.BalancerTag
	if balancerTag == "" {
		balancerTag = DefaultBalancerTag
	}
	tag := state.OriginalTag
	if tag == "" {
		tag = defaultProxyTag
	}

	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return err
	}

	outbounds, ok := config["outbounds"].([]interface{})
	if !ok {
		return fmt.Errorf("outbounds не найдены в %s", outboundsPath)
	}

	params, err := parseVLESSURI(server.RawURI)
	if err != nil {
		return fmt.Errorf("ошибка парсинга URI: %w", err)
	}

	_, template := findProxyOutbound(outbounds)
	single := mergeOutbound(template, buildOutboundFromURI(params, tag, detectOutboundFormat(template)))
	config["outbounds"] = append([]interface{}{single}, serviceOutbounds(outbounds)...)

	doc, err := findRoutingDoc(rt, ruleTargetsBalancer(balancerTag))
	if err != nil {
		return err
	}
	if err := disableBalancerRouting(doc, balancerTag, tag); err != nil {
		return err
	}

	if err := applyConfigs(rt, map[string]map[string]interface{}{
		outboundsPath: config,
		doc.path:      doc.config,
	}); err != nil {
		return err
	}

	// Убираем только тот api-блок, который создали сами: чужой мог поставить
	// `xkeen -sb on`, и его удаление сломало бы балансировку по скорости
	if state.APIFile != "" {
		if err := os.Remove(state.APIFile); err != nil && !os.IsNotExist(err) {
			log.Printf("[POOL] Не удалось удалить %s: %v", state.APIFile, err)
		}
	}

	return nil
}

// SyncPool refreshes pool membership from the subscription without touching
// routing — the balancer selects by tag prefix, so the tags stay stable.
func SyncPool(rt Runtime, outboundsPath string, servers []models.Server, state PoolState) error {
	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return err
	}

	outbounds, ok := config["outbounds"].([]interface{})
	if !ok {
		return fmt.Errorf("outbounds не найдены в %s", outboundsPath)
	}

	_, template := findProxyOutbound(outbounds)
	nodes, err := BuildPoolOutbounds(servers, state.Selector, template)
	if err != nil {
		return err
	}

	config["outbounds"] = append(nodes, serviceOutbounds(outbounds)...)

	return applyConfigs(rt, map[string]map[string]interface{}{outboundsPath: config})
}

// applyConfigs writes several config files and validates them together, since
// the core loads the directory as one config. A rejected result rolls every
// file back — a partial apply is what leaves XKeen unable to start.
func applyConfigs(rt Runtime, writes map[string]map[string]interface{}) error {
	written := make([]string, 0, len(writes))

	for path, config := range writes {
		if err := WriteOutboundsConfig(path, config); err != nil {
			rollback(written)
			return err
		}
		written = append(written, path)
	}

	if !rt.Installed || rt.Dispatcher == "" {
		return nil
	}

	output, err := TestConfig(rt.Dispatcher, rt.Core)
	if err == nil {
		return nil
	}

	rollback(written)

	return fmt.Errorf("конфигурация %s не прошла проверку, изменения отменены: %s", rt.Core, TailLines(output, 4))
}

func rollback(paths []string) {
	for _, path := range paths {
		if err := RestoreBackup(path); err != nil {
			log.Printf("[POOL] Откат %s не удался: %v", path, err)
		}
	}
}

// serviceOutbounds keeps direct/block and friends in their original order.
func serviceOutbounds(outbounds []interface{}) []interface{} {
	var kept []interface{}
	for _, raw := range outbounds {
		if ob, ok := raw.(map[string]interface{}); ok && isServiceOutbound(ob) {
			kept = append(kept, raw)
		}
	}
	return kept
}

func proxyOutboundTags(outbounds []interface{}) []string {
	var tags []string
	for _, raw := range outbounds {
		if ob, ok := raw.(map[string]interface{}); ok && !isServiceOutbound(ob) {
			if tag, _ := ob["tag"].(string); tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return tags
}
