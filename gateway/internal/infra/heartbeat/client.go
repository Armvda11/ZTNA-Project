// Package heartbeat implémente le heartbeat périodique vers le Control Plane.
//
// La gateway envoie un POST /api/v1/pep/heartbeat à intervalle régulier
// pour signaler qu'elle est vivante. Le CP utilise cette information pour
// marquer les gateways comme actives/inactives dans son registre.
package heartbeat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ztna-gateway/internal/config"
)

// Client gère le heartbeat périodique vers le CP.
type Client struct {
	cfg        *config.Config
	log        *slog.Logger
	httpClient *http.Client
}

// NewClient crée un client heartbeat.
func NewClient(cfg *config.Config, httpClient *http.Client, log *slog.Logger) *Client {
	return &Client{
		cfg:        cfg,
		log:        log,
		httpClient: httpClient,
	}
}

type heartbeatRequest struct {
	Version string `json:"version,omitempty"`
}

// StartLoop envoie un heartbeat à intervalle cfg.HeartbeatEvery.
// Bloque jusqu'à l'annulation du contexte.
func (c *Client) StartLoop(ctx context.Context) error {
	if c.httpClient == nil {
		c.log.Warn("heartbeat désactivé: client HTTP non initialisé")
		return nil
	}

	interval := c.cfg.HeartbeatEvery
	if interval <= 0 {
		interval = 30 * time.Second
	}

	c.log.Info("heartbeat loop démarré",
		"interval", interval.String(),
		"cp_base_url", c.cfg.ControlPlane.BaseURL,
	)

	// Premier beat immédiat
	c.beat(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.log.Info("heartbeat loop arrêté")
			return nil
		case <-ticker.C:
			c.beat(ctx)
		}
	}
}

func (c *Client) beat(ctx context.Context) {
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	body, _ := json.Marshal(heartbeatRequest{Version: "1.0.0"})
	url := fmt.Sprintf("%s/api/v1/pep/heartbeat", c.cfg.ControlPlane.BaseURL)

	httpReq, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewReader(body))
	if err != nil {
		c.log.Warn("heartbeat: erreur création requête", "error", err)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-PEP-ID", c.cfg.PEP.ID)
	httpReq.Header.Set("X-PEP-TOKEN", c.cfg.PEP.Token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.log.Warn("heartbeat: CP inaccessible", "error", err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusOK {
		c.log.Debug("heartbeat OK", "status", resp.StatusCode)
	} else {
		c.log.Warn("heartbeat: réponse inattendue", "status", resp.StatusCode)
	}
}
