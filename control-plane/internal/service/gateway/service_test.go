package gateway

import (
	"context"
	"testing"

	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
)

type fakeRepo struct {
	items map[string]model.Gateway
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]model.Gateway{}}
}

func (r *fakeRepo) RegisterGateway(_ context.Context, gw model.Gateway) error {
	if gw.Name == "" {
		gw.Name = gw.ID
	}
	gw.Active = true
	r.items[gw.ID] = gw
	return nil
}

func (r *fakeRepo) UpdateGatewayHeartbeat(_ context.Context, id, version string) error {
	gw, ok := r.items[id]
	if !ok {
		return domainErrors.ErrNotFound
	}
	gw.LastSeen = "now"
	if version != "" {
		gw.SoftwareVersion = version
	}
	r.items[id] = gw
	return nil
}

func (r *fakeRepo) GetGateway(_ context.Context, id string) (model.Gateway, error) {
	gw, ok := r.items[id]
	if !ok {
		return model.Gateway{}, domainErrors.ErrNotFound
	}
	return gw, nil
}

func (r *fakeRepo) SetGatewayActive(_ context.Context, id string, active bool) error {
	gw, ok := r.items[id]
	if !ok {
		return domainErrors.ErrNotFound
	}
	gw.Active = active
	r.items[id] = gw
	return nil
}

func (r *fakeRepo) ListGateways(_ context.Context) ([]model.Gateway, error) {
	out := make([]model.Gateway, 0, len(r.items))
	for _, gw := range r.items {
		out = append(out, gw)
	}
	return out, nil
}

func TestStatusUnregistered(t *testing.T) {
	svc := New(newFakeRepo())
	status, err := svc.Status(context.Background(), "gw-1")
	if err != domainErrors.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if status != HeartbeatUnregistered {
		t.Fatalf("expected status=%q got=%q", HeartbeatUnregistered, status)
	}
}

func TestRegisterThenHeartbeat(t *testing.T) {
	svc := New(newFakeRepo())
	if err := svc.Register(context.Background(), model.Gateway{
		ID:              "gw-1",
		Name:            "gw-1",
		Fingerprint:     "fp",
		SoftwareVersion: "v1",
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	status, err := svc.Heartbeat(context.Background(), "gw-1", "v2")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if status != HeartbeatRegistered {
		t.Fatalf("expected status=%q got=%q", HeartbeatRegistered, status)
	}
}

func TestRevokedGatewayHeartbeat(t *testing.T) {
	repo := newFakeRepo()
	svc := New(repo)
	_ = svc.Register(context.Background(), model.Gateway{ID: "gw-1", Name: "gw-1"})
	if err := repo.SetGatewayActive(context.Background(), "gw-1", false); err != nil {
		t.Fatalf("set inactive: %v", err)
	}

	status, err := svc.Heartbeat(context.Background(), "gw-1", "")
	if err != domainErrors.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if status != HeartbeatRevoked {
		t.Fatalf("expected status=%q got=%q", HeartbeatRevoked, status)
	}
}
