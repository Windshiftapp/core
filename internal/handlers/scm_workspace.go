package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/sso"
)

// SCMWorkspaceHandler handles workspace SCM connection endpoints
type SCMWorkspaceHandler struct {
	db                 database.Database
	encryption         *sso.SecretEncryption
	providerHandler    *SCMProviderHandler
	credentialResolver *scm.CredentialResolver
}

// WorkspaceSCMConnectionResponse represents a workspace SCM connection for API responses
type WorkspaceSCMConnectionResponse struct {
	ID                   int                    `json:"id"`
	WorkspaceID          int                    `json:"workspace_id"`
	SCMProviderID        int                    `json:"scm_provider_id"`
	ProviderName         string                 `json:"provider_name"`
	ProviderType         models.SCMProviderType `json:"provider_type"`
	ProviderSlug         string                 `json:"provider_slug"`
	Enabled              bool                   `json:"enabled"`
	DefaultBranchPattern string                 `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       string                 `json:"item_key_pattern,omitempty"`
	RepositoryCount      int                    `json:"repository_count"`
	CreatedBy            *int                   `json:"created_by,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
}

// WorkspaceRepositoryResponse represents a linked repository for API responses
type WorkspaceRepositoryResponse struct {
	ID                       int        `json:"id"`
	WorkspaceSCMConnectionID int        `json:"workspace_scm_connection_id"`
	RepositoryExternalID     string     `json:"repository_external_id"`
	RepositoryName           string     `json:"repository_name"`
	RepositoryURL            string     `json:"repository_url"`
	DefaultBranch            string     `json:"default_branch"`
	IsActive                 bool       `json:"is_active"`
	LastSyncedAt             *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

// CreateWorkspaceSCMConnectionRequest represents a request to create a connection
type CreateWorkspaceSCMConnectionRequest struct {
	SCMProviderID        int    `json:"scm_provider_id"`
	DefaultBranchPattern string `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       string `json:"item_key_pattern,omitempty"`
}

// UpdateWorkspaceSCMConnectionRequest represents a request to update a connection
type UpdateWorkspaceSCMConnectionRequest struct {
	Enabled              *bool  `json:"enabled,omitempty"`
	DefaultBranchPattern string `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       string `json:"item_key_pattern,omitempty"`
}

// LinkRepositoryRequest represents a request to link a repository
type LinkRepositoryRequest struct {
	RepositoryExternalID string `json:"repository_external_id"`
	RepositoryName       string `json:"repository_name"`
	RepositoryURL        string `json:"repository_url"`
	DefaultBranch        string `json:"default_branch,omitempty"`
}

// NewSCMWorkspaceHandler creates a new workspace SCM handler
func NewSCMWorkspaceHandler(db database.Database, encryption *sso.SecretEncryption, providerHandler *SCMProviderHandler) *SCMWorkspaceHandler {
	return &SCMWorkspaceHandler{
		db:                 db,
		encryption:         encryption,
		providerHandler:    providerHandler,
		credentialResolver: scm.NewCredentialResolver(db, encryption),
	}
}

// GetWorkspaceSCMConnections returns all SCM connections for a workspace
func (h *SCMWorkspaceHandler) GetWorkspaceSCMConnections(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	rows, err := h.db.Query(`
		SELECT
			wsc.id, wsc.workspace_id, wsc.scm_provider_id, wsc.enabled,
			wsc.default_branch_pattern, wsc.item_key_pattern,
			wsc.created_by, wsc.created_at, wsc.updated_at,
			sp.name, sp.provider_type, sp.slug,
			(SELECT COUNT(*) FROM workspace_repositories wr WHERE wr.workspace_scm_connection_id = wsc.id) as repo_count
		FROM workspace_scm_connections wsc
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wsc.workspace_id = ?
		ORDER BY sp.name
	`, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	connections := []WorkspaceSCMConnectionResponse{}
	for rows.Next() {
		var conn WorkspaceSCMConnectionResponse
		var defaultBranchPattern, itemKeyPattern sql.NullString
		var createdBy sql.NullInt64

		err := rows.Scan(
			&conn.ID, &conn.WorkspaceID, &conn.SCMProviderID, &conn.Enabled,
			&defaultBranchPattern, &itemKeyPattern,
			&createdBy, &conn.CreatedAt, &conn.UpdatedAt,
			&conn.ProviderName, &conn.ProviderType, &conn.ProviderSlug,
			&conn.RepositoryCount,
		)
		if err != nil {
			slog.Error("failed to scan connection", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}

		if defaultBranchPattern.Valid {
			conn.DefaultBranchPattern = defaultBranchPattern.String
		}
		if itemKeyPattern.Valid {
			conn.ItemKeyPattern = itemKeyPattern.String
		}
		if createdBy.Valid {
			cb := int(createdBy.Int64)
			conn.CreatedBy = &cb
		}

		connections = append(connections, conn)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(connections)
}

// CreateWorkspaceSCMConnection creates a new SCM connection for a workspace
func (h *SCMWorkspaceHandler) CreateWorkspaceSCMConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	var req CreateWorkspaceSCMConnectionRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.SCMProviderID == 0 {
		respondValidationError(w, r, "scm_provider_id is required")
		return
	}

	// Verify the provider exists and is enabled
	var providerEnabled bool
	err = h.db.QueryRow("SELECT enabled FROM scm_providers WHERE id = ?", req.SCMProviderID).Scan(&providerEnabled)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if !providerEnabled {
		respondBadRequest(w, r, "SCM provider is not enabled")
		return
	}

	// Check if workspace is allowed to use this provider
	if h.providerHandler != nil {
		var allowed bool
		allowed, err = h.providerHandler.IsWorkspaceAllowedForProvider(req.SCMProviderID, workspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !allowed {
			respondForbidden(w, r)
			return
		}
	}

	// Get user ID from context
	var createdBy *int
	if userID, ok := r.Context().Value("user_id").(int); ok {
		createdBy = &userID
	}

	// Insert the connection
	result, err := h.db.Exec(`
		INSERT INTO workspace_scm_connections (
			workspace_id, scm_provider_id, enabled,
			default_branch_pattern, item_key_pattern, created_by
		) VALUES (?, ?, 1, ?, ?, ?)
	`, workspaceID, req.SCMProviderID,
		nullString(req.DefaultBranchPattern), nullString(req.ItemKeyPattern), createdBy)
	if err != nil {
		slog.Error("failed to create connection", slog.String("component", "scm"), slog.Any("error", err))
		respondConflict(w, r, "Failed to create SCM connection. It may already exist.")
		return
	}

	id, _ := result.LastInsertId()

	// Get the created connection
	conn, err := h.getConnectionByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(conn)
}

// GetWorkspaceSCMConnection returns a single SCM connection
func (h *SCMWorkspaceHandler) GetWorkspaceSCMConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	conn, err := h.getConnectionByID(connID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Verify connection belongs to this workspace
	if conn.WorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(conn)
}

// UpdateWorkspaceSCMConnection updates an SCM connection
func (h *SCMWorkspaceHandler) UpdateWorkspaceSCMConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	var req UpdateWorkspaceSCMConnectionRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Verify connection exists and belongs to this workspace
	conn, err := h.getConnectionByID(connID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if conn.WorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	// Build update
	enabled := conn.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	_, err = h.db.Exec(`
		UPDATE workspace_scm_connections SET
			enabled = ?,
			default_branch_pattern = ?,
			item_key_pattern = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, enabled, nullString(req.DefaultBranchPattern), nullString(req.ItemKeyPattern), connID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get updated connection
	conn, err = h.getConnectionByID(connID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(conn)
}

// DeleteWorkspaceSCMConnection deletes an SCM connection
func (h *SCMWorkspaceHandler) DeleteWorkspaceSCMConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	// Verify connection belongs to this workspace
	var connWorkspaceID int
	err = h.db.QueryRow("SELECT workspace_id FROM workspace_scm_connections WHERE id = ?", connID).Scan(&connWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if connWorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	// Delete (cascade will handle repositories and item links)
	_, err = h.db.Exec("DELETE FROM workspace_scm_connections WHERE id = ?", connID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListAvailableRepositories lists repositories from the SCM provider
func (h *SCMWorkspaceHandler) ListAvailableRepositories(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	// Get connection and verify ownership
	var providerID int
	var connWorkspaceID int
	err = h.db.QueryRow(`
		SELECT workspace_id, scm_provider_id FROM workspace_scm_connections WHERE id = ?
	`, connID).Scan(&connWorkspaceID, &providerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if connWorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	// Strict enforcement: check if workspace is still allowed to use this provider
	if h.providerHandler != nil {
		var allowed bool
		allowed, err = h.providerHandler.IsWorkspaceAllowedForProvider(providerID, workspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !allowed {
			respondForbidden(w, r)
			return
		}
	}

	// Get provider with workspace credentials using CredentialResolver
	provider, err := h.credentialResolver.GetProviderForConnection(r.Context(), connID)
	if err != nil {
		slog.Error("failed to get provider", slog.String("component", "scm"), slog.Any("error", err))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":        err.Error(),
			"repositories": []interface{}{},
		})
		return
	}

	// Parse query params
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 30
	}

	opts := scm.ListRepositoriesOptions{
		Page:    page,
		PerPage: perPage,
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	repos, err := provider.ListRepositories(ctx, opts)
	if err != nil {
		slog.Error("failed to list repositories", slog.String("component", "scm"), slog.Any("error", err))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":        err.Error(),
			"repositories": []interface{}{},
		})
		return
	}

	// Get already linked repos to mark them
	linkedMap := make(map[string]bool)
	linkedRows, err := h.db.Query(`
		SELECT repository_external_id FROM workspace_repositories
		WHERE workspace_scm_connection_id = ?
	`, connID)
	if err == nil {
		defer func() { _ = linkedRows.Close() }()
		for linkedRows.Next() {
			var extID string
			if linkedRows.Scan(&extID) == nil {
				linkedMap[extID] = true
			}
		}
	}

	// Build response with linked status
	type RepoWithStatus struct {
		scm.Repository
		IsLinked bool `json:"is_linked"`
	}

	result := make([]RepoWithStatus, 0, len(repos))
	for _, repo := range repos {
		result = append(result, RepoWithStatus{
			Repository: repo,
			IsLinked:   linkedMap[repo.ID],
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"repositories": result,
		"page":         page,
		"per_page":     perPage,
	})
}

// GetLinkedRepositories returns repositories linked to a workspace connection
func (h *SCMWorkspaceHandler) GetLinkedRepositories(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	// Verify connection belongs to workspace
	var connWorkspaceID int
	err = h.db.QueryRow("SELECT workspace_id FROM workspace_scm_connections WHERE id = ?", connID).Scan(&connWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if connWorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	rows, err := h.db.Query(`
		SELECT id, workspace_scm_connection_id, repository_external_id,
			   repository_name, repository_url, default_branch,
			   is_active, last_synced_at, created_at, updated_at
		FROM workspace_repositories
		WHERE workspace_scm_connection_id = ?
		ORDER BY repository_name
	`, connID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	repos := []WorkspaceRepositoryResponse{}
	for rows.Next() {
		var repo WorkspaceRepositoryResponse
		var lastSyncedAt sql.NullTime

		err := rows.Scan(
			&repo.ID, &repo.WorkspaceSCMConnectionID, &repo.RepositoryExternalID,
			&repo.RepositoryName, &repo.RepositoryURL, &repo.DefaultBranch,
			&repo.IsActive, &lastSyncedAt, &repo.CreatedAt, &repo.UpdatedAt,
		)
		if err != nil {
			slog.Error("failed to scan repository", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}

		if lastSyncedAt.Valid {
			repo.LastSyncedAt = &lastSyncedAt.Time
		}

		repos = append(repos, repo)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(repos)
}

// LinkRepository links a repository to a workspace connection
func (h *SCMWorkspaceHandler) LinkRepository(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	var req LinkRepositoryRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.RepositoryExternalID == "" || req.RepositoryName == "" || req.RepositoryURL == "" {
		respondValidationError(w, r, "repository_external_id, repository_name, and repository_url are required")
		return
	}

	// Verify connection belongs to workspace
	var connWorkspaceID, providerID int
	err = h.db.QueryRow("SELECT workspace_id, scm_provider_id FROM workspace_scm_connections WHERE id = ?", connID).Scan(&connWorkspaceID, &providerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if connWorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	// Strict enforcement: check if workspace is still allowed to use this provider
	if h.providerHandler != nil {
		var allowed bool
		allowed, err = h.providerHandler.IsWorkspaceAllowedForProvider(providerID, workspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !allowed {
			respondForbidden(w, r)
			return
		}
	}

	defaultBranch := req.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	result, err := h.db.Exec(`
		INSERT INTO workspace_repositories (
			workspace_scm_connection_id, repository_external_id,
			repository_name, repository_url, default_branch, is_active
		) VALUES (?, ?, ?, ?, ?, 1)
	`, connID, req.RepositoryExternalID, req.RepositoryName, req.RepositoryURL, defaultBranch)
	if err != nil {
		slog.Error("failed to link repository", slog.String("component", "scm"), slog.Any("error", err))
		respondConflict(w, r, "Failed to link repository. It may already be linked.")
		return
	}

	id, _ := result.LastInsertId()

	// Get the created repo
	var repo WorkspaceRepositoryResponse
	var lastSyncedAt sql.NullTime
	err = h.db.QueryRow(`
		SELECT id, workspace_scm_connection_id, repository_external_id,
			   repository_name, repository_url, default_branch,
			   is_active, last_synced_at, created_at, updated_at
		FROM workspace_repositories WHERE id = ?
	`, id).Scan(
		&repo.ID, &repo.WorkspaceSCMConnectionID, &repo.RepositoryExternalID,
		&repo.RepositoryName, &repo.RepositoryURL, &repo.DefaultBranch,
		&repo.IsActive, &lastSyncedAt, &repo.CreatedAt, &repo.UpdatedAt,
	)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if lastSyncedAt.Valid {
		repo.LastSyncedAt = &lastSyncedAt.Time
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(repo)
}

// UnlinkRepository removes a repository from a workspace
func (h *SCMWorkspaceHandler) UnlinkRepository(w http.ResponseWriter, r *http.Request) {
	repoID, err := strconv.Atoi(r.PathValue("repoId"))
	if err != nil {
		respondInvalidID(w, r, "repositoryId")
		return
	}

	result, err := h.db.Exec("DELETE FROM workspace_repositories WHERE id = ?", repoID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "repository")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAvailableSCMProviders returns all enabled SCM providers for connecting to a workspace
func (h *SCMWorkspaceHandler) GetAvailableSCMProviders(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	// Get all enabled providers that are not already connected to this workspace
	// Filter out restricted providers that this workspace doesn't have access to
	rows, err := h.db.Query(`
		SELECT sp.id, sp.slug, sp.name, sp.provider_type, sp.auth_method,
			   sp.workspace_restriction_mode,
			   CASE WHEN wsc.id IS NOT NULL THEN 1 ELSE 0 END as is_connected
		FROM scm_providers sp
		LEFT JOIN workspace_scm_connections wsc
			ON wsc.scm_provider_id = sp.id AND wsc.workspace_id = ?
		WHERE sp.enabled = 1
		  AND (
			sp.workspace_restriction_mode = 'unrestricted'
			OR sp.workspace_restriction_mode IS NULL
			OR EXISTS (
				SELECT 1 FROM scm_provider_workspace_allowlist al
				WHERE al.provider_id = sp.id AND al.workspace_id = ?
			)
		  )
		ORDER BY sp.name
	`, workspaceID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type AvailableProvider struct {
		ID           int                    `json:"id"`
		Slug         string                 `json:"slug"`
		Name         string                 `json:"name"`
		ProviderType models.SCMProviderType `json:"provider_type"`
		AuthMethod   models.SCMAuthMethod   `json:"auth_method"`
		IsConnected  bool                   `json:"is_connected"`
	}

	providers := []AvailableProvider{}
	for rows.Next() {
		var p AvailableProvider
		var isConnected int
		var restrictionMode sql.NullString
		err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.ProviderType, &p.AuthMethod, &restrictionMode, &isConnected)
		if err != nil {
			slog.Error("failed to scan provider", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}
		p.IsConnected = isConnected == 1
		providers = append(providers, p)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providers)
}

// Helper methods

func (h *SCMWorkspaceHandler) getConnectionByID(id int) (*WorkspaceSCMConnectionResponse, error) {
	var conn WorkspaceSCMConnectionResponse
	var defaultBranchPattern, itemKeyPattern sql.NullString
	var createdBy sql.NullInt64

	err := h.db.QueryRow(`
		SELECT
			wsc.id, wsc.workspace_id, wsc.scm_provider_id, wsc.enabled,
			wsc.default_branch_pattern, wsc.item_key_pattern,
			wsc.created_by, wsc.created_at, wsc.updated_at,
			sp.name, sp.provider_type, sp.slug,
			(SELECT COUNT(*) FROM workspace_repositories wr WHERE wr.workspace_scm_connection_id = wsc.id) as repo_count
		FROM workspace_scm_connections wsc
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wsc.id = ?
	`, id).Scan(
		&conn.ID, &conn.WorkspaceID, &conn.SCMProviderID, &conn.Enabled,
		&defaultBranchPattern, &itemKeyPattern,
		&createdBy, &conn.CreatedAt, &conn.UpdatedAt,
		&conn.ProviderName, &conn.ProviderType, &conn.ProviderSlug,
		&conn.RepositoryCount,
	)
	if err != nil {
		return nil, err
	}

	if defaultBranchPattern.Valid {
		conn.DefaultBranchPattern = defaultBranchPattern.String
	}
	if itemKeyPattern.Valid {
		conn.ItemKeyPattern = itemKeyPattern.String
	}
	if createdBy.Valid {
		cb := int(createdBy.Int64)
		conn.CreatedBy = &cb
	}

	return &conn, nil
}

// StartWorkspaceOAuth initiates the OAuth flow for a workspace SCM connection
// POST /api/workspaces/{id}/scm-connections/{connId}/auth/start
func (h *SCMWorkspaceHandler) StartWorkspaceOAuth(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	// Get user from context
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	// Verify connection exists and belongs to this workspace
	var providerID, connWorkspaceID int
	err = h.db.QueryRow(`
		SELECT workspace_id, scm_provider_id FROM workspace_scm_connections WHERE id = ?
	`, connID).Scan(&connWorkspaceID, &providerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if connWorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	// Get provider details
	var providerType models.SCMProviderType
	var authMethod models.SCMAuthMethod
	var clientID, baseURL, oauthScopes, providerSlug sql.NullString

	err = h.db.QueryRow(`
		SELECT provider_type, auth_method, oauth_client_id, base_url, scopes, slug
		FROM scm_providers WHERE id = ?
	`, providerID).Scan(&providerType, &authMethod, &clientID, &baseURL, &oauthScopes, &providerSlug)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// OAuth is only valid for OAuth auth method
	if authMethod != models.SCMAuthMethodOAuth {
		respondBadRequest(w, r, "This provider does not use OAuth authentication")
		return
	}

	if !clientID.Valid || clientID.String == "" {
		respondBadRequest(w, r, "OAuth not configured for this provider")
		return
	}

	// Generate state token
	stateBytes := make([]byte, 32)
	_, _ = rand.Read(stateBytes)
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Determine redirect URI
	redirectURI := h.getWorkspaceOAuthRedirectURI(r, providerSlug.String)

	// Store state token with workspace_id
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = h.db.Exec(`
		INSERT INTO scm_oauth_state (provider_id, state, redirect_uri, user_id, workspace_id, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, providerID, state, redirectURI, user.ID, workspaceID, expiresAt)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Generate OAuth URL based on provider type
	var authURL string
	switch providerType {
	case models.SCMProviderTypeGitHub:
		scopes := "repo read:user user:email"
		if oauthScopes.Valid && oauthScopes.String != "" {
			scopes = oauthScopes.String
		}
		authURL = fmt.Sprintf(
			"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=%s&state=%s",
			clientID.String,
			url.QueryEscape(redirectURI),
			url.QueryEscape(scopes),
			state,
		)
	case models.SCMProviderTypeGitea:
		if !baseURL.Valid || baseURL.String == "" {
			respondBadRequest(w, r, "Base URL not configured for this provider")
			return
		}
		scopes := "read:repository write:repository"
		if oauthScopes.Valid && oauthScopes.String != "" {
			scopes = oauthScopes.String
		}
		authURL = fmt.Sprintf(
			"%s/login/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
			strings.TrimSuffix(baseURL.String, "/"),
			clientID.String,
			url.QueryEscape(redirectURI),
			url.QueryEscape(scopes),
			state,
		)
	default:
		respondBadRequest(w, r, "OAuth not supported for this provider type")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"auth_url": authURL,
	})
}

// SetWorkspacePAT sets a Personal Access Token for a workspace connection
// POST /api/workspaces/{id}/scm-connections/{connId}/auth/pat
func (h *SCMWorkspaceHandler) SetWorkspacePAT(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	var req struct {
		PersonalAccessToken string `json:"personal_access_token"`
	}
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.PersonalAccessToken == "" {
		respondValidationError(w, r, "personal_access_token is required")
		return
	}

	// Verify connection exists and belongs to this workspace
	var connWorkspaceID, providerID int
	err = h.db.QueryRow(`
		SELECT workspace_id, scm_provider_id FROM workspace_scm_connections WHERE id = ?
	`, connID).Scan(&connWorkspaceID, &providerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if connWorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	// Verify provider uses PAT auth
	var authMethod models.SCMAuthMethod
	err = h.db.QueryRow("SELECT auth_method FROM scm_providers WHERE id = ?", providerID).Scan(&authMethod)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if authMethod != models.SCMAuthMethodPAT {
		respondBadRequest(w, r, "This provider does not use PAT authentication")
		return
	}

	// Encrypt and store PAT
	patEnc, err := h.encryption.Encrypt(req.PersonalAccessToken)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_, err = h.db.Exec(`
		UPDATE workspace_scm_connections SET
			personal_access_token_encrypted = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, patEnc, connID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Personal Access Token configured successfully",
	})
}

// ClearWorkspaceCredentials removes workspace-level credentials
// DELETE /api/workspaces/{id}/scm-connections/{connId}/auth
func (h *SCMWorkspaceHandler) ClearWorkspaceCredentials(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	// Verify connection exists and belongs to this workspace
	var connWorkspaceID int
	err = h.db.QueryRow("SELECT workspace_id FROM workspace_scm_connections WHERE id = ?", connID).Scan(&connWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if connWorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	// Clear all workspace-level credentials
	_, err = h.db.Exec(`
		UPDATE workspace_scm_connections SET
			oauth_access_token_encrypted = NULL,
			oauth_refresh_token_encrypted = NULL,
			oauth_token_expires_at = NULL,
			personal_access_token_encrypted = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, connID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetWorkspaceConnectionAuthStatus returns the auth status of a workspace connection
// GET /api/workspaces/{id}/scm-connections/{connId}/auth/status
func (h *SCMWorkspaceHandler) GetWorkspaceConnectionAuthStatus(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}
	connID, err := strconv.Atoi(r.PathValue("connId"))
	if err != nil {
		respondInvalidID(w, r, "connectionId")
		return
	}

	// Get connection with workspace-level credentials info
	var connWorkspaceID, providerID int
	var wsOAuthTokenEnc, wsPATEnc sql.NullString
	var wsOAuthExpiresAt sql.NullTime

	err = h.db.QueryRow(`
		SELECT workspace_id, scm_provider_id,
			   oauth_access_token_encrypted, personal_access_token_encrypted,
			   oauth_token_expires_at
		FROM workspace_scm_connections WHERE id = ?
	`, connID).Scan(&connWorkspaceID, &providerID, &wsOAuthTokenEnc, &wsPATEnc, &wsOAuthExpiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if connWorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	// Get provider info
	var authMethod models.SCMAuthMethod
	var providerPATEnc, ghAppPrivateKeyEnc sql.NullString
	err = h.db.QueryRow(`
		SELECT auth_method, personal_access_token_encrypted, github_app_private_key_encrypted
		FROM scm_providers WHERE id = ?
	`, providerID).Scan(&authMethod, &providerPATEnc, &ghAppPrivateKeyEnc)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"auth_method":      authMethod,
		"is_authenticated": false,
	}

	switch authMethod {
	case models.SCMAuthMethodOAuth:
		hasToken := wsOAuthTokenEnc.Valid && wsOAuthTokenEnc.String != ""
		response["has_workspace_token"] = hasToken
		response["is_authenticated"] = hasToken
		if wsOAuthExpiresAt.Valid {
			response["token_expires_at"] = wsOAuthExpiresAt.Time
			response["token_expired"] = wsOAuthExpiresAt.Time.Before(time.Now())
		}
	case models.SCMAuthMethodPAT:
		hasWorkspacePAT := wsPATEnc.Valid && wsPATEnc.String != ""
		hasProviderPAT := providerPATEnc.Valid && providerPATEnc.String != ""
		response["has_workspace_pat"] = hasWorkspacePAT
		response["has_provider_pat"] = hasProviderPAT
		response["is_authenticated"] = hasWorkspacePAT || hasProviderPAT
	case models.SCMAuthMethodGitHubApp:
		hasAppKey := ghAppPrivateKeyEnc.Valid && ghAppPrivateKeyEnc.String != ""
		response["has_github_app_key"] = hasAppKey
		response["is_authenticated"] = hasAppKey
		response["auth_source"] = "provider"
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *SCMWorkspaceHandler) getWorkspaceOAuthRedirectURI(r *http.Request, providerSlug string) string {
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("PUBLIC_URL")
	}
	if baseURL != "" {
		return baseURL + "/api/scm/oauth/" + providerSlug + "/callback"
	}

	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/api/scm/oauth/%s/callback", scheme, r.Host, providerSlug)
}
