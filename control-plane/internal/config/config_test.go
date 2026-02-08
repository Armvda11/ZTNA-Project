package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	content := "server:\n" +
		"  host: \"0.0.0.0\"\n" +
		"  port: 9000\n" +
		"  tls:\n" +
		"    enabled: true\n" +
		"    cert: \"/tmp/test.crt\"\n" +
		"    key: \"/tmp/test.key\"\n" +
		"auth:\n" +
		"  jwt_secret: \"test-secret\"\n" +
		"  issuer: \"ztna-cp\"\n" +
		"  audience: \"ztna-clients\"\n" +
		"  token_expiry: \"10m\"\n" +
		"  refresh_token_expiry: \"24h\"\n" +
		"ssh:\n" +
		"  ca_key_path: \"/tmp/test_ca\"\n" +
		"  cert_validity: \"5m\"\n" +
		"  cert_principals: [\"test\"]\n" +
		"policies:\n" +
		"  default_deny: true\n" +
		"  rules: []\n" +
		"logging:\n" +
		"  level: \"debug\"\n" +
		"  format: \"json\"\n" +
		"  output: \"stdout\"\n" +
		"database:\n" +
		"  type: \"sqlite\"\n" +
		"  path: \":memory:\"\n\n" +
		"api:\n" +
		"  rate_limit:\n" +
		"    enabled: false\n" +
		"    requests_per_minute: 60\n" +
		"    burst: 30\n"
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// Load config
	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate fields
	if cfg.Server.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", cfg.Server.Port)
	}

	if cfg.Auth.JWTSecret != "test-secret" {
		t.Errorf("Expected jwt_secret 'test-secret', got '%s'", cfg.Auth.JWTSecret)
	}

	// Test duration parsing
	duration, err := cfg.Auth.TokenExpiryDuration()
	if err != nil {
		t.Errorf("Failed to parse token_expiry: %v", err)
	}
	if duration != 10*time.Minute {
		t.Errorf("Expected 10m duration, got %v", duration)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: Config{
				Server: ServerConfig{
					Port: 8443,
					TLS: TLSConfig{
						Enabled: true,
						Cert:    "/tmp/test.crt",
						Key:     "/tmp/test.key",
					},
				},
				Auth: AuthConfig{
					JWTSecret:   "secret",
					Issuer:      "ztna-cp",
					Audience:    "ztna-clients",
					TokenExpiry: "15m",
					RefreshTTL:  "24h",
				},
				API: APIConfig{
					RateLimit: RateLimitConfig{Enabled: false},
				},
				SSH: SSHConfig{
					CAKeyPath:    "/tmp/ca",
					CertValidity: "15m",
				},
				Database: DatabaseConfig{Type: "sqlite"},
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			config: Config{
				Server: ServerConfig{
					Port: 99999,
					TLS: TLSConfig{
						Enabled: true,
						Cert:    "/tmp/test.crt",
						Key:     "/tmp/test.key",
					},
				},
				Auth: AuthConfig{
					JWTSecret:   "secret",
					Issuer:      "ztna-cp",
					Audience:    "ztna-clients",
					TokenExpiry: "15m",
					RefreshTTL:  "24h",
				},
				API: APIConfig{
					RateLimit: RateLimitConfig{Enabled: false},
				},
				SSH: SSHConfig{
					CAKeyPath:    "/tmp/ca",
					CertValidity: "15m",
				},
				Database: DatabaseConfig{Type: "sqlite"},
			},
			wantErr: true,
		},
		{
			name: "missing jwt secret",
			config: Config{
				Server: ServerConfig{
					Port: 8443,
					TLS: TLSConfig{
						Enabled: true,
						Cert:    "/tmp/test.crt",
						Key:     "/tmp/test.key",
					},
				},
				Auth: AuthConfig{
					JWTSecret:   "",
					Issuer:      "ztna-cp",
					Audience:    "ztna-clients",
					TokenExpiry: "15m",
					RefreshTTL:  "24h",
				},
				API: APIConfig{
					RateLimit: RateLimitConfig{Enabled: false},
				},
				SSH: SSHConfig{
					CAKeyPath:    "/tmp/ca",
					CertValidity: "15m",
				},
				Database: DatabaseConfig{Type: "sqlite"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEnvironmentOverride(t *testing.T) {
	// Create config file
	content := "server:\n" +
		"  port: 8443\n" +
		"  tls:\n" +
		"    enabled: true\n" +
		"    cert: \"/tmp/test.crt\"\n" +
		"    key: \"/tmp/test.key\"\n" +
		"auth:\n" +
		"  jwt_secret: \"file-secret\"\n" +
		"  issuer: \"ztna-cp\"\n" +
		"  audience: \"ztna-clients\"\n" +
		"  token_expiry: \"15m\"\n" +
		"  refresh_token_expiry: \"24h\"\n" +
		"ssh:\n" +
		"  ca_key_path: \"/tmp/ca\"\n" +
		"  cert_validity: \"15m\"\n" +
		"database:\n" +
		"  type: \"sqlite\"\n" +
		"  path: \":memory:\"\n\n" +
		"api:\n" +
		"  rate_limit:\n" +
		"    enabled: false\n" +
		"    requests_per_minute: 60\n" +
		"    burst: 30\n"
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// Set environment variable
	os.Setenv("ZTNA_JWT_SECRET", "env-secret")
	defer os.Unsetenv("ZTNA_JWT_SECRET")

	// Load config
	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify environment override
	if cfg.Auth.JWTSecret != "env-secret" {
		t.Errorf("Expected jwt_secret 'env-secret' from env, got '%s'", cfg.Auth.JWTSecret)
	}
}

func TestEnvironmentOverridesExtended(t *testing.T) {
	content := "server:\n" +
		"  host: \"0.0.0.0\"\n" +
		"  port: 8443\n" +
		"  tls:\n" +
		"    enabled: true\n" +
		"    cert: \"/tmp/default.crt\"\n" +
		"    key: \"/tmp/default.key\"\n" +
		"auth:\n" +
		"  jwt_secret: \"file-secret\"\n" +
		"  issuer: \"ztna-cp\"\n" +
		"  audience: \"ztna-clients\"\n" +
		"  token_expiry: \"15m\"\n" +
		"  refresh_token_expiry: \"24h\"\n" +
		"  rate_limit:\n" +
		"    enabled: false\n" +
		"    requests_per_minute: 5\n" +
		"    burst: 10\n" +
		"ssh:\n" +
		"  ca_key_path: \"/tmp/ca\"\n" +
		"  cert_validity: \"15m\"\n" +
		"database:\n" +
		"  type: \"sqlite\"\n" +
		"  path: \":memory:\"\n\n" +
		"api:\n" +
		"  rate_limit:\n" +
		"    enabled: false\n" +
		"    requests_per_minute: 60\n" +
		"    burst: 30\n"
	tmpfile, err := os.CreateTemp("", "config-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	os.Setenv("ZTNA_CP_SERVER_PORT", "9443")
	os.Setenv("ZTNA_CP_TLS_ENABLED", "true")
	os.Setenv("ZTNA_CP_TLS_CERT", "/etc/ztna/tls/server.crt")
	os.Setenv("ZTNA_CP_TLS_KEY", "/etc/ztna/tls/server.key")
	os.Setenv("ZTNA_CP_RATE_LIMIT_ENABLED", "true")
	os.Setenv("ZTNA_CP_RATE_LIMIT_RPM", "60")
	os.Setenv("ZTNA_CP_RATE_LIMIT_BURST", "30")
	os.Setenv("ZTNA_CP_DB_PATH", "/var/lib/ztna/test.db")
	os.Setenv("ZTNA_CP_CORS_ALLOWED_ORIGINS", "https://admin.ztna.local, https://ops.ztna.local")
	os.Setenv("ZTNA_CP_CERT_PRINCIPALS", "p1,p2")
	os.Setenv("ZTNA_CP_POLICIES_DEFAULT_DENY", "false")
	os.Setenv("ZTNA_CP_POLICIES_RULES_JSON", `[{"user":"alice","resources":["lan-app"],"allowed":true}]`)
	defer os.Unsetenv("ZTNA_CP_SERVER_PORT")
	defer os.Unsetenv("ZTNA_CP_TLS_ENABLED")
	defer os.Unsetenv("ZTNA_CP_TLS_CERT")
	defer os.Unsetenv("ZTNA_CP_TLS_KEY")
	defer os.Unsetenv("ZTNA_CP_RATE_LIMIT_ENABLED")
	defer os.Unsetenv("ZTNA_CP_RATE_LIMIT_RPM")
	defer os.Unsetenv("ZTNA_CP_RATE_LIMIT_BURST")
	defer os.Unsetenv("ZTNA_CP_DB_PATH")
	defer os.Unsetenv("ZTNA_CP_CORS_ALLOWED_ORIGINS")
	defer os.Unsetenv("ZTNA_CP_CERT_PRINCIPALS")
	defer os.Unsetenv("ZTNA_CP_POLICIES_DEFAULT_DENY")
	defer os.Unsetenv("ZTNA_CP_POLICIES_RULES_JSON")

	cfg, err := Load(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config with environment overrides: %v", err)
	}

	if cfg.Server.Port != 9443 {
		t.Errorf("Expected server.port 9443, got %d", cfg.Server.Port)
	}
	if !cfg.Server.TLS.Enabled {
		t.Errorf("Expected TLS enabled via env override")
	}
	if cfg.Server.TLS.Cert != "/etc/ztna/tls/server.crt" {
		t.Errorf("Expected tls.cert override, got %s", cfg.Server.TLS.Cert)
	}
	if cfg.Server.TLS.Key != "/etc/ztna/tls/server.key" {
		t.Errorf("Expected tls.key override, got %s", cfg.Server.TLS.Key)
	}
	if !cfg.Auth.RateLimit.Enabled {
		t.Errorf("Expected rate limiting enabled via env override")
	}
	if cfg.Auth.RateLimit.RequestsPerMinute != 60 {
		t.Errorf("Expected rate_limit.requests_per_minute 60, got %d", cfg.Auth.RateLimit.RequestsPerMinute)
	}
	if cfg.Auth.RateLimit.Burst != 30 {
		t.Errorf("Expected rate_limit.burst 30, got %d", cfg.Auth.RateLimit.Burst)
	}
	if cfg.Database.Path != "/var/lib/ztna/test.db" {
		t.Errorf("Expected DB path override, got %s", cfg.Database.Path)
	}
	if len(cfg.Server.CORS.AllowedOrigins) != 2 {
		t.Errorf("Expected 2 CORS origins, got %d", len(cfg.Server.CORS.AllowedOrigins))
	}
	if len(cfg.SSH.CertPrincipals) != 2 {
		t.Errorf("Expected 2 cert principals, got %d", len(cfg.SSH.CertPrincipals))
	}
	if cfg.Policies.DefaultDeny {
		t.Errorf("Expected policies.default_deny to be false")
	}
	if len(cfg.Policies.Rules) != 1 {
		t.Errorf("Expected 1 policy rule from env override, got %d", len(cfg.Policies.Rules))
	}
}
