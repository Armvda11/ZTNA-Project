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
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"ztna-gateway/internal/config"
	"ztna-gateway/internal/core/domain"
	"ztna-gateway/internal/infra/tls"
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
		c.log.Debug("authorize appelé sans httpClient initialisé; mode skeleton/TODO")
	}

	c.log.Info("appel d'autorisation au Control Plane",
		"sub", req.Subject.Sub,
		"action", req.Action,
		"resource_type", req.Resource.Type,
		"resource_host", req.Resource.Host,
		"resource_port", req.Resource.Port,
	)

	// TODO: construire le body JSON conforme au format du CP
	// TODO: créer un http.Client avec la tls.Config appropriée
	// TODO: créer la requête POST avec les headers X-PEP-ID et X-PEP-TOKEN
	// TODO: envoyer la requête et lire la réponse
	// TODO: parser la réponse JSON en AuthzResponse
	// TODO: gérer les erreurs HTTP (401, 403, 500, timeout, etc.)

	return nil, fmt.Errorf("TODO: Authorize non implémenté")
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
