// Package listresources implémente le cas d'usage « lister les ressources
// publiées accessibles à l'utilisateur connecté ».
package listresources

import (
	"context"
	"fmt"
	"log/slog"

	"client/internal/core/ports"
	"client/internal/infra/cpapi"
)

// Service orchestre la récupération et l'affichage des ressources publiées.
type Service struct {
	tokens ports.TokenProvider
	cp     *cpapi.Client
	log    *slog.Logger
}

// NewService crée un nouveau service de listage de ressources.
func NewService(tokens ports.TokenProvider, cp *cpapi.Client, log *slog.Logger) *Service {
	return &Service{tokens: tokens, cp: cp, log: log}
}

// Run liste les ressources publiées accessibles à l'utilisateur.
func (s *Service) Run(ctx context.Context) ([]cpapi.PublishedResource, error) {
	accessToken, err := s.tokens.GetValidAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("impossible d'obtenir un access token valide: %w", err)
	}

	resources, err := s.cp.ListResources(accessToken)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des ressources: %w", err)
	}

	s.log.Info("ressources récupérées", "count", len(resources))
	return resources, nil
}