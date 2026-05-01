package scm

import (
	"context"
	"log/slog"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/services"
)

// processSmartCommitsForPR applies #comment / #<transition-slug> actions found
// in the PR body and per-commit messages, once per (PR, commit SHA). Idempotent
// across re-syncs: the PR body is gated by smart_commits_applied_at on the link
// row, and each commit SHA is recorded in scm_processed_commits.
func (s *SyncService) processSmartCommitsForPR(
	ctx context.Context,
	provider Provider,
	owner, repo string,
	pr PullRequest,
	repoID, workspaceID int,
	workspaceKey string,
) {
	if !s.smartCommitsEnabled() {
		return
	}
	if !s.smartCommitsEnabledForRepo(ctx, repoID) {
		return
	}

	// PR body: once per PR, guarded by smart_commits_applied_at on any link.
	if !s.prBodyAlreadyApplied(ctx, repoID, pr.Number) {
		for _, action := range s.detector.ParseSmartCommitActions(pr.Body, workspaceKey) {
			s.applySmartCommitAction(ctx, workspaceID, action, pr.Author.Email)
		}
		s.markPRBodyApplied(ctx, repoID, pr.Number)
	}

	// Commit messages: once per (SHA, repo), guarded by scm_processed_commits.
	commits, err := provider.ListPullRequestCommits(ctx, owner, repo, pr.Number)
	if err != nil {
		slog.Warn("smart commits: failed to list PR commits",
			slog.String("component", "scm"),
			slog.Int("repo_id", repoID), slog.Int("pr", pr.Number),
			slog.Any("error", err))
		return
	}

	for _, commit := range commits {
		if s.commitAlreadyProcessed(ctx, repoID, commit.SHA) {
			continue
		}
		applied := 0
		for _, action := range s.detector.ParseSmartCommitActions(commit.Message, workspaceKey) {
			if s.applySmartCommitAction(ctx, workspaceID, action, commit.Committer.Email) {
				applied++
			}
		}
		s.markCommitProcessed(ctx, repoID, commit.SHA, applied)
	}
}

func (s *SyncService) smartCommitsEnabled() bool {
	return s.workflowService != nil && s.commentService != nil &&
		s.permissionService != nil && s.conditionService != nil && s.itemRepo != nil
}

// smartCommitsEnabledForRepo resolves the workspace-level opt-in flag from the
// repository's connection. Default off; a workspace admin must explicitly
// enable the feature, having acknowledged that the feature trusts the raw git
// committer email on merged commits.
func (s *SyncService) smartCommitsEnabledForRepo(ctx context.Context, repoID int) bool {
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT wsc.smart_commits_enabled
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		WHERE wr.id = ?
	`, repoID).Scan(&enabled)
	return err == nil && enabled
}

func (s *SyncService) prBodyAlreadyApplied(ctx context.Context, repoID, prNumber int) bool {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM item_scm_links
		WHERE workspace_repository_id = ? AND link_type = 'pull_request'
		  AND external_id = ? AND smart_commits_applied_at IS NOT NULL
	`, repoID, strconv.Itoa(prNumber)).Scan(&count)
	return err == nil && count > 0
}

func (s *SyncService) markPRBodyApplied(ctx context.Context, repoID, prNumber int) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE item_scm_links SET smart_commits_applied_at = CURRENT_TIMESTAMP
		WHERE workspace_repository_id = ? AND link_type = 'pull_request'
		  AND external_id = ?
	`, repoID, strconv.Itoa(prNumber))
	if err != nil {
		slog.Warn("smart commits: failed to mark PR body applied",
			slog.String("component", "scm"), slog.Int("repo_id", repoID),
			slog.Int("pr", prNumber), slog.Any("error", err))
	}
}

func (s *SyncService) commitAlreadyProcessed(ctx context.Context, repoID int, sha string) bool {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM scm_processed_commits
		WHERE commit_sha = ? AND workspace_repository_id = ?
	`, sha, repoID).Scan(&count)
	return err == nil && count > 0
}

func (s *SyncService) markCommitProcessed(ctx context.Context, repoID int, sha string, applied int) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO scm_processed_commits (commit_sha, workspace_repository_id, actions_applied)
		VALUES (?, ?, ?)
	`, sha, repoID, applied)
	if err != nil {
		slog.Warn("smart commits: failed to record processed commit",
			slog.String("component", "scm"), slog.String("sha", sha),
			slog.Int("repo_id", repoID), slog.Any("error", err))
	}
}

// applySmartCommitAction applies a single parsed action. Returns true on a
// successful apply. Silently skips when the item is absent, the committer's
// email does not resolve to a workspace member with item-edit permission, or
// the action itself fails (errors are logged).
func (s *SyncService) applySmartCommitAction(
	ctx context.Context,
	workspaceID int,
	action SmartCommitAction,
	committerEmail string,
) bool {
	itemID, err := s.findItemByKey(ctx, workspaceID, action.Key.Prefix, action.Key.Number)
	if err != nil || itemID == 0 {
		return false
	}

	userID, ok := s.resolveActingUser(workspaceID, committerEmail)
	if !ok {
		slog.Info("smart commit skipped: committer email has no workspace member with edit permission",
			slog.String("component", "scm"),
			slog.String("email", committerEmail),
			slog.String("key", action.Key.Key),
			slog.String("command", action.Command))
		return false
	}

	switch action.Command {
	case "comment":
		if action.Payload == "" {
			return false
		}
		_, err := s.commentService.Create(services.CreateCommentParams{
			ItemID:      itemID,
			AuthorID:    userID,
			ActorUserID: userID,
			Content:     action.Payload,
		})
		if err != nil {
			slog.Warn("smart commit: failed to create comment",
				slog.String("component", "scm"), slog.Int("item_id", itemID),
				slog.Any("error", err))
			return false
		}
		return true

	default:
		return s.applyTransitionSlug(ctx, itemID, workspaceID, userID, action.Command)
	}
}

func (s *SyncService) applyTransitionSlug(
	ctx context.Context, itemID, workspaceID, userID int, slug string,
) bool {
	item, err := s.itemRepo.FindByID(itemID)
	if err != nil || item == nil || item.StatusID == nil {
		return false
	}

	toStatusID, found, err := s.workflowService.GetTransitionByName(
		workspaceID, item.ItemTypeID, int64(*item.StatusID), slug,
	)
	if err != nil {
		slog.Warn("smart commit: transition lookup failed",
			slog.String("component", "scm"), slog.Int("item_id", itemID),
			slog.String("slug", slug), slog.Any("error", err))
		return false
	}
	if !found {
		slog.Info("smart commit: no reachable transition",
			slog.String("component", "scm"), slog.Int("item_id", itemID),
			slog.String("slug", slug))
		return false
	}

	_, err = s.workflowService.PerformTransition(
		ctx,
		services.PerformTransitionRequest{
			ItemID:      itemID,
			ToStatusID:  int(toStatusID),
			ActorUserID: userID,
			Modes:       []string{"validator", "condition"},
		},
		s.itemRepo,
		s.conditionService,
		s.approvalService,
	)
	if err != nil {
		slog.Info("smart commit: transition rejected",
			slog.String("component", "scm"), slog.Int("item_id", itemID),
			slog.String("slug", slug), slog.Any("error", err))
		return false
	}
	return true
}

// resolveActingUser maps a committer email to an internal user ID, but only
// when that user has item-edit permission in the workspace. This mirrors
// Jira's smart-commit rule that the committer email must match a user with
// the permission to perform the action.
func (s *SyncService) resolveActingUser(workspaceID int, email string) (int, bool) {
	if email == "" {
		return 0, false
	}
	var userID int
	err := s.db.QueryRow(
		`SELECT id FROM users WHERE LOWER(email) = LOWER(?) LIMIT 1`,
		email,
	).Scan(&userID)
	if err != nil || userID == 0 {
		return 0, false
	}
	ok, err := s.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionItemEdit)
	if err != nil || !ok {
		return 0, false
	}
	return userID, true
}
