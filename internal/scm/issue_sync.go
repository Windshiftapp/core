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
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/sso"
)

// IssueSyncService handles synchronization of GitHub Issues into Windshift items.
type IssueSyncService struct {
	db          database.Database
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
	return &IssueSyncService{db: db, encryption: encryption}
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
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate issue sync configs: %w", err)
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
			_, _ = s.db.ExecWriteContext(ctx,
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

	// Paginate through all issues. Per-issue errors are collected and returned
	// together so callers do NOT advance last_full_sync_at when any issue fails
	// — otherwise a broken issue would be silently skipped forever (the next
	// pull uses last_full_sync_at as the "since" cutoff).
	var issueErrs []error
	page := 1
	for {
		opts.Page = page
		var issues []Issue
		var hasNext bool
		var err error
		if paginated, ok := provider.(PaginatedIssueProvider); ok {
			issues, hasNext, err = paginated.ListIssuesPage(ctx, owner, repo, opts)
		} else {
			// Compatibility fallback for providers that only expose the original
			// issue-list contract. Such providers must not remove entries before
			// returning the page if they rely on the length pagination signal.
			issues, err = provider.ListIssues(ctx, owner, repo, opts)
			hasNext = len(issues) == opts.PerPage
		}
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
				issueErrs = append(issueErrs, fmt.Errorf("issue #%d: %w", issues[i].Number, err))
			}
		}

		if !hasNext {
			break
		}
		page++
	}

	if len(issueErrs) > 0 {
		return fmt.Errorf("sync config %d: %d issue(s) failed: %w", config.ID, len(issueErrs), errors.Join(issueErrs...))
	}
	return nil
}

// syncIssue syncs a single GitHub issue to a Windshift item.
func (s *IssueSyncService) syncIssue(ctx context.Context, provider IssueProvider, config *models.IssueSyncConfig, owner, repo string, issue *Issue) error {
	var syncItemID int
	var itemID int
	var lastGHUpdated sql.NullTime
	var syncLock bool

	err := s.db.QueryRowContext(ctx,
		"SELECT id, item_id, last_github_updated_at, sync_lock FROM issue_sync_items WHERE issue_sync_config_id = ? AND github_issue_number = ?",
		config.ID, issue.Number,
	).Scan(&syncItemID, &itemID, &lastGHUpdated, &syncLock)

	if errors.Is(err, sql.ErrNoRows) {
		if err := s.createItemFromIssue(ctx, config, issue); err != nil {
			return err
		}
		if config.SyncComments {
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
		_, _ = s.db.ExecWriteContext(ctx,
			"UPDATE issue_sync_items SET sync_lock = ?, last_github_updated_at = ?, last_synced_at = ?, updated_at = ? WHERE id = ?",
			false, issue.UpdatedAt, now, now, syncItemID)
		return nil
	}

	if lastGHUpdated.Valid && !issue.UpdatedAt.After(lastGHUpdated.Time) {
		// Comments have their own update cadence.
		if config.SyncComments {
			s.syncComments(ctx, provider, owner, repo, issue.Number, syncItemID, itemID)
		}
		return nil
	}

	if err := s.updateItemFromIssue(ctx, config, issue, itemID, syncItemID); err != nil {
		return err
	}

	if config.SyncComments {
		s.syncComments(ctx, provider, owner, repo, issue.Number, syncItemID, itemID)
	}

	return nil
}

// createItemFromIssue creates a new Windshift item from a GitHub issue.
func (s *IssueSyncService) createItemFromIssue(ctx context.Context, config *models.IssueSyncConfig, issue *Issue) error {
	statusID := s.resolveStatusID(config, issue.State)

	assigneeID := s.resolveAssigneeID(config, issue)

	milestoneID := s.resolveMilestoneID(config, issue)

	input := services.ItemCreateInput{
		WorkspaceID: config.WorkspaceID,
		ItemTypeID:  config.DefaultItemTypeID,
		Title:       issue.Title,
		Description: issue.Body,
		StatusID:    statusID,
		PriorityID:  config.DefaultPriorityID,
		AssigneeID:  assigneeID,
	}

	milestoneIDs := []int{}
	if milestoneID != nil {
		milestoneIDs = append(milestoneIDs, *milestoneID)
	}
	input.MilestoneIDs = milestoneIDs
	item, err := services.NewExternalItemReconciliationService(s.db).Create(ctx, services.ExternalItemCreateRequest{
		Policy: services.GitHubIssueSyncReconciliationPolicy(),
		Input:  input,
		AfterCreate: func(ctx context.Context, tx database.Tx, itemID int) error {
			now := time.Now()
			if _, err := tx.ExecContext(ctx, `
			INSERT INTO issue_sync_items (
				issue_sync_config_id, item_id, github_issue_number, github_issue_id,
				github_issue_url, last_synced_at, last_github_updated_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
				config.ID, itemID, issue.Number, issue.ID,
				issue.URL, now, issue.UpdatedAt, now, now,
			); err != nil {
				return fmt.Errorf("insert sync item: %w", err)
			}

			// Keep item, labels, and sync metadata atomic.
			if err := s.syncLabels(ctx, tx, config, issue, itemID); err != nil {
				return fmt.Errorf("sync labels: %w", err)
			}
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("create item from issue: %w", err)
	}

	slog.Info("created item from GitHub issue",
		"config_id", config.ID, "issue_number", issue.Number, "item_id", item.ID)

	return nil
}

// updateItemFromIssue updates an existing Windshift item from a changed GitHub issue.
func (s *IssueSyncService) updateItemFromIssue(ctx context.Context, config *models.IssueSyncConfig, issue *Issue, itemID, syncItemID int) error {
	statusID := s.resolveStatusID(config, issue.State)
	assigneeID := s.resolveAssigneeID(config, issue)
	milestoneID := s.resolveMilestoneID(config, issue)

	milestoneIDs := []int{}
	if milestoneID != nil {
		milestoneIDs = append(milestoneIDs, *milestoneID)
	}
	var statusValue any
	if statusID != nil {
		statusValue = *statusID
	}
	var assigneeValue any
	if assigneeID != nil {
		assigneeValue = *assigneeID
	}
	_, err := services.NewExternalItemReconciliationService(s.db).Update(ctx, services.ExternalItemUpdateRequest{
		Policy: services.GitHubIssueSyncReconciliationPolicy(),
		ItemID: itemID,
		UpdateData: map[string]any{
			"title":         issue.Title,
			"description":   issue.Body,
			"status_id":     statusValue,
			"assignee_id":   assigneeValue,
			"milestone_ids": milestoneIDs,
		},
		AfterUpdate: func(ctx context.Context, tx database.Tx, _, _ *models.Item) error {
			now := time.Now()
			result, err := tx.ExecContext(ctx,
				"UPDATE issue_sync_items SET last_synced_at = ?, last_github_updated_at = ?, updated_at = ? WHERE id = ?",
				now, issue.UpdatedAt, now, syncItemID)
			if err != nil {
				return fmt.Errorf("update sync item %d: %w", syncItemID, err)
			}
			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("count updated sync items: %w", err)
			}
			if rowsAffected != 1 {
				return fmt.Errorf("sync item %d not found", syncItemID)
			}
			if err := s.syncLabels(ctx, tx, config, issue, itemID); err != nil {
				return fmt.Errorf("sync labels: %w", err)
			}
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("update item from issue: %w", err)
	}

	slog.Info("updated item from GitHub issue",
		"config_id", config.ID, "issue_number", issue.Number, "item_id", itemID)

	return nil
}

// PushStatusToGitHub pushes a Windshift status change back to GitHub.
func (s *IssueSyncService) PushStatusToGitHub(ctx context.Context, itemID, newStatusID int) {
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

	var statusMap map[string]string
	if err := json.Unmarshal([]byte(reverseMapping), &statusMap); err != nil {
		slog.Error("parse reverse status mapping", "config_id", configID, "error", err)
		return
	}

	ghState, ok := statusMap[strconv.Itoa(newStatusID)]
	if !ok {
		return // No mapping for this status
	}

	// Resolve provider. Note: we deliberately do NOT set sync_lock yet — the lock
	// is the signal to the next inbound sync to skip one cycle (loopback
	// prevention), so we only set it if we actually issue the GitHub PATCH.
	// Setting it earlier and bailing on preflight would wedge the item.
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

	// Preflight passed — claim the lock immediately before the remote write.
	_, _ = s.db.ExecWriteContext(ctx,
		"UPDATE issue_sync_items SET sync_lock = ?, updated_at = ? WHERE id = ?",
		true, time.Now(), syncItemID)

	_, err = issueProvider.UpdateIssue(ctx, parts[0], parts[1], issueNumber, UpdateIssueOptions{
		State: &ghState,
	})
	if err != nil {
		slog.Error("push status to GitHub", "config_id", configID, "issue", issueNumber, "state", ghState, "error", err)
		// Clear lock on failure so next sync can pick it up
		_, _ = s.db.ExecWriteContext(ctx,
			"UPDATE issue_sync_items SET sync_lock = ?, updated_at = ? WHERE id = ?",
			false, time.Now(), syncItemID)
	}
}

// PushCommentToGitHub pushes a Windshift comment to a linked GitHub issue.
func (s *IssueSyncService) PushCommentToGitHub(ctx context.Context, itemID, commentID, authorID int, commentBody string) {
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

	now := time.Now()
	_, _ = s.db.ExecWriteContext(ctx, `
		INSERT INTO issue_sync_comments (issue_sync_item_id, comment_id, github_comment_id, github_updated_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, syncItemID, commentID, ghCommentID, now, now, now)
}

// PushCommentUpdateToGitHub pushes a Windshift comment edit to the linked GitHub comment.
func (s *IssueSyncService) PushCommentUpdateToGitHub(ctx context.Context, commentID, authorID int, newBody string) {
	var ghCommentID int64
	var issueNumber int
	var repoName string
	var connectionID int

	err := s.db.QueryRowContext(ctx, `
		SELECT isc2.github_comment_id, isi.github_issue_number, wr.repository_name, wr.workspace_scm_connection_id
		FROM issue_sync_comments isc2
		JOIN issue_sync_items isi ON isi.id = isc2.issue_sync_item_id
		JOIN issue_sync_configs isc ON isc.id = isi.issue_sync_config_id
		JOIN workspace_repositories wr ON wr.id = isc.workspace_repository_id
		WHERE isc2.comment_id = ? AND isc.sync_enabled = ? AND isc.sync_comments = ?
	`, commentID, true, true).Scan(&ghCommentID, &issueNumber, &repoName, &connectionID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("lookup sync comment for update pushback", "comment_id", commentID, "error", err)
		}
		return
	}

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

	if err := issueProvider.UpdateIssueComment(ctx, parts[0], parts[1], issueNumber, ghCommentID, newBody); err != nil {
		slog.Error("push comment update to GitHub", "github_comment_id", ghCommentID, "error", err)
	}
}

// syncComments pulls GitHub issue comments into Windshift.
func (s *IssueSyncService) syncComments(ctx context.Context, provider IssueProvider, owner, repo string, issueNumber, syncItemID, itemID int) {
	// Fetch remotely before opening the transaction.
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

	// All comment-table writes go through CommentService (the single
	// comment-write chokepoint); the tx-aware variants keep the comment and its
	// sync-tracking row atomic. We publish once after commit below.
	commentSvc := services.NewCommentService(s.db)
	commentsChanged := false
	for _, ghComment := range comments {
		if strings.Contains(ghComment.Body, "commented in Windshift:") && strings.HasPrefix(ghComment.Body, "**") {
			continue
		}

		var trackingID int
		var existingCommentID sql.NullInt64
		var lastGHUpdated sql.NullTime

		err := tx.QueryRowContext(ctx,
			"SELECT id, comment_id, github_updated_at FROM issue_sync_comments WHERE issue_sync_item_id = ? AND github_comment_id = ?",
			syncItemID, ghComment.ID,
		).Scan(&trackingID, &existingCommentID, &lastGHUpdated)

		if errors.Is(err, sql.ErrNoRows) {
			body := fmt.Sprintf("**@%s** commented on GitHub:\n\n%s", ghComment.User.Username, ghComment.Body)
			now := time.Now()

			wsCommentID, insertErr := commentSvc.CreateInTx(ctx, tx, itemID, 0, body, now, itemevents.Integration("github", "scm"))
			if insertErr != nil {
				slog.Error("insert synced comment", "github_comment_id", ghComment.ID, "error", insertErr)
				continue
			}

			_, _ = tx.ExecContext(ctx, `
				INSERT INTO issue_sync_comments (issue_sync_item_id, comment_id, github_comment_id, github_updated_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)
			`, syncItemID, wsCommentID, ghComment.ID, ghComment.UpdatedAt, now, now)

			commentsChanged = true
			continue
		}
		if err != nil {
			slog.Error("lookup sync comment", "github_comment_id", ghComment.ID, "error", err)
			continue
		}

		if !existingCommentID.Valid {
			continue // Windshift comment was deleted, skip
		}
		if lastGHUpdated.Valid && !ghComment.UpdatedAt.After(lastGHUpdated.Time) {
			continue // No changes
		}

		body := fmt.Sprintf("**@%s** commented on GitHub:\n\n%s", ghComment.User.Username, ghComment.Body)
		now := time.Now()
		_ = commentSvc.UpdateContentInTx(ctx, tx, int(existingCommentID.Int64), body, now)
		_, _ = tx.ExecContext(ctx,
			"UPDATE issue_sync_comments SET github_updated_at = ?, updated_at = ? WHERE id = ?",
			ghComment.UpdatedAt, now, trackingID)
		commentsChanged = true
	}

	if commentsChanged {
		if err := repository.NewItemRepository(s.db).TouchActivity(tx, itemID, time.Now()); err != nil {
			slog.Error("bump item activity for synced comments", "item_id", itemID, "error", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		slog.Error("commit comment sync tx", "issue_number", issueNumber, "error", err)
		return
	}

	// Live-update publish (WI-483): GitHub-sourced comments committed; refresh
	// the item's comment list for anyone viewing it.
	if commentsChanged {
		services.PublishItemChange(itemID, services.ItemChangeComment)
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
	// Enforce one config per workspace. The workspace-scoped Get/Update/Delete
	// endpoints assume a single config; the schema's UNIQUE is only per repo,
	// so without this guard a workspace could end up with multiple configs and
	// GetSyncConfigForWorkspace's LIMIT 1 would silently pick whichever the DB
	// returned first. Look up the target workspace via the requested repo.
	var existingID int
	err := s.db.QueryRowContext(ctx, `
		SELECT isc.id
		FROM issue_sync_configs isc
		JOIN workspace_repositories wr_existing ON wr_existing.id = isc.workspace_repository_id
		JOIN workspace_scm_connections wsc_existing ON wsc_existing.id = wr_existing.workspace_scm_connection_id
		JOIN workspace_repositories wr_target ON wr_target.id = ?
		JOIN workspace_scm_connections wsc_target ON wsc_target.id = wr_target.workspace_scm_connection_id
		WHERE wsc_existing.workspace_id = wsc_target.workspace_id
		LIMIT 1
	`, req.WorkspaceRepositoryID).Scan(&existingID)
	if err == nil {
		return 0, ErrSyncConfigExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("check existing sync config: %w", err)
	}

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
	err = s.db.QueryRowContext(ctx, `
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
	_, err := s.db.ExecWriteContext(ctx, `
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
	_, err := s.db.ExecWriteContext(ctx, "DELETE FROM issue_sync_configs WHERE id = ?", configID)
	return err
}

// GetSyncedItems returns all synced items for a config.
func (s *IssueSyncService) GetSyncedItems(ctx context.Context, configID int) ([]models.IssueSyncItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT isi.id, isi.issue_sync_config_id, isi.item_id,
			   isi.github_issue_number, isi.github_issue_id, isi.github_issue_url,
			   isi.last_synced_at, isi.last_github_updated_at, isi.sync_lock,
			   isi.created_at, isi.updated_at
		FROM issue_sync_items isi
		WHERE isi.issue_sync_config_id = ?
		ORDER BY isi.github_issue_number
	`, configID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]models.IssueSyncItem, 0)
	itemIDs := make([]int, 0)
	for rows.Next() {
		var item models.IssueSyncItem
		var lastSync, lastGH sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.IssueSyncConfigID, &item.ItemID,
			&item.GitHubIssueNumber, &item.GitHubIssueID, &item.GitHubIssueURL,
			&lastSync, &lastGH, &item.SyncLock,
			&item.CreatedAt, &item.UpdatedAt,
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
		itemIDs = append(itemIDs, item.ItemID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	itemDetails, err := repository.NewItemRepository(s.db).FindByIDsWithDetails(itemIDs)
	if err != nil {
		return nil, fmt.Errorf("load synced item details: %w", err)
	}
	detailsByID := make(map[int]*models.Item, len(itemDetails))
	for _, item := range itemDetails {
		detailsByID[item.ID] = item
	}
	for i := range items {
		if item := detailsByID[items[i].ItemID]; item != nil {
			items[i].ItemTitle = item.Title
			items[i].WorkspaceItemNumber = item.WorkspaceItemNumber
			items[i].WorkspaceKey = item.WorkspaceKey
		}
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
	_, _ = s.db.ExecWriteContext(ctx,
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

func (s *IssueSyncService) syncLabels(ctx context.Context, tx database.Tx, config *models.IssueSyncConfig, issue *Issue, itemID int) error {
	switch config.LabelSyncMode {
	case "", models.IssueSyncLabelNone:
		return nil
	case models.IssueSyncLabelMapped:
		// Use explicit mappings
		var mappings []models.LabelMapping
		if err := json.Unmarshal([]byte(config.LabelMappings), &mappings); err != nil {
			return fmt.Errorf("parse label mappings: %w", err)
		}

		// Build lookup: github label name → windshift label ID
		ghToWS := make(map[string]int)
		for _, m := range mappings {
			ghToWS[m.GitHubLabel] = m.WindshiftLabelID
		}

		labelIDs := make([]int, 0, len(issue.Labels))
		for _, l := range issue.Labels {
			if wsLabelID, ok := ghToWS[l.Name]; ok {
				labelIDs = append(labelIDs, wsLabelID)
			}
		}
		if err := repository.NewLabelRepository(s.db).ReplaceItemLabelsTx(ctx, tx, itemID, labelIDs); err != nil {
			return fmt.Errorf("replace mapped labels: %w", err)
		}
	case models.IssueSyncLabelMirror:
		labelRepo := repository.NewLabelRepository(s.db)
		labelIDs := make([]int, 0, len(issue.Labels))
		for _, l := range issue.Labels {
			color := l.Color
			if color == "" {
				color = "808080"
			}
			labelID, err := labelRepo.EnsureByNameTx(ctx, tx, l.Name, color)
			if err != nil {
				return fmt.Errorf("ensure mirrored label %q: %w", l.Name, err)
			}
			labelIDs = append(labelIDs, labelID)
		}
		if err := labelRepo.ReplaceItemLabelsTx(ctx, tx, itemID, labelIDs); err != nil {
			return fmt.Errorf("replace mirrored labels: %w", err)
		}
	default:
		return fmt.Errorf("unsupported label sync mode %q", config.LabelSyncMode)
	}
	return nil
}

func (s *IssueSyncService) recordSyncError(configID int, errMsg string) {
	_, _ = s.db.ExecWrite(
		"UPDATE issue_sync_configs SET last_sync_error = ?, updated_at = ? WHERE id = ?",
		errMsg, time.Now(), configID)
}

// GetGitHubLabels fetches labels from a GitHub repository for mapping UI.
// workspaceID gates the lookup: the repo must belong to that workspace, otherwise
// ErrRepositoryNotInWorkspace is returned (handlers map this to 404).
func (s *IssueSyncService) GetGitHubLabels(ctx context.Context, workspaceID, workspaceRepoID int) ([]IssueLabel, error) {
	belongs, err := s.VerifyRepositoryInWorkspace(ctx, workspaceRepoID, workspaceID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, ErrRepositoryNotInWorkspace
	}

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
// See GetGitHubLabels for the workspaceID gating contract.
func (s *IssueSyncService) GetGitHubMilestones(ctx context.Context, workspaceID, workspaceRepoID int) ([]IssueMilestone, error) {
	belongs, err := s.VerifyRepositoryInWorkspace(ctx, workspaceRepoID, workspaceID)
	if err != nil {
		return nil, err
	}
	if !belongs {
		return nil, ErrRepositoryNotInWorkspace
	}

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
