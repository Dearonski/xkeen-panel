package xkeen

import (
	"log"
	"sort"
	"strings"
	"time"

	"xkeen-panel/internal/geoip"
	"xkeen-panel/internal/models"
)

// DefaultPoolMaxNodes caps how many subscription servers end up in a pool.
//
// Every node is probed separately by observatory, so a pool built from a large
// subscription turns into constant VLESS handshakes on a router that has better
// things to do. A handful of the closest live nodes is what leastPing needs.
const DefaultPoolMaxNodes = 10

// PoolSelection decides which subscription servers a pool is built from.
type PoolSelection struct {
	MaxNodes         int
	AvoidCountries   []string
	GeoIP            *geoip.Matcher
	ProbeTimeout     time.Duration
	ProbeConcurrency int
}

// SelectPoolServers filters and ranks the subscription for pool membership:
// VLESS only, no avoided countries, live first, closest first.
//
// The country filter matters because the balancer picks the node — an RU server
// left in the pool is one the balancer may route through, whatever
// auto_switch_avoid_countries says.
func SelectPoolServers(servers []models.Server, sel PoolSelection) []models.Server {
	max := sel.MaxNodes
	if max <= 0 {
		max = DefaultPoolMaxNodes
	}

	var allowed []models.Server
	for _, server := range servers {
		if server.RawURI == "" || (server.Protocol != "" && server.Protocol != "vless") {
			continue
		}
		if sel.avoided(server) {
			continue
		}
		allowed = append(allowed, server)
	}

	if len(allowed) == 0 {
		return nil
	}
	if len(allowed) <= max && sel.ProbeTimeout <= 0 {
		return allowed
	}

	ranked := sel.rankByLatency(allowed)
	if len(ranked) > max {
		ranked = ranked[:max]
	}

	return ranked
}

// rankByLatency probes the candidates and puts the responding ones first.
//
// A probe failure is not proof a server is dead — the router's own uplink may be
// down, which is exactly when a pool rebuild happens — so unreachable servers
// are kept as a tail rather than dropped. An empty pool would be worse.
func (sel PoolSelection) rankByLatency(servers []models.Server) []models.Server {
	if sel.ProbeTimeout <= 0 {
		return servers
	}

	checked := CheckAllLatencies(servers, sel.ProbeTimeout, sel.ProbeConcurrency)

	var live, dead []models.Server
	for _, server := range checked {
		if server.Latency >= 0 {
			live = append(live, server)
		} else {
			dead = append(dead, server)
		}
	}

	sort.SliceStable(live, func(i, j int) bool { return live[i].Latency < live[j].Latency })

	if len(live) == 0 {
		log.Printf("[POOL] Ни один сервер не ответил на пинг — беру список как есть")
		return dead
	}

	return append(live, dead...)
}

func (sel PoolSelection) avoided(server models.Server) bool {
	country := server.CountryOverride
	if country == "" {
		country = server.Country
	}
	if country != "" && sel.isAvoidedCode(country) {
		return true
	}

	// GeoIP is authoritative when the name says nothing useful
	if sel.GeoIP != nil {
		if code, resolved := sel.GeoIP.Inspect(server.Address); resolved && code != "" {
			return true
		}
	}

	return false
}

func (sel PoolSelection) isAvoidedCode(code string) bool {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, avoided := range sel.AvoidCountries {
		if strings.ToUpper(strings.TrimSpace(avoided)) == code {
			return true
		}
	}
	return false
}

// PoolSelectionFromConfig builds the selection rules out of the panel config.
func PoolSelectionFromConfig(cfg *models.Config, matcher *geoip.Matcher) PoolSelection {
	return PoolSelection{
		MaxNodes:         cfg.PoolMaxNodes,
		AvoidCountries:   cfg.AutoSwitchAvoidCountries,
		GeoIP:            matcher,
		ProbeTimeout:     time.Duration(cfg.ProbeTimeoutMs) * time.Millisecond,
		ProbeConcurrency: cfg.ProbeConcurrency,
	}
}
