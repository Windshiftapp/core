package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/services"
	"windshift/internal/services/actiontemplates"
)

// ActionTemplatesHandler exposes the embedded action-template registry and
// the "apply this template to a workspace" instantiation endpoint. The
// registry itself is read-only and shipped with the binary; this handler
// has no create/update/delete surface in v1.
type ActionTemplatesHandler struct {
	db              database.Database
	templateService *services.ActionTemplateService
	actionService   *services.ActionService
	keyCache        *WorkspaceKeyCache
}

// NewActionTemplatesHandler wires the handler against the embedded registry
// and the existing action cache invalidation hook.
func NewActionTemplatesHandler(db database.Database, actionService *services.ActionService, keyCache *WorkspaceKeyCache) *ActionTemplatesHandler {
	return &ActionTemplatesHandler{
		db:              db,
		templateService: services.NewActionTemplateService(db),
		actionService:   actionService,
		keyCache:        keyCache,
	}
}

// ListTemplates returns every embedded action template. Authenticated
// metadata only — actual application requires action.create on the
// destination workspace and is gated by CreateActionFromTemplate.
type templateSummary struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	TriggerType string `json:"trigger_type"`
	NodeCount   int    `json:"node_count"`
}

// ListTemplates returns the registry as a flat array, sorted as embedded.
func (h *ActionTemplatesHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	all := actiontemplates.Registry()
	out := make([]templateSummary, 0, len(all))
	for _, t := range all {
		out = append(out, templateSummary{
			Key:         t.Key,
			Name:        t.Name,
			Description: t.Description,
			Category:    t.Category,
			TriggerType: string(t.TriggerType),
			NodeCount:   len(t.Nodes),
		})
	}
	respondJSONOK(w, out)
}

// CreateActionFromTemplate snapshot-copies a template into the workspace
// as a new Action. Permission: action.manage on the workspace (matches the
// existing CreateAction route's gate). Returns 201 with the materialized
// action's summary on success.
func (h *ActionTemplatesHandler) CreateActionFromTemplate(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return
	}

	templateKey := r.PathValue("templateKey")
	if templateKey == "" {
		respondValidationError(w, r, "templateKey is required")
		return
	}
	if _, exists := actiontemplates.Get(templateKey); !exists {
		respondNotFound(w, r, "action template")
		return
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	result, err := h.templateService.ApplyToWorkspace(r.Context(), templateKey, workspaceID, currentUser.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if h.actionService != nil {
		h.actionService.InvalidateWorkspaceCache(workspaceID)
	}

	logAuditWithDetails(h.db, r, currentUser, logger.ActionAutomationCreate, logger.ResourceAutomation, &result.ActionID, result.Name, map[string]interface{}{
		"template_key": result.TemplateKey,
		"context":      "from_template",
	})

	respondJSONCreated(w, result)
}
