package xkeen

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"xkeen-panel/internal/models"
)

// ReadOutboundsConfig reads 04_outbounds.json.
func ReadOutboundsConfig(path string) (map[string]interface{}, error) {
	var config map[string]interface{}
	if err := ReadJSONC(path, &config); err != nil {
		return nil, err
	}

	return config, nil
}

// WriteOutboundsConfig writes the config back.
//
// Backup first, then write through a temp file in the same directory and rename:
// a half-written config in the directory Xray reads means XKeen cannot start,
// and the router can lose power mid-write.
//
// Comments in the original file are lost — the panel serialises the parsed tree.
func WriteOutboundsConfig(path string, config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка сериализации конфига: %w", err)
	}

	if err := backupFile(path); err != nil {
		return err
	}

	return writeFileAtomic(path, data, 0644)
}

// backupFile keeps a single .bak copy next to the original.
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

// RestoreBackup puts the .bak copy back — used when validation rejects a write.
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

// VLESSParams holds every parameter of a VLESS URI. Exported because the Mihomo
// config generator builds its proxies from the same fields.
type VLESSParams struct {
	UUID        string
	Address     string
	Port        int
	Security    string // reality, tls, none
	Network     string // tcp, xhttp, ws, grpc, h2
	SNI         string
	Fingerprint string
	PublicKey   string
	ShortID     string
	SpiderX     string
	Host        string
	Path        string
	Mode        string // packet-up, etc.
	Flow        string // xtls-rprx-vision
	ALPN        string
	Encryption  string
	Extra       string // JSON from the extra parameter (xhttp: downloadSettings, xmux, …)
}

// ParseVLESS parses a VLESS URI in full.
func ParseVLESS(uri string) (*VLESSParams, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "vless" {
		return nil, fmt.Errorf("не VLESS URI")
	}

	port := 443
	if p := u.Port(); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	q := u.Query()
	return &VLESSParams{
		UUID:        u.User.Username(),
		Address:     u.Hostname(),
		Port:        port,
		Security:    q.Get("security"),
		Network:     q.Get("type"),
		SNI:         q.Get("sni"),
		Fingerprint: q.Get("fp"),
		PublicKey:   q.Get("pbk"),
		ShortID:     q.Get("sid"),
		SpiderX:     q.Get("spx"),
		Host:        q.Get("host"),
		Path:        q.Get("path"),
		Mode:        q.Get("mode"),
		Flow:        q.Get("flow"),
		ALPN:        q.Get("alpn"),
		Encryption:  q.Get("encryption"),
		Extra:       q.Get("extra"),
	}, nil
}

// buildOutboundFromURI renders a complete outbound from a VLESS URI. The tag
// comes from the existing config so routing keeps matching, and the format is
// whichever credential shape that config already uses.
func buildOutboundFromURI(p *VLESSParams, tag string, format outboundFormat) map[string]interface{} {
	settings := buildVLESSSettings(p, format)

	// StreamSettings
	streamSettings := map[string]interface{}{}

	// Network
	network := p.Network
	if network == "" {
		network = "tcp"
	}
	streamSettings["network"] = network

	// Security (reality, tls, none)
	if p.Security != "" && p.Security != "none" {
		streamSettings["security"] = p.Security
	}

	// Reality settings
	if p.Security == "reality" {
		rs := map[string]interface{}{}
		if p.SNI != "" {
			rs["serverName"] = p.SNI
		}
		if p.Fingerprint != "" {
			rs["fingerprint"] = p.Fingerprint
		}
		if p.PublicKey != "" {
			rs["publicKey"] = p.PublicKey
		}
		if p.ShortID != "" {
			rs["shortId"] = p.ShortID
		}
		if p.SpiderX != "" {
			rs["spiderX"] = p.SpiderX
		}
		streamSettings["realitySettings"] = rs
	}

	// TLS settings
	if p.Security == "tls" {
		ts := map[string]interface{}{}
		if p.SNI != "" {
			ts["serverName"] = p.SNI
		}
		if p.Fingerprint != "" {
			ts["fingerprint"] = p.Fingerprint
		}
		if p.ALPN != "" {
			ts["alpn"] = strings.Split(p.ALPN, ",")
		}
		streamSettings["tlsSettings"] = ts
	}

	// Transport-specific settings
	switch network {
	case "ws":
		ws := map[string]interface{}{}
		if p.Path != "" {
			ws["path"] = p.Path
		}
		if p.Host != "" {
			ws["headers"] = map[string]interface{}{"Host": p.Host}
		}
		streamSettings["wsSettings"] = ws

	case "grpc":
		grpc := map[string]interface{}{}
		if p.Path != "" {
			grpc["serviceName"] = p.Path
		}
		if p.Mode != "" {
			grpc["multiMode"] = p.Mode == "multi"
		}
		streamSettings["grpcSettings"] = grpc

	case "xhttp":
		xs := map[string]interface{}{}
		if p.Host != "" {
			xs["host"] = p.Host
		} else if p.SNI != "" {
			xs["host"] = p.SNI
		}
		if p.Path != "" {
			xs["path"] = p.Path
		}
		if p.Mode != "" {
			xs["mode"] = p.Mode
		}
		// extra carries downloadSettings, xmux and other xhttp parameters as JSON
		if p.Extra != "" {
			var extraMap map[string]interface{}
			if err := json.Unmarshal([]byte(p.Extra), &extraMap); err == nil {
				for k, v := range extraMap {
					xs[k] = v
				}
			}
		}
		streamSettings["xhttpSettings"] = xs

	case "h2":
		h2 := map[string]interface{}{}
		if p.Path != "" {
			h2["path"] = p.Path
		}
		if p.Host != "" {
			h2["host"] = []interface{}{p.Host}
		}
		streamSettings["httpSettings"] = h2
	}

	outbound := map[string]interface{}{
		"protocol":       "vless",
		"settings":       settings,
		"streamSettings": streamSettings,
		"tag":            tag,
	}

	return outbound
}

// UpdateOutbound renders the outbound for server and writes it to the config.
func UpdateOutbound(outboundsPath string, server *models.Server) error {
	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return err
	}

	outbounds, ok := config["outbounds"].([]interface{})
	if !ok {
		return fmt.Errorf("outbounds не найдены в конфиге")
	}

	// Parse the VLESS URI
	if server.RawURI == "" {
		return fmt.Errorf("RawURI пуст — невозможно сгенерировать конфиг. Обновите подписку")
	}

	if server.Protocol != "" && server.Protocol != "vless" {
		return fmt.Errorf("автоконфигурация поддерживает только VLESS, протокол %q пока не поддержан", server.Protocol)
	}

	params, err := ParseVLESS(server.RawURI)
	if err != nil {
		return fmt.Errorf("ошибка парсинга URI: %w", err)
	}

	// A pool means routing picks the node through a balancer; replacing one of
	// its members would silently break the pool. Pools are handled separately.
	if n := countProxyOutbounds(outbounds); n > 1 {
		return fmt.Errorf("в конфиге %d прокси-outbound'ов (пул балансировщика) — переключение по одному не поддерживается", n)
	}

	// Tag and shape come from the existing outbound: the tag has to keep matching
	// routing, and nobody asked to convert the config between vnext and flat.
	replaceIdx, existing := findProxyOutbound(outbounds)
	newOutbound := mergeOutbound(existing, buildOutboundFromURI(
		params,
		outboundTag(existing),
		detectOutboundFormat(existing),
	))

	if replaceIdx >= 0 {
		outbounds[replaceIdx] = newOutbound
	} else {
		outbounds = append([]interface{}{newOutbound}, outbounds...)
	}

	config["outbounds"] = outbounds

	// Drop a stray routing section — it belongs in 05_routing.json
	delete(config, "routing")

	return WriteOutboundsConfig(outboundsPath, config)
}

// GetCurrentOutbound reads the configured upstream address, port and uuid.
func GetCurrentOutbound(outboundsPath string) (address string, port int, uuid string, err error) {
	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return "", 0, "", err
	}

	outbounds, ok := config["outbounds"].([]interface{})
	if !ok {
		return "", 0, "", fmt.Errorf("outbounds не найдены")
	}

	// First proxy outbound (not direct/block), in either credential shape
	for _, raw := range outbounds {
		outbound, ok := raw.(map[string]interface{})
		if !ok || isServiceOutbound(outbound) {
			continue
		}
		if address, port, uuid, ok := readProxyEndpoint(outbound); ok {
			return address, port, uuid, nil
		}
	}

	return "", 0, "", fmt.Errorf("proxy outbound не найден")
}

// extractUserInfo pulls the userinfo part out of a URI.
func extractUserInfo(uri string) string {
	idx := strings.Index(uri, "://")
	if idx == -1 {
		return ""
	}
	rest := uri[idx+3:]
	atIdx := strings.IndexByte(rest, '@')
	if atIdx == -1 {
		return ""
	}
	return rest[:atIdx]
}

// extractVMessUUID pulls the uuid out of a vmess URI (base64 JSON).
func extractVMessUUID(uri string) string {
	encoded := strings.TrimPrefix(uri, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, _ = base64.RawStdEncoding.DecodeString(encoded)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(decoded, &config); err != nil {
		return ""
	}

	id, _ := config["id"].(string)
	return id
}

// extractSSCredentials pulls method and password out of an ss URI.
func extractSSCredentials(uri string) (method, password string) {
	raw := strings.TrimPrefix(uri, "ss://")
	if idx := strings.LastIndex(raw, "#"); idx != -1 {
		raw = raw[:idx]
	}

	atIdx := strings.LastIndex(raw, "@")
	var encoded string
	if atIdx != -1 {
		encoded = raw[:atIdx]
	} else {
		encoded = raw
	}

	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			decoded, _ = base64.StdEncoding.DecodeString(encoded)
		}
	}

	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ""
}
