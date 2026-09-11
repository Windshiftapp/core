package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ApprovalRepository owns SQL for the approval runtime tables:
// approval_requests, approval_step_instances, approval_step_approvers,
// approval_decisions. Mirrors ChannelRepository: reads on db (or tx for
// in-transaction reads), writes on tx.
type ApprovalRepository struct {
	db database.Database
}

// NewApprovalRepository constructs an ApprovalRepository.
func NewApprovalRepository(db database.Database) *ApprovalRepository {
	return &ApprovalRepository{db: db}
}

// ApproverInsert is the data needed to insert one approval_step_approvers row.
// Exactly one of UserID / PortalCustomerID must be > 0 — the schema CHECK
// constraint enforces this.
type ApproverInsert struct {
	UserID               int // 0 if customer
	PortalCustomerID     int // 0 if internal user
	SourceRoleID         *int
	SourceGroupID        *int
	SubstitutedForUserID *int
}

// ============================================================================
// Read methods — outside transaction
// ============================================================================

// FindFullRequestByID loads a request with its step instances, approvers,
// and decisions. Returns ErrNotFound if no row matches.
func (r *ApprovalRepository) FindFullRequestByID(ctx context.Context, requestID int) (*models.ApprovalRequest, error) {
	return r.findFullRequestByID(ctx, requestID, nil)
}

// FindFullRequestByIDInChannel loads a request only when its item belongs to
// channelID. Portal routes use this instead of loading globally and checking
// the channel after hydration.
func (r *ApprovalRepository) FindFullRequestByIDInChannel(ctx context.Context, requestID, channelID int) (*models.ApprovalRequest, error) {
	return r.findFullRequestByID(ctx, requestID, &channelID)
}

func (r *ApprovalRepository) findFullRequestByID(ctx context.Context, requestID int, channelID *int) (*models.ApprovalRequest, error) {
	req, err := r.findRequestRowInChannel(ctx, requestID, channelID)
	if err != nil {
		return nil, err
	}

	stepRows, err := r.db.QueryContext(ctx, `
		SELECT id, approval_request_id, approval_step_id, display_order, status,
		       escalation_due_at, escalation_count, last_escalated_at, started_at, completed_at
		FROM approval_step_instances WHERE approval_request_id = ? ORDER BY display_order
	`, requestID)
	if err != nil {
		return nil, fmt.Errorf("query step instances: %w", err)
	}
	defer func() { _ = stepRows.Close() }()

	for stepRows.Next() {
		var si models.ApprovalStepInstance
		if err := stepRows.Scan(
			&si.ID, &si.ApprovalRequestID, &si.ApprovalStepID, &si.DisplayOrder, &si.Status,
			&si.EscalationDueAt, &si.EscalationCount, &si.LastEscalatedAt, &si.StartedAt, &si.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan step instance: %w", err)
		}

		appRows, err := r.db.QueryContext(ctx, `
			SELECT id, approval_step_instance_id, user_id, portal_customer_id, source_role_id, source_group_id,
			       substituted_for_user_id, is_active, created_at
			FROM approval_step_approvers WHERE approval_step_instance_id = ? ORDER BY id
		`, si.ID)
		if err != nil {
			return nil, fmt.Errorf("query approvers: %w", err)
		}
		for appRows.Next() {
			var a models.ApprovalStepApprover
			if err := appRows.Scan(&a.ID, &a.ApprovalStepInstanceID, &a.UserID, &a.PortalCustomerID,
				&a.SourceRoleID, &a.SourceGroupID, &a.SubstitutedForUserID, &a.IsActive, &a.CreatedAt); err != nil {
				_ = appRows.Close()
				return nil, fmt.Errorf("scan approver: %w", err)
			}
			si.Approvers = append(si.Approvers, a)
		}
		if err := appRows.Err(); err != nil {
			_ = appRows.Close()
			return nil, fmt.Errorf("iterate approvers: %w", err)
		}
		_ = appRows.Close()
		req.StepInstances = append(req.StepInstances, si)
	}
	if err := stepRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate step instances: %w", err)
	}

	// metadata is JSONB on Postgres / TEXT on SQLite. COALESCE(...,'') was
	// fine for TEXT but rejected by JSONB ('' isn't valid JSON), so we leave
	// it nullable and convert NULL → "" / present → bytes on the Go side.
	decRows, err := r.db.QueryContext(ctx, `
		SELECT id, approval_request_id, approval_step_instance_id, actor_user_id, actor_portal_customer_id,
		       decision, COALESCE(comment, ''), delegated_to_user_id, metadata, created_at
		FROM approval_decisions WHERE approval_request_id = ? ORDER BY created_at, id
	`, requestID)
	if err != nil {
		return nil, fmt.Errorf("query decisions: %w", err)
	}
	defer func() { _ = decRows.Close() }()
	for decRows.Next() {
		var d models.ApprovalDecision
		var metadata sql.NullString
		if err := decRows.Scan(
			&d.ID, &d.ApprovalRequestID, &d.ApprovalStepInstanceID, &d.ActorUserID, &d.ActorPortalCustomerID,
			&d.Decision, &d.Comment, &d.DelegatedToUserID, &metadata, &d.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		if metadata.Valid && metadata.String != "" {
			d.Metadata = json.RawMessage(metadata.String)
		}
		req.Decisions = append(req.Decisions, d)
	}
	if err := decRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate decisions: %w", err)
	}
	return req, nil
}

// FindPendingRequestIDForItem returns the single pending request id for an
// item, or (nil, nil) when none. The unique partial index
// uq_approval_requests_one_open_per_item guarantees at most one row.
func (r *ApprovalRepository) FindPendingRequestIDForItem(ctx context.Context, itemID int) (*int, error) {
	var id int
	err := r.db.QueryRowContext(ctx,
		`SELECT id FROM approval_requests WHERE item_id = ? AND status = 'pending'`, itemID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find pending request: %w", err)
	}
	return &id, nil
}

// FindRequestIDsForItem returns every request id for an item, in created_at order.
func (r *ApprovalRepository) FindRequestIDsForItem(ctx context.Context, itemID int) ([]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id FROM approval_requests WHERE item_id = ? ORDER BY created_at`, itemID)
	if err != nil {
		return nil, fmt.Errorf("query timeline: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntList(rows)
}

// FindRequestIDsForItemPage returns a newest-first bounded timeline page.
func (r *ApprovalRepository) FindRequestIDsForItemPage(ctx context.Context, itemID, limit, offset int) (ids []int, total int, err error) {
	if limit <= 0 {
		return nil, 0, errors.New("approval page limit must be positive")
	}
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM approval_requests WHERE item_id = ?", itemID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count item approvals: %w", err)
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM approval_requests
		WHERE item_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?`, itemID, limit, max(offset, 0))
	if err != nil {
		return nil, 0, fmt.Errorf("query item approval page: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids, err = scanIntList(rows)
	return ids, total, err
}

// FindRequestIDsForActor returns request ids where the actor (user or portal
// customer) is in the active approver pool of any pending step. actorColumn
// must be exactly "user_id" or "portal_customer_id" — the literal is
// concatenated into the query, callers cannot pass arbitrary strings.
func (r *ApprovalRepository) FindRequestIDsForActor(ctx context.Context, actorColumn string, actorID int, status string) ([]int, error) {
	return r.findRequestIDsForActor(ctx, actorColumn, actorID, status, nil)
}

// FindRequestIDsForActorPage returns a stable bounded page for an approver.
func (r *ApprovalRepository) FindRequestIDsForActorPage(ctx context.Context, actorColumn string, actorID int, status string, limit, offset int) (ids []int, total int, err error) {
	if actorColumn != "user_id" && actorColumn != "portal_customer_id" {
		return nil, 0, fmt.Errorf("invalid actor column %q", actorColumn)
	}
	if limit <= 0 {
		return nil, 0, errors.New("approval page limit must be positive")
	}
	if status == "" {
		status = models.ApprovalRequestStatusPending
	}
	base := fmt.Sprintf(`
		FROM approval_requests ar
		JOIN approval_step_instances asi ON asi.approval_request_id = ar.id AND asi.status = 'pending'
		JOIN approval_step_approvers asa ON asa.approval_step_instance_id = asi.id AND asa.is_active = true
		WHERE ar.status = ? AND asa.%s = ?`, actorColumn)
	args := []any{status, actorID}
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT ar.id) "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count approvals for actor: %w", err)
	}
	pageArgs := append(slices.Clone(args), limit, max(offset, 0))
	rows, err := r.db.QueryContext(ctx, "SELECT DISTINCT ar.id "+base+" ORDER BY ar.id DESC LIMIT ? OFFSET ?", pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query approval page for actor: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids, err = scanIntList(rows)
	return ids, total, err
}

// FindRequestIDsForActorInChannel is the portal-scoped counterpart to
// FindRequestIDsForActor. It filters at the item join so requests from another
// portal can never enter the hydration path.
func (r *ApprovalRepository) FindRequestIDsForActorInChannel(ctx context.Context, actorColumn string, actorID int, status string, channelID int) ([]int, error) {
	return r.findRequestIDsForActor(ctx, actorColumn, actorID, status, &channelID)
}

func (r *ApprovalRepository) findRequestIDsForActor(ctx context.Context, actorColumn string, actorID int, status string, channelID *int) ([]int, error) {
	if actorColumn != "user_id" && actorColumn != "portal_customer_id" {
		return nil, fmt.Errorf("invalid actor column %q", actorColumn)
	}
	if status == "" {
		status = models.ApprovalRequestStatusPending
	}
	channelJoin := ""
	args := []any{}
	if channelID != nil {
		channelJoin = "JOIN items i ON i.id = ar.item_id AND i.channel_id = ?"
		args = append(args, *channelID)
	}
	args = append(args, status, actorID)
	q := fmt.Sprintf(`
		SELECT DISTINCT ar.id
		FROM approval_requests ar
		JOIN approval_step_instances asi ON asi.approval_request_id = ar.id AND asi.status = 'pending'
		JOIN approval_step_approvers asa ON asa.approval_step_instance_id = asi.id AND asa.is_active = true
		%s
		WHERE ar.status = ? AND asa.%s = ?
		ORDER BY ar.id DESC
	`, channelJoin, actorColumn)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query for actor: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntList(rows)
}

// ActorHasActivePoolMembershipOnItem reports whether the actor is in an
// active-approver row of a pending step on a pending request for the item.
// Powers approver-derived item-view access on the permission service.
//
// channelID is optional. When non-nil the item must belong to that channel —
// used by portal flows where approver-derived access must not leak across
// portal-channel boundaries. Internal (non-portal) callers pass nil.
func (r *ApprovalRepository) ActorHasActivePoolMembershipOnItem(ctx context.Context, actorColumn string, actorID, itemID int, channelID *int) (bool, error) {
	if actorColumn != "user_id" && actorColumn != "portal_customer_id" {
		return false, fmt.Errorf("invalid actor column %q", actorColumn)
	}
	channelJoin := ""
	args := []any{}
	if channelID != nil {
		channelJoin = "JOIN items i ON i.id = ar.item_id AND i.channel_id = ?"
		args = append(args, *channelID)
	}
	args = append(args, itemID, actorID)
	q := fmt.Sprintf(`
		SELECT 1
		FROM approval_requests ar
		JOIN approval_step_instances asi ON asi.approval_request_id = ar.id AND asi.status = 'pending'
		JOIN approval_step_approvers asa ON asa.approval_step_instance_id = asi.id AND asa.is_active = true
		%s
		WHERE ar.item_id = ? AND ar.status = 'pending' AND asa.%s = ?
		LIMIT 1
	`, channelJoin, actorColumn)
	var one int
	err := r.db.QueryRowContext(ctx, q, args...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check active-pool membership: %w", err)
	}
	return true, nil
}

// FindGatedRequestForTransition returns the pending approval request id when
// (itemID, fromStatusID, toStatusID) names a transition the request engine
// owns (its approve_transition_id or deny_transition_id), or (nil, nil) when
// the transition isn't gated.
func (r *ApprovalRepository) FindGatedRequestForTransition(ctx context.Context, itemID, fromStatusID, toStatusID int) (*int, error) {
	var id int
	err := r.db.QueryRowContext(ctx, `
		SELECT ar.id
		FROM approval_requests ar
		JOIN approval_set_statuses ass ON ass.id = ar.approval_set_status_id
		JOIN workflow_transitions wt
			ON wt.id IN (ass.approve_transition_id, ass.deny_transition_id)
		WHERE ar.item_id = ? AND ar.status = 'pending'
		  AND (wt.from_status_id = ? OR wt.from_all_statuses = TRUE)
		  AND wt.to_status_id = ?
		LIMIT 1
	`, itemID, fromStatusID, toStatusID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find gated transition: %w", err)
	}
	return &id, nil
}

// GatedTransitionsView is what callers need from
// FindGatedTransitionsForItem: the request id + status, the approve/deny
// transition IDs that belong to it, and whether the user is in the active pool.
type GatedTransitionsView struct {
	RequestID           int
	Status              string
	ApproveTransitionID int
	DenyTransitionID    int
	UserCanDecide       bool
}

// FindGatedTransitionsForItem returns the gated-transition view for an item +
// user, or (nil, nil) if no approval is pending on the item.
func (r *ApprovalRepository) FindGatedTransitionsForItem(ctx context.Context, itemID, userID int) (*GatedTransitionsView, error) {
	var v GatedTransitionsView
	err := r.db.QueryRowContext(ctx, `
		SELECT ar.id, ar.status, ass.approve_transition_id, ass.deny_transition_id
		FROM approval_requests ar
		JOIN approval_set_statuses ass ON ass.id = ar.approval_set_status_id
		WHERE ar.item_id = ? AND ar.status = 'pending'
		LIMIT 1
	`, itemID).Scan(&v.RequestID, &v.Status, &v.ApproveTransitionID, &v.DenyTransitionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load gated transitions: %w", err)
	}

	err = r.db.QueryRowContext(ctx, `
		SELECT 1
		FROM approval_step_instances si
		JOIN approval_step_approvers a ON a.approval_step_instance_id = si.id
		WHERE si.approval_request_id = ?
		  AND si.status = 'pending'
		  AND si.started_at IS NOT NULL
		  AND a.is_active = true
		  AND a.user_id = ?
		LIMIT 1
	`, v.RequestID, userID).Scan(new(int))
	if err == nil {
		v.UserCanDecide = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("check user-can-decide: %w", err)
	}
	return &v, nil
}

// GetItemIDForRequest returns the item_id of an approval request.
// Returns ErrNotFound on missing row.
func (r *ApprovalRepository) GetItemIDForRequest(ctx context.Context, requestID int) (int, error) {
	var itemID int
	err := r.db.QueryRowContext(ctx,
		`SELECT item_id FROM approval_requests WHERE id = ?`, requestID).Scan(&itemID)
	if err != nil {
		return 0, notFoundOrWrap(err, "load item_id")
	}
	return itemID, nil
}

// StepInstanceBelongsToRequest reports whether the step instance is part of
// the given approval request. Used by handlers to confirm scoping before
// admin actions.
func (r *ApprovalRepository) StepInstanceBelongsToRequest(ctx context.Context, stepInstanceID, requestID int) (bool, error) {
	var owner int
	err := r.db.QueryRowContext(ctx,
		`SELECT approval_request_id FROM approval_step_instances WHERE id = ?`, stepInstanceID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load step instance owner: %w", err)
	}
	return owner == requestID, nil
}

// FindDueStepInstanceIDs returns up to batchSize pending step instance ids
// whose escalation_due_at has passed. Used by the escalation sweeper.
func (r *ApprovalRepository) FindDueStepInstanceIDs(ctx context.Context, batchSize int) ([]int, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM approval_step_instances
		WHERE status = 'pending'
		  AND escalation_due_at IS NOT NULL
		  AND escalation_due_at <= CURRENT_TIMESTAMP
		ORDER BY escalation_due_at
		LIMIT ?
	`, batchSize)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query due step instances: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntList(rows)
}

// CountPendingApproversForRole counts approval_step_approvers rows whose
// source_role_id = roleID and whose request is still pending. Used by the
// workspace-role delete path to refuse the request when the role is tied to
// in-flight approvals.
func (r *ApprovalRepository) CountPendingApproversForRole(ctx context.Context, roleID int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM approval_step_approvers asa
		JOIN approval_step_instances asi ON asi.id = asa.approval_step_instance_id
		JOIN approval_requests ar ON ar.id = asi.approval_request_id
		WHERE asa.source_role_id = ? AND ar.status = 'pending'
	`, roleID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending approvers for role: %w", err)
	}
	return count, nil
}

func (r *ApprovalRepository) findRequestRowInChannel(ctx context.Context, requestID int, channelID *int) (*models.ApprovalRequest, error) {
	var req models.ApprovalRequest
	var fromStatus sql.NullInt64
	query := `
		SELECT ar.id, ar.item_id, i.workspace_id, ar.approval_set_status_id, ar.status_id, ar.from_status_id,
		       ar.triggered_by_user_id, ar.status, ar.created_at, ar.completed_at
		FROM approval_requests ar
		JOIN items i ON i.id = ar.item_id
		WHERE ar.id = ?
	`
	args := []any{requestID}
	if channelID != nil {
		query = `
			SELECT ar.id, ar.item_id, i.workspace_id, ar.approval_set_status_id, ar.status_id, ar.from_status_id,
			       ar.triggered_by_user_id, ar.status, ar.created_at, ar.completed_at
			FROM approval_requests ar
			JOIN items i ON i.id = ar.item_id
			WHERE ar.id = ? AND i.channel_id = ?
		`
		args = append(args, *channelID)
	}
	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&req.ID, &req.ItemID, &req.WorkspaceID, &req.ApprovalSetStatusID, &req.StatusID,
		&fromStatus, &req.TriggeredByUserID, &req.Status, &req.CreatedAt, &req.CompletedAt,
	)
	if err != nil {
		return nil, notFoundOrWrap(err, "load request")
	}
	if fromStatus.Valid {
		v := int(fromStatus.Int64)
		req.FromStatusID = &v
	}
	return &req, nil
}

// ============================================================================
// Read methods — inside transaction
// ============================================================================

// LoadRequestByIDInTx loads a bare request row inside a tx. Returns
// ErrNotFound if no row matches.
func (r *ApprovalRepository) LoadRequestByIDInTx(ctx context.Context, tx database.Tx, requestID int) (*models.ApprovalRequest, error) {
	return r.loadRequestByIDInChannelInTx(ctx, tx, requestID, nil)
}

// LoadRequestByIDInChannelInTx is the decision-path guard for portal routes.
// It keeps the channel check in the same transaction as the state mutation.
func (r *ApprovalRepository) LoadRequestByIDInChannelInTx(ctx context.Context, tx database.Tx, requestID, channelID int) (*models.ApprovalRequest, error) {
	return r.loadRequestByIDInChannelInTx(ctx, tx, requestID, &channelID)
}

func (r *ApprovalRepository) loadRequestByIDInChannelInTx(ctx context.Context, tx database.Tx, requestID int, channelID *int) (*models.ApprovalRequest, error) {
	var req models.ApprovalRequest
	var fromStatus sql.NullInt64
	query := `
		SELECT ar.id, ar.item_id, i.workspace_id, ar.approval_set_status_id, ar.status_id, ar.from_status_id,
		       ar.triggered_by_user_id, ar.status, ar.created_at, ar.completed_at
		FROM approval_requests ar
		JOIN items i ON i.id = ar.item_id
		WHERE ar.id = ?
	`
	args := []any{requestID}
	if channelID != nil {
		query = `
			SELECT ar.id, ar.item_id, i.workspace_id, ar.approval_set_status_id, ar.status_id, ar.from_status_id,
			       ar.triggered_by_user_id, ar.status, ar.created_at, ar.completed_at
			FROM approval_requests ar
			JOIN items i ON i.id = ar.item_id
			WHERE ar.id = ? AND i.channel_id = ?
		`
		args = append(args, *channelID)
	}
	err := tx.QueryRowContext(ctx, query, args...).Scan(
		&req.ID, &req.ItemID, &req.WorkspaceID, &req.ApprovalSetStatusID, &req.StatusID,
		&fromStatus, &req.TriggeredByUserID, &req.Status, &req.CreatedAt, &req.CompletedAt,
	)
	if err != nil {
		return nil, notFoundOrWrap(err, "load request")
	}
	if fromStatus.Valid {
		v := int(fromStatus.Int64)
		req.FromStatusID = &v
	}
	return &req, nil
}

// LoadStepInstanceByIDInTx loads a single step instance row inside a tx.
// Returns ErrNotFound if no row matches.
func (r *ApprovalRepository) LoadStepInstanceByIDInTx(ctx context.Context, tx database.Tx, id int) (*models.ApprovalStepInstance, error) {
	var si models.ApprovalStepInstance
	err := tx.QueryRowContext(ctx, `
		SELECT id, approval_request_id, approval_step_id, display_order, status,
		       escalation_due_at, escalation_count, last_escalated_at, started_at, completed_at
		FROM approval_step_instances WHERE id = ?
	`, id).Scan(
		&si.ID, &si.ApprovalRequestID, &si.ApprovalStepID, &si.DisplayOrder, &si.Status,
		&si.EscalationDueAt, &si.EscalationCount, &si.LastEscalatedAt, &si.StartedAt, &si.CompletedAt,
	)
	if err != nil {
		return nil, notFoundOrWrap(err, "load step instance")
	}
	return &si, nil
}

// FindActiveStepForUser returns the lowest-display-order pending step instance
// where userID has an active approver row, or (nil, nil) if none.
func (r *ApprovalRepository) FindActiveStepForUser(ctx context.Context, tx database.Tx, requestID, userID int) (*models.ApprovalStepInstance, error) {
	return r.findActiveStepFor(ctx, tx, requestID, "user_id", userID)
}

// FindActiveStepForCustomer is the portal-customer counterpart.
func (r *ApprovalRepository) FindActiveStepForCustomer(ctx context.Context, tx database.Tx, requestID, customerID int) (*models.ApprovalStepInstance, error) {
	return r.findActiveStepFor(ctx, tx, requestID, "portal_customer_id", customerID)
}

func (r *ApprovalRepository) findActiveStepFor(ctx context.Context, tx database.Tx, requestID int, actorColumn string, actorID int) (*models.ApprovalStepInstance, error) {
	if actorColumn != "user_id" && actorColumn != "portal_customer_id" {
		return nil, fmt.Errorf("invalid actor column %q", actorColumn)
	}
	q := fmt.Sprintf(`
		SELECT asi.id, asi.approval_request_id, asi.approval_step_id, asi.display_order, asi.status,
		       asi.escalation_due_at, asi.escalation_count, asi.last_escalated_at, asi.started_at, asi.completed_at
		FROM approval_step_instances asi
		JOIN approval_step_approvers asa ON asa.approval_step_instance_id = asi.id AND asa.is_active = true AND asa.%s = ?
		WHERE asi.approval_request_id = ? AND asi.status = 'pending'
		ORDER BY asi.display_order
		LIMIT 1
	`, actorColumn)
	var si models.ApprovalStepInstance
	err := tx.QueryRowContext(ctx, q, actorID, requestID).Scan(
		&si.ID, &si.ApprovalRequestID, &si.ApprovalStepID, &si.DisplayOrder, &si.Status,
		&si.EscalationDueAt, &si.EscalationCount, &si.LastEscalatedAt, &si.StartedAt, &si.CompletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active step for actor: %w", err)
	}
	return &si, nil
}

// FindNextPendingStep returns the next pending step instance after the given
// display order. found=false means we're at the last step.
func (r *ApprovalRepository) FindNextPendingStep(ctx context.Context, tx database.Tx, requestID, afterDisplayOrder int) (stepInstanceID, stepID int, found bool, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT id, approval_step_id FROM approval_step_instances
		WHERE approval_request_id = ? AND display_order > ? AND status = 'pending'
		ORDER BY display_order
		LIMIT 1
	`, requestID, afterDisplayOrder).Scan(&stepInstanceID, &stepID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("find next pending step: %w", err)
	}
	return stepInstanceID, stepID, true, nil
}

// CountStepStates returns (pending, total) for a request — pending counts
// every step that is not yet 'approved'. Used by the parallel-mode evaluator.
func (r *ApprovalRepository) CountStepStates(ctx context.Context, tx database.Tx, requestID int) (pending, total int, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status NOT IN ('approved') THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM approval_step_instances WHERE approval_request_id = ?
	`, requestID).Scan(&pending, &total)
	if err != nil {
		return 0, 0, fmt.Errorf("count step states: %w", err)
	}
	return pending, total, nil
}

// CountVotes returns (approves, rejects) on a step instance. Comment rows
// don't count toward either column.
func (r *ApprovalRepository) CountVotes(ctx context.Context, tx database.Tx, stepInstanceID int) (approves, rejects int, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE WHEN decision = 'approve' THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN decision = 'reject'  THEN 1 ELSE 0 END), 0)
		FROM approval_decisions WHERE approval_step_instance_id = ?
	`, stepInstanceID).Scan(&approves, &rejects)
	if err != nil {
		return 0, 0, fmt.Errorf("count votes: %w", err)
	}
	return approves, rejects, nil
}

// CountActiveApprovers returns the size of the live approver pool on a
// step instance.
func (r *ApprovalRepository) CountActiveApprovers(ctx context.Context, tx database.Tx, stepInstanceID int) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM approval_step_approvers WHERE approval_step_instance_id = ? AND is_active = true`,
		stepInstanceID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active approvers: %w", err)
	}
	return n, nil
}

// LoadActiveApproverUserIDs returns active approver internal-user IDs for a
// step instance, sorted ascending. Portal-customer rows are skipped.
func (r *ApprovalRepository) LoadActiveApproverUserIDs(ctx context.Context, tx database.Tx, stepInstanceID int) ([]int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT user_id FROM approval_step_approvers
		WHERE approval_step_instance_id = ? AND is_active = true AND user_id IS NOT NULL ORDER BY user_id
	`, stepInstanceID)
	if err != nil {
		return nil, fmt.Errorf("query active user approvers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntList(rows)
}

// LoadActiveApproverCustomerIDs is the portal-customer counterpart.
func (r *ApprovalRepository) LoadActiveApproverCustomerIDs(ctx context.Context, tx database.Tx, stepInstanceID int) ([]int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT portal_customer_id FROM approval_step_approvers
		WHERE approval_step_instance_id = ? AND is_active = true AND portal_customer_id IS NOT NULL ORDER BY portal_customer_id
	`, stepInstanceID)
	if err != nil {
		return nil, fmt.Errorf("query active customer approvers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanIntList(rows)
}

// GetRequestIDForStep returns the parent approval_request_id for a step
// instance. Used during approver snapshotting to attach audit rows.
func (r *ApprovalRepository) GetRequestIDForStep(ctx context.Context, tx database.Tx, stepInstanceID int) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx,
		`SELECT approval_request_id FROM approval_step_instances WHERE id = ?`, stepInstanceID).Scan(&id)
	if err != nil {
		return 0, notFoundOrWrap(err, "load request id for step")
	}
	return id, nil
}

// GetItemCurrentStatusID returns an item's current status_id inside a tx —
// used by Cancel to detect drift before reverting.
func (r *ApprovalRepository) GetItemCurrentStatusID(ctx context.Context, tx database.Tx, itemID int) (int, error) {
	var statusID int
	err := tx.QueryRowContext(ctx, `SELECT status_id FROM items WHERE id = ?`, itemID).Scan(&statusID)
	if err != nil {
		return 0, notFoundOrWrap(err, "load item current status")
	}
	return statusID, nil
}

// GetTransitionEndpoints returns (from_status_id, to_status_id) for a
// workflow_transitions row. The source is nil for an all-statuses or initial
// transition. finalizeRequest only needs the destination status.
func (r *ApprovalRepository) GetTransitionEndpoints(ctx context.Context, tx database.Tx, transitionID int) (fromStatusID *int, toStatusID int, err error) {
	var fromStatus sql.NullInt64
	err = tx.QueryRowContext(ctx,
		`SELECT from_status_id, to_status_id FROM workflow_transitions WHERE id = ?`, transitionID,
	).Scan(&fromStatus, &toStatusID)
	if err != nil {
		return nil, 0, notFoundOrWrap(err, "load transition endpoints")
	}
	if fromStatus.Valid {
		id := int(fromStatus.Int64)
		fromStatusID = &id
	}
	return fromStatusID, toStatusID, nil
}

// ============================================================================
// Writes
// ============================================================================

// CreateRequest inserts a new approval_requests row with status='pending'
// and returns its id.
func (r *ApprovalRepository) CreateRequest(ctx context.Context, tx database.Tx, itemID, approvalSetStatusID, statusID int, fromStatusID *int, triggeredByUserID int) (int, error) {
	var fromStatus sql.NullInt64
	if fromStatusID != nil && *fromStatusID > 0 {
		fromStatus = sql.NullInt64{Int64: int64(*fromStatusID), Valid: true}
	}
	var id64 int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO approval_requests (item_id, approval_set_status_id, status_id, from_status_id, triggered_by_user_id, status, created_at)
		VALUES (?, ?, ?, ?, ?, 'pending', ?)
		RETURNING id
	`, itemID, approvalSetStatusID, statusID, fromStatus, triggeredByUserID, time.Now()).Scan(&id64); err != nil {
		return 0, fmt.Errorf("insert approval_request: %w", err)
	}
	return int(id64), nil
}

// CreateStepInstance inserts a new approval_step_instances row.
// Pass startedAt as a non-zero time to mark the step as started immediately;
// pass dueAt for an active escalation window. Status is 'pending' by default.
func (r *ApprovalRepository) CreateStepInstance(ctx context.Context, tx database.Tx, requestID, stepID, displayOrder int, startedAt, dueAt sql.NullTime) (int, error) {
	var id64 int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO approval_step_instances (approval_request_id, approval_step_id, display_order, status, escalation_due_at, started_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id
	`, requestID, stepID, displayOrder, models.ApprovalStepStatusPending, dueAt, startedAt).Scan(&id64); err != nil {
		return 0, fmt.Errorf("insert step instance: %w", err)
	}
	return int(id64), nil
}

// StartStepInstance sets started_at and escalation_due_at on a pending step.
// Used when sequential mode advances to the next step.
func (r *ApprovalRepository) StartStepInstance(ctx context.Context, tx database.Tx, id int, startedAt time.Time, dueAt sql.NullTime) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE approval_step_instances SET started_at = ?, escalation_due_at = ? WHERE id = ?`,
		startedAt, dueAt, id); err != nil {
		return fmt.Errorf("start step instance: %w", err)
	}
	return nil
}

// UpdateStepInstanceStatusComplete sets status + completed_at on a step
// instance (caller picks the terminal status: approved | rejected | skipped).
func (r *ApprovalRepository) UpdateStepInstanceStatusComplete(ctx context.Context, tx database.Tx, id int, status string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE approval_step_instances SET status = ?, completed_at = ? WHERE id = ?`,
		status, time.Now(), id); err != nil {
		return fmt.Errorf("update step instance status: %w", err)
	}
	return nil
}

// MarkStepEscalated terminates a step from the escalation flow: sets the
// terminal status, completed_at = now, and clears escalation_due_at. The
// WHERE-status='pending' clause makes it a no-op for already-resolved steps.
func (r *ApprovalRepository) MarkStepEscalated(ctx context.Context, tx database.Tx, id int, status string) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE approval_step_instances SET status = ?, completed_at = ?, escalation_due_at = NULL
		WHERE id = ? AND status = 'pending'
	`, status, time.Now(), id); err != nil {
		return fmt.Errorf("mark step escalated: %w", err)
	}
	return nil
}

// UpdateRequestStatusComplete sets status + completed_at on a request
// (caller picks one of the request-level terminal statuses; approve, reject,
// or the British-spelled cancel value the schema enum uses).
func (r *ApprovalRepository) UpdateRequestStatusComplete(ctx context.Context, tx database.Tx, id int, status string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE approval_requests SET status = ?, completed_at = ? WHERE id = ?`,
		status, time.Now(), id); err != nil {
		return fmt.Errorf("update request status: %w", err)
	}
	return nil
}

// SkipPendingPeerSteps marks every pending step on a request, except the
// one whose decision triggered the skip, as 'skipped' with completed_at=now.
// Used when a parallel-mode rejection terminates the request.
func (r *ApprovalRepository) SkipPendingPeerSteps(ctx context.Context, tx database.Tx, requestID, exceptStepInstanceID int) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE approval_step_instances
		SET status = ?, completed_at = ?
		WHERE approval_request_id = ? AND status = 'pending' AND id <> ?
	`, models.ApprovalStepStatusSkipped, time.Now(), requestID, exceptStepInstanceID); err != nil {
		return fmt.Errorf("skip peer steps: %w", err)
	}
	return nil
}

// DeactivateApprovers flips is_active=TRUE → is_active=FALSE on every active
// approver row for a step. Used by reassign / refresh flows to tombstone the
// prior pool while preserving snapshot history.
func (r *ApprovalRepository) DeactivateApprovers(ctx context.Context, tx database.Tx, stepInstanceID int) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE approval_step_approvers SET is_active = false WHERE approval_step_instance_id = ? AND is_active = true`,
		stepInstanceID); err != nil {
		return fmt.Errorf("deactivate approvers: %w", err)
	}
	return nil
}

// DeactivateApproverByUser tombstones a single user's active row in a step.
// Used by Delegate to retire the original seat before inserting the substitute.
func (r *ApprovalRepository) DeactivateApproverByUser(ctx context.Context, tx database.Tx, stepInstanceID, userID int) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE approval_step_approvers
		SET is_active = false
		WHERE approval_step_instance_id = ? AND user_id = ? AND is_active = true
	`, stepInstanceID, userID); err != nil {
		return fmt.Errorf("deactivate user approver: %w", err)
	}
	return nil
}

// InsertApprover inserts an approval_step_approvers row. Exactly one of
// UserID / PortalCustomerID must be > 0 — the schema CHECK enforces this.
func (r *ApprovalRepository) InsertApprover(ctx context.Context, tx database.Tx, stepInstanceID int, ai ApproverInsert) error {
	var userID, portalCustomerID any
	switch {
	case ai.PortalCustomerID > 0:
		portalCustomerID = ai.PortalCustomerID
	case ai.UserID > 0:
		userID = ai.UserID
	default:
		return errors.New("InsertApprover requires user_id or portal_customer_id")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approval_step_approvers
			(approval_step_instance_id, user_id, portal_customer_id, source_role_id, source_group_id, substituted_for_user_id, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, ?, true, ?)
	`, stepInstanceID, userID, portalCustomerID, ai.SourceRoleID, ai.SourceGroupID, ai.SubstitutedForUserID, time.Now()); err != nil {
		return fmt.Errorf("insert approver: %w", err)
	}
	return nil
}

// InsertDelegatedApprover inserts a single user-only approver row with
// substituted_for_user_id set, modeling the delegate flow.
func (r *ApprovalRepository) InsertDelegatedApprover(ctx context.Context, tx database.Tx, stepInstanceID, toUserID, substitutedForUserID int) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO approval_step_approvers
			(approval_step_instance_id, user_id, source_role_id, source_group_id, substituted_for_user_id, is_active, created_at)
		VALUES (?, ?, NULL, NULL, ?, true, ?)
	`, stepInstanceID, toUserID, substitutedForUserID, time.Now()); err != nil {
		return fmt.Errorf("insert delegated approver: %w", err)
	}
	return nil
}

// UpdateEscalationCounters bumps escalation_count + last_escalated_at + the
// new escalation_due_at on a pending step. Returns silently if the step is
// no longer pending.
func (r *ApprovalRepository) UpdateEscalationCounters(ctx context.Context, tx database.Tx, stepInstanceID, count int, lastEscalatedAt time.Time, dueAt sql.NullTime) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE approval_step_instances
		SET escalation_count = ?, last_escalated_at = ?, escalation_due_at = ?
		WHERE id = ? AND status = 'pending'
	`, count, lastEscalatedAt, dueAt, stepInstanceID); err != nil {
		return fmt.Errorf("update escalation counters: %w", err)
	}
	return nil
}

// WriteDecision inserts an approval_decisions audit row and returns the
// inserted decision (id + created_at populated). Pass nil for both actor
// params when the actor is the system (sweeper-driven).
func (r *ApprovalRepository) WriteDecision(ctx context.Context, tx database.Tx, requestID int, stepInstanceID, actorUserID, actorPortalCustomerID *int, decision, comment string, delegatedToUserID *int, metadata map[string]any) (*models.ApprovalDecision, error) {
	// metadata is JSONB on Postgres. Writing the empty string (which is what
	// `string(nil)` produces) fails JSONB parsing, so a nil map must hit the
	// driver as a Go nil interface (→ SQL NULL).
	var metadataArg any
	if metadata != nil {
		metaJSON, err := json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal decision metadata: %w", err)
		}
		metadataArg = string(metaJSON)
	}
	now := time.Now()
	var id64 int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO approval_decisions
			(approval_request_id, approval_step_instance_id, actor_user_id, actor_portal_customer_id,
			 decision, comment, delegated_to_user_id, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, requestID, stepInstanceID, actorUserID, actorPortalCustomerID,
		decision, comment, delegatedToUserID, metadataArg, now).Scan(&id64); err != nil {
		return nil, fmt.Errorf("insert decision: %w", err)
	}
	d := &models.ApprovalDecision{
		ID:                     int(id64),
		ApprovalRequestID:      requestID,
		ApprovalStepInstanceID: stepInstanceID,
		ActorUserID:            actorUserID,
		ActorPortalCustomerID:  actorPortalCustomerID,
		Decision:               decision,
		Comment:                comment,
		DelegatedToUserID:      delegatedToUserID,
		CreatedAt:              now,
	}
	if s, ok := metadataArg.(string); ok && s != "" {
		d.Metadata = json.RawMessage(s)
	}
	return d, nil
}

// ============================================================================
// Helpers
// ============================================================================

func scanIntList(rows *sql.Rows) ([]int, error) {
	var out []int
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scan int: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ints: %w", err)
	}
	return out, nil
}
