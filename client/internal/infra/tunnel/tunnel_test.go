package tunnel

import (
	"log/slog"
	"os"
	"testing"

	"client/internal/config"
)

func TestNewManager(t *testing.T) {
	cfg := &config.Config{}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mgr := NewManager(cfg, log)

	if mgr == nil {
		t.Fatal("NewManager() returned nil")
	}
	if mgr.cfg != cfg {
		t.Error("NewManager() did not store config")
	}
	if mgr.log != log {
		t.Error("NewManager() did not store logger")
	}
}

func TestBuildTLSConfig_MissingCert(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Address: "gateway.example.com:9443",
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mgr := NewManager(cfg, log)

	// Certificat invalide (vide)
	_, err := mgr.buildTLSConfig([]byte(""), []byte(""))

	if err == nil {
		t.Error("buildTLSConfig() should return error for invalid cert")
	}
}

func TestBuildTLSConfig_ValidCertAndKey(t *testing.T) {
	// Certificat et clé de test valides (PEM)
	certPEM := []byte(`-----BEGIN CERTIFICATE-----
MIIBhTCCASugAwIBAgIQIRi6zePL6mKjOipn+dNuaTAKBggqhkjOPQQDAjASMRAw
DgYDVQQKEwdBY21lIENvMB4XDTE3MTAyMDE5NDMwNloXDTE4MTAyMDE5NDMwNlow
EjEQMA4GA1UEChMHQWNtZSBDbzBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABD0d
7VNhbWvZLWPuj/RtHFjvtJBEwOkhbN/BnnE8rnZR8+sbwnc/KhCk3FhnpHZnQz7B
5aETbbIgmuvewdjvSBSjYzBhMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggr
BgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MCkGA1UdEQQiMCCCDmxvY2FsaG9zdDo1
NDUzgg4xMjcuMC4wLjE6NTQ1MzAKBggqhkjOPQQDAgNIADBFAiEA2zpJEPQyz6/l
Wf86aX6PepsntZv2GYlA5UpabfT2EZICICpJ5h/iI+i341gBmLiAFQOyTDT+/wQc
6MF9+Yw1Yy0t
-----END CERTIFICATE-----`)

	keyPEM := []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIIrYSSNQFaA2Hwf1duRSxKtLYX5CB04fSeQ6tF1aY/PuoAoGCCqGSM49
AwEHoUQDQgAEPR3tU2Fta9ktY+6P9G0cWO+0kETA6SFs38GecTyudlHz6xvCdz8q
EKTcWGekdmdDPsHloRNtsiCa697B2O9IFA==
-----END EC PRIVATE KEY-----`)

	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Address: "gateway.example.com:9443",
		},
		TLS: config.TLSConfig{
			CAFile: "",
		},
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	mgr := NewManager(cfg, log)

	tlsConfig, err := mgr.buildTLSConfig(certPEM, keyPEM)

	if err != nil {
		t.Fatalf("buildTLSConfig() error = %v", err)
	}
	if tlsConfig == nil {
		t.Fatal("buildTLSConfig() returned nil config")
	}
	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("len(Certificates) = %d, want 1", len(tlsConfig.Certificates))
	}
	if tlsConfig.ServerName != "gateway.example.com" {
		t.Errorf("ServerName = %q, want %q", tlsConfig.ServerName, "gateway.example.com")
	}
}
