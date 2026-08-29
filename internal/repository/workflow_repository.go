package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// Validation errors surfaced by ReplaceTransitions. Handlers map these to
// 400 responses; any other error is an internal failure.
var (
	// ErrTransitionToStatusRequired indicates a payload transition without a to-status id.
	ErrTransitionToStatusRequired = errors.New("to status ID is required for all transitions")
	// ErrTransitionToStatusNotFound indicates a payload transition referencing a missing to-status.
	ErrTransitionToStatusNotFound = errors.New("to status not found")
	// ErrTransitionFromStatusNotFound indicates a payload transition referencing a missing from-status.
	ErrTransitionFromStatusNotFound = errors.New("from status not found")
)

// WorkflowRepository owns SQL access to workflows and workflow_transitions
// for the legacy admin workflow editor.
type WorkflowRepository struct {
	db database.Database
}

// NewWorkflowRepository creates a WorkflowRepository.
func NewWorkflowRepository(db database.Database) *WorkflowRepository {
	return &WorkflowRepository{db: db}
}

// List returns all workflows ordered default-first, then by name.
// The result is never nil.
func (r *WorkflowRepository) List() ([]models.Workflow, error) {
	rows, err := r.db.Query(`
		SELECT id, COALESCE(builtin_key, ''), name, description, is_default, created_at, updated_at
		FROM workflows
		ORDER BY is_default DESC, name ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var workflows []models.Workflow
	for rows.Next() {
		var workflow models.Workflow
		if err := rows.Scan(&workflow.ID, &workflow.BuiltinKey, &workflow.Name, &workflow.Description,
			&workflow.IsDefault, &workflow.CreatedAt, &workflow.UpdatedAt); err != nil {
			return nil, err
		}
		workflows = append(workflows, workflow)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Always return an array, even if empty
	if workflows == nil {
		workflows = []models.Workflow{}
	}
	return workflows, nil
}

// ListByIDs returns workflows for the supplied IDs ordered by name.
func (r *WorkflowRepository) ListByIDs(ids []int) ([]models.Workflow, error) {
	if len(ids) == 0 {
		return []models.Workflow{}, nil
	}
	placeholders, args := inPlaceholders(ids)
	rows, err := r.db.Query(`
		SELECT id, COALESCE(builtin_key, ''), name, description, is_default, created_at, updated_at
		FROM workflows
		WHERE id IN (`+placeholders+`)
		ORDER BY name
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list workflows by id: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []models.Workflow{}
	for rows.Next() {
		var workflow models.Workflow
		if err := rows.Scan(&workflow.ID, &workflow.BuiltinKey, &workflow.Name, &workflow.Description,
			&workflow.IsDefault, &workflow.CreatedAt, &workflow.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan workflow by id: %w", err)
		}
		out = append(out, workflow)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflows by id: %w", err)
	}
	return out, nil
}

// Get returns a workflow by id, without its transitions.
// Returns ErrNotFound when no workflow with that id exists.
func (r *WorkflowRepository) Get(id int) (*models.Workflow, error) {
	var workflow models.Workflow
	err := r.db.QueryRow(`
		SELECT id, COALESCE(builtin_key, ''), name, description, is_default, created_at, updated_at
		FROM workflows
		WHERE id = ?
	`, id).Scan(&workflow.ID, &workflow.BuiltinKey, &workflow.Name, &workflow.Description,
		&workflow.IsDefault, &workflow.CreatedAt, &workflow.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &workflow, nil
}

// NameExists reports whether any workflow already uses the given name.
func (r *WorkflowRepository) NameExists(name string) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workflows WHERE name = ?)", name).Scan(&exists)
	return exists, err
}

// NameExistsExcluding reports whether a workflow other than excludeID uses the given name.
func (r *WorkflowRepository) NameExistsExcluding(name string, excludeID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workflows WHERE name = ? AND id != ?)", name, excludeID).Scan(&exists)
	return exists, err
}

// Create inserts a new workflow and returns its id.
func (r *WorkflowRepository) Create(name, description string, isDefault bool) (int, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO workflows (name, description, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, name, description, isDefault, now, now).Scan(&id)
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

// CreateImported creates a workflow and its complete imported transition graph.
func (r *WorkflowRepository) CreateImported(
	name string,
	statusIDs []int,
	fromAnywhere map[int]bool,
) (int, error) {
	workflowID, err := r.Create(name, "", false)
	if err != nil {
		return 0, err
	}
	order := 0
	transitions := make([]models.WorkflowTransition, 0, len(statusIDs)*len(statusIDs))
	for _, statusID := range statusIDs {
		if fromAnywhere[statusID] {
			order++
			transitions = append(transitions, models.WorkflowTransition{
				ToStatusID:   statusID,
				DisplayOrder: order,
			})
		}
	}
	for _, fromID := range statusIDs {
		for _, toID := range statusIDs {
			if fromID == toID {
				continue
			}
			order++
			sourceID := fromID
			transitions = append(transitions, models.WorkflowTransition{
				FromStatusID: &sourceID,
				ToStatusID:   toID,
				DisplayOrder: order,
			})
		}
	}
	if _, err := r.ReplaceTransitions(workflowID, transitions); err != nil {
		_, _ = r.Delete(workflowID)
		return 0, err
	}
	return workflowID, nil
}

// Update rewrites a workflow's mutable fields.
func (r *WorkflowRepository) Update(id int, name, description string, isDefault bool) error {
	_, err := r.db.ExecWrite(`
		UPDATE workflows
		SET name = ?, description = ?, is_default = ?, updated_at = ?
		WHERE id = ?
	`, name, description, isDefault, time.Now(), id)
	return err
}

// ConfigurationSetCount returns how many configuration sets reference the workflow.
func (r *WorkflowRepository) ConfigurationSetCount(workflowID int) (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM configuration_sets WHERE workflow_id = ?", workflowID).Scan(&count)
	return count, err
}

// Delete removes a workflow and its transitions atomically. Approval requests
// pinned to the workflow's transitions are hard-deleted first (see
// CancelApprovalRequestsForTransitions); their ids are returned so the caller
// can record an audit trail entry.
func (r *WorkflowRepository) Delete(id int) (cancelledApprovalIDs []int, err error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Collect every transition id on this workflow so we can cancel any
	// approval_requests pinned to them before the CASCADE-delete chain trips
	// the RESTRICT-FK from approval_requests → approval_set_statuses.
	transitionIDs := []int{}
	{
		rows, qErr := tx.Query("SELECT id FROM workflow_transitions WHERE workflow_id = ?", id)
		if qErr != nil {
			return nil, qErr
		}
		for rows.Next() {
			var tid int
			if sErr := rows.Scan(&tid); sErr != nil {
				_ = rows.Close()
				return nil, sErr
			}
			transitionIDs = append(transitionIDs, tid)
		}
		if rerr := rows.Err(); rerr != nil {
			_ = rows.Close()
			return nil, rerr
		}
		_ = rows.Close()
	}

	cancelledApprovalIDs, err = CancelApprovalRequestsForTransitions(tx, transitionIDs)
	if err != nil {
		return nil, err
	}

	// Delete workflow transitions first
	if _, err = tx.Exec("DELETE FROM workflow_transitions WHERE workflow_id = ?", id); err != nil {
		return nil, err
	}

	// Delete the workflow
	if _, err = tx.Exec("DELETE FROM workflows WHERE id = ?", id); err != nil {
		return nil, err
	}

	if cErr := tx.Commit(); cErr != nil {
		return nil, cErr
	}
	return cancelledApprovalIDs, nil
}

// ListTransitions returns a workflow's transitions with joined status and
// workflow names. The result is never nil.
func (r *WorkflowRepository) ListTransitions(workflowID int) ([]models.WorkflowTransition, error) {
	query := `
		SELECT wt.id, wt.workflow_id, wt.from_status_id, wt.to_status_id, wt.from_all_statuses, wt.display_order, wt.source_handle, wt.target_handle, wt.created_at,
		       fs.name as from_status_name, ts.name as to_status_name, w.name as workflow_name
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		JOIN workflows w ON wt.workflow_id = w.id
		WHERE wt.workflow_id = ?
		ORDER BY CASE WHEN wt.from_all_statuses THEN 1 ELSE 0 END, wt.from_status_id NULLS FIRST, wt.display_order ASC`

	rows, err := r.db.Query(query, workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanWorkflowTransitions(rows)
}

// ListAllTransitions returns every workflow transition in one bounded query
// for enriched workflow list responses.
func (r *WorkflowRepository) ListAllTransitions() ([]models.WorkflowTransition, error) {
	rows, err := r.db.Query(`
		SELECT wt.id, wt.workflow_id, wt.from_status_id, wt.to_status_id, wt.from_all_statuses, wt.display_order, wt.source_handle, wt.target_handle, wt.created_at,
		       fs.name as from_status_name, ts.name as to_status_name, w.name as workflow_name
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		JOIN workflows w ON wt.workflow_id = w.id
		ORDER BY wt.workflow_id, CASE WHEN wt.from_all_statuses THEN 1 ELSE 0 END, wt.from_status_id NULLS FIRST, wt.display_order ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanWorkflowTransitions(rows)
}

func scanWorkflowTransitions(rows *sql.Rows) ([]models.WorkflowTransition, error) {
	var transitions []models.WorkflowTransition
	for rows.Next() {
		var transition models.WorkflowTransition
		var fromStatusID sql.NullInt64
		var fromStatusName sql.NullString
		var sourceHandle sql.NullString
		var targetHandle sql.NullString

		err := rows.Scan(&transition.ID, &transition.WorkflowID, &fromStatusID, &transition.ToStatusID,
			&transition.FromAllStatuses, &transition.DisplayOrder, &sourceHandle, &targetHandle, &transition.CreatedAt, &fromStatusName,
			&transition.ToStatusName, &transition.WorkflowName)
		if err != nil {
			return nil, err
		}

		// Handle nullable from status fields
		if fromStatusID.Valid {
			val := int(fromStatusID.Int64)
			transition.FromStatusID = &val
		}
		if fromStatusName.Valid {
			transition.FromStatusName = fromStatusName.String
		}

		// Handle nullable handle fields
		if sourceHandle.Valid {
			transition.SourceHandle = sourceHandle.String
		}
		if targetHandle.Valid {
			transition.TargetHandle = targetHandle.String
		}

		transitions = append(transitions, transition)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Always return an array, even if empty
	if transitions == nil {
		transitions = []models.WorkflowTransition{}
	}
	return transitions, nil
}

// ListAvailableTransitions returns the directed transitions a workflow allows
// out of a given status: rows from that status plus from-all rows targeting
// statuses without a direct edge. NULL from-status rows that are not
// from-all are creation-only initial transitions and must never appear as
// moves from an existing item. The result is never nil.
func (r *WorkflowRepository) ListAvailableTransitions(workflowID, statusID int) ([]models.WorkflowTransition, error) {
	query := `
		SELECT wt.id, wt.workflow_id, wt.from_status_id, wt.to_status_id, wt.from_all_statuses, wt.display_order, wt.created_at,
		       fs.name as from_status_name, ts.name as to_status_name, w.name as workflow_name
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		JOIN workflows w ON wt.workflow_id = w.id
		WHERE wt.workflow_id = ?
		  AND (
			wt.from_status_id = ?
			OR (
				wt.from_all_statuses = TRUE
			AND wt.to_status_id NOT IN (
				SELECT to_status_id FROM workflow_transitions WHERE workflow_id = ? AND from_status_id = ?
			)
			)
		  )
		ORDER BY CASE WHEN wt.from_all_statuses THEN 1 ELSE 0 END, wt.display_order ASC`

	rows, err := r.db.Query(query, workflowID, statusID, workflowID, statusID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var transitions []models.WorkflowTransition
	for rows.Next() {
		var transition models.WorkflowTransition
		var fromStatusID sql.NullInt64
		var fromStatusName sql.NullString

		err := rows.Scan(&transition.ID, &transition.WorkflowID, &fromStatusID, &transition.ToStatusID,
			&transition.FromAllStatuses, &transition.DisplayOrder, &transition.CreatedAt, &fromStatusName,
			&transition.ToStatusName, &transition.WorkflowName)
		if err != nil {
			return nil, err
		}

		// Handle nullable from status fields
		if fromStatusID.Valid {
			val := int(fromStatusID.Int64)
			transition.FromStatusID = &val
		}
		if fromStatusName.Valid {
			transition.FromStatusName = fromStatusName.String
		}

		transitions = append(transitions, transition)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Always return an array, even if empty
	if transitions == nil {
		transitions = []models.WorkflowTransition{}
	}
	return transitions, nil
}

// ReplaceTransitions reconciles a workflow's transitions with the given
// payload inside a single transaction. Identity is the (from_status_id,
// from_all_statuses, to_status_id) triple, not the row id: unchanged
// transitions keep their id,
// which is what keeps condition_set_transitions / approval_set_statuses
// references intact and stops cosmetic edits from tripping the CASCADE →
// RESTRICT chain into approval_requests.
//
// Approval requests pinned to removed transitions are hard-deleted; their ids
// are returned so the caller can record an audit trail entry. Payload
// validation failures return ErrTransitionToStatusRequired,
// ErrTransitionToStatusNotFound or ErrTransitionFromStatusNotFound.
func (r *WorkflowRepository) ReplaceTransitions(workflowID int, transitions []models.WorkflowTransition) (cancelledApprovalIDs []int, err error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Load current transitions keyed by (from_status_id, from_all_statuses,
	// to_status_id). We diff the payload against this map below.
	type oldTransition struct {
		id           int
		displayOrder int
		sourceHandle sql.NullString
		targetHandle sql.NullString
	}
	oldByKey := map[string]oldTransition{}
	{
		oldRows, qErr := tx.Query(
			"SELECT id, from_status_id, to_status_id, from_all_statuses, display_order, source_handle, target_handle FROM workflow_transitions WHERE workflow_id = ?",
			workflowID,
		)
		if qErr != nil {
			return nil, qErr
		}
		for oldRows.Next() {
			var ot oldTransition
			var fromID sql.NullInt64
			var toID int
			var fromAll bool
			if sErr := oldRows.Scan(&ot.id, &fromID, &toID, &fromAll, &ot.displayOrder, &ot.sourceHandle, &ot.targetHandle); sErr != nil {
				_ = oldRows.Close()
				return nil, sErr
			}
			oldByKey[transitionKey(fromID, toID, fromAll)] = ot
		}
		if rerr := oldRows.Err(); rerr != nil {
			_ = oldRows.Close()
			return nil, rerr
		}
		_ = oldRows.Close()
	}

	// Validate the payload up front and key it by the same (from, to) pair.
	// Duplicate keys in the payload would already trip the UNIQUE(workflow_id,
	// from_status_id, to_status_id) constraint on insert — we don't enforce it
	// here but last-write-wins is the natural behavior.
	newByKey := map[string]models.WorkflowTransition{}
	for _, transition := range transitions {
		if transition.ToStatusID <= 0 {
			return nil, ErrTransitionToStatusRequired
		}

		var toStatusExists bool
		if sErr := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", transition.ToStatusID).Scan(&toStatusExists); sErr != nil {
			return nil, sErr
		}
		if !toStatusExists {
			return nil, ErrTransitionToStatusNotFound
		}

		// From-all transitions apply to every other status, so they carry no
		// concrete from-status.
		if transition.FromAllStatuses {
			transition.FromStatusID = nil
		}

		if transition.FromStatusID != nil {
			var fromStatusExists bool
			if sErr := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", *transition.FromStatusID).Scan(&fromStatusExists); sErr != nil {
				return nil, sErr
			}
			if !fromStatusExists {
				return nil, ErrTransitionFromStatusNotFound
			}
		}

		var fromNullInt sql.NullInt64
		if transition.FromStatusID != nil {
			fromNullInt = sql.NullInt64{Int64: int64(*transition.FromStatusID), Valid: true}
		}
		newByKey[transitionKey(fromNullInt, transition.ToStatusID, transition.FromAllStatuses)] = transition
	}

	// Diff: anything in old but not in new is being removed.
	toDeleteIDs := []int{}
	for key, ot := range oldByKey {
		if _, kept := newByKey[key]; !kept {
			toDeleteIDs = append(toDeleteIDs, ot.id)
		}
	}

	// Cancel approval_requests pinned to approval_set_statuses pointing at any
	// transition we are about to delete. See CancelApprovalRequestsForTransitions
	// for the rationale.
	cancelledApprovalIDs, err = CancelApprovalRequestsForTransitions(tx, toDeleteIDs)
	if err != nil {
		return nil, err
	}

	if len(toDeleteIDs) > 0 {
		delPlaceholders := make([]string, len(toDeleteIDs))
		delArgs := make([]any, len(toDeleteIDs))
		for i, id := range toDeleteIDs {
			delPlaceholders[i] = "?"
			delArgs[i] = id
		}
		if _, err = tx.Exec(
			fmt.Sprintf("DELETE FROM workflow_transitions WHERE id IN (%s)", strings.Join(delPlaceholders, ",")),
			delArgs...,
		); err != nil {
			return nil, err
		}
	}

	for key, transition := range newByKey {
		var fromNullInt sql.NullInt64
		if transition.FromStatusID != nil {
			fromNullInt = sql.NullInt64{Int64: int64(*transition.FromStatusID), Valid: true}
		}

		if ot, exists := oldByKey[key]; exists {
			if ot.displayOrder == transition.DisplayOrder &&
				ot.sourceHandle.String == transition.SourceHandle &&
				ot.targetHandle.String == transition.TargetHandle {
				continue
			}
			if _, err = tx.Exec(`
				UPDATE workflow_transitions
				SET display_order = ?, source_handle = ?, target_handle = ?
				WHERE id = ?
			`, transition.DisplayOrder, transition.SourceHandle, transition.TargetHandle, ot.id); err != nil {
				return nil, err
			}
			continue
		}

		if _, err = tx.Exec(`
			INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, from_all_statuses, display_order, source_handle, target_handle, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, workflowID, fromNullInt, transition.ToStatusID, transition.FromAllStatuses, transition.DisplayOrder, transition.SourceHandle, transition.TargetHandle, time.Now()); err != nil {
			return nil, err
		}
	}

	if cErr := tx.Commit(); cErr != nil {
		return nil, cErr
	}
	return cancelledApprovalIDs, nil
}

// transitionKey creates a unique key for a transition by its from/to status
// IDs and from-all flag. NULL-from rows split into initial ("nil") and
// from-all ("all") namespaces so the two wildcard kinds never collide.
func transitionKey(fromStatusID sql.NullInt64, toStatusID int, fromAll bool) string {
	if fromAll {
		return fmt.Sprintf("all:%d", toStatusID)
	}
	if fromStatusID.Valid {
		return fmt.Sprintf("%d:%d", fromStatusID.Int64, toStatusID)
	}
	return fmt.Sprintf("nil:%d", toStatusID)
}

// CancelApprovalRequestsForTransitions hard-deletes requests blocking removed
// approval transitions. RESTRICT foreign keys otherwise prevent workflow edits;
// callers record the deleted IDs in the durable audit log. Exported for the
// Jira importer's workflow-replacement transaction.
func CancelApprovalRequestsForTransitions(tx database.Tx, transitionIDs []int) ([]int, error) {
	if len(transitionIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(transitionIDs))
	for i := range transitionIDs {
		placeholders[i] = "?"
	}
	placeholderList := strings.Join(placeholders, ",")

	args := make([]any, 0, len(transitionIDs)*2)
	for _, id := range transitionIDs {
		args = append(args, id)
	}
	for _, id := range transitionIDs {
		args = append(args, id)
	}

	rows, err := tx.Query(fmt.Sprintf(`
		SELECT DISTINCT ar.id
		FROM approval_requests ar
		JOIN approval_set_statuses ass ON ass.id = ar.approval_set_status_id
		WHERE ass.approve_transition_id IN (%s) OR ass.deny_transition_id IN (%s)
	`, placeholderList, placeholderList), args...)
	if err != nil {
		return nil, fmt.Errorf("query blocking approval_requests: %w", err)
	}

	var requestIDs []int
	for rows.Next() {
		var id int
		if scanErr := rows.Scan(&id); scanErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan blocking approval_request id: %w", scanErr)
		}
		requestIDs = append(requestIDs, id)
	}
	if rerr := rows.Err(); rerr != nil {
		_ = rows.Close()
		return nil, rerr
	}
	_ = rows.Close()

	if len(requestIDs) == 0 {
		return nil, nil
	}

	delPlaceholders := make([]string, len(requestIDs))
	delArgs := make([]any, len(requestIDs))
	for i, id := range requestIDs {
		delPlaceholders[i] = "?"
		delArgs[i] = id
	}
	if _, err := tx.Exec(
		fmt.Sprintf("DELETE FROM approval_requests WHERE id IN (%s)", strings.Join(delPlaceholders, ",")),
		delArgs...,
	); err != nil {
		return nil, fmt.Errorf("delete blocking approval_requests: %w", err)
	}

	return requestIDs, nil
}
