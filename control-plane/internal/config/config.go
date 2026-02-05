package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	SSH      SSHConfig      `yaml:"ssh"`
	Policies PoliciesConfig `yaml:"policies"`
	Logging  LoggingConfig  `yaml:"logging"`
	Database DatabaseConfig `yaml:"database"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host string    `yaml:"host"`
	Port int       `yaml:"port"`
	TLS  TLSConfig `yaml:"tls"`
}

// TLSConfig holds TLS certificate configuration
type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret   string          `yaml:"jwt_secret"`
	TokenExpiry string          `yaml:"token_expiry"`
	RateLimit   RateLimitConfig `yaml:"rate_limit"`
}

// RateLimitConfig holds login rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
	Burst             int  `yaml:"burst"`
}

// TokenExpiryDuration returns the token expiry as time.Duration
func (a *AuthConfig) TokenExpiryDuration() (time.Duration, error) {
	return time.ParseDuration(a.TokenExpiry)
}

// SSHConfig holds SSH CA configuration
type SSHConfig struct {
	CAKeyPath      string   `yaml:"ca_key_path"`
	CertValidity   string   `yaml:"cert_validity"`
	CertPrincipals []string `yaml:"cert_principals"`
}

// CertValidityDuration returns the cert validity as time.Duration
func (s *SSHConfig) CertValidityDuration() (time.Duration, error) {
	return time.ParseDuration(s.CertValidity)
}

// PolicyRule represents a single policy rule
type PolicyRule struct {
	User      string   `yaml:"user"`
	Resources []string `yaml:"resources"`
	Allowed   bool     `yaml:"allowed"`
}

// PoliciesConfig holds policy engine configuration
type PoliciesConfig struct {
	DefaultDeny bool         `yaml:"default_deny"`
	Rules       []PolicyRule `yaml:"rules"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Type string `yaml:"type"`
	Path string `yaml:"path"`
	DSN  string `yaml:"dsn"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	// Try to read from specified path
	data, err := os.ReadFile(path)
	if err != nil {
		// If file doesn't exist, try local config.yaml
		if os.IsNotExist(err) {
			localPath := "config.yaml"
			data, err = os.ReadFile(localPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read config from %s or %s: %w", path, localPath, err)
			}
		} else {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Override JWT secret from environment if set
	if jwtSecret := os.Getenv("ZTNA_JWT_SECRET"); jwtSecret != "" {
		cfg.Auth.JWTSecret = jwtSecret
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Auth.JWTSecret == "" {
		return fmt.Errorf("jwt_secret is required")
	}

	if _, err := c.Auth.TokenExpiryDuration(); err != nil {
		return fmt.Errorf("invalid token_expiry: %w", err)
	}

	if c.Auth.RateLimit.Enabled {
		if c.Auth.RateLimit.RequestsPerMinute <= 0 {
			return fmt.Errorf("auth.rate_limit.requests_per_minute must be > 0")
		}
		if c.Auth.RateLimit.Burst <= 0 {
			return fmt.Errorf("auth.rate_limit.burst must be > 0")
		}
	}

	if c.SSH.CAKeyPath == "" {
		return fmt.Errorf("ssh ca_key_path is required")
	}

	if _, err := c.SSH.CertValidityDuration(); err != nil {
		return fmt.Errorf("invalid cert_validity: %w", err)
	}

	if c.Server.TLS.Enabled {
		if c.Server.TLS.Cert == "" || c.Server.TLS.Key == "" {
			return fmt.Errorf("tls enabled but cert or key is empty")
		}
	}

	if c.Database.Type != "sqlite" && c.Database.Type != "postgres" {
		return fmt.Errorf("unsupported database type: %s", c.Database.Type)
	}

	return nil
}
