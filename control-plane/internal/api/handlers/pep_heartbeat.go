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
	gatewaySvc          *gateway.Service
	requireRegistration bool
}

// NewPEPHeartbeatHandler creates the handler.
func NewPEPHeartbeatHandler(svc *gateway.Service, requireRegistration bool) *PEPHeartbeatHandler {
	return &PEPHeartbeatHandler{
		gatewaySvc:          svc,
		requireRegistration: requireRegistration,
	}
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

	status, err := h.gatewaySvc.Heartbeat(r.Context(), pepID, req.Version)
	if err != nil {
		if err == domainErrors.ErrForbidden && h.requireRegistration {
			writeJSON(w, http.StatusForbidden, map[string]string{"status": string(status)})
			return
		}
		if !h.requireRegistration && status == gateway.HeartbeatUnregistered {
			// Backward-compatible lab behavior when strict registration is disabled.
			writeJSON(w, http.StatusOK, map[string]string{"status": string(status)})
			return
		}
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": string(status)})
}
