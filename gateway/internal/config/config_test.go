package config

import (
	"os"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := []byte(`
server:
  listen_addr: "0.0.0.0:9443"
  tls:
    cert_file: "./certs/gateway.crt"
    key_file: "./certs/gateway.key"
    client_ca_file: "./certs/client-ca.crt"
control_plane:
  base_url: "https://10.10.20.30:8443"
pep:
  id: "ztna-gw-1"
  token: "test-token-value"
logging:
  level: debug
  format: text
`)
	tmpFile, err := os.CreateTemp("", "gw-config-*.yaml")
	if err != nil {
		t.Fatalf("impossible de créer le fichier temporaire: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("impossible d'écrire la configuration: %v", err)
	}
	tmpFile.Close()

	cfg, err := Load(tmpFile.Name())
	if err != nil {
		t.Fatalf("Load() a échoué: %v", err)
	}

	if cfg.Server.ListenAddr != "0.0.0.0:9443" {
		t.Errorf("listen_addr attendu 0.0.0.0:9443, obtenu %s", cfg.Server.ListenAddr)
	}
	if cfg.PEP.ID != "ztna-gw-1" {
		t.Errorf("pep.id attendu ztna-gw-1, obtenu %s", cfg.PEP.ID)
	}
}

func TestLoad_MissingCertFile(t *testing.T) {
	content := []byte(`
server:
  tls:
    key_file: "./certs/gateway.key"
    client_ca_file: "./certs/client-ca.crt"
control_plane:
  base_url: "https://localhost:8443"
pep:
  id: "gw-1"
  token: "test"
`)
	tmpFile, err := os.CreateTemp("", "gw-config-*.yaml")
	if err != nil {
		t.Fatalf("impossible de créer le fichier temporaire: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("impossible d'écrire la configuration: %v", err)
	}
	tmpFile.Close()

	_, err = Load(tmpFile.Name())
	if err == nil {
		t.Fatal("Load() aurait dû échouer avec cert_file manquant")
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Server.ListenAddr != "0.0.0.0:9443" {
		t.Errorf("listen_addr par défaut attendu 0.0.0.0:9443, obtenu %s", cfg.Server.ListenAddr)
	}
	if cfg.Proxy.DialTimeout != "10s" {
		t.Errorf("dial_timeout par défaut attendu 10s, obtenu %s", cfg.Proxy.DialTimeout)
	}
	if cfg.Proxy.MaxConns != 1000 {
		t.Errorf("max_conns par défaut attendu 1000, obtenu %d", cfg.Proxy.MaxConns)
	}
}

func TestValidate_ShortToken(t *testing.T) {
	content := []byte(`
server:
  listen_addr: "0.0.0.0:9443"
  tls:
    cert_file: "./certs/gateway.crt"
    key_file: "./certs/gateway.key"
    client_ca_file: "./certs/client-ca.crt"
control_plane:
  base_url: "https://10.10.20.30:8443"
pep:
  id: "ztna-gw-1"
  token: "short"
`)
	tmpFile, err := os.CreateTemp("", "gw-config-*.yaml")
	if err != nil {
		t.Fatalf("impossible de créer le fichier temporaire: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("impossible d'écrire la configuration: %v", err)
	}
	tmpFile.Close()

	_, err = Load(tmpFile.Name())
	if err == nil {
		t.Fatal("Load() aurait dû échouer avec un token trop court")
	}
}

func TestValidate_InvalidDialTimeout(t *testing.T) {
	content := []byte(`
server:
  listen_addr: "0.0.0.0:9443"
  tls:
    cert_file: "./certs/gateway.crt"
    key_file: "./certs/gateway.key"
    client_ca_file: "./certs/client-ca.crt"
control_plane:
  base_url: "https://10.10.20.30:8443"
pep:
  id: "ztna-gw-1"
  token: "valid-token-12345678"
proxy:
  dial_timeout: "invalid-duration"
`)
	tmpFile, err := os.CreateTemp("", "gw-config-*.yaml")
	if err != nil {
		t.Fatalf("impossible de créer le fichier temporaire: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("impossible d'écrire la configuration: %v", err)
	}
	tmpFile.Close()

	_, err = Load(tmpFile.Name())
	if err == nil {
		t.Fatal("Load() aurait dû échouer avec un dial_timeout invalide")
	}
}

func TestDialTimeoutDuration(t *testing.T) {
	cfg := &Config{
		Proxy: ProxyConfig{DialTimeout: "15s"},
	}

	duration := cfg.DialTimeoutDuration()
	if duration.Seconds() != 15 {
		t.Errorf("attendu 15s, obtenu %v", duration)
	}
}
