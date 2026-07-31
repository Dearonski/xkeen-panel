package xkeen

import (
	"testing"

	"xkeen-panel/internal/models"
)

// Syncing rewrites the config and restarts the core, dropping live connections,
// so an unchanged subscription has to be recognised as a no-op.
func TestPoolMatchesSubscription(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)
	servers := poolServers()

	if _, err := EnablePool(rt, outboundsPath, servers, PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	matches, err := PoolMatchesSubscription(outboundsPath, servers, DefaultPoolSelector)
	if err != nil {
		t.Fatalf("PoolMatchesSubscription: %v", err)
	}
	if !matches {
		t.Error("freshly built pool reported as out of sync")
	}
}

func TestPoolMatchesSubscriptionDetectsDrift(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)

	if _, err := EnablePool(rt, outboundsPath, poolServers(), PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	cases := map[string][]models.Server{
		"server removed": poolServers()[:1],
		"server added": append(poolServers(), models.Server{
			Protocol: "vless",
			RawURI:   "vless://33333333-4444-5555-6666-777777777777@new.example:443?type=tcp&security=reality",
		}),
		"different endpoint": {
			{Protocol: "vless", RawURI: "vless://11111111-2222-3333-4444-555555555555@other.example:443?type=tcp"},
			{Protocol: "vless", RawURI: wsURI},
		},
	}

	for name, servers := range cases {
		t.Run(name, func(t *testing.T) {
			matches, err := PoolMatchesSubscription(outboundsPath, servers, DefaultPoolSelector)
			if err != nil {
				t.Fatalf("PoolMatchesSubscription: %v", err)
			}
			if matches {
				t.Error("drift not detected — the pool would never be resynced")
			}
		})
	}
}

// Non-VLESS entries are skipped when building the pool, so they must be skipped
// when comparing too, or the pool would look permanently out of sync.
func TestPoolMatchesSubscriptionIgnoresNonVLESS(t *testing.T) {
	rt, outboundsPath := liveConfDir(t)
	servers := poolServers()

	if _, err := EnablePool(rt, outboundsPath, servers, PoolOptions{}); err != nil {
		t.Fatalf("EnablePool: %v", err)
	}

	withNoise := append(servers, models.Server{Protocol: "shadowsocks", RawURI: "ss://abc@h:443"})

	matches, err := PoolMatchesSubscription(outboundsPath, withNoise, DefaultPoolSelector)
	if err != nil {
		t.Fatalf("PoolMatchesSubscription: %v", err)
	}
	if !matches {
		t.Error("a non-VLESS server made the pool look out of sync")
	}
}
