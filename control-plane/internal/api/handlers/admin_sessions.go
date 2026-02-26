package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"control-plane/internal/api/middleware"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/service/session"
)

// AdminSessionsHandler expose les endpoints d'administration des sessions actives.
type AdminSessionsHandler struct {
	svc *session.Service
}

// NewAdminSessionsHandler crée un handler de gestion admin des sessions.
func NewAdminSessionsHandler(svc *session.Service) *AdminSessionsHandler {
	return &AdminSessionsHandler{svc: svc}
}

// Get traite GET /api/v1/admin/sessions/{id}
// Retourne le détail complet d'une session (inclus killed_at/killed_by si tuée).
func (h *AdminSessionsHandler) Get(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id manquant"})
		return
	}
	sess, err := h.svc.Get(r.Context(), sessionID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sess)
}

// Kill traite DELETE /api/v1/admin/sessions/{id}
// Marque immédiatement une session active comme tuée.
// La Gateway détectera ce changement via son poll /pep/sessions/{id}/valid
// dans les 5 secondes et coupera le proxy.
func (h *AdminSessionsHandler) Kill(w http.ResponseWriter, r *http.Request) {
	subject, ok := middleware.SubjectFromContext(r.Context())
	if !ok {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}

	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id manquant"})
		return
	}

	killed, err := h.svc.Kill(r.Context(), sessionID, subject.Sub)
	if err != nil {
		writeError(w, err)
		return
	}
	if !killed {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "session introuvable ou déjà terminée",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"session_id": sessionID,
		"killed_by":  subject.Sub,
		"status":     "killed",
		"message":    "La Gateway détectera la fermeture dans les 5 secondes.",
	})
}
