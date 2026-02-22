// Package gateway manages gateway (PEP) registrations and liveness tracking.
package gateway

import (
	"context"

	"control-plane/internal/domain/model"
	"control-plane/internal/domain/port"
)

// Service manages gateway registrations and heartbeats.
type Service struct {
	repo port.GatewayRepository
}

// New creates the gateway service.
func New(repo port.GatewayRepository) *Service {
	return &Service{repo: repo}
}

// Register idempotently registers a gateway.
func (s *Service) Register(ctx context.Context, gw model.Gateway) error {
	return s.repo.RegisterGateway(ctx, gw)
}

// Heartbeat records the last-seen timestamp for a gateway.
func (s *Service) Heartbeat(ctx context.Context, id string) error {
	return s.repo.UpdateGatewayHeartbeat(ctx, id)
}

// List returns all registered gateways.
func (s *Service) List(ctx context.Context) ([]model.Gateway, error) {
	return s.repo.ListGateways(ctx)
}
