package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"control-plane/internal/api/middleware"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/service/audit"
	"control-plane/internal/service/credentials"

	"github.com/go-chi/chi/v5"
)

// AdminDeviceCertsHandler handles administrative operations on device certificates.
type AdminDeviceCertsHandler struct {
	deviceCert *credentials.DeviceCertService
	audit      *audit.Service
}

// NewAdminDeviceCertsHandler creates the handler.
func NewAdminDeviceCertsHandler(svc *credentials.DeviceCertService, auditSvc *audit.Service) *AdminDeviceCertsHandler {
	return &AdminDeviceCertsHandler{deviceCert: svc, audit: auditSvc}
}

type revokeRequest struct {
	Reason string `json:"reason"`
}

// Revoke handles DELETE /api/v1/admin/device-certs/{serial}
// Marks a device certificate as revoked. The updated CRL is served at
// /pki/device-ca/crl.
func (h *AdminDeviceCertsHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	subject, ok := middleware.SubjectFromContext(r.Context())
	if !ok {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}

	serial := chi.URLParam(r, "serial")
	if serial == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}

	var req revokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Reason == "" {
		req.Reason = "admin-revocation"
	}

	if err := h.deviceCert.RevokeDeviceCert(r.Context(), serial, req.Reason); err != nil {
		writeError(w, err)
		return
	}

	if appendErr := h.audit.Append(r.Context(), model.AuditEvent{
		Subject:  formatSubjectForAudit(subject),
		Action:   "revoke_device_cert",
		Resource: "device_cert:" + serial,
		Decision: "allow",
		Reason:   req.Reason,
		SourceIP: extractRemoteIP(r),
	}); appendErr != nil {
		slog.Error("audit append failed", "action", "revoke_device_cert", "err", appendErr)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "serial": serial})
}
