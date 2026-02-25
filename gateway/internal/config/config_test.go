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
	"time"
)

func TestLoad_ValidConfig(t *testing.T) {
	yaml := `
listen_addr: "0.0.0.0:4433"
gateway_id: "gw-test"
cp_url: "https://10.10.20.30:8080"
cp_auth_mode: "token"
pep_id: "gw-test"
pep_token: "secret"
cp_tls_insecure: true
heartbeat_every: 30s
decision_cache_ttl: 45s
decision_cache_max_entries: 1234
cp_down_mode: "cache_allow"
routes:
  - resource_type: "ssh"
    resource_match: "ssh:lan-app:22"
    target: "10.10.30.10:22"
  - resource_type: "http"
    resource_match: "http:lan-app:80"
    target: "10.10.30.10:80"
`
	f, err := os.CreateTemp("", "gw-config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yaml)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.ListenAddr != "0.0.0.0:4433" {
		t.Errorf("ListenAddr: %q", cfg.ListenAddr)
	}
	if cfg.CPURL != "https://10.10.20.30:8080" {
		t.Errorf("CPURL: %q", cfg.CPURL)
	}
	if cfg.HeartbeatEvery != 30*time.Second {
		t.Errorf("HeartbeatEvery: %v", cfg.HeartbeatEvery)
	}
	if cfg.DecisionCacheTTL != 45*time.Second {
		t.Errorf("DecisionCacheTTL: %v", cfg.DecisionCacheTTL)
	}
	if cfg.DecisionCacheMaxKeys != 1234 {
		t.Errorf("DecisionCacheMaxKeys: %d", cfg.DecisionCacheMaxKeys)
	}
	if cfg.CPDownMode != "cache_allow" {
		t.Errorf("CPDownMode: %q", cfg.CPDownMode)
	}
	if len(cfg.Routes) != 2 {
		t.Errorf("Routes count: %d", len(cfg.Routes))
	}
	if cfg.Routes[0].Target != "10.10.30.10:22" {
		t.Errorf("Routes[0].Target: %q", cfg.Routes[0].Target)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() doit retourner une erreur pour un fichier inexistant")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	f, _ := os.CreateTemp("", "gw-config-*.yaml")
	defer os.Remove(f.Name())
	f.WriteString("{{{{ not yaml ::::")
	f.Close()

	_, err := Load(f.Name())
	if err == nil {
		t.Error("Load() doit retourner une erreur pour du YAML invalide")
	}
}
