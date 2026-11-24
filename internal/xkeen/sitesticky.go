package xkeen

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"xkeen-panel/internal/models"
)

// Two misses a minute apart: one lost handshake is noise, three minutes of a dead exit is an outage
const stickyFailLimit = 2

// resolveSticky keeps a site on the node it already uses for as long as that node opens it.
func (m *SiteManager) resolveSticky(site Site, nodes []map[string]interface{}) (string, map[string]interface{}) {
	passing := site.Passing()
	if len(passing) == 0 {
		return "", nil
	}

	byKey := make(map[string]map[string]interface{}, len(nodes))
	for _, node := range nodes {
		if ep, ok := endpointOfOutbound(node); ok {
			byKey[ep.Key()] = node
		}
	}

	ranked := make([]SiteResult, 0, len(passing))
	for _, r := range site.Results {
		if passing[r.Node] {
			ranked = append(ranked, r)
		}
	}
	// A node with a second exit shows a site spread over several networks two addresses, so one address beats both speed and the main node
	preferred := m.Preferred()
	rank := func(r SiteResult) int {
		exits, _ := m.Exits.Get(r.Node)
		switch {
		case exits.SingleExit() && r.Node == preferred:
			return 0
		case exits.SingleExit():
			return 1
		case r.Node == preferred:
			return 2
		}
		return 3
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if a, b := rank(ranked[i]), rank(ranked[j]); a != b {
			return a < b
		}
		return ranked[i].Ms < ranked[j].Ms
	})

	candidates := []string{site.Sticky}
	for _, r := range ranked {
		candidates = append(candidates, r.Node)
	}

	for _, key := range candidates {
		if key == "" || !passing[key] {
			continue
		}
		if node := byKey[key]; node != nil {
			return key, node
		}
		// A pool refresh may have dropped the node while the subscription still lists it
		if node := m.serverOutbound(key); node != nil {
			return key, node
		}
	}
	return "", nil
}

func (m *SiteManager) serverOutbound(nodeKey string) map[string]interface{} {
	server, ok := ServerForNode(vlessServers(m.subscription.GetServers()), nodeKey)
	if !ok {
		return nil
	}
	nodes, err := outboundsForServers(m.outboundsPath, []models.Server{server})
	if err != nil || len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

// The sticky node is rechecked even after it left the pool, or the next check would silently move the site
func (m *SiteManager) withSticky(site Site, nodes []map[string]interface{}) []map[string]interface{} {
	if site.Lock != nil || site.Sticky == "" {
		return nodes
	}
	for _, node := range nodes {
		if ep, ok := endpointOfOutbound(node); ok && ep.Key() == site.Sticky {
			return nodes
		}
	}
	if node := m.serverOutbound(site.Sticky); node != nil {
		return append(append([]map[string]interface{}{}, nodes...), node)
	}
	return nodes
}

func withStickyRow(rows []siteNodeRow, sticky string, servers []models.Server) []siteNodeRow {
	if sticky == "" {
		return rows
	}
	for _, row := range rows {
		if row.key == sticky {
			return rows
		}
	}
	server, ok := ServerForNode(servers, sticky)
	if !ok {
		return rows
	}
	return append(append([]siteNodeRow{}, rows...), siteNodeRow{key: sticky, address: server.Address, server: &server})
}

func (m *SiteManager) nodeName(key string) string {
	if server, ok := ServerForNode(m.subscription.GetServers(), key); ok {
		return server.Name
	}
	host, _, _ := strings.Cut(key, ":")
	return host
}

// stickyBroken reports that the site's node stopped carrying traffic to it. Only a TLS handshake is made: an HTTP request a minute would look like a bot to the site itself.
func (m *SiteManager) stickyBroken(site Site) bool {
	if site.Lock != nil || site.Sticky == "" {
		return false
	}
	addr := m.ProbeAddr()
	if addr == "" {
		return false
	}
	u, err := url.Parse(site.CheckURL)
	if err != nil || u.Scheme != "https" || !hostCovered(u.Hostname(), site.Domains) {
		return false
	}

	err = probeTLS(addr, u.Hostname(), m.timeout)
	if err == nil || IsRestarting() {
		delete(m.stickyFails, site.ID)
		return false
	}

	m.stickyFails[site.ID]++
	if m.stickyFails[site.ID] < stickyFailLimit {
		return false
	}
	if m.stickyFails[site.ID] == stickyFailLimit {
		m.Logf("[SITES] %s перестал отвечать через %s (%s) — перепроверяю ноды", site.Domains[0], m.nodeName(site.Sticky), shortNetError(err))
	}
	return true
}

// probeTLS completes a TLS handshake with host through a local socks5 inbound.
func probeTLS(socksAddr, host string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", socksAddr, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	if len(host) > 255 {
		return fmt.Errorf("слишком длинное имя хоста")
	}
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return err
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return err
	}
	if greeting[0] != 5 || greeting[1] != 0 {
		return fmt.Errorf("socks: вход не принял подключение")
	}

	request := append([]byte{5, 1, 0, 3, byte(len(host))}, host...)
	request = append(request, 443>>8, 443&0xff)
	if _, err := conn.Write(request); err != nil {
		return err
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[1] != 0 {
		return fmt.Errorf("socks: соединение отклонено (%d)", reply[1])
	}
	bound := 0
	switch reply[3] {
	case 1:
		bound = 4
	case 4:
		bound = 16
	case 3:
		size := make([]byte, 1)
		if _, err := io.ReadFull(conn, size); err != nil {
			return err
		}
		bound = int(size[0])
	}
	if _, err := io.ReadFull(conn, make([]byte, bound+2)); err != nil {
		return err
	}

	return tls.Client(conn, &tls.Config{ServerName: host}).Handshake()
}
