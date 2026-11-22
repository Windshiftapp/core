package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// SCMWorkspaceRepository contains persistence helpers for workspace-level
// SCM connections, linked repositories, and the related provider lookups
// used by the workspace SCM settings surface.
type SCMWorkspaceRepository struct {
	db database.Database
}

// ItemSCMLinkSummary is the source-control projection used by item briefings.
// ExternalID holds the PR number, commit SHA, or branch name depending on
// LinkType; Title and State are empty when the link has none stored.
type ItemSCMLinkSummary struct {
	LinkType   models.SCMLinkType
	ExternalID string
	Title      string
	State      string
}

func NewSCMWorkspaceRepository(db database.Database) *SCMWorkspaceRepository {
	return &SCMWorkspaceRepository{db: db}
}

// ListItemSCMLinkSummaries returns source-control links attached to an item.
func (r *SCMWorkspaceRepository) ListItemSCMLinkSummaries(itemID int) ([]ItemSCMLinkSummary, error) {
	rows, err := r.db.Query(`
		SELECT link_type, external_id, COALESCE(title, ''), COALESCE(state, '')
		FROM item_scm_links
		WHERE item_id = ?
		ORDER BY id
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("list SCM links for item %d: %w", itemID, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]ItemSCMLinkSummary, 0)
	for rows.Next() {
		var summary ItemSCMLinkSummary
		if err := rows.Scan(&summary.LinkType, &summary.ExternalID, &summary.Title, &summary.State); err != nil {
			return nil, fmt.Errorf("scan SCM link for item %d: %w", itemID, err)
		}
		out = append(out, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate SCM links for item %d: %w", itemID, err)
	}
	return out, nil
}

// SCMWorkspaceConnection represents a workspace SCM connection joined with
// provider metadata and the linked-repository count.
type SCMWorkspaceConnection struct {
	ID                   int                      `json:"id"`
	WorkspaceID          int                      `json:"workspace_id"`
	WorkspaceName        string                   `json:"workspace_name"`
	SCMProviderID        int                      `json:"scm_provider_id"`
	ProviderName         string                   `json:"provider_name"`
	ProviderType         models.SCMProviderType   `json:"provider_type"`
	ProviderSlug         string                   `json:"provider_slug"`
	Enabled              bool                     `json:"enabled"`
	SmartCommitsEnabled  bool                     `json:"smart_commits_enabled"`
	DefaultBranchPattern string                   `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       string                   `json:"item_key_pattern,omitempty"`
	RepositoryCount      int                      `json:"repository_count"`
	CreatedBy            *int                     `json:"created_by,omitempty"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`
	Repositories         []SCMLinkedRepository    `json:"repositories,omitempty"`
	AuthStatus           *SCMConnectionAuthStatus `json:"auth_status,omitempty"`
}

// SCMConnectionAuthStatus is the credential-presence summary rendered by the
// workspace connection list. It never contains credential material.
type SCMConnectionAuthStatus struct {
	AuthMethod        models.SCMAuthMethod `json:"auth_method"`
	IsAuthenticated   bool                 `json:"is_authenticated"`
	ProviderSlug      string               `json:"provider_slug"`
	HasWorkspaceToken bool                 `json:"has_workspace_token,omitempty"`
	HasUserToken      bool                 `json:"has_user_token,omitempty"`
	SCMUsername       string               `json:"scm_username,omitempty"`
	TokenExpiresAt    *time.Time           `json:"token_expires_at,omitempty"`
	TokenExpired      *bool                `json:"token_expired,omitempty"`
	HasWorkspacePAT   bool                 `json:"has_workspace_pat,omitempty"`
	HasProviderPAT    bool                 `json:"has_provider_pat,omitempty"`
	HasGitHubAppKey   bool                 `json:"has_github_app_key,omitempty"`
	AuthSource        string               `json:"auth_source,omitempty"`
}

// SCMLinkedRepository represents a repository linked to a workspace SCM connection.
type SCMLinkedRepository struct {
	ID                       int        `json:"id"`
	WorkspaceSCMConnectionID int        `json:"workspace_scm_connection_id"`
	RepositoryExternalID     string     `json:"repository_external_id"`
	RepositoryName           string     `json:"repository_name"`
	RepositoryURL            string     `json:"repository_url"`
	DefaultBranch            string     `json:"default_branch"`
	IsActive                 bool       `json:"is_active"`
	MilestoneTagPattern      string     `json:"milestone_tag_pattern"`
	MilestoneBranchPattern   string     `json:"milestone_branch_pattern"`
	LastSyncedAt             *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

// AvailableSCMProvider is an enabled SCM provider a workspace may connect
// to, plus whether the workspace already has a connection for it.
type AvailableSCMProvider struct {
	ID           int                    `json:"id"`
	Slug         string                 `json:"slug"`
	Name         string                 `json:"name"`
	ProviderType models.SCMProviderType `json:"provider_type"`
	AuthMethod   models.SCMAuthMethod   `json:"auth_method"`
	IsConnected  bool                   `json:"is_connected"`
}

// SCMProviderOAuthConfig is the provider configuration needed to start an
// OAuth authorization flow. Optional columns surface as empty strings.
type SCMProviderOAuthConfig struct {
	ProviderType models.SCMProviderType
	AuthMethod   models.SCMAuthMethod
	ClientID     string
	BaseURL      string
	Scopes       string
	Slug         string
}

// SCMConnectionAuthInfo describes workspace-level credential presence for a connection.
type SCMConnectionAuthInfo struct {
	WorkspaceID         int
	ProviderID          int
	HasOAuthToken       bool
	HasPAT              bool
	OAuthTokenExpiresAt *time.Time
}

// SCMProviderAuthInfo describes provider-level credential presence.
type SCMProviderAuthInfo struct {
	AuthMethod      models.SCMAuthMethod
	HasPAT          bool
	HasGitHubAppKey bool
	Slug            string
}

const scmWorkspaceConnectionSelect = `
	SELECT
		wsc.id, wsc.workspace_id, w.name, wsc.scm_provider_id, wsc.enabled,
		wsc.smart_commits_enabled,
		wsc.default_branch_pattern, wsc.item_key_pattern,
		wsc.created_by, wsc.created_at, wsc.updated_at,
		sp.name, sp.provider_type, sp.slug,
		(SELECT COUNT(*) FROM workspace_repositories wr WHERE wr.workspace_scm_connection_id = wsc.id) as repo_count
	FROM workspace_scm_connections wsc
	JOIN workspaces w ON w.id = wsc.workspace_id
	JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
`

type scmWorkspaceScanner interface {
	Scan(dest ...any) error
}

func scanSCMWorkspaceConnection(scanner scmWorkspaceScanner) (SCMWorkspaceConnection, error) {
	var conn SCMWorkspaceConnection
	var defaultBranchPattern, itemKeyPattern sql.NullString
	var createdBy sql.NullInt64

	if err := scanner.Scan(
		&conn.ID, &conn.WorkspaceID, &conn.WorkspaceName, &conn.SCMProviderID, &conn.Enabled,
		&conn.SmartCommitsEnabled,
		&defaultBranchPattern, &itemKeyPattern,
		&createdBy, &conn.CreatedAt, &conn.UpdatedAt,
		&conn.ProviderName, &conn.ProviderType, &conn.ProviderSlug,
		&conn.RepositoryCount,
	); err != nil {
		return conn, err
	}

	conn.DefaultBranchPattern = defaultBranchPattern.String
	conn.ItemKeyPattern = itemKeyPattern.String
	if createdBy.Valid {
		cb := int(createdBy.Int64)
		conn.CreatedBy = &cb
	}
	return conn, nil
}

func scanSCMLinkedRepository(scanner scmWorkspaceScanner) (SCMLinkedRepository, error) {
	var repo SCMLinkedRepository
	var lastSyncedAt sql.NullTime
	if err := scanner.Scan(
		&repo.ID, &repo.WorkspaceSCMConnectionID, &repo.RepositoryExternalID,
		&repo.RepositoryName, &repo.RepositoryURL, &repo.DefaultBranch,
		&repo.IsActive, &repo.MilestoneTagPattern, &repo.MilestoneBranchPattern,
		&lastSyncedAt, &repo.CreatedAt, &repo.UpdatedAt,
	); err != nil {
		return repo, err
	}
	if lastSyncedAt.Valid {
		repo.LastSyncedAt = &lastSyncedAt.Time
	}
	return repo, nil
}

// ListConnections returns all SCM connections for a workspace.
func (r *SCMWorkspaceRepository) ListConnections(workspaceID int) ([]SCMWorkspaceConnection, error) {
	rows, err := r.db.Query(scmWorkspaceConnectionSelect+`
		WHERE wsc.workspace_id = ?
		ORDER BY sp.name
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	connections := []SCMWorkspaceConnection{}
	for rows.Next() {
		conn, err := scanSCMWorkspaceConnection(rows)
		if err != nil {
			slog.Error("failed to scan connection", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}
		connections = append(connections, conn)
	}
	return connections, rows.Err()
}

// ListConnectionsForWorkspaces returns every SCM connection belonging to the
// supplied accessible workspace IDs in one query. An empty scope fails closed.
func (r *SCMWorkspaceRepository) ListConnectionsForWorkspaces(workspaceIDs []int) ([]SCMWorkspaceConnection, error) {
	if len(workspaceIDs) == 0 {
		return []SCMWorkspaceConnection{}, nil
	}
	placeholders := make([]string, len(workspaceIDs))
	args := make([]any, len(workspaceIDs))
	for i, workspaceID := range workspaceIDs {
		placeholders[i] = "?"
		args[i] = workspaceID
	}
	rows, err := r.db.Query(scmWorkspaceConnectionSelect+`
		WHERE wsc.workspace_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY w.name, sp.name
	`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	connections := []SCMWorkspaceConnection{}
	for rows.Next() {
		connection, err := scanSCMWorkspaceConnection(rows)
		if err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	return connections, rows.Err()
}

// GetConnectionByID returns a single connection (with provider metadata) by id.
func (r *SCMWorkspaceRepository) GetConnectionByID(id int) (*SCMWorkspaceConnection, error) {
	row := r.db.QueryRow(scmWorkspaceConnectionSelect+`
		WHERE wsc.id = ?
	`, id)
	conn, err := scanSCMWorkspaceConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &conn, nil
}

// GetConnectionWorkspaceAndProvider returns the owning workspace and
// provider ids for a connection.
func (r *SCMWorkspaceRepository) GetConnectionWorkspaceAndProvider(connID int) (workspaceID, providerID int, err error) {
	err = r.db.QueryRow(`
		SELECT workspace_id, scm_provider_id FROM workspace_scm_connections WHERE id = ?
	`, connID).Scan(&workspaceID, &providerID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrNotFound
	}
	return workspaceID, providerID, err
}

// CreateConnection inserts a new workspace SCM connection and returns its id.
// Empty patterns are stored as NULL.
func (r *SCMWorkspaceRepository) CreateConnection(workspaceID, scmProviderID int, defaultBranchPattern, itemKeyPattern string, createdBy *int) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO workspace_scm_connections (
			workspace_id, scm_provider_id, enabled,
			default_branch_pattern, item_key_pattern, created_by
		) VALUES (?, ?, true, ?, ?, ?) RETURNING id
	`, workspaceID, scmProviderID,
		scmNullString(defaultBranchPattern), scmNullString(itemKeyPattern), createdBy).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// UpdateConnection updates the mutable settings of a connection. Empty
// patterns are stored as NULL.
func (r *SCMWorkspaceRepository) UpdateConnection(connID int, enabled, smartCommitsEnabled bool, defaultBranchPattern, itemKeyPattern string) error {
	_, err := r.db.ExecWrite(`
		UPDATE workspace_scm_connections SET
			enabled = ?,
			smart_commits_enabled = ?,
			default_branch_pattern = ?,
			item_key_pattern = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, enabled, smartCommitsEnabled, scmNullString(defaultBranchPattern), scmNullString(itemKeyPattern), connID)
	return err
}

// DeleteConnection deletes a connection (cascade removes repositories and item links).
func (r *SCMWorkspaceRepository) DeleteConnection(connID int) error {
	_, err := r.db.ExecWrite("DELETE FROM workspace_scm_connections WHERE id = ?", connID)
	return err
}

// GetProviderEnabled reports whether an SCM provider exists and is enabled.
func (r *SCMWorkspaceRepository) GetProviderEnabled(providerID int) (bool, error) {
	var enabled bool
	err := r.db.QueryRow("SELECT enabled FROM scm_providers WHERE id = ?", providerID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	return enabled, err
}

// LinkedRepositoryExternalIDs returns the set of repository external ids
// already linked to a connection.
func (r *SCMWorkspaceRepository) LinkedRepositoryExternalIDs(connID int) (map[string]bool, error) {
	linked := make(map[string]bool)
	rows, err := r.db.Query(`
		SELECT repository_external_id FROM workspace_repositories
		WHERE workspace_scm_connection_id = ?
	`, connID)
	if err != nil {
		return linked, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var extID string
		if rows.Scan(&extID) == nil {
			linked[extID] = true
		}
	}
	return linked, rows.Err()
}

// ListLinkedRepositories returns the repositories linked to a connection.
func (r *SCMWorkspaceRepository) ListLinkedRepositories(connID int) ([]SCMLinkedRepository, error) {
	rows, err := r.db.Query(`
		SELECT id, workspace_scm_connection_id, repository_external_id,
			   repository_name, repository_url, default_branch,
			   is_active, milestone_tag_pattern, milestone_branch_pattern,
			   last_synced_at, created_at, updated_at
		FROM workspace_repositories
		WHERE workspace_scm_connection_id = ?
		ORDER BY repository_name
	`, connID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	repos := []SCMLinkedRepository{}
	for rows.Next() {
		repo, err := scanSCMLinkedRepository(rows)
		if err != nil {
			slog.Error("failed to scan repository", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}
		repos = append(repos, repo)
	}
	return repos, rows.Err()
}

// LinkedRepositoryIdentity is the minimal repository identity release callers
// resolve before creating milestone releases.
type LinkedRepositoryIdentity struct {
	ID             int
	RepositoryName string
}

// ResolveLinkedRepository finds a repository linked to a connection by id or
// exact name (id wins when both are supplied). Returns ErrNotFound when
// neither identifies a stored row.
func (r *SCMWorkspaceRepository) ResolveLinkedRepository(connID, repositoryID int, repositoryName string) (*LinkedRepositoryIdentity, error) {
	var identity LinkedRepositoryIdentity
	var err error
	switch {
	case repositoryID > 0:
		err = r.db.QueryRow(`
			SELECT id, repository_name FROM workspace_repositories
			WHERE id = ? AND workspace_scm_connection_id = ?
		`, repositoryID, connID).Scan(&identity.ID, &identity.RepositoryName)
	case repositoryName != "":
		err = r.db.QueryRow(`
			SELECT id, repository_name FROM workspace_repositories
			WHERE workspace_scm_connection_id = ? AND repository_name = ?
		`, connID, repositoryName).Scan(&identity.ID, &identity.RepositoryName)
	default:
		return nil, ErrNotFound
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("resolve linked repository: %w", err)
	}
	return &identity, nil
}

// ListLinkedRepositoriesForWorkspace loads repositories for every connection
// in a workspace with one read. Callers group the returned rows by
// WorkspaceSCMConnectionID.
func (r *SCMWorkspaceRepository) ListLinkedRepositoriesForWorkspace(workspaceID int) ([]SCMLinkedRepository, error) {
	rows, err := r.db.Query(`
		SELECT wr.id, wr.workspace_scm_connection_id, wr.repository_external_id,
		       wr.repository_name, wr.repository_url, wr.default_branch,
		       wr.is_active, wr.milestone_tag_pattern, wr.milestone_branch_pattern,
		       wr.last_synced_at, wr.created_at, wr.updated_at
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wsc.workspace_id = ?
		ORDER BY wr.workspace_scm_connection_id, wr.repository_name
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	repositories := []SCMLinkedRepository{}
	for rows.Next() {
		repository, err := scanSCMLinkedRepository(rows)
		if err != nil {
			return nil, err
		}
		repositories = append(repositories, repository)
	}
	return repositories, rows.Err()
}

// ListConnectionAuthStatusesForWorkspace loads credential-presence summaries
// for every workspace connection in one read. It joins the current user's
// optional OAuth token but never selects or returns credential material.
func (r *SCMWorkspaceRepository) ListConnectionAuthStatusesForWorkspace(workspaceID, userID int) (map[int]*SCMConnectionAuthStatus, error) {
	rows, err := r.db.Query(`
		SELECT wsc.id, sp.auth_method, sp.slug,
		       CASE WHEN wsc.oauth_access_token_encrypted IS NOT NULL AND wsc.oauth_access_token_encrypted != '' THEN 1 ELSE 0 END,
		       wsc.oauth_token_expires_at,
		       CASE WHEN wsc.personal_access_token_encrypted IS NOT NULL AND wsc.personal_access_token_encrypted != '' THEN 1 ELSE 0 END,
		       CASE WHEN sp.personal_access_token_encrypted IS NOT NULL AND sp.personal_access_token_encrypted != '' THEN 1 ELSE 0 END,
		       CASE WHEN sp.github_app_private_key_encrypted IS NOT NULL AND sp.github_app_private_key_encrypted != '' THEN 1 ELSE 0 END,
		       CASE WHEN user_token.oauth_access_token_encrypted IS NOT NULL AND user_token.oauth_access_token_encrypted != '' THEN 1 ELSE 0 END,
		       COALESCE(user_token.scm_username, '')
		FROM workspace_scm_connections wsc
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		LEFT JOIN user_scm_oauth_tokens user_token
		  ON user_token.user_id = ? AND user_token.scm_provider_id = wsc.scm_provider_id
		WHERE wsc.workspace_id = ?
	`, userID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	statuses := map[int]*SCMConnectionAuthStatus{}
	for rows.Next() {
		var connectionID int
		var status SCMConnectionAuthStatus
		var expiresAt sql.NullTime
		var hasWorkspaceOAuth, hasWorkspacePAT, hasProviderPAT, hasGitHubAppKey, hasUserOAuth bool
		if err := rows.Scan(
			&connectionID, &status.AuthMethod, &status.ProviderSlug,
			&hasWorkspaceOAuth, &expiresAt, &hasWorkspacePAT, &hasProviderPAT,
			&hasGitHubAppKey, &hasUserOAuth, &status.SCMUsername,
		); err != nil {
			return nil, err
		}
		switch status.AuthMethod {
		case models.SCMAuthMethodOAuth:
			status.HasWorkspaceToken = hasWorkspaceOAuth
			status.HasUserToken = hasUserOAuth
			status.IsAuthenticated = hasWorkspaceOAuth || hasUserOAuth
			if expiresAt.Valid {
				status.TokenExpiresAt = &expiresAt.Time
				expired := expiresAt.Time.Before(time.Now())
				status.TokenExpired = &expired
			}
		case models.SCMAuthMethodPAT:
			status.HasWorkspacePAT = hasWorkspacePAT
			status.HasProviderPAT = hasProviderPAT
			status.IsAuthenticated = hasWorkspacePAT || hasProviderPAT
		case models.SCMAuthMethodGitHubApp:
			status.HasGitHubAppKey = hasGitHubAppKey
			status.IsAuthenticated = hasGitHubAppKey
			status.AuthSource = "provider"
		}
		statusCopy := status
		statuses[connectionID] = &statusCopy
	}
	return statuses, rows.Err()
}

// LinkRepository links a repository to a connection and returns the new row id.
func (r *SCMWorkspaceRepository) LinkRepository(connID int, externalID, name, url, defaultBranch string) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO workspace_repositories (
			workspace_scm_connection_id, repository_external_id,
			repository_name, repository_url, default_branch, is_active
		) VALUES (?, ?, ?, ?, ?, true) RETURNING id
	`, connID, externalID, name, url, defaultBranch).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// GetRepositoryByID returns a linked repository by id.
func (r *SCMWorkspaceRepository) GetRepositoryByID(id int) (*SCMLinkedRepository, error) {
	row := r.db.QueryRow(`
		SELECT id, workspace_scm_connection_id, repository_external_id,
			   repository_name, repository_url, default_branch,
			   is_active, milestone_tag_pattern, milestone_branch_pattern,
			   last_synced_at, created_at, updated_at
		FROM workspace_repositories WHERE id = ?
	`, id)
	repo, err := scanSCMLinkedRepository(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &repo, nil
}

// GetRepositoryWorkspaceID resolves the workspace owning a linked
// repository via its connection.
func (r *SCMWorkspaceRepository) GetRepositoryWorkspaceID(repoID int) (int, error) {
	var workspaceID int
	err := r.db.QueryRow(`
		SELECT wsc.workspace_id FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wr.id = ?
	`, repoID).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return workspaceID, err
}

// DeleteRepository removes a linked repository.
func (r *SCMWorkspaceRepository) DeleteRepository(repoID int) error {
	_, err := r.db.ExecWrite("DELETE FROM workspace_repositories WHERE id = ?", repoID)
	return err
}

// UpdateRepositoryPatterns updates the milestone automation patterns of a
// linked repository. Nil pointers leave the column unchanged; at least one
// field must be set.
func (r *SCMWorkspaceRepository) UpdateRepositoryPatterns(repoID int, milestoneTagPattern, milestoneBranchPattern *string) error {
	sets := []string{"updated_at = CURRENT_TIMESTAMP"}
	args := []any{}
	if milestoneTagPattern != nil {
		sets = append(sets, "milestone_tag_pattern = ?")
		args = append(args, *milestoneTagPattern)
	}
	if milestoneBranchPattern != nil {
		sets = append(sets, "milestone_branch_pattern = ?")
		args = append(args, *milestoneBranchPattern)
	}
	if len(args) == 0 {
		return ErrInvalidInput
	}
	args = append(args, repoID)

	query := "UPDATE workspace_repositories SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	_, err := r.db.ExecWrite(query, args...)
	return err
}

// ListAvailableProviders returns all enabled SCM providers a workspace may
// connect to (honoring workspace restriction allowlists), with their
// connected state for that workspace.
func (r *SCMWorkspaceRepository) ListAvailableProviders(workspaceID int) ([]AvailableSCMProvider, error) {
	rows, err := r.db.Query(`
		SELECT sp.id, sp.slug, sp.name, sp.provider_type, sp.auth_method,
			   sp.workspace_restriction_mode,
			   CASE WHEN wsc.id IS NOT NULL THEN 1 ELSE 0 END as is_connected
		FROM scm_providers sp
		LEFT JOIN workspace_scm_connections wsc
			ON wsc.scm_provider_id = sp.id AND wsc.workspace_id = ?
		WHERE sp.enabled = true
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
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	providers := []AvailableSCMProvider{}
	for rows.Next() {
		var p AvailableSCMProvider
		var isConnected int
		var restrictionMode sql.NullString
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.ProviderType, &p.AuthMethod, &restrictionMode, &isConnected); err != nil {
			slog.Error("failed to scan provider", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}
		p.IsConnected = isConnected == 1
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

// GetProviderOAuthConfig returns the provider settings needed to start an
// OAuth flow. Optional columns come back as empty strings.
func (r *SCMWorkspaceRepository) GetProviderOAuthConfig(providerID int) (*SCMProviderOAuthConfig, error) {
	var cfg SCMProviderOAuthConfig
	var clientID, baseURL, scopes, slug sql.NullString
	err := r.db.QueryRow(`
		SELECT provider_type, auth_method, oauth_client_id, base_url, scopes, slug
		FROM scm_providers WHERE id = ?
	`, providerID).Scan(&cfg.ProviderType, &cfg.AuthMethod, &clientID, &baseURL, &scopes, &slug)
	if err != nil {
		return nil, err
	}
	cfg.ClientID = clientID.String
	cfg.BaseURL = baseURL.String
	cfg.Scopes = scopes.String
	cfg.Slug = slug.String
	return &cfg, nil
}

// CreateOAuthState stores a short-lived OAuth state token for a workspace flow.
func (r *SCMWorkspaceRepository) CreateOAuthState(providerID int, state, redirectURI string, userID, workspaceID int, expiresAt time.Time) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO scm_oauth_state (provider_id, state, redirect_uri, user_id, workspace_id, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, providerID, state, redirectURI, userID, workspaceID, expiresAt)
	return err
}

// GetProviderAuthMethod returns the auth method of an SCM provider.
func (r *SCMWorkspaceRepository) GetProviderAuthMethod(providerID int) (models.SCMAuthMethod, error) {
	var authMethod models.SCMAuthMethod
	err := r.db.QueryRow("SELECT auth_method FROM scm_providers WHERE id = ?", providerID).Scan(&authMethod)
	return authMethod, err
}

// SetConnectionPAT stores an encrypted personal access token on a connection.
func (r *SCMWorkspaceRepository) SetConnectionPAT(connID int, encryptedPAT string) error {
	_, err := r.db.ExecWrite(`
		UPDATE workspace_scm_connections SET
			personal_access_token_encrypted = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, encryptedPAT, connID)
	return err
}

// ClearConnectionCredentials removes all workspace-level credentials from a connection.
func (r *SCMWorkspaceRepository) ClearConnectionCredentials(connID int) error {
	_, err := r.db.ExecWrite(`
		UPDATE workspace_scm_connections SET
			oauth_access_token_encrypted = NULL,
			oauth_refresh_token_encrypted = NULL,
			oauth_token_expires_at = NULL,
			personal_access_token_encrypted = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, connID)
	return err
}

// GetConnectionAuthInfo returns workspace-level credential presence for a connection.
func (r *SCMWorkspaceRepository) GetConnectionAuthInfo(connID int) (*SCMConnectionAuthInfo, error) {
	var info SCMConnectionAuthInfo
	var oauthTokenEnc, patEnc sql.NullString
	var oauthExpiresAt sql.NullTime
	err := r.db.QueryRow(`
		SELECT workspace_id, scm_provider_id,
			   oauth_access_token_encrypted, personal_access_token_encrypted,
			   oauth_token_expires_at
		FROM workspace_scm_connections WHERE id = ?
	`, connID).Scan(&info.WorkspaceID, &info.ProviderID, &oauthTokenEnc, &patEnc, &oauthExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	info.HasOAuthToken = oauthTokenEnc.Valid && oauthTokenEnc.String != ""
	info.HasPAT = patEnc.Valid && patEnc.String != ""
	if oauthExpiresAt.Valid {
		info.OAuthTokenExpiresAt = &oauthExpiresAt.Time
	}
	return &info, nil
}

// GetProviderAuthInfo returns provider-level credential presence for a provider.
func (r *SCMWorkspaceRepository) GetProviderAuthInfo(providerID int) (*SCMProviderAuthInfo, error) {
	var info SCMProviderAuthInfo
	var patEnc, ghAppKeyEnc sql.NullString
	err := r.db.QueryRow(`
		SELECT auth_method, personal_access_token_encrypted, github_app_private_key_encrypted, slug
		FROM scm_providers WHERE id = ?
	`, providerID).Scan(&info.AuthMethod, &patEnc, &ghAppKeyEnc, &info.Slug)
	if err != nil {
		return nil, err
	}
	info.HasPAT = patEnc.Valid && patEnc.String != ""
	info.HasGitHubAppKey = ghAppKeyEnc.Valid && ghAppKeyEnc.String != ""
	return &info, nil
}

// GetUserOAuthTokenStatus reports whether a user has an OAuth token for a
// provider and the connected SCM username (nil when not recorded).
func (r *SCMWorkspaceRepository) GetUserOAuthTokenStatus(userID, providerID int) (hasOAuthToken bool, connectedUsername *string, err error) {
	var hasToken bool
	var scmUsername sql.NullString
	err = r.db.QueryRow(`
		SELECT CASE WHEN oauth_access_token_encrypted IS NOT NULL AND oauth_access_token_encrypted != '' THEN 1 ELSE 0 END,
		       scm_username
		FROM user_scm_oauth_tokens WHERE user_id = ? AND scm_provider_id = ?
	`, userID, providerID).Scan(&hasToken, &scmUsername)
	if err != nil {
		return false, nil, err
	}
	if scmUsername.Valid {
		return hasToken, &scmUsername.String, nil
	}
	return hasToken, nil, nil
}

// scmNullString stores empty strings as NULL.
func scmNullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
