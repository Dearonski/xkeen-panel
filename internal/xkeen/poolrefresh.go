package xkeen

import (
	"fmt"
	"log"

	"xkeen-panel/internal/models"
)

// SyncResult reports what a pool refresh did.
type SyncResult struct {
	Changed   bool     `json:"changed"`
	Added     []string `json:"added,omitempty"`
	Removed   []string `json:"removed,omitempty"`
	Restarted bool     `json:"restarted"`
	Live      bool     `json:"live"` // applied through the API, no connections dropped
}

// RefreshPool brings the pool in line with the subscription.
//
// This is what keeps a pool honest: nodes are generated from subscription URIs,
// so once the provider rotates a server the pool points at an endpoint that no
// longer answers — and if every node goes stale at once, the balancer has no
// healthy target left and traffic stops.
//
// Nothing happens when the pool already matches: rewriting it would mean a
// restart, and a restart drops live connections.
//
// The new set is applied through the core API when possible (no restart at all)
// and falls back to restarting when the API is unavailable — the api block only
// exists if the panel or `xkeen -sb on` installed it.
func RefreshPool(rt Runtime, outboundsPath, apiAddr string, servers []models.Server, state PoolState) (SyncResult, error) {
	result := SyncResult{}

	selector := state.Selector
	if selector == "" {
		selector = DefaultPoolSelector
	}

	matches, err := PoolMatchesSubscription(outboundsPath, servers, selector)
	if err != nil {
		return result, err
	}
	if matches {
		return result, nil
	}

	before, err := poolTagsInFile(outboundsPath, selector)
	if err != nil {
		return result, err
	}

	if err := SyncPool(rt, outboundsPath, servers, state); err != nil {
		return result, err
	}

	after, err := poolTagsInFile(outboundsPath, selector)
	if err != nil {
		return result, err
	}

	result.Changed = true
	result.Added = missingFrom(after, before)
	result.Removed = missingFrom(before, after)

	// A pool built by an older panel has an api block without HandlerService, so
	// the hot path would fail every time until the file is upgraded
	upgraded, err := ensureHandlerService(state.APIFile)
	if err != nil {
		log.Printf("[POOL] Не удалось обновить api-блок: %v", err)
	}
	if upgraded {
		log.Printf("[POOL] В api-блок добавлен HandlerService — применяю перезапуском")
	}

	// Tags are positional (sub-1, sub-2, …), so a server swapped in place keeps
	// its tag while its endpoint changes. Such a node has to be replaced in the
	// running core too, or it would keep the old address until the next restart.
	replaced := intersect(before, after)

	// The live path is attempted only for an api block the panel wrote, because
	// only then is HandlerService guaranteed present. Otherwise the removals
	// would land and the additions would fail, leaving the core with no nodes.
	if !upgraded && state.APIFile != "" {
		if err := applyPoolLive(rt, apiAddr, outboundsPath, selector, replaced, result.Removed); err == nil {
			result.Live = true
			return result, nil
		} else {
			log.Printf("[POOL] Горячее обновление недоступно (%v) — перезапускаю ядро", err)
		}
	}

	if _, err := Restart(rt.Dispatcher); err != nil {
		return result, fmt.Errorf("пул обновлён, но перезапуск не выполнен: %w", err)
	}
	result.Restarted = true

	return result, nil
}

// ensureHandlerService adds HandlerService to an api block the panel created.
// Only our own file is touched: a block from `xkeen -sb on` belongs to XKeen.
func ensureHandlerService(apiPath string) (bool, error) {
	if apiPath == "" {
		return false, nil
	}

	var cfg map[string]interface{}
	if err := ReadJSONC(apiPath, &cfg); err != nil {
		return false, err
	}

	api, ok := cfg["api"].(map[string]interface{})
	if !ok {
		return false, nil
	}

	for _, service := range asSlice(api["services"]) {
		if name, _ := service.(string); name == "HandlerService" {
			return false, nil
		}
	}

	api["services"] = append(asSlice(api["services"]), "HandlerService")
	cfg["api"] = api

	if err := WriteOutboundsConfig(apiPath, cfg); err != nil {
		return false, err
	}

	return true, nil
}

// applyPoolLive pushes the new pool into the running core: every tag that stays
// is removed and re-added (its endpoint may have changed), and tags that are
// gone are dropped.
func applyPoolLive(rt Runtime, apiAddr, outboundsPath, selector string, replaced, removed []string) error {
	if rt.Core != CoreXray {
		return fmt.Errorf("горячее обновление доступно только для xray")
	}
	if apiAddr == "" {
		return fmt.Errorf("адрес api не задан")
	}

	nodes, err := poolNodesInFile(outboundsPath, selector)
	if err != nil {
		return err
	}

	// Remove first: adding an outbound whose tag is already registered fails
	if err := RemoveOutbounds(rt, apiAddr, append(append([]string{}, removed...), replaced...)); err != nil {
		return err
	}

	var toAdd []interface{}
	for _, node := range nodes {
		toAdd = append(toAdd, node)
	}

	if err := AddOutbounds(rt, apiAddr, toAdd); err != nil {
		// The core is now missing the outbounds just removed — only a restart
		// can put the config back in charge
		return fmt.Errorf("%w (ядро осталось без части нод, нужен перезапуск)", err)
	}

	return nil
}

// poolNodesInFile returns the pool outbounds currently written to the config.
func poolNodesInFile(path, selector string) ([]map[string]interface{}, error) {
	config, err := ReadOutboundsConfig(path)
	if err != nil {
		return nil, err
	}

	var nodes []map[string]interface{}
	for _, raw := range asSlice(config["outbounds"]) {
		ob, ok := raw.(map[string]interface{})
		if !ok || isServiceOutbound(ob) {
			continue
		}
		if tag, _ := ob["tag"].(string); tag != "" && containsSelector(tag, selector) {
			nodes = append(nodes, ob)
		}
	}

	return nodes, nil
}

func poolTagsInFile(path, selector string) ([]string, error) {
	nodes, err := poolNodesInFile(path, selector)
	if err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(nodes))
	for _, node := range nodes {
		tag, _ := node["tag"].(string)
		tags = append(tags, tag)
	}

	return tags, nil
}

func containsSelector(tag, selector string) bool {
	return selector != "" && len(tag) >= len(selector) && tag[:len(selector)] == selector
}

func missingFrom(want, have []string) []string {
	present := make(map[string]bool, len(have))
	for _, tag := range have {
		present[tag] = true
	}

	var missing []string
	for _, tag := range want {
		if !present[tag] {
			missing = append(missing, tag)
		}
	}

	return missing
}

func intersect(a, b []string) []string {
	inB := make(map[string]bool, len(b))
	for _, tag := range b {
		inB[tag] = true
	}

	var both []string
	for _, tag := range a {
		if inB[tag] {
			both = append(both, tag)
		}
	}

	return both
}
