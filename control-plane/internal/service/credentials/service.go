package credentials

import (
	"context"
	"fmt"
	"strings"
	"time"

	"control-plane/internal/config"
	"control-plane/internal/crypto/sshca"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/store/sqlite"

	"golang.org/x/crypto/ssh"
)

type Service struct {
	ca  *sshca.CA
	cfg config.SSHCAConfig
	db  *sqlite.Store
}

func New(ca *sshca.CA, cfg config.SSHCAConfig, db *sqlite.Store) *Service {
	return &Service{ca: ca, cfg: cfg, db: db}
}

type IssueRequest struct {
	Subject   model.Subject
	PublicKey string
	TTL       *time.Duration
	Principals []string
}

type IssueResponse struct {
	Certificate string
	ValidBefore time.Time
	KeyID       string
	Principals  []string
}

func (s *Service) IssueSSHCert(ctx context.Context, req IssueRequest) (IssueResponse, error) {
	if err := s.db.UpsertUser(ctx, req.Subject); err != nil {
		return IssueResponse{}, err
	}

	parsedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(req.PublicKey))
	if err != nil {
		return IssueResponse{}, fmt.Errorf("parse public key: %w", err)
	}

	ttl, err := time.ParseDuration(s.cfg.DefaultTTL)
	if err != nil {
		return IssueResponse{}, fmt.Errorf("invalid sshca default ttl: %w", err)
	}
	if req.TTL != nil {
		ttl = *req.TTL
	}

	// Validate and clamp TTL to configured bounds
	minTTL := parseDurationOrZero(s.cfg.MinTTL)
	maxTTL := parseDurationOrZero(s.cfg.MaxTTL)
	if minTTL > 0 && ttl < minTTL {
		return IssueResponse{}, domainErrors.ErrInvalidInput
	}
	// Clamp to max_ttl if configured (security: prevent long-lived certs)
	if maxTTL > 0 && ttl > maxTTL {
		ttl = maxTTL // Clamp instead of reject for better UX
	}

	allowed := resolvePrincipals(s.cfg.AllowedPrincipals, req.Subject)
	if len(allowed) == 0 {
		return IssueResponse{}, domainErrors.ErrInvalidInput
	}

	principals, err := resolveRequestedPrincipals(req.Principals, allowed, req.Subject.Username)
	if err != nil {
		return IssueResponse{}, err
	}

	keyID := req.Subject.Sub
	if keyID == "" {
		keyID = req.Subject.Username
	}

	cert, validBefore, err := s.ca.SignUserCert(parsedKey, principals, ttl, keyID)
	if err != nil {
		return IssueResponse{}, err
	}

	return IssueResponse{
		Certificate: cert,
		ValidBefore: validBefore,
		KeyID:       keyID,
		Principals:  principals,
	}, nil
}

func resolvePrincipals(allowed []string, subject model.Subject) []string {
	if len(allowed) == 0 {
		return nil
	}
	principals := make([]string, 0, len(allowed))
	for _, raw := range allowed {
		value := strings.ReplaceAll(raw, "${username}", subject.Username)
		value = strings.ReplaceAll(value, "${sub}", subject.Sub)
		value = strings.TrimSpace(value)
		if value != "" {
			principals = append(principals, value)
		}
	}
	return principals
}

func parseDurationOrZero(raw string) time.Duration {
	if raw == "" {
		return 0
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}
	return parsed
}

func resolveRequestedPrincipals(requested []string, allowed []string, username string) ([]string, error) {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		allowedSet[value] = struct{}{}
	}

	if len(requested) == 0 {
		if username != "" {
			if _, ok := allowedSet[username]; ok {
				return []string{username}, nil
			}
		}
		return []string{allowed[0]}, nil
	}

	seen := map[string]struct{}{}
	filtered := make([]string, 0, len(requested))
	for _, value := range requested {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := allowedSet[trimmed]; !ok {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		filtered = append(filtered, trimmed)
	}

	if len(filtered) == 0 {
		return nil, domainErrors.ErrInvalidInput
	}
	if username != "" {
		if _, ok := allowedSet[username]; ok && !containsPrincipal(filtered, username) {
			return nil, domainErrors.ErrInvalidInput
		}
	}

	return filtered, nil
}

func containsPrincipal(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
