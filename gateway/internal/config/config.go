// Package config gère le chargement et la validation de la configuration
// de la Gateway ZTNA à partir d'un fichier YAML.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config représente la configuration complète de la Gateway ZTNA.
// Un seul format YAML est supporté: server/control_plane/pep/proxy/logging +
// les options globales runtime (gateway_id, routes, caches, heartbeat...).
type Config struct {
	Server       ServerConfig       `yaml:"server"`
	ControlPlane ControlPlaneConfig `yaml:"control_plane"`
	PEP          PEPConfig          `yaml:"pep"`
	Proxy        ProxyConfig        `yaml:"proxy"`
	Logging      LoggingConfig      `yaml:"logging"`

	GatewayID            string        `yaml:"gateway_id"`
	HeartbeatEvery       time.Duration `yaml:"heartbeat_every"`
	RequireRegistration  *bool         `yaml:"require_registration"`
	StrictRevocation     *bool         `yaml:"strict_revocation"`
	DecisionCacheTTL     time.Duration `yaml:"decision_cache_ttl"`
	DecisionCacheMaxKeys int           `yaml:"decision_cache_max_entries"`
	CPDownMode           string        `yaml:"cp_down_mode"`
	CRLRefreshInterval   time.Duration `yaml:"crl_refresh_interval"`
	Routes               []Route       `yaml:"routes"`
}

// ServerConfig contient les paramètres du serveur mTLS de la Gateway.
type ServerConfig struct {
	ListenAddr string          `yaml:"listen_addr"`
	TLS        ServerTLSConfig `yaml:"tls"`
}

// ServerTLSConfig contient les paramètres TLS du serveur Gateway.
type ServerTLSConfig struct {
	CertFile     string `yaml:"cert_file"`
	KeyFile      string `yaml:"key_file"`
	ClientCAFile string `yaml:"client_ca_file"`
}

// ControlPlaneConfig contient les paramètres de connexion au Control Plane.
type ControlPlaneConfig struct {
	BaseURL            string `yaml:"base_url"`
	CAFile             string `yaml:"ca_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
	AuthMode           string `yaml:"auth_mode"`
	ClientCertFile     string `yaml:"client_cert_file"`
	ClientKeyFile      string `yaml:"client_key_file"`
}

// PEPConfig contient les identifiants PEP.
type PEPConfig struct {
	ID    string `yaml:"id"`
	Token string `yaml:"token"`
}

// ProxyConfig contient les paramètres du proxy TCP.
type ProxyConfig struct {
	DialTimeout string `yaml:"dial_timeout"`
	MaxConns    int    `yaml:"max_conns"`
	RateLimit   int    `yaml:"rate_limit"`
}

// LoggingConfig définit le niveau et le format de log.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Route mappe une ressource vers une cible LAN.
type Route struct {
	ResourceType  string `yaml:"resource_type"`
	ResourceMatch string `yaml:"resource_match"`
	Target        string `yaml:"target"`
}

// Load charge la configuration depuis le fichier YAML indiqué.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("impossible de lire le fichier de configuration %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("erreur de parsing YAML: %w", err)
	}

	applyDefaults(&cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration invalide: %w", err)
	}

	return &cfg, nil
}

// applyDefaults remplit les valeurs par défaut pour les champs non renseignés.
func applyDefaults(cfg *Config) {
	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = "0.0.0.0:9443"
	}
	if cfg.GatewayID == "" {
		cfg.GatewayID = cfg.PEP.ID
	}
	if cfg.Proxy.DialTimeout == "" {
		cfg.Proxy.DialTimeout = "10s"
	}
	if cfg.Proxy.MaxConns == 0 {
		cfg.Proxy.MaxConns = 1000
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}

	if cfg.ControlPlane.AuthMode == "" {
		cfg.ControlPlane.AuthMode = "token"
	}
	if cfg.HeartbeatEvery == 0 {
		cfg.HeartbeatEvery = 30 * time.Second
	}
	if cfg.DecisionCacheTTL == 0 {
		cfg.DecisionCacheTTL = 60 * time.Second
	}
	if cfg.DecisionCacheMaxKeys <= 0 {
		cfg.DecisionCacheMaxKeys = 5000
	}
	if cfg.CPDownMode == "" {
		cfg.CPDownMode = "deny"
	}
	if cfg.CRLRefreshInterval == 0 {
		cfg.CRLRefreshInterval = 60 * time.Second
	}
}

// Validate vérifie que la configuration est complète et cohérente.
func (c *Config) Validate() error {
	if c.Server.ListenAddr == "" {
		return fmt.Errorf("server.listen_addr est requis")
	}

	if c.Server.TLS.CertFile != "" && c.Server.TLS.KeyFile == "" {
		return fmt.Errorf("server.tls.key_file est requis quand server.tls.cert_file est défini")
	}
	if c.Server.TLS.KeyFile != "" && c.Server.TLS.CertFile == "" {
		return fmt.Errorf("server.tls.cert_file est requis quand server.tls.key_file est défini")
	}

	if c.ControlPlane.BaseURL == "" {
		return fmt.Errorf("control_plane.base_url est requis")
	}

	if c.PEP.ID == "" {
		return fmt.Errorf("pep.id est requis")
	}

	if c.ControlPlane.AuthMode != "mtls" && c.ControlPlane.AuthMode != "token" {
		return fmt.Errorf("control_plane.auth_mode doit être mtls ou token")
	}

	if c.ControlPlane.AuthMode == "token" {
		if c.PEP.Token == "" {
			return fmt.Errorf("pep.token est requis")
		}
		if len(c.PEP.Token) < 16 {
			return fmt.Errorf("pep.token doit faire au moins 16 caractères")
		}
	}

	if c.ControlPlane.AuthMode == "mtls" {
		if c.ControlPlane.ClientCertFile == "" || c.ControlPlane.ClientKeyFile == "" {
			return fmt.Errorf("control_plane.client_cert_file et control_plane.client_key_file sont requis quand control_plane.auth_mode=mtls")
		}
		if !c.ControlPlane.InsecureSkipVerify && c.ControlPlane.CAFile == "" {
			return fmt.Errorf("control_plane.ca_file est requis quand control_plane.auth_mode=mtls et insecure_skip_verify=false")
		}
	}

	if _, err := time.ParseDuration(c.Proxy.DialTimeout); err != nil {
		return fmt.Errorf("proxy.dial_timeout invalide: %w", err)
	}
	if c.Proxy.MaxConns < 1 {
		return fmt.Errorf("proxy.max_conns doit être > 0")
	}

	if c.CPDownMode != "deny" && c.CPDownMode != "cache_allow" {
		return fmt.Errorf("cp_down_mode doit être deny ou cache_allow")
	}

	return nil
}

// DialTimeoutDuration retourne le dial_timeout parsé en time.Duration.
func (c *Config) DialTimeoutDuration() time.Duration {
	d, err := time.ParseDuration(c.Proxy.DialTimeout)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// RequireRegistrationEnabled retourne true par défaut.
func (c *Config) RequireRegistrationEnabled() bool {
	if c.RequireRegistration == nil {
		return true
	}
	return *c.RequireRegistration
}

// StrictRevocationEnabled retourne true par défaut.
func (c *Config) StrictRevocationEnabled() bool {
	if c.StrictRevocation == nil {
		return true
	}
	return *c.StrictRevocation
}
