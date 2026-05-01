package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
)

// ApprovalSetHandler handles CRUD for approval sets — the asynchronous sibling
// of ConditionSetHandler. Approval sets are templates: a set is owned by a
// workflow and contains approval_set_statuses (one per status that fires an
// approval) and approval_steps (the sequential or parallel approver steps).
type ApprovalSetHandler struct {
	db database.Database
}

// NewApprovalSetHandler constructs an ApprovalSetHandler.
func NewApprovalSetHandler(db database.Database) *ApprovalSetHandler {
	return &ApprovalSetHandler{db: db}
}

// GetAll returns all approval sets, optionally filtered by workflow_id.
func (h *ApprovalSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	query := `
		SELECT a.id, a.name, a.description, a.workflow_id, a.created_at, a.updated_at,
		       w.name as workflow_name
		FROM approval_sets a
		JOIN workflows w ON a.workflow_id = w.id`

	var args []interface{}
	if workflowIDStr := r.URL.Query().Get("workflow_id"); workflowIDStr != "" {
		workflowID, err := strconv.Atoi(workflowIDStr)
		if err != nil {
			respondValidationError(w, r, "Invalid workflow_id")
			return
		}
		query += " WHERE a.workflow_id = ?"
		args = append(args, workflowID)
	}
	query += " ORDER BY a.name"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	out, err := scanApprovalSets(rows)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if out == nil {
		out = []models.ApprovalSet{}
	}
	respondJSONOK(w, out)
}

// Get returns a single approval set with its approval_set_statuses and steps.
func (h *ApprovalSetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	set, err := h.loadApprovalSet(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "Approval set")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, set)
}

// GetByWorkflow returns approval sets for a specific workflow.
func (h *ApprovalSetHandler) GetByWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT a.id, a.name, a.description, a.workflow_id, a.created_at, a.updated_at,
		       w.name as workflow_name
		FROM approval_sets a
		JOIN workflows w ON a.workflow_id = w.id
		WHERE a.workflow_id = ?
		ORDER BY a.name
	`, workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	out, err := scanApprovalSets(rows)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if out == nil {
		out = []models.ApprovalSet{}
	}
	respondJSONOK(w, out)
}

// Create creates a new approval set with nested set-statuses and steps.
func (h *ApprovalSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, input, ok := h.decodeForEdit(w, r)
	if !ok {
		return
	}

	if input.WorkflowID == 0 {
		respondValidationError(w, r, "workflow_id is required")
		return
	}
	var workflowExists bool
	if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workflows WHERE id = ?)", input.WorkflowID).Scan(&workflowExists); err != nil || !workflowExists {
		respondValidationError(w, r, "Workflow not found")
		return
	}

	if err := h.validateSetStatuses(input.WorkflowID, input.SetStatuses); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now()
	res, err := tx.Exec(`
		INSERT INTO approval_sets (name, description, workflow_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, input.Name, input.Description, input.WorkflowID, now, now)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	id64, _ := res.LastInsertId()
	id := int(id64)

	if err := h.saveSetStatuses(tx, id, input.SetStatuses); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, user, logger.ActionApprovalSetCreate, logger.ResourceApprovalSet, &id, input.Name)

	out, err := h.loadApprovalSet(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONCreated(w, out)
}

// Update replaces an approval set's name/description and its nested
// set-statuses + steps. workflow_id is immutable.
func (h *ApprovalSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	user, input, ok := h.decodeForEdit(w, r)
	if !ok {
		return
	}

	var existingWorkflowID int
	err := h.db.QueryRow("SELECT workflow_id FROM approval_sets WHERE id = ?", id).Scan(&existingWorkflowID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "Approval set")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	if input.WorkflowID != 0 && input.WorkflowID != existingWorkflowID {
		respondValidationError(w, r, "Cannot change workflow_id of an existing approval set")
		return
	}

	if err := h.validateSetStatuses(existingWorkflowID, input.SetStatuses); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Refuse update if any pending requests reference this set's statuses —
	// changing transitions or step config mid-flight would invalidate snapshots.
	var openCount int
	if err := h.db.QueryRow(`
		SELECT COUNT(*) FROM approval_requests ar
		JOIN approval_set_statuses ass ON ass.id = ar.approval_set_status_id
		WHERE ass.approval_set_id = ? AND ar.status = 'pending'
	`, id).Scan(&openCount); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if openCount > 0 {
		respondConflict(w, r, fmt.Sprintf("Cannot update approval set: %d pending approval request(s) reference it", openCount))
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE approval_sets SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		input.Name, input.Description, time.Now(), id); err != nil {
		respondInternalError(w, r, err)
		return
	}
	// Cascade-delete and re-insert (no in-flight requests here, see openCount check).
	if _, err := tx.Exec(`DELETE FROM approval_set_statuses WHERE approval_set_id = ?`, id); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if err := h.saveSetStatuses(tx, id, input.SetStatuses); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, user, logger.ActionApprovalSetUpdate, logger.ResourceApprovalSet, &id, input.Name)

	out, err := h.loadApprovalSet(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, out)
}

// Delete deletes an approval set. Refuses if it's referenced by any
// configuration_set or has any non-canceled requests.
func (h *ApprovalSetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var name string
	if err := h.db.QueryRow("SELECT name FROM approval_sets WHERE id = ?", id).Scan(&name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "Approval set")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	var inUseCount int
	if err := h.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT id FROM configuration_sets WHERE approval_set_id = ?
			UNION ALL
			SELECT id FROM configuration_set_item_types WHERE approval_set_id = ?
		)
	`, id, id).Scan(&inUseCount); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if inUseCount > 0 {
		respondValidationError(w, r, "Cannot delete approval set: it is in use by one or more configuration sets")
		return
	}

	var openCount int
	if err := h.db.QueryRow(`
		SELECT COUNT(*) FROM approval_requests ar
		JOIN approval_set_statuses ass ON ass.id = ar.approval_set_status_id
		WHERE ass.approval_set_id = ? AND ar.status = 'pending'
	`, id).Scan(&openCount); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if openCount > 0 {
		respondConflict(w, r, fmt.Sprintf("Cannot delete approval set: %d pending approval request(s) still reference it", openCount))
		return
	}

	if _, err := h.db.Exec("DELETE FROM approval_sets WHERE id = ?", id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, user, logger.ActionApprovalSetDelete, logger.ResourceApprovalSet, &id, name)
	w.WriteHeader(http.StatusNoContent)
}

// --- internals ---

func scanApprovalSets(rows *sql.Rows) ([]models.ApprovalSet, error) {
	var out []models.ApprovalSet
	for rows.Next() {
		var s models.ApprovalSet
		var description sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &description, &s.WorkflowID,
			&s.CreatedAt, &s.UpdatedAt, &s.WorkflowName); err != nil {
			return nil, err
		}
		s.Description = description.String
		out = append(out, s)
	}
	return out, nil
}

func (h *ApprovalSetHandler) decodeForEdit(w http.ResponseWriter, r *http.Request) (*models.User, models.ApprovalSet, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, models.ApprovalSet{}, false
	}
	var input models.ApprovalSet
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid request body")
		return nil, models.ApprovalSet{}, false
	}
	if input.Name == "" {
		respondValidationError(w, r, "Name is required")
		return nil, models.ApprovalSet{}, false
	}
	return user, input, true
}

func (h *ApprovalSetHandler) loadApprovalSet(id int) (*models.ApprovalSet, error) {
	var s models.ApprovalSet
	var description sql.NullString
	err := h.db.QueryRow(`
		SELECT a.id, a.name, a.description, a.workflow_id, a.created_at, a.updated_at,
		       w.name as workflow_name
		FROM approval_sets a
		JOIN workflows w ON a.workflow_id = w.id
		WHERE a.id = ?
	`, id).Scan(&s.ID, &s.Name, &description, &s.WorkflowID,
		&s.CreatedAt, &s.UpdatedAt, &s.WorkflowName)
	if err != nil {
		return nil, err
	}
	s.Description = description.String

	statusRows, err := h.db.Query(`
		SELECT ass.id, ass.approval_set_id, ass.status_id, ass.approve_transition_id,
		       ass.deny_transition_id, ass.step_mode, ass.created_at,
		       st.name as status_name
		FROM approval_set_statuses ass
		JOIN statuses st ON st.id = ass.status_id
		WHERE ass.approval_set_id = ?
		ORDER BY ass.id
	`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = statusRows.Close() }()

	var setStatuses []models.ApprovalSetStatus
	for statusRows.Next() {
		var ass models.ApprovalSetStatus
		var statusName sql.NullString
		if err := statusRows.Scan(&ass.ID, &ass.ApprovalSetID, &ass.StatusID,
			&ass.ApproveTransitionID, &ass.DenyTransitionID,
			&ass.StepMode, &ass.CreatedAt, &statusName); err != nil {
			return nil, err
		}
		ass.StatusName = statusName.String
		setStatuses = append(setStatuses, ass)
	}

	for i := range setStatuses {
		stepRows, err := h.db.Query(`
			SELECT id, approval_set_status_id, display_order, name,
			       quorum_mode, quorum_count, quorum_percent, rejection_policy,
			       approver_source, approver_field_identifier, approver_field_id,
			       approver_role_id, approver_group_id, approver_user_id, allow_self_approval,
			       on_leave_strategy,
			       escalation_after_hours, escalation_action, escalation_target_source,
			       escalation_target_field_identifier, escalation_target_field_id,
			       escalation_target_role_id, escalation_target_group_id, escalation_target_user_id,
			       max_escalations, created_at
			FROM approval_steps WHERE approval_set_status_id = ?
			ORDER BY display_order, id
		`, setStatuses[i].ID)
		if err != nil {
			return nil, err
		}
		var steps []models.ApprovalStep
		for stepRows.Next() {
			var step models.ApprovalStep
			var allowSelf int
			var fieldIdent, action, escTargSrc, escFieldIdent sql.NullString
			if err := stepRows.Scan(
				&step.ID, &step.ApprovalSetStatusID, &step.DisplayOrder, &step.Name,
				&step.QuorumMode, &step.QuorumCount, &step.QuorumPercent, &step.RejectionPolicy,
				&step.ApproverSource, &fieldIdent, &step.ApproverFieldID,
				&step.ApproverRoleID, &step.ApproverGroupID, &step.ApproverUserID, &allowSelf,
				&step.OnLeaveStrategy,
				&step.EscalationAfterHours, &action, &escTargSrc,
				&escFieldIdent, &step.EscalationTargetFieldID,
				&step.EscalationTargetRoleID, &step.EscalationTargetGroupID, &step.EscalationTargetUserID,
				&step.MaxEscalations, &step.CreatedAt,
			); err != nil {
				_ = stepRows.Close()
				return nil, fmt.Errorf("scan approval step: %w", err)
			}
			step.AllowSelfApproval = allowSelf != 0
			step.ApproverFieldIdentifier = fieldIdent.String
			step.EscalationAction = action.String
			step.EscalationTargetSource = escTargSrc.String
			step.EscalationTargetFieldIdentifier = escFieldIdent.String
			steps = append(steps, step)
		}
		_ = stepRows.Close()
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

// validateSetStatuses checks template-level invariants before write.
func (h *ApprovalSetHandler) validateSetStatuses(workflowID int, setStatuses []models.ApprovalSetStatus) error {
	seenStatus := make(map[int]bool, len(setStatuses))
	for _, ass := range setStatuses {
		if ass.StatusID == 0 {
			return &validationErr{msg: "status_id is required for each approval_set_status"}
		}
		if seenStatus[ass.StatusID] {
			return &validationErr{msg: fmt.Sprintf("duplicate status_id %d in approval set", ass.StatusID)}
		}
		seenStatus[ass.StatusID] = true

		if ass.ApproveTransitionID == 0 || ass.DenyTransitionID == 0 {
			return &validationErr{msg: "approve_transition_id and deny_transition_id are required"}
		}
		if ass.ApproveTransitionID == ass.DenyTransitionID {
			return &validationErr{msg: "approve and deny transitions must differ"}
		}
		if ass.StepMode != models.ApprovalStepModeSequential && ass.StepMode != models.ApprovalStepModeParallel {
			return &validationErr{msg: "step_mode must be 'sequential' or 'parallel'"}
		}

		// Both transitions must belong to the workflow and originate at this status.
		if err := h.checkTransitionFromStatus(ass.ApproveTransitionID, workflowID, ass.StatusID, "approve_transition_id"); err != nil {
			return err
		}
		if err := h.checkTransitionFromStatus(ass.DenyTransitionID, workflowID, ass.StatusID, "deny_transition_id"); err != nil {
			return err
		}

		if len(ass.Steps) == 0 {
			return &validationErr{msg: "an approval_set_status must have at least one step"}
		}
		for _, step := range ass.Steps {
			if err := validateApprovalStep(step); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *ApprovalSetHandler) checkTransitionFromStatus(transitionID, workflowID, fromStatusID int, fieldName string) error {
	var ok bool
	if err := h.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM workflow_transitions
			WHERE id = ? AND workflow_id = ? AND from_status_id = ?
		)
	`, transitionID, workflowID, fromStatusID).Scan(&ok); err != nil || !ok {
		return &validationErr{msg: fmt.Sprintf("%s does not exist on this workflow as a transition out of the configured status", fieldName)}
	}
	return nil
}

func validateApprovalStep(step models.ApprovalStep) error {
	if step.Name == "" {
		return &validationErr{msg: "each step must have a name"}
	}
	switch step.QuorumMode {
	case models.ApprovalQuorumModeAny, models.ApprovalQuorumModeAll:
		// ok
	case models.ApprovalQuorumModeCount:
		if step.QuorumCount == nil || *step.QuorumCount < 1 {
			return &validationErr{msg: "quorum_mode 'count' requires quorum_count >= 1"}
		}
	case models.ApprovalQuorumModePercent:
		if step.QuorumPercent == nil || *step.QuorumPercent < 1 || *step.QuorumPercent > 100 {
			return &validationErr{msg: "quorum_mode 'percent' requires quorum_percent in [1,100]"}
		}
	default:
		return &validationErr{msg: "quorum_mode must be one of any|all|count|percent"}
	}

	switch step.RejectionPolicy {
	case "", models.ApprovalRejectionPolicyAnyFails, models.ApprovalRejectionPolicyQuorumRequired:
		// ok
	default:
		return &validationErr{msg: "rejection_policy must be 'any_rejection_fails' or 'requires_quorum_to_fail'"}
	}

	switch step.OnLeaveStrategy {
	case "", models.ApprovalOnLeaveUseSubstitute, models.ApprovalOnLeaveSkip, models.ApprovalOnLeaveKeep:
		// ok
	default:
		return &validationErr{msg: "on_leave_strategy must be 'use_substitute', 'skip', or 'keep'"}
	}

	if step.EscalationAction != "" {
		switch step.EscalationAction {
		case models.ApprovalEscalationActionReassign, models.ApprovalEscalationActionSkipStep, models.ApprovalEscalationActionAutoReject:
		default:
			return &validationErr{msg: "escalation_action must be 'reassign', 'skip_step', or 'auto_reject'"}
		}
	}

	switch step.ApproverSource {
	case models.ApprovalSourceCreator, models.ApprovalSourceAssignee, models.ApprovalSourceCurrentUser:
		// no extra fields required
	case models.ApprovalSourceUser:
		if step.ApproverUserID == nil || *step.ApproverUserID == 0 {
			return &validationErr{msg: "approver_source 'user' requires approver_user_id"}
		}
	case models.ApprovalSourceRegularField:
		if _, ok := models.AllowedRegularApproverFields[step.ApproverFieldIdentifier]; !ok {
			return &validationErr{msg: fmt.Sprintf("approver_field_identifier %q is not in the regular-field whitelist", step.ApproverFieldIdentifier)}
		}
	case models.ApprovalSourceCustomField:
		if step.ApproverFieldID == nil || *step.ApproverFieldID == 0 {
			return &validationErr{msg: "approver_source 'custom_field' requires approver_field_id"}
		}
	case models.ApprovalSourceRole:
		if step.ApproverRoleID == nil || *step.ApproverRoleID == 0 {
			return &validationErr{msg: "approver_source 'role' requires approver_role_id"}
		}
	case models.ApprovalSourceGroup:
		if step.ApproverGroupID == nil || *step.ApproverGroupID == 0 {
			return &validationErr{msg: "approver_source 'group' requires approver_group_id"}
		}
	default:
		return &validationErr{msg: "approver_source must be one of creator|assignee|current_user|user|regular_field|custom_field|role|group"}
	}

	if step.EscalationTargetSource != "" {
		// Validate target source the same way (subset of valid sources). Reuse a
		// scratch step to share the switch; we don't care about the action here.
		probe := step
		probe.ApproverSource = step.EscalationTargetSource
		probe.ApproverFieldIdentifier = step.EscalationTargetFieldIdentifier
		probe.ApproverFieldID = step.EscalationTargetFieldID
		probe.ApproverRoleID = step.EscalationTargetRoleID
		probe.ApproverGroupID = step.EscalationTargetGroupID
		probe.ApproverUserID = step.EscalationTargetUserID
		switch probe.ApproverSource {
		case models.ApprovalSourceCreator, models.ApprovalSourceAssignee, models.ApprovalSourceCurrentUser:
		case models.ApprovalSourceUser:
			if probe.ApproverUserID == nil || *probe.ApproverUserID == 0 {
				return &validationErr{msg: "escalation_target_source 'user' requires escalation_target_user_id"}
			}
		case models.ApprovalSourceRegularField:
			if _, ok := models.AllowedRegularApproverFields[probe.ApproverFieldIdentifier]; !ok {
				return &validationErr{msg: "escalation_target_field_identifier is not in the regular-field whitelist"}
			}
		case models.ApprovalSourceCustomField:
			if probe.ApproverFieldID == nil || *probe.ApproverFieldID == 0 {
				return &validationErr{msg: "escalation_target_source 'custom_field' requires escalation_target_field_id"}
			}
		case models.ApprovalSourceRole:
			if probe.ApproverRoleID == nil || *probe.ApproverRoleID == 0 {
				return &validationErr{msg: "escalation_target_source 'role' requires escalation_target_role_id"}
			}
		case models.ApprovalSourceGroup:
			if probe.ApproverGroupID == nil || *probe.ApproverGroupID == 0 {
				return &validationErr{msg: "escalation_target_source 'group' requires escalation_target_group_id"}
			}
		default:
			return &validationErr{msg: "escalation_target_source must be a valid source vocabulary value"}
		}
	}
	return nil
}

func (h *ApprovalSetHandler) saveSetStatuses(tx database.Tx, approvalSetID int, setStatuses []models.ApprovalSetStatus) error {
	for _, ass := range setStatuses {
		stepMode := ass.StepMode
		if stepMode == "" {
			stepMode = models.ApprovalStepModeSequential
		}
		res, err := tx.Exec(`
			INSERT INTO approval_set_statuses
				(approval_set_id, status_id, approve_transition_id, deny_transition_id, step_mode, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, approvalSetID, ass.StatusID, ass.ApproveTransitionID, ass.DenyTransitionID, stepMode, time.Now())
		if err != nil {
			return err
		}
		assID64, _ := res.LastInsertId()
		assID := int(assID64)

		for _, step := range ass.Steps {
			if err := insertApprovalStep(tx, assID, step); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertApprovalStep(tx database.Tx, approvalSetStatusID int, step models.ApprovalStep) error {
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
	_, err := tx.Exec(`
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
	return err
}

// nullStringIfEmpty returns nil for empty strings so the column scans/inserts
// as SQL NULL rather than the empty string. (Distinct from scm_providers.go's
// nullString which has a different signature/intent.)
func nullStringIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
