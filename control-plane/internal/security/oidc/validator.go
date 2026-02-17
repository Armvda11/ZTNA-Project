package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"control-plane/internal/config"
	"control-plane/internal/domain/model"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Validator validates OAuth2/OIDC access tokens (JWT format) via JWKS.
// This is intended for resource server validation (not ID token introspection).
// Access tokens must be JWT format with RS256 signature.
type Validator struct {
	issuer        string
	audience      string
	usernameClaim string
	groupsClaim   string
	audienceMode  string
	allowedAlgs   map[string]struct{}
	allowedList   []jose.SignatureAlgorithm
	jwksURL       string
	cacheTTL      time.Duration
	client        *http.Client

	mu    sync.RWMutex
	cache jwksCache
}

type jwksCache struct {
	keys      map[string]jose.JSONWebKey
	expiresAt time.Time
}

func NewValidator(cfg config.OIDCConfig) (*Validator, error) {
	ttl, err := time.ParseDuration(cfg.JWKSCacheTTL)
	if err != nil {
		return nil, fmt.Errorf("parse jwks cache ttl: %w", err)
	}

	allowed := make(map[string]struct{}, len(cfg.AllowedAlgs))
	allowedList := make([]jose.SignatureAlgorithm, 0, len(cfg.AllowedAlgs))
	for _, alg := range cfg.AllowedAlgs {
		trimmed := strings.ToUpper(strings.TrimSpace(alg))
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
		allowedList = append(allowedList, jose.SignatureAlgorithm(trimmed))
	}

	// Normalize issuer (trim trailing slash for consistent validation)
	issuer := strings.TrimRight(cfg.Issuer, "/")
	jwksURL := issuer + "/protocol/openid-connect/certs"

	// Allow HTTP issuer for lab/dev (Keycloak without TLS)
	// Note: allow_http_issuer does not disable TLS verification for https:// issuers
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	return &Validator{
		issuer:        issuer,
		audience:      cfg.Audience,
		usernameClaim: cfg.UsernameClaim,
		groupsClaim:   cfg.GroupsClaim,
		audienceMode:  cfg.AudienceMode,
		allowedAlgs:   allowed,
		allowedList:   allowedList,
		jwksURL:       jwksURL,
		cacheTTL:      ttl,
		client:        httpClient,
		cache: jwksCache{
			keys:      map[string]jose.JSONWebKey{},
			expiresAt: time.Time{},
		},
	}, nil
}

// Validate validates an OAuth2/OIDC access token (JWT format).
// This performs offline validation via JWKS (RS256 signature).
// Expected token format: JWT with iss, aud (or azp), sub, exp claims.
func (v *Validator) Validate(ctx context.Context, raw string) (model.Subject, error) {
	token, err := jwt.ParseSigned(raw, v.allowedList)
	if err != nil {
		return model.Subject{}, err
	}
	if len(token.Headers) == 0 {
		return model.Subject{}, errors.New("missing jwt header")
	}
	header := token.Headers[0]
	if header.Algorithm == "" {
		return model.Subject{}, errors.New("missing jwt alg")
	}
	if _, ok := v.allowedAlgs[strings.ToUpper(header.Algorithm)]; !ok {
		return model.Subject{}, errors.New("unexpected jwt alg")
	}

	key, err := v.keyForKID(ctx, header.KeyID)
	if err != nil {
		return model.Subject{}, err
	}

	var stdClaims jwt.Claims
	custom := map[string]any{}
	if err := token.Claims(key, &stdClaims, &custom); err != nil {
		return model.Subject{}, err
	}

	now := time.Now().UTC()
	if err := stdClaims.Validate(jwt.Expected{Issuer: v.issuer, Time: now}); err != nil {
		return model.Subject{}, err
	}
	if !v.audienceMatches(stdClaims, custom) {
		return model.Subject{}, errors.New("audience mismatch")
	}

	subject := model.Subject{
		Sub:      stdClaims.Subject,
		Username: claimString(custom, v.usernameClaim),
		Groups:   claimStrings(custom, v.groupsClaim),
	}
	if subject.Username == "" {
		subject.Username = subject.Sub
	}

	return subject, nil
}

func (v *Validator) audienceMatches(claims jwt.Claims, custom map[string]any) bool {
	if v.audience == "" {
		return true
	}
	audOk := claims.Audience.Contains(v.audience)
	if v.audienceMode == "aud_or_azp" {
		azp := claimString(custom, "azp")
		return audOk || azp == v.audience
	}
	return audOk
}

func (v *Validator) keyForKID(ctx context.Context, kid string) (any, error) {
	if strings.TrimSpace(kid) == "" {
		return nil, errors.New("missing kid")
	}

	if key, ok := v.cachedKey(kid); ok {
		return key, nil
	}
	if err := v.refreshJWKS(ctx, true); err != nil {
		return nil, err
	}
	if key, ok := v.cachedKey(kid); ok {
		return key, nil
	}

	return nil, fmt.Errorf("unknown kid: %s", kid)
}

func (v *Validator) cachedKey(kid string) (any, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if time.Now().After(v.cache.expiresAt) {
		return nil, false
	}

	key, ok := v.cache.keys[kid]
	if !ok {
		return nil, false
	}

	return key.Key, true
}

func (v *Validator) refreshJWKS(ctx context.Context, force bool) error {
	v.mu.RLock()
	cacheValid := time.Now().Before(v.cache.expiresAt)
	v.mu.RUnlock()
	if cacheValid && !force {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("jwks fetch failed: status %d", resp.StatusCode)
	}

	var set jose.JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return err
	}

	keys := make(map[string]jose.JSONWebKey, len(set.Keys))
	for _, key := range set.Keys {
		if key.KeyID == "" || key.Key == nil {
			continue
		}
		if key.Use != "" && key.Use != "sig" {
			continue
		}
		keys[key.KeyID] = key
	}

	v.mu.Lock()
	v.cache.keys = keys
	v.cache.expiresAt = time.Now().Add(v.cacheTTL)
	v.mu.Unlock()

	return nil
}

func claimString(claims map[string]any, name string) string {
	if raw, ok := claims[name]; ok {
		if value, ok := raw.(string); ok {
			return value
		}
	}
	return ""
}

func claimStrings(claims map[string]any, name string) []string {
	raw, ok := claims[name]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	case []string:
		return value
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil
		}
		if strings.Contains(trimmed, " ") {
			return strings.Fields(trimmed)
		}
		if strings.Contains(trimmed, ",") {
			parts := strings.Split(trimmed, ",")
			out := make([]string, 0, len(parts))
			for _, part := range parts {
				p := strings.TrimSpace(part)
				if p != "" {
					out = append(out, p)
				}
			}
			return out
		}
		return []string{trimmed}
	default:
		return nil
	}
}
