package config

import (
	"os"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := []byte("oidc:\n" +
		"  issuer: \"https://localhost:8081/realms/ztna\"\n" +
		"  client_id: \"ztna-client\"\n" +
		"control_plane:\n" +
		"  base_url: \"https://localhost:8080\"\n" +
		"gateway:\n" +
		"  address: \"localhost:9443\"\n" +
		"logging:\n" +
		"  level: debug\n" +
		"  format: text\n")
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
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

	if cfg.OIDC.Issuer != "https://localhost:8081/realms/ztna" {
		t.Errorf("issuer attendu https://localhost:8081/realms/ztna, obtenu %s", cfg.OIDC.Issuer)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("level attendu debug, obtenu %s", cfg.Logging.Level)
	}
}

func TestLoad_MissingIssuer(t *testing.T) {
	content := []byte(`
oidc:
  client_id: "ztna-client"
control_plane:
  base_url: "https://localhost:8080"
gateway:
  address: "localhost:9443"
`)
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
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
		t.Fatal("Load() aurait dû échouer avec un issuer manquant")
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Logging.Level != "info" {
		t.Errorf("level par défaut attendu info, obtenu %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("format par défaut attendu json, obtenu %s", cfg.Logging.Format)
	}
	if cfg.Storage.Path != "./.ztna" {
		t.Errorf("storage.path par défaut attendu ./.ztna, obtenu %s", cfg.Storage.Path)
	}
}

func TestValidate_HTTPIssuerWithoutInsecure(t *testing.T) {
	content := []byte(`
oidc:
  issuer: "http://localhost:8081/realms/ztna"
  client_id: "ztna-client"
control_plane:
  base_url: "https://localhost:8080"
gateway:
  address: "localhost:9443"
security:
  insecure_allow_http_oidc: false
`)
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
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
		t.Fatal("Load() aurait dû échouer avec un issuer HTTP sans insecure_allow_http_oidc")
	}
}

func TestValidate_HTTPIssuerWithInsecure(t *testing.T) {
	content := []byte(`
oidc:
  issuer: "http://localhost:8081/realms/ztna"
  client_id: "ztna-client"
control_plane:
  base_url: "https://localhost:8080"
gateway:
  address: "localhost:9443"
security:
  insecure_allow_http_oidc: true
`)
	tmpFile, err := os.CreateTemp("", "config-*.yaml")
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
		t.Fatalf("Load() ne devrait pas échouer avec insecure_allow_http_oidc: %v", err)
	}
	if cfg.OIDC.Issuer != "http://localhost:8081/realms/ztna" {
		t.Errorf("issuer attendu http://localhost:8081/realms/ztna, obtenu %s", cfg.OIDC.Issuer)
	}
}
