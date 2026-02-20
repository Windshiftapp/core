package scm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/sso"
)

// SyncService handles periodic synchronization of SCM repositories
// to detect PRs, branches, and commits linked to work items
type SyncService struct {
	db         database.Database
	encryption *sso.SecretEncryption
	detector   *ItemKeyDetector
}

// NewSyncService creates a new SCM sync service
func NewSyncService(db database.Database, encryption *sso.SecretEncryption) *SyncService {
	return &SyncService{
		db:         db,
		encryption: encryption,
		detector:   NewItemKeyDetector(),
	}
}

// SyncAllRepositories syncs all active repositories across all workspaces
// This should be called periodically (e.g., every 5 minutes) by the scheduler
func (s *SyncService) SyncAllRepositories(ctx context.Context) error {
	slog.Debug("Starting sync of all repositories", slog.String("component", "scm"))

	// Get all active workspace repositories
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			wr.id, wr.repository_name, wr.default_branch,
			wsc.workspace_id, wsc.scm_provider_id, wsc.item_key_pattern,
			w.key as workspace_key, wsc.id as connection_id
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN workspaces w ON w.id = wsc.workspace_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wr.is_active = 1 AND wsc.enabled = 1
		  AND sp.auth_method != 'oauth'
	`)
	if err != nil {
		return fmt.Errorf("failed to query repositories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type repoInfo struct {
		ID             int
		RepositoryName string
		DefaultBranch  string
		WorkspaceID    int
		ProviderID     int
		ItemKeyPattern string
		WorkspaceKey   string
		ConnectionID   int
	}

	var repos []repoInfo
	for rows.Next() {
		var r repoInfo
		var itemKeyPattern sql.NullString
		err := rows.Scan(&r.ID, &r.RepositoryName, &r.DefaultBranch,
			&r.WorkspaceID, &r.ProviderID, &itemKeyPattern, &r.WorkspaceKey, &r.ConnectionID)
		if err != nil {
			slog.Error("Failed to scan repository", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}
		if itemKeyPattern.Valid {
			r.ItemKeyPattern = itemKeyPattern.String
		}
		repos = append(repos, r)
	}

	slog.Debug("Found active repositories to sync", slog.String("component", "scm"), slog.Int("count", len(repos)))

	// Group repos by connection to minimize provider instance creation
	connectionRepos := make(map[int][]repoInfo)
	for _, r := range repos {
		connectionRepos[r.ConnectionID] = append(connectionRepos[r.ConnectionID], r)
	}

	// Sync each connection's repos using CredentialResolver for proper token resolution
	credResolver := NewCredentialResolver(s.db, s.encryption)
	for connectionID, connectionRepoList := range connectionRepos {
		creds, err := credResolver.GetCredentialsByConnectionID(ctx, connectionID)
		if err != nil {
			slog.Error("Failed to get credentials", slog.String("component", "scm"), slog.Int("connection_id", connectionID), slog.Any("error", err))
			continue
		}

		// Refresh OAuth token if needed (e.g., expired Gitea tokens)
		if creds.OAuthAccessToken != "" {
			newToken, err := credResolver.RefreshOAuthTokenIfNeeded(ctx, connectionID, creds)
			if err != nil {
				slog.Warn("Failed to refresh OAuth token, using existing", slog.String("component", "scm"), slog.Int("connection_id", connectionID), slog.Any("error", err))
			} else {
				creds.OAuthAccessToken = newToken
			}
		}

		provider, err := credResolver.CreateProvider(creds)
		if err != nil {
			slog.Error("Failed to create provider", slog.String("component", "scm"), slog.Int("connection_id", connectionID), slog.Any("error", err))
			continue
		}

		for _, repo := range connectionRepoList {
			if err := s.syncRepository(ctx, provider, repo.ID, repo.RepositoryName, repo.WorkspaceID, repo.WorkspaceKey, repo.ItemKeyPattern); err != nil {
				slog.Error("Failed to sync repository", slog.String("component", "scm"), slog.String("repository", repo.RepositoryName), slog.Any("error", err))
			}
		}
	}

	slog.Debug("Completed sync of all repositories", slog.String("component", "scm"))
	return nil
}

// SyncRepository syncs a specific repository by ID
func (s *SyncService) SyncRepository(ctx context.Context, repoID int) error {
	// Get repository info
	var repositoryName, defaultBranch, workspaceKey string
	var workspaceID, connectionID int
	var itemKeyPattern sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT
			wr.repository_name, wr.default_branch,
			wsc.workspace_id, wsc.item_key_pattern,
			w.key as workspace_key, wsc.id as connection_id
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN workspaces w ON w.id = wsc.workspace_id
		WHERE wr.id = ?
	`, repoID).Scan(&repositoryName, &defaultBranch, &workspaceID, &itemKeyPattern, &workspaceKey, &connectionID)
	if err != nil {
		return fmt.Errorf("failed to get repository info: %w", err)
	}

	// Use CredentialResolver for proper workspace-level token resolution
	credResolver := NewCredentialResolver(s.db, s.encryption)
	creds, err := credResolver.GetCredentialsByConnectionID(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get credentials: %w", err)
	}

	// Refresh OAuth token if needed (e.g., expired Gitea tokens)
	if creds.OAuthAccessToken != "" {
		newToken, refreshErr := credResolver.RefreshOAuthTokenIfNeeded(ctx, connectionID, creds)
		if refreshErr != nil {
			slog.Warn("Failed to refresh OAuth token, using existing", slog.String("component", "scm"), slog.Int("connection_id", connectionID), slog.Any("error", refreshErr))
		} else {
			creds.OAuthAccessToken = newToken
		}
	}

	provider, err := credResolver.CreateProvider(creds)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	pattern := ""
	if itemKeyPattern.Valid {
		pattern = itemKeyPattern.String
	}

	return s.syncRepository(ctx, provider, repoID, repositoryName, workspaceID, workspaceKey, pattern)
}

// syncRepository performs the actual sync for a single repository
func (s *SyncService) syncRepository(ctx context.Context, provider Provider, repoID int, repositoryName string, workspaceID int, workspaceKey, itemKeyPattern string) error {
	// Parse owner/repo from repository name
	parts := strings.SplitN(repositoryName, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository name format: %s", repositoryName)
	}
	owner, repo := parts[0], parts[1]

	slog.Debug("Syncing repository", slog.String("component", "scm"), slog.String("repository", repositoryName), slog.String("workspace", workspaceKey))

	// Sync open pull requests
	if err := s.syncPullRequests(ctx, provider, owner, repo, repoID, workspaceID, workspaceKey, itemKeyPattern); err != nil {
		slog.Error("Failed to sync pull requests", slog.String("component", "scm"), slog.String("repository", repositoryName), slog.Any("error", err))
	}

	// Sync branches
	if err := s.syncBranches(ctx, provider, owner, repo, repoID, workspaceID, workspaceKey, itemKeyPattern); err != nil {
		slog.Error("Failed to sync branches", slog.String("component", "scm"), slog.String("repository", repositoryName), slog.Any("error", err))
	}

	// Update last_synced_at
	_, err := s.db.Exec(`
		UPDATE workspace_repositories SET last_synced_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, repoID)
	if err != nil {
		slog.Error("Failed to update last_synced_at", slog.String("component", "scm"), slog.Int("repo_id", repoID), slog.Any("error", err))
	}

	return nil
}

// syncPullRequests syncs pull requests from a repository
func (s *SyncService) syncPullRequests(ctx context.Context, provider Provider, owner, repo string, repoID, workspaceID int, workspaceKey, _ string) error {
	// Get open pull requests (and recently closed ones)
	prs, err := provider.ListPullRequests(ctx, owner, repo, ListPROptions{
		State:   "all", // Get all to detect state changes
		Page:    1,
		PerPage: 100, // Limit to recent PRs
	})
	if err != nil {
		return fmt.Errorf("failed to list PRs: %w", err)
	}

	for _, pr := range prs {
		// Detect item keys in PR
		keys := s.detector.DetectFromPullRequest(&pr, workspaceKey)
		if len(keys) == 0 {
			continue
		}

		// For each detected key, create/update a link
		for _, key := range keys {
			itemID, err := s.findItemByKey(ctx, workspaceID, key.Prefix, key.Number)
			if err != nil || itemID == 0 {
				continue // Item doesn't exist in this workspace
			}

			// Determine state
			state := models.SCMLinkStateOpen
			if pr.IsMerged {
				state = models.SCMLinkStateMerged
			} else if pr.State == "closed" {
				state = models.SCMLinkStateClosed
			}

			err = s.upsertItemSCMLink(ctx, itemID, repoID, models.SCMLinkTypePullRequest,
				strconv.Itoa(pr.Number), pr.URL, pr.Title, state, pr.Author.ID, pr.Author.Name, string(key.Source))
			if err != nil {
				slog.Error("Failed to upsert PR link", slog.String("component", "scm"), slog.Int("item_id", itemID), slog.Any("error", err))
			}
		}
	}

	return nil
}

// syncBranches syncs branches from a repository
func (s *SyncService) syncBranches(ctx context.Context, provider Provider, owner, repo string, repoID, workspaceID int, workspaceKey, _ string) error {
	branches, err := provider.ListBranches(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	for _, branch := range branches {
		// Detect item keys in branch name
		keys := s.detector.DetectFromBranch(&branch, workspaceKey)
		if len(keys) == 0 {
			continue
		}

		// For each detected key, create/update a link
		for _, key := range keys {
			itemID, err := s.findItemByKey(ctx, workspaceID, key.Prefix, key.Number)
			if err != nil || itemID == 0 {
				continue // Item doesn't exist in this workspace
			}

			// Construct branch URL (best effort - varies by provider)
			branchURL := fmt.Sprintf("https://github.com/%s/%s/tree/%s", owner, repo, branch.Name)

			err = s.upsertItemSCMLink(ctx, itemID, repoID, models.SCMLinkTypeBranch,
				branch.Name, branchURL, branch.Name, "", "", "", string(key.Source))
			if err != nil {
				slog.Error("Failed to upsert branch link", slog.String("component", "scm"), slog.Int("item_id", itemID), slog.Any("error", err))
			}
		}
	}

	return nil
}

// findItemByKey finds an item by its workspace key and number
func (s *SyncService) findItemByKey(ctx context.Context, workspaceID int, workspaceKey string, itemNumber int) (int, error) {
	var itemID int
	err := s.db.QueryRowContext(ctx, `
		SELECT i.id FROM items i
		JOIN workspaces w ON w.id = i.workspace_id
		WHERE i.workspace_id = ? AND i.workspace_item_number = ? AND UPPER(w.key) = ?
	`, workspaceID, itemNumber, strings.ToUpper(workspaceKey)).Scan(&itemID)

	if err == sql.ErrNoRows {
		return 0, nil
	}
	return itemID, err
}

// upsertItemSCMLink creates or updates an SCM link for an item
func (s *SyncService) upsertItemSCMLink(ctx context.Context, itemID, repoID int, linkType models.SCMLinkType,
	externalID, externalURL, title string, state models.SCMLinkState, authorExternalID, authorName, detectionSource string) error {

	// Try to find existing link
	var existingID int
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM item_scm_links
		WHERE item_id = ? AND workspace_repository_id = ? AND link_type = ? AND external_id = ?
	`, itemID, repoID, linkType, externalID).Scan(&existingID)

	if err == sql.ErrNoRows {
		// Insert new link
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO item_scm_links (
				item_id, workspace_repository_id, link_type, external_id,
				external_url, title, state, author_external_id, author_name, detection_source
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, itemID, repoID, linkType, externalID, externalURL, title, state, authorExternalID, authorName, detectionSource)
		return err
	}

	if err != nil {
		return err
	}

	// Update existing link
	_, err = s.db.ExecContext(ctx, `
		UPDATE item_scm_links SET
			external_url = ?, title = ?, state = ?,
			author_external_id = ?, author_name = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, externalURL, title, state, authorExternalID, authorName, existingID)

	return err
}

// getProviderInstance creates a provider instance with decrypted credentials
func (s *SyncService) getProviderInstance(providerID int) (Provider, error) {
	var providerType models.SCMProviderType
	var authMethod models.SCMAuthMethod
	var baseURL, patEnc, oauthAccessTokenEnc sql.NullString

	err := s.db.QueryRow(`
		SELECT provider_type, auth_method, base_url,
			   personal_access_token_encrypted, oauth_access_token_encrypted
		FROM scm_providers WHERE id = ?
	`, providerID).Scan(&providerType, &authMethod, &baseURL, &patEnc, &oauthAccessTokenEnc)
	if err != nil {
		return nil, err
	}

	cfg := ProviderConfig{
		ProviderType: providerType,
		AuthMethod:   authMethod,
		BaseURL:      baseURL.String,
	}

	// Decrypt credentials based on auth method
	switch authMethod {
	case models.SCMAuthMethodOAuth:
		if oauthAccessTokenEnc.Valid && oauthAccessTokenEnc.String != "" {
			token, err := s.encryption.Decrypt(oauthAccessTokenEnc.String)
			if err != nil {
				return nil, err
			}
			cfg.OAuthAccessToken = token
		}
	case models.SCMAuthMethodPAT:
		if patEnc.Valid && patEnc.String != "" {
			token, err := s.encryption.Decrypt(patEnc.String)
			if err != nil {
				return nil, err
			}
			cfg.PersonalAccessToken = token
		}
	}

	return NewProvider(cfg)
}

// RefreshItemSCMLink refreshes the details of a specific SCM link from the provider
func (s *SyncService) RefreshItemSCMLink(ctx context.Context, linkID int) error {
	// Get link info including connection ID for proper credential resolution
	var itemID, repoID, connectionID int
	var linkType models.SCMLinkType
	var externalID, repositoryName string

	err := s.db.QueryRowContext(ctx, `
		SELECT isl.item_id, isl.workspace_repository_id, isl.link_type, isl.external_id,
			   wr.repository_name, wr.workspace_scm_connection_id
		FROM item_scm_links isl
		JOIN workspace_repositories wr ON wr.id = isl.workspace_repository_id
		WHERE isl.id = ?
	`, linkID).Scan(&itemID, &repoID, &linkType, &externalID, &repositoryName, &connectionID)
	if err != nil {
		return fmt.Errorf("failed to get link info: %w", err)
	}

	// Use CredentialResolver to properly handle workspace-level OAuth tokens and GitHub Apps
	credResolver := NewCredentialResolver(s.db, s.encryption)
	provider, err := credResolver.GetProviderForConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Parse owner/repo
	parts := strings.SplitN(repositoryName, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository name format: %s", repositoryName)
	}
	owner, repo := parts[0], parts[1]

	switch linkType {
	case models.SCMLinkTypePullRequest:
		prNumber, _ := strconv.Atoi(externalID)
		var pr *PullRequest
		pr, err = provider.GetPullRequest(ctx, owner, repo, prNumber)
		if err != nil {
			return fmt.Errorf("failed to get PR: %w", err)
		}

		state := models.SCMLinkStateOpen
		if pr.IsMerged {
			state = models.SCMLinkStateMerged
		} else if pr.State == "closed" {
			state = models.SCMLinkStateClosed
		}

		_, err = s.db.ExecContext(ctx, `
			UPDATE item_scm_links SET
				external_url = ?, title = ?, state = ?,
				author_external_id = ?, author_name = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, pr.URL, pr.Title, state, pr.Author.ID, pr.Author.Name, linkID)
		return err

	case models.SCMLinkTypeCommit:
		var commit *Commit
		commit, err = provider.GetCommit(ctx, owner, repo, externalID)
		if err != nil {
			return fmt.Errorf("failed to get commit: %w", err)
		}

		// Get first line of commit message as title
		title := strings.SplitN(commit.Message, "\n", 2)[0]

		_, err = s.db.ExecContext(ctx, `
			UPDATE item_scm_links SET
				external_url = ?, title = ?,
				author_external_id = ?, author_name = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, commit.URL, title, commit.Author.ID, commit.Author.Name, linkID)
		return err

	case models.SCMLinkTypeBranch:
		// Branches don't have much metadata to refresh
		// Just update the timestamp
		_, err = s.db.ExecContext(ctx, `
			UPDATE item_scm_links SET updated_at = CURRENT_TIMESTAMP WHERE id = ?
		`, linkID)
		return err
	}

	return nil
}

// CreateBranchForRepository creates a branch in a workspace repository.
// This method implements the plugins.SCMService interface.
// If userID is provided (> 0), it uses the user's personal OAuth token for OAuth providers.
func (s *SyncService) CreateBranchForRepository(ctx context.Context, workspaceRepoID int, branchName, baseBranch string, userID ...int) (string, error) {
	// Get user ID if provided
	var uid int
	if len(userID) > 0 {
		uid = userID[0]
	}
	// Get repository info
	var repositoryName, defaultBranch string
	var providerID int
	var baseURL sql.NullString
	var providerType models.SCMProviderType

	err := s.db.QueryRowContext(ctx, `
		SELECT wr.repository_name, wr.default_branch, wsc.scm_provider_id,
			   sp.base_url, sp.provider_type
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wr.id = ?
	`, workspaceRepoID).Scan(&repositoryName, &defaultBranch, &providerID, &baseURL, &providerType)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("workspace repository not found: %d", workspaceRepoID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get repository info: %w", err)
	}

	// Parse owner/repo from repository name
	parts := strings.SplitN(repositoryName, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid repository name format: %s", repositoryName)
	}
	owner, repo := parts[0], parts[1]

	// Use default branch if base branch not specified
	if baseBranch == "" {
		baseBranch = defaultBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
	}

	// Get provider instance using CredentialResolver
	credResolver := NewCredentialResolver(s.db, s.encryption)
	connectionID := s.getConnectionIDForRepo(ctx, workspaceRepoID)

	var provider Provider
	if uid > 0 {
		// Use user-level credentials for OAuth providers
		provider, err = credResolver.GetProviderForUser(ctx, connectionID, uid)
		if err != nil {
			// Return specific error for unconnected users
			if errors.Is(err, ErrUserSCMNotConnected) {
				return "", err
			}
			// Fall back to connection-level credentials (for GitHub App, PAT)
			provider, err = credResolver.GetProviderForConnection(ctx, connectionID)
		}
	} else {
		// No user context - use connection-level credentials
		provider, err = credResolver.GetProviderForConnection(ctx, connectionID)
	}
	if err != nil {
		// Final fallback to old method
		provider, err = s.getProviderInstance(providerID)
		if err != nil {
			return "", fmt.Errorf("failed to get provider: %w", err)
		}
	}

	// Create the branch
	if err := provider.CreateBranch(ctx, owner, repo, branchName, baseBranch); err != nil {
		return "", fmt.Errorf("failed to create branch: %w", err)
	}

	// Build the branch URL based on provider type
	repoBaseURL := baseURL.String
	if repoBaseURL == "" {
		// Use default URLs for each provider type
		switch providerType {
		case models.SCMProviderTypeGitHub:
			repoBaseURL = "https://github.com"
		case models.SCMProviderTypeGitea:
			repoBaseURL = "https://gitea.com"
		}
	}

	// Both GitHub and Gitea use /tree/ for branch URLs
	branchURL := fmt.Sprintf("%s/%s/tree/%s", repoBaseURL, repositoryName, branchName)

	slog.Debug("Created branch", slog.String("component", "scm"), slog.String("branch", branchName), slog.String("repository", repositoryName))
	return branchURL, nil
}

// CreateItemSCMLink creates a link between an item and an SCM resource.
// This method implements the plugins.SCMService interface.
func (s *SyncService) CreateItemSCMLink(ctx context.Context, itemID, workspaceRepoID int, linkType, externalID, externalURL, title string) (int, error) {
	// Validate link type
	var scmLinkType models.SCMLinkType
	switch linkType {
	case "branch":
		scmLinkType = models.SCMLinkTypeBranch
	case "pull_request":
		scmLinkType = models.SCMLinkTypePullRequest
	case "commit":
		scmLinkType = models.SCMLinkTypeCommit
	default:
		return 0, fmt.Errorf("invalid link type: %s", linkType)
	}

	// Verify the item exists
	var exists bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM items WHERE id = ?)", itemID).Scan(&exists)
	if err != nil || !exists {
		return 0, fmt.Errorf("item not found: %d", itemID)
	}

	// Verify the workspace repository exists
	err = s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM workspace_repositories WHERE id = ?)", workspaceRepoID).Scan(&exists)
	if err != nil || !exists {
		return 0, fmt.Errorf("workspace repository not found: %d", workspaceRepoID)
	}

	// Check if link already exists
	var existingID int
	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM item_scm_links
		WHERE item_id = ? AND workspace_repository_id = ? AND link_type = ? AND external_id = ?
	`, itemID, workspaceRepoID, scmLinkType, externalID).Scan(&existingID)
	if err == nil {
		// Link already exists, return existing ID
		return existingID, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to check existing link: %w", err)
	}

	// Insert the new link
	// For pull requests, set initial state to 'open'
	var state string
	if scmLinkType == models.SCMLinkTypePullRequest {
		state = string(models.SCMLinkStateOpen)
	}

	var linkID int
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO item_scm_links (
			item_id, workspace_repository_id, link_type, external_id,
			external_url, title, state, detection_source, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'plugin', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, itemID, workspaceRepoID, scmLinkType, externalID, externalURL, title, state).Scan(&linkID)
	if err != nil {
		return 0, fmt.Errorf("failed to create item SCM link: %w", err)
	}

	slog.Debug("Created item link", slog.String("component", "scm"), slog.Int("link_id", linkID), slog.Int("item_id", itemID), slog.String("link_type", linkType), slog.String("external_id", externalID))
	return linkID, nil
}

// CreatePullRequestForRepository creates a pull request in a workspace repository.
// If userID is provided (> 0), it uses the user's personal OAuth token for OAuth providers.
func (s *SyncService) CreatePullRequestForRepository(ctx context.Context, workspaceRepoID int, opts CreatePROptions, userID ...int) (*PullRequest, string, error) {
	// Get user ID if provided
	var uid int
	if len(userID) > 0 {
		uid = userID[0]
	}

	// Get repository info
	var repositoryName, defaultBranch string
	var providerID int
	var baseURL sql.NullString
	var providerType models.SCMProviderType

	err := s.db.QueryRowContext(ctx, `
		SELECT wr.repository_name, wr.default_branch, wsc.scm_provider_id,
			   sp.base_url, sp.provider_type
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wr.id = ?
	`, workspaceRepoID).Scan(&repositoryName, &defaultBranch, &providerID, &baseURL, &providerType)
	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("workspace repository not found: %d", workspaceRepoID)
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to get repository info: %w", err)
	}

	// Parse owner/repo from repository name
	parts := strings.SplitN(repositoryName, "/", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid repository name format: %s", repositoryName)
	}
	owner, repo := parts[0], parts[1]

	// Use default branch if base branch not specified
	if opts.BaseBranch == "" {
		opts.BaseBranch = defaultBranch
		if opts.BaseBranch == "" {
			opts.BaseBranch = "main"
		}
	}

	// Get provider instance using CredentialResolver
	credResolver := NewCredentialResolver(s.db, s.encryption)
	connectionID := s.getConnectionIDForRepo(ctx, workspaceRepoID)

	var provider Provider
	if uid > 0 {
		// Use user-level credentials for OAuth providers
		provider, err = credResolver.GetProviderForUser(ctx, connectionID, uid)
		if err != nil {
			// Return specific error for unconnected users
			if errors.Is(err, ErrUserSCMNotConnected) {
				return nil, "", err
			}
			// Fall back to connection-level credentials (for GitHub App, PAT)
			provider, err = credResolver.GetProviderForConnection(ctx, connectionID)
		}
	} else {
		// No user context - use connection-level credentials
		provider, err = credResolver.GetProviderForConnection(ctx, connectionID)
	}
	if err != nil {
		// Final fallback to old method
		provider, err = s.getProviderInstance(providerID)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get provider: %w", err)
		}
	}

	// Create the pull request
	pr, err := provider.CreatePullRequest(ctx, owner, repo, opts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create pull request: %w", err)
	}

	// Build the PR URL based on provider type
	repoBaseURL := baseURL.String
	if repoBaseURL == "" {
		switch providerType {
		case models.SCMProviderTypeGitHub:
			repoBaseURL = "https://github.com"
		case models.SCMProviderTypeGitea:
			repoBaseURL = "https://gitea.com"
		}
	}

	prURL := fmt.Sprintf("%s/%s/pull/%d", repoBaseURL, repositoryName, pr.Number)

	slog.Debug("Created pull request", slog.String("component", "scm"), slog.Int("pr_number", pr.Number), slog.String("repository", repositoryName))
	return pr, prURL, nil
}

// getConnectionIDForRepo gets the workspace_scm_connection ID for a repository
func (s *SyncService) getConnectionIDForRepo(ctx context.Context, workspaceRepoID int) int {
	var connID int
	err := s.db.QueryRowContext(ctx, `
		SELECT workspace_scm_connection_id FROM workspace_repositories WHERE id = ?
	`, workspaceRepoID).Scan(&connID)
	if err != nil {
		return 0
	}
	return connID
}

// RefreshItemSCMLinkForUser refreshes a specific SCM link using the user's personal credentials.
// For OAuth connections, this uses the user's personal OAuth token instead of the workspace-level token.
func (s *SyncService) RefreshItemSCMLinkForUser(ctx context.Context, linkID, userID int) error {
	// Get link info including connection ID for proper credential resolution
	var itemID, repoID, connectionID int
	var linkType models.SCMLinkType
	var externalID, repositoryName string

	err := s.db.QueryRowContext(ctx, `
		SELECT isl.item_id, isl.workspace_repository_id, isl.link_type, isl.external_id,
			   wr.repository_name, wr.workspace_scm_connection_id
		FROM item_scm_links isl
		JOIN workspace_repositories wr ON wr.id = isl.workspace_repository_id
		WHERE isl.id = ?
	`, linkID).Scan(&itemID, &repoID, &linkType, &externalID, &repositoryName, &connectionID)
	if err != nil {
		return fmt.Errorf("failed to get link info: %w", err)
	}

	// Use CredentialResolver with user-specific credentials
	credResolver := NewCredentialResolver(s.db, s.encryption)
	provider, err := credResolver.GetProviderForUser(ctx, connectionID, userID)
	if err != nil {
		return fmt.Errorf("failed to get provider for user: %w", err)
	}

	// Parse owner/repo
	parts := strings.SplitN(repositoryName, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository name format: %s", repositoryName)
	}
	owner, repo := parts[0], parts[1]

	switch linkType {
	case models.SCMLinkTypePullRequest:
		prNumber, _ := strconv.Atoi(externalID)
		pr, err := provider.GetPullRequest(ctx, owner, repo, prNumber)
		if err != nil {
			return fmt.Errorf("failed to get PR: %w", err)
		}

		state := models.SCMLinkStateOpen
		if pr.IsMerged {
			state = models.SCMLinkStateMerged
		} else if pr.State == "closed" {
			state = models.SCMLinkStateClosed
		}

		_, err = s.db.ExecContext(ctx, `
			UPDATE item_scm_links SET
				external_url = ?, title = ?, state = ?,
				author_external_id = ?, author_name = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, pr.URL, pr.Title, state, pr.Author.ID, pr.Author.Name, linkID)
		return err

	case models.SCMLinkTypeCommit:
		commit, err := provider.GetCommit(ctx, owner, repo, externalID)
		if err != nil {
			return fmt.Errorf("failed to get commit: %w", err)
		}

		title := strings.SplitN(commit.Message, "\n", 2)[0]

		_, err = s.db.ExecContext(ctx, `
			UPDATE item_scm_links SET
				external_url = ?, title = ?,
				author_external_id = ?, author_name = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, commit.URL, title, commit.Author.ID, commit.Author.Name, linkID)
		return err

	case models.SCMLinkTypeBranch:
		_, err = s.db.ExecContext(ctx, `
			UPDATE item_scm_links SET updated_at = CURRENT_TIMESTAMP WHERE id = ?
		`, linkID)
		return err
	}

	return nil
}

// RefreshOAuthLinksForItem refreshes all non-merged PR links for an item that use OAuth connections,
// using the specified user's personal OAuth token.
func (s *SyncService) RefreshOAuthLinksForItem(ctx context.Context, itemID, userID int) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT isl.id
		FROM item_scm_links isl
		JOIN workspace_repositories wr ON wr.id = isl.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE isl.item_id = ?
		  AND isl.link_type = 'pull_request'
		  AND (isl.state IS NULL OR isl.state != 'merged')
		  AND sp.auth_method = 'oauth'
	`, itemID)
	if err != nil {
		return fmt.Errorf("failed to query OAuth PR links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var linkIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		linkIDs = append(linkIDs, id)
	}

	if len(linkIDs) == 0 {
		return nil
	}

	slog.Debug("Refreshing OAuth PR links for item", slog.String("component", "scm"), slog.Int("item_id", itemID), slog.Int("user_id", userID), slog.Int("count", len(linkIDs)))

	for _, linkID := range linkIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := s.RefreshItemSCMLinkForUser(ctx, linkID, userID); err != nil {
			slog.Warn("Failed to refresh OAuth PR link for user", slog.String("component", "scm"), slog.Int("link_id", linkID), slog.Int("user_id", userID), slog.Any("error", err))
			// Continue with other links
		}
	}

	return nil
}

// RefreshAllPRLinkStates refreshes the state of all non-merged PR links.
// This should be called periodically (e.g., every 5 minutes) by the scheduler.
func (s *SyncService) RefreshAllPRLinkStates(ctx context.Context) error {
	// Query all PR links that aren't already merged (merged is a final state)
	// Skip links from OAuth connections — those are refreshed on-demand per user
	rows, err := s.db.QueryContext(ctx, `
		SELECT isl.id
		FROM item_scm_links isl
		JOIN workspace_repositories wr ON wr.id = isl.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE isl.link_type = 'pull_request'
		AND (isl.state IS NULL OR isl.state != 'merged')
		AND sp.auth_method != 'oauth'
	`)
	if err != nil {
		return fmt.Errorf("failed to query PR links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var linkIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		linkIDs = append(linkIDs, id)
	}

	if len(linkIDs) == 0 {
		return nil
	}

	slog.Debug("Refreshing state for PR links", slog.String("component", "scm"), slog.Int("count", len(linkIDs)))

	var refreshErrors int
	for _, linkID := range linkIDs {
		// Check context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := s.RefreshItemSCMLink(ctx, linkID); err != nil {
			slog.Error("Failed to refresh PR link", slog.String("component", "scm"), slog.Int("link_id", linkID), slog.Any("error", err))
			refreshErrors++
			// Continue with other links even if one fails
		}
	}

	if refreshErrors > 0 {
		slog.Warn("Completed PR state refresh with errors", slog.String("component", "scm"), slog.Int("errors", refreshErrors), slog.Int("total_links", len(linkIDs)))
	}

	return nil
}
