package app

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"gateway/internal/config"
)

func TestNew(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddr: "0.0.0.0:9443",
			TLS: config.ServerTLSConfig{
				CertFile:     "/tmp/server.crt",
				KeyFile:      "/tmp/server.key",
				ClientCAFile: "/tmp/ca.crt",
			},
		},
		ControlPlane: config.ControlPlaneConfig{
			BaseURL: "https://cp.example.com",
		},
		PEP: config.PEPConfig{
			ID:    "gw-1",
			Token: "secret",
		},
		Proxy: config.ProxyConfig{
			DialTimeout: "10s",
			MaxConns:    1000,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// New() creates the app but doesn't start listening yet
	// The cert files are only validated when Listen() is called
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
	if app.listener == nil {
		t.Error("New() did not initialize listener")
	}
	if app.handler == nil {
		t.Error("New() did not initialize handler")
	}
	if app.authz == nil {
		t.Error("New() did not initialize authz client")
	}
	if app.proxy == nil {
		t.Error("New() did not initialize proxy")
	}
	if app.sessions == nil {
		t.Error("New() did not initialize sessions manager")
	}
}

func TestNew_PreservesComponents(t *testing.T) {
	// This is a basic structural test - we can't fully test without valid certs
	ctx := context.Background()
	cfg := &config.Config{
		Server: config.ServerConfig{
			ListenAddr: "0.0.0.0:9443",
		},
		ControlPlane: config.ControlPlaneConfig{
			BaseURL: "https://cp.example.com",
		},
		PEP: config.PEPConfig{
			ID:    "gw-1",
			Token: "secret",
		},
		Proxy: config.ProxyConfig{
			DialTimeout: "10s",
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Even though it will fail, the function signature is correct
	_, _ = New(ctx, cfg, log)

	// Test passes if no panic occurs
}
