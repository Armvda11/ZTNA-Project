package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"control-plane/internal/api/middleware"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/service/audit"
	"control-plane/internal/service/credentials"
)

// DeviceCertHandler handles X.509 device certificate issuance.
type DeviceCertHandler struct {
	deviceCert *credentials.DeviceCertService
	audit      *audit.Service
}

// NewDeviceCertHandler creates the handler.
func NewDeviceCertHandler(svc *credentials.DeviceCertService, auditSvc *audit.Service) *DeviceCertHandler {
	return &DeviceCertHandler{deviceCert: svc, audit: auditSvc}
}

type deviceCertRequest struct {
	CSRPEM     string `json:"csr_pem"`
	TTLSeconds *int64 `json:"ttl_seconds,omitempty"`
}

type deviceCertResponse struct {
	CertificatePEM string    `json:"certificate_pem"`
	CACertPEM      string    `json:"ca_cert_pem"`
	Serial         string    `json:"serial"`
	ExpiresAt      time.Time `json:"expires_at"`
	Fingerprint    string    `json:"fingerprint"`
}

// Issue handles POST /api/v1/credentials/device-cert
// The caller must be authenticated via OIDC (RequireUser middleware must be
// applied upstream). The request body must contain a PEM-encoded CSR.
func (h *DeviceCertHandler) Issue(w http.ResponseWriter, r *http.Request) {
	subject, ok := middleware.SubjectFromContext(r.Context())
	if !ok {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}

	var req deviceCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if req.CSRPEM == "" {
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

	resp, err := h.deviceCert.IssueDeviceCert(r.Context(), credentials.IssueDeviceCertRequest{
		Subject: subject,
		CSRPEM:  []byte(req.CSRPEM),
		TTL:     ttl,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	sourceIP := extractRemoteIP(r)
	if appendErr := h.audit.Append(r.Context(), model.AuditEvent{
		Subject:  formatSubjectForAudit(subject),
		Action:   "issue_device_cert",
		Resource: "device_cert:" + resp.Serial,
		Decision: "allow",
		Reason:   "issued",
		SourceIP: sourceIP,
	}); appendErr != nil {
		slog.Error("audit append failed", "action", "issue_device_cert", "err", appendErr)
	}

	writeJSON(w, http.StatusOK, deviceCertResponse{
		CertificatePEM: string(resp.CertificatePEM),
		CACertPEM:      string(resp.CACertPEM),
		Serial:         resp.Serial,
		ExpiresAt:      resp.ExpiresAt,
		Fingerprint:    resp.Fingerprint,
	})
}
