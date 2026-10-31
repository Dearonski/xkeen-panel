package xkeen

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const lockCheckEvery = time.Minute

// Earlier cores hand traffic routed to a missing tag to the default outbound, which would turn a blocked lock into a leak
var lockMinXray = [3]int{25, 9, 11}

var xrayVersionRe = regexp.MustCompile(`Xray (\d+)\.(\d+)\.(\d+)`)

func xrayVersion(rt Runtime) ([3]int, error) {
	out, err := exec.Command(rt.CoreBin, "version").Output()
	if err != nil {
		return [3]int{}, fmt.Errorf("не удалось узнать версию xray: %w", err)
	}
	m := xrayVersionRe.FindStringSubmatch(string(out))
	if m == nil {
		return [3]int{}, fmt.Errorf("не удалось разобрать версию xray")
	}
	var v [3]int
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, nil
}

func versionAtLeast(v, min [3]int) bool {
	for i := range v {
		if v[i] != min[i] {
			return v[i] > min[i]
		}
	}
	return true
}

func hostCovered(host string, domains []string) bool {
	for _, d := range domains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (l SiteLock) matches(exit Exit) bool {
	return exit.IP == l.ExitIP && exit.Country == l.Country
}

func (m *SiteManager) Lock(id int, nodeKey string) error {
	if reason := m.Unavailable(); reason != "" {
		return fmt.Errorf("%s", reason)
	}

	rt := m.detector.Runtime()
	v, err := xrayVersion(rt)
	if err != nil {
		return err
	}
	if !versionAtLeast(v, lockMinXray) {
		return fmt.Errorf("закрепление требует Xray %d.%d.%d или новее, установлен %d.%d.%d",
			lockMinXray[0], lockMinXray[1], lockMinXray[2], v[0], v[1], v[2])
	}

	site, ok := m.store.Get(id)
	if !ok {
		return fmt.Errorf("сайт не найден")
	}
	host := TraceHost(site.CheckURL)
	if !hostCovered(host, site.Domains) {
		return fmt.Errorf("адрес проверки %s должен быть на одном из доменов сайта", host)
	}

	top := m.detector.Topology()
	pool, err := poolNodesInFile(m.outboundsPath, top.Selector())
	if err != nil {
		return err
	}
	var outbound map[string]interface{}
	for _, node := range pool {
		if ep, ok := endpointOfOutbound(node); ok && ep.Key() == nodeKey {
			outbound = deepCopy(node)
			delete(outbound, "tag")
			break
		}
	}
	if outbound == nil {
		return fmt.Errorf("ноды нет в пуле")
	}

	exit, err := TraceThroughNode(rt, outbound, host, m.timeout)
	if err != nil {
		return fmt.Errorf("не удалось узнать выходной IP: %w", err)
	}

	name := nodeKey
	if nodes, err := PoolNodes(m.outboundsPath, top.Selector(), m.subscription.GetServers()); err == nil {
		for _, n := range nodes {
			if (endpoint{Address: n.Address, Port: n.Port, UUID: n.UUID}).Key() == nodeKey && n.Server != nil {
				name = n.Server.Name
			}
		}
	}

	m.work.Lock()
	defer m.work.Unlock()

	site, ok = m.store.Get(id)
	if !ok {
		return fmt.Errorf("сайт не найден")
	}
	site.Lock = &SiteLock{
		Node:     nodeKey,
		Name:     name,
		Outbound: outbound,
		ExitIP:   exit.IP,
		Country:  exit.Country,
		LockedAt: time.Now(),
	}
	if err := m.store.Put(site); err != nil {
		return err
	}
	m.setVerified(id, false)

	if err := m.applyLocked(); err != nil {
		return err
	}

	m.Logf("[LOCK] %s закреплён за %s: %s (%s)", site.Domains[0], name, exit.IP, exit.Country)
	m.Notify()
	return nil
}

func (m *SiteManager) Unlock(id int) error {
	m.work.Lock()
	defer m.work.Unlock()

	site, ok := m.store.Get(id)
	if !ok {
		return fmt.Errorf("сайт не найден")
	}
	if site.Lock == nil {
		return nil
	}
	site.Lock = nil
	if err := m.store.Put(site); err != nil {
		return err
	}
	m.clearVerified(id)

	if err := m.applyLocked(); err != nil {
		return err
	}

	m.Logf("[LOCK] Закрепление %s снято", site.Domains[0])
	m.Notify()
	return nil
}

func (m *SiteManager) RunLocks(ctx context.Context) {
	if err := m.syncKillSwitch(m.store.List()); err != nil {
		m.Logf("[LOCK] Kill switch не применён: %v", err)
	}

	ticker := time.NewTicker(lockCheckEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if m.Unavailable() != "" || IsRestarting() {
			continue
		}
		for _, site := range m.store.List() {
			if site.Lock != nil {
				m.verifyLock(site)
			}
		}
	}
}

func (m *SiteManager) verifyLock(site Site) {
	lock := *site.Lock
	host := TraceHost(site.CheckURL)
	rt := m.detector.Runtime()

	// Blocked, the pin is gone from the core, so only a throwaway can tell whether the node is back
	if lock.Blocked {
		exit, err := TraceThroughNode(rt, lock.Outbound, host, m.timeout)
		if err == nil && lock.matches(exit) {
			m.setLockBlocked(site.ID, lock, false, "")
		}
		return
	}

	exit, live, err := m.traceLock(lock, host)
	if err != nil && live {
		if IsRestarting() {
			return
		}
		// A failing live path carries no traffic at all, so a throwaway decides whether the node itself is fine
		exit, err = TraceThroughNode(rt, lock.Outbound, host, m.timeout)
		live = false
	}

	switch {
	case err != nil:
		m.setLockBlocked(site.ID, lock, true, fmt.Sprintf("нода не отвечает: %v", err))
	case !lock.matches(exit):
		m.setLockBlocked(site.ID, lock, true, fmt.Sprintf("выход сменился на %s (%s)", exit.IP, exit.Country))
	default:
		m.setVerified(site.ID, live)
	}
}

func (m *SiteManager) traceLock(lock SiteLock, host string) (Exit, bool, error) {
	if addr := m.ProbeAddr(); addr != "" {
		proxy, err := url.Parse(ProbeProxyURL(addr))
		if err == nil {
			exit, err := Trace(proxy, host, m.timeout)
			return exit, true, err
		}
	}
	exit, err := TraceThroughNode(m.detector.Runtime(), lock.Outbound, host, m.timeout)
	return exit, false, err
}

func (m *SiteManager) setLockBlocked(id int, expected SiteLock, blocked bool, reason string) {
	m.work.Lock()
	defer m.work.Unlock()

	site, ok := m.store.Get(id)
	if !ok || site.Lock == nil || site.Lock.Node != expected.Node || site.Lock.ExitIP != expected.ExitIP || site.Lock.Blocked == blocked {
		return
	}

	site.Lock.Blocked = blocked
	site.Lock.Reason = reason
	if blocked {
		site.Lock.BlockedAt = time.Now()
	} else {
		site.Lock.BlockedAt = time.Time{}
	}
	if err := m.store.Put(site); err != nil {
		m.Logf("[LOCK] Не удалось сохранить состояние: %v", err)
	}

	if err := m.applyLocked(); err != nil {
		m.Logf("[LOCK] Не удалось применить: %v", err)
	}

	if blocked {
		m.clearVerified(id)
		m.Logf("[LOCK] %s заблокирован — %s", site.Domains[0], reason)
	} else {
		m.setVerified(id, false)
		m.Logf("[LOCK] %s снова идёт через %s (%s)", site.Domains[0], expected.ExitIP, expected.Country)
	}
	m.Notify()
}

func (m *SiteManager) setVerified(id int, live bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verified[id] = lockVerification{at: time.Now(), live: live}
}

func (m *SiteManager) clearVerified(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.verified, id)
}

func (m *SiteManager) verification(id int) (lockVerification, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.verified[id]
	return v, ok
}

type lockVerification struct {
	at   time.Time
	live bool
}
