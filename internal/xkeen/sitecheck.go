package xkeen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const siteCheckConcurrency = 5

// Browsers are what sites expect, and some bot filters answer a Go client differently
const siteCheckUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

// A throwaway Xray keeps the check away from the running core and its live connections
func CheckSite(rt Runtime, nodes []map[string]interface{}, checkURL string, timeout time.Duration) ([]SiteResult, error) {
	if len(nodes) == 0 {
		return nil, fmt.Errorf("в пуле нет нод")
	}
	if _, err := os.Stat(rt.CoreBin); err != nil {
		return nil, fmt.Errorf("бинарь xray не найден (%s)", rt.CoreBin)
	}

	ports, err := freePorts(len(nodes))
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "xkeen-panel-sitecheck-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	configPath := filepath.Join(dir, "config.json")
	data, err := json.Marshal(siteCheckConfig(nodes, ports))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, rt.CoreBin, "run", "-c", configPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("не удалось запустить проверочный xray: %w", err)
	}
	defer func() {
		cancel()
		cmd.Wait()
	}()

	if err := waitListening(ports[len(ports)-1], 10*time.Second); err != nil {
		return nil, fmt.Errorf("проверочный xray не поднялся: %s", TailLines(stderr.String(), 2))
	}

	results := make([]SiteResult, len(nodes))
	sem := make(chan struct{}, siteCheckConcurrency)
	var wg sync.WaitGroup

	for i, node := range nodes {
		key := ""
		if ep, ok := endpointOfOutbound(node); ok {
			key = ep.Key()
		}

		wg.Add(1)
		go func(i, port int, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := openThrough(port, checkURL, timeout)
			result.Node = key
			results[i] = result
		}(i, ports[i], key)
	}
	wg.Wait()

	return results, nil
}

func siteCheckConfig(nodes []map[string]interface{}, ports []int) map[string]interface{} {
	inbounds := make([]interface{}, len(nodes))
	outbounds := make([]interface{}, len(nodes))
	rules := make([]interface{}, len(nodes))

	for i, node := range nodes {
		in, out := fmt.Sprintf("in%d", i), fmt.Sprintf("out%d", i)

		inbounds[i] = map[string]interface{}{
			"tag":      in,
			"listen":   "127.0.0.1",
			"port":     ports[i],
			"protocol": "socks",
			"settings": map[string]interface{}{"auth": "noauth", "udp": false},
		}

		copied := deepCopy(node)
		copied["tag"] = out
		outbounds[i] = copied

		rules[i] = map[string]interface{}{
			"type":        "field",
			"inboundTag":  []interface{}{in},
			"outboundTag": out,
		}
	}

	return map[string]interface{}{
		"log":       map[string]interface{}{"loglevel": "warning"},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"routing":   map[string]interface{}{"rules": rules},
	}
}

func freePorts(n int) ([]int, error) {
	listeners := make([]net.Listener, 0, n)
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()

	ports := make([]int, 0, n)
	for range n {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, fmt.Errorf("нет свободного порта для проверки: %w", err)
		}
		listeners = append(listeners, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}

	return ports, nil
}

func waitListening(port int, limit time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("порт %d не открылся", port)
}

func openThrough(port int, checkURL string, timeout time.Duration) SiteResult {
	proxy := &url.URL{Scheme: "socks5", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	client := &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true},
	}

	req, err := http.NewRequest(http.MethodGet, checkURL, nil)
	if err != nil {
		return SiteResult{Error: err.Error()}
	}
	req.Header.Set("User-Agent", siteCheckUserAgent)
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return SiteResult{Error: shortNetError(err)}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	return SiteResult{
		OK:     resp.StatusCode < 400,
		Status: resp.StatusCode,
		Ms:     int(time.Since(start).Milliseconds()),
	}
}

func shortNetError(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "таймаут"
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 {
		msg = msg[i+2:]
	}
	return msg
}
