// Package gateway manages gateway (PEP) registrations and liveness tracking.
package gateway

import (
	"context"
	"fmt"

	domainErrors "control-plane/internal/domain/errors"
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
	if gw.ID == "" {
		return domainErrors.ErrInvalidInput
	}
	if gw.Name == "" {
		gw.Name = gw.ID
	}
	return s.repo.RegisterGateway(ctx, gw)
}

// HeartbeatStatus defines the strict status returned to gateways.
type HeartbeatStatus string

const (
	HeartbeatRegistered   HeartbeatStatus = "registered"
	HeartbeatUnregistered HeartbeatStatus = "unregistered"
	HeartbeatRevoked      HeartbeatStatus = "revoked"
)

// Heartbeat records liveness for a registered gateway and returns strict status.
func (s *Service) Heartbeat(ctx context.Context, id, version string) (HeartbeatStatus, error) {
	status, err := s.Status(ctx, id)
	if err != nil {
		return status, err
	}
	if status != HeartbeatRegistered {
		return status, domainErrors.ErrForbidden
	}
	if err := s.repo.UpdateGatewayHeartbeat(ctx, id, version); err != nil {
		if err == domainErrors.ErrNotFound {
			return HeartbeatUnregistered, domainErrors.ErrForbidden
		}
		return HeartbeatUnregistered, fmt.Errorf("update heartbeat: %w", err)
	}
	return HeartbeatRegistered, nil
}

// Status returns strict registration status without mutating liveness.
func (s *Service) Status(ctx context.Context, id string) (HeartbeatStatus, error) {
	if id == "" {
		return HeartbeatUnregistered, domainErrors.ErrUnauthorized
	}
	gw, err := s.repo.GetGateway(ctx, id)
	if err != nil {
		if err == domainErrors.ErrNotFound {
			return HeartbeatUnregistered, domainErrors.ErrForbidden
		}
		return HeartbeatUnregistered, fmt.Errorf("get gateway: %w", err)
	}
	if !gw.Active {
		return HeartbeatRevoked, domainErrors.ErrForbidden
	}
	return HeartbeatRegistered, nil
}

// List returns all registered gateways.
func (s *Service) List(ctx context.Context) ([]model.Gateway, error) {
	return s.repo.ListGateways(ctx)
}
