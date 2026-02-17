package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"control-plane/internal/api/middleware"
	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/service/audit"
	"control-plane/internal/service/policy"

	"github.com/go-chi/chi/v5"
)

type AdminPoliciesHandler struct {
	policy *policy.Service
	audit  *audit.Service
}

func NewAdminPoliciesHandler(policySvc *policy.Service, auditSvc *audit.Service) *AdminPoliciesHandler {
	return &AdminPoliciesHandler{policy: policySvc, audit: auditSvc}
}

type createPolicyRequest struct {
	Rules []model.PolicyRule `json:"rules"`
}

type createPolicyResponse struct {
	VersionID int64 `json:"version_id"`
}

func (h *AdminPoliciesHandler) CreateVersion(w http.ResponseWriter, r *http.Request) {
	subject, ok := middleware.SubjectFromContext(r.Context())
	if !ok {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}

	var req createPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if len(req.Rules) == 0 {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}

	id, err := h.policy.CreateVersion(r.Context(), subject.Username, req.Rules)
	if err != nil {
		writeError(w, err)
		return
	}

	_ = h.audit.Append(r.Context(), model.AuditEvent{
		Subject:  subject.Username,
		Action:   "policy_create",
		Resource: "policy",
		Decision: "allow",
		Reason:   "created",
		PepID:    "",
		SourceIP: r.RemoteAddr,
	})

	writeJSON(w, http.StatusCreated, createPolicyResponse{VersionID: id})
}

func (h *AdminPoliciesHandler) ActivateVersion(w http.ResponseWriter, r *http.Request) {
	subject, ok := middleware.SubjectFromContext(r.Context())
	if !ok {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}

	rawID := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}

	if err := h.policy.ActivateVersion(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}

	_ = h.audit.Append(r.Context(), model.AuditEvent{
		Subject:  subject.Username,
		Action:   "policy_activate",
		Resource: "policy",
		Decision: "allow",
		Reason:   "activated",
		PepID:    "",
		SourceIP: r.RemoteAddr,
	})

	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (h *AdminPoliciesHandler) ActivePolicy(w http.ResponseWriter, r *http.Request) {
	snapshot, err := h.policy.GetActive(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, snapshot)
}
