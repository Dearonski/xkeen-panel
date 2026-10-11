package keenetic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Responses captured on the home router (KeeneticOS 5.x), trimmed.
const (
	policyJSON = `{"Policy0":{"description":"xkeen","mark":"ffffaaa","table4":4096},
		"Policy1":{"description":"guests","mark":"ffffaab"}}`
	hotspotJSON = `{"host":[
		{"mac":"fe:4b:64:98:9d:03","name":"Watch","hostname":"Watch","ip":"192.168.2.148","active":true,"registered":true,"policy":"",
		 "interface":{"id":"Bridge0","name":"Home","description":"Основная"}},
		{"mac":"3c:0b:4f:15:86:3a","name":"Теплые полы","active":true,"registered":true,"policy":"Policy0"}]}`
)

func rciServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("X-Ndma-Tkn") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/rci/show/ip/policy":
			w.Write([]byte(policyJSON))
		case "/rci/show/ip/hotspot":
			w.Write([]byte(hotspotJSON))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestXKeenPolicyByDescription(t *testing.T) {
	c := NewClient(rciServer(t, "").URL, "")

	policies, err := c.Policies()
	if err != nil {
		t.Fatalf("Policies: %v", err)
	}
	p, ok := XKeenPolicy(policies)
	if !ok || p.Name != "Policy0" || p.Mark != "ffffaaa" {
		t.Errorf("XKeenPolicy = %+v, %v; want Policy0", p, ok)
	}

	if _, ok := XKeenPolicy([]Policy{{Name: "xkeen", Description: "other"}}); ok {
		t.Error("a policy merely named xkeen is not the one XKeen uses — it matches descriptions")
	}
}

func TestHosts(t *testing.T) {
	c := NewClient(rciServer(t, "").URL, "")

	hosts, err := c.Hosts()
	if err != nil {
		t.Fatalf("Hosts: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("hosts = %d, want 2", len(hosts))
	}
	if h := hosts[0]; h.MAC != "fe:4b:64:98:9d:03" || !h.Active || h.Interface.Name != "Home" {
		t.Errorf("first host = %+v", h)
	}
}

func TestTokenIsSent(t *testing.T) {
	srv := rciServer(t, "secret")

	if _, err := NewClient(srv.URL, "").Policies(); err == nil {
		t.Error("a locked API must reject a request without the token")
	}
	if _, err := NewClient(srv.URL, "secret").Policies(); err != nil {
		t.Errorf("with the token: %v", err)
	}
}

// The saved config from the home router: one explicit host, one following the
// segment, one on the main connection, and the segment defaulting to Policy0.
const hotspotConfigJSON = `{
	"policy":[{"interface":"Bridge0","policy":"Policy0"}],
	"host":[
		{"mac":"14:14:7d:98:24:b6","access":"permit","permit":true,"policy":"Policy0"},
		{"mac":"fe:4b:64:98:9d:03","access":"permit","permit":true},
		{"mac":"3c:0b:4f:15:86:3a","access":"permit","permit":true,"conform":true},
		{"mac":"50:b0:3b:19:32:9e","access":"permit","permit":true,"conform":true}]}`

func TestDevicesFollowSavedConfig(t *testing.T) {
	hosts := []Host{
		{MAC: "14:14:7d:98:24:b6", Name: "Macbook Pro", Active: true, Registered: true, Policy: "Policy0"},
		{MAC: "fe:4b:64:98:9d:03", Name: "Watch", Active: true, Registered: true},
		{MAC: "3c:0b:4f:15:86:3a", Name: "Теплые полы", Active: true, Registered: true, Policy: "Policy0"},
		// Offline and following its segment: the live policy reads blank
		{MAC: "50:b0:3b:19:32:9e", Name: "Laptop", Registered: true},
		{MAC: "aa:bb:cc:dd:ee:ff", Hostname: "guest-phone", Active: true},
	}
	for i := range hosts {
		hosts[i].Interface.ID = "Bridge0"
	}
	hosts[3].Interface.ID = ""

	var cfg HotspotConfig
	if err := json.Unmarshal([]byte(hotspotConfigJSON), &cfg); err != nil {
		t.Fatal(err)
	}

	want := map[string]struct {
		vpn bool
		via string
	}{
		"Macbook Pro": {true, ViaHost},
		"Watch":       {false, ""},
		"Теплые полы": {true, ViaSegment},
		"Laptop":      {true, ViaSegment},
		"guest-phone": {true, ViaSegment},
	}

	devices := Devices(hosts, cfg, "Policy0")
	if len(devices) != len(want) {
		t.Fatalf("devices = %d, want %d", len(devices), len(want))
	}
	for _, d := range devices {
		w := want[d.Name]
		if d.VPN != w.vpn || d.Via != w.via {
			t.Errorf("%s: vpn=%v via=%q, want vpn=%v via=%q", d.Name, d.VPN, d.Via, w.vpn, w.via)
		}
	}
	if devices[len(devices)-1].Name != "Laptop" {
		t.Errorf("offline devices go last, got %s", devices[len(devices)-1].Name)
	}
	if !cfg.NewDevicesVPN("Policy0") {
		t.Error("the segment defaults to Policy0 — new devices land in the VPN")
	}
}

type recorded struct {
	path string
	body map[string]interface{}
}

func recordingServer(t *testing.T, answer string) (*httptest.Server, *[]recorded) {
	t.Helper()
	var calls []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		calls = append(calls, recorded{r.URL.Path, body})
		w.Write([]byte(answer))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// Answers as ndm gave them on the home router.
const okAnswer = `{"policy":{"status":[{"status":"message","code":"1","ident":"Hotspot::Manager","message":"policy removed from host"}]}}`

func TestSetHostVPNOffDropsConform(t *testing.T) {
	srv, calls := recordingServer(t, okAnswer)

	if err := NewClient(srv.URL, "").SetHostVPN("50:b0:3b:19:32:9e", false, "Policy0"); err != nil {
		t.Fatalf("SetHostVPN: %v", err)
	}

	got := *calls
	if len(got) != 3 {
		t.Fatalf("calls = %+v, want policy, conform, save", got)
	}
	if got[0].body["policy"] != false || got[1].body["conform"] != false {
		t.Errorf("off must clear both policy and conform, got %+v", got[:2])
	}
	if got[2].path != "/rci/system/configuration/save" {
		t.Errorf("last call = %s, want the config saved", got[2].path)
	}
}

func TestSetHostVPNOn(t *testing.T) {
	srv, calls := recordingServer(t, okAnswer)

	if err := NewClient(srv.URL, "").SetHostVPN("50:b0:3b:19:32:9e", true, "Policy0"); err != nil {
		t.Fatalf("SetHostVPN: %v", err)
	}
	if got := (*calls)[0]; got.path != "/rci/ip/hotspot/host" || got.body["policy"] != "Policy0" {
		t.Errorf("first call = %+v, want the policy set on the host", got)
	}
}

// ndm answers 200 even when it refuses a command
func TestPostReadsRefusalFromBody(t *testing.T) {
	srv, _ := recordingServer(t, `{"policy":{"status":[{"status":"error","message":"unable to find policy \"Nope\""}]}}`)

	err := NewClient(srv.URL, "").SetHostVPN("50:b0:3b:19:32:9e", true, "Nope")
	if err == nil || !strings.Contains(err.Error(), "unable to find policy") {
		t.Errorf("err = %v, want the router's refusal", err)
	}
}

func TestSetNewDevicesVPN(t *testing.T) {
	var cfg HotspotConfig
	if err := json.Unmarshal([]byte(hotspotConfigJSON), &cfg); err != nil {
		t.Fatal(err)
	}

	srv, calls := recordingServer(t, okAnswer)
	if err := NewClient(srv.URL, "").SetNewDevicesVPN(cfg, false, "Policy0"); err != nil {
		t.Fatalf("off: %v", err)
	}
	got := *calls
	if len(got) != 2 || got[0].path != "/rci/ip/hotspot/policy" || got[0].body["interface"] != "Bridge0" || got[0].body["no"] != true {
		t.Errorf("off calls = %+v, want Bridge0 cleared with no:true, then save", got)
	}

	srv, calls = recordingServer(t, okAnswer)
	if err := NewClient(srv.URL, "").SetNewDevicesVPN(cfg, true, "Policy0"); err != nil {
		t.Fatalf("on: %v", err)
	}
	if got := (*calls)[0]; got.body["policy"] != "Policy0" || got.body["interface"] != HomeSegment {
		t.Errorf("on call = %+v, want the home segment set to Policy0", got)
	}
}

func TestCreateXKeenPolicyPicksFreeNameAndWAN(t *testing.T) {
	var posted []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rci/show/ip/policy":
			w.Write([]byte(`{"Policy0":{"description":"guests"},"Policy1":{"description":"kids"}}`))
		case "/rci/show/interface":
			w.Write([]byte(`{"Bridge0":{"global":false},"PPPoE0":{"defaultgw":true,"global":true},"GigabitEthernet1":{"global":true}}`))
		default:
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			posted = append(posted, recorded{r.URL.Path, body})
			w.Write([]byte(okAnswer))
		}
	}))
	t.Cleanup(srv.Close)

	p, err := NewClient(srv.URL, "").CreateXKeenPolicy()
	if err != nil {
		t.Fatalf("CreateXKeenPolicy: %v", err)
	}
	if p.Name != "Policy2" {
		t.Errorf("name = %s, want the first free one, Policy2", p.Name)
	}
	if len(posted) != 2 || posted[0].path != "/rci/ip/policy" || posted[1].path != "/rci/system/configuration/save" {
		t.Fatalf("calls = %+v, want create then save", posted)
	}
	permit, _ := posted[0].body["permit"].(map[string]interface{})
	if posted[0].body["description"] != "xkeen" || permit["interface"] != "PPPoE0" {
		t.Errorf("create body = %v, want description xkeen and the default-route WAN", posted[0].body)
	}
}

func TestCreateXKeenPolicyKeepsExisting(t *testing.T) {
	c := NewClient(rciServer(t, "").URL, "")
	p, err := c.CreateXKeenPolicy()
	if err != nil || p.Name != "Policy0" {
		t.Errorf("got %+v, %v — an existing xkeen policy must be reused, not duplicated", p, err)
	}
}
