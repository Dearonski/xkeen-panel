package xkeen

import (
	"os"
	"path/filepath"
	"strings"
)

// Topology describes how routing picks the upstream node.
//
// Xray merges every *.json in its config directory, so outbounds and balancers
// can live in any of them — the whole directory is scanned, exactly like the
// fork's own validator does.
type Topology struct {
	Mode        string   `json:"mode"` // single | pool
	BalancerTag string   `json:"balancer_tag,omitempty"`
	Selectors   []string `json:"-"`
	PoolTags    []string `json:"pool_tags,omitempty"`
	ProxyTags   []string `json:"proxy_tags,omitempty"`
}

const (
	TopologySingle = "single"
	TopologyPool   = "pool"
)

// ReadTopology inspects the core config directory. A missing or unreadable
// directory yields a single-node topology rather than an error: the panel still
// has to render, and writes are guarded separately.
func ReadTopology(rt Runtime) Topology {
	top := Topology{Mode: TopologySingle}

	files, err := filepath.Glob(filepath.Join(rt.XrayConfDir, "*.json"))
	if err != nil || len(files) == 0 {
		return top
	}

	var balancers []interface{}
	for _, file := range files {
		var cfg map[string]interface{}
		if err := ReadJSONC(file, &cfg); err != nil {
			continue
		}

		for _, raw := range asSlice(cfg["outbounds"]) {
			ob, ok := raw.(map[string]interface{})
			if !ok || isServiceOutbound(ob) {
				continue
			}
			if tag, _ := ob["tag"].(string); tag != "" {
				top.ProxyTags = append(top.ProxyTags, tag)
			}
		}

		if routing, ok := cfg["routing"].(map[string]interface{}); ok {
			balancers = append(balancers, asSlice(routing["balancers"])...)
		}
	}

	// The first balancer with a usable selector wins — XKeen's own speed
	// balancer works the same way, on one tag at a time.
	for _, raw := range balancers {
		balancer, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		selectors := stringsOf(balancer["selector"])
		if len(selectors) == 0 {
			continue
		}
		tag, _ := balancer["tag"].(string)
		if tag == "" {
			continue
		}

		top.Mode = TopologyPool
		top.BalancerTag = tag
		top.Selectors = selectors
		top.PoolTags = matchSelectors(top.ProxyTags, selectors)
		break
	}

	return top
}

// matchSelectors keeps the tags a balancer selects. Xray matches a selector
// against a tag by substring, not by prefix.
func matchSelectors(tags, selectors []string) []string {
	var matched []string
	for _, tag := range tags {
		for _, sel := range selectors {
			if sel != "" && strings.Contains(tag, sel) {
				matched = append(matched, tag)
				break
			}
		}
	}
	return matched
}

func asSlice(v interface{}) []interface{} {
	s, _ := v.([]interface{})
	return s
}

func stringsOf(v interface{}) []string {
	var out []string
	for _, raw := range asSlice(v) {
		if s, ok := raw.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ConfigFiles lists the JSON files Xray will merge, for callers that need to
// know where an outbound might be hiding.
func ConfigFiles(rt Runtime) []string {
	files, err := filepath.Glob(filepath.Join(rt.XrayConfDir, "*.json"))
	if err != nil {
		return nil
	}

	readable := files[:0]
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && !st.IsDir() {
			readable = append(readable, f)
		}
	}

	return readable
}
