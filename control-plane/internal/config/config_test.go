package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	content := `
server:
  host: "0.0.0.0"
  port: 9000
auth:
  jwt_secret: "test-secret"
  token_expiry: "10m"
ssh:
  ca_key_path: "/tmp/test_ca"
  cert_validity: "5m"
  cert_principals: ["test"]
policies:
  default_deny: true
  rules: []
logging:
  level: "debug"
  format: "json"
  output: "stdout"
database:
  type: "sqlite"
  path: ":memory:"
`
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
				Server: ServerConfig{Port: 8443},
				Auth: AuthConfig{
					JWTSecret:   "secret",
					TokenExpiry: "15m",
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
				Server: ServerConfig{Port: 99999},
				Auth: AuthConfig{
					JWTSecret:   "secret",
					TokenExpiry: "15m",
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
				Server: ServerConfig{Port: 8443},
				Auth: AuthConfig{
					JWTSecret:   "",
					TokenExpiry: "15m",
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
	content := `
server:
  port: 8443
auth:
  jwt_secret: "file-secret"
  token_expiry: "15m"
ssh:
  ca_key_path: "/tmp/ca"
  cert_validity: "15m"
database:
  type: "sqlite"
  path: ":memory:"
`
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
