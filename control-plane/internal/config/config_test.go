package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	content := "server:\n" +
		"  address: \"0.0.0.0\"\n" +
		"  port: 9000\n" +
		"  tls:\n" +
		"    enabled: true\n" +
		"    cert_file: \"/tmp/test.crt\"\n" +
		"    key_file: \"/tmp/test.key\"\n" +
		"database:\n" +
		"  path: \":memory:\"\n" +
		"oidc:\n" +
		"  issuer: \"http://issuer.example/realms/ztna\"\n" +
		"  allow_http_issuer: true\n" +
		"  audience: \"ztna-control-plane\"\n" +
		"  username_claim: \"preferred_username\"\n" +
		"  groups_claim: \"groups\"\n" +
		"  allowed_algs: [\"RS256\"]\n" +
		"  jwks_cache_ttl: \"10m\"\n" +
		"  audience_mode: \"aud_or_azp\"\n" +
		"  admin_group: \"ztna-admins\"\n" +
		"pep:\n" +
		"  auth_mode: \"token\"\n" +
		"  tokens:\n" +
		"    ztna-gw-1: \"secret\"\n" +
		"sshca:\n" +
		"  key_path: \"/tmp/ssh_ca\"\n" +
		"  default_ttl: \"15m\"\n" +
		"policy:\n" +
		"  seed_file: \"./policies.yaml\"\n"

	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Server.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.Server.Port)
	}
	if cfg.OIDC.Issuer != "http://issuer.example/realms/ztna" {
		t.Errorf("Expected issuer to load, got %s", cfg.OIDC.Issuer)
	}
	if cfg.PEP.AuthMode != "token" {
		t.Errorf("Expected pep.auth_mode token, got %s", cfg.PEP.AuthMode)
	}
}

func TestValidate(t *testing.T) {
	config := Config{
		Server: ServerConfig{
			Port: 8443,
			TLS: TLSConfig{
				Enabled:  true,
				CertFile: "/tmp/test.crt",
				KeyFile:  "/tmp/test.key",
			},
		},
		Database: DatabaseConfig{Path: ":memory:"},
		OIDC: OIDCConfig{
			Issuer:          "http://issuer.example/realms/ztna",
			AllowHTTPIssuer: true,
			Audience:        "ztna-control-plane",
			UsernameClaim:   "preferred_username",
			GroupsClaim:     "groups",
			AllowedAlgs:     []string{"RS256"},
			JWKSCacheTTL:    "10m",
			AudienceMode:    "aud_or_azp",
			AdminGroup:      "ztna-admins",
		},
		PEP: PEPConfig{
			AuthMode: "token",
			Tokens: map[string]string{
				"ztna-gw-1": "secret",
			},
		},
		SSHCA: SSHCAConfig{
			KeyPath:    "/tmp/ssh_ca",
			DefaultTTL: "15m",
		},
		DeviceCA: DeviceCAConfig{
			KeyPath:    "/tmp/device_ca.key",
			CertPath:   "/tmp/device_ca.crt",
			DefaultTTL: "168h",
		},
		Policy: PolicyConfig{SeedFile: "./policies.yaml"},
	}

	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}
