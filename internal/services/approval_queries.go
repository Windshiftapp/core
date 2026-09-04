package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// IsTransitionGatedByApproval returns the pending approval request ID if the
// requested (from→to) transition is the configured approve_transition_id or
// deny_transition_id of an in-flight pending approval on this item. Returns
// nil if not gated.
func (s *ApprovalService) IsTransitionGatedByApproval(ctx context.Context, itemID, fromStatusID, toStatusID int) (*int, error) {
	return s.runtimeRepo.FindGatedRequestForTransition(ctx, itemID, fromStatusID, toStatusID)
}

// PendingApprovalSummary is the compact view returned alongside available
// transitions so the picker can render a "Pending approval" affordance and so
// callers can avoid reproducing the active-pool check on the client.
type PendingApprovalSummary struct {
	ID           int    `json:"id"`
	Status       string `json:"status"`
	YouCanDecide bool   `json:"you_can_decide"`
}

// GetGatedTransitionsForItem returns the set of workflow_transition IDs that
// the user may not invoke directly because an in-flight approval owns them
// (its configured approve_transition_id and deny_transition_id), plus a compact
// summary of the pending request. Returns (nil, nil, nil) when no approval is
// pending.
func (s *ApprovalService) GetGatedTransitionsForItem(ctx context.Context, itemID, userID int) ([]int, *PendingApprovalSummary, error) {
	view, err := s.runtimeRepo.FindGatedTransitionsForItem(ctx, itemID, userID)
	if err != nil {
		return nil, nil, err
	}
	if view == nil {
		return nil, nil, nil
	}
	return []int{view.ApproveTransitionID, view.DenyTransitionID}, &PendingApprovalSummary{
		ID:           view.RequestID,
		Status:       view.Status,
		YouCanDecide: view.UserCanDecide,
	}, nil
}

// MaybeOpenForStatusEntry opens a new approval request iff the (workspace,
// item-type, status) tuple resolves to an approval_set_status. If no approval
// is configured for the destination status, returns (nil, nil) — safe to call
// for every transition.
func (s *ApprovalService) MaybeOpenForStatusEntry(ctx context.Context, itemID, statusID, fromStatusID, actorUserID int) (*models.ApprovalRequest, error) {
	item, err := repository.NewItemRepository(s.db).FindByID(itemID)
	if err != nil {
		return nil, err
	}
	ass, err := s.GetApprovalSetStatusForItem(ctx, item.WorkspaceID, item.ItemTypeID, statusID)
	if err != nil {
		return nil, err
	}
	if ass == nil {
		return nil, nil
	}
	return s.RequestApproval(ctx, itemID, statusID, fromStatusID, actorUserID)
}

// ----------------------------------------------------------------------------
// Listing helpers
// ----------------------------------------------------------------------------

// GetPendingForItem returns the single pending approval request for an item, or nil.
func (s *ApprovalService) GetPendingForItem(ctx context.Context, itemID int) (*models.ApprovalRequest, error) {
	id, err := s.runtimeRepo.FindPendingRequestIDForItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	if id == nil {
		return nil, nil
	}
	return s.GetRequest(ctx, *id)
}

// GetTimelineForItem returns all approval requests for an item, ordered by created_at.
func (s *ApprovalService) GetTimelineForItem(ctx context.Context, itemID int) ([]*models.ApprovalRequest, error) {
	ids, err := s.runtimeRepo.FindRequestIDsForItem(ctx, itemID)
	if err != nil {
		return nil, err
	}
	return s.loadRequests(ctx, ids)
}

// GetTimelineForItemPage hydrates only a repository-bounded timeline page.
func (s *ApprovalService) GetTimelineForItemPage(ctx context.Context, itemID, limit, offset int) ([]*models.ApprovalRequest, int, error) {
	ids, total, err := s.runtimeRepo.FindRequestIDsForItemPage(ctx, itemID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.loadRequests(ctx, ids)
	return items, total, err
}

// GetDecisionCommentsForItem returns a cursor-filtered, bounded page of
// comment-bearing approval decisions shaped like feed comment rows.
func (s *ApprovalService) GetDecisionCommentsForItem(itemID int, includeAgentOwner bool, options CommentFeedOptions) ([]models.Comment, error) {
	query := `
		SELECT -d.id, ar.item_id, d.actor_user_id, d.comment, d.created_at,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), 'Unknown User') AS author_name,
		       u.email, u.avatar_url,
		       COALESCE(u.is_agent, FALSE) AS is_agent,
		       COALESCE(NULLIF(TRIM(COALESCE(owner.first_name, '') || ' ' || COALESCE(owner.last_name, '')), ''), owner.username, '') AS agent_owner_name
		FROM approval_decisions d
		JOIN approval_requests ar ON ar.id = d.approval_request_id
		LEFT JOIN users u ON u.id = d.actor_user_id
		LEFT JOIN users owner ON owner.id = u.agent_owner_user_id
		WHERE ar.item_id = ? AND d.comment IS NOT NULL AND d.comment <> ''
	`
	args := []any{itemID}
	order := "DESC"
	switch {
	case options.Before != nil:
		query += ` AND (d.created_at < ? OR (d.created_at = ? AND -d.id < ?))`
		args = append(args, options.Before.CreatedAt, options.Before.CreatedAt, options.Before.ID)
	case options.Since != nil:
		query += ` AND (d.created_at > ? OR (d.created_at = ? AND -d.id > ?))`
		args = append(args, options.Since.CreatedAt, options.Since.CreatedAt, options.Since.ID)
		order = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY d.created_at %s, -d.id %s LIMIT ?", order, order)
	args = append(args, normalizeCommentFeedLimit(options.Limit)+1)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get approval decision comments for item %d: %w", itemID, err)
	}
	defer func() { _ = rows.Close() }()

	var comments []models.Comment
	for rows.Next() {
		var c models.Comment
		var authorID sql.NullInt64
		var authorName, authorEmail, authorAvatar, agentOwnerName sql.NullString
		if err := rows.Scan(
			&c.ID, &c.ItemID, &authorID, &c.Content, &c.CreatedAt,
			&authorName, &authorEmail, &authorAvatar,
			&c.IsAgent, &agentOwnerName,
		); err != nil {
			return nil, fmt.Errorf("scan approval decision comment for item %d: %w", itemID, err)
		}
		if authorID.Valid {
			id := int(authorID.Int64)
			c.AuthorID = &id
		}
		c.UpdatedAt = c.CreatedAt
		c.Source = "approval"
		c.AuthorName = authorName.String
		c.AuthorEmail = authorEmail.String
		c.AuthorAvatar = authorAvatar.String
		if includeAgentOwner {
			c.AgentOwnerName = agentOwnerName.String
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approval decision comments for item %d: %w", itemID, err)
	}
	return comments, nil
}

// CountDecisionCommentsForItem returns the number of approval decisions with
// comments that participate in the merged item comment feed.
func (s *ApprovalService) CountDecisionCommentsForItem(itemID int) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM approval_decisions d
		JOIN approval_requests ar ON ar.id = d.approval_request_id
		WHERE ar.item_id = ? AND d.comment IS NOT NULL AND d.comment <> ''
	`, itemID).Scan(&count)
	return count, err
}

// GetForUser returns approval requests where the user is in the active approver
// pool of any pending step. status filters request status (empty = "pending").
func (s *ApprovalService) GetForUser(ctx context.Context, userID int, status string) ([]*models.ApprovalRequest, error) {
	return s.getForActor(ctx, "user_id", userID, status, nil)
}

// GetForUserPage hydrates only the requested repository-bounded approver page.
func (s *ApprovalService) GetForUserPage(ctx context.Context, userID int, status string, limit, offset int) ([]*models.ApprovalRequest, int, error) {
	ids, total, err := s.runtimeRepo.FindRequestIDsForActorPage(ctx, "user_id", userID, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.loadRequests(ctx, ids)
	return items, total, err
}

// GetForUserInChannel restricts the user's active approval pool to items that
// belong to channelID. Portal callers must use this scoped variant.
func (s *ApprovalService) GetForUserInChannel(ctx context.Context, userID int, status string, channelID int) ([]*models.ApprovalRequest, error) {
	return s.getForActor(ctx, "user_id", userID, status, &channelID)
}

// UserHasActivePoolMembershipOnItem returns true iff the user is in an active
// approver row of a step that is currently active on a pending approval request
// for itemID. This is the gate used by approver-derived item-view access:
// when the step closes (is_active flipped to FALSE) or the request is no longer
// pending, the user immediately loses approver-derived access.
//
// channelID is optional. Non-nil restricts the lookup to items in that channel,
// preventing approver-derived access from leaking across portal channels.
// Internal (non-portal) callers pass nil.
func (s *ApprovalService) UserHasActivePoolMembershipOnItem(ctx context.Context, userID, itemID int, channelID *int) (bool, error) {
	return s.runtimeRepo.ActorHasActivePoolMembershipOnItem(ctx, "user_id", userID, itemID, channelID)
}

// PortalCustomerHasActivePoolMembershipOnItem is the portal-customer counterpart
// to UserHasActivePoolMembershipOnItem. channelID semantics match.
func (s *ApprovalService) PortalCustomerHasActivePoolMembershipOnItem(ctx context.Context, customerID, itemID int, channelID *int) (bool, error) {
	return s.runtimeRepo.ActorHasActivePoolMembershipOnItem(ctx, "portal_customer_id", customerID, itemID, channelID)
}

// GetForPortalCustomer is the customer-flavored counterpart to GetForUser.
// Returns approval requests where the portal customer is in the active pool.
func (s *ApprovalService) GetForPortalCustomer(ctx context.Context, customerID int, status string) ([]*models.ApprovalRequest, error) {
	return s.getForActor(ctx, "portal_customer_id", customerID, status, nil)
}

// GetForPortalCustomerInChannel restricts the customer's active approval pool
// to items that belong to channelID. Portal callers must use this scoped variant.
func (s *ApprovalService) GetForPortalCustomerInChannel(ctx context.Context, customerID int, status string, channelID int) ([]*models.ApprovalRequest, error) {
	return s.getForActor(ctx, "portal_customer_id", customerID, status, &channelID)
}

func (s *ApprovalService) getForActor(ctx context.Context, actorColumn string, actorID int, status string, channelID *int) ([]*models.ApprovalRequest, error) {
	var (
		ids []int
		err error
	)
	if channelID != nil {
		ids, err = s.runtimeRepo.FindRequestIDsForActorInChannel(ctx, actorColumn, actorID, status, *channelID)
	} else {
		ids, err = s.runtimeRepo.FindRequestIDsForActor(ctx, actorColumn, actorID, status)
	}
	if err != nil {
		return nil, err
	}
	if channelID != nil {
		return s.loadRequestsInChannel(ctx, ids, *channelID)
	}
	return s.loadRequests(ctx, ids)
}

func (s *ApprovalService) loadRequests(ctx context.Context, ids []int) ([]*models.ApprovalRequest, error) {
	out := make([]*models.ApprovalRequest, 0, len(ids))
	for _, id := range ids {
		req, err := s.GetRequest(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, nil
}

func (s *ApprovalService) loadRequestsInChannel(ctx context.Context, ids []int, channelID int) ([]*models.ApprovalRequest, error) {
	out := make([]*models.ApprovalRequest, 0, len(ids))
	for _, id := range ids {
		req, err := s.GetRequestInChannel(ctx, id, channelID)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, nil
}

// GetRequest loads a request with its step instances, approvers, and decisions.
func (s *ApprovalService) GetRequest(ctx context.Context, requestID int) (*models.ApprovalRequest, error) {
	req, err := s.runtimeRepo.FindFullRequestByID(ctx, requestID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, sql.ErrNoRows
	}
	return req, err
}

// GetRequestInChannel loads a request only if its item belongs to channelID.
// This is the portal-safe detail lookup; a mismatch is indistinguishable from
// a missing approval.
func (s *ApprovalService) GetRequestInChannel(ctx context.Context, requestID, channelID int) (*models.ApprovalRequest, error) {
	req, err := s.runtimeRepo.FindFullRequestByIDInChannel(ctx, requestID, channelID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, sql.ErrNoRows
	}
	return req, err
}

// GetItemIDForRequest returns the item id behind an approval request, or
// sql.ErrNoRows if none. Pass-through to ApprovalRepository so handlers don't
// need a repo reference of their own.
func (s *ApprovalService) GetItemIDForRequest(ctx context.Context, requestID int) (int, error) {
	id, err := s.runtimeRepo.GetItemIDForRequest(ctx, requestID)
	if errors.Is(err, repository.ErrNotFound) {
		return 0, sql.ErrNoRows
	}
	return id, err
}

// StepInstanceBelongsToRequest reports whether a step instance belongs to the
// given approval request. Pass-through to ApprovalRepository.
func (s *ApprovalService) StepInstanceBelongsToRequest(ctx context.Context, stepInstanceID, requestID int) (bool, error) {
	return s.runtimeRepo.StepInstanceBelongsToRequest(ctx, stepInstanceID, requestID)
}

// CountPendingApproversForRole returns the number of pending approval-step
// approver rows that resolved through this workspace role. Used by the
// workspace-role delete path to refuse the request when it would orphan an
// in-flight pool.
func (s *ApprovalService) CountPendingApproversForRole(ctx context.Context, roleID int) (int, error) {
	return s.runtimeRepo.CountPendingApproversForRole(ctx, roleID)
}

// ----------------------------------------------------------------------------
// Internals
// ----------------------------------------------------------------------------
