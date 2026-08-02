package xkeen

import (
	"fmt"
	"log"
	"sort"
	"time"

	"xkeen-panel/internal/models"
)

// PoolNode ties a pool outbound to the subscription server behind it.
type PoolNode struct {
	Tag     string
	Address string
	Port    int
	UUID    string
	Server  *models.Server // nil when the node is not in the current subscription
	Latency int
}

// PoolNodes reads the pool from the config and matches every node against the
// subscription, so callers can reason about nodes in terms of servers.
func PoolNodes(outboundsPath, selector string, servers []models.Server) ([]PoolNode, error) {
	if selector == "" {
		selector = DefaultPoolSelector
	}

	raw, err := poolNodesInFile(outboundsPath, selector)
	if err != nil {
		return nil, err
	}

	byEndpoint := make(map[[3]interface{}]*models.Server, len(servers))
	for i := range servers {
		params, err := ParseVLESS(servers[i].RawURI)
		if err != nil {
			continue
		}
		byEndpoint[[3]interface{}{params.Address, params.Port, params.UUID}] = &servers[i]
	}

	nodes := make([]PoolNode, 0, len(raw))
	for _, ob := range raw {
		address, port, uuid, ok := readProxyEndpoint(ob)
		if !ok {
			continue
		}
		tag, _ := ob["tag"].(string)

		node := PoolNode{Tag: tag, Address: address, Port: port, UUID: uuid, Latency: -1}
		if server, found := byEndpoint[[3]interface{}{address, port, uuid}]; found {
			node.Server = server
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// PinBestNode measures the pool and pins the fastest node the caller has not
// excluded, returning its tag.
//
// Pinning is what makes a pool usable for everyday traffic: a balancer picks an
// outbound per connection, so without an override the outgoing IP moves between
// nodes and anything IP-bound — Telegram sessions, CDN anti-abuse — breaks.
func PinBestNode(rt Runtime, apiAddr, outboundsPath string, top Topology, servers []models.Server, excluded map[string]bool, probeTimeout time.Duration, concurrency int) (string, error) {
	selector := DefaultPoolSelector
	if len(top.Selectors) > 0 {
		selector = top.Selectors[0]
	}

	nodes, err := PoolNodes(outboundsPath, selector, servers)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("в пуле нет нод")
	}

	candidates := make([]models.Server, 0, len(nodes))
	tagOf := make(map[string]string, len(nodes))
	for _, node := range nodes {
		if excluded[node.Tag] {
			continue
		}
		candidates = append(candidates, models.Server{
			Name:    node.Tag,
			Address: node.Address,
			Port:    node.Port,
			RawURI:  node.Tag,
		})
		tagOf[node.Tag] = node.Tag
	}

	// Everything is excluded — better a suspect node than no traffic at all
	if len(candidates) == 0 {
		log.Printf("[PIN] Все ноды исключены — снимаю исключения")
		for _, node := range nodes {
			candidates = append(candidates, models.Server{Name: node.Tag, Address: node.Address, Port: node.Port, RawURI: node.Tag})
		}
	}

	if probeTimeout > 0 {
		checked := CheckAllLatencies(candidates, probeTimeout, concurrency)
		sort.SliceStable(checked, func(i, j int) bool {
			switch {
			case checked[i].Latency < 0:
				return false
			case checked[j].Latency < 0:
				return true
			default:
				return checked[i].Latency < checked[j].Latency
			}
		})
		candidates = checked
	}

	best := candidates[0].Name
	if err := OverrideBalancerTarget(rt, apiAddr, top.BalancerTag, best); err != nil {
		return "", err
	}

	return best, nil
}

// EnsurePinned re-applies the stored pin when the core lost it.
//
// A balancer override lives only in the core's memory and has no TTL, so every
// Xray restart silently returns the pool to per-connection selection — the exact
// behaviour pinning exists to avoid.
func EnsurePinned(rt Runtime, apiAddr string, top Topology, wanted string) (bool, error) {
	if wanted == "" || top.Mode != TopologyPool {
		return false, nil
	}

	current, err := CurrentBalancerTarget(rt, apiAddr, top.BalancerTag)
	if err != nil {
		return false, err
	}
	if current == wanted {
		return false, nil
	}

	if err := OverrideBalancerTarget(rt, apiAddr, top.BalancerTag, wanted); err != nil {
		return false, err
	}

	return true, nil
}
