package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"xkeen-panel/internal/infraupdate"
)

type InfraUpdateHandler struct {
	updater *infraupdate.Updater
}

func NewInfraUpdateHandler(u *infraupdate.Updater) *InfraUpdateHandler {
	return &InfraUpdateHandler{updater: u}
}

func (h *InfraUpdateHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.updater.State())
}

func (h *InfraUpdateHandler) HandleCheck(w http.ResponseWriter, r *http.Request) {
	state, err := h.updater.Check(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *InfraUpdateHandler) HandleApply(w http.ResponseWriter, r *http.Request) {
	if err := h.updater.Apply(chi.URLParam(r, "item")); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, h.updater.State())
}
