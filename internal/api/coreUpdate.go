package api

import (
	"net/http"

	"xkeen-panel/internal/coreupdate"
)

type CoreUpdateHandler struct {
	updater *coreupdate.Updater
}

func NewCoreUpdateHandler(u *coreupdate.Updater) *CoreUpdateHandler {
	return &CoreUpdateHandler{updater: u}
}

func (h *CoreUpdateHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.updater.State())
}

func (h *CoreUpdateHandler) HandleCheck(w http.ResponseWriter, r *http.Request) {
	state, err := h.updater.Check(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *CoreUpdateHandler) HandleApply(w http.ResponseWriter, r *http.Request) {
	if err := h.updater.Apply(); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, h.updater.State())
}

func (h *CoreUpdateHandler) HandleRollback(w http.ResponseWriter, r *http.Request) {
	if err := h.updater.Rollback(); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, h.updater.State())
}
