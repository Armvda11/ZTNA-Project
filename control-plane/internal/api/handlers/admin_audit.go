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

// List retourne les entrées d'audit les plus récentes. Le paramètre GET
// `limit` permet de limiter le nombre d'entrées retournées (par défaut 100).
func (h *AdminAuditHandler) List(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	events, err := h.audit.List(r.Context(), limit)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, events)
}
