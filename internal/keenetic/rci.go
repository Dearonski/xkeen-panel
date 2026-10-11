// Package keenetic talks to the router's own configuration API (RCI), the one
// XKeen itself uses to find its policy and the hosts in it.
package keenetic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultAddr is where ndm serves RCI to local processes without a login.
const DefaultAddr = "http://127.0.0.1:79"

// PolicyDescription is how XKeen recognises its policy: by description, case
// aside, never by name — the web interface names policies Policy0, Policy1…
const PolicyDescription = "xkeen"

type Client struct {
	base  string
	token string
	http  *http.Client
}

// NewClient talks to RCI at base. token is XKeen's optional `rci_token`, sent
// the way XKeen sends it when the owner has locked the local API down.
func NewClient(base, token string) *Client {
	return &Client{
		base:  strings.TrimRight(base, "/"),
		token: token,
		http:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) get(path string, v interface{}) error {
	return c.do(http.MethodGet, path, nil, v)
}

// post sends a config command. ndm answers 200 even when it refuses one, so
// the refusal is read from the status messages in the body.
func (c *Client) post(path string, body interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	var resp interface{}
	if err := c.do(http.MethodPost, path, payload, &resp); err != nil {
		return err
	}
	if msg := statusError(resp); msg != "" {
		return fmt.Errorf("роутер отклонил команду: %s", msg)
	}
	return nil
}

// statusError finds an error among the status messages nested anywhere in an
// RCI answer.
func statusError(v interface{}) string {
	switch t := v.(type) {
	case map[string]interface{}:
		if t["status"] == "error" {
			msg, _ := t["message"].(string)
			if msg == "" {
				msg = "без описания"
			}
			return msg
		}
		for _, child := range t {
			if msg := statusError(child); msg != "" {
				return msg
			}
		}
	case []interface{}:
		for _, child := range t {
			if msg := statusError(child); msg != "" {
				return msg
			}
		}
	}
	return ""
}

func (c *Client) do(method, path string, payload []byte, v interface{}) error {
	req, err := http.NewRequest(method, c.base+"/rci/"+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("X-Ndma-Tkn", c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("API роутера недоступен: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("ошибка чтения ответа RCI %s: %w", path, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("RCI %s ответил %d", path, resp.StatusCode)
	}

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("ошибка разбора ответа RCI %s: %w", path, err)
	}
	return nil
}

// Policy is one Keenetic connection policy.
type Policy struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Mark        string `json:"mark"`
}

// Policies lists the router's connection policies.
func (c *Client) Policies() ([]Policy, error) {
	var raw map[string]Policy
	if err := c.get("show/ip/policy", &raw); err != nil {
		return nil, err
	}

	policies := make([]Policy, 0, len(raw))
	for name, p := range raw {
		p.Name = name
		policies = append(policies, p)
	}
	return policies, nil
}

// XKeenPolicy finds the policy XKeen proxies, matched the way XKeen does.
func XKeenPolicy(policies []Policy) (Policy, bool) {
	for _, p := range policies {
		if strings.EqualFold(strings.TrimSpace(p.Description), PolicyDescription) {
			return p, true
		}
	}
	return Policy{}, false
}

// Host is a device the router knows.
type Host struct {
	MAC        string `json:"mac"`
	Name       string `json:"name"`
	Hostname   string `json:"hostname"`
	IP         string `json:"ip"`
	Active     bool   `json:"active"`
	Registered bool   `json:"registered"`
	Interface  struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"interface"`

	// The policy ndm applies right now. It goes blank for some offline hosts
	// that follow their segment, so it cannot tell the owner's intent.
	Policy string `json:"policy"`
}

// Hosts lists the devices the router knows, online or not.
func (c *Client) Hosts() ([]Host, error) {
	var raw struct {
		Host []Host `json:"host"`
	}
	if err := c.get("show/ip/hotspot", &raw); err != nil {
		return nil, err
	}
	return raw.Host, nil
}

// WANInterface carries the default route; a policy without it leaves its devices offline
func (c *Client) WANInterface() (string, error) {
	var ifaces map[string]struct {
		DefaultGW bool `json:"defaultgw"`
		Global    bool `json:"global"`
	}
	if err := c.get("show/interface", &ifaces); err != nil {
		return "", err
	}
	for id, iface := range ifaces {
		if iface.DefaultGW && iface.Global {
			return id, nil
		}
	}
	return "", fmt.Errorf("не найден интерфейс с маршрутом по умолчанию")
}

// CreateXKeenPolicy adds the policy XKeen proxies, under the first free PolicyN name
func (c *Client) CreateXKeenPolicy() (Policy, error) {
	policies, err := c.Policies()
	if err != nil {
		return Policy{}, err
	}
	if p, ok := XKeenPolicy(policies); ok {
		return p, nil
	}

	wan, err := c.WANInterface()
	if err != nil {
		return Policy{}, err
	}

	taken := make(map[string]bool, len(policies))
	for _, p := range policies {
		taken[p.Name] = true
	}
	name := ""
	for i := 0; name == ""; i++ {
		if candidate := fmt.Sprintf("Policy%d", i); !taken[candidate] {
			name = candidate
		}
	}

	if err := c.post("ip/policy", map[string]interface{}{
		"name":        name,
		"description": PolicyDescription,
		"permit":      map[string]interface{}{"global": true, "interface": wan},
	}); err != nil {
		return Policy{}, err
	}
	if err := c.post("system/configuration/save", map[string]interface{}{}); err != nil {
		return Policy{}, err
	}

	return Policy{Name: name, Description: PolicyDescription}, nil
}
