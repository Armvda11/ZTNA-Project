package handlers

import (
	"encoding/json"
	"net/http"

	domainErrors "control-plane/internal/domain/errors"
	"control-plane/internal/domain/model"
	"control-plane/internal/api/middleware"
	"control-plane/internal/service/resource"

	"github.com/go-chi/chi/v5"
)

// ResourceHandler serves user and admin resource endpoints.
type ResourceHandler struct {
	svc *resource.Service
}

func NewResourceHandler(svc *resource.Service) *ResourceHandler {
	return &ResourceHandler{svc: svc}
}

// listResourceRequest is the JSON body for listing filtered resources.
type listResourceResponse struct {
	Resources []resourceDTO `json:"resources"`
}

type resourceDTO struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Type        string   `json:"type"`
	AccessMode  string   `json:"access_mode"`
	Description string   `json:"description"`
	GatewayID   string   `json:"gateway_id"`
	GroupMatch  []string `json:"group_match,omitempty"`
	Backend     string   `json:"backend,omitempty"`
}

func toDTO(r model.PublishedResource, includeBackend bool) resourceDTO {
	dto := resourceDTO{
		Name:        r.Name,
		DisplayName: r.DisplayName,
		Type:        r.Type,
		AccessMode:  string(r.AccessMode),
		Description: r.Description,
		GatewayID:   r.GatewayID,
	}
	if includeBackend {
		dto.GroupMatch = r.GroupMatch
		dto.Backend = r.Backend
	}
	return dto
}

// ListForUser returns resources visible to the authenticated user based on their groups.
// GET /api/v1/resources
func (h *ResourceHandler) ListForUser(w http.ResponseWriter, r *http.Request) {
	subject, ok := middleware.SubjectFromContext(r.Context())
	if !ok {
		writeError(w, domainErrors.ErrUnauthorized)
		return
	}
	resources, err := h.svc.ListForGroups(r.Context(), subject.Groups)
	if err != nil {
		writeError(w, err)
		return
	}
	dtos := make([]resourceDTO, 0, len(resources))
	for _, res := range resources {
		dtos = append(dtos, toDTO(res, false))
	}
	writeJSON(w, http.StatusOK, listResourceResponse{Resources: dtos})
}

// AdminList returns all resources with full details.
// GET /api/v1/admin/resources
func (h *ResourceHandler) AdminList(w http.ResponseWriter, r *http.Request) {
	resources, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	dtos := make([]resourceDTO, 0, len(resources))
	for _, res := range resources {
		dtos = append(dtos, toDTO(res, true))
	}
	writeJSON(w, http.StatusOK, listResourceResponse{Resources: dtos})
}

type createResourceRequest struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Type        string   `json:"type"`
	Backend     string   `json:"backend"`
	GatewayID   string   `json:"gateway_id"`
	GroupMatch  []string `json:"group_match"`
	AccessMode  string   `json:"access_mode"`
	Description string   `json:"description"`
}

// AdminCreate creates a new published resource.
// POST /api/v1/admin/resources
func (h *ResourceHandler) AdminCreate(w http.ResponseWriter, r *http.Request) {
	var req createResourceRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if req.Name == "" || req.Type == "" || req.Backend == "" || req.AccessMode == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}

	res := model.PublishedResource{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Type:        req.Type,
		Backend:     req.Backend,
		GatewayID:   req.GatewayID,
		GroupMatch:  req.GroupMatch,
		AccessMode:  model.AccessMode(req.AccessMode),
		Description: req.Description,
	}
	if err := h.svc.Create(r.Context(), res); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "created", "name": req.Name})
}

// AdminUpdate updates an existing resource.
// PUT /api/v1/admin/resources/{name}
func (h *ResourceHandler) AdminUpdate(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	var req createResourceRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}

	res := model.PublishedResource{
		Name:        name,
		DisplayName: req.DisplayName,
		Type:        req.Type,
		Backend:     req.Backend,
		GatewayID:   req.GatewayID,
		GroupMatch:  req.GroupMatch,
		AccessMode:  model.AccessMode(req.AccessMode),
		Description: req.Description,
	}
	if err := h.svc.Update(r.Context(), res); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated", "name": name})
}

// AdminDelete removes a published resource.
// DELETE /api/v1/admin/resources/{name}
func (h *ResourceHandler) AdminDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, domainErrors.ErrInvalidInput)
		return
	}
	if err := h.svc.Delete(r.Context(), name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}
