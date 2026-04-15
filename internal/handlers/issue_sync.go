package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/services"
)

// IssueSyncHandler handles GitHub Issue sync configuration endpoints.
type IssueSyncHandler struct {
	db                database.Database
	issueSyncService  *scm.IssueSyncService
	permissionService *services.PermissionService
}

// NewIssueSyncHandler creates a new IssueSyncHandler.
func NewIssueSyncHandler(db database.Database, issueSyncService *scm.IssueSyncService, permService *services.PermissionService) *IssueSyncHandler {
	return &IssueSyncHandler{
		db:                db,
		issueSyncService:  issueSyncService,
		permissionService: permService,
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
// the issue sync configuration. Returns the config and true on success; writes an
// error response and returns nil/false on failure.
func (h *IssueSyncHandler) requireSyncConfig(w http.ResponseWriter, r *http.Request) (*models.IssueSyncConfig, bool) {
	_, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return nil, false
	}

	config, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	if config == nil {
		respondNotFound(w, r, "issue sync config")
		return nil, false
	}

	return config, true
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

	// Verify the repository belongs to this workspace
	var repoWorkspaceID int
	err := h.db.QueryRow(`
		SELECT wsc.workspace_id FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wr.id = ?
	`, req.WorkspaceRepositoryID).Scan(&repoWorkspaceID)
	if err != nil || repoWorkspaceID != workspaceID {
		respondNotFound(w, r, "repository")
		return
	}

	// Set defaults
	if req.StatusMapping == "" {
		req.StatusMapping = "{}"
	}
	if req.ReverseStatusMapping == "" {
		req.ReverseStatusMapping = "{}"
	}
	if req.LabelSyncMode == "" {
		req.LabelSyncMode = models.IssueSyncLabelNone
	}
	if req.LabelMappings == "" {
		req.LabelMappings = "[]"
	}
	if req.FilterLabels == "" {
		req.FilterLabels = "[]"
	}
	if req.AssigneeMappings == "" {
		req.AssigneeMappings = "{}"
	}
	if req.MilestoneMappings == "" {
		req.MilestoneMappings = "{}"
	}

	var configID int
	err = h.db.QueryRow(`
		INSERT INTO issue_sync_configs (
			workspace_repository_id, sync_enabled,
			status_mapping, reverse_status_mapping,
			label_sync_mode, label_mappings, filter_labels,
			assignee_mappings, milestone_mappings,
			default_item_type_id, default_priority_id,
			sync_comments, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		req.WorkspaceRepositoryID, req.SyncEnabled,
		req.StatusMapping, req.ReverseStatusMapping,
		req.LabelSyncMode, req.LabelMappings, req.FilterLabels,
		req.AssigneeMappings, req.MilestoneMappings,
		req.DefaultItemTypeID, req.DefaultPriorityID,
		req.SyncComments, user.ID,
	).Scan(&configID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Fetch and return the created config
	config, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, config)
}

// UpdateSyncConfig updates an existing issue sync configuration.
func (h *IssueSyncHandler) UpdateSyncConfig(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := h.requireAuthWorkspaceID(w, r)
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

	_, err = h.db.Exec(`
		UPDATE issue_sync_configs SET
			sync_enabled = ?, status_mapping = ?, reverse_status_mapping = ?,
			label_sync_mode = ?, label_mappings = ?, filter_labels = ?,
			assignee_mappings = ?, milestone_mappings = ?,
			default_item_type_id = ?, default_priority_id = ?,
			sync_comments = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`,
		req.SyncEnabled, req.StatusMapping, req.ReverseStatusMapping,
		req.LabelSyncMode, req.LabelMappings, req.FilterLabels,
		req.AssigneeMappings, req.MilestoneMappings,
		req.DefaultItemTypeID, req.DefaultPriorityID,
		req.SyncComments, config.ID,
	)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	updated, err := h.issueSyncService.GetSyncConfigForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, updated)
}

// DeleteSyncConfig deletes an issue sync configuration and unlinks items.
func (h *IssueSyncHandler) DeleteSyncConfig(w http.ResponseWriter, r *http.Request) {
	config, ok := h.requireSyncConfig(w, r)
	if !ok {
		return
	}

	// Delete cascades to issue_sync_items
	_, err := h.db.Exec("DELETE FROM issue_sync_configs WHERE id = ?", config.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TriggerSync triggers an immediate sync for the workspace's issue sync config.
func (h *IssueSyncHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	config, ok := h.requireSyncConfig(w, r)
	if !ok {
		return
	}

	if err := h.issueSyncService.TriggerSync(r.Context(), config.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{"status": "ok"})
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
	_, _, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return
	}

	repoID, ok := requireRepoIDParam(w, r)
	if !ok {
		return
	}

	labels, err := h.issueSyncService.GetGitHubLabels(r.Context(), repoID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, labels)
}

// GetGitHubMilestones fetches milestones from the linked GitHub repo for the mapping UI.
func (h *IssueSyncHandler) GetGitHubMilestones(w http.ResponseWriter, r *http.Request) {
	_, _, ok := h.requireAuthWorkspaceID(w, r)
	if !ok {
		return
	}

	repoID, ok := requireRepoIDParam(w, r)
	if !ok {
		return
	}

	milestones, err := h.issueSyncService.GetGitHubMilestones(r.Context(), repoID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, milestones)
}
