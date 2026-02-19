package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"control-plane/internal/config"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/security/oidc"
)

type ctxKey string

const subjectKey ctxKey = "subject"

type OIDCValidator struct {
	validator *oidc.Validator
}

func NewOIDCValidator(cfg config.OIDCConfig) (*OIDCValidator, error) {
	validator, err := oidc.NewValidator(cfg)
	if err != nil {
		return nil, err
	}
	return &OIDCValidator{validator: validator}, nil
}

func (v *OIDCValidator) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := bearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeError(w, domainErrors.ErrUnauthorized)
			return
		}

		subject, err := v.validator.Validate(r.Context(), token)
		if err != nil {
			writeError(w, domainErrors.ErrUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), subjectKey, subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func SubjectFromContext(ctx context.Context) (model.Subject, bool) {
	value := ctx.Value(subjectKey)
	subject, ok := value.(model.Subject)
	return subject, ok
}

func bearerToken(header string) (string, error) {
	if header == "" {
		return "", errors.New("missing authorization")
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid authorization")
	}
	return strings.TrimSpace(parts[1]), nil
}
