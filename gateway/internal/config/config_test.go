package config

import (
	"os"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	tmpfile, err := os.CreateTemp("", "gateway-config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		_ = tmpfile.Close()
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = os.Remove(tmpfile.Name())
	})

	return tmpfile.Name()
}

func TestLoad(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "0.0.0.0"
  ssh_port: 2222
  admin_port: 9090
controlplane:
  url: "https://10.10.20.30:8443"
  ca_public_key_endpoint: "/api/v1/ca/public-key"
  policy_check_endpoint: "/api/v1/policies"
  tls_skip_verify: true
ssh:
  host_key_path: "/etc/ztna/gateway_host_key"
  trusted_ca_keys_path: "/etc/ztna/trusted_user_ca_keys"
routing:
  targets:
    - name: "lan-app"
      host: "10.10.30.10"
      port: 22
      user: "ztna"
logging:
  level: "info"
  format: "json"
  output: "stdout"
session:
  timeout: "15m"
  max_concurrent: 100
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.SSHPort != 2222 {
		t.Fatalf("expected ssh_port=2222, got %d", cfg.Server.SSHPort)
	}
	if cfg.ControlPlane.URL != "https://10.10.20.30:8443" {
		t.Fatalf("unexpected controlplane url: %s", cfg.ControlPlane.URL)
	}
	if _, err := cfg.GetTarget("lan-app"); err != nil {
		t.Fatalf("expected target lan-app: %v", err)
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	path := writeTempConfig(t, `
server:
  host: "0.0.0.0"
  ssh_port: 2222
  admin_port: 9090
controlplane:
  url: "https://10.10.20.30:8443"
  ca_public_key_endpoint: "/api/v1/ca/public-key"
  policy_check_endpoint: "/api/v1/policies"
  tls_skip_verify: false
ssh:
  host_key_path: "/etc/ztna/gateway_host_key"
  trusted_ca_keys_path: "/etc/ztna/trusted_user_ca_keys"
routing:
  targets:
    - name: "lan-app"
      host: "10.10.30.10"
      port: 22
      user: "ztna"
logging:
  level: "info"
  format: "json"
  output: "stdout"
session:
  timeout: "15m"
  max_concurrent: 100
`)

	t.Setenv("ZTNA_GW_CP_URL", "https://cp.example.local:9443")
	t.Setenv("ZTNA_GW_CP_TLS_SKIP_VERIFY", "true")
	t.Setenv("ZTNA_GW_SSH_PORT", "2022")
	t.Setenv("ZTNA_GW_HOST_KEY_PATH", "/run/secrets/host_key")
	t.Setenv("ZTNA_GW_SESSION_MAX_CONCURRENT", "250")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed with environment overrides: %v", err)
	}

	if cfg.ControlPlane.URL != "https://cp.example.local:9443" {
		t.Fatalf("expected env override for controlplane.url, got %s", cfg.ControlPlane.URL)
	}
	if !cfg.ControlPlane.TLSSkipVerify {
		t.Fatalf("expected env override for tls_skip_verify=true")
	}
	if cfg.Server.SSHPort != 2022 {
		t.Fatalf("expected env override for ssh_port=2022, got %d", cfg.Server.SSHPort)
	}
	if cfg.SSH.HostKeyPath != "/run/secrets/host_key" {
		t.Fatalf("expected env override for host_key_path, got %s", cfg.SSH.HostKeyPath)
	}
	if cfg.Session.MaxConcurrent != 250 {
		t.Fatalf("expected env override for max_concurrent=250, got %d", cfg.Session.MaxConcurrent)
	}
}
