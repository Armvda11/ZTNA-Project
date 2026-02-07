package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	API      APIConfig      `yaml:"api"`
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
	Issuer      string          `yaml:"issuer"`
	Audience    string          `yaml:"audience"`
	TokenExpiry string          `yaml:"token_expiry"`
	RefreshTTL  string          `yaml:"refresh_token_expiry"`
	RateLimit   RateLimitConfig `yaml:"rate_limit"`
}

// APIConfig holds API security configuration
type APIConfig struct {
	RateLimit RateLimitConfig `yaml:"rate_limit"`
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

// RefreshTokenExpiryDuration returns the refresh token expiry as time.Duration
func (a *AuthConfig) RefreshTokenExpiryDuration() (time.Duration, error) {
	return time.ParseDuration(a.RefreshTTL)
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
	if err := applyEnvOverrides(&cfg); err != nil {
		return nil, fmt.Errorf("failed to apply environment overrides: %w", err)
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

	if c.Auth.Issuer == "" {
		return fmt.Errorf("auth.issuer is required")
	}

	if c.Auth.Audience == "" {
		return fmt.Errorf("auth.audience is required")
	}

	if _, err := c.Auth.TokenExpiryDuration(); err != nil {
		return fmt.Errorf("invalid token_expiry: %w", err)
	}

	if _, err := c.Auth.RefreshTokenExpiryDuration(); err != nil {
		return fmt.Errorf("invalid refresh_token_expiry: %w", err)
	}

	if c.Auth.RateLimit.Enabled {
		if c.Auth.RateLimit.RequestsPerMinute <= 0 {
			return fmt.Errorf("auth.rate_limit.requests_per_minute must be > 0")
		}
		if c.Auth.RateLimit.Burst <= 0 {
			return fmt.Errorf("auth.rate_limit.burst must be > 0")
		}
	}

	if c.API.RateLimit.Enabled {
		if c.API.RateLimit.RequestsPerMinute <= 0 {
			return fmt.Errorf("api.rate_limit.requests_per_minute must be > 0")
		}
		if c.API.RateLimit.Burst <= 0 {
			return fmt.Errorf("api.rate_limit.burst must be > 0")
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
	} else {
		return fmt.Errorf("tls must be enabled for control plane")
	}

	if c.Database.Type != "sqlite" && c.Database.Type != "postgres" {
		return fmt.Errorf("unsupported database type: %s", c.Database.Type)
	}

	return nil
}

func applyEnvOverrides(cfg *Config) error {
	overrideString := func(env string, target *string) {
		if value := os.Getenv(env); value != "" {
			*target = value
		}
	}

	overrideInt := func(env string, target *int) error {
		value := os.Getenv(env)
		if value == "" {
			return nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer: %w", env, err)
		}
		*target = parsed
		return nil
	}

	overrideBool := func(env string, target *bool) error {
		value := os.Getenv(env)
		if value == "" {
			return nil
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean: %w", env, err)
		}
		*target = parsed
		return nil
	}

	// Auth and security-sensitive values
	overrideString("ZTNA_CP_JWT_SECRET", &cfg.Auth.JWTSecret)
	overrideString("ZTNA_CP_JWT_ISSUER", &cfg.Auth.Issuer)
	overrideString("ZTNA_CP_JWT_AUDIENCE", &cfg.Auth.Audience)
	overrideString("ZTNA_CP_TOKEN_EXPIRY", &cfg.Auth.TokenExpiry)
	overrideString("ZTNA_CP_REFRESH_TOKEN_EXPIRY", &cfg.Auth.RefreshTTL)
	if err := overrideBool("ZTNA_CP_RATE_LIMIT_ENABLED", &cfg.Auth.RateLimit.Enabled); err != nil {
		return err
	}
	if err := overrideInt("ZTNA_CP_RATE_LIMIT_RPM", &cfg.Auth.RateLimit.RequestsPerMinute); err != nil {
		return err
	}
	if err := overrideInt("ZTNA_CP_RATE_LIMIT_BURST", &cfg.Auth.RateLimit.Burst); err != nil {
		return err
	}
	if err := overrideBool("ZTNA_CP_API_RATE_LIMIT_ENABLED", &cfg.API.RateLimit.Enabled); err != nil {
		return err
	}
	if err := overrideInt("ZTNA_CP_API_RATE_LIMIT_RPM", &cfg.API.RateLimit.RequestsPerMinute); err != nil {
		return err
	}
	if err := overrideInt("ZTNA_CP_API_RATE_LIMIT_BURST", &cfg.API.RateLimit.Burst); err != nil {
		return err
	}

	// Server/TLS
	overrideString("ZTNA_CP_SERVER_HOST", &cfg.Server.Host)
	if err := overrideInt("ZTNA_CP_SERVER_PORT", &cfg.Server.Port); err != nil {
		return err
	}
	if err := overrideBool("ZTNA_CP_TLS_ENABLED", &cfg.Server.TLS.Enabled); err != nil {
		return err
	}
	overrideString("ZTNA_CP_TLS_CERT", &cfg.Server.TLS.Cert)
	overrideString("ZTNA_CP_TLS_KEY", &cfg.Server.TLS.Key)

	// SSH CA
	overrideString("ZTNA_CP_CA_KEY_PATH", &cfg.SSH.CAKeyPath)
	overrideString("ZTNA_CP_CERT_VALIDITY", &cfg.SSH.CertValidity)

	// Database
	overrideString("ZTNA_CP_DB_TYPE", &cfg.Database.Type)
	overrideString("ZTNA_CP_DB_PATH", &cfg.Database.Path)
	overrideString("ZTNA_CP_DB_DSN", &cfg.Database.DSN)

	// Logging
	overrideString("ZTNA_CP_LOG_LEVEL", &cfg.Logging.Level)
	overrideString("ZTNA_CP_LOG_FORMAT", &cfg.Logging.Format)
	overrideString("ZTNA_CP_LOG_OUTPUT", &cfg.Logging.Output)

	return nil
}
