package xkeen

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"xkeen-panel/internal/models"
)

// ReadOutboundsConfig читает файл 04_outbounds.json
func ReadOutboundsConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения %s: %w", path, err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("ошибка парсинга %s: %w", path, err)
	}

	return config, nil
}

// WriteOutboundsConfig записывает конфиг обратно
func WriteOutboundsConfig(path string, config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка сериализации конфига: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// vlessParams — все параметры из VLESS URI
type vlessParams struct {
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
	Extra       string // JSON из параметра extra (для xhttp: downloadSettings, xmux и т.д.)
}

// parseVLESSURI полностью парсит VLESS URI
func parseVLESSURI(uri string) (*vlessParams, error) {
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
	return &vlessParams{
		UUID:        u.User.Username(),
		Address:     u.Hostname(),
		Port:        port,
		Security:    q.Get("security"),
		Network:     q.Get("type"),
		SNI:         q.Get("sni"),
		Fingerprint: q.Get("fp"),
		PublicKey:    q.Get("pbk"),
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

// buildOutboundFromURI генерирует ПОЛНЫЙ outbound из VLESS URI
// tag — тег из существующего конфига (чтобы совпадал с routing)
func buildOutboundFromURI(p *vlessParams, tag string) map[string]interface{} {
	// Users
	user := map[string]interface{}{
		"id":         p.UUID,
		"encryption": "none",
		"level":      0,
	}
	if p.Flow != "" {
		user["flow"] = p.Flow
	}

	// Settings
	settings := map[string]interface{}{
		"vnext": []interface{}{
			map[string]interface{}{
				"address": p.Address,
				"port":    p.Port,
				"users":   []interface{}{user},
			},
		},
	}

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
		// extra — JSON с downloadSettings, xmux и другими параметрами xhttp
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

// UpdateOutbound генерирует полный outbound из VLESS URI и записывает в конфиг
func UpdateOutbound(outboundsPath string, server *models.Server) error {
	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return err
	}

	outbounds, ok := config["outbounds"].([]interface{})
	if !ok {
		return fmt.Errorf("outbounds не найдены в конфиге")
	}

	// Парсим VLESS URI
	if server.RawURI == "" {
		return fmt.Errorf("RawURI пуст — невозможно сгенерировать конфиг. Обновите подписку")
	}

	params, err := parseVLESSURI(server.RawURI)
	if err != nil {
		return fmt.Errorf("ошибка парсинга URI: %w", err)
	}

	// Определяем тег из существующего proxy-outbound (чтобы совпадал с routing в 05_routing.json)
	existingTag := ""
	replaceIdx := -1
	for i, ob := range outbounds {
		outbound, ok := ob.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		protocol, _ := outbound["protocol"].(string)

		if protocol != "freedom" && protocol != "blackhole" && tag != "direct" && tag != "block" {
			existingTag = tag
			replaceIdx = i
			break
		}
	}

	if existingTag == "" {
		existingTag = "vless-reality"
	}

	// Генерируем полный outbound с правильным тегом
	newOutbound := buildOutboundFromURI(params, existingTag)

	if replaceIdx >= 0 {
		outbounds[replaceIdx] = newOutbound
	} else {
		outbounds = append([]interface{}{newOutbound}, outbounds...)
	}

	config["outbounds"] = outbounds

	// Убираем routing из 04_outbounds — он есть в 05_routing.json
	delete(config, "routing")

	return WriteOutboundsConfig(outboundsPath, config)
}

// GetCurrentOutbound читает текущий адрес/порт из конфига
func GetCurrentOutbound(outboundsPath string) (address string, port int, uuid string, err error) {
	config, err := ReadOutboundsConfig(outboundsPath)
	if err != nil {
		return "", 0, "", err
	}

	outbounds, ok := config["outbounds"].([]interface{})
	if !ok {
		return "", 0, "", fmt.Errorf("outbounds не найдены")
	}

	// Ищем первый proxy-outbound (не direct/block)
	for _, ob := range outbounds {
		outbound, ok := ob.(map[string]interface{})
		if !ok {
			continue
		}
		protocol, _ := outbound["protocol"].(string)
		tag, _ := outbound["tag"].(string)
		if protocol == "freedom" || protocol == "blackhole" || tag == "direct" || tag == "block" {
			continue
		}

		settings, _ := outbound["settings"].(map[string]interface{})
		if settings == nil {
			continue
		}
		vnext, _ := settings["vnext"].([]interface{})
		if len(vnext) == 0 {
			continue
		}
		entry, _ := vnext[0].(map[string]interface{})
		if entry == nil {
			continue
		}

		address, _ = entry["address"].(string)
		portF, _ := entry["port"].(float64)
		port = int(portF)

		users, _ := entry["users"].([]interface{})
		if len(users) > 0 {
			user, _ := users[0].(map[string]interface{})
			if user != nil {
				uuid, _ = user["id"].(string)
			}
		}
		return
	}

	return "", 0, "", fmt.Errorf("proxy outbound не найден")
}

// extractUserInfo извлекает userinfo из URI
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

// extractVMessUUID извлекает UUID из vmess URI (base64 JSON)
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

// extractSSCredentials извлекает method и password из ss URI
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
