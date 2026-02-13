package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/services"
	"windshift/internal/sso"
)

// SCMItemLinksHandler handles item SCM link endpoints
type SCMItemLinksHandler struct {
	db                database.Database
	encryption        *sso.SecretEncryption
	syncService       *scm.SyncService
	permissionService *services.PermissionService
}

// getItemURL constructs the URL to an item based on the request context
func getItemURL(r *http.Request, workspaceID, itemID int) string {
	scheme := "https"
	if r.TLS == nil {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else {
			scheme = "http"
		}
	}
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}
	return fmt.Sprintf("%s://%s/workspaces/%d/items/%d", scheme, host, workspaceID, itemID)
}

// ItemSCMLinkResponse represents an SCM link for API responses
type ItemSCMLinkResponse struct {
	ID                    int       `json:"id"`
	ItemID                int       `json:"item_id"`
	WorkspaceRepositoryID int       `json:"workspace_repository_id"`
	LinkType              string    `json:"link_type"`
	ExternalID            string    `json:"external_id"`
	ExternalURL           string    `json:"external_url,omitempty"`
	Title                 string    `json:"title,omitempty"`
	State                 string    `json:"state,omitempty"`
	AuthorExternalID      string    `json:"author_external_id,omitempty"`
	AuthorName            string    `json:"author_name,omitempty"`
	DetectionSource       string    `json:"detection_source,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	// Joined fields
	RepositoryName string `json:"repository_name,omitempty"`
	RepositoryURL  string `json:"repository_url,omitempty"`
	ProviderType   string `json:"provider_type,omitempty"`
}

// CreateItemSCMLinkRequest represents a request to create an SCM link
type CreateItemSCMLinkRequest struct {
	WorkspaceRepositoryID int    `json:"workspace_repository_id"`
	LinkType              string `json:"link_type"`
	ExternalID            string `json:"external_id"`
	ExternalURL           string `json:"external_url,omitempty"`
	Title                 string `json:"title,omitempty"`
	State                 string `json:"state,omitempty"`
	AuthorName            string `json:"author_name,omitempty"`
}

// CreateBranchForItemRequest represents a request to create a branch (and optionally PR) for an item
type CreateBranchForItemRequest struct {
	WorkspaceRepositoryID int    `json:"workspace_repository_id"`
	BranchName            string `json:"branch_name"`
	BaseBranch            string `json:"base_branch,omitempty"`
	CreatePR              bool   `json:"create_pr"`
	PRTitle               string `json:"pr_title,omitempty"`
	PRBody                string `json:"pr_body,omitempty"`
}

// CreateBranchForItemResponse represents the response from creating a branch
type CreateBranchForItemResponse struct {
	BranchURL string `json:"branch_url"`
	PRURL     string `json:"pr_url,omitempty"`
	PRNumber  int    `json:"pr_number,omitempty"`
	LinkID    int    `json:"link_id"`
	PRLinkID  int    `json:"pr_link_id,omitempty"`
}

// NewSCMItemLinksHandler creates a new item SCM links handler
func NewSCMItemLinksHandler(db database.Database, encryption *sso.SecretEncryption, permissionService *services.PermissionService) *SCMItemLinksHandler {
	return &SCMItemLinksHandler{
		db:                db,
		encryption:        encryption,
		syncService:       scm.NewSyncService(db, encryption),
		permissionService: permissionService,
	}
}

// GetItemSCMLinks returns all SCM links for an item
func (h *SCMItemLinksHandler) GetItemSCMLinks(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	rows, err := h.db.Query(`
		SELECT
			isl.id, isl.item_id, isl.workspace_repository_id, isl.link_type,
			isl.external_id, isl.external_url, isl.title, isl.state,
			isl.author_external_id, isl.author_name, isl.detection_source,
			isl.created_at, isl.updated_at,
			wr.repository_name, wr.repository_url,
			sp.provider_type
		FROM item_scm_links isl
		JOIN workspace_repositories wr ON wr.id = isl.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE isl.item_id = ?
		ORDER BY isl.created_at DESC
	`, itemID)
	if err != nil {
		slog.Error("failed to get links", slog.String("component", "scm_item_links"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	links := []ItemSCMLinkResponse{}
	for rows.Next() {
		var link ItemSCMLinkResponse
		var externalURL, title, state, authorExternalID, authorName, detectionSource sql.NullString

		err := rows.Scan(
			&link.ID, &link.ItemID, &link.WorkspaceRepositoryID, &link.LinkType,
			&link.ExternalID, &externalURL, &title, &state,
			&authorExternalID, &authorName, &detectionSource,
			&link.CreatedAt, &link.UpdatedAt,
			&link.RepositoryName, &link.RepositoryURL,
			&link.ProviderType,
		)
		if err != nil {
			slog.Error("failed to scan link", slog.String("component", "scm_item_links"), slog.Any("error", err))
			continue
		}

		if externalURL.Valid {
			link.ExternalURL = externalURL.String
		}
		if title.Valid {
			link.Title = title.String
		}
		if state.Valid {
			link.State = state.String
		}
		if authorExternalID.Valid {
			link.AuthorExternalID = authorExternalID.String
		}
		if authorName.Valid {
			link.AuthorName = authorName.String
		}
		if detectionSource.Valid {
			link.DetectionSource = detectionSource.String
		}

		links = append(links, link)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(links)
}

// CreateItemSCMLink creates a new SCM link for an item
func (h *SCMItemLinksHandler) CreateItemSCMLink(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}

	var req CreateItemSCMLinkRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if req.WorkspaceRepositoryID == 0 {
		respondValidationError(w, r, "workspace_repository_id is required")
		return
	}
	if req.LinkType == "" {
		respondValidationError(w, r, "link_type is required")
		return
	}
	if req.ExternalID == "" {
		respondValidationError(w, r, "external_id is required")
		return
	}

	// Validate link type
	linkType := models.SCMLinkType(req.LinkType)
	if linkType != models.SCMLinkTypePullRequest &&
		linkType != models.SCMLinkTypeCommit &&
		linkType != models.SCMLinkTypeBranch {
		respondValidationError(w, r, "Invalid link_type. Must be pull_request, commit, or branch")
		return
	}

	// Verify item exists
	var itemExists int
	err = h.db.QueryRow("SELECT 1 FROM items WHERE id = ?", itemID).Scan(&itemExists)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
		} else {
			respondInternalError(w, r, fmt.Errorf("failed to verify item: %w", err))
		}
		return
	}

	// Verify workspace repository exists and get workspace to verify item belongs to same workspace
	var repoWorkspaceID, itemWorkspaceID int
	err = h.db.QueryRow(`
		SELECT wsc.workspace_id
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wr.id = ?
	`, req.WorkspaceRepositoryID).Scan(&repoWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "repository")
		} else {
			respondInternalError(w, r, fmt.Errorf("failed to verify repository: %w", err))
		}
		return
	}

	err = h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&itemWorkspaceID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to verify item workspace: %w", err))
		return
	}

	if repoWorkspaceID != itemWorkspaceID {
		respondValidationError(w, r, "Repository does not belong to the item's workspace")
		return
	}

	// Insert the link
	result, err := h.db.Exec(`
		INSERT INTO item_scm_links (
			item_id, workspace_repository_id, link_type, external_id,
			external_url, title, state, author_name, detection_source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'manual')
	`, itemID, req.WorkspaceRepositoryID, linkType, req.ExternalID,
		nullString(req.ExternalURL), nullString(req.Title), nullString(req.State), nullString(req.AuthorName))
	if err != nil {
		slog.Error("failed to create link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		// Check for unique constraint violation
		respondConflict(w, r, "Failed to create SCM link. It may already exist.")
		return
	}

	id, _ := result.LastInsertId()

	// Get the created link
	link, err := h.getLinkByID(int(id))
	if err != nil {
		slog.Error("failed to get created link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("link created but failed to retrieve: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(link)
}

// getItemIDForLink looks up the item_id for a given SCM link ID
func (h *SCMItemLinksHandler) getItemIDForLink(linkID int) (int, error) {
	var itemID int
	err := h.db.QueryRow("SELECT item_id FROM item_scm_links WHERE id = ?", linkID).Scan(&itemID)
	return itemID, err
}

// DeleteItemSCMLink deletes an SCM link
func (h *SCMItemLinksHandler) DeleteItemSCMLink(w http.ResponseWriter, r *http.Request) {
	linkID, err := strconv.Atoi(r.PathValue("linkId"))
	if err != nil {
		respondInvalidID(w, r, "linkId")
		return
	}

	// Look up item for permission check
	itemID, err := h.getItemIDForLink(linkID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "link")
		} else {
			respondInternalError(w, r, fmt.Errorf("failed to verify link: %w", err))
		}
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}

	_, err = h.db.Exec("DELETE FROM item_scm_links WHERE id = ?", linkID)
	if err != nil {
		slog.Error("failed to delete link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RefreshItemSCMLink refreshes the details of an SCM link from the provider
func (h *SCMItemLinksHandler) RefreshItemSCMLink(w http.ResponseWriter, r *http.Request) {
	linkID, err := strconv.Atoi(r.PathValue("linkId"))
	if err != nil {
		respondInvalidID(w, r, "linkId")
		return
	}

	// Look up item for permission check
	itemID, err := h.getItemIDForLink(linkID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "link")
		} else {
			respondInternalError(w, r, fmt.Errorf("failed to get link: %w", err))
		}
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	err = h.syncService.RefreshItemSCMLink(ctx, linkID)
	if err != nil {
		slog.Error("failed to refresh link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to refresh link: %w", err))
		return
	}

	// Return the updated link
	link, err := h.getLinkByID(linkID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to retrieve updated link: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(link)
}

// SyncWorkspaceRepository triggers a manual sync for a repository
func (h *SCMItemLinksHandler) SyncWorkspaceRepository(w http.ResponseWriter, r *http.Request) {
	repoID, err := strconv.Atoi(r.PathValue("repoId"))
	if err != nil {
		respondInvalidID(w, r, "repoId")
		return
	}

	// Look up the workspace ID for this repository
	var workspaceID int
	err = h.db.QueryRow(`
		SELECT wsc.workspace_id FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wr.workspace_scm_connection_id = wsc.id
		WHERE wr.id = ?
	`, repoID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "repository")
		} else {
			respondInternalError(w, r, fmt.Errorf("failed to verify repository: %w", err))
		}
		return
	}

	// Require workspace admin permission
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}
	hasPermission, err := h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionWorkspaceAdmin)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !hasPermission {
		respondForbidden(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	err = h.syncService.SyncRepository(ctx, repoID)
	if err != nil {
		slog.Error("failed to sync repository", slog.String("component", "scm_item_links"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to sync repository: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Repository sync completed",
	})
}

// GetWorkspaceRepositoriesForItem returns repositories available for linking to an item
func (h *SCMItemLinksHandler) GetWorkspaceRepositoriesForItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	// Get item's workspace
	var workspaceID int
	err = h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
		} else {
			respondInternalError(w, r, fmt.Errorf("failed to get item: %w", err))
		}
		return
	}

	// Get repositories linked to this workspace
	rows, err := h.db.Query(`
		SELECT
			wr.id, wr.repository_name, wr.repository_url, wr.default_branch,
			sp.provider_type, sp.name as provider_name
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wsc.workspace_id = ? AND wr.is_active = 1 AND wsc.enabled = 1
		ORDER BY wr.repository_name
	`, workspaceID)
	if err != nil {
		slog.Error("failed to get repositories", slog.String("component", "scm_item_links"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type RepoInfo struct {
		ID             int    `json:"id"`
		RepositoryName string `json:"repository_name"`
		RepositoryURL  string `json:"repository_url"`
		DefaultBranch  string `json:"default_branch"`
		ProviderType   string `json:"provider_type"`
		ProviderName   string `json:"provider_name"`
	}

	repos := []RepoInfo{}
	for rows.Next() {
		var repo RepoInfo
		err := rows.Scan(&repo.ID, &repo.RepositoryName, &repo.RepositoryURL,
			&repo.DefaultBranch, &repo.ProviderType, &repo.ProviderName)
		if err != nil {
			slog.Error("failed to scan repository", slog.String("component", "scm_item_links"), slog.Any("error", err))
			continue
		}
		repos = append(repos, repo)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(repos)
}

// CreateBranchForItem creates a branch (and optionally a draft PR) for an item
func (h *SCMItemLinksHandler) CreateBranchForItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}

	var req CreateBranchForItemRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if req.WorkspaceRepositoryID == 0 {
		respondValidationError(w, r, "workspace_repository_id is required")
		return
	}
	if req.BranchName == "" {
		respondValidationError(w, r, "branch_name is required")
		return
	}

	// Verify item exists and get item info for PR body
	var itemKey, itemTitle string
	var itemWorkspaceID int
	err = h.db.QueryRow(`
		SELECT i.workspace_id, w.key || '-' || i.workspace_item_number, i.title
		FROM items i
		JOIN workspaces w ON w.id = i.workspace_id
		WHERE i.id = ?
	`, itemID).Scan(&itemWorkspaceID, &itemKey, &itemTitle)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
		} else {
			slog.Error("failed to get item", slog.String("component", "scm_item_links"), slog.Any("error", err))
			respondInternalError(w, r, err)
		}
		return
	}

	// Verify repository belongs to the item's workspace
	var repoWorkspaceID int
	err = h.db.QueryRow(`
		SELECT wsc.workspace_id
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wr.id = ?
	`, req.WorkspaceRepositoryID).Scan(&repoWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "repository")
		} else {
			respondInternalError(w, r, fmt.Errorf("failed to verify repository: %w", err))
		}
		return
	}

	if repoWorkspaceID != itemWorkspaceID {
		respondValidationError(w, r, "Repository does not belong to the item's workspace")
		return
	}

	// Get authenticated user for per-user OAuth tokens
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Create the branch using user's credentials
	branchURL, err := h.syncService.CreateBranchForRepository(ctx, req.WorkspaceRepositoryID, req.BranchName, req.BaseBranch, user.ID)
	if err != nil {
		if errors.Is(err, scm.ErrUserSCMNotConnected) {
			// User needs to connect their SCM account
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "scm_not_connected",
				"message": "You need to connect your SCM account before creating branches or PRs",
			})
			return
		}
		slog.Error("failed to create branch", slog.String("component", "scm_item_links"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to create branch: %w", err))
		return
	}

	// Create branch link
	branchLinkID, err := h.syncService.CreateItemSCMLink(ctx, itemID, req.WorkspaceRepositoryID, "branch", req.BranchName, branchURL, req.BranchName)
	if err != nil {
		slog.Error("failed to create branch link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		// Branch was created but link failed - return success with warning
	}

	response := CreateBranchForItemResponse{
		BranchURL: branchURL,
		LinkID:    branchLinkID,
	}

	// Create PR if requested
	if req.CreatePR {
		prTitle := req.PRTitle
		if prTitle == "" {
			prTitle = itemKey + ": " + itemTitle
		}

		prBody := req.PRBody
		if prBody == "" {
			itemURL := getItemURL(r, itemWorkspaceID, itemID)
			prBody = fmt.Sprintf("Linked to [%s](%s)", itemKey, itemURL)
		}

		pr, prURL, err := h.syncService.CreatePullRequestForRepository(ctx, req.WorkspaceRepositoryID, scm.CreatePROptions{
			Title:      prTitle,
			Body:       prBody,
			HeadBranch: req.BranchName,
			BaseBranch: req.BaseBranch,
			Draft:      true,
		}, user.ID)
		if err != nil {
			slog.Error("failed to create PR", slog.String("component", "scm_item_links"), slog.Any("error", err))
			// Branch was created but PR failed - return partial success
			errorMsg := "Branch created but failed to create pull request"
			if errors.Is(err, scm.ErrAlreadyExists) {
				errorMsg = "Branch created but a pull request already exists for this branch"
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPartialContent)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"branch_url": branchURL,
				"link_id":    branchLinkID,
				"error":      errorMsg,
			})
			return
		}

		response.PRURL = prURL
		response.PRNumber = pr.Number

		// Create PR link
		prLinkID, err := h.syncService.CreateItemSCMLink(ctx, itemID, req.WorkspaceRepositoryID, "pull_request", strconv.Itoa(pr.Number), prURL, prTitle)
		if err != nil {
			slog.Error("failed to create PR link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		} else {
			response.PRLinkID = prLinkID
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

// CreatePRFromBranchRequest represents a request to create a PR from an existing branch link
type CreatePRFromBranchRequest struct {
	PRTitle    string `json:"pr_title,omitempty"`
	PRBody     string `json:"pr_body,omitempty"`
	BaseBranch string `json:"base_branch,omitempty"`
}

// CreatePRFromBranchResponse represents the response from creating a PR from a branch
type CreatePRFromBranchResponse struct {
	PRURL    string               `json:"pr_url"`
	PRNumber int                  `json:"pr_number"`
	PRLink   *ItemSCMLinkResponse `json:"pr_link"`
}

// CreatePRFromBranch creates a pull request from an existing branch link
func (h *SCMItemLinksHandler) CreatePRFromBranch(w http.ResponseWriter, r *http.Request) {
	linkID, err := strconv.Atoi(r.PathValue("linkId"))
	if err != nil {
		respondInvalidID(w, r, "linkId")
		return
	}

	var req CreatePRFromBranchRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Get the branch link details
	var itemID, workspaceRepoID, itemWorkspaceID int
	var branchName, linkType string
	var itemKey, itemTitle string
	err = h.db.QueryRow(`
		SELECT
			isl.item_id, isl.workspace_repository_id, isl.external_id, isl.link_type,
			w.key || '-' || i.workspace_item_number, i.title, i.workspace_id
		FROM item_scm_links isl
		JOIN items i ON i.id = isl.item_id
		JOIN workspaces w ON w.id = i.workspace_id
		WHERE isl.id = ?
	`, linkID).Scan(&itemID, &workspaceRepoID, &branchName, &linkType, &itemKey, &itemTitle, &itemWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "link")
		} else {
			slog.Error("failed to get link", slog.String("component", "scm_item_links"), slog.Any("error", err))
			respondInternalError(w, r, err)
		}
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}

	// Verify this is a branch link
	if linkType != "branch" {
		respondValidationError(w, r, "Can only create PR from a branch link")
		return
	}

	// Get default branch if not specified
	baseBranch := req.BaseBranch
	if baseBranch == "" {
		err = h.db.QueryRow(`
			SELECT default_branch FROM workspace_repositories WHERE id = ?
		`, workspaceRepoID).Scan(&baseBranch)
		if err != nil {
			baseBranch = "main" // fallback
		}
	}

	// Set PR title if not provided
	prTitle := req.PRTitle
	if prTitle == "" {
		prTitle = itemKey + ": " + itemTitle
	}

	// Set PR body if not provided
	prBody := req.PRBody
	if prBody == "" {
		itemURL := getItemURL(r, itemWorkspaceID, itemID)
		prBody = fmt.Sprintf("Linked to [%s](%s)", itemKey, itemURL)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// Create the PR
	pr, prURL, err := h.syncService.CreatePullRequestForRepository(ctx, workspaceRepoID, scm.CreatePROptions{
		Title:      prTitle,
		Body:       prBody,
		HeadBranch: branchName,
		BaseBranch: baseBranch,
		Draft:      false,
	})
	if err != nil {
		slog.Error("failed to create PR", slog.String("component", "scm_item_links"), slog.Any("error", err))
		if errors.Is(err, scm.ErrAlreadyExists) {
			respondConflict(w, r, "A pull request already exists for this branch")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to create pull request: %w", err))
		return
	}

	// Create PR link
	prLinkID, err := h.syncService.CreateItemSCMLink(ctx, itemID, workspaceRepoID, "pull_request", strconv.Itoa(pr.Number), prURL, prTitle)
	if err != nil {
		slog.Error("failed to create PR link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		// PR was created but link failed - return success with the PR info
	}

	// Get the created PR link
	var prLink *ItemSCMLinkResponse
	if prLinkID > 0 {
		prLink, _ = h.getLinkByID(prLinkID)
	}

	response := CreatePRFromBranchResponse{
		PRURL:    prURL,
		PRNumber: pr.Number,
		PRLink:   prLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

// getLinkByID retrieves a single SCM link by ID
func (h *SCMItemLinksHandler) getLinkByID(id int) (*ItemSCMLinkResponse, error) {
	var link ItemSCMLinkResponse
	var externalURL, title, state, authorExternalID, authorName, detectionSource sql.NullString

	err := h.db.QueryRow(`
		SELECT
			isl.id, isl.item_id, isl.workspace_repository_id, isl.link_type,
			isl.external_id, isl.external_url, isl.title, isl.state,
			isl.author_external_id, isl.author_name, isl.detection_source,
			isl.created_at, isl.updated_at,
			wr.repository_name, wr.repository_url,
			sp.provider_type
		FROM item_scm_links isl
		JOIN workspace_repositories wr ON wr.id = isl.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE isl.id = ?
	`, id).Scan(
		&link.ID, &link.ItemID, &link.WorkspaceRepositoryID, &link.LinkType,
		&link.ExternalID, &externalURL, &title, &state,
		&authorExternalID, &authorName, &detectionSource,
		&link.CreatedAt, &link.UpdatedAt,
		&link.RepositoryName, &link.RepositoryURL,
		&link.ProviderType,
	)
	if err != nil {
		return nil, err
	}

	if externalURL.Valid {
		link.ExternalURL = externalURL.String
	}
	if title.Valid {
		link.Title = title.String
	}
	if state.Valid {
		link.State = state.String
	}
	if authorExternalID.Valid {
		link.AuthorExternalID = authorExternalID.String
	}
	if authorName.Valid {
		link.AuthorName = authorName.String
	}
	if detectionSource.Valid {
		link.DetectionSource = detectionSource.String
	}

	return &link, nil
}

// GetSCMConnectionStatus returns the user's SCM connection status for an item's workspace
func (h *SCMItemLinksHandler) GetSCMConnectionStatus(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	// Get the workspace for this item
	var workspaceID int
	err = h.db.QueryRow(`SELECT workspace_id FROM items WHERE id = ?`, itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
		} else {
			respondInternalError(w, r, fmt.Errorf("failed to get item: %w", err))
		}
		return
	}

	// Get all OAuth SCM providers connected to this workspace and user's connection status
	rows, err := h.db.Query(`
		SELECT
			sp.id, sp.name, sp.provider_type, sp.slug, sp.auth_method,
			CASE WHEN ut.id IS NOT NULL THEN 1 ELSE 0 END as user_connected,
			ut.scm_username
		FROM workspace_scm_connections wsc
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		LEFT JOIN user_scm_oauth_tokens ut ON ut.scm_provider_id = sp.id AND ut.user_id = ?
		WHERE wsc.workspace_id = ? AND wsc.enabled = 1 AND sp.enabled = 1
		ORDER BY sp.name
	`, user.ID, workspaceID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get connection status: %w", err))
		return
	}
	defer func() { _ = rows.Close() }()

	type ProviderConnectionStatus struct {
		ProviderID    int                    `json:"provider_id"`
		ProviderName  string                 `json:"provider_name"`
		ProviderType  models.SCMProviderType `json:"provider_type"`
		ProviderSlug  string                 `json:"provider_slug"`
		AuthMethod    models.SCMAuthMethod   `json:"auth_method"`
		UserConnected bool                   `json:"user_connected"`
		SCMUsername   string                 `json:"scm_username,omitempty"`
	}

	providers := []ProviderConnectionStatus{}
	allConnected := true
	hasOAuthProvider := false

	for rows.Next() {
		var p ProviderConnectionStatus
		var userConnected int
		var scmUsername sql.NullString

		err := rows.Scan(
			&p.ProviderID, &p.ProviderName, &p.ProviderType, &p.ProviderSlug, &p.AuthMethod,
			&userConnected, &scmUsername,
		)
		if err != nil {
			continue
		}

		p.UserConnected = userConnected == 1
		p.SCMUsername = scmUsername.String

		// For OAuth providers, track if user is connected
		if p.AuthMethod == models.SCMAuthMethodOAuth {
			hasOAuthProvider = true
			if !p.UserConnected {
				allConnected = false
			}
		}

		providers = append(providers, p)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"providers":          providers,
		"user_connected":     allConnected || !hasOAuthProvider, // Connected if all OAuth providers are connected or no OAuth providers
		"has_oauth_provider": hasOAuthProvider,
	})
}
