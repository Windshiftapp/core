package scm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/services"
)

const prReviewFirstSightWindow = 10 * time.Minute

type prReviewEvent struct {
	ID           int64
	ItemID       int
	Kind         string
	ExternalID   int64
	AuthorID     string
	Author       string
	Association  string
	Body         string
	ContextJSON  string
	Status       string
	RunID        int
	AckID        int64
	Attempts     int
	TerminalBody string
}

func normalizeEventKind(kind string) string {
	switch kind {
	case "review", "review_comment":
		return kind
	default:
		return "issue_comment"
	}
}

// listPRReviewEvents reads every supported review surface. The durable unique
// key makes repeated full listings harmless and lets polling reconcile webhook
// deliveries without a second idempotency mechanism.
func listPRReviewEvents(ctx context.Context, provider Provider, owner, repo string, prNumber int) ([]IssueComment, error) {
	issues, ok := provider.(IssueCommentProvider)
	if !ok {
		return nil, nil
	}
	events, err := issues.ListIssueComments(ctx, owner, repo, prNumber)
	if err != nil {
		return nil, err
	}
	if reviews, ok := provider.(PullRequestReviewProvider); ok {
		reviewEvents, reviewErr := reviews.ListPullRequestReviewEvents(ctx, owner, repo, prNumber)
		if reviewErr != nil {
			slog.Warn("PR review poll: review surfaces unavailable; continuing with conversation comments", slog.Int("pr", prNumber), slog.Any("error", reviewErr))
		} else {
			events = append(events, reviewEvents...)
		}
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].CreatedAt.Equal(events[j].CreatedAt) {
			return events[i].ID < events[j].ID
		}
		return events[i].CreatedAt.Before(events[j].CreatedAt)
	})
	return events, nil
}

func (s *SyncService) ownedItemForPR(ctx context.Context, repoID, prNumber, fallback int) int {
	var itemID int
	err := s.db.QueryRowContext(ctx, `SELECT item_id FROM agent_pr_ownerships WHERE workspace_repository_id=? AND pr_number=?`, repoID, prNumber).Scan(&itemID)
	if err == nil && itemID > 0 {
		return itemID
	}
	return fallback
}

func (s *SyncService) authorizePRReviewEvent(ctx context.Context, provider Provider, repoID, workspaceID int, owner, repo string, event IssueComment) (authorized bool, reason string, err error) {
	association := strings.ToUpper(strings.TrimSpace(event.AuthorAssociation))
	switch association {
	case "OWNER", "MEMBER", "COLLABORATOR":
		return true, "trusted author association", nil
	}
	// Provider fixtures and legacy providers may omit user data. Real GitHub and
	// Gitea payloads always carry it; retaining this narrow compatibility path
	// keeps old installations/test doubles functional without trusting a named
	// external identity.
	if event.User.ID == "" && event.User.Username == "" {
		if _, productionProvider := provider.(RepositoryPermissionProvider); productionProvider {
			return false, "provider omitted author identity", nil
		}
		return true, "legacy provider without author identity", nil
	}
	// A mapped SCM identity that is a member of this workspace is trusted even
	// when the provider omits author_association (notably Gitea versions).
	var mapped int
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM workspace_repositories wr
		JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
		JOIN user_scm_oauth_tokens tok ON tok.scm_provider_id = wsc.scm_provider_id
		JOIN user_workspace_roles uwr ON uwr.user_id = tok.user_id AND uwr.workspace_id = ?
		WHERE wr.id = ? AND (tok.scm_user_id = ? OR lower(tok.scm_username) = lower(?))
	`, workspaceID, repoID, event.User.ID, event.User.Username).Scan(&mapped)
	if err == nil && mapped > 0 {
		return true, "mapped workspace member", nil
	}
	permissions, ok := provider.(RepositoryPermissionProvider)
	if !ok || strings.TrimSpace(event.User.Username) == "" {
		return false, "provider cannot establish repository permission", nil
	}
	allowed, err := permissions.CanUserWriteRepository(ctx, owner, repo, event.User.Username)
	if err != nil {
		return false, "repository permission check failed", err
	}
	if !allowed {
		return false, "commenter lacks repository triage/write permission", nil
	}
	return true, "repository collaborator permission", nil
}

func reviewContextJSON(event IssueComment) string {
	contextJSON, _ := json.Marshal(map[string]any{
		"path": event.Path, "line": event.Line, "side": event.Side, "thread_id": event.ThreadID,
	})
	return string(contextJSON)
}

func (s *SyncService) insertPRReviewEvent(ctx context.Context, repoID, workspaceID, itemID, prNumber int, event IssueComment) (int64, error) {
	kind := normalizeEventKind(event.Kind)
	_, err := s.db.ExecWriteContext(ctx, `
		INSERT INTO agent_pr_review_events(
			workspace_repository_id, workspace_id, item_id, pr_number, event_kind, external_id,
			author_id, author_login, author_association, body, context_json, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')
		ON CONFLICT(workspace_repository_id, pr_number, event_kind, external_id) DO NOTHING
	`, repoID, workspaceID, itemID, prNumber, kind, event.ID, event.User.ID, event.User.Username,
		event.AuthorAssociation, event.Body, reviewContextJSON(event))
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM agent_pr_review_events
		WHERE workspace_repository_id = ? AND pr_number = ? AND event_kind = ? AND external_id = ?
	`, repoID, prNumber, kind, event.ID).Scan(&id)
	return id, err
}

func (s *SyncService) hasReviewKindLedger(ctx context.Context, repoID, prNumber int, kind string) bool {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_pr_review_events WHERE workspace_repository_id=? AND pr_number=? AND event_kind=?`, repoID, prNumber, kind).Scan(&count)
	return err == nil && count > 0
}

// IngestPRReviewEvent is the common webhook/poller admission seam. Provider
// webhook adapters can call it directly; periodic polling calls the same path
// as reconciliation, and the ledger uniqueness key prevents double admission.
func (s *SyncService) IngestPRReviewEvent(ctx context.Context, provider Provider, owner, repo string, pr PullRequest, repoID, workspaceID, itemID int, event IssueComment) (int64, error) {
	if strings.Contains(event.Body, models.AgentCommentMarker) || !prCommentTriggerRE.MatchString(event.Body) {
		return 0, nil
	}
	eventID, err := s.insertPRReviewEvent(ctx, repoID, workspaceID, itemID, pr.Number, event)
	if err != nil {
		return 0, err
	}
	if s.continuationStarter != nil {
		s.processPRReviewInbox(ctx, provider, owner, repo, pr, repoID, workspaceID)
	}
	return eventID, nil
}

func (s *SyncService) setReviewEventStatus(ctx context.Context, eventID int64, status, lastError string, runID int) {
	var run any
	if runID > 0 {
		run = runID
	}
	if _, err := s.db.ExecWriteContext(ctx, `
		UPDATE agent_pr_review_events
		SET status = ?, last_error = ?, agent_run_id = COALESCE(?, agent_run_id),
			attempts = attempts + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, lastError, run, eventID); err != nil {
		slog.Warn("PR review event: status update failed", slog.Int64("event", eventID), slog.Any("error", err))
	}
}

func (s *SyncService) pendingPRReviewEvents(ctx context.Context, repoID, prNumber int) ([]prReviewEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, item_id, event_kind, external_id, COALESCE(author_id,''), COALESCE(author_login,''),
			COALESCE(author_association,''), body, COALESCE(CAST(context_json AS TEXT),''), status, COALESCE(agent_run_id,0),
			COALESCE(ack_comment_id,0), attempts, COALESCE(terminal_body,'')
		FROM agent_pr_review_events
		WHERE workspace_repository_id = ? AND pr_number = ?
		  AND status IN ('pending','running','reply_pending')
		ORDER BY id
	`, repoID, prNumber)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var events []prReviewEvent
	for rows.Next() {
		var event prReviewEvent
		if err := rows.Scan(&event.ID, &event.ItemID, &event.Kind, &event.ExternalID, &event.AuthorID,
			&event.Author, &event.Association, &event.Body, &event.ContextJSON, &event.Status,
			&event.RunID, &event.AckID, &event.Attempts, &event.TerminalBody); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func reviewInstruction(event prReviewEvent) string {
	if event.Kind == "issue_comment" || strings.TrimSpace(event.ContextJSON) == "" {
		return event.Body
	}
	var reviewContext struct {
		Path     string `json:"path"`
		Line     int    `json:"line"`
		Side     string `json:"side"`
		ThreadID string `json:"thread_id"`
	}
	if json.Unmarshal([]byte(event.ContextJSON), &reviewContext) != nil {
		return event.Body
	}
	location := reviewContext.Path
	if reviewContext.Line > 0 {
		location += fmt.Sprintf(":%d", reviewContext.Line)
	}
	return fmt.Sprintf("PR %s request at %s (side=%s, thread=%s):\n\n%s", event.Kind, location, reviewContext.Side, reviewContext.ThreadID, event.Body)
}

func eventMarker(eventID int64, phase string) string {
	return fmt.Sprintf("<!-- windshift-agent event:%d phase:%s -->", eventID, phase)
}

func (s *SyncService) postEventCommentOnce(ctx context.Context, issues IssueCommentProvider, owner, repo string, prNumber int, eventID int64, phase, body string) (int64, error) {
	marker := eventMarker(eventID, phase)
	comments, err := issues.ListIssueComments(ctx, owner, repo, prNumber)
	if err == nil {
		for _, comment := range comments {
			if strings.Contains(comment.Body, marker) {
				return comment.ID, nil
			}
		}
	}
	return issues.CreateIssueComment(ctx, owner, repo, prNumber, marker+"\n"+models.AgentCommentMarker+"\n\n"+body)
}

func (s *SyncService) ensureEventReply(ctx context.Context, issues IssueCommentProvider, owner, repo string, prNumber int, event prReviewEvent, phase, body string) {
	id, err := s.postEventCommentOnce(ctx, issues, owner, repo, prNumber, event.ID, phase, body)
	if err != nil {
		s.setReviewEventStatus(ctx, event.ID, event.Status, "post "+phase+" reply: "+err.Error(), event.RunID)
		return
	}
	column := "ack_comment_id"
	if phase == "terminal" {
		column = "terminal_comment_id"
	}
	query := fmt.Sprintf("UPDATE agent_pr_review_events SET %s = ?, status = CASE WHEN ? = 'terminal' THEN 'replied' ELSE status END, last_error = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?", column)
	if _, err := s.db.ExecWriteContext(ctx, query, id, phase, event.ID); err != nil {
		slog.Warn("PR review event: persist reply failed", slog.Int64("event", event.ID), slog.Any("error", err))
	}
}

func (s *SyncService) processPRReviewInbox(ctx context.Context, provider Provider, owner, repo string, pr PullRequest, repoID, workspaceID int) {
	issues, ok := provider.(IssueCommentProvider)
	if !ok {
		return
	}
	events, err := s.pendingPRReviewEvents(ctx, repoID, pr.Number)
	if err != nil {
		slog.Warn("PR review event: load inbox failed", slog.Int("pr", pr.Number), slog.Any("error", err))
		return
	}
	headBranch := s.resolveHeadBranch(ctx, provider, owner, repo, pr)
	detailedStarter, supportsReplies := s.continuationStarter.(PRCommentContinuationDetailedStarter)
	for _, event := range events {
		switch event.Status {
		case "running":
			if supportsReplies && event.AckID == 0 {
				s.ensureEventReply(ctx, issues, owner, repo, pr.Number, event, "ack", fmt.Sprintf("Windshift queued coding-agent run %d for this review request.", event.RunID))
			}
			continue
		case "reply_pending":
			s.ensureEventReply(ctx, issues, owner, repo, pr.Number, event, "terminal", event.TerminalBody)
			continue
		}
		allowed, reason, authErr := s.authorizePRReviewEvent(ctx, provider, repoID, workspaceID, owner, repo, IssueComment{
			ID: event.ExternalID, Kind: event.Kind, Body: event.Body,
			User: User{ID: event.AuthorID, Username: event.Author}, AuthorAssociation: event.Association,
		})
		if authErr != nil {
			if event.Attempts >= 2 {
				body := "Windshift could not verify that this reviewer is authorized to run the coding agent. No run was started."
				_, _ = s.db.ExecWriteContext(ctx, `UPDATE agent_pr_review_events SET status='reply_pending', terminal_body=?, last_error=?, attempts=attempts+1, updated_at=CURRENT_TIMESTAMP WHERE id=?`, body, authErr.Error(), event.ID)
				event.Status, event.TerminalBody = "reply_pending", body
				s.ensureEventReply(ctx, issues, owner, repo, pr.Number, event, "terminal", body)
			} else {
				s.setReviewEventStatus(ctx, event.ID, "pending", "authorize reviewer: "+authErr.Error(), 0)
			}
			continue
		}
		if !allowed {
			s.setReviewEventStatus(ctx, event.ID, "ignored", reason, 0)
			continue
		}
		if headBranch == "" {
			body := "The coding agent could not continue this PR because its head branch is unavailable."
			_, _ = s.db.ExecWriteContext(ctx, `UPDATE agent_pr_review_events SET status='reply_pending', terminal_body=?, last_error=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, body, "PR head branch unavailable", event.ID)
			event.Status, event.TerminalBody = "reply_pending", body
			s.ensureEventReply(ctx, issues, owner, repo, pr.Number, event, "terminal", body)
			continue
		}
		input := services.PRCommentContinuation{WorkspaceID: workspaceID, ItemID: event.ItemID, RepoSlug: owner + "/" + repo,
			PRNumber: pr.Number, HeadBranch: headBranch, HeadRepo: pr.HeadRepo, CommentID: event.ExternalID,
			EventID: event.ID, CommentKind: event.Kind, CommentBody: reviewInstruction(event)}
		result := services.PRCommentStartResult{}
		if supportsReplies {
			result, err = detailedStarter.StartPRCommentContinuationDetailed(ctx, input)
		} else {
			result.Started, err = s.continuationStarter.StartPRCommentContinuation(ctx, input)
		}
		if err != nil {
			if event.Attempts >= 2 {
				body := "The coding agent could not start this review request after several attempts. Please check the runner and source-control connection before retrying."
				_, _ = s.db.ExecWriteContext(ctx, `UPDATE agent_pr_review_events SET status='reply_pending', terminal_body=?, last_error=?, attempts=attempts+1, updated_at=CURRENT_TIMESTAMP WHERE id=?`, body, err.Error(), event.ID)
				event.Status, event.TerminalBody = "reply_pending", body
				s.ensureEventReply(ctx, issues, owner, repo, pr.Number, event, "terminal", body)
			} else {
				s.setReviewEventStatus(ctx, event.ID, "pending", err.Error(), 0)
			}
			continue
		}
		if !result.Started {
			if result.Terminal {
				body := result.Reason
				if strings.TrimSpace(body) == "" {
					body = "The coding agent could not accept this review request."
				}
				_, _ = s.db.ExecWriteContext(ctx, `UPDATE agent_pr_review_events SET status='reply_pending', terminal_body=?, last_error=NULL, updated_at=CURRENT_TIMESTAMP WHERE id=?`, body, event.ID)
				event.Status, event.TerminalBody = "reply_pending", body
				s.ensureEventReply(ctx, issues, owner, repo, pr.Number, event, "terminal", body)
			} else if supportsReplies && strings.TrimSpace(result.Reason) != "" && event.AckID == 0 {
				s.ensureEventReply(ctx, issues, owner, repo, pr.Number, event, "ack", result.Reason)
			}
			continue
		}
		s.setReviewEventStatus(ctx, event.ID, "running", "", result.RunID)
		event.RunID = result.RunID
		if supportsReplies {
			s.ensureEventReply(ctx, issues, owner, repo, pr.Number, event, "ack", fmt.Sprintf("Windshift queued coding-agent run %d for this review request.", result.RunID))
		}
	}
}

func isRecentFirstSight(event IssueComment, now time.Time) bool {
	return !event.CreatedAt.IsZero() && !event.CreatedAt.Before(now.Add(-prReviewFirstSightWindow))
}

func isNoRows(err error) bool { //nolint:unused // retained for the in-progress review-event persistence path
	return errors.Is(err, sql.ErrNoRows)
}
