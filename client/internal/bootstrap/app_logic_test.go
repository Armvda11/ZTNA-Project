package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"client/internal/config"
	"client/internal/core/domain"
)

// TestCompleteLoginWorkflow tests the end-to-end login workflow.
// Requires a live OIDC server — skipped for unit tests.
func TestCompleteLoginWorkflow(t *testing.T) {
	t.Skip("Requires live OIDC server (integration test)")
}

// TestCompleteCertWorkflow tests the end-to-end certificate request workflow.
// Requires live OIDC server + Control Plane — skipped for unit tests.
func TestCompleteCertWorkflow(t *testing.T) {
	t.Skip("Requires live OIDC + CP servers (integration test)")
}

// TestCompleteConnectWorkflow tests the end-to-end connection workflow.
// Requires live Gateway — skipped for unit tests.
func TestCompleteConnectWorkflow(t *testing.T) {
	t.Skip("Requires live Gateway (integration test)")
}

// TestWorkflowWithExpiredToken tests RunCert without valid tokens.
// Without stored tokens, GetValidAccessToken fails → RunCert returns ErrNotAuthenticated.
func TestWorkflowWithExpiredToken(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   "https://auth.example.com",
			ClientID: "test-client",
		},
		Storage: config.StorageConfig{
			Path: t.TempDir(),
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	app, err := New(ctx, cfg, log)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = app.RunCert(ctx)
	if err == nil {
		t.Fatal("RunCert() should fail when not authenticated")
	}
	if !errors.Is(err, domain.ErrNotAuthenticated) {
		t.Errorf("error should wrap ErrNotAuthenticated, got: %v", err)
	}
}

// TestWorkflowWithExpiredCertificate tests RunConnect with no certificate.
// Skipped: connect usecase loads cert internally — needs mock infra.
func TestWorkflowWithExpiredCertificate(t *testing.T) {
	t.Skip("Requires mock certificate storage + gateway (integration test)")
}

// TestWorkflowNotAuthenticated tests RunCert without prior login.
// Empty token store → ErrNotAuthenticated.
func TestWorkflowNotAuthenticated(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL: "https://cp.example.com",
		},
		Storage: config.StorageConfig{
			Path: t.TempDir(),
		},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	app, err := New(ctx, cfg, log)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = app.RunCert(ctx)
	if err == nil {
		t.Fatal("RunCert() should fail when not authenticated")
	}
	if !errors.Is(err, domain.ErrNotAuthenticated) {
		t.Errorf("error should wrap ErrNotAuthenticated, got: %v", err)
	}
}
