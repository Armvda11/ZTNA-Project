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
	"strings"
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
		log.Warn("client HTTP CP non initialisé, fallback TODO", "error", err)
	}
	return &Client{cfg: cfg, log: log, httpClient: httpClient}
}

// Authorize envoie une requête d'autorisation au Control Plane et
// retourne la décision (allow/deny).
//
// La requête est envoyée à :
//   POST {control_plane.base_url}/api/v1/pep/authorize
//
// Headers :
//   Content-Type: application/json
//   X-PEP-ID:     {pep.id}
//   X-PEP-TOKEN:  {pep.token}
//
// Body (JSON) — correspond au format attendu par le CP :
//   {
//     "subject": {
//       "sub": "<sub from cert>",
//       "username": "<username from cert>",
//       "groups": ["<group1>", ...]
//     },
//     "action": "connect",
//     "resource": {
//       "type": "ssh",
//       "host": "10.10.20.40",
//       "port": 22
//     },
//     "context": {
//       "src_ip": "10.10.20.10",
//       "gateway_id": "ztna-gw-1"
//     }
//   }
//
// Réponse attendue du CP :
//   {
//     "decision": "allow" | "deny",
//     "ttl_seconds": 300,
//     "reason": "...",
//     "policy_version": 1,
//     "decision_id": "uuid"
//   }
//
// TODO: Implémenter l'appel HTTP complet avec :
//   - TLS vers le CP (utiliser control_plane.ca_file si configuré)
//   - Timeouts configurables
//   - Retry avec backoff en cas d'erreur réseau (attention au retry sur deny)
//   - Journalisation structurée de chaque appel
//
// TODO: Supporter le mode mTLS entre Gateway et CP (évolution future)
func (c *Client) Authorize(req *AuthzRequest) (*AuthzResponse, error) {
	if c.httpClient == nil {
		return nil, fmt.Errorf("client HTTP non initialisé, vérifier la config TLS du CP")
	}

	c.log.Info("appel d'autorisation au Control Plane",
		"sub", req.Subject.Sub,
		"action", req.Action,
		"resource_type", req.Resource.Type,
		"resource_host", req.Resource.Host,
		"resource_port", req.Resource.Port,
	)

	// Construire le body JSON
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("impossible de sérialiser la requête d'autorisation: %w", err)
	}

	cpURL := strings.TrimRight(c.cfg.ControlPlane.BaseURL, "/") + "/api/v1/pep/authorize"
	httpReq, err := http.NewRequest(http.MethodPost, cpURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("impossible de construire la requête HTTP: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-PEP-ID", c.cfg.PEP.ID)
	httpReq.Header.Set("X-PEP-TOKEN", c.cfg.PEP.Token)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("impossible de joindre le Control Plane: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erreur CP (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var authzResp AuthzResponse
	if err := json.NewDecoder(resp.Body).Decode(&authzResp); err != nil {
		return nil, fmt.Errorf("impossible de parser la réponse CP: %w", err)
	}

	c.log.Info("décision d'autorisation reçue",
		"decision", authzResp.Decision,
		"decision_id", authzResp.DecisionID,
		"reason", authzResp.Reason,
		"sub", req.Subject.Sub,
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
