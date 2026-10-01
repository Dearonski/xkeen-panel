package monitor

import (
	"testing"
	"time"

	"xkeen-panel/internal/models"
	"xkeen-panel/internal/xkeen"
)

func pooledWatchdog(t *testing.T, state xkeen.PoolState) (*Watchdog, *xkeen.PoolStore) {
	t.Helper()
	w := newWatchdog(t, &models.Config{OutboundsFile: t.TempDir() + "/missing.json"})
	store := xkeen.NewPoolStore(t.TempDir())
	if err := store.Set(state); err != nil {
		t.Fatal(err)
	}
	w.SetPoolStore(store)
	return w, store
}

var poolTopology = xkeen.Topology{Mode: xkeen.TopologyPool, BalancerTag: "balancer"}

// A node picked by hand that stops working is given up: traffic matters more
// than the choice, and the owner is told why.
func TestRotateExitDropsManualPin(t *testing.T) {
	w, store := pooledWatchdog(t, xkeen.PoolState{Enabled: true, PinnedTag: "sub-3", PinManual: true})

	w.rotateExit(xkeen.Runtime{}, poolTopology)

	got := store.Get()
	if got.PinManual {
		t.Error("manual pin survived a failed exit")
	}
	if got.PinNote == "" {
		t.Error("the owner is not told why the manual pin was dropped")
	}
	if _, condemned := w.ExcludedNodes()["sub-3"]; !condemned {
		t.Error("the failed node was not excluded")
	}
}

func TestSuperviseDropsManualPinThatLeftPool(t *testing.T) {
	w, store := pooledWatchdog(t, xkeen.PoolState{Enabled: true, PinnedTag: "sub-3", PinnedNode: "gone:443:u", PinManual: true})

	w.superviseP(xkeen.Runtime{}, poolTopology)

	if got := store.Get(); got.PinManual || got.PinNote == "" {
		t.Errorf("state = %+v, want the manual pin dropped with a note", got)
	}
}

func TestNodePinnedLiftsExclusion(t *testing.T) {
	w, _ := pooledWatchdog(t, xkeen.PoolState{Enabled: true})
	w.badNodes["sub-2"] = time.Now().Add(time.Hour)

	w.NodePinned("sub-2")

	if _, condemned := w.ExcludedNodes()["sub-2"]; condemned {
		t.Error("a node the owner picked by hand is still excluded")
	}
}

func TestExcludedNodesHidesExpired(t *testing.T) {
	w, _ := pooledWatchdog(t, xkeen.PoolState{})
	w.badNodes["sub-1"] = time.Now().Add(time.Hour)
	w.badNodes["sub-2"] = time.Now().Add(-time.Second)

	got := w.ExcludedNodes()
	if _, ok := got["sub-1"]; !ok || len(got) != 1 {
		t.Errorf("excluded = %v, want only sub-1", got)
	}
}
