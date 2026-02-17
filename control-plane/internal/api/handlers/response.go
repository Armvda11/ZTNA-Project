// Package handlers contient les helpers et les handlers pour l'API.
package handlers

import (
	"encoding/json"
	"net/http"

	domainErrors "control-plane/internal/domain/errors"
)

// errorResponse définit la structure JSON envoyée en cas d'erreur.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON sérialise `payload` en JSON et écrit la réponse HTTP.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError mappe les erreurs d'application vers des codes HTTP
// et renvoie une réponse JSON formattée.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch err {
	case domainErrors.ErrUnauthorized:
		status = http.StatusUnauthorized
	case domainErrors.ErrForbidden:
		status = http.StatusForbidden
	case domainErrors.ErrInvalidInput:
		status = http.StatusBadRequest
	case domainErrors.ErrNotFound:
		status = http.StatusNotFound
	case domainErrors.ErrConflict:
		status = http.StatusConflict
	}

	writeJSON(w, status, errorResponse{Error: err.Error()})
}
