// Package config gère le chargement et la validation de la configuration
// de la Gateway ZTNA à partir d'un fichier YAML. La structure suit les
// mêmes conventions que le Control Plane.
// Package config loads and validates the ZTNA gateway configuration.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config représente la configuration complète de la Gateway ZTNA.
type Config struct {
	Server       ServerConfig       `yaml:"server"`
	ControlPlane ControlPlaneConfig `yaml:"control_plane"`
	PEP          PEPConfig          `yaml:"pep"`
	Proxy        ProxyConfig        `yaml:"proxy"`
	Logging      LoggingConfig      `yaml:"logging"`
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
	ClientCAFile string `yaml:"client_ca_file"` // CA pour vérifier les certs clients
}

// ControlPlaneConfig contient les paramètres de connexion au Control Plane.
type ControlPlaneConfig struct {
	BaseURL  string `yaml:"base_url"`
	CAFile   string `yaml:"ca_file"`              // CA pour vérifier le cert du CP
	Insecure bool   `yaml:"insecure_skip_verify"` // Lab uniquement
}

// PEPConfig contient les identifiants PEP pour s'authentifier auprès du CP.
// Correspond aux headers X-PEP-ID et X-PEP-TOKEN attendus par le CP.
type PEPConfig struct {
	ID    string `yaml:"id"`
	Token string `yaml:"token"`
}

// ProxyConfig contient les paramètres du proxy TCP.
type ProxyConfig struct {
	DialTimeout string `yaml:"dial_timeout"`
	MaxConns    int    `yaml:"max_conns"`
	RateLimit   int    `yaml:"rate_limit"` // Requêtes par seconde par sujet (0 = illimité)
}

// LoggingConfig définit le niveau et le format de log.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
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
}

// Validate vérifie que la configuration est complète et cohérente.
func (c *Config) Validate() error {
	// Validation server
	if c.Server.ListenAddr == "" {
		return fmt.Errorf("server.listen_addr est requis")
	}
	if c.Server.TLS.CertFile == "" {
		return fmt.Errorf("server.tls.cert_file est requis")
	}
	if c.Server.TLS.KeyFile == "" {
		return fmt.Errorf("server.tls.key_file est requis")
	}
	if c.Server.TLS.ClientCAFile == "" {
		return fmt.Errorf("server.tls.client_ca_file est requis (mTLS obligatoire)")
	}

	// Validation Control Plane
	if c.ControlPlane.BaseURL == "" {
		return fmt.Errorf("control_plane.base_url est requis")
	}

	// Validation PEP
	if c.PEP.ID == "" {
		return fmt.Errorf("pep.id est requis")
	}
	if c.PEP.Token == "" {
		return fmt.Errorf("pep.token est requis")
	}
	if len(c.PEP.Token) < 16 {
		return fmt.Errorf("pep.token doit faire au moins 16 caractères")
	}

	// Validation proxy
	if _, err := time.ParseDuration(c.Proxy.DialTimeout); err != nil {
		return fmt.Errorf("proxy.dial_timeout invalide: %w", err)
	}
	if c.Proxy.MaxConns < 1 {
		return fmt.Errorf("proxy.max_conns doit être > 0")
	}

	return nil
}

// DialTimeoutDuration retourne le dial_timeout parsé en time.Duration.
// Cette méthode ne retourne pas d'erreur car la validation a déjà été
// effectuée dans Validate().
func (c *Config) DialTimeoutDuration() time.Duration {
	d, err := time.ParseDuration(c.Proxy.DialTimeout)
	if err != nil {
		// Ne devrait jamais arriver si Validate() a été appelé
		return 10 * time.Second
	}
	return d
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
