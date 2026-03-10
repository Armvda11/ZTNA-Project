package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration loaded from YAML.
type Config struct {
	Server    ServerConfig   `yaml:"server"`
	PEPServer ServerConfig   `yaml:"pep_server"`
	Database  DatabaseConfig `yaml:"database"`
	OIDC      OIDCConfig     `yaml:"oidc"`
	PEP       PEPConfig      `yaml:"pep"`

	// SSH user certificate authority used for short-lived SSH user certs.
	SSHCA SSHCAConfig `yaml:"sshca"`

	// DeviceCA is the X.509 CA used to sign client certificates for mTLS user<->gateway.
	DeviceCA DeviceCAConfig `yaml:"device_ca"`

	Policy  PolicyConfig  `yaml:"policy"`
	Resource ResourceConfig `yaml:"resource"`
	Logging LoggingConfig `yaml:"logging"`
}

// -------------------- Server / TLS --------------------

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

// -------------------- Database --------------------

type DatabaseConfig struct {
	Path        string   `yaml:"path"`
	BusyTimeout string   `yaml:"busy_timeout"`
	Pragmas     []string `yaml:"pragmas"`
}

func (c *Config) BusyTimeout() time.Duration {
	return parseDurationOrDefault(c.Database.BusyTimeout, 5*time.Second)
}

// -------------------- OIDC --------------------

type OIDCConfig struct {
	Issuer             string   `yaml:"issuer"`
	Audience           string   `yaml:"audience"`
	UsernameClaim      string   `yaml:"username_claim"`
	GroupsClaim        string   `yaml:"groups_claim"`
	AllowedAlgs        []string `yaml:"allowed_algs"`
	JWKSCacheTTL       string   `yaml:"jwks_cache_ttl"`
	AudienceMode       string   `yaml:"audience_mode"`
	AdminGroup         string   `yaml:"admin_group"`
	AllowHTTPIssuer    bool     `yaml:"allow_http_issuer"`    // lab-only: allow http:// issuer
	CAFile             string   `yaml:"ca_file"`              // custom CA for HTTPS JWKS (self-signed Keycloak)
	InsecureSkipVerify bool     `yaml:"insecure_skip_verify"` // skip TLS verify for JWKS endpoint (lab-only)
}

// -------------------- PEP (gateway -> CP) auth --------------------

type PEPConfig struct {
	AuthMode            string            `yaml:"auth_mode"`            // "token" or "mtls"
	Tokens              map[string]string `yaml:"tokens"`               // pep_id -> pep_token (token mode)
	DecisionTTLSeconds  int               `yaml:"decision_ttl_seconds"` // advertised cache TTL for gateway decisions
	RequireRegistration *bool             `yaml:"require_registration"` // if true, heartbeat/authorize require prior /pep/register
	RevokedPEPIDs       []string          `yaml:"revoked_pep_ids"`      // explicit deny-list for compromised PEP identities
}

// -------------------- SSH CA (user certs) --------------------

type SSHCAConfig struct {
	KeyPath           string   `yaml:"key_path"`
	DefaultTTL        string   `yaml:"default_ttl"`
	MinTTL            string   `yaml:"min_ttl"`
	MaxTTL            string   `yaml:"max_ttl"`
	AllowedPrincipals []string `yaml:"allowed_principals"`
}

// -------------------- Device CA (X.509 client certs for mTLS user<->GW) --------------------

type DeviceCAConfig struct {
	KeyPath    string `yaml:"key_path"`
	CertPath   string `yaml:"cert_path"`
	DefaultTTL string `yaml:"default_ttl"`
	MinTTL     string `yaml:"min_ttl"`
	MaxTTL     string `yaml:"max_ttl"`

	// Restrict what key types are allowed in CSR.
	// Accepted values: "ed25519", "ecdsa-p256"
	AllowedKeyTypes []string `yaml:"allowed_key_types"`
}

func (c *Config) DeviceDefaultTTL() time.Duration {
	return parseDurationOrDefault(c.DeviceCA.DefaultTTL, 7*24*time.Hour)
}
func (c *Config) DeviceMinTTL() time.Duration {
	return parseDurationOrDefault(c.DeviceCA.MinTTL, 1*time.Hour)
}
func (c *Config) DeviceMaxTTL() time.Duration {
	return parseDurationOrDefault(c.DeviceCA.MaxTTL, 30*24*time.Hour)
}

// -------------------- Policy / Logging --------------------

type PolicyConfig struct {
	SeedFile string `yaml:"seed_file"`
}

// ResourceConfig holds resource catalog configuration.
type ResourceConfig struct {
	SeedFile string `yaml:"seed_file"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"` // "json" or "text"
}

// -------------------- Load / Validate --------------------

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
	// --- server ---
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be 1-65535")
	}
	if c.Server.TLS.Enabled {
		if c.Server.TLS.CertFile == "" || c.Server.TLS.KeyFile == "" {
			return fmt.Errorf("server.tls cert_file and key_file are required when server.tls.enabled=true")
		}
	}

	// --- oidc ---
	if c.OIDC.Issuer == "" {
		return fmt.Errorf("oidc.issuer is required")
	}
	if !c.OIDC.AllowHTTPIssuer && strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.OIDC.Issuer)), "http://") {
		return fmt.Errorf("oidc.issuer uses http:// but oidc.allow_http_issuer=false")
	}
	if c.OIDC.Audience == "" {
		return fmt.Errorf("oidc.audience is required")
	}
	if c.OIDC.UsernameClaim == "" {
		return fmt.Errorf("oidc.username_claim is required")
	}
	if c.OIDC.GroupsClaim == "" {
		return fmt.Errorf("oidc.groups_claim is required")
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
	// Validate ca_file is set when using HTTPS issuer with self-signed cert
	if c.OIDC.CAFile != "" {
		if _, err := os.Stat(c.OIDC.CAFile); err != nil {
			return fmt.Errorf("oidc.ca_file %q not found: %w", c.OIDC.CAFile, err)
		}
	}

	// --- database ---
	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}
	if c.Database.BusyTimeout != "" {
		if _, err := time.ParseDuration(c.Database.BusyTimeout); err != nil {
			return fmt.Errorf("database.busy_timeout invalid: %w", err)
		}
	}

	// --- sshca ---
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
			return fmt.Errorf("sshca.min_ttl must be <= sshca.max_ttl")
		}
	}

	// --- device_ca ---
	if c.DeviceCA.KeyPath == "" {
		return fmt.Errorf("device_ca.key_path is required")
	}
	if c.DeviceCA.CertPath == "" {
		return fmt.Errorf("device_ca.cert_path is required")
	}
	if _, err := time.ParseDuration(c.DeviceCA.DefaultTTL); err != nil {
		return fmt.Errorf("device_ca.default_ttl invalid: %w", err)
	}
	if c.DeviceCA.MinTTL != "" {
		if _, err := time.ParseDuration(c.DeviceCA.MinTTL); err != nil {
			return fmt.Errorf("device_ca.min_ttl invalid: %w", err)
		}
	}
	if c.DeviceCA.MaxTTL != "" {
		if _, err := time.ParseDuration(c.DeviceCA.MaxTTL); err != nil {
			return fmt.Errorf("device_ca.max_ttl invalid: %w", err)
		}
	}
	if c.DeviceCA.MinTTL != "" && c.DeviceCA.MaxTTL != "" {
		minTTL, _ := time.ParseDuration(c.DeviceCA.MinTTL)
		maxTTL, _ := time.ParseDuration(c.DeviceCA.MaxTTL)
		if maxTTL > 0 && minTTL > maxTTL {
			return fmt.Errorf("device_ca.min_ttl must be <= device_ca.max_ttl")
		}
	}
	for _, kt := range c.DeviceCA.AllowedKeyTypes {
		kt = strings.ToLower(strings.TrimSpace(kt))
		if kt != "ed25519" && kt != "ecdsa-p256" && kt != "" {
			return fmt.Errorf("device_ca.allowed_key_types must be ed25519 and/or ecdsa-p256")
		}
	}

	// --- pep auth mode (gateway->cp) ---
	if c.PEP.AuthMode != "token" && c.PEP.AuthMode != "mtls" {
		return fmt.Errorf("pep.auth_mode must be token or mtls")
	}
	if c.PEP.AuthMode == "token" && len(c.PEP.Tokens) == 0 {
		return fmt.Errorf("pep.tokens required for token auth")
	}
	if c.PEP.AuthMode == "mtls" {
		// pep_server must be valid and must be TLS with client auth CA.
		if c.PEPServer.Port < 1 || c.PEPServer.Port > 65535 {
			return fmt.Errorf("pep_server.port must be 1-65535 when pep.auth_mode=mtls")
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
	for _, pepID := range c.PEP.RevokedPEPIDs {
		if strings.TrimSpace(pepID) == "" {
			return fmt.Errorf("pep.revoked_pep_ids must not contain empty values")
		}
	}

	return nil
}

// -------------------- Defaults helpers --------------------

func applyDefaults(cfg *Config) {
	applyServerDefaults(&cfg.Server, 8080)

	// database
	if cfg.Database.BusyTimeout == "" {
		cfg.Database.BusyTimeout = "5s"
	}

	// oidc
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

	// pep auth
	if cfg.PEP.AuthMode == "" {
		cfg.PEP.AuthMode = "mtls"
	}
	if cfg.PEP.DecisionTTLSeconds <= 0 {
		cfg.PEP.DecisionTTLSeconds = 60
	}
	if cfg.PEP.AuthMode == "mtls" {
		applyServerDefaults(&cfg.PEPServer, 8443)
		cfg.PEPServer.TLS.Enabled = true
		cfg.PEPServer.TLS.RequireClientAuth = true

		// Inherit cert/key from public server if not set.
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

	// sshca
	if cfg.SSHCA.DefaultTTL == "" {
		cfg.SSHCA.DefaultTTL = "15m"
	}
	if cfg.SSHCA.MinTTL == "" {
		cfg.SSHCA.MinTTL = "1m"
	}
	if cfg.SSHCA.MaxTTL == "" {
		cfg.SSHCA.MaxTTL = "60m"
	}

	// device_ca defaults
	if cfg.DeviceCA.KeyPath == "" {
		cfg.DeviceCA.KeyPath = "pki/device_ca.key"
	}
	if cfg.DeviceCA.CertPath == "" {
		cfg.DeviceCA.CertPath = "pki/device_ca.crt"
	}
	if cfg.DeviceCA.DefaultTTL == "" {
		cfg.DeviceCA.DefaultTTL = "168h" // 7 days
	}
	if cfg.DeviceCA.MinTTL == "" {
		cfg.DeviceCA.MinTTL = "1h"
	}
	if cfg.DeviceCA.MaxTTL == "" {
		cfg.DeviceCA.MaxTTL = "720h" // 30 days
	}
	if len(cfg.DeviceCA.AllowedKeyTypes) == 0 {
		cfg.DeviceCA.AllowedKeyTypes = []string{"ed25519", "ecdsa-p256"}
	}

	// logging
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
}

// PEPRequireRegistrationEnabled returns true by default when unset.
func (c *Config) PEPRequireRegistrationEnabled() bool {
	if c.PEP.RequireRegistration == nil {
		return true
	}
	return *c.PEP.RequireRegistration
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

func parseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
