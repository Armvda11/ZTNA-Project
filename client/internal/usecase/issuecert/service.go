// Package issuecert orchestre la demande de certificat mTLS client.
package issuecert

import (
	"context"
	"fmt"
	"log/slog"

	"client/internal/core/domain"
	"client/internal/core/ports"
)

// Service implémente le cas d'usage de délivrance de certificat client.
type Service struct {
	tokens ports.TokenProvider
	issuer ports.CertificateIssuer
	log    *slog.Logger
}

// NewService crée un usecase de demande de certificat.
func NewService(tokens ports.TokenProvider, issuer ports.CertificateIssuer, log *slog.Logger) *Service {
	return &Service{tokens: tokens, issuer: issuer, log: log}
}

// Run exécute le workflow de demande de certificat mTLS.
func (s *Service) Run(ctx context.Context) error {
	s.log.Info("issuecert usecase démarré")

	accessToken, err := s.tokens.GetValidAccessToken(ctx)
	if err != nil {
		// TODO: mapper précisément les erreurs infra vers les erreurs domaine.
		return fmt.Errorf("%w: impossible d'obtenir un access token valide: %v", domain.ErrNotAuthenticated, err)
	}

	if err := s.issuer.RequestMTLSCertFromCP(accessToken); err != nil {
		return fmt.Errorf("échec de demande du certificat mTLS: %w", err)
	}

	return nil
}
