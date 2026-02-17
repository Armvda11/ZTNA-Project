package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig   `yaml:"server"`
	PEPServer ServerConfig   `yaml:"pep_server"`
	Database  DatabaseConfig `yaml:"database"`
	OIDC      OIDCConfig     `yaml:"oidc"`
	PEP       PEPConfig      `yaml:"pep"`
	SSHCA     SSHCAConfig    `yaml:"sshca"`
	Policy    PolicyConfig   `yaml:"policy"`
	Logging   LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Address           string    `yaml:"address"`
	Port              int       `yaml:"port"`
	ReadTimeout       string    `yaml:"read_timeout"`
	WriteTimeout      string    `yaml:"write_timeout"`
	IdleTimeout       string    `yaml:"idle_timeout"`
	ReadHeaderTimeout string    `yaml:"read_header_timeout"`
	TLS               TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Enabled           bool   `yaml:"enabled"`
	CertFile          string `yaml:"cert_file"`
	KeyFile           string `yaml:"key_file"`
	ClientCAFile      string `yaml:"client_ca_file"`
	RequireClientAuth bool   `yaml:"require_client_auth"`
}

type DatabaseConfig struct {
	Path        string   `yaml:"path"`
	BusyTimeout string   `yaml:"busy_timeout"`
	Pragmas     []string `yaml:"pragmas"`
}

type OIDCConfig struct {
	Issuer          string   `yaml:"issuer"`
	Audience        string   `yaml:"audience"`
	UsernameClaim   string   `yaml:"username_claim"`
	GroupsClaim     string   `yaml:"groups_claim"`
	AllowedAlgs     []string `yaml:"allowed_algs"`
	JWKSCacheTTL    string   `yaml:"jwks_cache_ttl"`
	AudienceMode    string   `yaml:"audience_mode"`
	AdminGroup      string   `yaml:"admin_group"`
	AllowHTTPIssuer bool     `yaml:"allow_http_issuer"` // For lab: accept http:// issuer (not https://)
}

type PEPConfig struct {
	AuthMode string            `yaml:"auth_mode"`
	Tokens   map[string]string `yaml:"tokens"`
}

type SSHCAConfig struct {
	KeyPath           string   `yaml:"key_path"`
	DefaultTTL        string   `yaml:"default_ttl"`
	MinTTL            string   `yaml:"min_ttl"`
	MaxTTL            string   `yaml:"max_ttl"`
	AllowedPrincipals []string `yaml:"allowed_principals"`
}

type PolicyConfig struct {
	SeedFile string `yaml:"seed_file"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be 1-65535")
	}
	if c.OIDC.Issuer == "" {
		return fmt.Errorf("oidc.issuer is required")
	}
	if c.OIDC.Audience == "" {
		return fmt.Errorf("oidc.audience is required")
	}
	if c.OIDC.UsernameClaim == "" {
		return fmt.Errorf("oidc.username_claim is required")
	}
	if len(c.OIDC.AllowedAlgs) == 0 {
		return fmt.Errorf("oidc.allowed_algs is required")
	}
	for _, alg := range c.OIDC.AllowedAlgs {
		if strings.ToUpper(strings.TrimSpace(alg)) != "RS256" {
			return fmt.Errorf("oidc.allowed_algs must contain only RS256")
		}
	}
	if c.OIDC.JWKSCacheTTL != "" {
		if _, err := time.ParseDuration(c.OIDC.JWKSCacheTTL); err != nil {
			return fmt.Errorf("oidc.jwks_cache_ttl invalid: %w", err)
		}
	}
	if c.OIDC.AudienceMode != "" && c.OIDC.AudienceMode != "aud" && c.OIDC.AudienceMode != "aud_or_azp" {
		return fmt.Errorf("oidc.audience_mode must be aud or aud_or_azp")
	}
	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	if c.SSHCA.KeyPath == "" {
		return fmt.Errorf("sshca.key_path is required")
	}
	if _, err := time.ParseDuration(c.SSHCA.DefaultTTL); err != nil {
		return fmt.Errorf("sshca.default_ttl invalid: %w", err)
	}
	if c.SSHCA.MinTTL != "" {
		if _, err := time.ParseDuration(c.SSHCA.MinTTL); err != nil {
			return fmt.Errorf("sshca.min_ttl invalid: %w", err)
		}
	}
	if c.SSHCA.MaxTTL != "" {
		if _, err := time.ParseDuration(c.SSHCA.MaxTTL); err != nil {
			return fmt.Errorf("sshca.max_ttl invalid: %w", err)
		}
	}
	if c.SSHCA.MinTTL != "" && c.SSHCA.MaxTTL != "" {
		minTTL, _ := time.ParseDuration(c.SSHCA.MinTTL)
		maxTTL, _ := time.ParseDuration(c.SSHCA.MaxTTL)
		if maxTTL > 0 && minTTL > maxTTL {
			return fmt.Errorf("sshca.min_ttl must be <= max_ttl")
		}
	}
	if c.PEP.AuthMode != "token" && c.PEP.AuthMode != "mtls" {
		return fmt.Errorf("pep.auth_mode must be token or mtls")
	}
	if c.PEP.AuthMode == "token" && len(c.PEP.Tokens) == 0 {
		return fmt.Errorf("pep.tokens required for token auth")
	}
	if c.Server.TLS.Enabled {
		if c.Server.TLS.CertFile == "" || c.Server.TLS.KeyFile == "" {
			return fmt.Errorf("server.tls cert_file and key_file are required")
		}
	}
	if c.PEP.AuthMode == "mtls" {
		if c.PEPServer.Port < 1 || c.PEPServer.Port > 65535 {
			return fmt.Errorf("pep_server.port must be 1-65535")
		}
		if !c.PEPServer.TLS.Enabled {
			return fmt.Errorf("pep_server.tls.enabled must be true when pep.auth_mode=mtls")
		}
		if c.PEPServer.TLS.CertFile == "" || c.PEPServer.TLS.KeyFile == "" {
			return fmt.Errorf("pep_server.tls cert_file and key_file are required when pep.auth_mode=mtls")
		}
		if c.PEPServer.TLS.ClientCAFile == "" {
			return fmt.Errorf("pep_server.tls.client_ca_file is required when pep.auth_mode=mtls")
		}
	}
	return nil
}

func (c *Config) BusyTimeout() time.Duration {
	return parseDurationOrDefault(c.Database.BusyTimeout, 5*time.Second)
}

func applyDefaults(cfg *Config) {
	applyServerDefaults(&cfg.Server, 8080)
	if cfg.Database.BusyTimeout == "" {
		cfg.Database.BusyTimeout = "5s"
	}
	if cfg.OIDC.GroupsClaim == "" {
		cfg.OIDC.GroupsClaim = "groups"
	}
	if len(cfg.OIDC.AllowedAlgs) == 0 {
		cfg.OIDC.AllowedAlgs = []string{"RS256"}
	}
	if cfg.OIDC.JWKSCacheTTL == "" {
		cfg.OIDC.JWKSCacheTTL = "10m"
	}
	if cfg.OIDC.AudienceMode == "" {
		cfg.OIDC.AudienceMode = "aud"
	}
	if cfg.PEP.AuthMode == "" {
		cfg.PEP.AuthMode = "token"
	}
	if cfg.PEP.AuthMode == "mtls" {
		applyServerDefaults(&cfg.PEPServer, 8443)
		cfg.PEPServer.TLS.Enabled = true
		cfg.PEPServer.TLS.RequireClientAuth = true
		if cfg.PEPServer.TLS.CertFile == "" {
			cfg.PEPServer.TLS.CertFile = cfg.Server.TLS.CertFile
		}
		if cfg.PEPServer.TLS.KeyFile == "" {
			cfg.PEPServer.TLS.KeyFile = cfg.Server.TLS.KeyFile
		}
		if cfg.PEPServer.TLS.ClientCAFile == "" {
			cfg.PEPServer.TLS.ClientCAFile = cfg.Server.TLS.ClientCAFile
		}
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
}

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func applyServerDefaults(server *ServerConfig, defaultPort int) {
	if server.Address == "" {
		server.Address = "0.0.0.0"
	}
	if server.Port == 0 {
		server.Port = defaultPort
	}
	if server.ReadTimeout == "" {
		server.ReadTimeout = "10s"
	}
	if server.WriteTimeout == "" {
		server.WriteTimeout = "10s"
	}
	if server.IdleTimeout == "" {
		server.IdleTimeout = "60s"
	}
	if server.ReadHeaderTimeout == "" {
		server.ReadHeaderTimeout = "5s"
	}
}

func (s ServerConfig) ReadTimeoutDuration() time.Duration {
	return parseDurationOrDefault(s.ReadTimeout, 10*time.Second)
}

func (s ServerConfig) WriteTimeoutDuration() time.Duration {
	return parseDurationOrDefault(s.WriteTimeout, 10*time.Second)
}

func (s ServerConfig) IdleTimeoutDuration() time.Duration {
	return parseDurationOrDefault(s.IdleTimeout, 60*time.Second)
}

func (s ServerConfig) ReadHeaderTimeoutDuration() time.Duration {
	return parseDurationOrDefault(s.ReadHeaderTimeout, 5*time.Second)
}
