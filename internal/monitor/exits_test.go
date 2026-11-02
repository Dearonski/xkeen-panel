package monitor

import (
	"os"
	"strings"
	"testing"
	"time"

	"xkeen-panel/internal/geoip"
	"xkeen-panel/internal/models"
	"xkeen-panel/internal/xkeen"
)

// exitWatchdog builds a two-node pool whose nodes exit from the given IPs; 1.2.3.x is Russian in the test database.
func exitWatchdog(t *testing.T, exit1, exit2 string, state xkeen.PoolState) (*Watchdog, *xkeen.PoolStore, string) {
	t.Helper()
	w, store, calls := routerWatchdog(t, []models.Server{
		{Name: "NL", Protocol: "vless", RawURI: nlURI},
		{Name: "FI", Protocol: "vless", RawURI: fiURI},
	})

	err := os.WriteFile(w.config.OutboundsFile, []byte(`{"outbounds":[
		{"tag":"sub-1","protocol":"vless","settings":{"address":"nl.example","port":443,"id":"11111111-2222-3333-4444-555555555555"}},
		{"tag":"sub-2","protocol":"vless","settings":{"address":"fi.example","port":443,"id":"33333333-4444-5555-6666-777777777777"}},
		{"tag":"direct","protocol":"freedom"}]}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	matcher, err := geoip.Load(buildGeoIP(t), []string{"RU"})
	if err != nil {
		t.Fatal(err)
	}
	w.SetGeoIP(matcher)

	layout, err := xkeen.ReadPoolLayout(w.config.OutboundsFile, "sub-")
	if err != nil {
		t.Fatal(err)
	}
	exits := xkeen.NewExitStore(t.TempDir())
	scan := func(ip string) xkeen.NodeExits {
		return xkeen.NodeExits{Exits: []xkeen.NodeExit{{Via: "Google Cloud", IP: ip}}, CheckedAt: time.Now()}
	}
	if err := exits.Replace(layout, map[string]xkeen.NodeExits{
		layout["sub-1"].Key(): scan(exit1),
		layout["sub-2"].Key(): scan(exit2),
	}); err != nil {
		t.Fatal(err)
	}
	w.SetExitStore(exits)

	if state.PinnedTag != "" {
		state.PinnedNode = layout[state.PinnedTag].Key()
	}
	if err := store.Set(state); err != nil {
		t.Fatal(err)
	}
	return w, store, calls
}

func TestAutoPinLeavesAvoidedExit(t *testing.T) {
	w, store, calls := exitWatchdog(t, "1.2.3.5", "8.8.8.8", xkeen.PoolState{Enabled: true, PinnedTag: "sub-1"})

	w.superviseP(w.detector.Runtime(), w.detector.Topology())

	if got := store.Get(); got.PinnedTag != "sub-2" || got.PinManual {
		t.Errorf("state = %+v, want an automatic pin on sub-2", got)
	}
	if log := readCalls(t, calls); !strings.Contains(log, "-b balancer sub-2") {
		t.Errorf("the balancer was not moved to sub-2:\n%s", log)
	}
}

func TestManualPinOnAvoidedExitStays(t *testing.T) {
	w, store, _ := exitWatchdog(t, "1.2.3.5", "8.8.8.8", xkeen.PoolState{Enabled: true, PinnedTag: "sub-1", PinManual: true})

	w.superviseP(w.detector.Runtime(), w.detector.Topology())

	if got := store.Get(); got.PinnedTag != "sub-1" || !got.PinManual {
		t.Errorf("state = %+v, want the owner's pick kept", got)
	}
	if exit, ok := w.AvoidedExits()["sub-1"]; !ok || exit.Country != "RU" || exit.Via != "Google Cloud" {
		t.Errorf("AvoidedExits = %v, want sub-1 flagged for the pool view", w.AvoidedExits())
	}
}

// With every exit in an avoided country a pin on one of them beats churning between them
func TestAllExitsAvoidedKeepsPin(t *testing.T) {
	w, store, _ := exitWatchdog(t, "1.2.3.5", "1.2.3.6", xkeen.PoolState{Enabled: true, PinnedTag: "sub-1"})

	w.superviseP(w.detector.Runtime(), w.detector.Topology())

	if got := store.Get(); got.PinnedTag != "sub-1" {
		t.Errorf("state = %+v, want the pin left on sub-1", got)
	}
}

func TestPinBestSkipsAvoidedExit(t *testing.T) {
	w, store, _ := exitWatchdog(t, "1.2.3.5", "8.8.8.8", xkeen.PoolState{Enabled: true})

	tag, err := w.PinBest()
	if err != nil {
		t.Fatal(err)
	}
	if tag != "sub-2" || store.Get().PinnedTag != "sub-2" {
		t.Errorf("picked %s, want sub-2", tag)
	}
}
