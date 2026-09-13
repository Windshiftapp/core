package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/sanitize"
	"windshift/internal/scm"
	"windshift/internal/services"
	"windshift/internal/sso"
)

// SCMWorkspaceHandler handles workspace SCM connection endpoints
type SCMWorkspaceHandler struct {
	repo               *repository.SCMWorkspaceRepository
	encryption         *sso.SecretEncryption
	providerHandler    *SCMProviderHandler
	credentialResolver *scm.CredentialResolver
	permissionService  *services.PermissionService
	baseURL            string // public URL of the application (from config.Load)
}

// WorkspaceSCMConnectionResponse represents a workspace SCM connection for API responses
type WorkspaceSCMConnectionResponse = repository.SCMWorkspaceConnection

// WorkspaceRepositoryResponse represents a linked repository for API responses
type WorkspaceRepositoryResponse = repository.SCMLinkedRepository

// UpdateWorkspaceRepositoryRequest is the body of the per-repo PATCH used
// by the workspace settings UI to set the milestone-from-tag globs.
// Only the milestone_* fields are mutable in v1; other columns are
// managed by the link/unlink flow.
type UpdateWorkspaceRepositoryRequest struct {
	MilestoneTagPattern    *string `json:"milestone_tag_pattern,omitempty"`
	MilestoneBranchPattern *string `json:"milestone_branch_pattern,omitempty"`
}

// CreateWorkspaceSCMConnectionRequest represents a request to create a connection
type CreateWorkspaceSCMConnectionRequest struct {
	SCMProviderID        int    `json:"scm_provider_id"`
	DefaultBranchPattern string `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       string `json:"item_key_pattern,omitempty"`
}

// UpdateWorkspaceSCMConnectionRequest represents a request to update a connection
type UpdateWorkspaceSCMConnectionRequest struct {
	Enabled              *bool   `json:"enabled,omitempty"`
	SmartCommitsEnabled  *bool   `json:"smart_commits_enabled,omitempty"`
	DefaultBranchPattern *string `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       *string `json:"item_key_pattern,omitempty"`
}

// LinkRepositoryRequest represents a request to link a repository
type LinkRepositoryRequest struct {
	RepositoryExternalID string `json:"repository_external_id"`
	RepositoryName       string `json:"repository_name"`
	RepositoryURL        string `json:"repository_url"`
	DefaultBranch        string `json:"default_branch,omitempty"`
}

// NewSCMWorkspaceHandler creates a new workspace SCM handler.
// baseURL: public URL of the application (from config.Load), used to build
// OAuth callback URIs. Empty falls back to deriving from the request Host.
func NewSCMWorkspaceHandler(repo *repository.SCMWorkspaceRepository, encryption *sso.SecretEncryption, providerHandler *SCMProviderHandler, credentialResolver *scm.CredentialResolver, permissionService *services.PermissionService, baseURL string) *SCMWorkspaceHandler {
	return &SCMWorkspaceHandler{
		repo:               repo,
		encryption:         encryption,
		providerHandler:    providerHandler,
		credentialResolver: credentialResolver,
		permissionService:  permissionService,
		baseURL:            baseURL,
	}
}

// GetWorkspaceSCMConnections returns all SCM connections for a workspace
func (h *SCMWorkspaceHandler) GetWorkspaceSCMConnections(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	connections, err := h.repo.ListConnections(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if strings.EqualFold(r.URL.Query().Get("include_repositories"), "true") {
		repositories, err := h.repo.ListLinkedRepositoriesForWorkspace(workspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		connectionIndexes := make(map[int]int, len(connections))
		for i := range connections {
			connectionIndexes[connections[i].ID] = i
			connections[i].Repositories = []repository.SCMLinkedRepository{}
		}
		for _, linkedRepository := range repositories {
			if index, exists := connectionIndexes[linkedRepository.WorkspaceSCMConnectionID]; exists {
				connections[index].Repositories = append(connections[index].Repositories, linkedRepository)
			}
		}
	}

	if strings.EqualFold(r.URL.Query().Get("include_auth_status"), "true") {
		user, ok := RequireAuth(w, r)
		if !ok {
			return
		}
		authStatuses, err := h.repo.ListConnectionAuthStatusesForWorkspace(workspaceID, user.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		for i := range connections {
			connections[i].AuthStatus = authStatuses[connections[i].ID]
		}
	}

	respondJSONOK(w, connections)
}

// GetAccessibleSCMConnections returns connections across every workspace the
// caller can view. It supports global release pickers without a workspace-list
// request followed by one connection request per workspace.
func (h *SCMWorkspaceHandler) GetAccessibleSCMConnections(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	workspaceIDs, err := h.permissionService.AccessibleWorkspaceIDs(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	connections, err := h.repo.ListConnectionsForWorkspaces(workspaceIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, connections)
}

// CreateWorkspaceSCMConnection creates a new SCM connection for a workspace
func (h *SCMWorkspaceHandler) CreateWorkspaceSCMConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	req, ok := decodeJSON[CreateWorkspaceSCMConnectionRequest](w, r)
	if !ok {
		return
	}
	// Sanitize identifier-shaped patterns while preserving supported placeholders.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.DefaultBranchPattern, Policy: sanitize.ShortIdentifier, Label: "Default branch pattern"},
		sanitize.Pair{Target: &req.ItemKeyPattern, Policy: sanitize.ShortIdentifier, Label: "Item key pattern"},
	)

	if req.SCMProviderID == 0 {
		respondValidationError(w, r, "scm_provider_id is required")
		return
	}

	providerEnabled, err := h.repo.GetProviderEnabled(req.SCMProviderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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

	var createdBy *int
	if userID, ok := r.Context().Value("user_id").(int); ok {
		createdBy = &userID
	}

	id, err := h.repo.CreateConnection(workspaceID, req.SCMProviderID, req.DefaultBranchPattern, req.ItemKeyPattern, createdBy)
	if err != nil {
		slog.Error("failed to create connection", slog.String("component", "scm"), slog.Any("error", err))
		respondConflict(w, r, "Failed to create SCM connection. It may already exist.")
		return
	}

	conn, err := h.repo.GetConnectionByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, struct {
		*WorkspaceSCMConnectionResponse
		Warnings []string `json:"warnings,omitempty"`
	}{conn, warnings})
}

// GetWorkspaceSCMConnection returns a single SCM connection
func (h *SCMWorkspaceHandler) GetWorkspaceSCMConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	conn, err := h.repo.GetConnectionByID(connID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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

	respondJSONOK(w, conn)
}

// UpdateWorkspaceSCMConnection updates an SCM connection
func (h *SCMWorkspaceHandler) UpdateWorkspaceSCMConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	var err error

	req, ok := decodeJSON[UpdateWorkspaceSCMConnectionRequest](w, r)
	if !ok {
		return
	}
	// Sanitize only supplied pointer fields; nil means leave unchanged.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: req.DefaultBranchPattern, Policy: sanitize.ShortIdentifier, Label: "Default branch pattern"},
		sanitize.Pair{Target: req.ItemKeyPattern, Policy: sanitize.ShortIdentifier, Label: "Item key pattern"},
	)

	conn, err := h.repo.GetConnectionByID(connID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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

	enabled := conn.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	smartCommits := conn.SmartCommitsEnabled
	if req.SmartCommitsEnabled != nil {
		smartCommits = *req.SmartCommitsEnabled
	}

	defaultBranchPattern := conn.DefaultBranchPattern
	if req.DefaultBranchPattern != nil {
		defaultBranchPattern = *req.DefaultBranchPattern
	}
	itemKeyPattern := conn.ItemKeyPattern
	if req.ItemKeyPattern != nil {
		itemKeyPattern = *req.ItemKeyPattern
	}

	if err := h.repo.UpdateConnection(connID, enabled, smartCommits, defaultBranchPattern, itemKeyPattern); err != nil {
		respondInternalError(w, r, err)
		return
	}

	conn, err = h.repo.GetConnectionByID(connID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, struct {
		*WorkspaceSCMConnectionResponse
		Warnings []string `json:"warnings,omitempty"`
	}{conn, warnings})
}

// DeleteWorkspaceSCMConnection deletes an SCM connection
func (h *SCMWorkspaceHandler) DeleteWorkspaceSCMConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	// Verify connection belongs to this workspace
	connWorkspaceID, _, err := h.repo.GetConnectionWorkspaceAndProvider(connID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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
	if err := h.repo.DeleteConnection(connID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// validateWorkspaceConnectionAccess verifies that a connection belongs to the
// requested workspace and that the workspace may use its provider. It writes
// the endpoint's existing error response when validation fails.
func (h *SCMWorkspaceHandler) validateWorkspaceConnectionAccess(w http.ResponseWriter, r *http.Request, workspaceID, connID int) bool {
	connWorkspaceID, providerID, err := h.repo.GetConnectionWorkspaceAndProvider(connID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return false
	}

	if connWorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return false
	}

	if h.providerHandler != nil {
		allowed, err := h.providerHandler.IsWorkspaceAllowedForProvider(providerID, workspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !allowed {
			respondForbidden(w, r)
			return false
		}
	}

	return true
}

// ListAvailableRepositories lists repositories from the SCM provider
func (h *SCMWorkspaceHandler) ListAvailableRepositories(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	if !h.validateWorkspaceConnectionAccess(w, r, workspaceID, connID) {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	// Credential resolution refreshes expiring OAuth tokens internally;
	// a dead refresh token surfaces as ErrRefreshTokenInvalid and the
	// stored credentials are already wiped, so reconnecting is the only fix.
	provider, err := h.credentialResolver.GetProviderForUser(r.Context(), connID, user.ID)
	if err != nil {
		if errors.Is(err, scm.ErrUserSCMNotConnected) {
			respondJSONOK(w, map[string]any{
				"error":        "Please connect your SCM account first",
				"error_code":   "user_scm_not_connected",
				"repositories": []any{},
			})
			return
		}
		if errors.Is(err, scm.ErrRefreshTokenInvalid) {
			respondError(w, r, restapi.NewAPIError(http.StatusUnauthorized, "SCM_REAUTH_REQUIRED", "Your SCM connection is no longer valid. Please reconnect."))
			return
		}
		slog.Error("failed to get provider", slog.String("component", "scm"), slog.Any("error", err))
		respondJSONOK(w, map[string]any{
			"error":        err.Error(),
			"repositories": []any{},
		})
		return
	}

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
		respondJSONOK(w, map[string]any{
			"error":        err.Error(),
			"repositories": []any{},
		})
		return
	}

	linkedMap, _ := h.repo.LinkedRepositoryExternalIDs(connID)

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

	respondJSONOK(w, map[string]any{
		"repositories": result,
		"page":         page,
		"per_page":     perPage,
	})
}

// GetLinkedRepositories returns repositories linked to a workspace connection
func (h *SCMWorkspaceHandler) GetLinkedRepositories(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	// Verify connection belongs to workspace
	connWorkspaceID, _, err := h.repo.GetConnectionWorkspaceAndProvider(connID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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

	repos, err := h.repo.ListLinkedRepositories(connID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, repos)
}

// LinkRepository links a repository to a workspace connection
func (h *SCMWorkspaceHandler) LinkRepository(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	var err error

	req, ok := decodeJSON[LinkRepositoryRequest](w, r)
	if !ok {
		return
	}
	// RepositoryName renders in repo picker / item link panels; the
	// branch ref is identifier-shaped. ExternalID + URL are SCM-side
	// values validated separately.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.RepositoryName, Policy: sanitize.PlainTextField, Label: "Repository name"},
		sanitize.Pair{Target: &req.DefaultBranch, Policy: sanitize.ShortIdentifier, Label: "Default branch"},
	)
	_ = warnings // response is built ad-hoc below; warnings surfacing is a follow-up.

	if req.RepositoryExternalID == "" || req.RepositoryName == "" || req.RepositoryURL == "" {
		respondValidationError(w, r, "repository_external_id, repository_name, and repository_url are required")
		return
	}

	if !h.validateWorkspaceConnectionAccess(w, r, workspaceID, connID) {
		return
	}

	defaultBranch := req.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	id, err := h.repo.LinkRepository(connID, req.RepositoryExternalID, req.RepositoryName, req.RepositoryURL, defaultBranch)
	if err != nil {
		slog.Error("failed to link repository", slog.String("component", "scm"), slog.Any("error", err))
		respondConflict(w, r, "Failed to link repository. It may already be linked.")
		return
	}

	// Get the created repo
	repo, err := h.repo.GetRepositoryByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, repo)
}

// UnlinkRepository removes a repository from a workspace
func (h *SCMWorkspaceHandler) UnlinkRepository(w http.ResponseWriter, r *http.Request) {
	repoID, ok := requireIDParam(w, r, "repoId")
	if !ok {
		return
	}

	// Look up the workspace via the connection to check permission
	workspaceID, err := h.repo.GetRepositoryWorkspaceID(repoID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "repository")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}

	if err := h.repo.DeleteRepository(repoID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateRepository sets the per-repo automation patterns (milestone
// tag/branch globs). Workspace admin only. Body fields are optional —
// nil leaves the column unchanged so the UI can patch one field at a
// time. Empty string is rejected to prevent accidentally disabling
// detection by saving a blank pattern.
func (h *SCMWorkspaceHandler) UpdateRepository(w http.ResponseWriter, r *http.Request) {
	repoID, ok := requireIDParam(w, r, "repoId")
	if !ok {
		return
	}

	workspaceID, err := h.repo.GetRepositoryWorkspaceID(repoID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "repository")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}

	req, ok := decodeJSON[UpdateWorkspaceRepositoryRequest](w, r)
	if !ok {
		return
	}
	// Milestone tag/branch patterns are short identifier-shaped globs
	// (e.g. "v*", "release/{{milestone}}"). Sanitize in place; nil
	// pointers are no-ops.
	sanitize.ApplyAll(
		sanitize.Pair{Target: req.MilestoneTagPattern, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: req.MilestoneBranchPattern, Policy: sanitize.ShortIdentifier},
	)

	// Unset fields aren't overwritten. Reject empty strings explicitly —
	// they would disable detection silently.
	if req.MilestoneTagPattern != nil && *req.MilestoneTagPattern == "" {
		respondValidationError(w, r, "milestone_tag_pattern must not be empty")
		return
	}
	if req.MilestoneBranchPattern != nil && *req.MilestoneBranchPattern == "" {
		respondValidationError(w, r, "milestone_branch_pattern must not be empty")
		return
	}
	if req.MilestoneTagPattern == nil && req.MilestoneBranchPattern == nil {
		respondValidationError(w, r, "no fields to update")
		return
	}

	if err := h.repo.UpdateRepositoryPatterns(repoID, req.MilestoneTagPattern, req.MilestoneBranchPattern); err != nil {
		respondInternalError(w, r, err)
		return
	}

	repo, err := h.repo.GetRepositoryByID(repoID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, repo)
}

// GetAvailableSCMProviders returns all enabled SCM providers for connecting to a workspace
func (h *SCMWorkspaceHandler) GetAvailableSCMProviders(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get all enabled providers that are not already connected to this workspace.
	// Restricted providers this workspace doesn't have access to are filtered out.
	providers, err := h.repo.ListAvailableProviders(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, providers)
}

// StartWorkspaceOAuth initiates the OAuth flow for a workspace SCM connection
// POST /api/workspaces/{id}/scm-connections/{connId}/auth/start
func (h *SCMWorkspaceHandler) StartWorkspaceOAuth(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	// Get user from context
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Verify connection exists and belongs to this workspace
	connWorkspaceID, providerID, err := h.repo.GetConnectionWorkspaceAndProvider(connID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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
	oauthCfg, err := h.repo.GetProviderOAuthConfig(providerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// OAuth is only valid for OAuth auth method
	if oauthCfg.AuthMethod != models.SCMAuthMethodOAuth {
		respondBadRequest(w, r, "This provider does not use OAuth authentication")
		return
	}

	if oauthCfg.ClientID == "" {
		respondBadRequest(w, r, "OAuth not configured for this provider")
		return
	}

	// Generate state token
	stateBytes := make([]byte, 32)
	_, _ = rand.Read(stateBytes)
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Determine redirect URI
	redirectURI, err := h.getWorkspaceOAuthRedirectURI(oauthCfg.Slug)
	if err != nil {
		slog.Error("workspace SCM OAuth start: redirect URI unavailable", slog.String("component", "scm"), slog.String("slug", oauthCfg.Slug), slog.Any("error", err))
		respondServiceUnavailable(w, r, err.Error())
		return
	}

	// Store state token with workspace_id
	expiresAt := time.Now().Add(5 * time.Minute)
	if err := h.repo.CreateOAuthState(providerID, state, redirectURI, user.ID, workspaceID, expiresAt); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Generate OAuth URL based on provider type
	var authURL string
	switch oauthCfg.ProviderType {
	case models.SCMProviderTypeGitHub:
		scopes := "repo read:user user:email"
		if oauthCfg.Scopes != "" {
			scopes = oauthCfg.Scopes
		}
		authURL = fmt.Sprintf(
			"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=%s&state=%s",
			oauthCfg.ClientID,
			url.QueryEscape(redirectURI),
			url.QueryEscape(scopes),
			state,
		)
	case models.SCMProviderTypeGitea:
		if oauthCfg.BaseURL == "" {
			respondBadRequest(w, r, "Base URL not configured for this provider")
			return
		}
		scopes := "read:user read:repository write:repository"
		if oauthCfg.Scopes != "" {
			scopes = oauthCfg.Scopes
		}
		authURL = fmt.Sprintf(
			"%s/login/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
			strings.TrimSuffix(oauthCfg.BaseURL, "/"),
			oauthCfg.ClientID,
			url.QueryEscape(redirectURI),
			url.QueryEscape(scopes),
			state,
		)
	case models.SCMProviderTypeGitLab:
		gitlabBaseURL := "https://gitlab.com"
		if strings.TrimSpace(oauthCfg.BaseURL) != "" {
			gitlabBaseURL = strings.TrimSuffix(strings.TrimSuffix(oauthCfg.BaseURL, "/"), "/api/v4")
		}
		scopes := "api"
		if oauthCfg.Scopes != "" {
			scopes = oauthCfg.Scopes
		}
		authURL = fmt.Sprintf(
			"%s/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
			gitlabBaseURL,
			oauthCfg.ClientID,
			url.QueryEscape(redirectURI),
			url.QueryEscape(scopes),
			state,
		)
	default:
		respondBadRequest(w, r, "OAuth not supported for this provider type")
		return
	}

	respondJSONOK(w, map[string]string{
		"auth_url": authURL,
	})
}

// SetWorkspacePAT sets a Personal Access Token for a workspace connection
// POST /api/workspaces/{id}/scm-connections/{connId}/auth/pat
func (h *SCMWorkspaceHandler) SetWorkspacePAT(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	var req struct {
		PersonalAccessToken string `json:"personal_access_token"`
	}
	if err := newJSONDecoder(w, r).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.PersonalAccessToken == "" {
		respondValidationError(w, r, "personal_access_token is required")
		return
	}

	// Verify connection exists and belongs to this workspace
	connWorkspaceID, providerID, err := h.repo.GetConnectionWorkspaceAndProvider(connID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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
	authMethod, err := h.repo.GetProviderAuthMethod(providerID)
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

	if err := h.repo.SetConnectionPAT(connID, patEnc); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{
		"status":  "ok",
		"message": "Personal Access Token configured successfully",
	})
}

// ClearWorkspaceCredentials removes workspace-level credentials
// DELETE /api/workspaces/{id}/scm-connections/{connId}/auth
func (h *SCMWorkspaceHandler) ClearWorkspaceCredentials(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	// Verify connection exists and belongs to this workspace
	connWorkspaceID, _, err := h.repo.GetConnectionWorkspaceAndProvider(connID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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
	if err := h.repo.ClearConnectionCredentials(connID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetWorkspaceConnectionAuthStatus returns the auth status of a workspace connection
// GET /api/workspaces/{id}/scm-connections/{connId}/auth/status
func (h *SCMWorkspaceHandler) GetWorkspaceConnectionAuthStatus(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	connID, ok := requireIDParam(w, r, "connId")
	if !ok {
		return
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get connection with workspace-level credentials info
	connInfo, err := h.repo.GetConnectionAuthInfo(connID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "connection")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if connInfo.WorkspaceID != workspaceID {
		respondNotFound(w, r, "connection")
		return
	}

	// Get provider info
	providerInfo, err := h.repo.GetProviderAuthInfo(connInfo.ProviderID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := map[string]any{
		"auth_method":      providerInfo.AuthMethod,
		"is_authenticated": false,
		"provider_slug":    providerInfo.Slug,
	}

	switch providerInfo.AuthMethod {
	case models.SCMAuthMethodOAuth:
		hasWorkspaceToken := connInfo.HasOAuthToken
		// Also check user-level token (best-effort: a missing row leaves
		// the user-token flags unset).
		hasUserToken, scmUsername, _ := h.repo.GetUserOAuthTokenStatus(currentUser.ID, connInfo.ProviderID)

		response["has_workspace_token"] = hasWorkspaceToken
		response["has_user_token"] = hasUserToken
		response["is_authenticated"] = hasWorkspaceToken || hasUserToken
		if scmUsername != nil {
			response["scm_username"] = *scmUsername
		}
		if connInfo.OAuthTokenExpiresAt != nil {
			response["token_expires_at"] = *connInfo.OAuthTokenExpiresAt
			response["token_expired"] = connInfo.OAuthTokenExpiresAt.Before(time.Now())
		}
	case models.SCMAuthMethodPAT:
		hasWorkspacePAT := connInfo.HasPAT
		hasProviderPAT := providerInfo.HasPAT
		response["has_workspace_pat"] = hasWorkspacePAT
		response["has_provider_pat"] = hasProviderPAT
		response["is_authenticated"] = hasWorkspacePAT || hasProviderPAT
	case models.SCMAuthMethodGitHubApp:
		response["has_github_app_key"] = providerInfo.HasGitHubAppKey
		response["is_authenticated"] = providerInfo.HasGitHubAppKey
		response["auth_source"] = "provider"
	}

	respondJSONOK(w, response)
}

// getWorkspaceOAuthRedirectURI returns the canonical OAuth callback URL
// for the given provider slug. As with the equivalent helpers in
// scm_providers_oauth.go and integration_oauth.go, it is built strictly
// from the configured baseURL — never from request headers. If baseURL
// is unset the handler must treat the OAuth flow as misconfigured.
func (h *SCMWorkspaceHandler) getWorkspaceOAuthRedirectURI(providerSlug string) (string, error) {
	if h.baseURL == "" {
		return "", fmt.Errorf("workspace SCM OAuth is not configured: server baseURL is unset")
	}
	return h.baseURL + "/api/scm/oauth/" + providerSlug + "/callback", nil
}
