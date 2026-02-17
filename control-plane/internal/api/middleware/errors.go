package middleware

import (
	"encoding/json"
	"net/http"

	domainErrors "control-plane/internal/domain/errors"
)

type errorResponse struct {
	Error string `json:"error"`
}

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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: err.Error()})
}
