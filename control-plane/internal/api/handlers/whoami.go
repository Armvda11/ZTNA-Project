// Package handlers contient les handlers HTTP exposant les API du control-plane.
package handlers

import (
	"net/http"

	"control-plane/internal/api/middleware"
	domainErrors "control-plane/internal/domain/errors"
)

// WhoamiHandler fournit l'endpoint permettant au client d'obtenir
// l'identité du sujet actuellement authentifié (claims extraits du token).
type WhoamiHandler struct{}

// NewWhoamiHandler crée un nouveau WhoamiHandler.
func NewWhoamiHandler() *WhoamiHandler {
	return &WhoamiHandler{}
}

// Get renvoie les informations du sujet extrait du contexte (sub, username, groups).
func (h *WhoamiHandler) Get(w http.ResponseWriter, r *http.Request) {
	subject, ok := middleware.SubjectFromContext(r.Context())
	if !ok {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, subject)
}
