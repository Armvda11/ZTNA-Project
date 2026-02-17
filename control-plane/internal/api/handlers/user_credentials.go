package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"control-plane/internal/api/middleware"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/service/audit"
	"control-plane/internal/service/credentials"
)

type CredentialsHandler struct {
	creds *credentials.Service
	audit *audit.Service
}

func NewCredentialsHandler(creds *credentials.Service, auditSvc *audit.Service) *CredentialsHandler {
	return &CredentialsHandler{creds: creds, audit: auditSvc}
}

type sshCertRequest struct {
	PublicKey  string   `json:"public_key"`
	TTLSeconds *int64   `json:"ttl_seconds,omitempty"`
	Principals []string `json:"principals,omitempty"`
}

type sshCertResponse struct {
	Certificate string    `json:"certificate"`
	ValidBefore time.Time `json:"valid_before"`
	KeyID       string    `json:"key_id"`
	Principals  []string  `json:"principals"`
}

func (h *CredentialsHandler) IssueSSHCert(w http.ResponseWriter, r *http.Request) {
	subject, ok := middleware.SubjectFromContext(r.Context())
	if !ok {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}

	var req sshCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if req.PublicKey == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}

	var ttl *time.Duration
	if req.TTLSeconds != nil {
		if *req.TTLSeconds <= 0 {
			writeError(w, domainErrors.ErrInvalidInput)
			return
		}
		parsed := time.Duration(*req.TTLSeconds) * time.Second
		ttl = &parsed
	}

	resp, err := h.creds.IssueSSHCert(r.Context(), credentials.IssueRequest{
		Subject:    subject,
		PublicKey:  req.PublicKey,
		TTL:        ttl,
		Principals: req.Principals,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	sourceIP := extractRemoteIP(r)
	_ = h.audit.Append(r.Context(), model.AuditEvent{
		Subject:       formatSubjectForAudit(subject),
		Action:        "issue_ssh_cert",
		Resource:      "ssh_cert",
		Decision:      "allow",
		Reason:        "issued",
		PepID:         "",
		SourceIP:      sourceIP,
		PolicyVersion: 0,
	})

	writeJSON(w, http.StatusOK, sshCertResponse{
		Certificate: resp.Certificate,
		ValidBefore: resp.ValidBefore,
		KeyID:       resp.KeyID,
		Principals:  resp.Principals,
	})
}
