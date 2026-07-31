// Package mihomo generates the proxy section of a Mihomo config from the
// panel's subscription. Mihomo is the second proxy core XKeen 2.x can run
// (`xkeen -mihomo`), and its config is YAML rather than Xray's JSON.
package mihomo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"xkeen-panel/internal/models"
	"xkeen-panel/internal/xkeen"
)

// EntwareBypassMark is the mark XKeen looks for when Entware proxying is on:
// it accepts routing-mark 255 globally, on a proxy-provider override, or on
// every single proxy. Generated proxies keep whatever the config already used.
const EntwareBypassMark = 255

// Config is a parsed Mihomo config. It is kept as a yaml.Node so that comments,
// key order and every section the panel does not understand survive a write.
type Config struct {
	path string
	doc  yaml.Node
}

// Read loads the config file.
func Read(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения %s: %w", path, err)
	}

	cfg := &Config{path: path}
	if err := yaml.Unmarshal(data, &cfg.doc); err != nil {
		return nil, fmt.Errorf("ошибка парсинга %s: %w", path, err)
	}

	if cfg.root() == nil {
		// An empty file parses into an empty document — start a mapping
		cfg.doc = yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}},
		}
	}

	return cfg, nil
}

// Write stores the config, keeping the previous content as .bak.
func (c *Config) Write() error {
	var buf strings.Builder

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&c.doc); err != nil {
		return fmt.Errorf("ошибка сериализации %s: %w", c.path, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("ошибка сериализации %s: %w", c.path, err)
	}

	if err := backupFile(c.path); err != nil {
		return err
	}

	return writeFileAtomic(c.path, []byte(buf.String()), 0644)
}

// Path returns the file this config was read from.
func (c *Config) Path() string { return c.path }

func (c *Config) root() *yaml.Node {
	if len(c.doc.Content) == 0 {
		return nil
	}
	root := c.doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	return root
}

// ProxyNames lists the names of the proxies currently in the config.
func (c *Config) ProxyNames() []string {
	proxies := mapValue(c.root(), "proxies")
	if proxies == nil || proxies.Kind != yaml.SequenceNode {
		return nil
	}

	var names []string
	for _, node := range proxies.Content {
		if name := mapValue(node, "name"); name != nil {
			names = append(names, name.Value)
		}
	}

	return names
}

// RoutingMark reports the routing-mark the config already relies on: the global
// one, or the one carried by the first proxy. Zero means none is set.
func (c *Config) RoutingMark() int {
	if global := mapValue(c.root(), "routing-mark"); global != nil {
		return atoiOrZero(global.Value)
	}

	proxies := mapValue(c.root(), "proxies")
	if proxies == nil || proxies.Kind != yaml.SequenceNode {
		return 0
	}
	for _, node := range proxies.Content {
		if mark := mapValue(node, "routing-mark"); mark != nil {
			return atoiOrZero(mark.Value)
		}
	}

	return 0
}

// SyncProxies replaces the proxies section with one entry per VLESS server and
// points every proxy-group at the new names.
//
// The routing mark already in the config is carried over: XKeen validates it on
// every proxy when Entware proxying is on, and dropping it disables that feature
// (or, with strict PBR on the Beta channel, stops the core from starting).
func (c *Config) SyncProxies(servers []models.Server) ([]string, error) {
	mark := c.RoutingMark()

	var proxies []*yaml.Node
	var names []string
	used := map[string]bool{}

	for _, server := range servers {
		if server.RawURI == "" || (server.Protocol != "" && server.Protocol != "vless") {
			continue
		}

		params, err := xkeen.ParseVLESS(server.RawURI)
		if err != nil {
			continue
		}

		name := uniqueName(server.Name, params.Address, used)
		node, err := proxyNode(name, params, mark)
		if err != nil {
			return nil, err
		}

		proxies = append(proxies, node)
		names = append(names, name)
	}

	if len(proxies) == 0 {
		return nil, fmt.Errorf("нет VLESS-серверов для конфигурации Mihomo")
	}

	setMapValue(c.root(), "proxies", &yaml.Node{
		Kind:    yaml.SequenceNode,
		Tag:     "!!seq",
		Content: proxies,
	})

	c.retargetGroups(names)

	return names, nil
}

// retargetGroups rewrites the proxy list of every group, keeping the special
// entries Mihomo resolves itself (DIRECT, REJECT, PASS and nested group names).
func (c *Config) retargetGroups(names []string) {
	groups := mapValue(c.root(), "proxy-groups")
	if groups == nil || groups.Kind != yaml.SequenceNode {
		return
	}

	groupNames := map[string]bool{}
	for _, group := range groups.Content {
		if name := mapValue(group, "name"); name != nil {
			groupNames[name.Value] = true
		}
	}

	for _, group := range groups.Content {
		list := mapValue(group, "proxies")
		if list == nil || list.Kind != yaml.SequenceNode {
			continue
		}

		var kept []*yaml.Node
		for _, entry := range list.Content {
			if isReservedProxy(entry.Value) || groupNames[entry.Value] {
				kept = append(kept, entry)
			}
		}

		for _, name := range names {
			kept = append(kept, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name})
		}

		list.Content = kept
	}
}

func isReservedProxy(name string) bool {
	switch name {
	case "DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE", "GLOBAL":
		return true
	}
	return false
}

// uniqueName keeps subscription names readable but unique — Mihomo rejects a
// config with two proxies sharing a name.
func uniqueName(name, fallback string, used map[string]bool) string {
	candidate := strings.TrimSpace(name)
	if candidate == "" {
		candidate = fallback
	}

	unique := candidate
	for i := 2; used[unique]; i++ {
		unique = fmt.Sprintf("%s #%d", candidate, i)
	}
	used[unique] = true

	return unique
}

// backupFile and writeFileAtomic mirror the guarantees the Xray side gives:
// a config the core is about to read must never be half-written.
func backupFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("ошибка чтения %s для резервной копии: %w", path, err)
	}

	if err := os.WriteFile(path+".bak", data, 0644); err != nil {
		return fmt.Errorf("не удалось создать резервную копию %s: %w", path+".bak", err)
	}

	return nil
}

// RestoreBackup puts the .bak copy back after a failed validation.
func RestoreBackup(path string) error {
	data, err := os.ReadFile(path + ".bak")
	if err != nil {
		return fmt.Errorf("резервная копия %s недоступна: %w", path+".bak", err)
	}

	return writeFileAtomic(path, data, 0644)
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("не удалось создать временный файл в %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("ошибка записи %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("ошибка сброса на диск %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("ошибка закрытия %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("ошибка прав доступа %s: %w", tmpName, err)
	}

	return os.Rename(tmpName, path)
}
