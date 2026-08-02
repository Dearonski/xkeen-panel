package xkeen

import (
	"testing"

	"xkeen-panel/internal/models"
)

// The pool holds only the best N of the subscription, so nodes have to be
// matched to servers by endpoint — a subscription index says nothing about
// which node, if any, carries that server.
func TestPoolNodesMatchByEndpoint(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	// Only the second subscription server is offered for matching
	nodes, err := PoolNodes(outboundsPath, DefaultPoolSelector, poolServers()[1:])
	if err != nil {
		t.Fatalf("PoolNodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(nodes))
	}

	matched := 0
	for _, node := range nodes {
		if node.Server == nil {
			continue
		}
		matched++
		if node.Tag != "sub-2" {
			t.Errorf("matched tag = %q, want sub-2 — the second server is node 2", node.Tag)
		}
	}
	if matched != 1 {
		t.Errorf("matched %d nodes, want exactly 1", matched)
	}
}

func TestPoolNodesReadsEndpoints(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	nodes, err := PoolNodes(outboundsPath, DefaultPoolSelector, poolServers())
	if err != nil {
		t.Fatalf("PoolNodes: %v", err)
	}

	for _, node := range nodes {
		if node.Address == "" || node.Port == 0 || node.UUID == "" {
			t.Errorf("node %s has an incomplete endpoint: %+v", node.Tag, node)
		}
		if node.Server == nil {
			t.Errorf("node %s did not match any subscription server", node.Tag)
		}
	}
}

// A node the health check condemned must not be picked again while excluded.
func TestPinBestNodeSkipsExcluded(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	// No dispatcher in the fixture, so the override call fails — but the choice
	// is made before that, and the error names the tag it tried
	_, err := PinBestNode(rt, "", outboundsPath, Topology{BalancerTag: "balancer"},
		poolServers(), map[string]bool{"sub-1": true}, 0, 1)
	if err == nil {
		t.Skip("override unexpectedly succeeded without a core")
	}
	if got := err.Error(); !contains(got, "sub-2") {
		t.Errorf("error = %q, want it to name sub-2 — sub-1 was excluded", got)
	}
}

// Excluding everything must not leave the router with no exit at all.
func TestPinBestNodeFallsBackWhenAllExcluded(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	_, err := PinBestNode(rt, "", outboundsPath, Topology{BalancerTag: "balancer"},
		poolServers(), map[string]bool{"sub-1": true, "sub-2": true}, 0, 1)

	if err == nil {
		t.Skip("override unexpectedly succeeded without a core")
	}
	if got := err.Error(); !contains(got, "sub-") {
		t.Errorf("error = %q, want a node to have been chosen anyway", got)
	}
}

func TestPinBestNodeEmptyPool(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	if _, err := PinBestNode(rt, "", outboundsPath, Topology{BalancerTag: "balancer"},
		[]models.Server{}, nil, 0, 1); err == nil {
		t.Error("expected an error when the config holds no pool")
	}
}

// A pin is meaningless without a pool.
func TestEnsurePinnedIgnoresSingleMode(t *testing.T) {
	changed, err := EnsurePinned(Runtime{}, "127.0.0.1:10085", Topology{Mode: TopologySingle}, "sub-1")

	if changed || err != nil {
		t.Errorf("changed=%v err=%v, want a no-op outside pool mode", changed, err)
	}
}

func TestEnsurePinnedIgnoresEmptyTag(t *testing.T) {
	changed, err := EnsurePinned(Runtime{}, "127.0.0.1:10085", Topology{Mode: TopologyPool}, "")

	if changed || err != nil {
		t.Errorf("changed=%v err=%v, want a no-op with nothing pinned", changed, err)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
