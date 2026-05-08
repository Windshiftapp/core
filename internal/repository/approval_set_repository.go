package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ApprovalSetRepository owns SQL for the approval template tables:
// approval_sets, approval_set_statuses, approval_steps. Mirrors the
// ChannelRepository pattern: reads on db, writes on tx.
type ApprovalSetRepository struct {
	db database.Database
}

// NewApprovalSetRepository constructs an ApprovalSetRepository.
func NewApprovalSetRepository(db database.Database) *ApprovalSetRepository {
	return &ApprovalSetRepository{db: db}
}

// ApprovalSetTransitionDriver is one approval set whose approve_transition_id
// or deny_transition_id is the queried transition. Used by
// TransitionGovernanceHandler to render override warnings.
type ApprovalSetTransitionDriver struct {
	ApprovalSetID       int    `json:"approval_set_id"`
	ApprovalSetName     string `json:"approval_set_name"`
	ApprovalSetStatusID int    `json:"approval_set_status_id"`
	Role                string `json:"role"` // 'approve_transition_id' | 'deny_transition_id'
}

// ============================================================================
// Reads
// ============================================================================

// FindAll returns approval sets, optionally filtered by workflow ID. Each row
// is the bare set + workflow_name; no nested set-statuses or steps. Use
// FindByID for the full graph.
func (r *ApprovalSetRepository) FindAll(ctx context.Context, workflowID *int) ([]models.ApprovalSet, error) {
	query := `
		SELECT a.id, a.name, a.description, a.workflow_id, a.created_at, a.updated_at, w.name AS workflow_name
		FROM approval_sets a
		JOIN workflows w ON a.workflow_id = w.id`
	var args []interface{}
	if workflowID != nil {
		query += " WHERE a.workflow_id = ?"
		args = append(args, *workflowID)
	}
	query += " ORDER BY a.name"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query approval sets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out, err := scanApprovalSetRows(rows)
	if err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approval sets: %w", err)
	}
	return out, nil
}

// FindByID returns a single approval set with its active set-statuses and
// each status's steps fully populated. Returns ErrNotFound if no row exists.
//
// Issues 1 set query + 1 statuses query + 1 step query per status (preserves
// the prior handler shape). Step counts are small in practice.
func (r *ApprovalSetRepository) FindByID(ctx context.Context, id int) (*models.ApprovalSet, error) {
	var s models.ApprovalSet
	var description sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT a.id, a.name, a.description, a.workflow_id, a.created_at, a.updated_at, w.name AS workflow_name
		FROM approval_sets a
		JOIN workflows w ON a.workflow_id = w.id
		WHERE a.id = ?
	`, id).Scan(&s.ID, &s.Name, &description, &s.WorkflowID, &s.CreatedAt, &s.UpdatedAt, &s.WorkflowName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load approval set: %w", err)
	}
	s.Description = description.String

	statusRows, err := r.db.QueryContext(ctx, `
		SELECT ass.id, ass.approval_set_id, ass.status_id, ass.approve_transition_id, ass.deny_transition_id,
		       ass.step_mode, ass.created_at, st.name AS status_name
		FROM approval_set_statuses ass
		JOIN statuses st ON st.id = ass.status_id
		WHERE ass.approval_set_id = ? AND ass.is_active = true
		ORDER BY ass.id
	`, id)
	if err != nil {
		return nil, fmt.Errorf("load approval_set_statuses: %w", err)
	}
	defer func() { _ = statusRows.Close() }()

	var setStatuses []models.ApprovalSetStatus
	for statusRows.Next() {
		var ass models.ApprovalSetStatus
		var statusName sql.NullString
		if err := statusRows.Scan(&ass.ID, &ass.ApprovalSetID, &ass.StatusID,
			&ass.ApproveTransitionID, &ass.DenyTransitionID,
			&ass.StepMode, &ass.CreatedAt, &statusName); err != nil {
			return nil, fmt.Errorf("scan approval_set_status: %w", err)
		}
		ass.StatusName = statusName.String
		setStatuses = append(setStatuses, ass)
	}
	if err := statusRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approval_set_statuses: %w", err)
	}

	for i := range setStatuses {
		steps, err := r.FindStepsByStatusID(ctx, setStatuses[i].ID)
		if err != nil {
			return nil, err
		}
		if steps == nil {
			steps = []models.ApprovalStep{}
		}
		setStatuses[i].Steps = steps
	}
	if setStatuses == nil {
		setStatuses = []models.ApprovalSetStatus{}
	}
	s.SetStatuses = setStatuses
	return &s, nil
}

// FindGatedStatusesForSets returns a map keyed by approval_set_id of
// ApprovalSetStatusSummary entries — used by list endpoints to render status
// chips without a per-row detail fetch. One IN-batched query.
func (r *ApprovalSetRepository) FindGatedStatusesForSets(ctx context.Context, setIDs []int) (map[int][]models.ApprovalSetStatusSummary, error) {
	out := make(map[int][]models.ApprovalSetStatusSummary, len(setIDs))
	if len(setIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(setIDs))
	args := make([]interface{}, len(setIDs))
	for i, id := range setIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT ass.approval_set_id, ass.status_id, st.name, sc.color
		FROM approval_set_statuses ass
		JOIN statuses st ON st.id = ass.status_id
		LEFT JOIN status_categories sc ON sc.id = st.category_id
		WHERE ass.is_active = true AND ass.approval_set_id IN (%s)
		ORDER BY ass.approval_set_id, ass.id
	`, strings.Join(placeholders, ","))

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("load gated statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var setID int
		var summary models.ApprovalSetStatusSummary
		var color sql.NullString
		if err := rows.Scan(&setID, &summary.StatusID, &summary.StatusName, &color); err != nil {
			return nil, fmt.Errorf("scan gated status: %w", err)
		}
		summary.CategoryColor = color.String
		out[setID] = append(out[setID], summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate gated statuses: %w", err)
	}
	return out, nil
}

// FindActiveStatusBySetAndStatus returns the active approval_set_status row
// for a (approvalSetID, statusID) pair, or (nil, nil) if no row matches.
// is_active=1 is the partial-unique guarantee enforcing one current row.
func (r *ApprovalSetRepository) FindActiveStatusBySetAndStatus(ctx context.Context, approvalSetID, statusID int) (*models.ApprovalSetStatus, error) {
	var ass models.ApprovalSetStatus
	err := r.db.QueryRowContext(ctx, `
		SELECT id, approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode, created_at
		FROM approval_set_statuses
		WHERE approval_set_id = ? AND status_id = ? AND is_active = true
	`, approvalSetID, statusID).Scan(
		&ass.ID, &ass.ApprovalSetID, &ass.StatusID,
		&ass.ApproveTransitionID, &ass.DenyTransitionID,
		&ass.StepMode, &ass.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load approval_set_status: %w", err)
	}
	return &ass, nil
}

// FindStatusByIDInTx loads a single approval_set_status by id inside a
// transaction. Used by the runtime engine when finalizing a request — the
// snapshot row may be is_active=0, so this query does NOT filter on is_active.
func (r *ApprovalSetRepository) FindStatusByIDInTx(ctx context.Context, tx database.Tx, id int) (*models.ApprovalSetStatus, error) {
	var ass models.ApprovalSetStatus
	err := tx.QueryRowContext(ctx, `
		SELECT id, approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode, created_at
		FROM approval_set_statuses WHERE id = ?
	`, id).Scan(&ass.ID, &ass.ApprovalSetID, &ass.StatusID,
		&ass.ApproveTransitionID, &ass.DenyTransitionID, &ass.StepMode, &ass.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load approval_set_status: %w", err)
	}
	return &ass, nil
}

// FindStepsByStatusID returns approval_steps for a status, in display order.
func (r *ApprovalSetRepository) FindStepsByStatusID(ctx context.Context, approvalSetStatusID int) ([]models.ApprovalStep, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, approval_set_status_id, display_order, name,
		       quorum_mode, quorum_count, quorum_percent, rejection_policy,
		       approver_source, approver_field_identifier, approver_field_id,
		       approver_role_id, approver_group_id, approver_user_id, allow_self_approval,
		       on_leave_strategy,
		       escalation_after_hours, escalation_action, escalation_target_source,
		       escalation_target_field_identifier, escalation_target_field_id,
		       escalation_target_role_id, escalation_target_group_id, escalation_target_user_id,
		       max_escalations, created_at
		FROM approval_steps WHERE approval_set_status_id = ? ORDER BY display_order, id
	`, approvalSetStatusID)
	if err != nil {
		return nil, fmt.Errorf("query approval_steps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.ApprovalStep
	for rows.Next() {
		step, err := scanApprovalStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approval_steps: %w", err)
	}
	return out, nil
}

// FindStepByIDInTx loads a single approval_step by id inside a transaction.
// Used by the runtime engine when advancing/escalating a step.
func (r *ApprovalSetRepository) FindStepByIDInTx(ctx context.Context, tx database.Tx, id int) (*models.ApprovalStep, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, approval_set_status_id, display_order, name,
		       quorum_mode, quorum_count, quorum_percent, rejection_policy,
		       approver_source, approver_field_identifier, approver_field_id,
		       approver_role_id, approver_group_id, approver_user_id, allow_self_approval,
		       on_leave_strategy,
		       escalation_after_hours, escalation_action, escalation_target_source,
		       escalation_target_field_identifier, escalation_target_field_id,
		       escalation_target_role_id, escalation_target_group_id, escalation_target_user_id,
		       max_escalations, created_at
		FROM approval_steps WHERE id = ?
	`, id)
	step, err := scanApprovalStepRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load approval_step: %w", err)
	}
	return step, nil
}

// WorkflowExists is the existence check used by Create to validate the
// referenced workflow.
func (r *ApprovalSetRepository) WorkflowExists(ctx context.Context, workflowID int) (bool, error) {
	var ok bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workflows WHERE id = ?)`, workflowID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check workflow existence: %w", err)
	}
	return ok, nil
}

// GetWorkflowIDForSet returns the workflow_id for an approval set.
// Returns ErrNotFound if no row matches.
func (r *ApprovalSetRepository) GetWorkflowIDForSet(ctx context.Context, id int) (int, error) {
	var workflowID int
	err := r.db.QueryRowContext(ctx, `SELECT workflow_id FROM approval_sets WHERE id = ?`, id).Scan(&workflowID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("load workflow_id: %w", err)
	}
	return workflowID, nil
}

// GetSetName returns the display name of an approval set.
// Returns ErrNotFound if no row matches.
func (r *ApprovalSetRepository) GetSetName(ctx context.Context, id int) (string, error) {
	var name string
	err := r.db.QueryRowContext(ctx, `SELECT name FROM approval_sets WHERE id = ?`, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load approval_set name: %w", err)
	}
	return name, nil
}

// CountReferencingConfigSets returns the number of configuration_sets and
// configuration_set_item_types rows that point at this approval set.
// Used by Delete to refuse the request when the set is in use.
func (r *ApprovalSetRepository) CountReferencingConfigSets(ctx context.Context, id int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM (
			SELECT id FROM configuration_sets WHERE approval_set_id = ?
			UNION ALL
			SELECT id FROM configuration_set_item_types WHERE approval_set_id = ?
		)
	`, id, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count config-set references: %w", err)
	}
	return count, nil
}

// CountPendingRequestsForSet returns the number of pending approval_requests
// rows whose snapshot points at any (active or archived) status of this set.
func (r *ApprovalSetRepository) CountPendingRequestsForSet(ctx context.Context, id int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM approval_requests ar
		JOIN approval_set_statuses ass ON ass.id = ar.approval_set_status_id
		WHERE ass.approval_set_id = ? AND ar.status = 'pending'
	`, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending requests: %w", err)
	}
	return count, nil
}

// TransitionExistsFromStatus reports whether (transitionID, workflowID,
// fromStatusID) names an existing workflow_transitions row. Used to validate
// approve_transition_id / deny_transition_id wiring on Create / Update.
func (r *ApprovalSetRepository) TransitionExistsFromStatus(ctx context.Context, transitionID, workflowID, fromStatusID int) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM workflow_transitions
			WHERE id = ? AND workflow_id = ? AND from_status_id = ?
		)
	`, transitionID, workflowID, fromStatusID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check transition: %w", err)
	}
	return ok, nil
}

// FindDriversForTransition returns approval sets whose
// approve_transition_id or deny_transition_id is the queried transition.
// Powers the transition-governance endpoint's override-warning UI.
func (r *ApprovalSetRepository) FindDriversForTransition(ctx context.Context, transitionID int) ([]ApprovalSetTransitionDriver, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT aset.id, aset.name, ass.id, 'approve_transition_id' AS role
		FROM approval_set_statuses ass
		JOIN approval_sets aset ON aset.id = ass.approval_set_id
		WHERE ass.approve_transition_id = ? AND ass.is_active = true
		UNION ALL
		SELECT aset.id, aset.name, ass.id, 'deny_transition_id' AS role
		FROM approval_set_statuses ass
		JOIN approval_sets aset ON aset.id = ass.approval_set_id
		WHERE ass.deny_transition_id = ? AND ass.is_active = true
		ORDER BY 2
	`, transitionID, transitionID)
	if err != nil {
		return nil, fmt.Errorf("query approval drivers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []ApprovalSetTransitionDriver{}
	for rows.Next() {
		var d ApprovalSetTransitionDriver
		if err := rows.Scan(&d.ApprovalSetID, &d.ApprovalSetName, &d.ApprovalSetStatusID, &d.Role); err != nil {
			return nil, fmt.Errorf("scan approval driver: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate approval drivers: %w", err)
	}
	return out, nil
}

// IsWorkspacePersonal reports whether a workspace is the user's personal
// space. Personal workspaces never get an approval set.
func (r *ApprovalSetRepository) IsWorkspacePersonal(ctx context.Context, workspaceID int) (bool, error) {
	var isPersonal bool
	err := r.db.QueryRowContext(ctx, `SELECT is_personal FROM workspaces WHERE id = ?`, workspaceID).Scan(&isPersonal)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load workspace is_personal: %w", err)
	}
	return isPersonal, nil
}

// ResolveForWorkspace mirrors the resolution order in
// ApprovalService.GetApprovalSetIDForItem: item-type override on the
// workspace's bound config-set → workspace-level default → global default.
// Returns (nil, nil) when no approval set is configured.
func (r *ApprovalSetRepository) ResolveForWorkspace(ctx context.Context, workspaceID int, itemTypeID *int) (*int, error) {
	var approvalSetID *int

	if itemTypeID != nil {
		err := r.db.QueryRowContext(ctx, `
			SELECT COALESCE(csit.approval_set_id, cs.approval_set_id) AS approval_set_id
			FROM workspace_configuration_sets wcs
			JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
			LEFT JOIN configuration_set_item_types csit
				ON cs.id = csit.configuration_set_id AND csit.item_type_id = ?
			WHERE wcs.workspace_id = ?
		`, *itemTypeID, workspaceID).Scan(&approvalSetID)
		if err == nil && approvalSetID != nil {
			return approvalSetID, nil
		}
	}

	err := r.db.QueryRowContext(ctx, `
		SELECT cs.approval_set_id
		FROM workspace_configuration_sets wcs
		JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
		WHERE wcs.workspace_id = ?
	`, workspaceID).Scan(&approvalSetID)
	if err == nil && approvalSetID != nil {
		return approvalSetID, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("load workspace config-set approval: %w", err)
	}

	if itemTypeID != nil {
		err = r.db.QueryRowContext(ctx, `
			SELECT COALESCE(csit.approval_set_id, cs.approval_set_id) AS approval_set_id
			FROM configuration_sets cs
			LEFT JOIN configuration_set_item_types csit
				ON cs.id = csit.configuration_set_id AND csit.item_type_id = ?
			WHERE cs.is_default = true
		`, *itemTypeID).Scan(&approvalSetID)
		if err == nil && approvalSetID != nil {
			return approvalSetID, nil
		}
	}

	err = r.db.QueryRowContext(ctx, `SELECT approval_set_id FROM configuration_sets WHERE is_default = true`).Scan(&approvalSetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("load global default approval set: %w", err)
	}
	return approvalSetID, nil
}

// ============================================================================
// Writes (all on tx)
// ============================================================================

// CreateSet inserts a new approval_sets row and returns its id.
func (r *ApprovalSetRepository) CreateSet(ctx context.Context, tx database.Tx, set *models.ApprovalSet) (int, error) {
	now := time.Now()
	set.CreatedAt = now
	set.UpdatedAt = now
	var id64 int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO approval_sets (name, description, workflow_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id
	`, set.Name, set.Description, set.WorkflowID, now, now).Scan(&id64); err != nil {
		return 0, fmt.Errorf("insert approval_set: %w", err)
	}
	return int(id64), nil
}

// UpdateSet updates the name/description/updated_at of an existing approval set.
func (r *ApprovalSetRepository) UpdateSet(ctx context.Context, tx database.Tx, id int, name, description string) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE approval_sets SET name = ?, description = ?, updated_at = ? WHERE id = ?
	`, name, description, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update approval_set: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSet removes an approval set by id. Caller is responsible for the
// "in use" / "pending requests" pre-checks.
func (r *ApprovalSetRepository) DeleteSet(ctx context.Context, tx database.Tx, id int) error {
	res, err := tx.ExecContext(ctx, `DELETE FROM approval_sets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete approval_set: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUnreferencedStatuses removes approval_set_statuses rows for this set
// that no in-flight approval_request points at. Used during Update to keep the
// table tidy without breaking the FK on snapshot rows.
func (r *ApprovalSetRepository) DeleteUnreferencedStatuses(ctx context.Context, tx database.Tx, approvalSetID int) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM approval_set_statuses
		WHERE approval_set_id = ?
		  AND id NOT IN (SELECT DISTINCT approval_set_status_id FROM approval_requests)
	`, approvalSetID); err != nil {
		return fmt.Errorf("delete unreferenced statuses: %w", err)
	}
	return nil
}

// DeactivateActiveStatuses flips is_active=1 rows to is_active=0 for the
// given approval set. Soft-archive: keeps the snapshot for in-flight requests.
func (r *ApprovalSetRepository) DeactivateActiveStatuses(ctx context.Context, tx database.Tx, approvalSetID int) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE approval_set_statuses SET is_active = false
		WHERE approval_set_id = ? AND is_active = true
	`, approvalSetID); err != nil {
		return fmt.Errorf("deactivate statuses: %w", err)
	}
	return nil
}

// CreateStatus inserts a new approval_set_statuses row and returns its id.
// Caller picks step_mode (defaults to sequential at the SQL DEFAULT level if
// blank — but the service layer resolves it explicitly to keep ass.StepMode
// consistent in the response model).
func (r *ApprovalSetRepository) CreateStatus(ctx context.Context, tx database.Tx, ass *models.ApprovalSetStatus) (int, error) {
	stepMode := ass.StepMode
	if stepMode == "" {
		stepMode = models.ApprovalStepModeSequential
	}
	var id64 int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO approval_set_statuses
			(approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		RETURNING id
	`, ass.ApprovalSetID, ass.StatusID, ass.ApproveTransitionID, ass.DenyTransitionID, stepMode, time.Now()).Scan(&id64); err != nil {
		return 0, fmt.Errorf("insert approval_set_status: %w", err)
	}
	return int(id64), nil
}

// CreateStep inserts a new approval_steps row under the given
// approval_set_status. Defaults are normalized server-side.
func (r *ApprovalSetRepository) CreateStep(ctx context.Context, tx database.Tx, approvalSetStatusID int, step models.ApprovalStep) error {
	quorumMode := step.QuorumMode
	if quorumMode == "" {
		quorumMode = models.ApprovalQuorumModeAny
	}
	rejectionPolicy := step.RejectionPolicy
	if rejectionPolicy == "" {
		rejectionPolicy = models.ApprovalRejectionPolicyAnyFails
	}
	onLeave := step.OnLeaveStrategy
	if onLeave == "" {
		onLeave = models.ApprovalOnLeaveUseSubstitute
	}
	allowSelf := 0
	if step.AllowSelfApproval {
		allowSelf = 1
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO approval_steps
			(approval_set_status_id, display_order, name,
			 quorum_mode, quorum_count, quorum_percent, rejection_policy,
			 approver_source, approver_field_identifier, approver_field_id,
			 approver_role_id, approver_group_id, approver_user_id, allow_self_approval,
			 on_leave_strategy,
			 escalation_after_hours, escalation_action, escalation_target_source,
			 escalation_target_field_identifier, escalation_target_field_id,
			 escalation_target_role_id, escalation_target_group_id, escalation_target_user_id,
			 max_escalations, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		approvalSetStatusID, step.DisplayOrder, step.Name,
		quorumMode, step.QuorumCount, step.QuorumPercent, rejectionPolicy,
		step.ApproverSource, nullStringIfEmpty(step.ApproverFieldIdentifier), step.ApproverFieldID,
		step.ApproverRoleID, step.ApproverGroupID, step.ApproverUserID, allowSelf,
		onLeave,
		step.EscalationAfterHours, nullStringIfEmpty(step.EscalationAction), nullStringIfEmpty(step.EscalationTargetSource),
		nullStringIfEmpty(step.EscalationTargetFieldIdentifier), step.EscalationTargetFieldID,
		step.EscalationTargetRoleID, step.EscalationTargetGroupID, step.EscalationTargetUserID,
		step.MaxEscalations, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("insert approval_step: %w", err)
	}
	return nil
}

// ============================================================================
// Helpers
// ============================================================================

func scanApprovalSetRows(rows *sql.Rows) ([]models.ApprovalSet, error) {
	var out []models.ApprovalSet
	for rows.Next() {
		var s models.ApprovalSet
		var description sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &description, &s.WorkflowID,
			&s.CreatedAt, &s.UpdatedAt, &s.WorkflowName); err != nil {
			return nil, fmt.Errorf("scan approval_set: %w", err)
		}
		s.Description = description.String
		out = append(out, s)
	}
	return out, nil
}

// approvalStepScanner is satisfied by both *sql.Row and *sql.Rows so the
// shared scanner can populate an ApprovalStep from either source.
type approvalStepScanner interface {
	Scan(dest ...interface{}) error
}

func scanApprovalStepCols(sc approvalStepScanner) (*models.ApprovalStep, error) {
	var step models.ApprovalStep
	var fieldIdent, action, escTargSrc, escFieldIdent sql.NullString
	// allow_self_approval is BOOLEAN on Postgres / INTEGER (0/1) on SQLite.
	// Scanning into a bool works on both: lib/pq returns the bool natively,
	// and SQLite's go-sqlite3 maps 0/1 to false/true via the database/sql
	// driver.Value path.
	if err := sc.Scan(
		&step.ID, &step.ApprovalSetStatusID, &step.DisplayOrder, &step.Name,
		&step.QuorumMode, &step.QuorumCount, &step.QuorumPercent, &step.RejectionPolicy,
		&step.ApproverSource, &fieldIdent, &step.ApproverFieldID,
		&step.ApproverRoleID, &step.ApproverGroupID, &step.ApproverUserID, &step.AllowSelfApproval,
		&step.OnLeaveStrategy,
		&step.EscalationAfterHours, &action, &escTargSrc,
		&escFieldIdent, &step.EscalationTargetFieldID,
		&step.EscalationTargetRoleID, &step.EscalationTargetGroupID, &step.EscalationTargetUserID,
		&step.MaxEscalations, &step.CreatedAt,
	); err != nil {
		return nil, err
	}
	step.ApproverFieldIdentifier = fieldIdent.String
	step.EscalationAction = action.String
	step.EscalationTargetSource = escTargSrc.String
	step.EscalationTargetFieldIdentifier = escFieldIdent.String
	return &step, nil
}

func scanApprovalStep(rows *sql.Rows) (*models.ApprovalStep, error) {
	return scanApprovalStepCols(rows)
}

func scanApprovalStepRow(row *sql.Row) (*models.ApprovalStep, error) {
	return scanApprovalStepCols(row)
}

// nullStringIfEmpty returns nil for empty strings so the column scans/inserts
// as SQL NULL rather than the empty string.
func nullStringIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
