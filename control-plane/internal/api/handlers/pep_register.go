package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/logger"
	"control-plane/internal/service/gateway"
)

// PEPRegisterHandler handles gateway registration lifecycle.
type PEPRegisterHandler struct {
	gatewaySvc *gateway.Service
}

// NewPEPRegisterHandler creates a registration handler.
func NewPEPRegisterHandler(svc *gateway.Service) *PEPRegisterHandler {
	return &PEPRegisterHandler{gatewaySvc: svc}
}

type registerRequest struct {
	GatewayID   string `json:"gateway_id,omitempty"`
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Register handles POST /api/v1/pep/register.
//
// The gateway identity comes from PEPAuth (mTLS CN or X-PEP-ID+token).
// Request payload can provide metadata (name/version/fingerprint) but cannot
// override the authenticated gateway ID.
func (h *PEPRegisterHandler) Register(w http.ResponseWriter, r *http.Request) {
	pepID := logger.PepIDFromContext(r.Context())
	if pepID == "" {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}

	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if req.GatewayID != "" && req.GatewayID != pepID {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}

	gw := model.Gateway{
		ID:              pepID,
		Name:            req.Name,
		Fingerprint:     req.Fingerprint,
		SoftwareVersion: req.Version,
		Active:          true,
	}
	if gw.Name == "" {
		gw.Name = pepID
	}
	if err := h.gatewaySvc.Register(r.Context(), gw); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "registered",
		"gateway_id": pepID,
	})
}
