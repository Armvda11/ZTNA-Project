package oidc

import (
	"log/slog"
	"os"
	"testing"

	"client/internal/config"
)

func TestNewClient(t *testing.T) {
	cfg := &config.Config{
		OIDC: config.OIDCConfig{
			Issuer:   "https://auth.example.com",
			ClientID: "test-client",
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
