// Package handlers expose les endpoints de la partie API publique du
// control-plane (PEP, admin, credentials, etc.). Les handlers sont
// responsables de la lecture des requêtes HTTP, de l'appel des services
// applicatifs et de la sérialisation des réponses JSON.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/logger"
	"control-plane/internal/service/audit"
	"control-plane/internal/service/decision"
)

type PEPHandler struct {
	decision *decision.Service
	audit    *audit.Service
}

// NewPEPHandler crée un handler PEP qui traite les requêtes d'autorisation
// et journalise les décisions via le service d'audit.
func NewPEPHandler(decisionSvc *decision.Service, auditSvc *audit.Service) *PEPHandler {
	return &PEPHandler{decision: decisionSvc, audit: auditSvc}
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

type resourceRequest struct {
	Type string `json:"type"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

type authorizeResponse struct {
	Decision      string `json:"decision"`
	TTLSeconds    int    `json:"ttl_seconds"`
	Reason        string `json:"reason"`
	PolicyVersion int64  `json:"policy_version"`
	DecisionID    string `json:"decision_id"`
}

// Authorize traite une requête d'autorisation envoyée par le PEP. Elle
// valide l'entrée, appelle le service `decision` pour obtenir une
// décision, journalise l'événement d'audit et renvoie la réponse JSON.
// Le champ `context.src_ip` est priorisé pour la source IP (fourni par
// la gateway) avant de retomber sur les en-têtes HTTP classiques.
func (h *PEPHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	var req authorizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if req.Action == "" || req.Resource.Type == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if req.Subject.Username == "" && req.Subject.Sub == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if strings.ToLower(req.Resource.Type) != string(model.ResourceSSH) {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if strings.TrimSpace(req.Resource.Host) == "" || req.Resource.Port <= 0 {
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
	resource := model.Resource{
		Type: model.ResourceSSH,
		SSH: &model.SSHResource{
			Host: req.Resource.Host,
			Port: req.Resource.Port,
		},
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
	_ = h.audit.Append(r.Context(), model.AuditEvent{
		Subject:       formatSubjectForAudit(subject),
		Action:        req.Action,
		Resource:      resource.Canonical(),
		Decision:      string(decisionResp.Effect),
		Reason:        decisionResp.Reason,
		PepID:         pepID,
		SourceIP:      srcIP,
		PolicyVersion: decisionResp.PolicyVersion,
	})

	writeJSON(w, http.StatusOK, authorizeResponse{
		Decision:      string(decisionResp.Effect),
		TTLSeconds:    decisionResp.TTLSeconds,
		Reason:        decisionResp.Reason,
		PolicyVersion: decisionResp.PolicyVersion,
		DecisionID:    decisionResp.DecisionID,
	})
}
