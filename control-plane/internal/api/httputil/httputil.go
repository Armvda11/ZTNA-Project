// Package httputil provides shared HTTP helpers used by both the handlers
// and middleware packages to avoid duplication.
package httputil

import (
	"encoding/json"
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
// returns a JSON error envelope.
func WriteError(w http.ResponseWriter, err error) {
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
	WriteJSON(w, status, ErrorResponse{Error: err.Error()})
}
