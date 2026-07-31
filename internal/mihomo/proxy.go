package mihomo

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"xkeen-panel/internal/xkeen"
)

// vlessProxy is Mihomo's shape of a VLESS node. Field order here is the order
// written to the file, so it stays readable when the owner opens it.
type vlessProxy struct {
	Name              string       `yaml:"name"`
	Type              string       `yaml:"type"`
	Server            string       `yaml:"server"`
	Port              int          `yaml:"port"`
	UUID              string       `yaml:"uuid"`
	Network           string       `yaml:"network,omitempty"`
	UDP               bool         `yaml:"udp"`
	TLS               bool         `yaml:"tls,omitempty"`
	Servername        string       `yaml:"servername,omitempty"`
	Flow              string       `yaml:"flow,omitempty"`
	ClientFingerprint string       `yaml:"client-fingerprint,omitempty"`
	ALPN              []string     `yaml:"alpn,omitempty"`
	RealityOpts       *realityOpts `yaml:"reality-opts,omitempty"`
	WSOpts            *wsOpts      `yaml:"ws-opts,omitempty"`
	GRPCOpts          *grpcOpts    `yaml:"grpc-opts,omitempty"`
	RoutingMark       int          `yaml:"routing-mark,omitempty"`
}

type realityOpts struct {
	PublicKey string `yaml:"public-key"`
	ShortID   string `yaml:"short-id,omitempty"`
}

type wsOpts struct {
	Path    string            `yaml:"path,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

type grpcOpts struct {
	ServiceName string `yaml:"grpc-service-name,omitempty"`
}

// proxyNode converts a parsed VLESS URI into a Mihomo proxy entry.
func proxyNode(name string, p *xkeen.VLESSParams, routingMark int) (*yaml.Node, error) {
	proxy := vlessProxy{
		Name:              name,
		Type:              "vless",
		Server:            p.Address,
		Port:              p.Port,
		UUID:              p.UUID,
		Network:           mihomoNetwork(p.Network),
		UDP:               true,
		Flow:              p.Flow,
		Servername:        p.SNI,
		ClientFingerprint: p.Fingerprint,
		RoutingMark:       routingMark,
	}

	// Mihomo has no separate "reality" security: reality is TLS plus reality-opts
	switch p.Security {
	case "tls":
		proxy.TLS = true
	case "reality":
		proxy.TLS = true
		proxy.RealityOpts = &realityOpts{PublicKey: p.PublicKey, ShortID: p.ShortID}
	}

	if p.ALPN != "" {
		proxy.ALPN = strings.Split(p.ALPN, ",")
	}

	switch proxy.Network {
	case "ws":
		opts := &wsOpts{Path: p.Path}
		if p.Host != "" {
			opts.Headers = map[string]string{"Host": p.Host}
		}
		proxy.WSOpts = opts
	case "grpc":
		proxy.GRPCOpts = &grpcOpts{ServiceName: p.Path}
	}

	var node yaml.Node
	if err := node.Encode(proxy); err != nil {
		return nil, err
	}

	return &node, nil
}

// mihomoNetwork maps Xray transport names onto Mihomo's. Xray renamed "tcp" to
// "raw", but Mihomo still expects "tcp" — and expresses it by omitting the key.
func mihomoNetwork(network string) string {
	switch network {
	case "", "tcp", "raw":
		return ""
	case "h2":
		return "h2"
	default:
		return network
	}
}

func atoiOrZero(text string) int {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0
	}
	return n
}

// mapValue returns the value node for key in a mapping node.
func mapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}

	return nil
}

// setMapValue replaces the value for key, or appends the pair when absent.
func setMapValue(node *yaml.Node, key string, value *yaml.Node) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			// Keep the key's own comments; only the value is replaced
			node.Content[i+1] = value
			return
		}
	}

	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}
