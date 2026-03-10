// Package cpapi fournit un client HTTP vers l'API publique du Control Plane.
// Utilisé côté client pour lister les ressources publiées accessibles à l'utilisateur.
package cpapi

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"client/internal/config"
)

// PublishedResource représente une ressource publiée renvoyée par le CP.
type PublishedResource struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	AccessMode  string `json:"access_mode"`
	Description string `json:"description"`
}

// Client appelle l'API publique du Control Plane.
type Client struct {
	cfg        *config.Config
	log        *slog.Logger
	httpClient *http.Client
}

// NewClient crée un client API CP avec la CA configurée.
func NewClient(cfg *config.Config, log *slog.Logger) *Client {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	if cfg.ControlPlane.CAFile != "" {
		caCert, err := os.ReadFile(cfg.ControlPlane.CAFile)
		if err == nil {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caCert)
			tlsConfig.RootCAs = pool
		} else {
			log.Warn("impossible de lire le CA du CP", "error", err)
		}
	}
	if cfg.ControlPlane.Insecure {
		tlsConfig.InsecureSkipVerify = true
	}

	return &Client{
		cfg: cfg,
		log: log,
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsConfig},
		},
	}
}

// ListResources appelle GET /api/v1/resources avec le bearer token OIDC.
func (c *Client) ListResources(accessToken string) ([]PublishedResource, error) {
	url := fmt.Sprintf("%s/api/v1/resources", c.cfg.ControlPlane.BaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("création requête: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("appel CP: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("lecture réponse: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("non authentifié (401) — exécutez 'ztna login'")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("réponse CP inattendue: status=%d body=%s", resp.StatusCode, string(body))
	}

	var resources []PublishedResource
	if err := json.Unmarshal(body, &resources); err != nil {
		return nil, fmt.Errorf("désérialisation réponse: %w", err)
	}

	return resources, nil
}