package middleware

import (
	"net/http"

	"control-plane/internal/config"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/logger"
)

type PEPAuth struct {
	cfg config.PEPConfig
}

func NewPEPAuth(cfg config.PEPConfig) *PEPAuth {
	return &PEPAuth{cfg: cfg}
}

func (p *PEPAuth) RequirePEP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.cfg.AuthMode == "mtls" {
			if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
				writeError(w, domainErrors.ErrUnauthorized)
				return
			}
			pepID := r.TLS.PeerCertificates[0].Subject.CommonName
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
		if !ok || expected == "" || expected != token {
			writeError(w, domainErrors.ErrUnauthorized)
			return
		}

		ctx := logger.WithPepID(r.Context(), pepID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
