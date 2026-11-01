package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"windshift/internal/llm"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// AdminAgentTemplateCatalogHandler backs the system-admin "Agent Templates"
// surface (WI-922). It manages the global override rows that, when enabled,
// win over the embedded Agent Studio creation catalog defaults. Every
// endpoint is gated on system-admin and emits an audit entry.
type AdminAgentTemplateCatalogHandler struct {
	repo              *repository.AgentTemplateCatalogRepository
	permissionService *services.PermissionService
	auditor           *logger.Auditor
	defaults          llm.TemplateSource
}

// NewAdminAgentTemplateCatalogHandler wires the handler.
func NewAdminAgentTemplateCatalogHandler(
	repo *repository.AgentTemplateCatalogRepository,
	permissionService *services.PermissionService,
	auditor *logger.Auditor,
) *AdminAgentTemplateCatalogHandler {
	return &AdminAgentTemplateCatalogHandler{
		repo:              repo,
		permissionService: permissionService,
		auditor:           auditor,
	}
}

// SetDefaults wires the embedded default creation catalog so admins can seed
// an override that overwrites a built-in template (WI-922).
func (h *AdminAgentTemplateCatalogHandler) SetDefaults(defaults llm.TemplateSource) {
	h.defaults = defaults
}

type agentTemplateCatalogResponse struct {
	ID              int64                   `json:"id"`
	TemplateKey     string                  `json:"template_key"`
	Name            string                  `json:"name"`
	DefaultType     models.AgentProfileType `json:"default_type"`
	Instructions    string                  `json:"instructions"`
	Enabled         bool                    `json:"enabled"`
	CreatedByUserID *int                    `json:"created_by_user_id,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
}

func toAgentTemplateCatalogResponse(e *models.AgentTemplateCatalogEntry) agentTemplateCatalogResponse {
	return agentTemplateCatalogResponse{
		ID:              e.ID,
		TemplateKey:     e.TemplateKey,
		Name:            e.Name,
		DefaultType:     e.DefaultType,
		Instructions:    e.Instructions,
		Enabled:         e.Enabled,
		CreatedByUserID: e.CreatedByUserID,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}

type agentTemplateCatalogCreateRequest struct {
	TemplateKey  string                  `json:"template_key"`
	Name         string                  `json:"name"`
	DefaultType  models.AgentProfileType `json:"default_type,omitempty"`
	Instructions string                  `json:"instructions"`
	Enabled      *bool                   `json:"enabled,omitempty"`
}

type agentTemplateCatalogUpdateRequest struct {
	Name         *string                  `json:"name,omitempty"`
	DefaultType  *models.AgentProfileType `json:"default_type,omitempty"`
	Instructions *string                  `json:"instructions,omitempty"`
	Enabled      *bool                    `json:"enabled,omitempty"`
}

// DefaultTemplates returns the embedded default creation catalog so admins
// can seed an override that overwrites a built-in template. The merged
// templates endpoint consumed by workspace admins reflects these defaults
// overlaid by the enabled override rows.
func (h *AdminAgentTemplateCatalogHandler) DefaultTemplates(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	if h.defaults == nil {
		respondServiceUnavailable(w, r, "Agent Studio templates are not configured")
		return
	}
	respondJSONOK(w, h.defaults.AgentTemplates())
}

// ListTemplates returns all catalog override rows.
func (h *AdminAgentTemplateCatalogHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	entries, err := h.repo.List(r.Context())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	out := make([]agentTemplateCatalogResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, toAgentTemplateCatalogResponse(e))
	}
	respondJSONOK(w, out)
}

// GetTemplate returns a single override row by id.
func (h *AdminAgentTemplateCatalogHandler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	entry, err := h.repo.Get(r.Context(), int64(id))
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "agent_template")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, toAgentTemplateCatalogResponse(entry))
}

// CreateTemplate inserts a new override row.
func (h *AdminAgentTemplateCatalogHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	req, ok := decodeJSON[agentTemplateCatalogCreateRequest](w, r)
	if !ok {
		return
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.TemplateKey, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField},
	)
	req.TemplateKey = strings.TrimSpace(req.TemplateKey)
	req.Name = strings.TrimSpace(req.Name)
	if req.TemplateKey == "" {
		respondValidationError(w, r, "template_key is required")
		return
	}
	if req.Name == "" {
		respondValidationError(w, r, "name is required")
		return
	}
	if req.DefaultType == "" {
		req.DefaultType = models.AgentProfileStandard
	}
	if req.DefaultType != models.AgentProfileStandard && req.DefaultType != models.AgentProfileCoding {
		respondValidationError(w, r, "default_type must be 'standard' or 'coding'")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	createdBy := user.ID
	entry := &models.AgentTemplateCatalogEntry{
		TemplateKey:     req.TemplateKey,
		Name:            req.Name,
		DefaultType:     req.DefaultType,
		Instructions:    strings.TrimSpace(req.Instructions),
		Enabled:         enabled,
		CreatedByUserID: &createdBy,
	}
	created, err := h.repo.Create(r.Context(), entry)
	if errors.Is(err, repository.ErrDuplicateEntry) {
		respondConflict(w, r, "a template with this key already exists")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentTemplateCatalogCreate, logger.ResourceAgentTemplateCatalog, nil,
		created.TemplateKey, map[string]any{
			"template_key": created.TemplateKey,
			"enabled":      created.Enabled,
		})
	respondJSON(w, http.StatusCreated, toAgentTemplateCatalogResponse(created))
}

// UpdateTemplate applies a partial update to an override row.
func (h *AdminAgentTemplateCatalogHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	req, ok := decodeJSON[agentTemplateCatalogUpdateRequest](w, r)
	if !ok {
		return
	}
	current, err := h.repo.Get(r.Context(), int64(id))
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "agent_template")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if req.Name != nil {
		sanitize.Apply(req.Name, sanitize.PlainTextField)
		if *req.Name == "" {
			respondValidationError(w, r, "name cannot be empty")
			return
		}
		current.Name = *req.Name
	}
	if req.DefaultType != nil {
		if *req.DefaultType != models.AgentProfileStandard && *req.DefaultType != models.AgentProfileCoding {
			respondValidationError(w, r, "default_type must be 'standard' or 'coding'")
			return
		}
		current.DefaultType = *req.DefaultType
	}
	if req.Instructions != nil {
		current.Instructions = strings.TrimSpace(*req.Instructions)
	}
	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	}
	if err := h.repo.Update(r.Context(), current); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentTemplateCatalogUpdate, logger.ResourceAgentTemplateCatalog, nil,
		current.TemplateKey, map[string]any{
			"id":      current.ID,
			"enabled": current.Enabled,
		})
	respondJSONOK(w, toAgentTemplateCatalogResponse(current))
}

// DeleteTemplate removes an override row, restoring the embedded default.
func (h *AdminAgentTemplateCatalogHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireSystemAdmin(w, r, user.ID, h.permissionService) {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	entry, err := h.repo.Get(r.Context(), int64(id))
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "agent_template")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if err := h.repo.Delete(r.Context(), int64(id)); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "agent_template")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentTemplateCatalogDelete, logger.ResourceAgentTemplateCatalog, nil,
		entry.TemplateKey, map[string]any{
			"id": id,
		})
	w.WriteHeader(http.StatusNoContent)
}
