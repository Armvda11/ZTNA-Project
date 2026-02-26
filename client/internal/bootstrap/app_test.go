package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"client/internal/config"
)

func TestNew(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   "https://auth.example.com",
			ClientID: "test-client",
		},
		Gateway: config.GatewayConfig{
			Address: "gateway.example.com:9443",
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	app, err := New(ctx, cfg, log)

	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if app == nil {
		t.Fatal("New() returned nil app")
	}
	if app.cfg != cfg {
		t.Error("New() did not store config")
	}
	if app.log != log {
		t.Error("New() did not store logger")
	}
	if app.oidc == nil {
		t.Error("New() did not initialize OIDC client")
	}
	if app.creds == nil {
		t.Error("New() did not initialize credentials client")
	}
	if app.tunnel == nil {
		t.Error("New() did not initialize tunnel manager")
	}
}

func TestRunLogin_NotImplemented(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   "https://auth.example.com",
			ClientID: "test-client",
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app, _ := New(ctx, cfg, log)

	err := app.RunLogin(ctx)

	if err == nil {
		t.Error("RunLogin() should return error (not implemented)")
	}
}

func TestRunCert_NotImplemented(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app, _ := New(ctx, cfg, log)

	err := app.RunCert(ctx)

	if err == nil {
		t.Error("RunCert() should return error (not implemented)")
	}
}

func TestRunConnect_NotImplemented(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app, _ := New(ctx, cfg, log)

	err := app.RunConnect(ctx, "ssh://backend:22")

	if err == nil {
		t.Error("RunConnect() should return error (not implemented)")
	}
}
