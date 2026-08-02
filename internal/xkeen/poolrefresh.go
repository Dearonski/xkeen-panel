package xkeen

import (
	"fmt"
	"log"
	"sort"

	"xkeen-panel/internal/models"
)

// SyncResult reports what a pool refresh did.
type SyncResult struct {
	Changed   bool     `json:"changed"`
	Added     []string `json:"added,omitempty"`
	Removed   []string `json:"removed,omitempty"`
	Replaced  []string `json:"replaced,omitempty"`
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
func RefreshPool(rt Runtime, outboundsPath, apiAddr string, servers []models.Server, state PoolState, sel PoolSelection) (SyncResult, error) {
	result := SyncResult{}

	selector := state.Selector
	if selector == "" {
		selector = DefaultPoolSelector
	}

	current, err := ReadPoolLayout(outboundsPath, selector)
	if err != nil {
		return result, err
	}
	wanted := SelectPoolServers(servers, sel.WithCurrentPool(current))

	matches, err := PoolMatchesSubscription(outboundsPath, wanted, selector)
	if err != nil {
		return result, err
	}
	if matches {
		return result, nil
	}

	before := current

	if err := SyncPool(rt, outboundsPath, wanted, state, PoolSelection{MaxNodes: len(wanted)}); err != nil {
		return result, err
	}

	after, err := ReadPoolLayout(outboundsPath, selector)
	if err != nil {
		return result, err
	}

	result.Changed = true
	result.Added = tagsMissingFrom(after, before)
	result.Removed = tagsMissingFrom(before, after)

	// A pool built by an older panel has an api block that never listened, so the
	// hot path would fail every time until the file is migrated
	upgraded, err := ensureAPIConfig(state.APIFile, apiAddr)
	if err != nil {
		log.Printf("[POOL] Не удалось обновить api-блок: %v", err)
	}
	if upgraded {
		log.Printf("[POOL] api-блок приведён к текущей форме — применяю перезапуском")
	}

	// Probe load scales with pool size, so a pool that shrank or grew may need a
	// different interval. This lives in the routing file, which the API cannot
	// reload — changing it forces the restart path.
	observatoryChanged, err := updateObservatoryInterval(rt, selector, len(wanted))
	if err != nil {
		log.Printf("[POOL] Не удалось обновить observatory: %v", err)
	}

	// Only the tags whose endpoint actually changed need touching in the running
	// core. Treating every surviving tag as replaced tore down all ten outbounds
	// on each refresh, and with them every connection they carried.
	result.Replaced = changedTags(before, after)

	// The live path is attempted only for an api block the panel wrote, because
	// only then is HandlerService guaranteed present. Otherwise the removals
	// would land and the additions would fail, leaving the core with no nodes.
	if !upgraded && !observatoryChanged && state.APIFile != "" {
		if err := applyPoolLive(rt, apiAddr, outboundsPath, selector, result.Added, result.Replaced, result.Removed); err == nil {
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

// updateObservatoryInterval keeps the probe interval in step with pool size.
func updateObservatoryInterval(rt Runtime, selector string, nodes int) (bool, error) {
	doc, err := findRoutingDoc(rt, func(map[string]interface{}) bool { return false })
	if err != nil {
		return false, err
	}

	observatory, ok := doc.config["observatory"].(map[string]interface{})
	if !ok {
		return false, nil
	}

	want := probeIntervalFor(nodes)
	if current, _ := observatory["probeInterval"].(string); current == want {
		return false, nil
	}

	observatory["probeInterval"] = want
	doc.config["observatory"] = observatory

	if err := WriteOutboundsConfig(doc.path, doc.config); err != nil {
		return false, err
	}

	return true, nil
}

// EnsureAPIConfig migrates the api block on startup.
//
// A pool whose subscription has not drifted never reaches the refresh path, so
// an install carrying the old non-listening api block would keep failing to pin
// forever. Returns true when the file changed and the core has to be restarted.
func EnsureAPIConfig(state PoolState, apiAddr string) (bool, error) {
	if !state.Enabled {
		return false, nil
	}
	return ensureAPIConfig(state.APIFile, apiAddr)
}

// ensureAPIConfig brings an api block the panel created up to the current shape.
// Only our own file is touched: a block from `xkeen -sb on` belongs to XKeen.
//
// Two migrations happen here. Early pools asked only for RoutingService, which
// is not enough to swap outbounds in a running core. And the original recipe
// bound the port through a dokodemo-door inbound plus a routing rule, which did
// not actually listen — `api.listen` replaces both.
func ensureAPIConfig(apiPath, apiAddr string) (bool, error) {
	if apiPath == "" || apiAddr == "" {
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

	changed := false

	if listen, _ := api["listen"].(string); listen != apiAddr {
		api["listen"] = apiAddr
		changed = true
	}

	if !hasService(api["services"], "HandlerService") {
		api["services"] = append(asSlice(api["services"]), "HandlerService")
		changed = true
	}

	// The inbound and its rule only existed to reach the api tag; with a direct
	// listener they are dead weight, and the inbound confuses XKeen's port scan
	if _, present := cfg["inbounds"]; present {
		delete(cfg, "inbounds")
		delete(cfg, "routing")
		changed = true
	}

	if !changed {
		return false, nil
	}

	cfg["api"] = api
	if err := WriteOutboundsConfig(apiPath, cfg); err != nil {
		return false, err
	}

	return true, nil
}

func hasService(services interface{}, want string) bool {
	for _, service := range asSlice(services) {
		if name, _ := service.(string); name == want {
			return true
		}
	}
	return false
}

// applyPoolLive pushes only the difference into the running core: tags that are
// gone are dropped, tags whose endpoint changed are replaced, new tags added.
//
// Untouched tags are deliberately left alone — removing an outbound closes the
// connections riding on it, which is what a refresh must avoid.
func applyPoolLive(rt Runtime, apiAddr, outboundsPath, selector string, added, replaced, removed []string) error {
	if rt.Core != CoreXray {
		return fmt.Errorf("горячее обновление доступно только для xray")
	}
	if apiAddr == "" {
		return fmt.Errorf("адрес api не задан")
	}
	if len(added)+len(replaced)+len(removed) == 0 {
		return nil
	}

	// Remove first: adding an outbound whose tag is already registered fails.
	// Only tags the core already knows are removed — `rmo` on an unknown tag errors.
	if err := RemoveOutbounds(rt, apiAddr, concat(removed, replaced)); err != nil {
		return err
	}

	toAdd, err := poolNodesWithTags(outboundsPath, selector, concat(added, replaced))
	if err != nil {
		return err
	}

	if err := AddOutbounds(rt, apiAddr, toAdd); err != nil {
		// The core is now missing the outbounds just removed — only a restart
		// can put the config back in charge
		return fmt.Errorf("%w (ядро осталось без части нод, нужен перезапуск)", err)
	}

	return nil
}

// poolNodesWithTags returns the config's outbounds for the given tags.
func poolNodesWithTags(path, selector string, tags []string) ([]interface{}, error) {
	if len(tags) == 0 {
		return nil, nil
	}

	wanted := make(map[string]bool, len(tags))
	for _, tag := range tags {
		wanted[tag] = true
	}

	nodes, err := poolNodesInFile(path, selector)
	if err != nil {
		return nil, err
	}

	var picked []interface{}
	for _, node := range nodes {
		if tag, _ := node["tag"].(string); wanted[tag] {
			picked = append(picked, node)
		}
	}

	return picked, nil
}

// tagsMissingFrom lists tags present in a but not in b.
func tagsMissingFrom(a, b PoolLayout) []string {
	var missing []string
	for tag := range a {
		if _, present := b[tag]; !present {
			missing = append(missing, tag)
		}
	}
	sort.Strings(missing)
	return missing
}

// changedTags lists tags present in both layouts whose endpoint differs.
func changedTags(before, after PoolLayout) []string {
	var changed []string
	for tag, oldEP := range before {
		if newEP, present := after[tag]; present && newEP != oldEP {
			changed = append(changed, tag)
		}
	}
	sort.Strings(changed)
	return changed
}

func concat(lists ...[]string) []string {
	var all []string
	for _, list := range lists {
		all = append(all, list...)
	}
	return all
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
