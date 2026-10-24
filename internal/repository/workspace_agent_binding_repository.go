package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
)

// WorkspaceAgentBindingRepository persists workspace_agent_bindings rows.
type WorkspaceAgentBindingRepository struct {
	db workspaceAgentBindingStore
}

type workspaceAgentBindingStore interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	ExecWrite(query string, args ...any) (sql.Result, error)
	ExecWriteContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// NewWorkspaceAgentBindingRepository constructs a new repository.
func NewWorkspaceAgentBindingRepository(db database.Database) *WorkspaceAgentBindingRepository {
	return &WorkspaceAgentBindingRepository{db: db}
}

// NewWorkspaceAgentBindingRepositoryTx binds repository writes to an existing
// transaction. Agent Studio uses this to create an acting identity, its default
// workspace role, and the Draft profile atomically.
func NewWorkspaceAgentBindingRepositoryTx(tx database.Tx) *WorkspaceAgentBindingRepository {
	return &WorkspaceAgentBindingRepository{db: tx}
}

// AgentRunContext returns the workspace key and workspace-scoped item number
// for environment variables injected into the coding-agent container. itemID
// may be zero for future manual runs; in that case ItemKey is empty.
type AgentRunContext struct {
	WorkspaceKey string
	ItemNumber   int
	ItemKey      string
}

func (r *WorkspaceAgentBindingRepository) AgentRunContext(ctx context.Context, workspaceID, itemID int) (AgentRunContext, error) {
	var out AgentRunContext
	if itemID > 0 {
		err := r.db.QueryRowContext(ctx, `
			SELECT w.key, i.workspace_item_number
			FROM workspaces w
			JOIN items i ON i.workspace_id = w.id AND i.id = ?
			WHERE w.id = ?
		`, itemID, workspaceID).Scan(&out.WorkspaceKey, &out.ItemNumber)
		if err != nil {
			return out, fmt.Errorf("load agent run workspace/item context: %w", err)
		}
		out.ItemKey = fmt.Sprintf("%s-%d", out.WorkspaceKey, out.ItemNumber)
		return out, nil
	}
	err := r.db.QueryRowContext(ctx, `SELECT key FROM workspaces WHERE id = ?`, workspaceID).Scan(&out.WorkspaceKey)
	if err != nil {
		return out, fmt.Errorf("load agent run workspace context: %w", err)
	}
	return out, nil
}

// ErrBindingDuplicate is returned when a caller tries to create a second
// binding for the same (workspace, acting_user). The handler layer maps
// this to a 409 Conflict.
var ErrBindingDuplicate = errors.New("workspace agent binding: a binding for this acting user already exists in this workspace")

// Insert persists a new binding and returns its id. token_ttl_minutes
// defaults to 60 when caller passes <= 0; scopes default to an empty array
// (RunTokenService expands that to auth.DefaultCodingAgentRunScopes at mint
// time).
func (r *WorkspaceAgentBindingRepository) Insert(ctx context.Context, b *models.WorkspaceAgentBinding) (int, error) {
	if b.TokenTTLMinutes <= 0 {
		b.TokenTTLMinutes = 60
	}
	if b.ProfileType == "" {
		if b.TargetPoolID == nil {
			b.ProfileType = models.AgentProfileLegacy
		} else {
			b.ProfileType = models.AgentProfileCoding
		}
	}
	if b.Lifecycle == "" {
		b.Lifecycle = models.AgentLifecycleReady
	}
	if b.ProfileVersion <= 0 {
		b.ProfileVersion = 1
	}
	if b.IdentityClass == "" {
		if b.ActingUserKind == "agent" {
			b.IdentityClass = models.AgentIdentityUserOwned
		} else {
			b.IdentityClass = models.AgentIdentityCentralized
		}
	}
	scopesJSON, err := json.Marshal(b.TokenScopes)
	if err != nil {
		return 0, fmt.Errorf("marshal token scopes: %w", err)
	}
	capabilityGroupsJSON, err := json.Marshal(b.CapabilityGroups)
	if err != nil {
		return 0, fmt.Errorf("marshal capability groups: %w", err)
	}
	// Persist the primary repo onto the deprecated scalar columns (rollback
	// net for one release) — synthesized from b.Repos when the caller set the
	// new field, else taken from the legacy scalar fields the caller passed.
	repos := bindingReposToPersist(b)
	if p := primaryOf(repos); p != nil {
		b.RepoSlug = p.RepoSlug
		b.RepoBaseRef = p.RepoBaseRef
		b.SCMConnectionID = p.SCMConnectionID
	}
	// RETURNING id (not LastInsertId) for Postgres compatibility.
	var id int64
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO workspace_agent_bindings
			(workspace_id, acting_user_id, acting_user_kind,
			 profile_type, lifecycle, profile_version, identity_class, purpose, capability_groups_json,
			 repo_slug, repo_base_ref,
			 llm_connection_id, scm_connection_id, target_pool_id, token_scopes_json, token_ttl_minutes, max_runs_per_day, instructions, runner_image, created_by_user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		b.WorkspaceID, b.ActingUserID, b.ActingUserKind,
		b.ProfileType, b.Lifecycle, b.ProfileVersion, b.IdentityClass, b.Purpose, string(capabilityGroupsJSON),
		nullStringArg(b.RepoSlug), nullStringArg(b.RepoBaseRef),
		nullIntArg(b.LLMConnectionID), nullIntArg(b.SCMConnectionID), nullIntArg(b.TargetPoolID),
		string(scopesJSON), b.TokenTTLMinutes, b.MaxRunsPerDay,
		b.Instructions, nullStringArg(b.RunnerImage), b.CreatedByUserID,
	).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrBindingDuplicate
		}
		return 0, fmt.Errorf("insert binding: %w", err)
	}
	// Persist the binding's repos in the child table (WI-449). If this fails,
	// roll back the binding row so a half-written binding never persists.
	if len(repos) > 0 {
		if err := r.ReplaceBindingRepos(ctx, int(id), repos); err != nil {
			_, _ = r.Delete(ctx, int(id), b.WorkspaceID)
			return 0, fmt.Errorf("insert binding repos: %w", err)
		}
	}
	b.ID = int(id)
	b.Repos = repos
	return int(id), nil
}

// bindingReposToPersist returns the repos to write for a binding: b.Repos when
// the caller populated the new field, otherwise a single primary repo
// synthesized from the deprecated scalar RepoSlug/RepoBaseRef/SCMConnectionID
// fields (so direct Insert callers and pre-migration code stay working).
func bindingReposToPersist(b *models.WorkspaceAgentBinding) []models.BindingRepo {
	if len(b.Repos) > 0 {
		return b.Repos
	}
	if b.RepoSlug == "" {
		return nil
	}
	return []models.BindingRepo{{
		SCMConnectionID: b.SCMConnectionID,
		RepoSlug:        b.RepoSlug,
		RepoBaseRef:     b.RepoBaseRef,
		IsPrimary:       true,
		Position:        0,
	}}
}

func primaryOf(repos []models.BindingRepo) *models.BindingRepo {
	for i := range repos {
		if repos[i].IsPrimary {
			return &repos[i]
		}
	}
	if len(repos) > 0 {
		return &repos[0]
	}
	return nil
}

// Get loads a single binding by id, with its repos hydrated.
func (r *WorkspaceAgentBindingRepository) Get(ctx context.Context, id int) (*models.WorkspaceAgentBinding, error) {
	row := r.db.QueryRowContext(ctx, bindingSelectSQL+` WHERE id = ?`, id)
	b, err := scanBinding(row)
	if err != nil {
		return nil, err
	}
	if err := r.hydrateRepos(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

// ListForWorkspace returns every binding configured in the workspace, oldest
// first.
func (r *WorkspaceAgentBindingRepository) ListForWorkspace(ctx context.Context, workspaceID int) ([]*models.WorkspaceAgentBinding, error) {
	rows, err := r.db.QueryContext(ctx, bindingSelectSQL+` WHERE workspace_id = ? ORDER BY created_at ASC, id ASC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*models.WorkspaceAgentBinding
	for rows.Next() {
		b, err := scanBindingRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.hydrateReposBatch(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// FindByActingUser returns the binding for a specific (workspace, acting
// user) pair, if one exists. Returns (nil, nil) when not found — the
// assignee-change trigger calls this in the hot path and absence is the
// expected case.
func (r *WorkspaceAgentBindingRepository) FindByActingUser(ctx context.Context, workspaceID, actingUserID int) (*models.WorkspaceAgentBinding, error) {
	row := r.db.QueryRowContext(
		ctx,
		bindingSelectSQL+` WHERE workspace_id = ? AND acting_user_id = ? AND lifecycle = 'ready'`,
		workspaceID,
		actingUserID,
	)
	b, err := scanBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := r.hydrateRepos(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

// SetLifecycle changes only persisted lifecycle state. Runtime-affecting
// configuration methods own profile-version increments; validating a Draft
// and marking it Ready does not create another definition version.
func (r *WorkspaceAgentBindingRepository) SetLifecycle(ctx context.Context, id, workspaceID int, lifecycle models.AgentLifecycle) (int64, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE workspace_agent_bindings
		SET lifecycle = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ? AND lifecycle <> 'archived'
	`, lifecycle, id, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("set binding lifecycle: %w", err)
	}
	return res.RowsAffected()
}

// MigrateLegacyToRunner is the one explicit runtime transition allowed by
// Agent Studio. It preserves the profile, identity, history, repositories,
// and attribution while moving a grandfathered local binding onto an
// authorized runner pool. Standard and Coding profiles never match this
// update and remain immutable.
func (r *WorkspaceAgentBindingRepository) MigrateLegacyToRunner(ctx context.Context, id, workspaceID, poolID int) (int64, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE workspace_agent_bindings
		SET profile_type = 'coding',
		    target_pool_id = ?,
		    lifecycle = 'draft',
		    profile_version = profile_version + 1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ?
		  AND profile_type = 'legacy' AND lifecycle <> 'archived'
	`, poolID, id, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("migrate Legacy binding to runner: %w", err)
	}
	return res.RowsAffected()
}

// ConnectCodingRunner authorizes the first remote pool for a Coding Draft.
// Existing pool assignments are immutable so changing execution authority
// cannot be smuggled through a profile edit.
func (r *WorkspaceAgentBindingRepository) ConnectCodingRunner(ctx context.Context, id, workspaceID, poolID int) (int64, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE workspace_agent_bindings
		SET target_pool_id = ?,
		    lifecycle = 'draft',
		    profile_version = profile_version + 1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ?
		  AND profile_type = 'coding'
		  AND target_pool_id IS NULL
		  AND lifecycle <> 'archived'
	`, poolID, id, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("connect Coding profile runner: %w", err)
	}
	return res.RowsAffected()
}

// Archive makes a profile unavailable without deleting its stable binding or
// acting-identity references. The identity snapshot keeps historical cards and
// bylines renderable even if the user row is later tombstoned.
func (r *WorkspaceAgentBindingRepository) Archive(ctx context.Context, id, workspaceID, archivedByUserID int) (int64, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE workspace_agent_bindings
		SET lifecycle = 'archived',
		    archived_at = CURRENT_TIMESTAMP,
		    archived_by_user_id = ?,
		    last_known_name = COALESCE((
		      SELECT CASE
		        WHEN TRIM(COALESCE(first_name, '') || ' ' || COALESCE(last_name, '')) <> ''
		          THEN TRIM(COALESCE(first_name, '') || ' ' || COALESCE(last_name, ''))
		        ELSE username
		      END
		      FROM users WHERE users.id = workspace_agent_bindings.acting_user_id
		    ), last_known_name),
		    last_known_handle = COALESCE((
		      SELECT username FROM users WHERE users.id = workspace_agent_bindings.acting_user_id
		    ), last_known_handle),
		    last_known_avatar = COALESCE((
		      SELECT avatar_url FROM users WHERE users.id = workspace_agent_bindings.acting_user_id
		    ), last_known_avatar),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ? AND lifecycle <> 'archived'
	`, archivedByUserID, id, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("archive binding: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// Restore returns an archived profile to Draft so current dependencies and
// permissions must be validated before it can accept new work again.
func (r *WorkspaceAgentBindingRepository) Restore(ctx context.Context, id, workspaceID int) (int64, error) {
	res, err := r.db.ExecWriteContext(ctx, `
		UPDATE workspace_agent_bindings
		SET lifecycle = 'draft',
		    archived_at = NULL,
		    archived_by_user_id = NULL,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ? AND lifecycle = 'archived'
	`, id, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("restore binding: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// UpdateInstructions rewrites a binding's custom instructions, scoped by
// workspace (WI-258).
func (r *WorkspaceAgentBindingRepository) UpdateInstructions(ctx context.Context, id, workspaceID int, instructions string) error {
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE workspace_agent_bindings
		SET instructions = ?,
		    lifecycle = CASE WHEN lifecycle = 'archived' THEN lifecycle ELSE 'draft' END,
		    profile_version = profile_version + 1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ?
	`, instructions, id, workspaceID)
	if err != nil {
		return fmt.Errorf("update binding instructions: %w", err)
	}
	return nil
}

// UpdateConfig rewrites a binding's mutable configuration in place, scoped by
// (id, workspace_id): LLM connection, token scopes/TTL, daily budget,
// instructions, and the repository set. The acting identity, workspace, target
// pool, and runner image are NOT touched here (identity is immutable; runner
// image has its own presence-aware setter). The primary repo is mirrored onto
// the deprecated scalar columns, matching Insert (WI-450).
func (r *WorkspaceAgentBindingRepository) UpdateConfig(ctx context.Context, b *models.WorkspaceAgentBinding) error {
	scopesJSON, err := json.Marshal(b.TokenScopes)
	if err != nil {
		return fmt.Errorf("marshal token scopes: %w", err)
	}
	capabilitiesJSON, err := json.Marshal(b.CapabilityGroups)
	if err != nil {
		return fmt.Errorf("marshal capability groups: %w", err)
	}
	repos := bindingReposToPersist(b)
	// Re-mirror the primary onto the scalar columns (or clear them when no repo).
	b.RepoSlug, b.RepoBaseRef, b.SCMConnectionID = "", "", nil
	if p := primaryOf(repos); p != nil {
		b.RepoSlug = p.RepoSlug
		b.RepoBaseRef = p.RepoBaseRef
		b.SCMConnectionID = p.SCMConnectionID
	}
	if _, err := r.db.ExecWriteContext(ctx, `
		UPDATE workspace_agent_bindings
		SET llm_connection_id = ?, scm_connection_id = ?, repo_slug = ?, repo_base_ref = ?,
		    token_scopes_json = ?, token_ttl_minutes = ?, max_runs_per_day = ?,
		    instructions = ?, capability_groups_json = ?,
		    lifecycle = CASE WHEN lifecycle = 'archived' THEN lifecycle ELSE 'draft' END,
		    profile_version = profile_version + 1,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ?
	`,
		nullIntArg(b.LLMConnectionID), nullIntArg(b.SCMConnectionID),
		nullStringArg(b.RepoSlug), nullStringArg(b.RepoBaseRef),
		string(scopesJSON), b.TokenTTLMinutes, b.MaxRunsPerDay,
		b.Instructions, string(capabilitiesJSON), b.ID, b.WorkspaceID,
	); err != nil {
		return fmt.Errorf("update binding config: %w", err)
	}
	if err := r.ReplaceBindingRepos(ctx, b.ID, repos); err != nil {
		return fmt.Errorf("replace binding repos: %w", err)
	}
	b.Repos = repos
	return nil
}

// UpdateLLMConnection rewrites a binding's model connection in place, scoped by
// (id, workspace_id). It does not touch the lifecycle or profile version: the
// caller's instruction rewrite in UpdateAgentConfig owns the single Draft/version
// transition for the whole configuration change.
func (r *WorkspaceAgentBindingRepository) UpdateLLMConnection(ctx context.Context, id, workspaceID, connectionID int) error {
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE workspace_agent_bindings
		SET llm_connection_id = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ?
	`, connectionID, id, workspaceID)
	if err != nil {
		return fmt.Errorf("update binding llm connection: %w", err)
	}
	return nil
}

// UpdateRunnerImage rewrites a binding's custom runner image, scoped by
// workspace. An empty image clears the override (NULL = the runner's default
// windshift-agent image) (WI-450).
func (r *WorkspaceAgentBindingRepository) UpdateRunnerImage(ctx context.Context, id, workspaceID int, image string) error {
	_, err := r.db.ExecWriteContext(ctx, `
		UPDATE workspace_agent_bindings
		SET runner_image = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND workspace_id = ?
	`, nullStringArg(image), id, workspaceID)
	if err != nil {
		return fmt.Errorf("update binding runner image: %w", err)
	}
	return nil
}

// Delete removes a binding by (id, workspace_id). Returns the number of
// rows affected so the handler can distinguish "deleted" from "no such
// binding (or wrong workspace)". The workspace filter is required: a
// workspace admin must not be able to delete a binding belonging to a
// different workspace by guessing its id.
func (r *WorkspaceAgentBindingRepository) Delete(ctx context.Context, id, workspaceID int) (int64, error) {
	res, err := r.db.ExecWriteContext(ctx, `DELETE FROM workspace_agent_bindings WHERE id = ? AND workspace_id = ?`, id, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("delete binding: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ReplaceBindingRepos sets the binding's repos to exactly repos (WI-449). It
// deletes any existing rows then inserts each. The caller (service layer) is
// responsible for validating the repos (slug shape, one primary, no dupes)
// before calling — the partial unique index on is_primary and the
// (binding_id, repo_slug) unique index are the last line of defense and
// surface as a generic error. Like ReplaceBindingSkills this is not wrapped
// in an explicit tx; on create the service deletes the binding if this fails.
func (r *WorkspaceAgentBindingRepository) ReplaceBindingRepos(ctx context.Context, bindingID int, repos []models.BindingRepo) error {
	if _, err := r.db.ExecWriteContext(ctx, `DELETE FROM workspace_agent_binding_repos WHERE binding_id = ?`, bindingID); err != nil {
		return fmt.Errorf("clear binding repos: %w", err)
	}
	for i, rp := range repos {
		pos := rp.Position
		if pos == 0 {
			pos = i
		}
		if _, err := r.db.ExecWriteContext(ctx, `
			INSERT INTO workspace_agent_binding_repos
				(binding_id, scm_connection_id, repo_slug, repo_base_ref, is_primary, position)
			VALUES (?, ?, ?, ?, ?, ?)
		`, bindingID, nullIntArg(rp.SCMConnectionID), rp.RepoSlug, rp.RepoBaseRef, rp.IsPrimary, pos); err != nil {
			return fmt.Errorf("attach repo %q: %w", rp.RepoSlug, err)
		}
	}
	return nil
}

// ListReposForBinding returns the binding's repos ordered by position then id.
func (r *WorkspaceAgentBindingRepository) ListReposForBinding(ctx context.Context, bindingID int) ([]models.BindingRepo, error) {
	rows, err := r.db.QueryContext(ctx, reposSelectSQL+` WHERE binding_id = ? ORDER BY position ASC, id ASC`, bindingID)
	if err != nil {
		return nil, fmt.Errorf("list binding repos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []models.BindingRepo
	for rows.Next() {
		rp, err := scanBindingRepo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rp)
	}
	return out, rows.Err()
}

// hydrateRepos loads b.Repos and mirrors the primary onto the deprecated
// scalar repo fields so straggler readers keep working during the transition.
func (r *WorkspaceAgentBindingRepository) hydrateRepos(ctx context.Context, b *models.WorkspaceAgentBinding) error {
	if b == nil {
		return nil
	}
	repos, err := r.ListReposForBinding(ctx, b.ID)
	if err != nil {
		return err
	}
	b.Repos = repos
	mirrorPrimaryRepo(b)
	return nil
}

// hydrateReposBatch loads repos for many bindings with a single query, avoiding
// N+1 in ListForWorkspace.
func (r *WorkspaceAgentBindingRepository) hydrateReposBatch(ctx context.Context, bindings []*models.WorkspaceAgentBinding) error {
	if len(bindings) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(bindings)), ",")
	args := make([]any, 0, len(bindings))
	for _, b := range bindings {
		args = append(args, b.ID)
	}
	//nolint:gosec // G201: placeholders is a fixed "?," pattern, never user input
	rows, err := r.db.QueryContext(ctx, reposSelectSQL+` WHERE binding_id IN (`+placeholders+`) ORDER BY binding_id ASC, position ASC, id ASC`, args...)
	if err != nil {
		return fmt.Errorf("batch list binding repos: %w", err)
	}
	defer func() { _ = rows.Close() }()
	byBinding := make(map[int][]models.BindingRepo, len(bindings))
	for rows.Next() {
		rp, err := scanBindingRepo(rows)
		if err != nil {
			return err
		}
		byBinding[rp.BindingID] = append(byBinding[rp.BindingID], rp)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, b := range bindings {
		b.Repos = byBinding[b.ID]
		mirrorPrimaryRepo(b)
	}
	return nil
}

// mirrorPrimaryRepo copies the primary repo onto the deprecated scalar fields.
func mirrorPrimaryRepo(b *models.WorkspaceAgentBinding) {
	if p := b.PrimaryRepo(); p != nil {
		b.RepoSlug = p.RepoSlug
		b.RepoBaseRef = p.RepoBaseRef
		b.SCMConnectionID = p.SCMConnectionID
	}
}

const reposSelectSQL = `
	SELECT id, binding_id, scm_connection_id, repo_slug, repo_base_ref, is_primary, position
	FROM workspace_agent_binding_repos
`

func scanBindingRepo(scanner bindingRowScanner) (models.BindingRepo, error) {
	var rp models.BindingRepo
	var scmConn sql.NullInt64
	var baseRef sql.NullString
	if err := scanner.Scan(&rp.ID, &rp.BindingID, &scmConn, &rp.RepoSlug, &baseRef, &rp.IsPrimary, &rp.Position); err != nil {
		return rp, err
	}
	if scmConn.Valid {
		v := int(scmConn.Int64)
		rp.SCMConnectionID = &v
	}
	if baseRef.Valid {
		rp.RepoBaseRef = baseRef.String
	}
	return rp, nil
}

const bindingSelectSQL = `
	SELECT id, workspace_id, acting_user_id, acting_user_kind,
	       profile_type, lifecycle, profile_version, identity_class,
	       purpose, capability_groups_json,
	       repo_slug, repo_base_ref,
	       llm_connection_id, scm_connection_id, target_pool_id,
	       token_scopes_json, token_ttl_minutes, max_runs_per_day,
	       instructions, runner_image, created_by_user_id,
	       archived_at, archived_by_user_id,
	       last_known_name, last_known_handle, last_known_avatar,
	       COALESCE((
	         SELECT CASE
	           WHEN TRIM(COALESCE(first_name, '') || ' ' || COALESCE(last_name, '')) <> ''
	             THEN TRIM(COALESCE(first_name, '') || ' ' || COALESCE(last_name, ''))
	           ELSE username
	         END
	         FROM users WHERE users.id = workspace_agent_bindings.acting_user_id
	       ), last_known_name, ''),
	       COALESCE((SELECT username FROM users WHERE users.id = workspace_agent_bindings.acting_user_id), last_known_handle, ''),
	       COALESCE((SELECT avatar_url FROM users WHERE users.id = workspace_agent_bindings.acting_user_id), last_known_avatar, ''),
	       created_at, updated_at
	FROM workspace_agent_bindings
`

type bindingRowScanner interface {
	Scan(dest ...any) error
}

func scanBinding(row bindingRowScanner) (*models.WorkspaceAgentBinding, error) {
	return scanBindingFrom(row)
}

func scanBindingRows(rows *sql.Rows) (*models.WorkspaceAgentBinding, error) {
	return scanBindingFrom(rows)
}

func scanBindingFrom(scanner bindingRowScanner) (*models.WorkspaceAgentBinding, error) {
	b := &models.WorkspaceAgentBinding{}
	var repoSlug, repoBaseRef, runnerImage sql.NullString
	var llmConn, scmConn, targetPool, archivedBy sql.NullInt64
	var archivedAt sql.NullTime
	var scopesJSON, capabilityGroupsJSON string
	if err := scanner.Scan(
		&b.ID, &b.WorkspaceID, &b.ActingUserID, &b.ActingUserKind,
		&b.ProfileType, &b.Lifecycle, &b.ProfileVersion, &b.IdentityClass,
		&b.Purpose, &capabilityGroupsJSON,
		&repoSlug, &repoBaseRef,
		&llmConn, &scmConn, &targetPool, &scopesJSON, &b.TokenTTLMinutes, &b.MaxRunsPerDay,
		&b.Instructions, &runnerImage, &b.CreatedByUserID,
		&archivedAt, &archivedBy,
		&b.LastKnownName, &b.LastKnownHandle, &b.LastKnownAvatar,
		&b.DisplayName, &b.Handle, &b.AvatarURL,
		&b.CreatedAt, &b.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if runnerImage.Valid {
		b.RunnerImage = runnerImage.String
	}
	if repoSlug.Valid {
		b.RepoSlug = repoSlug.String
	}
	if repoBaseRef.Valid {
		b.RepoBaseRef = repoBaseRef.String
	}
	if llmConn.Valid {
		v := int(llmConn.Int64)
		b.LLMConnectionID = &v
	}
	if scmConn.Valid {
		v := int(scmConn.Int64)
		b.SCMConnectionID = &v
	}
	if targetPool.Valid {
		v := int(targetPool.Int64)
		b.TargetPoolID = &v
	}
	if scopesJSON != "" {
		_ = json.Unmarshal([]byte(scopesJSON), &b.TokenScopes)
	}
	if capabilityGroupsJSON != "" {
		_ = json.Unmarshal([]byte(capabilityGroupsJSON), &b.CapabilityGroups)
	}
	if archivedAt.Valid {
		b.ArchivedAt = &archivedAt.Time
	}
	if archivedBy.Valid {
		v := int(archivedBy.Int64)
		b.ArchivedByUserID = &v
	}
	return b, nil
}
