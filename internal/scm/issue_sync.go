package scm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sso"
)

// queryExecer is the common subset of database.Database and database.Tx used by
// helper methods so they can run inside or outside an explicit transaction.
type queryExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// IssueSyncService handles synchronization of GitHub Issues into Windshift items.
type IssueSyncService struct {
	db          database.Database
	itemRepo    *repository.ItemRepository
	encryption  *sso.SecretEncryption
	syncMu      sync.Mutex
	userService interface {
		GetByID(int) (*models.User, error)
	}
}

// SetUserService sets the user service for looking up comment authors.
func (s *IssueSyncService) SetUserService(us interface {
	GetByID(int) (*models.User, error)
}) {
	s.userService = us
}

// NewIssueSyncService creates a new IssueSyncService.
func NewIssueSyncService(db database.Database, encryption *sso.SecretEncryption) *IssueSyncService {
	return &IssueSyncService{db: db, itemRepo: repository.NewItemRepository(db), encryption: encryption}
}

// SyncAll finds all enabled issue sync configs and syncs each one.
func (s *IssueSyncService) SyncAll(ctx context.Context) error {
	if !s.syncMu.TryLock() {
		slog.Info("Issue sync skipped: previous run still active")
		return nil
	}
	defer s.syncMu.Unlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT isc.id, isc.workspace_repository_id, isc.status_mapping, isc.reverse_status_mapping,
			   isc.label_sync_mode, isc.label_mappings, isc.filter_labels,
			   isc.assignee_mappings, isc.milestone_mappings,
			   isc.default_item_type_id, isc.default_priority_id, isc.sync_comments,
			   isc.last_full_sync_at,
			   wr.repository_name, wr.workspace_scm_connection_id,
			   wsc.scm_provider_id, wsc.workspace_id
		FROM issue_sync_configs isc
		JOIN workspace_repositories wr ON wr.id = isc.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE isc.sync_enabled = ?
		  AND wr.is_active = ?
		  AND wsc.enabled = ?
	`, true, true, true)
	if err != nil {
		return fmt.Errorf("query issue sync configs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type syncJob struct {
		config       models.IssueSyncConfig
		repoName     string
		connectionID int
		providerID   int
		workspaceID  int
	}

	var jobs []syncJob
	for rows.Next() {
		var j syncJob
		var lastSync sql.NullTime
		var defaultItemType, defaultPriority sql.NullInt64
		if err := rows.Scan(
			&j.config.ID, &j.config.WorkspaceRepositoryID,
			&j.config.StatusMapping, &j.config.ReverseStatusMapping,
			&j.config.LabelSyncMode, &j.config.LabelMappings, &j.config.FilterLabels,
			&j.config.AssigneeMappings, &j.config.MilestoneMappings,
			&defaultItemType, &defaultPriority, &j.config.SyncComments,
			&lastSync,
			&j.repoName, &j.connectionID, &j.providerID, &j.workspaceID,
		); err != nil {
			slog.Error("scan issue sync config", "error", err)
			continue
		}
		if lastSync.Valid {
			j.config.LastFullSyncAt = &lastSync.Time
		}
		if defaultItemType.Valid {
			v := int(defaultItemType.Int64)
			j.config.DefaultItemTypeID = &v
		}
		if defaultPriority.Valid {
			v := int(defaultPriority.Int64)
			j.config.DefaultPriorityID = &v
		}
		j.config.WorkspaceID = j.workspaceID
		jobs = append(jobs, j)
	}

	credResolver := &CredentialResolver{db: s.db, encryption: s.encryption}

	for _, j := range jobs {
		provider, err := credResolver.GetProviderForConnection(ctx, j.connectionID)
		if err != nil {
			slog.Error("resolve provider for issue sync", "config_id", j.config.ID, "error", err)
			s.recordSyncError(j.config.ID, err.Error())
			continue
		}

		issueProvider, ok := provider.(IssueProvider)
		if !ok {
			slog.Warn("provider does not support issues", "config_id", j.config.ID)
			s.recordSyncError(j.config.ID, "provider does not support issue sync")
			continue
		}

		if err := s.syncConfig(ctx, issueProvider, &j.config, j.repoName); err != nil {
			slog.Error("issue sync failed", "config_id", j.config.ID, "repo", j.repoName, "error", err)
			s.recordSyncError(j.config.ID, err.Error())
		} else {
			// Clear error on success and update last sync time
			now := time.Now()
			_, _ = s.db.ExecContext(ctx,
				"UPDATE issue_sync_configs SET last_full_sync_at = ?, last_sync_error = NULL, updated_at = ? WHERE id = ?",
				now, now, j.config.ID)
		}
	}

	return nil
}

// syncConfig syncs a single issue sync configuration.
func (s *IssueSyncService) syncConfig(ctx context.Context, provider IssueProvider, config *models.IssueSyncConfig, repoName string) error {
	parts := strings.SplitN(repoName, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository name: %s", repoName)
	}
	owner, repo := parts[0], parts[1]

	// Parse filter labels
	var filterLabels []string
	if config.FilterLabels != "" && config.FilterLabels != "[]" {
		_ = json.Unmarshal([]byte(config.FilterLabels), &filterLabels)
	}

	opts := ListIssueOptions{
		State:   "all",
		Since:   config.LastFullSyncAt,
		PerPage: 100,
	}
	if len(filterLabels) > 0 {
		opts.Labels = filterLabels
	}

	// Paginate through all issues
	page := 1
	for {
		opts.Page = page
		issues, err := provider.ListIssues(ctx, owner, repo, opts)
		if err != nil {
			if errors.Is(err, ErrRateLimited) {
				return fmt.Errorf("list issues page %d: %w", page, err)
			}
			return fmt.Errorf("list issues page %d: %w", page, err)
		}

		for i := range issues {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("sync interrupted: %w", err)
			}
			if err := s.syncIssue(ctx, provider, config, owner, repo, &issues[i]); err != nil {
				slog.Error("sync issue", "config_id", config.ID, "issue_number", issues[i].Number, "error", err)
			}
		}

		if len(issues) < opts.PerPage {
			break
		}
		page++
	}

	return nil
}

// syncIssue syncs a single GitHub issue to a Windshift item.
func (s *IssueSyncService) syncIssue(ctx context.Context, provider IssueProvider, config *models.IssueSyncConfig, owner, repo string, issue *Issue) error {
	// Check if we already have this issue mapped
	var syncItemID int
	var itemID int
	var lastGHUpdated sql.NullTime
	var syncLock bool

	err := s.db.QueryRowContext(ctx,
		"SELECT id, item_id, last_github_updated_at, sync_lock FROM issue_sync_items WHERE issue_sync_config_id = ? AND github_issue_number = ?",
		config.ID, issue.Number,
	).Scan(&syncItemID, &itemID, &lastGHUpdated, &syncLock)

	if errors.Is(err, sql.ErrNoRows) {
		// New issue — create Windshift item
		if err := s.createItemFromIssue(ctx, config, issue); err != nil {
			return err
		}
		// After creating the item, sync comments if enabled
		if config.SyncComments {
			// Look up the newly created sync item
			var newSyncItemID, newItemID int
			if lookupErr := s.db.QueryRowContext(ctx,
				"SELECT id, item_id FROM issue_sync_items WHERE issue_sync_config_id = ? AND github_issue_number = ?",
				config.ID, issue.Number,
			).Scan(&newSyncItemID, &newItemID); lookupErr == nil {
				s.syncComments(ctx, provider, owner, repo, issue.Number, newSyncItemID, newItemID)
			}
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("lookup sync item: %w", err)
	}

	// If sync_lock is set, this was recently pushed back from Windshift — skip once and clear lock
	if syncLock {
		now := time.Now()
		_, _ = s.db.ExecContext(ctx,
			"UPDATE issue_sync_items SET sync_lock = ?, last_github_updated_at = ?, last_synced_at = ?, updated_at = ? WHERE id = ?",
			false, issue.UpdatedAt, now, now, syncItemID)
		return nil
	}

	// Check if GitHub issue has been updated since last sync
	if lastGHUpdated.Valid && !issue.UpdatedAt.After(lastGHUpdated.Time) {
		// Even if issue hasn't changed, still sync comments (they update independently)
		if config.SyncComments {
			s.syncComments(ctx, provider, owner, repo, issue.Number, syncItemID, itemID)
		}
		return nil
	}

	// Update existing item
	if err := s.updateItemFromIssue(ctx, config, issue, itemID, syncItemID); err != nil {
		return err
	}

	// Sync comments if enabled
	if config.SyncComments {
		s.syncComments(ctx, provider, owner, repo, issue.Number, syncItemID, itemID)
	}

	return nil
}

// createItemFromIssue creates a new Windshift item from a GitHub issue.
func (s *IssueSyncService) createItemFromIssue(ctx context.Context, config *models.IssueSyncConfig, issue *Issue) error {
	// Resolve status
	statusID := s.resolveStatusID(config, issue.State)

	// Resolve assignee
	assigneeID := s.resolveAssigneeID(config, issue)

	// Resolve milestone
	milestoneID := s.resolveMilestoneID(config, issue)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Get next workspace item number
	nextNum, err := s.itemRepo.GetNextWorkspaceItemNumber(tx, config.WorkspaceID)
	if err != nil {
		return fmt.Errorf("get next item number: %w", err)
	}

	newItem := &models.Item{
		WorkspaceID:         config.WorkspaceID,
		WorkspaceItemNumber: nextNum,
		ItemTypeID:          config.DefaultItemTypeID,
		Title:               issue.Title,
		Description:         issue.Body,
		StatusID:            statusID,
		PriorityID:          config.DefaultPriorityID,
		AssigneeID:          assigneeID,
	}
	itemID, err := s.itemRepo.Create(tx, newItem)
	if err != nil {
		return fmt.Errorf("create item: %w", err)
	}

	// Carry the inferred milestone (if any) into item_milestones.
	if milestoneID != nil {
		if _, err := tx.Exec(
			"INSERT INTO item_milestones (item_id, milestone_id, created_at) VALUES (?, ?, ?)",
			itemID, *milestoneID, time.Now(),
		); err != nil {
			return fmt.Errorf("attach milestone: %w", err)
		}
	}

	// Create sync item mapping
	now := time.Now()
	_, err = tx.Exec(`
		INSERT INTO issue_sync_items (
			issue_sync_config_id, item_id, github_issue_number, github_issue_id,
			github_issue_url, last_synced_at, last_github_updated_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		config.ID, itemID, issue.Number, issue.ID,
		issue.URL, now, issue.UpdatedAt, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert sync item: %w", err)
	}

	// Sync labels inside the transaction so item + labels are atomic
	s.syncLabels(ctx, tx, config, issue, itemID)

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	slog.Info("created item from GitHub issue",
		"config_id", config.ID, "issue_number", issue.Number, "item_id", itemID)

	return nil
}

// updateItemFromIssue updates an existing Windshift item from a changed GitHub issue.
func (s *IssueSyncService) updateItemFromIssue(ctx context.Context, config *models.IssueSyncConfig, issue *Issue, itemID, syncItemID int) error {
	statusID := s.resolveStatusID(config, issue.State)
	assigneeID := s.resolveAssigneeID(config, issue)
	milestoneID := s.resolveMilestoneID(config, issue)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.itemRepo.UpdateFields(tx, itemID, map[string]interface{}{
		"title":        issue.Title,
		"description":  issue.Body,
		"status_id":    statusID,
		"assignee_id":  assigneeID,
		"milestone_id": milestoneID,
	}); err != nil {
		return fmt.Errorf("update item: %w", err)
	}

	// Update sync tracking
	now := time.Now()
	_, _ = tx.ExecContext(ctx,
		"UPDATE issue_sync_items SET last_synced_at = ?, last_github_updated_at = ?, updated_at = ? WHERE id = ?",
		now, issue.UpdatedAt, now, syncItemID)

	// Sync labels inside the transaction
	s.syncLabels(ctx, tx, config, issue, itemID)

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	slog.Info("updated item from GitHub issue",
		"config_id", config.ID, "issue_number", issue.Number, "item_id", itemID)

	return nil
}

// PushStatusToGitHub pushes a Windshift status change back to GitHub.
func (s *IssueSyncService) PushStatusToGitHub(ctx context.Context, itemID, newStatusID int) {
	// Look up sync item
	var syncItemID int
	var configID int
	var issueNumber int
	var repoName string
	var connectionID int
	var reverseMapping string

	err := s.db.QueryRowContext(ctx, `
		SELECT isi.id, isi.issue_sync_config_id, isi.github_issue_number,
			   wr.repository_name, wr.workspace_scm_connection_id,
			   isc.reverse_status_mapping
		FROM issue_sync_items isi
		JOIN issue_sync_configs isc ON isc.id = isi.issue_sync_config_id
		JOIN workspace_repositories wr ON wr.id = isc.workspace_repository_id
		WHERE isi.item_id = ? AND isc.sync_enabled = ?
	`, itemID, true).Scan(&syncItemID, &configID, &issueNumber, &repoName, &connectionID, &reverseMapping)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("lookup sync item for pushback", "item_id", itemID, "error", err)
		}
		return
	}

	// Parse reverse status mapping
	var statusMap map[string]string
	if err := json.Unmarshal([]byte(reverseMapping), &statusMap); err != nil {
		slog.Error("parse reverse status mapping", "config_id", configID, "error", err)
		return
	}

	ghState, ok := statusMap[strconv.Itoa(newStatusID)]
	if !ok {
		return // No mapping for this status
	}

	// Set sync lock before pushing
	_, _ = s.db.ExecContext(ctx,
		"UPDATE issue_sync_items SET sync_lock = ?, updated_at = ? WHERE id = ?",
		true, time.Now(), syncItemID)

	// Resolve provider
	credResolver := &CredentialResolver{db: s.db, encryption: s.encryption}
	provider, err := credResolver.GetProviderForConnection(ctx, connectionID)
	if err != nil {
		slog.Error("resolve provider for status pushback", "config_id", configID, "error", err)
		return
	}

	issueProvider, ok := provider.(IssueProvider)
	if !ok {
		slog.Error("provider does not support issues for pushback", "config_id", configID)
		return
	}

	parts := strings.SplitN(repoName, "/", 2)
	if len(parts) != 2 {
		return
	}

	_, err = issueProvider.UpdateIssue(ctx, parts[0], parts[1], issueNumber, UpdateIssueOptions{
		State: &ghState,
	})
	if err != nil {
		slog.Error("push status to GitHub", "config_id", configID, "issue", issueNumber, "state", ghState, "error", err)
		// Clear lock on failure so next sync can pick it up
		_, _ = s.db.ExecContext(ctx,
			"UPDATE issue_sync_items SET sync_lock = ?, updated_at = ? WHERE id = ?",
			false, time.Now(), syncItemID)
	}
}

// PushCommentToGitHub pushes a Windshift comment to a linked GitHub issue.
func (s *IssueSyncService) PushCommentToGitHub(ctx context.Context, itemID, commentID, authorID int, commentBody string) {
	// Look up author display name
	if s.userService != nil {
		if user, err := s.userService.GetByID(authorID); err == nil {
			authorName := strings.TrimSpace(user.FullName)
			if authorName == "" {
				authorName = user.Username
			}
			if authorName != "" {
				commentBody = fmt.Sprintf("**%s** commented in Windshift:\n\n%s", authorName, commentBody)
			}
		}
	}
	var syncItemID int
	var issueNumber int
	var repoName string
	var connectionID int

	err := s.db.QueryRowContext(ctx, `
		SELECT isi.id, isi.github_issue_number, wr.repository_name, wr.workspace_scm_connection_id
		FROM issue_sync_items isi
		JOIN issue_sync_configs isc ON isc.id = isi.issue_sync_config_id
		JOIN workspace_repositories wr ON wr.id = isc.workspace_repository_id
		WHERE isi.item_id = ? AND isc.sync_enabled = ? AND isc.sync_comments = ?
	`, itemID, true, true).Scan(&syncItemID, &issueNumber, &repoName, &connectionID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("lookup sync item for comment pushback", "item_id", itemID, "error", err)
		}
		return
	}

	credResolver := &CredentialResolver{db: s.db, encryption: s.encryption}
	provider, err := credResolver.GetProviderForConnection(ctx, connectionID)
	if err != nil {
		slog.Error("resolve provider for comment pushback", "error", err)
		return
	}

	issueProvider, ok := provider.(IssueProvider)
	if !ok {
		return
	}

	parts := strings.SplitN(repoName, "/", 2)
	if len(parts) != 2 {
		return
	}

	ghCommentID, err := issueProvider.CreateIssueComment(ctx, parts[0], parts[1], issueNumber, commentBody)
	if err != nil {
		slog.Error("push comment to GitHub", "issue", issueNumber, "error", err)
		return
	}

	// Track the comment mapping
	now := time.Now()
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO issue_sync_comments (issue_sync_item_id, comment_id, github_comment_id, github_updated_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, syncItemID, commentID, ghCommentID, now, now, now)
}

// PushCommentUpdateToGitHub pushes a Windshift comment edit to the linked GitHub comment.
func (s *IssueSyncService) PushCommentUpdateToGitHub(ctx context.Context, commentID, authorID int, newBody string) {
	// Look up the GitHub comment ID and repo info via the tracking table
	var ghCommentID int64
	var repoName string
	var connectionID int

	err := s.db.QueryRowContext(ctx, `
		SELECT isc2.github_comment_id, wr.repository_name, wr.workspace_scm_connection_id
		FROM issue_sync_comments isc2
		JOIN issue_sync_items isi ON isi.id = isc2.issue_sync_item_id
		JOIN issue_sync_configs isc ON isc.id = isi.issue_sync_config_id
		JOIN workspace_repositories wr ON wr.id = isc.workspace_repository_id
		WHERE isc2.comment_id = ? AND isc.sync_enabled = ? AND isc.sync_comments = ?
	`, commentID, true, true).Scan(&ghCommentID, &repoName, &connectionID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("lookup sync comment for update pushback", "comment_id", commentID, "error", err)
		}
		return
	}

	// Add author prefix
	if s.userService != nil {
		if user, err := s.userService.GetByID(authorID); err == nil {
			authorName := strings.TrimSpace(user.FullName)
			if authorName == "" {
				authorName = user.Username
			}
			if authorName != "" {
				newBody = fmt.Sprintf("**%s** commented in Windshift:\n\n%s", authorName, newBody)
			}
		}
	}

	credResolver := &CredentialResolver{db: s.db, encryption: s.encryption}
	provider, err := credResolver.GetProviderForConnection(ctx, connectionID)
	if err != nil {
		slog.Error("resolve provider for comment update pushback", "error", err)
		return
	}

	issueProvider, ok := provider.(IssueProvider)
	if !ok {
		return
	}

	parts := strings.SplitN(repoName, "/", 2)
	if len(parts) != 2 {
		return
	}

	if err := issueProvider.UpdateIssueComment(ctx, parts[0], parts[1], ghCommentID, newBody); err != nil {
		slog.Error("push comment update to GitHub", "github_comment_id", ghCommentID, "error", err)
	}
}

// syncComments pulls GitHub issue comments into Windshift.
func (s *IssueSyncService) syncComments(ctx context.Context, provider IssueProvider, owner, repo string, issueNumber, syncItemID, itemID int) {
	// API call stays outside the transaction to avoid holding a write lock
	comments, err := provider.ListIssueComments(ctx, owner, repo, issueNumber)
	if err != nil {
		slog.Error("list issue comments", "issue_number", issueNumber, "error", err)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("begin comment sync tx", "issue_number", issueNumber, "error", err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	for _, ghComment := range comments {
		// Skip comments that were pushed from Windshift (contain our attribution marker)
		if strings.Contains(ghComment.Body, "commented in Windshift:") && strings.HasPrefix(ghComment.Body, "**") {
			continue
		}

		// Check if we already track this GitHub comment
		var trackingID int
		var existingCommentID sql.NullInt64
		var lastGHUpdated sql.NullTime

		err := tx.QueryRowContext(ctx,
			"SELECT id, comment_id, github_updated_at FROM issue_sync_comments WHERE issue_sync_item_id = ? AND github_comment_id = ?",
			syncItemID, ghComment.ID,
		).Scan(&trackingID, &existingCommentID, &lastGHUpdated)

		if errors.Is(err, sql.ErrNoRows) {
			// New comment from GitHub — create in Windshift
			body := fmt.Sprintf("**@%s** commented on GitHub:\n\n%s", ghComment.User.Username, ghComment.Body)
			now := time.Now()

			var wsCommentID int64
			insertErr := tx.QueryRowContext(ctx, `
				INSERT INTO comments (item_id, author_id, content, created_at, updated_at)
				VALUES (?, NULL, ?, ?, ?) RETURNING id
			`, itemID, body, now, now).Scan(&wsCommentID)
			if insertErr != nil {
				slog.Error("insert synced comment", "github_comment_id", ghComment.ID, "error", insertErr)
				continue
			}

			// Create tracking row
			_, _ = tx.ExecContext(ctx, `
				INSERT INTO issue_sync_comments (issue_sync_item_id, comment_id, github_comment_id, github_updated_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)
			`, syncItemID, wsCommentID, ghComment.ID, ghComment.UpdatedAt, now, now)

			continue
		}
		if err != nil {
			slog.Error("lookup sync comment", "github_comment_id", ghComment.ID, "error", err)
			continue
		}

		// Existing tracked comment — check if GitHub version is newer
		if !existingCommentID.Valid {
			continue // Windshift comment was deleted, skip
		}
		if lastGHUpdated.Valid && !ghComment.UpdatedAt.After(lastGHUpdated.Time) {
			continue // No changes
		}

		// Update the Windshift comment content
		body := fmt.Sprintf("**@%s** commented on GitHub:\n\n%s", ghComment.User.Username, ghComment.Body)
		now := time.Now()
		_, _ = tx.ExecContext(ctx,
			"UPDATE comments SET content = ?, updated_at = ? WHERE id = ?",
			body, now, existingCommentID.Int64)
		_, _ = tx.ExecContext(ctx,
			"UPDATE issue_sync_comments SET github_updated_at = ?, updated_at = ? WHERE id = ?",
			ghComment.UpdatedAt, now, trackingID)
	}

	if err := tx.Commit(); err != nil {
		slog.Error("commit comment sync tx", "issue_number", issueNumber, "error", err)
	}
}

// GetSyncConfigForWorkspace returns the sync config for a workspace, if any.
func (s *IssueSyncService) GetSyncConfigForWorkspace(ctx context.Context, workspaceID int) (*models.IssueSyncConfig, error) {
	var config models.IssueSyncConfig
	var lastSync sql.NullTime
	var lastError sql.NullString
	var defaultItemType, defaultPriority sql.NullInt64
	var createdBy sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
		SELECT isc.id, isc.workspace_repository_id, isc.sync_enabled,
			   isc.status_mapping, isc.reverse_status_mapping,
			   isc.label_sync_mode, isc.label_mappings, isc.filter_labels,
			   isc.assignee_mappings, isc.milestone_mappings,
			   isc.default_item_type_id, isc.default_priority_id, isc.sync_comments,
			   isc.last_full_sync_at, isc.last_sync_error,
			   isc.created_by, isc.created_at, isc.updated_at,
			   wr.repository_name
		FROM issue_sync_configs isc
		JOIN workspace_repositories wr ON wr.id = isc.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wsc.workspace_id = ?
		LIMIT 1
	`, workspaceID).Scan(
		&config.ID, &config.WorkspaceRepositoryID, &config.SyncEnabled,
		&config.StatusMapping, &config.ReverseStatusMapping,
		&config.LabelSyncMode, &config.LabelMappings, &config.FilterLabels,
		&config.AssigneeMappings, &config.MilestoneMappings,
		&defaultItemType, &defaultPriority, &config.SyncComments,
		&lastSync, &lastError,
		&createdBy, &config.CreatedAt, &config.UpdatedAt,
		&config.RepositoryName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if lastSync.Valid {
		config.LastFullSyncAt = &lastSync.Time
	}
	if lastError.Valid {
		config.LastSyncError = lastError.String
	}
	if defaultItemType.Valid {
		v := int(defaultItemType.Int64)
		config.DefaultItemTypeID = &v
	}
	if defaultPriority.Valid {
		v := int(defaultPriority.Int64)
		config.DefaultPriorityID = &v
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		config.CreatedBy = &v
	}
	config.WorkspaceID = workspaceID

	// Get synced item count
	_ = s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM issue_sync_items WHERE issue_sync_config_id = ?", config.ID,
	).Scan(&config.SyncedItemCount)

	return &config, nil
}

// VerifyRepositoryInWorkspace returns true when the given workspace repository
// belongs to the given workspace.
func (s *IssueSyncService) VerifyRepositoryInWorkspace(ctx context.Context, workspaceRepositoryID, workspaceID int) (bool, error) {
	var repoWorkspaceID int
	err := s.db.QueryRowContext(ctx, `
		SELECT wsc.workspace_id FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wr.id = ?
	`, workspaceRepositoryID).Scan(&repoWorkspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return repoWorkspaceID == workspaceID, nil
}

// CreateSyncConfig inserts a new issue sync configuration row, applying the
// caller-supplied request defaults. Returns the new config ID.
func (s *IssueSyncService) CreateSyncConfig(ctx context.Context, createdByUserID int, req models.IssueSyncConfigRequest) (int, error) {
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
	err := s.db.QueryRowContext(ctx, `
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
		req.SyncComments, createdByUserID,
	).Scan(&configID)
	return configID, err
}

// UpdateSyncConfig updates the writable fields on a sync config row.
func (s *IssueSyncService) UpdateSyncConfig(ctx context.Context, configID int, req models.IssueSyncConfigRequest) error {
	_, err := s.db.ExecContext(ctx, `
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
		req.SyncComments, configID,
	)
	return err
}

// DeleteSyncConfig removes a sync config row. Cascades clean up linked
// issue_sync_items rows.
func (s *IssueSyncService) DeleteSyncConfig(ctx context.Context, configID int) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM issue_sync_configs WHERE id = ?", configID)
	return err
}

// GetSyncedItems returns all synced items for a config.
func (s *IssueSyncService) GetSyncedItems(ctx context.Context, configID int) ([]models.IssueSyncItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT isi.id, isi.issue_sync_config_id, isi.item_id,
			   isi.github_issue_number, isi.github_issue_id, isi.github_issue_url,
			   isi.last_synced_at, isi.last_github_updated_at, isi.sync_lock,
			   isi.created_at, isi.updated_at,
			   i.title, i.workspace_item_number, w.key
		FROM issue_sync_items isi
		JOIN items i ON i.id = isi.item_id
		JOIN workspaces w ON w.id = i.workspace_id
		WHERE isi.issue_sync_config_id = ?
		ORDER BY isi.github_issue_number
	`, configID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []models.IssueSyncItem
	for rows.Next() {
		var item models.IssueSyncItem
		var lastSync, lastGH sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.IssueSyncConfigID, &item.ItemID,
			&item.GitHubIssueNumber, &item.GitHubIssueID, &item.GitHubIssueURL,
			&lastSync, &lastGH, &item.SyncLock,
			&item.CreatedAt, &item.UpdatedAt,
			&item.ItemTitle, &item.WorkspaceItemNumber, &item.WorkspaceKey,
		); err != nil {
			return nil, err
		}
		if lastSync.Valid {
			item.LastSyncedAt = &lastSync.Time
		}
		if lastGH.Valid {
			item.LastGitHubUpdatedAt = &lastGH.Time
		}
		items = append(items, item)
	}
	return items, nil
}

// TriggerSync runs a single sync for a specific config.
func (s *IssueSyncService) TriggerSync(ctx context.Context, configID int) error {
	var repoName string
	var connectionID int
	var config models.IssueSyncConfig
	var lastSync sql.NullTime
	var defaultItemType, defaultPriority sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
		SELECT isc.id, isc.workspace_repository_id, isc.status_mapping, isc.reverse_status_mapping,
			   isc.label_sync_mode, isc.label_mappings, isc.filter_labels,
			   isc.assignee_mappings, isc.milestone_mappings,
			   isc.default_item_type_id, isc.default_priority_id, isc.sync_comments,
			   isc.last_full_sync_at,
			   wr.repository_name, wr.workspace_scm_connection_id,
			   wsc.workspace_id
		FROM issue_sync_configs isc
		JOIN workspace_repositories wr ON wr.id = isc.workspace_repository_id
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE isc.id = ?
	`, configID).Scan(
		&config.ID, &config.WorkspaceRepositoryID,
		&config.StatusMapping, &config.ReverseStatusMapping,
		&config.LabelSyncMode, &config.LabelMappings, &config.FilterLabels,
		&config.AssigneeMappings, &config.MilestoneMappings,
		&defaultItemType, &defaultPriority, &config.SyncComments,
		&lastSync,
		&repoName, &connectionID, &config.WorkspaceID,
	)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if lastSync.Valid {
		config.LastFullSyncAt = &lastSync.Time
	}
	if defaultItemType.Valid {
		v := int(defaultItemType.Int64)
		config.DefaultItemTypeID = &v
	}
	if defaultPriority.Valid {
		v := int(defaultPriority.Int64)
		config.DefaultPriorityID = &v
	}

	credResolver := &CredentialResolver{db: s.db, encryption: s.encryption}
	provider, err := credResolver.GetProviderForConnection(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("resolve provider: %w", err)
	}

	issueProvider, ok := provider.(IssueProvider)
	if !ok {
		return fmt.Errorf("provider does not support issue sync")
	}

	if err := s.syncConfig(ctx, issueProvider, &config, repoName); err != nil {
		s.recordSyncError(config.ID, err.Error())
		return err
	}

	now := time.Now()
	_, _ = s.db.ExecContext(ctx,
		"UPDATE issue_sync_configs SET last_full_sync_at = ?, last_sync_error = NULL, updated_at = ? WHERE id = ?",
		now, now, config.ID)
	return nil
}

// Helper methods

func (s *IssueSyncService) resolveStatusID(config *models.IssueSyncConfig, ghState string) *int {
	var mapping map[string]int
	if err := json.Unmarshal([]byte(config.StatusMapping), &mapping); err != nil {
		return nil
	}
	if id, ok := mapping[ghState]; ok {
		return &id
	}
	return nil
}

func (s *IssueSyncService) resolveAssigneeID(config *models.IssueSyncConfig, issue *Issue) *int {
	if len(issue.Assignees) == 0 {
		return nil
	}
	var mapping map[string]int
	if err := json.Unmarshal([]byte(config.AssigneeMappings), &mapping); err != nil {
		return nil
	}
	// Use first matching assignee
	for _, a := range issue.Assignees {
		if id, ok := mapping[a.Username]; ok {
			return &id
		}
	}
	return nil
}

func (s *IssueSyncService) resolveMilestoneID(config *models.IssueSyncConfig, issue *Issue) *int {
	if issue.Milestone == nil {
		return nil
	}
	var mapping map[string]int
	if err := json.Unmarshal([]byte(config.MilestoneMappings), &mapping); err != nil {
		return nil
	}
	key := strconv.Itoa(issue.Milestone.Number)
	if id, ok := mapping[key]; ok {
		return &id
	}
	return nil
}

func (s *IssueSyncService) syncLabels(ctx context.Context, db queryExecer, config *models.IssueSyncConfig, issue *Issue, itemID int) {
	if config.LabelSyncMode == "" || config.LabelSyncMode == models.IssueSyncLabelNone {
		return
	}

	if config.LabelSyncMode == models.IssueSyncLabelMapped {
		// Use explicit mappings
		var mappings []models.LabelMapping
		if err := json.Unmarshal([]byte(config.LabelMappings), &mappings); err != nil {
			return
		}

		// Build lookup: github label name → windshift label ID
		ghToWS := make(map[string]int)
		for _, m := range mappings {
			ghToWS[m.GitHubLabel] = m.WindshiftLabelID
		}

		// Clear existing labels and set mapped ones
		_, _ = db.ExecContext(ctx, "DELETE FROM item_labels WHERE item_id = ?", itemID)
		for _, l := range issue.Labels {
			if wsLabelID, ok := ghToWS[l.Name]; ok {
				_, _ = db.ExecContext(ctx,
					"INSERT INTO item_labels (item_id, label_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
					itemID, wsLabelID)
			}
		}
	} else if config.LabelSyncMode == models.IssueSyncLabelMirror {
		// Auto-create labels that don't exist
		_, _ = db.ExecContext(ctx, "DELETE FROM item_labels WHERE item_id = ?", itemID)

		for _, l := range issue.Labels {
			// Try to find existing label by name in workspace
			var labelID int
			err := db.QueryRowContext(ctx,
				"SELECT id FROM labels WHERE workspace_id = ? AND LOWER(name) = LOWER(?)",
				config.WorkspaceID, l.Name,
			).Scan(&labelID)
			if errors.Is(err, sql.ErrNoRows) {
				// Create the label
				color := l.Color
				if color == "" {
					color = "808080"
				}
				err = db.QueryRowContext(ctx,
					"INSERT INTO labels (workspace_id, name, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?) RETURNING id",
					config.WorkspaceID, l.Name, color, time.Now(), time.Now(),
				).Scan(&labelID)
				if err != nil {
					continue
				}
			} else if err != nil {
				continue
			}

			_, _ = db.ExecContext(ctx,
				"INSERT INTO item_labels (item_id, label_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
				itemID, labelID)
		}
	}
}

func (s *IssueSyncService) recordSyncError(configID int, errMsg string) {
	_, _ = s.db.Exec(
		"UPDATE issue_sync_configs SET last_sync_error = ?, updated_at = ? WHERE id = ?",
		errMsg, time.Now(), configID)
}

// GetGitHubLabels fetches labels from a GitHub repository for mapping UI.
func (s *IssueSyncService) GetGitHubLabels(ctx context.Context, workspaceRepoID int) ([]IssueLabel, error) {
	provider, repoName, err := s.resolveProviderForRepo(ctx, workspaceRepoID)
	if err != nil {
		return nil, err
	}

	issueProvider, ok := provider.(IssueProvider)
	if !ok {
		return nil, fmt.Errorf("provider does not support issues")
	}

	parts := strings.SplitN(repoName, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository name: %s", repoName)
	}

	return issueProvider.ListRepoLabels(ctx, parts[0], parts[1])
}

// GetGitHubMilestones fetches milestones from a GitHub repository for mapping UI.
func (s *IssueSyncService) GetGitHubMilestones(ctx context.Context, workspaceRepoID int) ([]IssueMilestone, error) {
	provider, repoName, err := s.resolveProviderForRepo(ctx, workspaceRepoID)
	if err != nil {
		return nil, err
	}

	issueProvider, ok := provider.(IssueProvider)
	if !ok {
		return nil, fmt.Errorf("provider does not support issues")
	}

	parts := strings.SplitN(repoName, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository name: %s", repoName)
	}

	return issueProvider.ListRepoMilestones(ctx, parts[0], parts[1])
}

func (s *IssueSyncService) resolveProviderForRepo(ctx context.Context, workspaceRepoID int) (Provider, string, error) {
	var repoName string
	var connectionID int

	err := s.db.QueryRowContext(ctx, `
		SELECT wr.repository_name, wr.workspace_scm_connection_id
		FROM workspace_repositories wr
		WHERE wr.id = ?
	`, workspaceRepoID).Scan(&repoName, &connectionID)
	if err != nil {
		return nil, "", fmt.Errorf("lookup repo: %w", err)
	}

	credResolver := &CredentialResolver{db: s.db, encryption: s.encryption}
	provider, err := credResolver.GetProviderForConnection(ctx, connectionID)
	if err != nil {
		return nil, "", err
	}

	return provider, repoName, nil
}
