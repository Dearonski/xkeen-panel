package xkeen

import (
	"path/filepath"
	"testing"
	"time"

	"xkeen-panel/internal/models"
)

func TestDescribePoolNamesNodesAfterServers(t *testing.T) {
	server := &models.Server{ID: 7, Name: "🇳🇱 Amsterdam", Country: "NL", CountryOverride: "DE", Latency: 42}
	nodes := []PoolNode{
		{Tag: "sub-1", Address: "a.example", Port: 443, Server: server},
		{Tag: "sub-2", Address: "b.example", Port: 443},
	}

	views := DescribePool(nodes, nil)

	if views[0].ServerID != 7 || views[0].Name != server.Name || views[0].Latency != 42 {
		t.Errorf("matched node = %+v, want server 7 with its name and latency", views[0])
	}
	if views[0].Country != "DE" {
		t.Errorf("country = %q, want the manual override DE", views[0].Country)
	}
	if views[1].ServerID != -1 || views[1].Name != "sub-2" || views[1].Latency != -1 {
		t.Errorf("unmatched node = %+v, want id -1 named by its tag", views[1])
	}
}

func TestDescribePoolMarksOnlyLiveExclusions(t *testing.T) {
	nodes := []PoolNode{{Tag: "sub-1"}, {Tag: "sub-2"}, {Tag: "sub-3"}}
	excluded := map[string]time.Time{
		"sub-1": time.Now().Add(10 * time.Minute),
		"sub-2": time.Now().Add(-time.Minute),
	}

	views := DescribePool(nodes, excluded)

	if views[0].ExcludedUntil == nil {
		t.Error("sub-1 is still excluded but has no expiry")
	}
	if views[1].ExcludedUntil != nil {
		t.Error("sub-2's exclusion has expired and must not be shown")
	}
	if views[2].ExcludedUntil != nil {
		t.Error("sub-3 was never excluded")
	}
}

// The header must name the server behind the pin, not the subscription entry
// the pool was built from.
func TestServerForNodeFindsPinnedServer(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)
	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	node := NodeKeyForTag(outboundsPath, DefaultPoolSelector, "sub-2")
	if node == "" {
		t.Fatal("sub-2 has no node key")
	}

	server, ok := ServerForNode(poolServers(), node)
	if !ok || server.Name != "DE" {
		t.Errorf("ServerForNode = %+v, %v; want DE", server, ok)
	}

	if _, ok := ServerForNode(poolServers(), ""); ok {
		t.Error("an empty node key must not match anything")
	}
}

func TestPoolTagsByServer(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)
	servers := poolServers()
	for i := range servers {
		servers[i].ID = i
	}
	if _, err := EnablePool(rt, outboundsPath, servers, PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	nodes, err := PoolNodes(outboundsPath, DefaultPoolSelector, servers[1:])
	if err != nil {
		t.Fatalf("PoolNodes: %v", err)
	}

	tags := PoolTagsByServer(nodes)
	if len(tags) != 1 || tags[1] != "sub-2" {
		t.Errorf("tags = %v, want only server 1 on sub-2", tags)
	}
}

func TestPoolStoreManualPin(t *testing.T) {
	store := NewPoolStore(t.TempDir())

	if err := store.DropManual("нода перестала работать"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPinned("sub-1", "a:443:u", true); err != nil {
		t.Fatal(err)
	}
	if got := store.Get(); !got.PinManual || got.PinNote != "" {
		t.Errorf("after a manual pin = %+v, want manual with the old note cleared", got)
	}

	if err := store.DropManual("нода перестала работать"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPinned("sub-2", "b:443:u", false); err != nil {
		t.Fatal(err)
	}
	got := store.Get()
	if got.PinManual || got.PinNote == "" {
		t.Errorf("after the panel re-pinned = %+v, want auto with the note kept for the owner", got)
	}

	reloaded := NewPoolStore(filepath.Dir(store.filePath()))
	if err := reloaded.Load(); err != nil {
		t.Fatal(err)
	}
	if reloaded.Get() != got {
		t.Errorf("reloaded = %+v, want %+v", reloaded.Get(), got)
	}
}
