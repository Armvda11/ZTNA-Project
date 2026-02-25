package middleware

import (
	"net/http"

	"control-plane/internal/api/httputil"
)

// writeError is a thin wrapper around httputil.WriteError used within the
// middleware package to avoid importing the handlers package (cycle prevention).
func writeError(w http.ResponseWriter, err error) {
	httputil.WriteError(w, err)
}
