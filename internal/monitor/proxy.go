package monitor

import (
	"io"
	"net/http"
	"net/url"
	"time"

	"xkeen-panel/internal/mihomo"
	"xkeen-panel/internal/xkeen"
)

// proxiedTransport sends requests through the core's local inbound. A nil proxy
// means a direct request from the router, which XKeen never intercepts.
func proxiedTransport(proxy *url.URL) *http.Transport {
	return &http.Transport{
		Proxy:             http.ProxyURL(proxy),
		DisableKeepAlives: true,
	}
}

// fetchLatency times a GET of target and discards the body.
func fetchLatency(target string, proxy *url.URL, timeout time.Duration) (int, error) {
	client := &http.Client{Timeout: timeout, Transport: proxiedTransport(proxy)}

	start := time.Now()
	resp, err := client.Get(target)
	if err != nil {
		return -1, err
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()

	return int(time.Since(start).Milliseconds()), nil
}

// SetProbeAddr tells the watchdog where the Xray probe inbound listens. Empty
// leaves Xray checks on the direct link, which proves nothing about the proxy.
func (w *Watchdog) SetProbeAddr(addr string) {
	w.mu.Lock()
	w.probeAddr = addr
	w.mu.Unlock()
}

// proxyURL is how checks reach the proxy: the panel's own socks inbound on Xray,
// the mixed/socks port on Mihomo. Nil when neither is available.
func (w *Watchdog) proxyURL() *url.URL {
	addr := ""
	switch rt := w.detector.Runtime(); rt.Core {
	case xkeen.CoreMihomo:
		if cfg, err := mihomo.Read(rt.MihomoConf); err == nil {
			addr = cfg.LocalProxyAddr()
		}
	default:
		w.mu.RLock()
		addr = w.probeAddr
		w.mu.RUnlock()
	}

	if addr == "" {
		return nil
	}
	u, err := url.Parse(xkeen.ProbeProxyURL(addr))
	if err != nil {
		return nil
	}
	return u
}

// EnsureProbe installs the Xray probe route and points checks at it. Returns
// true when the core config changed and needs a restart.
func (w *Watchdog) EnsureProbe() (bool, error) {
	w.detector.InvalidateTopology()
	ready, changed, err := xkeen.EnsureProbeRoute(w.detector.Runtime(), w.config.ProbeAddr, w.detector.Topology())

	addr := ""
	if ready {
		addr = w.config.ProbeAddr
	}
	w.SetProbeAddr(addr)

	return changed, err
}
