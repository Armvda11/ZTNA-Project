// Package tlsutil regroupe les helpers TLS partagés par la Gateway.
//
// Objectif: centraliser le chargement des certificats, des pools CA et la
// construction des clients HTTPS vers le Control Plane pour éviter la
// duplication entre les couches mtls, authorize et futurs composants.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"ztna-gateway/internal/config"
)

// LoadKeyPair charge le certificat serveur et sa clé privée.
func LoadKeyPair(certFile, keyFile string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("charger cert/key TLS: %w", err)
	}
	return cert, nil
}

// LoadCertPoolFromPEMFile charge un fichier PEM dans un CertPool.
func LoadCertPoolFromPEMFile(pemFile string) (*x509.CertPool, error) {
	pemData, err := os.ReadFile(pemFile)
	if err != nil {
		return nil, fmt.Errorf("lire fichier CA %s: %w", pemFile, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("parser CA PEM depuis %s", pemFile)
	}
	return pool, nil
}

// NewControlPlaneHTTPClient crée un client HTTPS pour le CP.
//
// Supporte:
// - mode token (cert client absent)
// - mode mtls (cert client requis)
// - validation via CA pinning (ca_file) ou skip verify (lab)
func NewControlPlaneHTTPClient(cfg *config.Config, timeout time.Duration) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}
	if cfg.ControlPlane.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true //nolint:gosec
	} else if cfg.ControlPlane.CAFile != "" {
		pool, err := LoadCertPoolFromPEMFile(cfg.ControlPlane.CAFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	}

	if cfg.ControlPlane.AuthMode == "mtls" {
		cert, err := LoadKeyPair(cfg.ControlPlane.ClientCertFile, cfg.ControlPlane.ClientKeyFile)
		if err != nil {
			return nil, err
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}, nil
}
