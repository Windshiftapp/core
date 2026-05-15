package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/services"
)

// IssueSyncHandler handles GitHub Issue sync configuration endpoints.
type IssueSyncHandler struct {
	issueSyncService  *scm.IssueSyncService
	permissionService *services.PermissionService
	auditor           *logger.Auditor
}

// NewIssueSyncHandler creates a new IssueSyncHandler.
func NewIssueSyncHandler(issueSyncService *scm.IssueSyncService, permService *services.PermissionService, auditor *logger.Auditor) *IssueSyncHandler {
	return &IssueSyncHandler{
		issueSyncService:  issueSyncService,
		permissionService: permService,
		auditor:           auditor,
	}
}

// requireAuthWorkspaceID authenticates the request and parses the "id" path
// value as a workspace ID. Returns the user, workspace ID, and true on success;
// writes an error response and returns zero values/false on failure.
func (h *IssueSyncHandler) requireAuthWorkspaceID(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, 0, false
	}

	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return nil, 0, false
	}

	return user, workspaceID, true
}

// requireSyncConfig authenticates the request, parses the workspace ID, and loads
// the issue sync configuration. Returns the user, config and true on success;
// writes an error response and returns nil/false on failure.
func (h *IssueSyncHandler) requireSyncConfig(w http.ResponseWriter, r *http.Request) (*models.User, *models.IssueSyncConfig, bool) {
	user, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return nil, nil, false
	}

	config, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, nil, false
	}
	if config == nil {
		respondNotFound(w, r, "issue sync config")
		return nil, nil, false
	}

	return user, config, true
}

// GetSyncConfig returns the issue sync configuration for a workspace.
func (h *IssueSyncHandler) GetSyncConfig(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return
	}

	config, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if config == nil {
		respondJSONOK(w, nil)
		return
	}

	respondJSONOK(w, config)
}

// CreateSyncConfig creates a new issue sync configuration.
func (h *IssueSyncHandler) CreateSyncConfig(w http.ResponseWriter, r *http.Request) {
	user, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return
	}

	var req models.IssueSyncConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.WorkspaceRepositoryID == 0 {
		respondValidationError(w, r, "workspace_repository_id is required")
		return
	}

	belongs, err := h.issueSyncService.VerifyRepositoryInWorkspace(r.Context(), req.WorkspaceRepositoryID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !belongs {
		respondNotFound(w, r, "repository")
		return
	}

	if _, err := h.issueSyncService.CreateSyncConfig(r.Context(), user.ID, req); err != nil {
		if errors.Is(err, scm.ErrSyncConfigExists) {
			respondConflict(w, r, "issue sync config already exists for this workspace")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Fetch and return the created config
	config, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.audit(r, user, logger.ActionIssueSyncConfigCreate, config)
	respondJSONCreated(w, config)
}

// UpdateSyncConfig updates an existing issue sync configuration.
func (h *IssueSyncHandler) UpdateSyncConfig(w http.ResponseWriter, r *http.Request) {
	user, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return
	}

	var req models.IssueSyncConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Find the existing config
	config, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if config == nil {
		respondNotFound(w, r, "issue sync config")
		return
	}

	// Set defaults for empty values
	if req.StatusMapping == "" {
		req.StatusMapping = config.StatusMapping
	}
	if req.ReverseStatusMapping == "" {
		req.ReverseStatusMapping = config.ReverseStatusMapping
	}
	if req.LabelSyncMode == "" {
		req.LabelSyncMode = config.LabelSyncMode
	}
	if req.LabelMappings == "" {
		req.LabelMappings = config.LabelMappings
	}
	if req.FilterLabels == "" {
		req.FilterLabels = config.FilterLabels
	}
	if req.AssigneeMappings == "" {
		req.AssigneeMappings = config.AssigneeMappings
	}
	if req.MilestoneMappings == "" {
		req.MilestoneMappings = config.MilestoneMappings
	}

	if err := h.issueSyncService.UpdateSyncConfig(r.Context(), config.ID, req); err != nil {
		respondInternalError(w, r, err)
		return
	}

	updated, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.audit(r, user, logger.ActionIssueSyncConfigUpdate, updated)
	respondJSONOK(w, updated)
}

// DeleteSyncConfig deletes an issue sync configuration and unlinks items.
func (h *IssueSyncHandler) DeleteSyncConfig(w http.ResponseWriter, r *http.Request) {
	user, config, ok := h.requireSyncConfig(w, r)
	if !ok {
		return
	}

	if err := h.issueSyncService.DeleteSyncConfig(r.Context(), config.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.audit(r, user, logger.ActionIssueSyncConfigDelete, config)
	w.WriteHeader(http.StatusNoContent)
}

// TriggerSync triggers an immediate sync for the workspace's issue sync config.
func (h *IssueSyncHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	user, config, ok := h.requireSyncConfig(w, r)
	if !ok {
		return
	}

	if err := h.issueSyncService.TriggerSync(r.Context(), config.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.audit(r, user, logger.ActionIssueSyncTrigger, config)
	respondJSONOK(w, map[string]string{"status": "ok"})
}

func (h *IssueSyncHandler) audit(r *http.Request, user *models.User, action string, config *models.IssueSyncConfig) {
	if h.auditor == nil || user == nil || config == nil {
		return
	}
	h.auditor.LogWithDetails(r, user, action, logger.ResourceIssueSyncConfig, &config.ID, config.RepositoryName, map[string]interface{}{
		"workspace_id":            config.WorkspaceID,
		"workspace_repository_id": config.WorkspaceRepositoryID,
		"sync_enabled":            config.SyncEnabled,
		"sync_comments":           config.SyncComments,
		"label_sync_mode":         config.LabelSyncMode,
		"default_item_type_id":    config.DefaultItemTypeID,
		"default_priority_id":     config.DefaultPriorityID,
	})
}

// GetSyncStatus returns the sync status for the workspace's config.
func (h *IssueSyncHandler) GetSyncStatus(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return
	}

	config, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if config == nil {
		respondJSONOK(w, map[string]interface{}{"configured": false})
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"configured":        true,
		"sync_enabled":      config.SyncEnabled,
		"last_sync_at":      config.LastFullSyncAt,
		"last_sync_error":   config.LastSyncError,
		"synced_item_count": config.SyncedItemCount,
	})
}

// GetSyncedItems returns the list of synced items with GitHub links.
func (h *IssueSyncHandler) GetSyncedItems(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return
	}

	config, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if config == nil {
		respondJSONOK(w, []interface{}{})
		return
	}

	items, err := h.issueSyncService.GetSyncedItems(r.Context(), config.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, items)
}

// requireRepoIDParam parses and returns the required "repository_id" query
// parameter. On failure it writes an error response and returns 0, false.
func requireRepoIDParam(w http.ResponseWriter, r *http.Request) (int, bool) {
	repoIDStr := r.URL.Query().Get("repository_id")
	if repoIDStr == "" {
		respondValidationError(w, r, "repository_id query parameter required")
		return 0, false
	}
	repoID, err := strconv.Atoi(repoIDStr)
	if err != nil {
		respondValidationError(w, r, "invalid repository_id")
		return 0, false
	}
	return repoID, true
}

// GetGitHubLabels fetches labels from the linked GitHub repo for the mapping UI.
func (h *IssueSyncHandler) GetGitHubLabels(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return
	}

	repoID, ok := requireRepoIDParam(w, r)
	if !ok {
		return
	}

	labels, err := h.issueSyncService.GetGitHubLabels(r.Context(), workspaceID, repoID)
	if err != nil {
		if errors.Is(err, scm.ErrRepositoryNotInWorkspace) {
			respondNotFound(w, r, "repository")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, labels)
}

// GetGitHubMilestones fetches milestones from the linked GitHub repo for the mapping UI.
func (h *IssueSyncHandler) GetGitHubMilestones(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return
	}

	repoID, ok := requireRepoIDParam(w, r)
	if !ok {
		return
	}

	milestones, err := h.issueSyncService.GetGitHubMilestones(r.Context(), workspaceID, repoID)
	if err != nil {
		if errors.Is(err, scm.ErrRepositoryNotInWorkspace) {
			respondNotFound(w, r, "repository")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, milestones)
}
