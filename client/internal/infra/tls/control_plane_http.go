// Package tls centralise la configuration TLS sortante du client.
//
// Ce fichier vise les appels HTTPS vers le Control Plane (cert issuance,
// futures APIs de session/heartbeat).
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
	}, nil
}

func buildControlPlaneTLSConfig(cfg *config.Config) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: cfg.ControlPlane.Insecure,
	}

	if cfg.ControlPlane.CAFile == "" {
		return tlsConfig, nil
	}

	caData, err := os.ReadFile(cfg.ControlPlane.CAFile)
	if err != nil {
		return nil, fmt.Errorf("impossible de lire control_plane.ca_file: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caData) {
		return nil, fmt.Errorf("control_plane.ca_file invalide (PEM)")
	}

	tlsConfig.RootCAs = pool

	// TODO: Ajouter le pinning cert/public key optionnel pour durcir
	// les échanges client->control-plane dans les environnements sensibles.
	return tlsConfig, nil
}
