package handlers

import (
	"encoding/json"
	"net/http"

	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/logger"
	"control-plane/internal/service/gateway"
)

// PEPHeartbeatHandler records liveness signals from registered gateways.
type PEPHeartbeatHandler struct {
	gatewaySvc *gateway.Service
}

// NewPEPHeartbeatHandler creates the handler.
func NewPEPHeartbeatHandler(svc *gateway.Service) *PEPHeartbeatHandler {
	return &PEPHeartbeatHandler{gatewaySvc: svc}
}

type heartbeatRequest struct {
	// Version is the gateway software version (informational, optional).
	Version string `json:"version,omitempty"`
}

// Beat handles POST /api/v1/pep/heartbeat
// The gateway calls this endpoint periodically to signal it is alive.
// Authentication is handled upstream by the PEPAuth middleware.
func (h *PEPHeartbeatHandler) Beat(w http.ResponseWriter, r *http.Request) {
	pepID := logger.PepIDFromContext(r.Context())
	if pepID == "" {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}

	var req heartbeatRequest
	// Ignore decode errors — the body is optional.
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.gatewaySvc.Heartbeat(r.Context(), pepID); err != nil {
		// Not fatal: gateway might not be registered yet. Log and continue.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "note": "gateway not registered"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
