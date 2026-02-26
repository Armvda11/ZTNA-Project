// Package tls centralise la configuration TLS sortante du client.
//
// Ce fichier fournit les clients HTTP sécurisés pour les appels HTTPS
// vers le Control Plane et le fournisseur OIDC (Keycloak).
package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"

	"client/internal/config"
)

const (
	defaultDialTimeout      = 10 * time.Second
	defaultTLSHandshakeTime = 10 * time.Second
	defaultHTTPTimeout      = 20 * time.Second
)

// NewControlPlaneHTTPClient construit un client HTTP sécurisé pour joindre
// le Control Plane depuis des machines distantes (lab/prod).
func NewControlPlaneHTTPClient(cfg *config.Config) (*http.Client, error) {
	tlsConfig, err := buildControlPlaneTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	return newHTTPClientWithTLS(tlsConfig), nil
}

// NewOIDCHTTPClient construit un client HTTP sécurisé pour joindre le
// fournisseur OIDC (Keycloak). Il utilise la CA du provider OIDC
// (oidc.ca_file) distincte de celle du Control Plane.
func NewOIDCHTTPClient(cfg *config.Config) (*http.Client, error) {
	tlsConfig, err := buildOIDCTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	return newHTTPClientWithTLS(tlsConfig), nil
}

// newHTTPClientWithTLS crée un http.Client avec la config TLS fournie.
func newHTTPClientWithTLS(tlsConfig *tls.Config) *http.Client {
	transport := &http.Transport{
		TLSClientConfig:     tlsConfig,
		TLSHandshakeTimeout: defaultTLSHandshakeTime,
		IdleConnTimeout:     30 * time.Second,
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 8,
	}

	// TODO: brancher un net.Dialer explicite avec KeepAlive finement réglé
	// selon les conditions WAN du projet (latence/pertes).
	_ = defaultDialTimeout

	return &http.Client{
		Timeout:   defaultHTTPTimeout,
		Transport: transport,
	}
}

func buildControlPlaneTLSConfig(cfg *config.Config) (*tls.Config, error) {
	return buildTLSConfigFromCA(cfg.ControlPlane.CAFile, cfg.ControlPlane.Insecure)
}

func buildOIDCTLSConfig(cfg *config.Config) (*tls.Config, error) {
	return buildTLSConfigFromCA(cfg.OIDC.CAFile, cfg.OIDC.Insecure)
}

// buildTLSConfigFromCA construit une tls.Config TLS 1.3 avec une CA custom
// optionnelle et un flag insecure pour les environnements de lab.
func buildTLSConfigFromCA(caFile string, insecure bool) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: insecure,
	}

	if caFile == "" {
		return tlsConfig, nil
	}

	caData, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("impossible de lire ca_file %s: %w", caFile, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("ca_file invalide (PEM): %s", caFile)
	}

	tlsConfig.RootCAs = pool

	// TODO: Ajouter le pinning cert/public key optionnel pour durcir
	// les échanges dans les environnements sensibles.
	return tlsConfig, nil
}
