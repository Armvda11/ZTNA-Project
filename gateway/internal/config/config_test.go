package config

import (
	"os"
	"testing"
	"time"
)

func writeTempConfig(t *testing.T, yamlContent string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "gw-config-*.yaml")
	if err != nil {
		t.Fatalf("impossible de créer le fichier temporaire: %v", err)
	}

	if _, err := tmpFile.Write([]byte(yamlContent)); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("impossible d'écrire la configuration: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("impossible de fermer le fichier temporaire: %v", err)
	}

	return tmpFile.Name()
}

func TestLoad_ValidConfig(t *testing.T) {
	yaml := `
server:
  listen_addr: "0.0.0.0:9443"
  tls:
    cert_file: "./certs/gateway.crt"
    key_file: "./certs/gateway.key"
    client_ca_file: "./certs/client-ca.crt"
control_plane:
  base_url: "https://10.10.20.30:8443"
  auth_mode: "token"
  ca_file: "./certs/cp-ca.crt"
  insecure_skip_verify: false
pep:
  id: "ztna-gw-1"
  token: "valid-token-123456"
proxy:
  dial_timeout: "15s"
  max_conns: 250
logging:
  level: debug
  format: text
gateway_id: "ztna-gw-1"
heartbeat_every: 45s
decision_cache_ttl: 90s
decision_cache_max_entries: 1234
cp_down_mode: "cache_allow"
crl_refresh_interval: 75s
routes:
  - resource_type: "ssh"
    resource_match: "ssh:lan-app:22"
    target: "10.10.30.10:22"
`

	path := writeTempConfig(t, yaml)
	defer os.Remove(path)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() a échoué: %v", err)
	}

	if cfg.Server.ListenAddr != "0.0.0.0:9443" {
		t.Errorf("server.listen_addr: %q", cfg.Server.ListenAddr)
	}
	if cfg.ControlPlane.BaseURL != "https://10.10.20.30:8443" {
		t.Errorf("control_plane.base_url: %q", cfg.ControlPlane.BaseURL)
	}
	if cfg.PEP.ID != "ztna-gw-1" {
		t.Errorf("pep.id: %q", cfg.PEP.ID)
	}
	if cfg.HeartbeatEvery != 45*time.Second {
		t.Errorf("HeartbeatEvery: %v", cfg.HeartbeatEvery)
	}
	if cfg.DecisionCacheTTL != 90*time.Second {
		t.Errorf("DecisionCacheTTL: %v", cfg.DecisionCacheTTL)
	}
	if cfg.DecisionCacheMaxKeys != 1234 {
		t.Errorf("DecisionCacheMaxKeys: %d", cfg.DecisionCacheMaxKeys)
	}
	if cfg.CPDownMode != "cache_allow" {
		t.Errorf("CPDownMode: %q", cfg.CPDownMode)
	}
	if len(cfg.Routes) != 1 {
		t.Errorf("Routes count: %d", len(cfg.Routes))
	}
	if cfg.Routes[0].Target != "10.10.30.10:22" {
		t.Errorf("Routes[0].Target: %q", cfg.Routes[0].Target)
	}
}

func TestLoad_Defaults(t *testing.T) {
	yaml := `
server: {}
control_plane:
  base_url: "https://localhost:8443"
pep:
  id: "gw-1"
  token: "valid-token-123456"
proxy: {}
logging: {}
`

	path := writeTempConfig(t, yaml)
	defer os.Remove(path)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() a échoué: %v", err)
	}

	if cfg.Server.ListenAddr != "0.0.0.0:9443" {
		t.Errorf("server.listen_addr par défaut: %q", cfg.Server.ListenAddr)
	}
	if cfg.GatewayID != "gw-1" {
		t.Errorf("gateway_id par défaut depuis pep.id: %q", cfg.GatewayID)
	}
	if cfg.Proxy.DialTimeout != "10s" {
		t.Errorf("proxy.dial_timeout par défaut: %q", cfg.Proxy.DialTimeout)
	}
	if cfg.Proxy.MaxConns != 1000 {
		t.Errorf("proxy.max_conns par défaut: %d", cfg.Proxy.MaxConns)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("logging.level par défaut: %q", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("logging.format par défaut: %q", cfg.Logging.Format)
	}
	if cfg.ControlPlane.AuthMode != "token" {
		t.Errorf("control_plane.auth_mode par défaut: %q", cfg.ControlPlane.AuthMode)
	}
	if cfg.DecisionCacheMaxKeys != 5000 {
		t.Errorf("decision_cache_max_entries par défaut: %d", cfg.DecisionCacheMaxKeys)
	}
}

func TestValidate_MissingKeyWithCert(t *testing.T) {
	yaml := `
server:
  tls:
    cert_file: "./certs/gateway.crt"
control_plane:
  base_url: "https://localhost:8443"
pep:
  id: "gw-1"
  token: "valid-token-123456"
`

	path := writeTempConfig(t, yaml)
	defer os.Remove(path)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() aurait dû échouer avec key_file manquant")
	}
}

func TestValidate_ShortToken(t *testing.T) {
	yaml := `
server:
  listen_addr: "0.0.0.0:9443"
control_plane:
  base_url: "https://10.10.20.30:8443"
pep:
  id: "ztna-gw-1"
  token: "short"
`

	path := writeTempConfig(t, yaml)
	defer os.Remove(path)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() aurait dû échouer avec un token trop court")
	}
}

func TestValidate_MTLSMissingClientCerts(t *testing.T) {
	yaml := `
server:
  listen_addr: "0.0.0.0:9443"
control_plane:
  base_url: "https://10.10.20.30:8443"
  auth_mode: "mtls"
  ca_file: "./certs/cp-ca.crt"
pep:
  id: "ztna-gw-1"
`

	path := writeTempConfig(t, yaml)
	defer os.Remove(path)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() aurait dû échouer sans client_cert_file/client_key_file en mode mtls")
	}
}

func TestValidate_InvalidAuthMode(t *testing.T) {
	yaml := `
server:
  listen_addr: "0.0.0.0:9443"
control_plane:
  base_url: "https://10.10.20.30:8443"
  auth_mode: "apikey"
pep:
  id: "ztna-gw-1"
  token: "valid-token-12345678"
`

	path := writeTempConfig(t, yaml)
	defer os.Remove(path)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() aurait dû échouer avec un auth_mode invalide")
	}
}

func TestValidate_InvalidDialTimeout(t *testing.T) {
	yaml := `
server:
  listen_addr: "0.0.0.0:9443"
control_plane:
  base_url: "https://10.10.20.30:8443"
pep:
  id: "ztna-gw-1"
  token: "valid-token-12345678"
proxy:
  dial_timeout: "invalid-duration"
`

	path := writeTempConfig(t, yaml)
	defer os.Remove(path)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() aurait dû échouer avec un dial_timeout invalide")
	}
}

func TestDialTimeoutDuration(t *testing.T) {
	cfg := &Config{
		Proxy: ProxyConfig{DialTimeout: "15s"},
	}

	duration := cfg.DialTimeoutDuration()
	if duration != 15*time.Second {
		t.Errorf("attendu 15s, obtenu %v", duration)
	}
}

func TestDialTimeoutDuration_FallbackOnInvalid(t *testing.T) {
	cfg := &Config{
		Proxy: ProxyConfig{DialTimeout: "nope"},
	}

	duration := cfg.DialTimeoutDuration()
	if duration != 10*time.Second {
		t.Errorf("attendu fallback 10s, obtenu %v", duration)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() doit retourner une erreur pour un fichier inexistant")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTempConfig(t, "{{{{ not yaml ::::")
	defer os.Remove(path)

	_, err := Load(path)
	if err == nil {
		t.Error("Load() doit retourner une erreur pour du YAML invalide")
	}
}
