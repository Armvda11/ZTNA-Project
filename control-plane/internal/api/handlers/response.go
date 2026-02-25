// Package handlers contient les helpers et les handlers pour l'API.
package handlers

import (
	"net/http"

	"control-plane/internal/api/httputil"
)

// writeJSON serialises payload as JSON and writes the HTTP response.
// Delegates to the shared httputil package.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	httputil.WriteJSON(w, status, payload)
}

// writeError maps a domain error to the appropriate HTTP status code.
// Delegates to the shared httputil package.
func writeError(w http.ResponseWriter, err error) {
	httputil.WriteError(w, err)
}
