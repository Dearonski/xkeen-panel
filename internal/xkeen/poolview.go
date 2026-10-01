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
			view.Country = server.CountryOverride
			if view.Country == "" {
				view.Country = server.Country
			}
		}

		if until, ok := excluded[node.Tag]; ok && time.Now().Before(until) {
			until := until
			view.ExcludedUntil = &until
		}

		views = append(views, view)
	}

	return views
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

func PoolTagsByServer(nodes []PoolNode) map[int]string {
	tags := make(map[int]string, len(nodes))
	for _, node := range nodes {
		if node.Server != nil {
			tags[node.Server.ID] = node.Tag
		}
	}
	return tags
}
