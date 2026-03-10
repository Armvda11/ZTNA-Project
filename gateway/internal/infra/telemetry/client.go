// Package telemetry implémente l'envoi de télémétrie de session vers le CP.
//
// Le CP expose deux endpoints :
//   - POST /api/v1/pep/sessions/start — notifie le début d'une session
//   - POST /api/v1/pep/sessions/end   — notifie la fin d'une session
//
// La télémétrie est envoyée en fire-and-forget (goroutine non bloquante)
// pour ne pas ralentir le trafic proxy.
package telemetry

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

// Client envoie la télémétrie de session au Control Plane.
type Client struct {
	cfg        *config.Config
	log        *slog.Logger
	httpClient *http.Client
}

// NewClient crée un client de télémétrie.
func NewClient(cfg *config.Config, httpClient *http.Client, log *slog.Logger) *Client {
	return &Client{
		cfg:        cfg,
		log:        log,
		httpClient: httpClient,
	}
}

// SessionStartRequest est la payload envoyée au CP au début d'une session.
type SessionStartRequest struct {
	SessionID       string `json:"session_id"`
	DecisionID      string `json:"decision_id"`
	SubjectSub      string `json:"subject_sub"`
	SubjectUsername string `json:"subject_username"`
	ResourceType    string `json:"resource_type"`
	ResourceName    string `json:"resource_name,omitempty"`
	ResourceMatch   string `json:"resource_match"`
	DeviceSerial    string `json:"device_serial,omitempty"`
}

// SessionEndRequest est la payload envoyée au CP en fin de session.
type SessionEndRequest struct {
	SessionID  string `json:"session_id"`
	BytesIn    int64  `json:"bytes_in"`
	BytesOut   int64  `json:"bytes_out"`
	DurationMs int64  `json:"duration_ms"`
	EndReason  string `json:"end_reason"`
}

// NotifyStart informe le CP qu'une session a démarré (non bloquant).
func (c *Client) NotifyStart(ctx context.Context, req SessionStartRequest) {
	go func() {
		if err := c.postJSON(ctx, "/api/v1/pep/sessions/start", req); err != nil {
			c.log.Warn("échec notification session start",
				"session_id", req.SessionID,
				"error", err,
			)
		} else {
			c.log.Debug("session start notifié au CP", "session_id", req.SessionID)
		}
	}()
}

// NotifyEnd informe le CP qu'une session s'est terminée (non bloquant).
func (c *Client) NotifyEnd(ctx context.Context, req SessionEndRequest) {
	go func() {
		if err := c.postJSON(ctx, "/api/v1/pep/sessions/end", req); err != nil {
			c.log.Warn("échec notification session end",
				"session_id", req.SessionID,
				"error", err,
			)
		} else {
			c.log.Debug("session end notifié au CP",
				"session_id", req.SessionID,
				"duration_ms", req.DurationMs,
				"bytes_in", req.BytesIn,
				"bytes_out", req.BytesOut,
			)
		}
	}()
}

func (c *Client) postJSON(ctx context.Context, path string, payload any) error {
	if c.httpClient == nil {
		return fmt.Errorf("client HTTP non initialisé")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sérialisation telemetry: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s%s", c.cfg.ControlPlane.BaseURL, path)
	httpReq, err := http.NewRequestWithContext(reqCtx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("création requête: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-PEP-ID", c.cfg.PEP.ID)
	httpReq.Header.Set("X-PEP-TOKEN", c.cfg.PEP.Token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("envoi telemetry: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain body

	if resp.StatusCode >= 400 {
		return fmt.Errorf("CP telemetry retourne status %d", resp.StatusCode)
	}

	return nil
}
