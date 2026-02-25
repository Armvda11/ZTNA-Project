package config

import (
	"os"
	"testing"
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
