package xkeen

import (
	"os"
	"testing"

	"xkeen-panel/internal/models"
)

func serverAt(host string) models.Server {
	return models.Server{
		Name:     host,
		Protocol: "vless",
		Address:  host,
		Port:     443,
		RawURI:   "vless://11111111-2222-3333-4444-555555555555@" + host + ":443?type=raw&security=reality&pbk=KEY",
	}
}

// Candidates are ranked by measured latency, which jitters between runs. When
// the comparison was order-sensitive, that jitter reported a drift on nearly
// every refresh — and each "drift" rewrote the whole pool.
func TestPoolMatchesSubscriptionIgnoresOrder(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)
	servers := []models.Server{serverAt("a.example"), serverAt("b.example"), serverAt("c.example")}

	if _, err := EnablePool(rt, outboundsPath, servers, PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	shuffled := []models.Server{servers[2], servers[0], servers[1]}

	matches, err := PoolMatchesSubscription(outboundsPath, shuffled, DefaultPoolSelector)
	if err != nil {
		t.Fatalf("PoolMatchesSubscription: %v", err)
	}
	if !matches {
		t.Error("reordering alone reported as drift — this is what rebuilt the pool every 30 minutes")
	}
}

func TestPoolMatchesSubscriptionStillSeesRealChange(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)
	servers := []models.Server{serverAt("a.example"), serverAt("b.example")}

	if _, err := EnablePool(rt, outboundsPath, servers, PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	swapped := []models.Server{serverAt("a.example"), serverAt("z.example")}

	matches, err := PoolMatchesSubscription(outboundsPath, swapped, DefaultPoolSelector)
	if err != nil {
		t.Fatalf("PoolMatchesSubscription: %v", err)
	}
	if matches {
		t.Error("a genuinely swapped server must still count as drift")
	}
}

// A server already in the pool keeps its tag. Otherwise a reshuffle silently
// makes every tag mean a different server — including the pinned one.
func TestAssignTagsKeepsExistingTags(t *testing.T) {
	existing := PoolLayout{
		"sub-1": {Address: "a.example", Port: 443, UUID: "11111111-2222-3333-4444-555555555555"},
		"sub-2": {Address: "b.example", Port: 443, UUID: "11111111-2222-3333-4444-555555555555"},
		"sub-3": {Address: "c.example", Port: 443, UUID: "11111111-2222-3333-4444-555555555555"},
	}

	// Same three servers, latency put them in a different order this time
	reordered := []models.Server{serverAt("c.example"), serverAt("a.example"), serverAt("b.example")}

	got := assignTags(reordered, DefaultPoolSelector, existing)

	want := map[string]string{"sub-1": "a.example", "sub-2": "b.example", "sub-3": "c.example"}
	for _, entry := range got {
		if want[entry.Tag] != entry.Endpoint.Address {
			t.Errorf("%s = %s, want %s — the tag must follow the server", entry.Tag, entry.Endpoint.Address, want[entry.Tag])
		}
	}
}

// A newcomer takes the slot the departing server left behind.
func TestAssignTagsFillsFreedSlot(t *testing.T) {
	existing := PoolLayout{
		"sub-1": {Address: "a.example", Port: 443, UUID: "11111111-2222-3333-4444-555555555555"},
		"sub-2": {Address: "b.example", Port: 443, UUID: "11111111-2222-3333-4444-555555555555"},
	}

	servers := []models.Server{serverAt("a.example"), serverAt("z.example")}

	byTag := map[string]string{}
	for _, entry := range assignTags(servers, DefaultPoolSelector, existing) {
		byTag[entry.Tag] = entry.Endpoint.Address
	}

	if byTag["sub-1"] != "a.example" {
		t.Errorf("sub-1 = %s, want the surviving server to stay put", byTag["sub-1"])
	}
	if byTag["sub-2"] != "z.example" {
		t.Errorf("sub-2 = %s, want the newcomer in the freed slot", byTag["sub-2"])
	}
}

// The whole point: a refresh that changes nothing real must touch nothing in
// the running core, because replacing an outbound closes its connections.
func TestRefreshPoolTouchesNothingOnReorder(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)
	servers := []models.Server{serverAt("a.example"), serverAt("b.example"), serverAt("c.example")}

	state, err := EnablePool(rt, outboundsPath, servers, PoolOptions{})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	before, _ := os.ReadFile(outboundsPath)

	result, err := RefreshPool(rt, outboundsPath, "", []models.Server{servers[1], servers[2], servers[0]}, state, PoolSelection{})
	if err != nil {
		t.Fatalf("RefreshPool: %v", err)
	}

	if result.Changed {
		t.Errorf("result = %+v, want no change for a pure reorder", result)
	}
	if after, _ := os.ReadFile(outboundsPath); string(after) != string(before) {
		t.Error("config rewritten for a pure reorder")
	}
}

// When one server really is swapped out, only that tag is disturbed.
func TestRefreshPoolReplacesOnlyTheChangedTag(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)
	servers := []models.Server{serverAt("a.example"), serverAt("b.example"), serverAt("c.example")}

	state, err := EnablePool(rt, outboundsPath, servers, PoolOptions{})
	if err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	// b leaves, z arrives; a and c are untouched
	updated := []models.Server{serverAt("a.example"), serverAt("z.example"), serverAt("c.example")}

	result, err := RefreshPool(rt, outboundsPath, "", updated, state, PoolSelection{})
	if err != nil {
		t.Fatalf("RefreshPool: %v", err)
	}
	if !result.Changed {
		t.Fatal("a real swap must count as change")
	}

	if len(result.Replaced) != 1 || result.Replaced[0] != "sub-2" {
		t.Errorf("Replaced = %v, want only sub-2 — the others carry live traffic", result.Replaced)
	}

	layout, _ := ReadPoolLayout(outboundsPath, DefaultPoolSelector)
	if layout["sub-1"].Address != "a.example" || layout["sub-3"].Address != "c.example" {
		t.Errorf("untouched tags moved: %+v", layout)
	}
	if layout["sub-2"].Address != "z.example" {
		t.Errorf("sub-2 = %s, want the replacement", layout["sub-2"].Address)
	}
}

// A live pool member is not evicted because a measurement wobbled.
func TestSelectPoolServersKeepsIncumbents(t *testing.T) {
	incumbent := serverAt("incumbent.example")
	incumbent.Latency = 90

	newcomer := serverAt("newcomer.example")
	newcomer.Latency = 10

	ep, _ := endpointOfServer(incumbent)
	sel := PoolSelection{MaxNodes: 1, Keep: map[endpoint]bool{ep: true}}

	// Both already carry latencies, so no probing is needed
	got := sel.incumbentsFirst([]models.Server{newcomer, incumbent})

	if len(got) == 0 || got[0].Address != "incumbent.example" {
		t.Errorf("first = %+v, want the incumbent to hold its slot", got)
	}
}

// A dead incumbent has no claim on its slot.
func TestSelectPoolServersDropsDeadIncumbent(t *testing.T) {
	dead := serverAt("dead.example")
	dead.Latency = -1

	alive := serverAt("alive.example")
	alive.Latency = 50

	ep, _ := endpointOfServer(dead)
	sel := PoolSelection{MaxNodes: 1, Keep: map[endpoint]bool{ep: true}}

	got := sel.incumbentsFirst([]models.Server{alive, dead})

	if len(got) == 0 || got[0].Address != "alive.example" {
		t.Errorf("first = %+v, want the reachable server", got)
	}
}
