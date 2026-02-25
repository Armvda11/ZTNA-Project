// Package port defines the repository interfaces (output ports) that the
// domain services depend on. Concrete implementations live in store/.
package port

import (
	"context"

	"control-plane/internal/domain/model"
)

// UserRepository persists user identity records.
type UserRepository interface {
	UpsertUser(ctx context.Context, subject model.Subject) error
}

// PolicyRepository persists policy versions and rules.
type PolicyRepository interface {
	CreatePolicyVersion(ctx context.Context, createdBy string, rules []model.PolicyRule) (int64, error)
	ActivatePolicyVersion(ctx context.Context, id int64) error
	GetActivePolicy(ctx context.Context) (model.PolicySnapshot, error)
}

// AuditRepository persists and queries audit events.
type AuditRepository interface {
	InsertAuditEvent(ctx context.Context, event model.AuditEvent) error
	ListAuditEvents(ctx context.Context, limit, offset int) ([]model.AuditEvent, error)
}

// DeviceCertRepository persists X.509 device certificates (used for mTLS
// between clients and gateways).
type DeviceCertRepository interface {
	StoreDeviceCert(ctx context.Context, cert model.DeviceCert) error
	RevokeDeviceCert(ctx context.Context, serial, reason string) error
	ListRevokedDeviceCerts(ctx context.Context) ([]model.DeviceCert, error)
	GetDeviceCert(ctx context.Context, serial string) (model.DeviceCert, error)
}

// GatewayRepository persists gateway registration and liveness data.
type GatewayRepository interface {
	RegisterGateway(ctx context.Context, gw model.Gateway) error
	UpdateGatewayHeartbeat(ctx context.Context, id, version string) error
	GetGateway(ctx context.Context, id string) (model.Gateway, error)
	SetGatewayActive(ctx context.Context, id string, active bool) error
	ListGateways(ctx context.Context) ([]model.Gateway, error)
}
