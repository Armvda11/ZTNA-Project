// Package httputil provides shared HTTP helpers used by both the handlers
// and middleware packages to avoid duplication.
package httputil

import (
	"encoding/json"
	"errors"
	"net/http"

	domainErrors "control-plane/internal/domain/errors"
)

// ErrorResponse is the canonical JSON error envelope.
type ErrorResponse struct {
	Error string `json:"error"`
}

// WriteJSON serialises payload as JSON and writes the HTTP response.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError maps a domain error to the appropriate HTTP status code and
// returns a JSON error envelope. Uses errors.Is for proper wrapped error
// matching. Internal errors are sanitised to avoid leaking implementation details.
func WriteError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := "erreur interne du serveur" // safe default

	switch {
	case errors.Is(err, domainErrors.ErrUnauthorized):
		status = http.StatusUnauthorized
		msg = err.Error()
	case errors.Is(err, domainErrors.ErrForbidden):
		status = http.StatusForbidden
		msg = err.Error()
	case errors.Is(err, domainErrors.ErrInvalidInput):
		status = http.StatusBadRequest
		msg = err.Error()
	case errors.Is(err, domainErrors.ErrNotFound):
		status = http.StatusNotFound
		msg = err.Error()
	case errors.Is(err, domainErrors.ErrConflict):
		status = http.StatusConflict
		msg = err.Error()
	}

	WriteJSON(w, status, ErrorResponse{Error: msg})
}
