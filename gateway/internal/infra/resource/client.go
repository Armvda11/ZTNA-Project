// Package resource fournit un client HTTP pour résoudre les ressources publiées
// via le Control Plane (GET /api/v1/pep/resources/{name}).
//
// La Gateway s'authentifie auprès du CP avec les headers PEP (X-PEP-ID, X-PEP-TOKEN).
package resource

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"ztna-gateway/internal/config"
	tlsutil "ztna-gateway/internal/infra/tls"
)

// ResolvedResource contient les informations d'une ressource publiée renvoyées par le CP.
type ResolvedResource struct {
	Name       string `json:"name"`
	Backend    string `json:"backend"`
	Type       string `json:"type"`
	AccessMode string `json:"access_mode"`
}

type cacheEntry struct {
	resource  *ResolvedResource
	expiresAt time.Time
}

// Client résout les ressources publiées via le Control Plane.
type Client struct {
	cfg        *config.Config
	log        *slog.Logger
	httpClient *http.Client
	mu         sync.RWMutex
	cache      map[string]*cacheEntry
	cacheTTL   time.Duration
}

// NewClient crée un nouveau client de résolution de ressources.
func NewClient(cfg *config.Config, log *slog.Logger) *Client {
	httpClient, err := tlsutil.NewControlPlaneHTTPClient(cfg, 10*time.Second)
	if err != nil {
		log.Warn("client HTTP CP non initialisé pour resource client", "error", err)
	}
	return &Client{
		cfg:        cfg,
		log:        log,
		httpClient: httpClient,
		cache:      make(map[string]*cacheEntry),
		cacheTTL:   60 * time.Second,
	}
}

// GetResource résout une ressource publiée par nom via le CP, avec un cache TTL.
func (c *Client) GetResource(name string) (*ResolvedResource, error) {
	// Consulter le cache
	c.mu.RLock()
	if entry, ok := c.cache[name]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		c.log.Debug("ressource servie depuis le cache", "name", name)
		return entry.resource, nil
	}
	c.mu.RUnlock()

	if c.httpClient == nil {
		return nil, fmt.Errorf("client HTTP non initialisé pour le CP")
	}

	url := fmt.Sprintf("%s/api/v1/pep/resources/%s", c.cfg.ControlPlane.BaseURL, name)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("création requête HTTP: %w", err)
	}
	req.Header.Set("X-PEP-ID", c.cfg.PEP.ID)
	req.Header.Set("X-PEP-TOKEN", c.cfg.PEP.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("appel CP résolution ressource: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("lecture réponse CP: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("ressource non trouvée: %s", name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("réponse CP inattendue: status=%d body=%s", resp.StatusCode, string(body))
	}

	var resolved ResolvedResource
	if err := json.Unmarshal(body, &resolved); err != nil {
		return nil, fmt.Errorf("désérialisation réponse CP: %w", err)
	}

	// Mettre en cache
	c.mu.Lock()
	c.cache[name] = &cacheEntry{
		resource:  &resolved,
		expiresAt: time.Now().Add(c.cacheTTL),
	}
	c.mu.Unlock()

	return &resolved, nil
}