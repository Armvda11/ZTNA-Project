package handlers

import (
	"database/sql"
	"net/http"

	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/logger"
	"control-plane/internal/service/resource"

	"github.com/go-chi/chi/v5"
)

// PEPResourceHandler serves resource resolution for gateways (PEP).
type PEPResourceHandler struct {
	svc *resource.Service
}

func NewPEPResourceHandler(svc *resource.Service) *PEPResourceHandler {
	return &PEPResourceHandler{svc: svc}
}

type pepResourceResponse struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Backend    string `json:"backend"`
	GatewayID  string `json:"gateway_id"`
	AccessMode string `json:"access_mode"`
}

// Resolve returns the backend and metadata for a published resource.
// GET /api/v1/pep/resources/{name}
func (h *PEPResourceHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}

	res, err := h.svc.GetByName(r.Context(), name)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "resource not found"})
			return
		}
		writeError(w, err)
		return
	}

	// Verify that the requesting gateway is authorized to serve this resource.
	pepID := logger.PepIDFromContext(r.Context())
	if res.GatewayID != "" && pepID != "" && res.GatewayID != pepID {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "gateway not authorized for this resource",
		})
		return
	}

	writeJSON(w, http.StatusOK, pepResourceResponse{
		Name:       res.Name,
		Type:       res.Type,
		Backend:    res.Backend,
		GatewayID:  res.GatewayID,
		AccessMode: string(res.AccessMode),
	})
}
