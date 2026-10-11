package monitor

import (
	"fmt"
	"sort"
	"time"

	"xkeen-panel/internal/models"
	"xkeen-panel/internal/xkeen"
)

// PinServer sends all traffic through server. A pool member is pinned by its
// tag; anything else goes into the manual slot first, since the owner may need
// one particular country that the pool's fastest nodes do not cover.
func (w *Watchdog) PinServer(server models.Server, manual bool, note string) (string, error) {
	if w.poolStore == nil {
		return "", fmt.Errorf("состояние пула недоступно")
	}

	top := w.detector.Topology()
	if top.Mode != xkeen.TopologyPool {
		return "", fmt.Errorf("пул не включён")
	}
	rt := w.detector.Runtime()
	path := w.config.OutboundsFile

	tag := ""
	if nodes, err := xkeen.PoolNodes(path, top.Selector(), []models.Server{server}); err == nil {
		for _, node := range nodes {
			if node.Server != nil {
				tag = node.Tag
				break
			}
		}
	}

	restart := false
	if tag == "" {
		if err := xkeen.SetManualOutbound(rt, path, server); err != nil {
			return "", err
		}
		w.detector.InvalidateTopology()
		tag = xkeen.ManualTag

		if err := xkeen.ApplyManualLive(rt, w.config.XrayAPIAddr, path); err != nil {
			w.writeLog("[PIN] Горячая замена недоступна (%v) — перезапускаю ядро", err)
			restart = true
		}
	}

	node := xkeen.NodeKeyForTag(path, top.Selector(), tag)
	if node == "" {
		return "", fmt.Errorf("ноды %q нет в конфиге", tag)
	}

	// Stored before the override: after a restart superviseP re-applies whatever is stored here
	if err := w.poolStore.SetPinnedServer(tag, node, manual, xkeen.ServerCountry(server), note); err != nil {
		w.writeLog("[PIN] Не удалось сохранить закрепление: %v", err)
	}

	if restart {
		go w.restartAndRepin(rt, top)
	} else if err := xkeen.OverrideBalancerTarget(rt, w.config.XrayAPIAddr, top.BalancerTag, tag); err != nil {
		return "", fmt.Errorf("%w. Закрепление ноды требует блок api в конфиге Xray — его добавляет `xkeen -sb on`", err)
	}

	w.NodePinned(tag)
	w.writeLog("[PIN] Трафик закреплён за %s (%s)", server.Name, tag)

	return tag, nil
}

// restartAndRepin restarts the core and puts the stored pin back, since the
// override lives only in the core's memory.
func (w *Watchdog) restartAndRepin(rt xkeen.Runtime, top xkeen.Topology) {
	if _, err := xkeen.Restart(rt.Dispatcher); err != nil {
		w.writeLog("[PIN] Ошибка перезапуска ядра: %v", err)
		return
	}

	deadline := time.Now().Add(90 * time.Second)
	for xkeen.IsRestarting() && time.Now().Before(deadline) {
		time.Sleep(time.Second)
	}
	// The core needs a moment past process start before its API answers
	time.Sleep(3 * time.Second)

	if _, err := xkeen.EnsurePinned(rt, w.config.XrayAPIAddr, top, w.poolStore.Get().PinnedTag); err != nil {
		w.writeLog("[PIN] Не удалось восстановить закрепление после перезапуска: %v", err)
	}
	w.publishPool()
}

// sameCountryReplacement finds the fastest live server of the given country,
// other than the one that just failed.
func (w *Watchdog) sameCountryReplacement(country, failedURI string) (*models.Server, error) {
	var candidates []models.Server
	for _, s := range w.subscription.GetServers() {
		if s.RawURI == failedURI || w.isBlacklisted(s.RawURI) {
			continue
		}
		if s.Protocol != "" && s.Protocol != "vless" {
			continue
		}
		if xkeen.ServerCountry(s) != country {
			continue
		}
		candidates = append(candidates, s)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("других серверов страны %s в подписке нет", country)
	}

	timeout := time.Duration(w.config.ProbeTimeoutMs) * time.Millisecond
	checked := xkeen.CheckAllLatencies(candidates, timeout, w.config.ProbeConcurrency)

	var live []models.Server
	for _, s := range checked {
		if s.Latency >= 0 {
			live = append(live, s)
		}
	}
	if len(live) == 0 {
		return nil, fmt.Errorf("ни один сервер страны %s не ответил", country)
	}

	sort.SliceStable(live, func(i, j int) bool { return live[i].Latency < live[j].Latency })
	return &live[0], nil
}

// replaceManualPick keeps a failed manual choice inside its country. Returns
// false when there is no such replacement and the pool has to take over.
func (w *Watchdog) replaceManualPick(state xkeen.PoolState) bool {
	if state.PinCountry == "" {
		return false
	}

	failed, known := xkeen.ServerForNode(w.subscription.GetServers(), state.PinnedNode)
	failedName := state.PinnedTag
	if known {
		failedName = failed.Name
		w.blacklistServer(failed.RawURI)
	}

	next, err := w.sameCountryReplacement(state.PinCountry, failed.RawURI)
	if err != nil {
		w.writeLog("[PIN] Замены для %s нет: %v", failedName, err)
		return false
	}

	note := fmt.Sprintf("%s перестал работать — выбран %s той же страны", failedName, next.Name)
	if _, err := w.PinServer(*next, true, note); err != nil {
		w.writeLog("[PIN] Не удалось закрепить %s: %v", next.Name, err)
		return false
	}

	w.writeLog("[PIN] %s", note)
	return true
}
