package keenetic

import "sort"

// HotspotConfig is the saved host configuration, as opposed to the live state
// in Hosts: it says what the owner set, including for devices that are offline.
type HotspotConfig struct {
	// Default policy per segment, keyed by interface id (Bridge0, …)
	Policy []struct {
		Interface string `json:"interface"`
		Policy    string `json:"policy"`
	} `json:"policy"`

	Host []struct {
		MAC     string      `json:"mac"`
		Access  string      `json:"access"`
		Policy  interface{} `json:"policy"`  // a name, or false once cleared
		Conform bool        `json:"conform"` // follow the segment's policy
	} `json:"host"`
}

func (c *Client) HotspotConfig() (HotspotConfig, error) {
	var cfg HotspotConfig
	err := c.get("show/rc/ip/hotspot", &cfg)
	return cfg, err
}

func (cfg HotspotConfig) segmentPolicy(iface string) string {
	// An offline host may come without a segment; a single segment rule is then unambiguous
	if iface == "" && len(cfg.Policy) == 1 {
		return cfg.Policy[0].Policy
	}
	for _, p := range cfg.Policy {
		if p.Interface == iface {
			return p.Policy
		}
	}
	return ""
}

// How a device ends up in a policy.
const (
	ViaHost    = "host"    // set on the device itself
	ViaSegment = "segment" // inherited from its segment's default
)

type Device struct {
	MAC        string `json:"mac"`
	Name       string `json:"name"`
	IP         string `json:"ip,omitempty"`
	Active     bool   `json:"active"`
	Registered bool   `json:"registered"`
	Segment    string `json:"segment,omitempty"`
	Policy     string `json:"policy,omitempty"` // the policy the device is configured into
	Via        string `json:"via,omitempty"`
	VPN        bool   `json:"vpn"` // its traffic goes through XKeen's policy
}

// Devices works out which policy each device is meant to be in.
//
// The live `policy` field of a host cannot answer this: ndm leaves it blank
// for some offline devices that follow their segment. The saved config can:
// an explicit policy wins, `conform` and unregistered devices take the
// segment's default, and a registered device with neither uses the main
// connection.
func Devices(hosts []Host, cfg HotspotConfig, xkeenPolicy string) []Device {
	configured := make(map[string]int, len(cfg.Host))
	for i, h := range cfg.Host {
		configured[h.MAC] = i
	}

	devices := make([]Device, 0, len(hosts))
	for _, h := range hosts {
		d := Device{
			MAC:        h.MAC,
			Name:       displayName(h),
			IP:         h.IP,
			Active:     h.Active,
			Registered: h.Registered,
			Segment:    h.Interface.Description,
		}

		i, known := configured[h.MAC]
		switch {
		case known && policyName(cfg.Host[i].Policy) != "":
			d.Policy, d.Via = policyName(cfg.Host[i].Policy), ViaHost
		case !h.Registered || (known && cfg.Host[i].Conform):
			if p := cfg.segmentPolicy(h.Interface.ID); p != "" {
				d.Policy, d.Via = p, ViaSegment
			}
		}

		d.VPN = xkeenPolicy != "" && d.Policy == xkeenPolicy
		devices = append(devices, d)
	}

	sort.SliceStable(devices, func(i, j int) bool {
		if devices[i].Active != devices[j].Active {
			return devices[i].Active
		}
		return devices[i].Name < devices[j].Name
	})

	return devices
}

// NewDevicesVPN reports whether devices joining the segment land in the policy.
func (cfg HotspotConfig) NewDevicesVPN(xkeenPolicy string) bool {
	for _, p := range cfg.Policy {
		if p.Policy == xkeenPolicy && xkeenPolicy != "" {
			return true
		}
	}
	return false
}

func policyName(v interface{}) string {
	name, _ := v.(string)
	return name
}

func displayName(h Host) string {
	switch {
	case h.Name != "":
		return h.Name
	case h.Hostname != "":
		return h.Hostname
	default:
		return h.MAC
	}
}

// SetHostVPN puts a device into the policy or takes it out, then saves the
// router config so the choice survives a reboot.
//
// Taking it out has to drop `conform` as well: a device that follows its
// segment would otherwise be pulled straight back in by the segment default.
func (c *Client) SetHostVPN(mac string, on bool, policy string) error {
	if on {
		if err := c.post("ip/hotspot/host", map[string]interface{}{"mac": mac, "policy": policy}); err != nil {
			return err
		}
	} else {
		for _, cmd := range []map[string]interface{}{
			{"mac": mac, "policy": false},
			{"mac": mac, "conform": false},
		} {
			if err := c.post("ip/hotspot/host", cmd); err != nil {
				return err
			}
		}
	}

	return c.post("system/configuration/save", map[string]interface{}{})
}
