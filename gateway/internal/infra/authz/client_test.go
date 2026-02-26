package authorize

import (
	"log/slog"
	"os"
	"testing"

	"ztna-gateway/internal/config"
)

func TestNewClient(t *testing.T) {
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL: "https://cp.example.com",
		},
		PEP: config.PEPConfig{
			ID:    "gw-1",
			Token: "secret-token",
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	client := NewClient(cfg, log)

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.cfg != cfg {
		t.Error("NewClient() did not store config")
	}
	if client.log != log {
		t.Error("NewClient() did not store logger")
	}
}

func TestAuthorize_NotImplemented(t *testing.T) {
	cfg := &config.Config{
		ControlPlane: config.ControlPlaneConfig{
			BaseURL: "https://cp.example.com",
		},
		PEP: config.PEPConfig{
			ID:    "gw-1",
			Token: "secret-token",
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client := NewClient(cfg, log)

	req := &AuthzRequest{
		Action: "connect",
	}

	_, err := client.Authorize(req)

	if err == nil {
		t.Error("Authorize() should return error (not implemented)")
	}
}
