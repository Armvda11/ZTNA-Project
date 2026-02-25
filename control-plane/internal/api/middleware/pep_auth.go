package middleware

import (
	"crypto/subtle"
	"net/http"

	"control-plane/internal/config"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/logger"
)

type PEPAuth struct {
	cfg        config.PEPConfig
	revokedSet map[string]struct{}
}

func NewPEPAuth(cfg config.PEPConfig) *PEPAuth {
	revoked := make(map[string]struct{}, len(cfg.RevokedPEPIDs))
	for _, id := range cfg.RevokedPEPIDs {
		revoked[id] = struct{}{}
	}
	return &PEPAuth{cfg: cfg, revokedSet: revoked}
}

func (p *PEPAuth) RequirePEP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.cfg.AuthMode == "mtls" {
			if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
				writeError(w, domainErrors.ErrUnauthorized)
				return
			}
			pepID := r.TLS.PeerCertificates[0].Subject.CommonName
			if p.isRevoked(pepID) {
				writeError(w, domainErrors.ErrForbidden)
				return
			}
			ctx := logger.WithPepID(r.Context(), pepID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		pepID := r.Header.Get("X-PEP-ID")
		if pepID == "" {
			writeError(w, domainErrors.ErrUnauthorized)
			return
		}
		token := r.Header.Get("X-PEP-TOKEN")
		if token == "" {
			writeError(w, domainErrors.ErrUnauthorized)
			return
		}

		expected, ok := p.cfg.Tokens[pepID]
		// Use constant-time comparison to prevent timing-based token enumeration.
		if !ok || expected == "" || subtle.ConstantTimeCompare([]byte(expected), []byte(token)) != 1 {
			writeError(w, domainErrors.ErrUnauthorized)
			return
		}
		if p.isRevoked(pepID) {
			writeError(w, domainErrors.ErrForbidden)
			return
		}

		ctx := logger.WithPepID(r.Context(), pepID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (p *PEPAuth) isRevoked(pepID string) bool {
	_, ok := p.revokedSet[pepID]
	return ok
}
