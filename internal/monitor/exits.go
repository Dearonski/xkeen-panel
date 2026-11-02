package monitor

import (
	"context"
	"time"

	"xkeen-panel/internal/xkeen"
)

const exitScanTimeout = 12 * time.Second

// Keeps a scan that keeps failing, say on a missing xray binary, from running every minute
const exitScanRetry = 10 * time.Minute

func (w *Watchdog) SetExitStore(store *xkeen.ExitStore) {
	w.exits = store
}

func (w *Watchdog) ExitStore() *xkeen.ExitStore {
	return w.exits
}

func (w *Watchdog) AvoidedExits() map[string]xkeen.AvoidedExit {
	avoided, _ := w.avoidedExits()
	return avoided
}

func (w *Watchdog) avoidedExits() (map[string]xkeen.AvoidedExit, int) {
	avoided := map[string]xkeen.AvoidedExit{}
	if w.exits == nil || w.geoip == nil {
		return avoided, 0
	}
	top := w.detector.Topology()
	if top.Mode != xkeen.TopologyPool {
		return avoided, 0
	}
	layout, err := xkeen.ReadPoolLayout(w.config.OutboundsFile, top.Selector())
	if err != nil {
		return avoided, 0
	}

	for tag, ep := range layout {
		exits, ok := w.exits.Get(ep.Key())
		if !ok {
			continue
		}
		if exit, bad := exits.Avoided(w.geoip); bad {
			avoided[tag] = exit
		}
	}
	return avoided, len(layout)
}

func (w *Watchdog) nodeExitAvoided(node string) bool {
	if w.geoip == nil || node == "" {
		return false
	}
	exits, ok := w.exits.Get(node)
	if !ok {
		return false
	}
	_, bad := exits.Avoided(w.geoip)
	return bad
}

func (w *Watchdog) leaveAvoidedExit(rt xkeen.Runtime, top xkeen.Topology, tag string) bool {
	avoided, poolSize := w.avoidedExits()
	exit, bad := avoided[tag]
	if !bad || len(avoided) >= poolSize {
		return false
	}

	next, err := w.pinBest(rt, top)
	if err != nil {
		w.writeLog("[PIN] Не удалось уйти с %s: %v", tag, err)
		return false
	}
	if next != tag {
		w.writeLog("[PIN] У %s выход через %s из %s (%s) — трафик переведён на %s",
			tag, exit.Via, exit.Country, exit.IP, next)
	}
	return true
}

func (w *Watchdog) RunExitScan(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	var attempted time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if w.exits == nil || w.geoip == nil || time.Since(attempted) < exitScanRetry {
			continue
		}
		rt := w.detector.Runtime()
		top := w.detector.Topology()
		if rt.Core != xkeen.CoreXray || top.Mode != xkeen.TopologyPool {
			continue
		}

		attempted = time.Now()
		sel := xkeen.PoolSelectionFromConfig(w.config, w.geoip, w.exits)
		count, err := xkeen.ScanSubscriptionExits(rt, w.config.OutboundsFile, top.Selector(), w.subscription.GetServers(), sel, w.exits, exitScanTimeout)
		if err != nil {
			w.writeLog("[EXIT] Не удалось проверить выходы серверов: %v", err)
			continue
		}
		if count == 0 {
			continue
		}
		w.writeLog("[EXIT] Проверены выходы серверов подписки: %d", count)

		if avoided := w.AvoidedExits(); len(avoided) > 0 && w.poolStore != nil {
			w.rebuildPoolWithoutAvoided(rt, top, sel, len(avoided))
		}
		for tag, exit := range w.AvoidedExits() {
			w.writeLog("[EXIT] %s выходит через %s из %s (%s) — чистой замены в подписке нет, автовыбор её пропускает", tag, exit.Via, exit.Country, exit.IP)
		}
		w.publishPool()

		if w.IsActive() {
			w.CheckNow()
		}
	}
}

func (w *Watchdog) rebuildPoolWithoutAvoided(rt xkeen.Runtime, top xkeen.Topology, sel xkeen.PoolSelection, avoided int) {
	state := w.poolStore.Get()
	state.BalancerTag = top.BalancerTag
	state.Selector = top.Selector()

	result, err := xkeen.RefreshPool(rt, w.config.OutboundsFile, w.config.XrayAPIAddr, w.subscription.GetServers(), state, sel)
	switch {
	case err != nil:
		w.writeLog("[EXIT] Не удалось пересобрать пул: %v", err)
	case result.Changed:
		w.detector.InvalidateTopology()
		w.writeLog("[EXIT] В пуле было нод с выходом в избегаемой стране: %d — пул пересобран: +%d, -%d, заменено %d",
			avoided, len(result.Added), len(result.Removed), len(result.Replaced))
	}
}
