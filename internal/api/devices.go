package api

import (
	"encoding/json"
	"net/http"
	"regexp"

	"xkeen-panel/internal/keenetic"
	"xkeen-panel/internal/xkeen"
)

type DevicesHandler struct {
	detector *xkeen.Detector
	rciAddr  string
}

func NewDevicesHandler(det *xkeen.Detector, rciAddr string) *DevicesHandler {
	return &DevicesHandler{detector: det, rciAddr: rciAddr}
}

// client picks up XKeen's `rci_token` on every call, so locking the router's
// API down in xkeen.json needs no panel restart.
func (h *DevicesHandler) client() *keenetic.Client {
	token := ""
	if settings, err := xkeen.ReadSettings(h.detector.Runtime().XkeenJSON); err == nil {
		token, _ = settings["rci_token"].(string)
	}
	return keenetic.NewClient(h.rciAddr, token)
}

// HandleList — GET /api/devices
func (h *DevicesHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	c := h.client()

	policies, err := c.Policies()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	hosts, err := c.Hosts()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	cfg, err := c.HotspotConfig()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	policy, found := keenetic.XKeenPolicy(policies)
	devices := keenetic.Devices(hosts, cfg, policy.Name)

	// Without its policy XKeen proxies every client of the router
	if !found {
		for i := range devices {
			devices[i].VPN = true
		}
	}

	resp := map[string]interface{}{
		"policy_found":    found,
		"new_devices_vpn": !found || cfg.NewDevicesVPN(policy.Name),
		"devices":         devices,
	}
	if found {
		resp["policy"] = policy
	}

	writeJSON(w, http.StatusOK, resp)
}

var macRe = regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)

// HandleSetVPN — POST /api/devices/vpn
func (h *DevicesHandler) HandleSetVPN(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MAC string `json:"mac"`
		VPN bool   `json:"vpn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !macRe.MatchString(req.MAC) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	c := h.client()
	policies, err := c.Policies()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	policy, found := keenetic.XKeenPolicy(policies)
	if !found {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "на роутере нет политики с описанием xkeen — без неё XKeen проксирует все устройства",
		})
		return
	}

	if err := c.SetHostVPN(req.MAC, req.VPN, policy.Name); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true})
}
