// Package middleware fournit des middlewares HTTP réutilisables
// pour l'API du control-plane. Les middlewares gèrent l'authentification
// et l'autorisation au niveau des handlers HTTP.
package middleware

import (
	"net/http"
	"strings"

	"control-plane/internal/config"
	domainErrors "control-plane/internal/domain/errors"
)

// AdminAuth encapsule la configuration nécessaire pour vérifier
// si un sujet appartient au groupe d'administration configuré.
type AdminAuth struct {
	cfg config.OIDCConfig
}

// NewAdminAuth crée une instance de AdminAuth à partir de la configuration OIDC.
func NewAdminAuth(cfg config.OIDCConfig) *AdminAuth {
	return &AdminAuth{cfg: cfg}
}

// RequireAdmin retourne un middleware qui autorise l'accès seulement aux
// sujets appartenant au groupe administrateur configuré dans `a.cfg.AdminGroup`.
// Il normalise également les groupes renvoyés par Keycloak en supprimant
// un préfixe "/" éventuel pour gérer les mappers qui retournent "/group".
func (a *AdminAuth) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject, ok := SubjectFromContext(r.Context())
		if !ok {
			writeError(w, domainErrors.ErrUnauthorized)
			return
		}
		if a.cfg.AdminGroup == "" {
			writeError(w, domainErrors.ErrForbidden)
			return
		}
		// Normalize groups (trim leading / for Keycloak mappers that return "/ztna-admins")
		for _, group := range subject.Groups {
			normalized := strings.TrimPrefix(group, "/")
			if normalized == a.cfg.AdminGroup || group == a.cfg.AdminGroup {
				next.ServeHTTP(w, r)
				return
			}
		}
		writeError(w, domainErrors.ErrForbidden)
	})
}
