package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"control-plane/internal/domain/model"
	"control-plane/internal/service/session"
)

// PEPSessionHandler expose les endpoints POST /pep/sessions/start et /pep/sessions/end
// ainsi que GET /admin/sessions pour l'audit.
// Ces endpoints sont protégés par le même middleware PEPAuth que /pep/authorize.
type PEPSessionHandler struct {
	svc *session.Service
}

// NewPEPSessionHandler crée un handler de télémétrie de session.
func NewPEPSessionHandler(svc *session.Service) *PEPSessionHandler {
	return &PEPSessionHandler{svc: svc}
}

// Start traite POST /api/v1/pep/sessions/start.
// Body : SessionStartRequest (session_id, decision_id, subject_*, device_serial, resource_*).
func (h *PEPSessionHandler) Start(w http.ResponseWriter, r *http.Request) {
	var req session.StartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.SessionID == "" || req.DecisionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id and decision_id are required"})
		return
	}

	if err := h.svc.Start(r.Context(), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// End traite POST /api/v1/pep/sessions/end.
// Body : SessionEndRequest (session_id, bytes_in, bytes_out, duration_ms, end_reason).
func (h *PEPSessionHandler) End(w http.ResponseWriter, r *http.Request) {
	var req session.EndRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.SessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_id is required"})
		return
	}

	if err := h.svc.End(r.Context(), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ended"})
}

// List traite GET /api/v1/admin/sessions.
// Query param : limit (défaut 100, max 1000).
func (h *PEPSessionHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	sessions, err := h.svc.List(r.Context(), limit)
	if err != nil {
		writeError(w, err)
		return
	}
	if sessions == nil {
		sessions = []model.Session{}
	}
	writeJSON(w, http.StatusOK, sessions)
}
