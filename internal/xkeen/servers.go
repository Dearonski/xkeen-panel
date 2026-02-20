package xkeen

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"xkeen-panel/internal/models"
)

// ParseSubscription парсит содержимое подписки (base64 или plain text)
func ParseSubscription(content string) ([]models.Server, error) {
	// Попробовать декодировать base64
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(content))
	if err != nil {
		// Попробовать URL-safe base64
		decoded, err = base64.URLEncoding.DecodeString(strings.TrimSpace(content))
		if err != nil {
			// Попробовать base64 без паддинга
			decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(content))
			if err != nil {
				// Возможно, уже plain text
				decoded = []byte(content)
			}
		}
	}

	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	var servers []models.Server

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var server *models.Server
		var parseErr error

		switch {
		case strings.HasPrefix(line, "vless://"):
			server, parseErr = parseVLESS(line)
		case strings.HasPrefix(line, "vmess://"):
			server, parseErr = parseVMess(line)
		case strings.HasPrefix(line, "trojan://"):
			server, parseErr = parseTrojan(line)
		case strings.HasPrefix(line, "ss://"):
			server, parseErr = parseShadowsocks(line)
		default:
			continue
		}

		if parseErr != nil || server == nil {
			continue
		}

		server.ID = len(servers)
		server.RawURI = line
		servers = append(servers, *server)
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("не удалось распарсить ни одного сервера")
	}

	return servers, nil
}

// parseVLESS парсит vless://uuid@host:port?params#name
func parseVLESS(uri string) (*models.Server, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}

	name := u.Fragment
	if name == "" {
		name = host
	}

	return &models.Server{
		Name:     name,
		Address:  host,
		Port:     port,
		Protocol: "vless",
		Latency:  -1,
	}, nil
}

// parseVMess парсит vmess://base64json
func parseVMess(uri string) (*models.Server, error) {
	encoded := strings.TrimPrefix(uri, "vmess://")

	// Декодирование base64
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("ошибка декодирования vmess: %w", err)
		}
	}

	var vmessConfig map[string]interface{}
	if err := json.Unmarshal(decoded, &vmessConfig); err != nil {
		return nil, fmt.Errorf("ошибка парсинга vmess JSON: %w", err)
	}

	address, _ := vmessConfig["add"].(string)
	portStr := fmt.Sprintf("%v", vmessConfig["port"])
	port, _ := strconv.Atoi(portStr)
	name, _ := vmessConfig["ps"].(string)

	if address == "" {
		return nil, fmt.Errorf("vmess: отсутствует адрес")
	}
	if port == 0 {
		port = 443
	}
	if name == "" {
		name = address
	}

	return &models.Server{
		Name:     name,
		Address:  address,
		Port:     port,
		Protocol: "vmess",
		Latency:  -1,
	}, nil
}

// parseTrojan парсит trojan://password@host:port?params#name
func parseTrojan(uri string) (*models.Server, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	if port == 0 {
		port = 443
	}

	name := u.Fragment
	if name == "" {
		name = host
	}

	return &models.Server{
		Name:     name,
		Address:  host,
		Port:     port,
		Protocol: "trojan",
		Latency:  -1,
	}, nil
}

// parseShadowsocks парсит ss://base64@host:port#name или ss://base64#name
func parseShadowsocks(uri string) (*models.Server, error) {
	// Убрать префикс
	raw := strings.TrimPrefix(uri, "ss://")

	// Извлечь имя из фрагмента
	name := ""
	if idx := strings.LastIndex(raw, "#"); idx != -1 {
		name = raw[idx+1:]
		raw = raw[:idx]
	}
	name, _ = url.QueryUnescape(name)

	var host string
	var port int

	// Формат 1: base64@host:port
	if atIdx := strings.LastIndex(raw, "@"); atIdx != -1 {
		hostPort := raw[atIdx+1:]
		h, p, err := net.SplitHostPort(hostPort)
		if err == nil {
			host = h
			port, _ = strconv.Atoi(p)
		}
	} else {
		// Формат 2: всё в base64
		decoded, err := base64.URLEncoding.DecodeString(raw)
		if err != nil {
			decoded, err = base64.RawURLEncoding.DecodeString(raw)
			if err != nil {
				decoded, err = base64.StdEncoding.DecodeString(raw)
				if err != nil {
					return nil, fmt.Errorf("не удалось декодировать ss URI")
				}
			}
		}

		// method:password@host:port
		parts := string(decoded)
		if atIdx := strings.LastIndex(parts, "@"); atIdx != -1 {
			hostPort := parts[atIdx+1:]
			h, p, err := net.SplitHostPort(hostPort)
			if err == nil {
				host = h
				port, _ = strconv.Atoi(p)
			}
		}
	}

	if host == "" {
		return nil, fmt.Errorf("ss: не удалось извлечь адрес")
	}
	if port == 0 {
		port = 443
	}
	if name == "" {
		name = host
	}

	return &models.Server{
		Name:     name,
		Address:  host,
		Port:     port,
		Protocol: "shadowsocks",
		Latency:  -1,
	}, nil
}

// CheckLatency проверяет задержку TCP-соединения к серверу
func CheckLatency(address string, port int, timeout time.Duration) int {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", address, port), timeout)
	if err != nil {
		return -1
	}
	conn.Close()
	return int(time.Since(start).Milliseconds())
}

// CheckAllLatencies проверяет задержку всех серверов параллельно с общим таймаутом
func CheckAllLatencies(servers []models.Server, timeout time.Duration) []models.Server {
	var wg sync.WaitGroup
	result := make([]models.Server, len(servers))
	copy(result, servers)

	for i := range result {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result[idx].Latency = CheckLatency(result[idx].Address, result[idx].Port, timeout)
		}(i)
	}

	wg.Wait()
	return result
}
