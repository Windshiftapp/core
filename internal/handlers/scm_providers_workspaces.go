package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/utils"
)

// requireSCMProviderExists parses the provider "id" route param and verifies
// the provider exists. It writes an HTTP error and returns ok=false on failure.
func (h *SCMProviderHandler) requireSCMProviderExists(w http.ResponseWriter, r *http.Request) (int, bool) {
	providerID, ok := requireIDParam(w, r, "id")
	if !ok {
		return 0, false
	}

	_, err := h.getProviderByID(providerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return 0, false
	}

	return providerID, true
}

// GetProviderAllowedWorkspaces lists all workspaces allowed to use an SCM provider
func (h *SCMProviderHandler) GetProviderAllowedWorkspaces(w http.ResponseWriter, r *http.Request) {
	providerID, ok := h.requireSCMProviderExists(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT a.id, a.provider_id, a.workspace_id, a.created_at, a.created_by,
			   w.name as workspace_name, w.key as workspace_key
		FROM scm_provider_workspace_allowlist a
		JOIN workspaces w ON a.workspace_id = w.id
		WHERE a.provider_id = ?
		ORDER BY w.name
	`, providerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var allowlist []models.SCMProviderWorkspaceAllowlist
	for rows.Next() {
		var entry models.SCMProviderWorkspaceAllowlist
		var createdBy sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.ProviderID, &entry.WorkspaceID,
			&entry.CreatedAt, &createdBy, &entry.WorkspaceName, &entry.WorkspaceKey); err != nil {
			slog.Error("failed to scan allowlist entry", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}
		if createdBy.Valid {
			createdByInt := int(createdBy.Int64)
			entry.CreatedBy = &createdByInt
		}
		allowlist = append(allowlist, entry)
	}

	if allowlist == nil {
		allowlist = []models.SCMProviderWorkspaceAllowlist{}
	}

	respondJSONOK(w, allowlist)
}

// AddWorkspaceToProviderAllowlist adds a workspace to the provider's allowlist
func (h *SCMProviderHandler) AddWorkspaceToProviderAllowlist(w http.ResponseWriter, r *http.Request) {
	providerID, ok := h.requireSCMProviderExists(w, r)
	if !ok {
		return
	}

	var req struct {
		WorkspaceID int `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.WorkspaceID == 0 {
		respondValidationError(w, r, "workspace_id is required")
		return
	}

	// Check if workspace exists
	var workspaceExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", req.WorkspaceID).Scan(&workspaceExists)
	if err != nil || !workspaceExists {
		respondNotFound(w, r, "workspace")
		return
	}

	// Get user ID from context if available
	var createdBy interface{}
	if user := utils.GetCurrentUser(r); user != nil {
		createdBy = user.ID
	}

	// Insert the allowlist entry
	_, err = h.db.Exec(`
		INSERT INTO scm_provider_workspace_allowlist (provider_id, workspace_id, created_by)
		VALUES (?, ?, ?)
	`, providerID, req.WorkspaceID, createdBy)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Workspace is already in the allowlist")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// RemoveWorkspaceFromProviderAllowlist removes a workspace from the provider's allowlist
func (h *SCMProviderHandler) RemoveWorkspaceFromProviderAllowlist(w http.ResponseWriter, r *http.Request) {
	providerID, ok := h.requireSCMProviderExists(w, r)
	if !ok {
		return
	}

	workspaceID, ok := requireIDParam(w, r, "workspace_id")
	if !ok {
		return
	}

	result, err := h.db.Exec(`
		DELETE FROM scm_provider_workspace_allowlist
		WHERE provider_id = ? AND workspace_id = ?
	`, providerID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "allowlist_entry")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateProviderAllowedWorkspaces replaces the entire allowlist for a provider
func (h *SCMProviderHandler) UpdateProviderAllowedWorkspaces(w http.ResponseWriter, r *http.Request) {
	providerID, ok := h.requireSCMProviderExists(w, r)
	if !ok {
		return
	}

	var req struct {
		WorkspaceIDs []int `json:"workspace_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Get user ID from context if available
	var createdBy interface{}
	if user := utils.GetCurrentUser(r); user != nil {
		createdBy = user.ID
	}

	// Start a transaction to replace the entire allowlist
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete all existing entries for this provider
	_, err = tx.Exec("DELETE FROM scm_provider_workspace_allowlist WHERE provider_id = ?", providerID)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			slog.Error("failed to rollback transaction", slog.String("component", "scm"), slog.Any("error", rbErr))
		}
		respondInternalError(w, r, err)
		return
	}

	// Insert new entries
	for _, workspaceID := range req.WorkspaceIDs {
		_, err = tx.Exec(`
			INSERT INTO scm_provider_workspace_allowlist (provider_id, workspace_id, created_by)
			VALUES (?, ?, ?)
		`, providerID, workspaceID, createdBy)
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Error("failed to rollback transaction", slog.String("component", "scm"), slog.Any("error", rbErr))
			}
			respondInternalError(w, r, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated allowlist
	h.GetProviderAllowedWorkspaces(w, r)
}

// IsWorkspaceAllowedForProvider checks if a workspace is allowed to use an SCM provider
// This is a helper method used by other handlers for enforcement
func (h *SCMProviderHandler) IsWorkspaceAllowedForProvider(providerID, workspaceID int) (bool, error) {
	provider, err := h.getProviderByID(providerID)
	if err != nil {
		return false, err
	}

	// If unrestricted, all workspaces are allowed
	if provider.WorkspaceRestrictionMode == "unrestricted" {
		return true, nil
	}

	// Check if workspace is in the allowlist
	var exists bool
	err = h.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM scm_provider_workspace_allowlist
			WHERE provider_id = ? AND workspace_id = ?
		)
	`, providerID, workspaceID).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// GitHubAppInstallation represents a GitHub App installation for discovery
type GitHubAppInstallation struct {
	ID               int64  `json:"id"`
	AccountLogin     string `json:"account_login"`
	AccountType      string `json:"account_type"`
	AccountID        int64  `json:"account_id"`
	AccountAvatarURL string `json:"account_avatar_url,omitempty"`
}

// DiscoverGitHubAppInstallationsRequest represents request for discovering installations
type DiscoverGitHubAppInstallationsRequest struct {
	AppID      string `json:"app_id"`
	PrivateKey string `json:"private_key"`
}

// DiscoverGitHubAppInstallations discovers GitHub App installations for configuration
// POST /api/scm-providers/github-app/discover-installations
func (h *SCMProviderHandler) DiscoverGitHubAppInstallations(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[DiscoverGitHubAppInstallationsRequest](w, r)
	if !ok {
		return
	}

	if req.AppID == "" || req.PrivateKey == "" {
		respondValidationError(w, r, "app_id and private_key are required")
		return
	}

	// Create GitHub provider with App credentials for discovery
	cfg := scm.ProviderConfig{
		ProviderType:        models.SCMProviderTypeGitHub,
		AuthMethod:          models.SCMAuthMethodGitHubApp,
		GitHubAppID:         req.AppID,
		GitHubAppPrivateKey: req.PrivateKey,
	}

	provider, err := scm.NewGitHubProvider(cfg)
	if err != nil {
		slog.Error("failed to create GitHub provider for discovery", slog.String("component", "scm"), slog.Any("error", err))
		respondJSONOK(w, map[string]interface{}{
			"success":       false,
			"error":         "Failed to initialize GitHub App: " + err.Error(),
			"installations": []interface{}{},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	installations, err := provider.ListAppInstallations(ctx)
	if err != nil {
		slog.Error("failed to discover GitHub App installations", slog.String("component", "scm"), slog.Any("error", err))
		respondJSONOK(w, map[string]interface{}{
			"success":       false,
			"error":         "Failed to list installations: " + err.Error(),
			"installations": []interface{}{},
		})
		return
	}

	// Convert to response format
	result := make([]GitHubAppInstallation, 0, len(installations))
	for _, inst := range installations {
		result = append(result, GitHubAppInstallation{
			ID:               inst.ID,
			AccountLogin:     inst.AccountLogin,
			AccountType:      inst.AccountType,
			AccountID:        inst.AccountID,
			AccountAvatarURL: inst.AccountAvatarURL,
		})
	}

	respondJSONOK(w, map[string]interface{}{
		"success":       true,
		"installations": result,
	})
}

// RefreshGitHubAppInstallation refreshes the installation_id for a provider using org_id
// POST /api/scm-providers/{id}/github-app/refresh-installation
func (h *SCMProviderHandler) RefreshGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Get provider details
	var authMethod models.SCMAuthMethod
	var ghAppID, ghAppKeyEnc sql.NullString
	var ghOrgID sql.NullInt64

	err = h.db.QueryRow(`
		SELECT auth_method, github_app_id, github_app_private_key_encrypted, github_org_id
		FROM scm_providers WHERE id = ?
	`, id).Scan(&authMethod, &ghAppID, &ghAppKeyEnc, &ghOrgID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if authMethod != models.SCMAuthMethodGitHubApp {
		respondBadRequest(w, r, "Provider does not use GitHub App authentication")
		return
	}

	if !ghAppID.Valid || !ghAppKeyEnc.Valid || !ghOrgID.Valid {
		respondBadRequest(w, r, "GitHub App not fully configured (missing app_id, private_key, or org_id)")
		return
	}

	// Decrypt private key
	privateKey, err := h.encryption.Decrypt(ghAppKeyEnc.String)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create provider and find installation for org
	cfg := scm.ProviderConfig{
		ProviderType:        models.SCMProviderTypeGitHub,
		AuthMethod:          models.SCMAuthMethodGitHubApp,
		GitHubAppID:         ghAppID.String,
		GitHubAppPrivateKey: privateKey,
	}

	provider, err := scm.NewGitHubProvider(cfg)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to initialize GitHub App: %w", err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	installations, err := provider.ListAppInstallations(ctx)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to list installations: %w", err))
		return
	}

	// Find installation matching our org_id
	var foundInstallation *scm.GitHubAppInstallation
	for i := range installations {
		if installations[i].AccountID == ghOrgID.Int64 {
			foundInstallation = &installations[i]
			break
		}
	}

	if foundInstallation == nil {
		respondJSONOK(w, map[string]interface{}{
			"success": false,
			"error":   "App is no longer installed for this organization",
		})
		return
	}

	// Update installation_id
	_, err = h.db.Exec(`
		UPDATE scm_providers SET
			github_app_installation_id = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, fmt.Sprintf("%d", foundInstallation.ID), id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success":         true,
		"installation_id": foundInstallation.ID,
		"account_login":   foundInstallation.AccountLogin,
	})
}
