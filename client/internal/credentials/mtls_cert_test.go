package credentials

import (
	"log/slog"
	"os"
	"testing"

	"client/internal/config"
)

func TestNewClient(t *testing.T) {
	cfg := &config.Config{}
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

func TestRequestMTLSCertFromCP_NotImplemented(t *testing.T) {
	cfg := &config.Config{}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client := NewClient(cfg, log)

	err := client.RequestMTLSCertFromCP("fake-token")

	if err == nil {
		t.Error("RequestMTLSCertFromCP() should return an error (not implemented)")
	}
	// Vérifier que le message contient "TODO" ou "non implémenté"
	msg := err.Error()
	if !contains(msg, "TODO") && !contains(msg, "non implémenté") {
		t.Errorf("RequestMTLSCertFromCP() error message should indicate not implemented, got: %q", msg)
	}
}

func TestSaveCertAndKey_NotImplemented(t *testing.T) {
	cfg := &config.Config{}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	client := NewClient(cfg, log)

	err := client.SaveCertAndKey([]byte("cert"), []byte("key"))

	if err == nil {
		t.Error("SaveCertAndKey() should return an error (not implemented)")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
