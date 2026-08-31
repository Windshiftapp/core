package scm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/redact"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/sso"
	"windshift/internal/utils"
)

// Sync pagination/budget constants applied to every per-repo sync. These
// bound the number of provider pages fetched and the time window we look
// back for changes. Tuned for the in-process 5-minute scheduler tick.
const (
	syncPRsPerPage = 100
	syncMaxPRs     = 500
	syncPRLookback = 7 * 24 * time.Hour

	syncCommitsPerPage = 100
	syncMaxCommits     = 500
	syncCommitLookback = 24 * time.Hour

	// smartCommitFirstSyncWindow caps how far back smart-commits will fire
	// when a repo is being synced for the first time (last_synced_at is
	// null). Without this, connecting an existing repo with hundreds of
	// historical merged PRs would trigger a transition / comment for
	// every one of them on the very first tick.
	smartCommitFirstSyncWindow = 7 * 24 * time.Hour

	// syncPerConnectionConcurrency bounds how many repos are synced in
	// parallel for a single connection. Per-connection (not global) so
	// one noisy connection cannot starve others; capped low so we don't
	// exhaust the provider's rate-limit budget for that one token.
	syncPerConnectionConcurrency = 4
)

// ActionEventEmitter is the dependency SyncService uses to surface
// SCM-triggered events (tag created, release-branch created) to the
// action engine. ActionService implements this; tests use a fake.
// Decoupled via interface so the scm package does not import services
// (which would close a layering cycle).
type ActionEventEmitter interface {
	EmitActionEvent(*models.ActionEvent)
}

type DurableActionEventRecorder interface {
	EmitActionEventInTx(context.Context, database.Tx, *models.ActionEvent) error
}

// SyncService handles periodic synchronization of SCM repositories
// to detect PRs, branches, and commits linked to work items
type SyncService struct {
	db                      database.Database
	encryption              *sso.SecretEncryption
	detector                *ItemKeyDetector
	healthRepo              *repository.SCMHealthRepository
	resolveProviderOverride func(context.Context, int) (Provider, error)

	// syncMu guards SyncAllRepositories and refreshMu guards
	// RefreshAllPRLinkStates so that an overrunning scheduler tick (>5
	// min) does not start a second copy on top of the first. Mirrors the
	// pattern in IssueSyncService.
	syncMu    sync.Mutex
	refreshMu sync.Mutex

	// Smart-commit dependencies. All four must be wired via
	// SetSmartCommitServices for smart commits to run; otherwise processing
	// is skipped silently (letting callers that only need basic link sync
	// continue to construct a SyncService without these services).
	workflowService   *services.WorkflowService
	commentService    *services.CommentService
	permissionService *services.PermissionService
	conditionService  *services.ConditionService
	approvalService   *services.ApprovalService
	itemRepo          *repository.ItemRepository

	// Optional: when wired, the tag / release-branch sync paths emit
	// ActionEvents for the action engine to dispatch. Nil means the
	// detection still happens (and the idempotency ledger fills) but
	// nothing downstream consumes the events.
	actionEvents        ActionEventEmitter
	durableActionEvents DurableActionEventRecorder

	// Optional: when wired, the PR-comment poller starts continuation runs from
	// "@agent" PR comments (WI-426). Nil disables the poller (e.g. the
	// coding-agent harness is off).
	continuationStarter PRCommentContinuationStarter
}

// PRCommentContinuationStarter starts a continuation run for an "@agent" PR
// comment detected by the poller. BindingService implements it.
type PRCommentContinuationStarter interface {
	StartPRCommentContinuation(ctx context.Context, in services.PRCommentContinuation) (bool, error)
}

type PRCommentContinuationDetailedStarter interface {
	StartPRCommentContinuationDetailed(ctx context.Context, in services.PRCommentContinuation) (services.PRCommentStartResult, error)
}

// SetContinuationStarter wires the PR-comment continuation trigger. Optional;
// without it the poller is inert.
func (s *SyncService) SetContinuationStarter(st PRCommentContinuationStarter) {
	s.continuationStarter = st
}

// NewSyncService creates a new SCM sync service
func NewSyncService(db database.Database, encryption *sso.SecretEncryption) *SyncService {
	return &SyncService{
		db:         db,
		encryption: encryption,
		detector:   NewItemKeyDetector(),
		healthRepo: repository.NewSCMHealthRepository(db),
	}
}

// SetSmartCommitServices wires in the services needed to apply smart-commit
// actions (#comment, #<transition-slug>) detected in PR bodies and commit
// messages when a PR transitions to merged during a sync.
func (s *SyncService) SetSmartCommitServices(
	workflowService *services.WorkflowService,
	commentService *services.CommentService,
	permissionService *services.PermissionService,
	conditionService *services.ConditionService,
	itemRepo *repository.ItemRepository,
) {
	s.workflowService = workflowService
	s.commentService = commentService
	s.permissionService = permissionService
	s.conditionService = conditionService
	s.itemRepo = itemRepo
}

// SetApprovalService wires the approval service so smart-commit-driven
// transitions are gated by approvals.
func (s *SyncService) SetApprovalService(ap *services.ApprovalService) {
	s.approvalService = ap
}

// SetActionEvents wires the action event emitter so that newly observed
// matching tags / release branches surface as ActionEvents. Without it,
// the sync still runs but emits nothing; this lets callers that only
// need link sync continue without depending on the action engine.
func (s *SyncService) SetActionEvents(e ActionEventEmitter) {
	s.actionEvents = e
}

// SetDurableActionEvents makes SCM observation admission atomic with the SCM
// idempotency ledger. The legacy emitter remains available to lightweight
// embeddings and tests that do not install the durable event schema.
func (s *SyncService) SetDurableActionEvents(recorder DurableActionEventRecorder) {
	s.durableActionEvents = recorder
}

// resolveProvider creates an SCM provider for a connection. Credential
// resolution (including the OAuth refresh-if-expiring step) lives in
// CredentialResolver, shared with every other consumer.
func (s *SyncService) resolveProvider(ctx context.Context, connectionID int) (Provider, error) {
	if s.resolveProviderOverride != nil {
		return s.resolveProviderOverride(ctx, connectionID)
	}
	credResolver := NewCredentialResolver(s.db, s.encryption)
	provider, err := credResolver.GetProviderForConnection(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}
	return provider, nil
}

// repoInfo bundles the workspace-repo metadata loaded for a sync tick.
// Promoted to package scope so it can be passed into per-repo goroutines
// inside SyncAllRepositories.
type repoInfo struct {
	ID             int
	RepositoryName string
	DefaultBranch  string
	WorkspaceID    int
	ProviderID     int
	ItemKeyPattern string
	WorkspaceKey   string
	ConnectionID   int
	LastSyncedAt   time.Time
}

type prRefreshTarget struct {
	LinkID         int
	RepositoryName string
	ExternalID     string
}

// SyncAllRepositories syncs all active repositories across all workspaces
// This should be called periodically (e.g., every 5 minutes) by the scheduler
func (s *SyncService) SyncAllRepositories(ctx context.Context) error {
	if !s.syncMu.TryLock() {
		slog.Info("SCM repo sync skipped: previous run still active", slog.String("component", "scm"))
		return nil
	}
	defer s.syncMu.Unlock()

	slog.Debug("Starting sync of all repositories", slog.String("component", "scm"))

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			wr.id, wr.repository_name, wr.default_branch,
			wsc.workspace_id, wsc.scm_provider_id, wsc.item_key_pattern,
			w.key as workspace_key, wsc.id as connection_id,
			wr.last_synced_at
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN workspaces w ON w.id = wsc.workspace_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wr.is_active = true AND wsc.enabled = true
		  AND sp.auth_method != 'oauth'
	`)
	if err != nil {
		return fmt.Errorf("failed to query repositories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var repos []repoInfo
	for rows.Next() {
		var r repoInfo
		var itemKeyPattern sql.NullString
		var lastSyncedAt sql.NullTime
		err := rows.Scan(&r.ID, &r.RepositoryName, &r.DefaultBranch,
			&r.WorkspaceID, &r.ProviderID, &itemKeyPattern, &r.WorkspaceKey, &r.ConnectionID,
			&lastSyncedAt)
		if err != nil {
			slog.Error("Failed to scan repository", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}
		if itemKeyPattern.Valid {
			r.ItemKeyPattern = itemKeyPattern.String
		}
		if lastSyncedAt.Valid {
			r.LastSyncedAt = lastSyncedAt.Time
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate repositories: %w", err)
	}
	_ = rows.Close()

	slog.Debug("Found active repositories to sync", slog.String("component", "scm"), slog.Int("count", len(repos)))

	connectionRepos := make(map[int][]repoInfo)
	for _, r := range repos {
		connectionRepos[r.ConnectionID] = append(connectionRepos[r.ConnectionID], r)
	}
	connectionRows, err := s.db.QueryContext(ctx, `
		SELECT wsc.id
		FROM workspace_scm_connections wsc
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wsc.enabled = true AND sp.auth_method != 'oauth'
	`)
	if err != nil {
		return fmt.Errorf("query SCM connections: %w", err)
	}
	for connectionRows.Next() {
		var connectionID int
		if err := connectionRows.Scan(&connectionID); err != nil {
			_ = connectionRows.Close()
			return fmt.Errorf("scan SCM connection: %w", err)
		}
		if _, exists := connectionRepos[connectionID]; !exists {
			connectionRepos[connectionID] = nil
		}
	}
	if err := connectionRows.Err(); err != nil {
		_ = connectionRows.Close()
		return fmt.Errorf("iterate SCM connections: %w", err)
	}
	_ = connectionRows.Close()

	for connectionID, connectionRepoList := range connectionRepos {
		if len(connectionRepoList) == 0 {
			s.recordConnectionHealth(ctx, repository.SCMHealthResult{
				ConnectionID: connectionID,
				Operation:    repository.SCMHealthOperationRepositorySync,
				AttemptedAt:  time.Now().UTC(),
			})
			continue
		}
		provider, err := s.resolveProvider(ctx, connectionID)
		if err != nil {
			s.recordConnectionHealth(ctx, repository.SCMHealthResult{
				ConnectionID:     connectionID,
				Operation:        repository.SCMHealthOperationRepositorySync,
				AttemptedAt:      time.Now().UTC(),
				CheckedResources: len(connectionRepoList),
				FailedResources:  len(connectionRepoList),
				LastError:        fmt.Sprintf("resolve provider: %v", err),
			})
			continue
		}

		sem := make(chan struct{}, syncPerConnectionConcurrency)
		var wg sync.WaitGroup
		var healthMu sync.Mutex
		var syncErrors []error
		for _, repo := range connectionRepoList {
			if ctx.Err() != nil {
				break
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(repo repoInfo) {
				defer wg.Done()
				defer func() { <-sem }()
				if err := s.syncRepository(ctx, provider, repo.ID, repo.RepositoryName, repo.DefaultBranch, repo.WorkspaceID, repo.WorkspaceKey, repo.ItemKeyPattern, repo.LastSyncedAt); err != nil {
					healthMu.Lock()
					syncErrors = append(syncErrors, fmt.Errorf("repository %s: %w", repo.RepositoryName, err))
					healthMu.Unlock()
				}
			}(repo)
		}
		wg.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.recordConnectionHealth(ctx, repository.SCMHealthResult{
			ConnectionID:     connectionID,
			Operation:        repository.SCMHealthOperationRepositorySync,
			AttemptedAt:      time.Now().UTC(),
			CheckedResources: len(connectionRepoList),
			FailedResources:  len(syncErrors),
			LastError:        joinedError(syncErrors),
		})
	}
	// OAuth has no workspace-level principal. Repositories with an agent-owned
	// PR are synced using the user who opened that PR, so review polling and
	// token refresh work without reintroducing a shared "last user" token.
	if err := s.syncOAuthAgentRepositories(ctx); err != nil {
		slog.Error("Failed to sync OAuth agent repositories", slog.String("component", "scm"), slog.Any("error", err))
	}

	slog.Debug("Completed sync of all repositories", slog.String("component", "scm"))
	return nil
}

func (s *SyncService) syncOAuthAgentRepositories(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT wr.id, wr.repository_name, wr.default_branch, wsc.workspace_id,
			w.key, wsc.item_key_pattern, wsc.id, wr.last_synced_at,
			(SELECT o.triggered_by_user_id FROM agent_pr_ownerships o
			 WHERE o.workspace_repository_id = wr.id AND o.triggered_by_user_id IS NOT NULL
			 ORDER BY o.updated_at DESC LIMIT 1)
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN workspaces w ON w.id = wsc.workspace_id
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wr.is_active = true AND wsc.enabled = true AND sp.auth_method = 'oauth'
		  AND EXISTS (SELECT 1 FROM agent_pr_ownerships o WHERE o.workspace_repository_id = wr.id AND o.triggered_by_user_id IS NOT NULL)
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	type oauthRepo struct {
		repoInfo
		userID int
	}
	var repos []oauthRepo
	for rows.Next() {
		var r oauthRepo
		var pattern sql.NullString
		var last sql.NullTime
		if err := rows.Scan(&r.ID, &r.RepositoryName, &r.DefaultBranch, &r.WorkspaceID, &r.WorkspaceKey, &pattern, &r.ConnectionID, &last, &r.userID); err != nil {
			return err
		}
		if pattern.Valid {
			r.ItemKeyPattern = pattern.String
		}
		if last.Valid {
			r.LastSyncedAt = last.Time
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	resolver := NewCredentialResolver(s.db, s.encryption)
	type connectionResult struct {
		checked int
		errors  []error
	}
	results := make(map[int]*connectionResult)
	for _, r := range repos {
		result := results[r.ConnectionID]
		if result == nil {
			result = &connectionResult{}
			results[r.ConnectionID] = result
		}
		result.checked++
		creds, err := resolver.GetCredentialsForUser(ctx, r.ConnectionID, r.userID)
		if err != nil {
			result.errors = append(result.errors, fmt.Errorf("repository %s: resolve user credential: %w", r.RepositoryName, err))
			continue
		}
		provider, err := resolver.CreateProvider(creds)
		if err != nil {
			result.errors = append(result.errors, fmt.Errorf("repository %s: create provider: %w", r.RepositoryName, err))
			continue
		}
		if err := s.syncRepository(ctx, provider, r.ID, r.RepositoryName, r.DefaultBranch, r.WorkspaceID, r.WorkspaceKey, r.ItemKeyPattern, r.LastSyncedAt); err != nil {
			result.errors = append(result.errors, fmt.Errorf("repository %s: %w", r.RepositoryName, err))
		}
	}
	for connectionID, result := range results {
		s.recordConnectionHealth(ctx, repository.SCMHealthResult{
			ConnectionID:     connectionID,
			Operation:        repository.SCMHealthOperationRepositorySync,
			AttemptedAt:      time.Now().UTC(),
			CheckedResources: result.checked,
			FailedResources:  len(result.errors),
			LastError:        joinedError(result.errors),
		})
	}
	return nil
}

// SyncRepository syncs a specific repository by ID
func (s *SyncService) SyncRepository(ctx context.Context, repoID int) error {
	var repositoryName, defaultBranch, workspaceKey string
	var workspaceID, connectionID int
	var itemKeyPattern sql.NullString
	var lastSyncedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT
			wr.repository_name, wr.default_branch,
			wsc.workspace_id, wsc.item_key_pattern,
			w.key as workspace_key, wsc.id as connection_id,
			wr.last_synced_at
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN workspaces w ON w.id = wsc.workspace_id
		WHERE wr.id = ?
	`, repoID).Scan(&repositoryName, &defaultBranch, &workspaceID, &itemKeyPattern, &workspaceKey, &connectionID, &lastSyncedAt)
	if err != nil {
		return fmt.Errorf("failed to get repository info: %w", err)
	}

	provider, err := s.resolveProvider(ctx, connectionID)
	if err != nil {
		return err
	}

	pattern := ""
	if itemKeyPattern.Valid {
		pattern = itemKeyPattern.String
	}

	var lastSync time.Time
	if lastSyncedAt.Valid {
		lastSync = lastSyncedAt.Time
	}
	return s.syncRepository(ctx, provider, repoID, repositoryName, defaultBranch, workspaceID, workspaceKey, pattern, lastSync)
}

// syncRepository performs the actual sync for a single repository
func (s *SyncService) syncRepository(ctx context.Context, provider Provider, repoID int, repositoryName, defaultBranch string, workspaceID int, workspaceKey, itemKeyPattern string, lastSyncedAt time.Time) error {
	owner, repo, ok := utils.SplitRepositoryPath(repositoryName)
	if !ok {
		return fmt.Errorf("invalid repository name format: %s", repositoryName)
	}

	slog.Debug("Syncing repository", slog.String("component", "scm"), slog.String("repository", repositoryName), slog.String("workspace", workspaceKey))

	var syncErrs []error
	if err := s.syncPullRequests(ctx, provider, owner, repo, repoID, workspaceID, workspaceKey, itemKeyPattern, lastSyncedAt); err != nil {
		syncErrs = append(syncErrs, fmt.Errorf("sync pull requests: %w", err))
	}

	// Sync commits on the default branch so commit messages containing item keys
	// create commit links even when PR titles/bodies do not mention the item.
	if err := s.syncCommits(ctx, provider, owner, repo, repoID, workspaceID, workspaceKey, itemKeyPattern, defaultBranch, lastSyncedAt); err != nil {
		syncErrs = append(syncErrs, fmt.Errorf("sync commits: %w", err))
	}

	if err := s.syncBranches(ctx, provider, owner, repo, repoID, workspaceID, workspaceKey, itemKeyPattern); err != nil {
		syncErrs = append(syncErrs, fmt.Errorf("sync branches: %w", err))
	}

	baselineRefs := lastSyncedAt.IsZero()
	if err := s.syncReleaseBranches(ctx, provider, owner, repo, repoID, workspaceID, baselineRefs); err != nil {
		syncErrs = append(syncErrs, fmt.Errorf("sync release branches: %w", err))
	}

	if err := s.syncTagsAndReleases(ctx, provider, owner, repo, repoID, workspaceID, baselineRefs); err != nil {
		syncErrs = append(syncErrs, fmt.Errorf("sync tags: %w", err))
	}
	if err := s.syncAttachedReleases(ctx, provider, owner, repo, repoID); err != nil {
		syncErrs = append(syncErrs, fmt.Errorf("sync attached releases: %w", err))
	}
	if err := errors.Join(syncErrs...); err != nil {
		return fmt.Errorf("sync repository %q: %w", repositoryName, err)
	}

	_, err := s.db.ExecWriteContext(ctx, `
		UPDATE workspace_repositories SET last_synced_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, repoID)
	if err != nil {
		return fmt.Errorf("update repository sync checkpoint: %w", err)
	}

	return nil
}

// syncAttachedReleases refreshes milestone release records from current SCM
// truth. Tags and releases are intentionally distinct: deleting a release
// leaves a tag_only record, while deleting both marks the attachment missing.
func (s *SyncService) syncAttachedReleases(ctx context.Context, provider Provider, owner, repo string, repoID int) error {
	var attached int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM milestone_releases WHERE workspace_repository_id = ? AND state = 'created'`, repoID).Scan(&attached); err != nil || attached == 0 {
		return err
	}
	releaseProvider, releasesOK := provider.(ReleaseProvider)
	refProvider, refsOK := provider.(RefProvider)
	if !releasesOK || !refsOK {
		return nil
	}
	releases, err := releaseProvider.ListReleases(ctx, owner, repo)
	if err != nil {
		return err
	}
	tags, err := refProvider.ListTags(ctx, owner, repo, time.Time{})
	if err != nil {
		return err
	}
	releaseByTag := make(map[string]Release, len(releases))
	for _, release := range releases {
		releaseByTag[release.TagName] = release
	}
	tagByName := make(map[string]Tag, len(tags))
	for _, tag := range tags {
		tagByName[tag.Name] = tag
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, tag_name FROM milestone_releases WHERE workspace_repository_id = ? AND state = 'created'`, repoID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	type attachedRelease struct {
		id  int
		tag string
	}
	var attachedRows []attachedRelease
	for rows.Next() {
		var row attachedRelease
		if err := rows.Scan(&row.id, &row.tag); err != nil {
			return err
		}
		attachedRows = append(attachedRows, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, row := range attachedRows {
		if release, ok := releaseByTag[row.tag]; ok {
			assetsJSON, _ := json.Marshal(release.Assets)
			releasedAt := release.ReleasedAt
			if releasedAt == nil {
				releasedAt = release.PublishedAt
			}
			status := release.Status
			if status == "" {
				if release.IsDraft {
					status = "draft"
				} else if release.IsPrerelease {
					status = "prerelease"
				} else {
					status = "released"
				}
			}
			_, err = s.db.ExecWriteContext(ctx, `
				UPDATE milestone_releases
				SET name=?, body=?, is_draft=?, is_prerelease=?, tag_url=?, release_status=?,
				    released_at=?, assets_json=?, scm_release_id=?, scm_release_url=?,
				    last_synced_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP
				WHERE id=?
			`, release.Name, release.Body, release.IsDraft, release.IsPrerelease, release.TagURL, status,
				releasedAt, string(assetsJSON), release.ID, release.URL, row.id)
			if err != nil {
				return err
			}
			continue
		}
		if tag, ok := tagByName[row.tag]; ok {
			_, err = s.db.ExecWriteContext(ctx, `
				UPDATE milestone_releases
				SET name=NULL, body=NULL, is_draft=false, is_prerelease=false, tag_url=?,
				    release_status='tag_only', released_at=NULL, assets_json='[]',
				    scm_release_id=NULL, scm_release_url=NULL, last_synced_at=CURRENT_TIMESTAMP,
				    updated_at=CURRENT_TIMESTAMP WHERE id=?
			`, tag.URL, row.id)
		} else {
			_, err = s.db.ExecWriteContext(ctx, `
				UPDATE milestone_releases SET release_status='missing', tag_url=NULL,
				    released_at=NULL, assets_json='[]', scm_release_id=NULL, scm_release_url=NULL,
				    last_synced_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?
			`, row.id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// syncPullRequests syncs pull requests from a repository. The paginated
// fetch is delegated to iteratePullRequests so the loop can be unit-tested
// in isolation; the callback below performs the per-PR work.
func (s *SyncService) syncPullRequests(ctx context.Context, provider Provider, owner, repo string, repoID, workspaceID int, workspaceKey, itemKeyPattern string, lastSyncedAt time.Time) error {
	return iteratePullRequests(ctx, provider, owner, repo, lastSyncedAt, syncMaxPRs, func(pr PullRequest) error {
		return s.processPullRequest(ctx, provider, owner, repo, pr, repoID, workspaceID, workspaceKey, itemKeyPattern, lastSyncedAt)
	})
}

// shouldRunSmartCommits decides whether a newly observed merged PR is
// recent enough to fire smart-commit actions. The cap exists to prevent
// the very first sync of a long-lived repo from replaying transitions
// for hundreds of historical merges. On steady-state syncs (lastSyncedAt
// non-zero) any merge after lastSyncedAt qualifies; on first sync, only
// merges within smartCommitFirstSyncWindow.
func shouldRunSmartCommits(pr PullRequest, lastSyncedAt, now time.Time) bool {
	if pr.MergedAt == nil {
		return false
	}
	if !lastSyncedAt.IsZero() {
		return pr.MergedAt.After(lastSyncedAt)
	}
	return pr.MergedAt.After(now.Add(-smartCommitFirstSyncWindow))
}

// shouldEmitPRLinkEvent decides whether a newly discovered PR is recent
// enough to fire the scm_pr_linked action trigger. Mirrors the smart-commit
// lookback so connecting an old repository doesn't flood the action engine
// with stale PR links.
func shouldEmitPRLinkEvent(pr PullRequest, lastSyncedAt, now time.Time) bool {
	if !lastSyncedAt.IsZero() {
		return pr.UpdatedAt.After(lastSyncedAt)
	}
	return pr.UpdatedAt.After(now.Add(-smartCommitFirstSyncWindow))
}

// shouldEmitPRMergeEvent decides whether a newly observed merged PR is
// recent enough to fire the scm_pr_merged action trigger.
func shouldEmitPRMergeEvent(pr PullRequest, lastSyncedAt, now time.Time) bool {
	return shouldRunSmartCommits(pr, lastSyncedAt, now)
}

// iteratePullRequests walks descending update pages until the last page, cap,
// lookback cutoff, or cancellation. First syncs omit the time cutoff.
func iteratePullRequests(ctx context.Context, provider Provider, owner, repo string, lastSyncedAt time.Time, maxPRs int, fn func(PullRequest) error) error {
	var cutoff time.Time
	if !lastSyncedAt.IsZero() {
		cutoff = lastSyncedAt.Add(-syncPRLookback)
	}

	processed := 0
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		prs, err := provider.ListPullRequests(ctx, owner, repo, ListPROptions{
			State:     "all", // Get all to detect state changes
			Page:      page,
			PerPage:   syncPRsPerPage,
			Sort:      "updated",
			Direction: "desc",
		})
		if err != nil {
			return fmt.Errorf("failed to list PRs: %w", err)
		}
		if len(prs) == 0 {
			return nil
		}

		stop := false
		for _, pr := range prs {
			if !cutoff.IsZero() && pr.UpdatedAt.Before(cutoff) {
				stop = true
				break
			}
			if err := fn(pr); err != nil {
				return fmt.Errorf("process pull request %d: %w", pr.Number, err)
			}
			processed++
			if processed >= maxPRs {
				stop = true
				break
			}
		}
		if stop {
			return nil
		}
		if len(prs) < syncPRsPerPage {
			return nil
		}
	}
}

// processPullRequest handles key detection, link upsert, smart-commit
// dispatch, and action-engine events for a single PR. Extracted so the
// paged loop above stays tight.
func (s *SyncService) processPullRequest(ctx context.Context, provider Provider, owner, repo string, pr PullRequest, repoID, workspaceID int, workspaceKey, itemKeyPattern string, lastSyncedAt time.Time) error {
	keys := s.detectPullRequestKeys(&pr, workspaceKey, itemKeyPattern)
	if len(keys) == 0 {
		return nil
	}

	// Determine whether this PR is newly observed as merged (used to
	// gate smart-commit processing for the PR body). Check BEFORE the
	// upserts overwrite stored state.
	newlyMerged := false
	if pr.IsMerged {
		newlyMerged = s.isPRNewlyMerged(ctx, repoID, pr.Number)
	}

	now := time.Now()
	var itemIDs []int
	linkedItems := make(map[int]bool) // item ids for which a *new* link was created
	var observationErrs []error
	for _, key := range keys {
		itemID, err := s.findItemByKey(ctx, workspaceID, key.Prefix, key.Number)
		if err != nil || itemID == 0 {
			continue // Item doesn't exist in this workspace
		}
		itemIDs = append(itemIDs, itemID)

		state := models.SCMLinkStateOpen
		if pr.IsMerged {
			state = models.SCMLinkStateMerged
		} else if pr.State == "closed" {
			state = models.SCMLinkStateClosed
		}

		emitLinked := shouldEmitPRLinkEvent(pr, lastSyncedAt, now)
		emitMerged := shouldEmitPRMergeEvent(pr, lastSyncedAt, now)
		created, becameMerged, err := s.upsertPullRequestSCMLink(ctx, itemID, repoID,
			strconv.Itoa(pr.Number), pr.URL, pr.Title, state, pr.Author.ID, pr.Author.Name, string(key.Source),
			s.prLinkedEvent(workspaceID, itemID, repoID, owner, repo, pr),
			s.prMergedEvent(workspaceID, itemID, repoID, owner, repo, pr), emitLinked, emitMerged)
		if err != nil {
			observationErrs = append(observationErrs, fmt.Errorf("persist PR %d observation for item %d: %w", pr.Number, itemID, err))
			continue
		}
		if created && emitLinked && s.durableActionEvents == nil {
			linkedItems[itemID] = true
		}
		if becameMerged && emitMerged && s.durableActionEvents == nil {
			s.emitPRMergedEvent(workspaceID, itemID, repoID, owner, repo, pr)
		}
	}
	if err := errors.Join(observationErrs...); err != nil {
		return err
	}

	if len(linkedItems) > 0 {
		for itemID := range linkedItems {
			s.emitPRLinkedEvent(workspaceID, itemID, repoID, owner, repo, pr)
		}
	}

	// Outbound "@agent" PR-comment trigger (WI-426): on an open linked PR, poll
	// its comments and continue the PR when a human asks the agent to. Outbound
	// only — Windshift is typically behind NAT, so no inbound webhook.
	if !pr.IsMerged && pr.State != "closed" && len(itemIDs) > 0 {
		s.pollPRCommentTriggers(ctx, provider, owner, repo, pr, repoID, workspaceID, itemIDs)
	}

	if newlyMerged && shouldRunSmartCommits(pr, lastSyncedAt, now) {
		s.processSmartCommitsForPR(ctx, provider, owner, repo, pr, repoID, workspaceID, workspaceKey)
	}
	return nil
}

// prCommentTriggerRE matches the literal agent trigger token as a whole word
// (so "@agentic" does not match) case-insensitively. Detection only — the
// most-recently-active binding is chosen downstream, so no "@agent:<name>"
// parsing is needed.
var prCommentTriggerRE = regexp.MustCompile(`(?i)` + regexp.QuoteMeta(models.DefaultAgentTriggerToken) + `\b`)

// pollPRCommentTriggers starts only new non-agent continuation requests; first poll seeds the cursor.
func (s *SyncService) pollPRCommentTriggers(ctx context.Context, provider Provider, owner, repo string, pr PullRequest, repoID, workspaceID int, itemIDs []int) {
	if s.continuationStarter == nil {
		return // poller not wired (e.g. coding-agent harness disabled)
	}
	events, err := listPRReviewEvents(ctx, provider, owner, repo, pr.Number)
	if err != nil {
		slog.Warn("PR review poll: list events failed", slog.String("component", "scm"), slog.Int("pr", pr.Number), slog.Any("error", err))
		return
	}

	cursor, exists := s.prCommentCursor(ctx, repoID, pr.Number)
	maxID := cursor
	for _, event := range events {
		if normalizeEventKind(event.Kind) == "issue_comment" && event.ID > maxID {
			maxID = event.ID
		}
	}
	now := time.Now().UTC()
	itemID := 0
	if len(itemIDs) > 0 {
		itemID = itemIDs[0]
	}
	itemID = s.ownedItemForPR(ctx, repoID, pr.Number, itemID)
	reviewKindSeen := map[string]bool{
		"review":         s.hasReviewKindLedger(ctx, repoID, pr.Number, "review"),
		"review_comment": s.hasReviewKindLedger(ctx, repoID, pr.Number, "review_comment"),
	}
	for _, event := range events {
		kind := normalizeEventKind(event.Kind)
		if exists && kind == "issue_comment" && event.ID <= cursor {
			continue
		}
		firstSight := !exists
		if kind != "issue_comment" {
			firstSight = !reviewKindSeen[kind]
		}
		if firstSight && !isRecentFirstSight(event, now) {
			if kind != "issue_comment" && prCommentTriggerRE.MatchString(event.Body) && !strings.Contains(event.Body, models.AgentCommentMarker) {
				eventID, insertErr := s.insertPRReviewEvent(ctx, repoID, workspaceID, itemID, pr.Number, event)
				if insertErr == nil {
					s.setReviewEventStatus(ctx, eventID, "ignored", "historical review baseline", 0)
				}
			}
			continue // migration baseline: only preserve live comments near first sight
		}
		_, insertErr := s.IngestPRReviewEvent(ctx, provider, owner, repo, pr, repoID, workspaceID, itemID, event)
		if insertErr != nil {
			slog.Warn("PR review poll: persist event failed", slog.Int("pr", pr.Number), slog.Any("error", insertErr))
		}
	}
	s.setPRCommentCursor(ctx, repoID, pr.Number, maxID)
	s.processPRReviewInbox(ctx, provider, owner, repo, pr, repoID, workspaceID)
}

// resolveHeadBranch returns the PR's head branch, fetching the PR fresh only
// when the listed object didn't carry one (some list endpoints omit it).
func (s *SyncService) resolveHeadBranch(ctx context.Context, provider Provider, owner, repo string, pr PullRequest) string {
	if pr.HeadBranch != "" {
		return pr.HeadBranch
	}
	full, err := provider.GetPullRequest(ctx, owner, repo, pr.Number)
	if err != nil || full == nil {
		return ""
	}
	return full.HeadBranch
}

// prCommentCursor returns the last processed comment id for a PR and whether a
// cursor row exists. A read error is reported as "no cursor" so the caller
// baselines instead of firing — the safe direction.
func (s *SyncService) prCommentCursor(ctx context.Context, repoID, prNumber int) (int64, bool) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		SELECT last_comment_id FROM pr_comment_cursors
		WHERE workspace_repository_id = ? AND pr_number = ?
	`, repoID, prNumber).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false
	}
	if err != nil {
		slog.Warn("PR comment poll: read cursor failed", slog.String("component", "scm"), slog.Int("pr", prNumber), slog.Any("error", err))
		return 0, false
	}
	return id, true
}

// setPRCommentCursor upserts the high-water comment id for a PR.
func (s *SyncService) setPRCommentCursor(ctx context.Context, repoID, prNumber int, id int64) {
	if _, err := s.db.ExecWriteContext(ctx, `
		INSERT INTO pr_comment_cursors (workspace_repository_id, pr_number, last_comment_id)
		VALUES (?, ?, ?)
		ON CONFLICT(workspace_repository_id, pr_number)
		DO UPDATE SET last_comment_id = excluded.last_comment_id, updated_at = CURRENT_TIMESTAMP
	`, repoID, prNumber, id); err != nil {
		slog.Warn("PR comment poll: write cursor failed", slog.String("component", "scm"), slog.Int("pr", prNumber), slog.Any("error", err))
	}
}

// isPRNewlyMerged returns true if the PR appears merged on the provider but
// no stored link row for that PR is yet recorded as merged. Handles both
// "first time seeing this PR" and "stored state was open/closed before".
func (s *SyncService) isPRNewlyMerged(ctx context.Context, repoID, prNumber int) bool {
	var mergedCount int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM item_scm_links
		WHERE workspace_repository_id = ? AND link_type = 'pull_request'
		  AND external_id = ? AND state = ?
	`, repoID, strconv.Itoa(prNumber), models.SCMLinkStateMerged).Scan(&mergedCount)
	if err != nil {
		return false
	}
	return mergedCount == 0
}

// syncCommits syncs recent commits from the repository's default branch and
// creates/updates commit SCM links for any item keys found in commit messages.
func (s *SyncService) syncCommits(ctx context.Context, provider Provider, owner, repo string, repoID, workspaceID int, workspaceKey, itemKeyPattern, defaultBranch string, lastSyncedAt time.Time) error {
	commitProvider, ok := provider.(CommitProvider)
	if !ok {
		return nil
	}
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	var since *time.Time
	if !lastSyncedAt.IsZero() {
		sinceTime := lastSyncedAt.Add(-syncCommitLookback)
		since = &sinceTime
	}

	processed := 0
	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		commits, err := commitProvider.ListCommits(ctx, owner, repo, ListCommitsOptions{
			Sha:     defaultBranch,
			Since:   since,
			Page:    page,
			PerPage: syncCommitsPerPage,
		})
		if err != nil {
			return fmt.Errorf("failed to list commits: %w", err)
		}
		if len(commits) == 0 {
			return nil
		}

		for _, commit := range commits {
			s.processCommit(ctx, commit, repoID, workspaceID, workspaceKey, itemKeyPattern)
			processed++
			if processed >= syncMaxCommits {
				return nil
			}
		}
		if len(commits) < syncCommitsPerPage {
			return nil
		}
	}
}

func (s *SyncService) processCommit(ctx context.Context, commit Commit, repoID, workspaceID int, workspaceKey, itemKeyPattern string) {
	keys := s.detectKeysInText(commit.Message, workspaceKey, itemKeyPattern, DetectionSourceCommitMessage)
	if len(keys) == 0 {
		return
	}

	title := strings.SplitN(commit.Message, "\n", 2)[0]
	authorExternalID := commit.Author.ID
	if authorExternalID == "" {
		authorExternalID = commit.Committer.ID
	}
	authorName := scmUserDisplayName(commit.Author)
	if authorName == "" {
		authorName = scmUserDisplayName(commit.Committer)
	}

	for _, key := range keys {
		itemID, err := s.findItemByKey(ctx, workspaceID, key.Prefix, key.Number)
		if err != nil || itemID == 0 {
			continue
		}
		err = s.upsertItemSCMLink(ctx, itemID, repoID, models.SCMLinkTypeCommit,
			commit.SHA, commit.URL, title, "", authorExternalID, authorName, string(key.Source))
		if err != nil {
			slog.Error("Failed to upsert commit link", slog.String("component", "scm"), slog.Int("item_id", itemID), slog.String("sha", commit.SHA), slog.Any("error", err))
		}
	}
}

func scmUserDisplayName(u User) string {
	if u.Name != "" {
		return u.Name
	}
	if u.Username != "" {
		return u.Username
	}
	return u.Email
}

func (s *SyncService) detectPullRequestKeys(pr *PullRequest, workspaceKey, itemKeyPattern string) []DetectedItemKey {
	var allKeys []DetectedItemKey
	seen := make(map[string]bool)
	sources := []struct {
		text   string
		source DetectionSource
	}{
		{pr.Title, DetectionSourcePRTitle},
		{pr.Body, DetectionSourcePRBody},
		{pr.HeadBranch, DetectionSourceBranchName},
	}
	for _, source := range sources {
		for _, key := range s.detectKeysInText(source.text, workspaceKey, itemKeyPattern, source.source) {
			seenKey := strings.ToUpper(key.Prefix) + "-" + strconv.Itoa(key.Number)
			if seen[seenKey] {
				continue
			}
			seen[seenKey] = true
			allKeys = append(allKeys, key)
		}
	}
	return allKeys
}

func (s *SyncService) detectKeysInText(text, workspaceKey, itemKeyPattern string, source DetectionSource) []DetectedItemKey {
	if itemKeyPattern != "" {
		return s.detector.DetectItemKeysWithPattern(text, itemKeyPattern, source)
	}
	if workspaceKey != "" {
		return s.detector.DetectItemKeysForPrefix(text, workspaceKey, source)
	}
	return s.detector.DetectItemKeys(text, source)
}

// syncBranches syncs branches from a repository
func (s *SyncService) syncBranches(ctx context.Context, provider Provider, owner, repo string, repoID, workspaceID int, workspaceKey, itemKeyPattern string) error {
	branches, err := provider.ListBranches(ctx, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}

	for _, branch := range branches {
		// Detect item keys in branch name
		keys := s.detectKeysInText(branch.Name, workspaceKey, itemKeyPattern, DetectionSourceBranchName)
		if len(keys) == 0 {
			continue
		}

		// For each detected key, create/update a link
		for _, key := range keys {
			itemID, err := s.findItemByKey(ctx, workspaceID, key.Prefix, key.Number)
			if err != nil || itemID == 0 {
				continue // Item doesn't exist in this workspace
			}

			branchURL := branchWebURL(provider, owner, repo, branch.Name)

			err = s.upsertItemSCMLink(ctx, itemID, repoID, models.SCMLinkTypeBranch,
				branch.Name, branchURL, branch.Name, "", "", "", string(key.Source))
			if err != nil {
				slog.Error("Failed to upsert branch link", slog.String("component", "scm"), slog.Int("item_id", itemID), slog.Any("error", err))
			}
		}
	}

	return nil
}

func branchWebURL(provider Provider, owner, repo, branch string) string {
	escapedBranch := url.PathEscape(branch)
	switch typed := provider.(type) {
	case *GitLabProvider:
		return typed.projectWebURL(owner, repo) + "/-/tree/" + escapedBranch
	case *GiteaProvider:
		return typed.baseURL + "/" + owner + "/" + repo + "/src/branch/" + escapedBranch
	default:
		return "https://github.com/" + owner + "/" + repo + "/tree/" + escapedBranch
	}
}

// findItemByKey finds an item by its workspace key and number. Returns 0
// (without error) when no item matches.
func (s *SyncService) findItemByKey(_ context.Context, workspaceID int, workspaceKey string, itemNumber int) (int, error) {
	itemID, err := repository.NewItemRepository(s.db).FindIDByKeyAndNumberInWorkspace(workspaceID, workspaceKey, itemNumber)
	if errors.Is(err, repository.ErrNotFound) {
		return 0, nil
	}
	return itemID, err
}

// upsertItemSCMLink creates or updates a non-PR SCM link for an item.
func (s *SyncService) upsertItemSCMLink(ctx context.Context, itemID, repoID int, linkType models.SCMLinkType,
	externalID, externalURL, title string, state models.SCMLinkState, authorExternalID, authorName, detectionSource string) error {

	// Try to find existing link
	var existingID int
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM item_scm_links
		WHERE item_id = ? AND workspace_repository_id = ? AND link_type = ? AND external_id = ?
	`, itemID, repoID, linkType, externalID).Scan(&existingID)

	if errors.Is(err, sql.ErrNoRows) {
		// Insert new link
		_, err = s.db.ExecWriteContext(ctx, `
			INSERT INTO item_scm_links (
				item_id, workspace_repository_id, link_type, external_id,
				external_url, title, state, author_external_id, author_name, detection_source
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, itemID, repoID, linkType, externalID, externalURL, title, state, authorExternalID, authorName, detectionSource)
		if err == nil {
			// Live-update publish (WI-484): a webhook/poll detected an external
			// PR/branch/commit for this item; refresh its SCM-links section.
			services.PublishItemChange(itemID, services.ItemChangeLink)
		}
		return err
	}

	if err != nil {
		return err
	}

	// Update existing link
	_, err = s.db.ExecWriteContext(ctx, `
		UPDATE item_scm_links SET
			external_url = ?, title = ?, state = ?,
			author_external_id = ?, author_name = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, externalURL, title, state, authorExternalID, authorName, existingID)
	if err == nil {
		// Live-update publish (WI-484): an external link changed state (e.g. PR
		// merged/closed) — a visible SCM-section change.
		services.PublishItemChange(itemID, services.ItemChangeLink)
	}

	return err
}

func (s *SyncService) upsertPullRequestSCMLink(
	ctx context.Context,
	itemID, repoID int,
	externalID, externalURL, title string,
	state models.SCMLinkState,
	authorExternalID, authorName, detectionSource string,
	linkedEvent, mergedEvent *models.ActionEvent,
	emitLinked, emitMerged bool,
) (created, becameMerged bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, false, fmt.Errorf("begin PR link observation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var existingID int
	var previousState models.SCMLinkState
	err = tx.QueryRowContext(ctx, `
		SELECT id, state FROM item_scm_links
		WHERE item_id = ? AND workspace_repository_id = ? AND link_type = ? AND external_id = ?
	`, itemID, repoID, models.SCMLinkTypePullRequest, externalID).Scan(&existingID, &previousState)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		created = true
		becameMerged = state == models.SCMLinkStateMerged
		_, err = tx.ExecContext(ctx, `
			INSERT INTO item_scm_links (
				item_id, workspace_repository_id, link_type, external_id,
				external_url, title, state, author_external_id, author_name, detection_source
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, itemID, repoID, models.SCMLinkTypePullRequest, externalID, externalURL, title, state, authorExternalID, authorName, detectionSource)
		if err != nil {
			return false, false, err
		}
		if emitLinked && s.durableActionEvents != nil {
			if err := s.durableActionEvents.EmitActionEventInTx(ctx, tx, linkedEvent); err != nil {
				return false, false, err
			}
		}
		if becameMerged && emitMerged && s.durableActionEvents != nil {
			if err := s.durableActionEvents.EmitActionEventInTx(ctx, tx, mergedEvent); err != nil {
				return false, false, err
			}
		}
	case err != nil:
		return false, false, err
	default:
		becameMerged = state == models.SCMLinkStateMerged && previousState != models.SCMLinkStateMerged
		_, err = tx.ExecContext(ctx, `
			UPDATE item_scm_links SET
				external_url = ?, title = ?, state = ?,
				author_external_id = ?, author_name = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, externalURL, title, state, authorExternalID, authorName, existingID)
		if err != nil {
			return false, false, err
		}
		if becameMerged && emitMerged && s.durableActionEvents != nil {
			if err := s.durableActionEvents.EmitActionEventInTx(ctx, tx, mergedEvent); err != nil {
				return false, false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, false, fmt.Errorf("commit PR link observation: %w", err)
	}
	services.PublishItemChange(itemID, services.ItemChangeLink)
	return created, becameMerged, nil
}

// emitPRLinkedEvent dispatches an scm_pr_linked action event for one linked
// item. The payload mirrors the scm_tag_created / scm_release_branch_created
// shape so templates can share repo variables and also read pr.* fields.
func (s *SyncService) emitPRLinkedEvent(workspaceID, itemID, repoID int, owner, repo string, pr PullRequest) {
	if s.actionEvents == nil {
		return
	}
	s.actionEvents.EmitActionEvent(s.prLinkedEvent(workspaceID, itemID, repoID, owner, repo, pr))
}

func (s *SyncService) prLinkedEvent(workspaceID, itemID, repoID int, owner, repo string, pr PullRequest) *models.ActionEvent {
	return &models.ActionEvent{
		EventType:   models.ActionTriggerSCMPRLinked,
		WorkspaceID: workspaceID,
		ItemID:      itemID,
		ActorUserID: 0, // sync loop has no authenticated actor; action must use actor_user_id override
		NewValues:   s.prEventValues(pr, repoID, owner, repo),
	}
}

// emitPRMergedEvent dispatches an scm_pr_merged action event for one linked
// item. The payload carries pr.is_merged and pr.merged_at so downstream
// nodes can reason about the merge details.
func (s *SyncService) emitPRMergedEvent(workspaceID, itemID, repoID int, owner, repo string, pr PullRequest) {
	if s.actionEvents == nil {
		return
	}
	s.actionEvents.EmitActionEvent(s.prMergedEvent(workspaceID, itemID, repoID, owner, repo, pr))
}

func (s *SyncService) prMergedEvent(workspaceID, itemID, repoID int, owner, repo string, pr PullRequest) *models.ActionEvent {
	return &models.ActionEvent{
		EventType:   models.ActionTriggerSCMPRMerged,
		WorkspaceID: workspaceID,
		ItemID:      itemID,
		ActorUserID: 0, // sync loop has no authenticated actor; action must use actor_user_id override
		NewValues:   s.prEventValues(pr, repoID, owner, repo),
	}
}

// prEventValues builds the shared NewValues payload for PR-linked and
// PR-merged action events.
func (s *SyncService) prEventValues(pr PullRequest, repoID int, owner, repo string) map[string]any {
	values := map[string]any{
		"pr.number":                    pr.Number,
		"pr.title":                     pr.Title,
		"pr.url":                       pr.URL,
		"pr.head_branch":               pr.HeadBranch,
		"pr.base_branch":               pr.BaseBranch,
		"pr.state":                     pr.State,
		"pr.is_merged":                 pr.IsMerged,
		"repo.workspace_repository_id": repoID,
		"repo.owner":                   owner,
		"repo.name":                    repo,
		"repo.full_name":               fmt.Sprintf("%s/%s", owner, repo),
	}
	if pr.MergedAt != nil {
		values["pr.merged_at"] = *pr.MergedAt
	}
	return values
}

// getProviderInstance creates a provider instance from provider-level
// credentials only (PAT or GitHub App). It is the last-resort fallback for
// callers that hit a non-OAuth path; OAuth is intentionally rejected because
// per-user / workspace OAuth tokens are the only valid sources of OAuth
// credentials, and falling back to a provider-level OAuth token here would
// silently impersonate whichever user happened to connect most recently.
func (s *SyncService) getProviderInstance(providerID int) (Provider, error) {
	var providerType models.SCMProviderType
	var authMethod models.SCMAuthMethod
	var baseURL, patEnc sql.NullString

	err := s.db.QueryRow(`
		SELECT provider_type, auth_method, base_url,
			   personal_access_token_encrypted
		FROM scm_providers WHERE id = ?
	`, providerID).Scan(&providerType, &authMethod, &baseURL, &patEnc)
	if err != nil {
		return nil, err
	}

	cfg := ProviderConfig{
		ProviderType: providerType,
		AuthMethod:   authMethod,
		BaseURL:      baseURL.String,
	}

	switch authMethod {
	case models.SCMAuthMethodOAuth:
		return nil, fmt.Errorf("OAuth provider %d cannot be resolved without a user or workspace context", providerID)
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

// RefreshItemSCMLink refreshes the details of a specific SCM link using the
// workspace-level credentials for the connection.
func (s *SyncService) RefreshItemSCMLink(ctx context.Context, linkID int) error {
	return s.refreshItemSCMLink(ctx, linkID, nil)
}

// refreshItemSCMLink is the shared implementation behind RefreshItemSCMLink and
// RefreshItemSCMLinkForUser. When userID is non-nil it resolves the provider
// using that user's personal OAuth token; otherwise it uses workspace-level
// credentials / GitHub Apps.
func (s *SyncService) refreshItemSCMLink(ctx context.Context, linkID int, userID *int) error {
	var itemID, repoID, connectionID int
	var linkType models.SCMLinkType
	var externalID, repositoryName string
	var externalURL sql.NullString

	err := s.db.QueryRowContext(ctx, `
		SELECT isl.item_id, isl.workspace_repository_id, isl.link_type, isl.external_id,
			   isl.external_url, wr.repository_name, wr.workspace_scm_connection_id
		FROM item_scm_links isl
		JOIN workspace_repositories wr ON wr.id = isl.workspace_repository_id
		WHERE isl.id = ?
	`, linkID).Scan(&itemID, &repoID, &linkType, &externalID, &externalURL, &repositoryName, &connectionID)
	if err != nil {
		return fmt.Errorf("failed to get link info: %w", err)
	}

	var provider Provider
	if userID != nil {
		credResolver := NewCredentialResolver(s.db, s.encryption)
		provider, err = credResolver.GetProviderForUser(ctx, connectionID, *userID)
		if err != nil {
			return fmt.Errorf("failed to get provider for user: %w", err)
		}
	} else {
		provider, err = s.resolveProvider(ctx, connectionID)
		if err != nil {
			return fmt.Errorf("failed to get provider: %w", err)
		}
	}

	owner, repo, ok := utils.SplitRepositoryPath(repositoryName)
	if !ok {
		return fmt.Errorf("invalid repository name format: %s", repositoryName)
	}

	return s.updateLinkFromProvider(ctx, provider, owner, repo, linkID, linkType, externalID, externalURL.String)
}

// prNumberFromURL extracts the per-repo pull-request number from a PR's HTML
// URL by reading its trailing path segment. Works for both GitHub
// (".../pull/41") and Gitea/Forgejo (".../pulls/41"); query strings and
// fragments are stripped. Returns 0 if no number can be parsed.
func prNumberFromURL(rawURL string) int {
	if rawURL == "" {
		return 0
	}
	if i := strings.IndexAny(rawURL, "?#"); i >= 0 {
		rawURL = rawURL[:i]
	}
	rawURL = strings.TrimRight(rawURL, "/")
	seg := rawURL[strings.LastIndex(rawURL, "/")+1:]
	n, err := strconv.Atoi(seg)
	if err != nil {
		return 0
	}
	return n
}

// updateLinkFromProvider fetches updated metadata from the SCM provider and updates the link row.
func (s *SyncService) updateLinkFromProvider(ctx context.Context, provider Provider, owner, repo string, linkID int, linkType models.SCMLinkType, externalID, externalURL string) error {
	switch linkType {
	case models.SCMLinkTypePullRequest:
		// The canonical key is the per-repo PR *number*. Links created before
		// the WI-423 fix stored the provider's global database ID instead, so
		// GetPullRequest(globalID) 404s and the link never updates. The stored
		// external_url always ends in the real number — prefer it, and rewrite
		// the stale external_id so the row self-heals and the UI stops showing
		// the global ID as the PR "number".
		prNumber, _ := strconv.Atoi(externalID)
		if urlNum := prNumberFromURL(externalURL); urlNum > 0 && urlNum != prNumber {
			prNumber = urlNum
			externalID = strconv.Itoa(urlNum)
		}
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

		_, err = s.db.ExecWriteContext(ctx, `
			UPDATE item_scm_links SET
				external_id = ?, external_url = ?, title = ?, state = ?,
				author_external_id = ?, author_name = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, externalID, pr.URL, pr.Title, state, pr.Author.ID, pr.Author.Name, linkID)
		return err

	case models.SCMLinkTypeCommit:
		commit, err := provider.GetCommit(ctx, owner, repo, externalID)
		if err != nil {
			return fmt.Errorf("failed to get commit: %w", err)
		}

		title := strings.SplitN(commit.Message, "\n", 2)[0]

		_, err = s.db.ExecWriteContext(ctx, `
			UPDATE item_scm_links SET
				external_url = ?, title = ?,
				author_external_id = ?, author_name = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, commit.URL, title, commit.Author.ID, commit.Author.Name, linkID)
		return err

	case models.SCMLinkTypeBranch:
		_, err := s.db.ExecWriteContext(ctx, `
			UPDATE item_scm_links SET updated_at = CURRENT_TIMESTAMP WHERE id = ?
		`, linkID)
		return err
	}

	return nil
}

// repoContext holds the resolved repository metadata, parsed owner/repo, and
// an authenticated SCM provider ready for API calls.
type repoContext struct {
	RepositoryName string
	DefaultBranch  string
	Owner          string
	Repo           string
	ProviderType   models.SCMProviderType
	BaseURL        string // resolved base URL (e.g. "https://github.com")
	Provider       Provider
}

// resolveRepoAndProvider loads repository metadata from the database, parses
// owner/repo, and resolves an authenticated provider.  When uid > 0 the
// user's personal OAuth token is preferred; otherwise connection-level or
// legacy provider-level credentials are used as fallback.
func (s *SyncService) resolveRepoAndProvider(ctx context.Context, workspaceRepoID, uid int) (*repoContext, error) {
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("workspace repository not found: %d", workspaceRepoID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get repository info: %w", err)
	}

	// Parse owner/repo from repository name
	owner, repo, ok := utils.SplitRepositoryPath(repositoryName)
	if !ok {
		return nil, fmt.Errorf("invalid repository name format: %s", repositoryName)
	}

	// Resolve provider credentials
	credResolver := NewCredentialResolver(s.db, s.encryption)
	connectionID := s.getConnectionIDForRepo(ctx, workspaceRepoID)

	var provider Provider
	if uid > 0 {
		// Use user-level credentials for OAuth providers
		provider, err = credResolver.GetProviderForUser(ctx, connectionID, uid)
		if err != nil {
			// Return specific error for unconnected users
			if errors.Is(err, ErrUserSCMNotConnected) {
				return nil, err
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
			return nil, fmt.Errorf("failed to get provider: %w", err)
		}
	}

	// Resolve base URL, falling back to well-known defaults
	resolvedBaseURL := baseURL.String
	if resolvedBaseURL == "" {
		switch providerType {
		case models.SCMProviderTypeGitHub:
			resolvedBaseURL = "https://github.com"
		case models.SCMProviderTypeGitLab:
			resolvedBaseURL = "https://gitlab.com"
		case models.SCMProviderTypeGitea:
			resolvedBaseURL = "https://gitea.com"
		}
	}

	return &repoContext{
		RepositoryName: repositoryName,
		DefaultBranch:  defaultBranch,
		Owner:          owner,
		Repo:           repo,
		ProviderType:   providerType,
		BaseURL:        resolvedBaseURL,
		Provider:       provider,
	}, nil
}

// ListReleaseCandidates returns existing tags, enriched with first-class SCM
// release metadata when the provider exposes a release for the same tag.
func (s *SyncService) ListReleaseCandidates(ctx context.Context, workspaceRepoID, userID int) ([]ReleaseCandidate, error) {
	rc, err := s.resolveRepoAndProvider(ctx, workspaceRepoID, userID)
	if err != nil {
		return nil, err
	}
	refProvider, ok := rc.Provider.(RefProvider)
	if !ok {
		return nil, errors.New("SCM provider does not support tags")
	}
	releaseProvider, ok := rc.Provider.(ReleaseProvider)
	if !ok {
		return nil, errors.New("SCM provider does not support releases")
	}

	tags, err := refProvider.ListTags(ctx, rc.Owner, rc.Repo, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	releases, err := releaseProvider.ListReleases(ctx, rc.Owner, rc.Repo)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	releasesByTag := make(map[string]Release, len(releases))
	for _, release := range releases {
		releasesByTag[release.TagName] = release
	}

	candidates := make([]ReleaseCandidate, 0, len(tags)+len(releases))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		candidate := ReleaseCandidate{TagName: tag.Name, TagURL: tag.URL, SHA: tag.SHA, Status: "tag_only"}
		if release, exists := releasesByTag[tag.Name]; exists {
			releaseCopy := release
			candidate.Release = &releaseCopy
			candidate.Status = release.Status
			if candidate.Status == "" {
				candidate.Status = "released"
			}
			if release.TagURL != "" {
				candidate.TagURL = release.TagURL
			}
		}
		candidates = append(candidates, candidate)
		seen[tag.Name] = struct{}{}
	}
	for _, release := range releases {
		if _, exists := seen[release.TagName]; exists {
			continue
		}
		releaseCopy := release
		status := release.Status
		if status == "" {
			status = "released"
		}
		candidates = append(candidates, ReleaseCandidate{TagName: release.TagName, TagURL: release.TagURL, Status: status, Release: &releaseCopy})
	}
	return candidates, nil
}

// CreateBranchForRepository creates a branch in a workspace repository.
// This method implements the plugins.SCMService interface.
// If userID is provided (> 0), it uses the user's personal OAuth token for OAuth providers.
func (s *SyncService) CreateBranchForRepository(ctx context.Context, workspaceRepoID int, branchName, baseBranch string, userID ...int) (string, error) {
	var uid int
	if len(userID) > 0 {
		uid = userID[0]
	}

	rc, err := s.resolveRepoAndProvider(ctx, workspaceRepoID, uid)
	if err != nil {
		return "", err
	}

	// Use default branch if base branch not specified
	if baseBranch == "" {
		baseBranch = rc.DefaultBranch
		if baseBranch == "" {
			baseBranch = "main"
		}
	}

	// Create the branch
	if err := rc.Provider.CreateBranch(ctx, rc.Owner, rc.Repo, branchName, baseBranch); err != nil {
		return "", fmt.Errorf("failed to create branch: %w", err)
	}

	branchURL := branchWebURL(rc.Provider, rc.Owner, rc.Repo, branchName)

	slog.Debug("Created branch", slog.String("component", "scm"), slog.String("branch", branchName), slog.String("repository", rc.RepositoryName))
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
	exists, err := repository.NewItemRepository(s.db).Exists(itemID)
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
	if !errors.Is(err, sql.ErrNoRows) {
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
	var uid int
	if len(userID) > 0 {
		uid = userID[0]
	}

	rc, err := s.resolveRepoAndProvider(ctx, workspaceRepoID, uid)
	if err != nil {
		return nil, "", err
	}

	// Use default branch if base branch not specified
	if opts.BaseBranch == "" {
		opts.BaseBranch = rc.DefaultBranch
		if opts.BaseBranch == "" {
			opts.BaseBranch = "main"
		}
	}

	// Create the pull request
	pr, err := rc.Provider.CreatePullRequest(ctx, rc.Owner, rc.Repo, opts)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create pull request: %w", err)
	}

	// Build the PR URL -- prefer the URL returned by the provider API
	prURL := pr.URL
	if prURL == "" {
		// Fallback: construct URL manually
		switch rc.ProviderType {
		case models.SCMProviderTypeGitea:
			prURL = fmt.Sprintf("%s/%s/pulls/%d", rc.BaseURL, rc.RepositoryName, pr.Number)
		case models.SCMProviderTypeGitLab:
			prURL = fmt.Sprintf("%s/%s/-/merge_requests/%d", rc.BaseURL, rc.RepositoryName, pr.Number)
		default:
			prURL = fmt.Sprintf("%s/%s/pull/%d", rc.BaseURL, rc.RepositoryName, pr.Number)
		}
	}

	slog.Debug("Created pull request", slog.String("component", "scm"), slog.Int("pr_number", pr.Number), slog.String("repository", rc.RepositoryName))
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
	return s.refreshItemSCMLink(ctx, linkID, &userID)
}

// scanLinkIDs runs a SELECT that returns a single int-id column and collects
// the rows into a []int. Used by the two bulk-refresh paths below.
func scanLinkIDs(rows *sql.Rows) []int {
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
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
	linkIDs := scanLinkIDs(rows)
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
// This should be called periodically by the scheduler. Links are bucketed
// by connection so the per-connection concurrency cap applies per-token
// (a single noisy connection cannot exhaust its own rate-limit budget,
// nor starve refresh attempts on other connections).
func (s *SyncService) RefreshAllPRLinkStates(ctx context.Context) error {
	if !s.refreshMu.TryLock() {
		slog.Info("SCM PR refresh skipped: previous run still active", slog.String("component", "scm"))
		return nil
	}
	defer s.refreshMu.Unlock()

	// Include every enabled non-OAuth connection, even when it currently has no
	// refreshable links. A zero-link successful attempt can clear stale health
	// after a broken link is removed.
	rows, err := s.db.QueryContext(ctx, `
		SELECT wsc.id, isl.id, wr.repository_name, isl.external_id
		FROM workspace_scm_connections wsc
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		LEFT JOIN workspace_repositories wr
			ON wr.workspace_scm_connection_id = wsc.id AND wr.is_active = true
		LEFT JOIN item_scm_links isl
			ON isl.workspace_repository_id = wr.id
			AND isl.link_type = 'pull_request'
			AND (isl.state IS NULL OR isl.state != 'merged')
		WHERE wsc.enabled = true AND sp.auth_method != 'oauth'
	`)
	if err != nil {
		return fmt.Errorf("failed to query PR links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	linksByConnection := make(map[int][]prRefreshTarget)
	totalLinks := 0
	for rows.Next() {
		var connectionID int
		var linkID sql.NullInt64
		var repositoryName, externalID sql.NullString
		if err := rows.Scan(&connectionID, &linkID, &repositoryName, &externalID); err != nil {
			return fmt.Errorf("scan PR refresh connection: %w", err)
		}
		if _, exists := linksByConnection[connectionID]; !exists {
			linksByConnection[connectionID] = nil
		}
		if linkID.Valid {
			linksByConnection[connectionID] = append(linksByConnection[connectionID], prRefreshTarget{
				LinkID:         int(linkID.Int64),
				RepositoryName: repositoryName.String,
				ExternalID:     externalID.String,
			})
			totalLinks++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate PR links: %w", err)
	}
	if len(linksByConnection) == 0 {
		return nil
	}

	slog.Debug("Refreshing state for PR links", slog.String("component", "scm"), slog.Int("count", totalLinks), slog.Int("connections", len(linksByConnection)))

	for connectionID, targets := range linksByConnection {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		sem := make(chan struct{}, syncPerConnectionConcurrency)
		var wg sync.WaitGroup
		var mu sync.Mutex
		var refreshErrors []error
		for _, target := range targets {
			if ctx.Err() != nil {
				break
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(target prRefreshTarget) {
				defer wg.Done()
				defer func() { <-sem }()
				if err := s.RefreshItemSCMLink(ctx, target.LinkID); err != nil {
					mu.Lock()
					refreshErrors = append(refreshErrors, fmt.Errorf("repository %s PR #%s: %w", target.RepositoryName, strings.TrimPrefix(target.ExternalID, "#"), err))
					mu.Unlock()
				}
			}(target)
		}
		wg.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.recordConnectionHealth(ctx, repository.SCMHealthResult{
			ConnectionID:     connectionID,
			Operation:        repository.SCMHealthOperationPRLinkRefresh,
			AttemptedAt:      time.Now().UTC(),
			CheckedResources: len(targets),
			FailedResources:  len(refreshErrors),
			LastError:        joinedError(refreshErrors),
		})
	}

	return nil
}

func (s *SyncService) recordConnectionHealth(ctx context.Context, result repository.SCMHealthResult) {
	if s.healthRepo == nil {
		s.healthRepo = repository.NewSCMHealthRepository(s.db)
	}
	transition, err := s.healthRepo.RecordResult(ctx, result)
	if err != nil {
		slog.Warn("Failed to record SCM connection health",
			slog.String("component", "scm"),
			slog.Int("connection_id", result.ConnectionID),
			slog.String("operation", result.Operation),
			slog.Any("error", err),
		)
		return
	}
	attributes := make([]any, 0, 6)
	attributes = append(attributes,
		slog.String("component", "scm"),
		slog.Int("connection_id", result.ConnectionID),
		slog.String("operation", result.Operation),
		slog.Int("checked_resources", result.CheckedResources),
		slog.Int("failed_resources", result.FailedResources),
	)
	if transition.Recovered {
		slog.Info("SCM connection recovered", attributes...)
		return
	}
	if result.FailedResources == 0 {
		return
	}
	attributes = append(attributes, slog.String("error", redact.String(result.LastError)))
	if transition.BecameUnhealthy || transition.ErrorChanged {
		slog.Warn("SCM connection operation unhealthy", attributes...)
		return
	}
	// Unchanged failures remain observable at debug level while diagnostics
	// retain the durable counter and latest sanitized error.
	slog.Debug("SCM connection operation still unhealthy", attributes...)
}

func joinedError(operationErrors []error) string {
	if len(operationErrors) == 0 {
		return ""
	}
	return errors.Join(operationErrors...).Error()
}
