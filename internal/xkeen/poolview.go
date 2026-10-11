package xkeen

import (
	"time"

	"xkeen-panel/internal/models"
)

func (t Topology) Selector() string {
	if len(t.Selectors) > 0 {
		return t.Selectors[0]
	}
	return DefaultPoolSelector
}

type PoolNodeView struct {
	Tag           string     `json:"tag"`
	ServerID      int        `json:"server_id"` // -1 when the node is not in the subscription
	Name          string     `json:"name"`
	Address       string     `json:"address"`
	Port          int        `json:"port"`
	Country       string     `json:"country,omitempty"`
	Latency       int        `json:"latency_ms"`
	ExcludedUntil *time.Time `json:"excluded_until,omitempty"`
}

func DescribePool(nodes []PoolNode, excluded map[string]time.Time) []PoolNodeView {
	views := make([]PoolNodeView, 0, len(nodes))

	for _, node := range nodes {
		view := PoolNodeView{
			Tag:      node.Tag,
			ServerID: -1,
			Name:     node.Tag,
			Address:  node.Address,
			Port:     node.Port,
			Latency:  -1,
		}

		if server := node.Server; server != nil {
			view.ServerID = server.ID
			view.Name = server.Name
			view.Latency = server.Latency
			view.Country = ServerCountry(*server)
		}

		if until, ok := excluded[node.Tag]; ok && time.Now().Before(until) {
			until := until
			view.ExcludedUntil = &until
		}

		views = append(views, view)
	}

	return views
}

// ServerCountry is the owner's override when set, otherwise the country read
// from the server name.
func ServerCountry(server models.Server) string {
	if server.CountryOverride != "" {
		return server.CountryOverride
	}
	return server.Country
}

func ServerForNode(servers []models.Server, node string) (models.Server, bool) {
	if node == "" {
		return models.Server{}, false
	}
	for _, server := range servers {
		if ep, ok := endpointOfServer(server); ok && ep.Key() == node {
			return server, true
		}
	}
	return models.Server{}, false
}

// PoolTagsByNode maps node keys to pool tags. Keyed by endpoint, not server ID:
// a subscription can list one node under several names, and each of them is
// that pool member.
func PoolTagsByNode(nodes []PoolNode) map[string]string {
	tags := make(map[string]string, len(nodes))
	for _, node := range nodes {
		tags[endpoint{Address: node.Address, Port: node.Port, UUID: node.UUID}.Key()] = node.Tag
	}
	return tags
}

// NodeKeyOfServer is the identity a pin stores for this server, empty when the
// server has no VLESS endpoint.
func NodeKeyOfServer(server models.Server) string {
	if ep, ok := endpointOfServer(server); ok {
		return ep.Key()
	}
	return ""
}
