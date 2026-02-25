// Package config loads and validates the ZTNA gateway configuration.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for the ZTNA gateway.
type Config struct {
	ListenAddr string    `yaml:"listen_addr"`
	GatewayID  string    `yaml:"gateway_id"`
	TLS        TLSConfig `yaml:"tls"`

	CPURL         string `yaml:"cp_url"`
	CPAuthMode    string `yaml:"cp_auth_mode"` // "mtls" (default) or "token" (lab fallback)
	CPCACert      string `yaml:"cp_ca_cert"`
	CPClientCert  string `yaml:"cp_client_cert"`
	CPClientKey   string `yaml:"cp_client_key"`
	CPTLSInsecure bool   `yaml:"cp_tls_insecure"`

	PEPID    string `yaml:"pep_id"`
	PEPToken string `yaml:"pep_token"` // required in token mode only

	HeartbeatEvery       time.Duration `yaml:"heartbeat_every"`
	RequireRegistration  *bool         `yaml:"require_registration"`
	StrictRevocation     *bool         `yaml:"strict_revocation"`
	DecisionCacheTTL     time.Duration `yaml:"decision_cache_ttl"`
	DecisionCacheMaxKeys int           `yaml:"decision_cache_max_entries"`
	CPDownMode           string        `yaml:"cp_down_mode"` // "deny" (default) | "cache_allow"
	// CRLRefreshInterval définit la fréquence de rafraîchissement de la CRL
	// depuis le Control Plane. Défaut : 60 s. 0 = désactivé (pas de CRL).
	CRLRefreshInterval time.Duration `yaml:"crl_refresh_interval"`
	Routes             []Route       `yaml:"routes"`
}

// TLSConfig holds TLS material paths.
type TLSConfig struct {
	DeviceCACert string `yaml:"device_ca_cert"`
	ServerCert   string `yaml:"server_cert"`
	ServerKey    string `yaml:"server_key"`
}

// Route maps a resource type + match pattern to a backend address.
type Route struct {
	ResourceType  string `yaml:"resource_type"`
	ResourceMatch string `yaml:"resource_match"`
	Target        string `yaml:"target"`
}

// Load reads and validates the YAML configuration at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if c.ListenAddr == "" {
		c.ListenAddr = "0.0.0.0:4433"
	}
	if c.GatewayID == "" {
		return fmt.Errorf("gateway_id is required")
	}
	if c.CPURL == "" {
		return fmt.Errorf("cp_url is required")
	}
	if c.CPAuthMode == "" {
		c.CPAuthMode = "mtls"
	}
	if c.CPAuthMode != "mtls" && c.CPAuthMode != "token" {
		return fmt.Errorf("cp_auth_mode must be mtls or token")
	}
	if c.PEPID == "" {
		return fmt.Errorf("pep_id is required")
	}
	if c.CPAuthMode == "token" && c.PEPToken == "" {
		return fmt.Errorf("pep_token is required")
	}
	if c.CPAuthMode == "mtls" {
		if c.CPClientCert == "" || c.CPClientKey == "" {
			return fmt.Errorf("cp_client_cert and cp_client_key are required when cp_auth_mode=mtls")
		}
		if !c.CPTLSInsecure && c.CPCACert == "" {
			return fmt.Errorf("cp_ca_cert is required when cp_auth_mode=mtls and cp_tls_insecure=false")
		}
	}
	if c.HeartbeatEvery == 0 {
		c.HeartbeatEvery = 30 * time.Second
	}
	if c.DecisionCacheTTL == 0 {
		c.DecisionCacheTTL = 60 * time.Second
	}
	if c.DecisionCacheMaxKeys <= 0 {
		c.DecisionCacheMaxKeys = 5000
	}
	if c.CPDownMode == "" {
		c.CPDownMode = "deny"
	}
	if c.CPDownMode != "deny" && c.CPDownMode != "cache_allow" {
		return fmt.Errorf("cp_down_mode must be deny or cache_allow")
	}
	if c.CRLRefreshInterval == 0 {
		c.CRLRefreshInterval = 60 * time.Second
	}
	return nil
}

// RequireRegistrationEnabled returns true by default when unset.
func (c *Config) RequireRegistrationEnabled() bool {
	if c.RequireRegistration == nil {
		return true
	}
	return *c.RequireRegistration
}

// StrictRevocationEnabled returns true by default when unset.
func (c *Config) StrictRevocationEnabled() bool {
	if c.StrictRevocation == nil {
		return true
	}
	return *c.StrictRevocation
}
