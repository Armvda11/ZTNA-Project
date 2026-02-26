// Package login orchestre les workflows d'authentification client.
package login

import (
	"context"
	"fmt"
	"log/slog"

	"client/internal/core/ports"
)

// Mode indique le flux d'authentification à utiliser.
type Mode string

const (
	ModeAuto             Mode = "auto"
	ModeDeviceFlow       Mode = "device_flow"
	ModePasswordGrantLAB Mode = "password_grant_lab"
)

// Options paramètre le workflow de login.
type Options struct {
	Mode     Mode
	Username string
	Password string
}

// Service exécute les cas d'usage d'authentification.
type Service struct {
	idp ports.IdentityProvider
	log *slog.Logger
}

// NewService construit un usecase login.
func NewService(idp ports.IdentityProvider, log *slog.Logger) *Service {
	return &Service{idp: idp, log: log}
}

// Run exécute un login selon le mode demandé.
func (s *Service) Run(ctx context.Context, opts Options) (*ports.LoginResult, error) {
	mode := opts.Mode
	if mode == "" {
		mode = ModeAuto
	}

	s.log.Info("login usecase démarré", "mode", mode)

	switch mode {
	case ModeAuto, ModeDeviceFlow:
		result, err := s.idp.DeviceFlowLogin(ctx)
		if err != nil {
			return nil, fmt.Errorf("échec login device flow: %w", err)
		}
		return result, nil
	case ModePasswordGrantLAB:
		if opts.Username == "" || opts.Password == "" {
			return nil, fmt.Errorf("username/password requis pour le mode %s", ModePasswordGrantLAB)
		}
		result, err := s.idp.LoginPasswordGrantLAB(ctx, opts.Username, opts.Password)
		if err != nil {
			return nil, fmt.Errorf("échec login password grant lab: %w", err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("mode de login non supporté: %s", mode)
	}
}
