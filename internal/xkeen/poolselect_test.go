package xkeen

import (
	"fmt"
	"net"
	"testing"
	"time"

	"xkeen-panel/internal/models"
)

func vlessAt(name, country, host string, port int) models.Server {
	return models.Server{
		Name:     name,
		Country:  country,
		Protocol: "vless",
		Address:  host,
		Port:     port,
		RawURI:   fmt.Sprintf("vless://11111111-2222-3333-4444-555555555555@%s:%d?type=raw&security=reality&pbk=KEY", host, port),
	}
}

// The balancer routes through whatever sits in the pool, so a server from an
// avoided country must never get there — auto_switch_avoid_countries has to
// hold for pool mode too, not just for single-outbound switching.
func TestSelectPoolServersDropsAvoidedCountries(t *testing.T) {
	servers := []models.Server{
		vlessAt("NL-1", "NL", "nl.example", 443),
		vlessAt("RU-1", "RU", "ru.example", 443),
		vlessAt("BY-1", "BY", "by.example", 443),
		vlessAt("DE-1", "DE", "de.example", 443),
	}

	got := SelectPoolServers(servers, PoolSelection{AvoidCountries: []string{"RU", "BY"}})

	if len(got) != 2 {
		t.Fatalf("selected %d servers, want 2", len(got))
	}
	for _, server := range got {
		if server.Country == "RU" || server.Country == "BY" {
			t.Errorf("%s (%s) must not be in the pool", server.Name, server.Country)
		}
	}
}

// A manual override wins over the detected country.
func TestSelectPoolServersHonoursCountryOverride(t *testing.T) {
	server := vlessAt("mystery", "", "x.example", 443)
	server.CountryOverride = "ru"

	if got := SelectPoolServers([]models.Server{server}, PoolSelection{AvoidCountries: []string{"RU"}}); len(got) != 0 {
		t.Errorf("selected %v, want none", got)
	}
}

func TestSelectPoolServersSkipsNonVLESS(t *testing.T) {
	servers := []models.Server{
		vlessAt("NL-1", "NL", "nl.example", 443),
		{Name: "ss", Protocol: "shadowsocks", RawURI: "ss://x@h:1"},
		{Name: "no-uri", Protocol: "vless"},
	}

	if got := SelectPoolServers(servers, PoolSelection{}); len(got) != 1 {
		t.Errorf("selected %d, want only the VLESS entry", len(got))
	}
}

// Every node is probed by observatory separately, so the pool is capped.
func TestSelectPoolServersCapsPoolSize(t *testing.T) {
	var servers []models.Server
	for i := range 30 {
		servers = append(servers, vlessAt(fmt.Sprintf("s%d", i), "NL", fmt.Sprintf("h%d.example", i), 443))
	}

	got := SelectPoolServers(servers, PoolSelection{MaxNodes: 5, ProbeTimeout: 10 * time.Millisecond})
	if len(got) != 5 {
		t.Fatalf("selected %d, want 5", len(got))
	}

	got = SelectPoolServers(servers, PoolSelection{ProbeTimeout: 10 * time.Millisecond})
	if len(got) != DefaultPoolMaxNodes {
		t.Errorf("selected %d with no cap set, want the default %d", len(got), DefaultPoolMaxNodes)
	}
}

// Reachable servers rank ahead of unreachable ones, closest first.
func TestSelectPoolServersPrefersLiveNodes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port := 0
	fmt.Sscanf(portText, "%d", &port)

	servers := []models.Server{
		vlessAt("dead-1", "NL", "192.0.2.1", 9), // TEST-NET-1, guaranteed unroutable
		vlessAt("live", "NL", host, port),
		vlessAt("dead-2", "NL", "192.0.2.2", 9),
	}

	got := SelectPoolServers(servers, PoolSelection{
		MaxNodes:         2,
		ProbeTimeout:     300 * time.Millisecond,
		ProbeConcurrency: 4,
	})

	if len(got) == 0 || got[0].Name != "live" {
		t.Errorf("first pick = %v, want the reachable server first", got)
	}
}

// A failed probe may mean the router's own uplink is down — which is exactly
// when a pool rebuild happens — so an empty pool must never be the result.
func TestSelectPoolServersKeepsUnreachableAsFallback(t *testing.T) {
	servers := []models.Server{
		vlessAt("dead-1", "NL", "192.0.2.1", 9),
		vlessAt("dead-2", "NL", "192.0.2.2", 9),
	}

	got := SelectPoolServers(servers, PoolSelection{
		MaxNodes:         5,
		ProbeTimeout:     200 * time.Millisecond,
		ProbeConcurrency: 2,
	})

	if len(got) != 2 {
		t.Errorf("selected %d, want both kept when nothing answers", len(got))
	}
}

func TestSelectPoolServersEmptyWhenAllAvoided(t *testing.T) {
	servers := []models.Server{vlessAt("RU-1", "RU", "ru.example", 443)}

	if got := SelectPoolServers(servers, PoolSelection{AvoidCountries: []string{"RU"}}); got != nil {
		t.Errorf("selected %v, want nothing", got)
	}
}

// Probe load scales with pool size, so the interval has to scale with it.
func TestProbeIntervalScalesWithPoolSize(t *testing.T) {
	cases := map[int]string{5: "1m", 20: "1m", 21: "3m", 40: "3m", 81: "5m"}

	for nodes, want := range cases {
		if got := probeIntervalFor(nodes); got != want {
			t.Errorf("probeIntervalFor(%d) = %q, want %q", nodes, got, want)
		}
	}
}
