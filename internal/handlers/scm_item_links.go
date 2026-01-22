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
	"windshift/internal/sso"
)

// SCMItemLinksHandler handles item SCM link endpoints
type SCMItemLinksHandler struct {
	db          database.Database
	encryption  *sso.SecretEncryption
	syncService *scm.SyncService
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
func NewSCMItemLinksHandler(db database.Database, encryption *sso.SecretEncryption) *SCMItemLinksHandler {
	return &SCMItemLinksHandler{
		db:          db,
		encryption:  encryption,
		syncService: scm.NewSyncService(db, encryption),
	}
}

// GetItemSCMLinks returns all SCM links for an item
func (h *SCMItemLinksHandler) GetItemSCMLinks(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
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
		http.Error(w, "Failed to get SCM links", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

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
	json.NewEncoder(w).Encode(links)
}

// CreateItemSCMLink creates a new SCM link for an item
func (h *SCMItemLinksHandler) CreateItemSCMLink(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	var req CreateItemSCMLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.WorkspaceRepositoryID == 0 {
		http.Error(w, "workspace_repository_id is required", http.StatusBadRequest)
		return
	}
	if req.LinkType == "" {
		http.Error(w, "link_type is required", http.StatusBadRequest)
		return
	}
	if req.ExternalID == "" {
		http.Error(w, "external_id is required", http.StatusBadRequest)
		return
	}

	// Validate link type
	linkType := models.SCMLinkType(req.LinkType)
	if linkType != models.SCMLinkTypePullRequest &&
		linkType != models.SCMLinkTypeCommit &&
		linkType != models.SCMLinkTypeBranch {
		http.Error(w, "Invalid link_type. Must be pull_request, commit, or branch", http.StatusBadRequest)
		return
	}

	// Verify item exists
	var itemExists int
	err = h.db.QueryRow("SELECT 1 FROM items WHERE id = ?", itemID).Scan(&itemExists)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Item not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to verify item", http.StatusInternalServerError)
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
			http.Error(w, "Repository not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to verify repository", http.StatusInternalServerError)
		}
		return
	}

	err = h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&itemWorkspaceID)
	if err != nil {
		http.Error(w, "Failed to verify item workspace", http.StatusInternalServerError)
		return
	}

	if repoWorkspaceID != itemWorkspaceID {
		http.Error(w, "Repository does not belong to the item's workspace", http.StatusBadRequest)
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
		http.Error(w, "Failed to create SCM link. It may already exist.", http.StatusConflict)
		return
	}

	id, _ := result.LastInsertId()

	// Get the created link
	link, err := h.getLinkByID(int(id))
	if err != nil {
		slog.Error("failed to get created link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		http.Error(w, "Link created but failed to retrieve", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(link)
}

// DeleteItemSCMLink deletes an SCM link
func (h *SCMItemLinksHandler) DeleteItemSCMLink(w http.ResponseWriter, r *http.Request) {
	linkID, err := strconv.Atoi(r.PathValue("linkId"))
	if err != nil {
		http.Error(w, "Invalid link ID", http.StatusBadRequest)
		return
	}

	// Verify link exists
	var exists int
	err = h.db.QueryRow("SELECT 1 FROM item_scm_links WHERE id = ?", linkID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Link not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to verify link", http.StatusInternalServerError)
		}
		return
	}

	_, err = h.db.Exec("DELETE FROM item_scm_links WHERE id = ?", linkID)
	if err != nil {
		slog.Error("failed to delete link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		http.Error(w, "Failed to delete link", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RefreshItemSCMLink refreshes the details of an SCM link from the provider
func (h *SCMItemLinksHandler) RefreshItemSCMLink(w http.ResponseWriter, r *http.Request) {
	linkID, err := strconv.Atoi(r.PathValue("linkId"))
	if err != nil {
		http.Error(w, "Invalid link ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	err = h.syncService.RefreshItemSCMLink(ctx, linkID)
	if err != nil {
		slog.Error("failed to refresh link", slog.String("component", "scm_item_links"), slog.Any("error", err))
		http.Error(w, "Failed to refresh link: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated link
	link, err := h.getLinkByID(linkID)
	if err != nil {
		http.Error(w, "Failed to retrieve updated link", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(link)
}

// SyncWorkspaceRepository triggers a manual sync for a repository
func (h *SCMItemLinksHandler) SyncWorkspaceRepository(w http.ResponseWriter, r *http.Request) {
	repoID, err := strconv.Atoi(r.PathValue("repoId"))
	if err != nil {
		http.Error(w, "Invalid repository ID", http.StatusBadRequest)
		return
	}

	// Verify repository exists
	var exists int
	err = h.db.QueryRow("SELECT 1 FROM workspace_repositories WHERE id = ?", repoID).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Repository not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to verify repository", http.StatusInternalServerError)
		}
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	err = h.syncService.SyncRepository(ctx, repoID)
	if err != nil {
		slog.Error("failed to sync repository", slog.String("component", "scm_item_links"), slog.Any("error", err))
		http.Error(w, "Failed to sync repository: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Repository sync completed",
	})
}

// GetWorkspaceRepositoriesForItem returns repositories available for linking to an item
func (h *SCMItemLinksHandler) GetWorkspaceRepositoriesForItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	// Get item's workspace
	var workspaceID int
	err = h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Item not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to get item", http.StatusInternalServerError)
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
		http.Error(w, "Failed to get repositories", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

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
	json.NewEncoder(w).Encode(repos)
}

// CreateBranchForItem creates a branch (and optionally a draft PR) for an item
func (h *SCMItemLinksHandler) CreateBranchForItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	var req CreateBranchForItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.WorkspaceRepositoryID == 0 {
		http.Error(w, "workspace_repository_id is required", http.StatusBadRequest)
		return
	}
	if req.BranchName == "" {
		http.Error(w, "branch_name is required", http.StatusBadRequest)
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
			http.Error(w, "Item not found", http.StatusNotFound)
		} else {
			slog.Error("failed to get item", slog.String("component", "scm_item_links"), slog.Any("error", err))
			http.Error(w, "Failed to get item", http.StatusInternalServerError)
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
			http.Error(w, "Repository not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to verify repository", http.StatusInternalServerError)
		}
		return
	}

	if repoWorkspaceID != itemWorkspaceID {
		http.Error(w, "Repository does not belong to the item's workspace", http.StatusBadRequest)
		return
	}

	// Get authenticated user for per-user OAuth tokens
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "scm_not_connected",
				"message": "You need to connect your SCM account before creating branches or PRs",
			})
			return
		}
		slog.Error("failed to create branch", slog.String("component", "scm_item_links"), slog.Any("error", err))
		http.Error(w, "Failed to create branch: "+err.Error(), http.StatusInternalServerError)
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
			itemURL := getItemURL(r, itemWorkspaceID, int(itemID))
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
			json.NewEncoder(w).Encode(map[string]interface{}{
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
	json.NewEncoder(w).Encode(response)
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
		http.Error(w, "Invalid link ID", http.StatusBadRequest)
		return
	}

	var req CreatePRFromBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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
			http.Error(w, "Link not found", http.StatusNotFound)
		} else {
			slog.Error("failed to get link", slog.String("component", "scm_item_links"), slog.Any("error", err))
			http.Error(w, "Failed to get link", http.StatusInternalServerError)
		}
		return
	}

	// Verify this is a branch link
	if linkType != "branch" {
		http.Error(w, "Can only create PR from a branch link", http.StatusBadRequest)
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
			http.Error(w, "A pull request already exists for this branch", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create pull request", http.StatusInternalServerError)
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
	json.NewEncoder(w).Encode(response)
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
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get the workspace for this item
	var workspaceID int
	err = h.db.QueryRow(`SELECT workspace_id FROM items WHERE id = ?`, itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Item not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to get item", http.StatusInternalServerError)
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
		http.Error(w, "Failed to get connection status", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"providers":          providers,
		"user_connected":     allConnected || !hasOAuthProvider, // Connected if all OAuth providers are connected or no OAuth providers
		"has_oauth_provider": hasOAuthProvider,
	})
}
