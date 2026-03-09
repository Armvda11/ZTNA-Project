// Package authorize fournit le client HTTP pour appeler l'endpoint
// d'autorisation du Control Plane (POST /api/v1/pep/authorize).
//
// La Gateway s'authentifie auprès du CP avec les headers PEP :
//   - X-PEP-ID    : identifiant de la gateway (ex: "ztna-gw-1")
//   - X-PEP-TOKEN : secret partagé configuré dans le CP
//
// Ce mécanisme d'authentification correspond au mode "token" du CP.
// Une évolution future pourrait utiliser mTLS entre la Gateway et le CP.
package authorize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"ztna-gateway/internal/config"
	"ztna-gateway/internal/core/domain"
	tlsutil "ztna-gateway/internal/infra/tls"
)

// Client est le client d'autorisation vers le Control Plane.
type Client struct {
	cfg        *config.Config
	log        *slog.Logger
	httpClient *http.Client
}

// NewClient crée un nouveau client d'autorisation.
func NewClient(cfg *config.Config, log *slog.Logger) *Client {
	httpClient, err := tlsutil.NewControlPlaneHTTPClient(cfg, 10*time.Second)
	if err != nil {
		log.Warn("client HTTP CP non initialisé", "error", err)
	}
	return &Client{cfg: cfg, log: log, httpClient: httpClient}
}

// GatewayID retourne l'identifiant de la gateway depuis la config.
func (c *Client) GatewayID() string {
	return c.cfg.GatewayID
}

// Authorize envoie une requête d'autorisation au Control Plane et
// retourne la décision (allow/deny).
func (c *Client) Authorize(req *AuthzRequest) (*AuthzResponse, error) {
	if c.httpClient == nil {
		return nil, fmt.Errorf("client HTTP non initialisé pour le CP")
	}

	c.log.Debug("envoi requête authorize au CP",
		"sub", req.Subject.Sub,
		"action", req.Action,
		"resource_type", req.Resource.Type,
		"resource_host", req.Resource.Host,
		"resource_port", req.Resource.Port,
	)

	// Construire le body JSON conforme au format du CP
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("sérialisation requête authorize: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/pep/authorize", c.cfg.ControlPlane.BaseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("création requête HTTP: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-PEP-ID", c.cfg.PEP.ID)
	httpReq.Header.Set("X-PEP-TOKEN", c.cfg.PEP.Token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrControlPlaneUnreachable, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("lecture réponse CP: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentification PEP refusée (401) — vérifier pep.id/pep.token")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("gateway non enregistrée auprès du CP (403)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("réponse CP inattendue: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var authzResp AuthzResponse
	if err := json.Unmarshal(respBody, &authzResp); err != nil {
		return nil, fmt.Errorf("désérialisation réponse CP: %w", err)
	}

	// Le CP retourne "effect" mais notre struct lit "decision" — mapper si besoin
	if authzResp.Decision == "" {
		// Essayer de lire "effect" comme champ alternatif
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(respBody, &raw); err == nil {
			if effectRaw, ok := raw["effect"]; ok {
				var effect string
				if json.Unmarshal(effectRaw, &effect) == nil {
					authzResp.Decision = effect
				}
			}
		}
	}

	c.log.Info("réponse authorize CP",
		"decision", authzResp.Decision,
		"decision_id", authzResp.DecisionID,
		"reason", authzResp.Reason,
		"ttl_seconds", authzResp.TTLSeconds,
	)

	return &authzResp, nil
}

// AuthzRequest est la requête d'autorisation envoyée au CP.
// Elle correspond au format attendu par POST /api/v1/pep/authorize.
type AuthzRequest struct {
	Subject  domain.SubjectRef `json:"subject"`
	Action   string            `json:"action"`
	Resource ResourceRef       `json:"resource"`
	Context  AuthzContext       `json:"context"`
}

// ResourceRef identifie la ressource pour la requête d'autorisation.
// Correspond au format resourceRequest du CP.
type ResourceRef struct {
	Type string `json:"type"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// AuthzContext contient le contexte de la requête d'autorisation.
type AuthzContext struct {
	SourceIP  string `json:"src_ip,omitempty"`
	GatewayID string `json:"gateway_id,omitempty"`
}

// AuthzResponse est la réponse du CP à une requête d'autorisation.
type AuthzResponse struct {
	Decision      string `json:"decision"`
	TTLSeconds    int    `json:"ttl_seconds"`
	Reason        string `json:"reason"`
	PolicyVersion int64  `json:"policy_version"`
	DecisionID    string `json:"decision_id"`
}
