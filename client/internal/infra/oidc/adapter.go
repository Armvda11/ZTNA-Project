package oidc

import (
	"context"
	"time"

	"client/internal/core/domain"
	"client/internal/core/ports"
)

// Adapter expose le client OIDC infra via les contrats core/ports.
type Adapter struct {
	client *Client
}

// NewAdapter construit un adaptateur OIDC pour les usecases.
func NewAdapter(client *Client) *Adapter {
	return &Adapter{client: client}
}

// LoginPasswordGrantLAB exécute le login lab et mappe la réponse vers LoginResult.
func (a *Adapter) LoginPasswordGrantLAB(ctx context.Context, username, password string) (*ports.LoginResult, error) {
	tokens, err := a.client.LoginPasswordGrantLAB(ctx, username, password)
	if err != nil {
		return nil, err
	}
	return toLoginResult(tokens), nil
}

// DeviceFlowLogin exécute le login device flow et mappe la réponse vers LoginResult.
func (a *Adapter) DeviceFlowLogin(ctx context.Context) (*ports.LoginResult, error) {
	tokens, err := a.client.DeviceFlowLogin(ctx)
	if err != nil {
		return nil, err
	}
	return toLoginResult(tokens), nil
}

func toLoginResult(tokens *TokenSet) *ports.LoginResult {
	if tokens == nil {
		return &ports.LoginResult{}
	}

	// TODO: Extraire SubjectRef depuis les claims du id_token quand le parsing
	// JWT sécurisé sera en place (validation signature + audience + nonce).
	return &ports.LoginResult{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    coalesceTime(tokens.ExpiresAt),
		Subject:      domain.SubjectRef{},
	}
}

func coalesceTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now()
	}
	return value
}
