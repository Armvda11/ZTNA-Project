// Package config gère le chargement et la validation de la configuration
// du client ZTNA à partir d'un fichier YAML. La structure suit les mêmes
// conventions que le Control Plane (Load → applyDefaults → Validate).
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config représente la configuration complète du client ZTNA.
type Config struct {
	OIDC         OIDCConfig         `yaml:"oidc"`
	ControlPlane ControlPlaneConfig `yaml:"control_plane"`
	Gateway      GatewayConfig      `yaml:"gateway"`
	TLS          TLSConfig          `yaml:"tls"`
	Storage      StorageConfig      `yaml:"storage"`
	Security     SecurityConfig     `yaml:"security"`
	Logging      LoggingConfig      `yaml:"logging"`
}

// OIDCConfig contient les paramètres du fournisseur OpenID Connect.
type OIDCConfig struct {
	Issuer       string `yaml:"issuer"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"` // Lab uniquement ; en prod utiliser PKCE
	Audience     string `yaml:"audience"`
}

// ControlPlaneConfig contient les paramètres de connexion au Control Plane.
type ControlPlaneConfig struct {
	BaseURL  string `yaml:"base_url"`
	CAFile   string `yaml:"ca_file"`
	Insecure bool   `yaml:"insecure_skip_verify"` // Lab uniquement
}

// GatewayConfig contient l'adresse de la Gateway ZTNA.
type GatewayConfig struct {
	Address string `yaml:"address"` // host:port pour le listener mTLS
}

// TLSConfig contient les paramètres TLS pour communiquer avec la Gateway.
type TLSConfig struct {
	CAFile string `yaml:"ca_file"` // CA de confiance pour vérifier le certificat Gateway
}

// StorageConfig indique où sauvegarder les tokens et certificats.
type StorageConfig struct {
	Path string `yaml:"path"` // Répertoire de stockage (ex: ~/.ztna/)
}

// SecurityConfig regroupe les options de sécurité du client.
type SecurityConfig struct {
	// InsecureAllowHTTPOIDC autorise les URL OIDC en HTTP au lieu de HTTPS.
	// À utiliser UNIQUEMENT en environnement de lab.
	InsecureAllowHTTPOIDC bool `yaml:"insecure_allow_http_oidc"`
}

// LoggingConfig définit le niveau et le format de log.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Load charge la configuration depuis le fichier YAML indiqué, applique
// les valeurs par défaut et valide la cohérence des champs.
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
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Storage.Path == "" {
		cfg.Storage.Path = "./.ztna"
	}
}

// Validate vérifie que la configuration est complète et cohérente.
// Retourne une erreur descriptive si un champ obligatoire est manquant.
func (c *Config) Validate() error {
	if c.OIDC.Issuer == "" {
		return fmt.Errorf("oidc.issuer est requis")
	}
	if c.OIDC.ClientID == "" {
		return fmt.Errorf("oidc.client_id est requis")
	}
	if c.ControlPlane.BaseURL == "" {
		return fmt.Errorf("control_plane.base_url est requis")
	}
	if c.Gateway.Address == "" {
		return fmt.Errorf("gateway.address est requis")
	}

	// Valider que l'issuer est HTTPS sauf si insecure_allow_http_oidc est activé
	if !c.Security.InsecureAllowHTTPOIDC {
		if len(c.OIDC.Issuer) >= 7 && c.OIDC.Issuer[:7] == "http://" {
			return fmt.Errorf("oidc.issuer doit être HTTPS en production (utilisez security.insecure_allow_http_oidc: true en lab)")
		}
	}

	return nil
}

// tokenExpiryMargin est la marge de sécurité avant l'expiration d'un token.
// Un token est considéré expiré tokenExpiryMargin avant sa date d'expiration réelle.
var tokenExpiryMargin = 30 * time.Second

// TokenExpiryMargin retourne la marge de sécurité configurée pour l'expiration des tokens.
func TokenExpiryMargin() time.Duration {
	return tokenExpiryMargin
}
