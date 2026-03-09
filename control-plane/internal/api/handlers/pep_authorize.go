// Package handlers expose les endpoints de la partie API publique du
// control-plane (PEP, admin, credentials, etc.). Les handlers sont
// responsables de la lecture des requêtes HTTP, de l'appel des services
// applicatifs et de la sérialisation des réponses JSON.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/logger"
	"control-plane/internal/service/audit"
	"control-plane/internal/service/decision"
	"control-plane/internal/service/gateway"
)

type PEPHandler struct {
	decision            *decision.Service
	audit               *audit.Service
	gatewaySvc          *gateway.Service
	requireRegistration bool
}

// NewPEPHandler crée un handler PEP qui traite les requêtes d'autorisation
// et journalise les décisions via le service d'audit.
func NewPEPHandler(
	decisionSvc *decision.Service,
	auditSvc *audit.Service,
	gatewaySvc *gateway.Service,
	requireRegistration bool,
) *PEPHandler {
	return &PEPHandler{
		decision:            decisionSvc,
		audit:               auditSvc,
		gatewaySvc:          gatewaySvc,
		requireRegistration: requireRegistration,
	}
}

type authorizeRequest struct {
	Subject  subjectRequest  `json:"subject"`
	Action   string          `json:"action"`
	Resource resourceRequest `json:"resource"`
	Context  map[string]any  `json:"context"`
}

type subjectRequest struct {
	Sub      string   `json:"sub"`
	Username string   `json:"username"`
	Groups   []string `json:"groups"`
}

type resourceHostPort struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// resourceRequest supporte deux formats JSON pour la ressource afin de rester
// compatible avec des clients futurs et le format envoyé par la gateway :
//
//	Format structuré (gateway) :
//	  {"type":"http", "http":{"host":"lan-app","port":80}}
//	  {"type":"ssh",  "ssh": {"host":"lan-app","port":22}}
//
//	Format plat (usage direct) :
//	  {"type":"ssh", "host":"lan-app", "port":22}
//
// Les helpers resolvedHost / resolvedPort normalisent les deux formes.
type resourceRequest struct {
	Type string `json:"type"`
	// Flat fields (legacy / simple format)
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
	// Structured sub-objects (gateway format)
	SSH  *resourceHostPort `json:"ssh,omitempty"`
	HTTP *resourceHostPort `json:"http,omitempty"`
}

// resolvedHost renvoie le host en privilégiant le sous-objet typé (ssh/http)
// sur le champ plat host pour maintenir la compatibilité descendante.
func (r resourceRequest) resolvedHost() string {
	switch strings.ToLower(r.Type) {
	case "ssh":
		if r.SSH != nil && r.SSH.Host != "" {
			return r.SSH.Host
		}
	case "http":
		if r.HTTP != nil && r.HTTP.Host != "" {
			return r.HTTP.Host
		}
	}
	return r.Host
}

// resolvedPort returns the port from either the flat or structured form.
func (r resourceRequest) resolvedPort() int {
	switch strings.ToLower(r.Type) {
	case "ssh":
		if r.SSH != nil && r.SSH.Port > 0 {
			return r.SSH.Port
		}
	case "http":
		if r.HTTP != nil && r.HTTP.Port > 0 {
			return r.HTTP.Port
		}
	}
	return r.Port
}

type authorizeResponse struct {
	Effect        string `json:"effect"`
	TTLSeconds    int    `json:"ttl_seconds"`
	Reason        string `json:"reason"`
	PolicyVersion int64  `json:"policy_version"`
	DecisionID    string `json:"decision_id"`
}

// Authorize traite une requête d'autorisation envoyée par un PEP (gateway).
//
// Flux complet :
//  1. Décodage + validation de la requête JSON.
//  2. Normalisation du sujet (username = sub si username vide).
//  3. Construction du model.Resource selon le type (ssh | http).
//  4. Appel au service décision qui évalue les politiques actives.
//  5. Enregistrement de la décision dans le journal d'audit.
//  6. Réponse JSON avec effect ("allow" | "deny") + raison + TTL.
//
// Note sur le champ JSON de réponse : le champ s'appelle `effect` (et non
// `decision`) pour correspondre au champ lu par le client gateway.
func (h *PEPHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	var req authorizeRequest
	// Limit request body to 1 MB to prevent OOM attacks
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if h.requireRegistration && h.gatewaySvc != nil {
		pepID := logger.PepIDFromContext(r.Context())
		if pepID == "" {
			writeError(w, domainErrors.ErrUnauthorized)
			return
		}
		status, err := h.gatewaySvc.Status(r.Context(), pepID)
		if err != nil || status != gateway.HeartbeatRegistered {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error":  domainErrors.ErrForbidden.Error(),
				"status": string(status),
			})
			return
		}
	}
	if req.Action == "" || req.Resource.Type == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if req.Subject.Username == "" && req.Subject.Sub == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	resType := strings.ToLower(req.Resource.Type)
	if resType != string(model.ResourceSSH) && resType != string(model.ResourceHTTP) {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if strings.TrimSpace(req.Resource.resolvedHost()) == "" || req.Resource.resolvedPort() <= 0 {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}

	subject := model.Subject{
		Sub:      req.Subject.Sub,
		Username: req.Subject.Username,
		Groups:   req.Subject.Groups,
	}
	if subject.Username == "" {
		subject.Username = subject.Sub
	}

	var resource model.Resource
	switch resType {
	case string(model.ResourceHTTP):
		resource = model.Resource{
			Type: model.ResourceHTTP,
			HTTP: &model.HTTPResource{
				Host: req.Resource.resolvedHost(),
				Port: req.Resource.resolvedPort(),
			},
		}
	default:
		resource = model.Resource{
			Type: model.ResourceSSH,
			SSH: &model.SSHResource{
				Host: req.Resource.resolvedHost(),
				Port: req.Resource.resolvedPort(),
			},
		}
	}

	decisionResp, err := h.decision.Authorize(r.Context(), decision.AuthorizeRequest{
		Subject:  subject,
		Action:   req.Action,
		Resource: resource,
		Context:  req.Context,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	pepID := logger.PepIDFromContext(r.Context())
	// Priority: 1) context.src_ip from gateway, 2) X-Forwarded-For/X-Real-IP, 3) RemoteAddr
	srcIP := extractContextIP(req.Context)
	if srcIP == "" {
		srcIP = extractRemoteIP(r)
	}
	if appendErr := h.audit.Append(r.Context(), model.AuditEvent{
		Subject:       formatSubjectForAudit(subject),
		Action:        req.Action,
		Resource:      resource.Canonical(),
		Decision:      string(decisionResp.Effect),
		Reason:        decisionResp.Reason,
		PepID:         pepID,
		SourceIP:      srcIP,
		PolicyVersion: decisionResp.PolicyVersion,
	}); appendErr != nil {
		slog.Error("audit append failed", "action", req.Action, "err", appendErr)
	}

	writeJSON(w, http.StatusOK, authorizeResponse{
		Effect:        string(decisionResp.Effect),
		TTLSeconds:    decisionResp.TTLSeconds,
		Reason:        decisionResp.Reason,
		PolicyVersion: decisionResp.PolicyVersion,
		DecisionID:    decisionResp.DecisionID,
	})
}
