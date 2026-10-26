package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"xkeen-panel/internal/xkeen"

	"github.com/go-chi/chi/v5"
)

type SitesHandler struct {
	sites *xkeen.SiteManager
}

func NewSitesHandler(sites *xkeen.SiteManager) *SitesHandler {
	return &SitesHandler{sites: sites}
}

// HandleList — GET /api/sites
func (h *SitesHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"unavailable": h.sites.Unavailable(),
		"sites":       h.sites.Views(),
	})
}

// HandleCreate — POST /api/sites
func (h *SitesHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domains  string `json:"domains"`
		CheckURL string `json:"check_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный формат запроса"})
		return
	}

	site, err := h.sites.Create([]string{req.Domains}, req.CheckURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"success": true, "id": site.ID})
}

// HandleCheck — POST /api/sites/{id}/check
func (h *SitesHandler) HandleCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := siteID(w, r)
	if !ok {
		return
	}
	if reason := h.sites.Unavailable(); reason != "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": reason})
		return
	}

	h.sites.CheckAsync(id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// HandleDelete — DELETE /api/sites/{id}
func (h *SitesHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := siteID(w, r)
	if !ok {
		return
	}

	if err := h.sites.Delete(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func siteID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "неверный id сайта"})
		return 0, false
	}
	return id, true
}
