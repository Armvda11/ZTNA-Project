// Package handlers contient les handlers HTTP qui exposent
// les API REST du control-plane (admin, PEP, credentials, etc.).
package handlers

import (
	"net/http"
	"strconv"

	"control-plane/internal/service/audit"
)

// AdminAuditHandler sert les endpoints d'audit destinés aux administrateurs.
type AdminAuditHandler struct {
	audit *audit.Service
}

// NewAdminAuditHandler crée un handler pour les opérations d'audit.
func NewAdminAuditHandler(auditSvc *audit.Service) *AdminAuditHandler {
	return &AdminAuditHandler{audit: auditSvc}
}

// List retourne les entrées d'audit les plus récentes.
// Query params:
//   - limit  : nombre d'entrées (défaut 100, max 500)
//   - offset : décalage pour la pagination (défaut 0)
func (h *AdminAuditHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	// Server-side cap to prevent runaway queries.
	if limit > 500 {
		limit = 500
	}

	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	events, err := h.audit.List(r.Context(), limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, events)
}
