package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type WorkflowHandler struct {
	db              database.Database
	workflowService *services.WorkflowService
}

// SetWorkflowService sets the workflow service for cache invalidation
func (h *WorkflowHandler) SetWorkflowService(ws *services.WorkflowService) {
	h.workflowService = ws
}

func NewWorkflowHandler(db database.Database) *WorkflowHandler {
	return &WorkflowHandler{db: db}
}

func (h *WorkflowHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, name, description, is_default, created_at, updated_at
		FROM workflows
		ORDER BY is_default DESC, name ASC`

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var workflows []models.Workflow
	for rows.Next() {
		var workflow models.Workflow

		err := rows.Scan(&workflow.ID, &workflow.Name, &workflow.Description,
			&workflow.IsDefault, &workflow.CreatedAt, &workflow.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		workflows = append(workflows, workflow)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Always return an array, even if empty
	if workflows == nil {
		workflows = []models.Workflow{}
	}

	slog.Info("workflows listed", "count", len(workflows))
	respondJSONOK(w, workflows)
}

func (h *WorkflowHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var workflow models.Workflow
	err := h.db.QueryRow(`
		SELECT id, name, description, is_default, created_at, updated_at
		FROM workflows
		WHERE id = ?
	`, id).Scan(&workflow.ID, &workflow.Name, &workflow.Description,
		&workflow.IsDefault, &workflow.CreatedAt, &workflow.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "workflow")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load transitions for this workflow
	transitions, err := h.getWorkflowTransitions(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	workflow.Transitions = transitions

	respondJSONOK(w, workflow)
}

func (h *WorkflowHandler) Create(w http.ResponseWriter, r *http.Request) {
	workflow, ok := decodeJSON[models.Workflow](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if strings.TrimSpace(workflow.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Check if name already exists
	var exists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workflows WHERE name = ?)", workflow.Name).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Workflow with this name already exists")
		return
	}

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO workflows (name, description, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, workflow.Name, workflow.Description, workflow.IsDefault, now, now).Scan(&id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	slog.Info("workflow created", "id", id, "name", workflow.Name)

	// Return the created workflow
	var createdWorkflow models.Workflow
	err = h.db.QueryRow(`
		SELECT id, name, description, is_default, created_at, updated_at
		FROM workflows
		WHERE id = ?
	`, id).Scan(&createdWorkflow.ID, &createdWorkflow.Name, &createdWorkflow.Description,
		&createdWorkflow.IsDefault, &createdWorkflow.CreatedAt, &createdWorkflow.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load transitions (will be empty for new workflow)
	transitions, err := h.getWorkflowTransitions(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	createdWorkflow.Transitions = transitions

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		intID := int(id)
		logAudit(h.db, r, currentUser, logger.ActionWorkflowCreate, logger.ResourceWorkflow, &intID, workflow.Name)
	}

	respondJSONCreated(w, createdWorkflow)
}

func (h *WorkflowHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	workflow, ok := decodeJSON[models.Workflow](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if strings.TrimSpace(workflow.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	// Check if name already exists (excluding current record)
	var exists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workflows WHERE name = ? AND id != ?)", workflow.Name, id).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Workflow with this name already exists")
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE workflows
		SET name = ?, description = ?, is_default = ?, updated_at = ?
		WHERE id = ?
	`, workflow.Name, workflow.Description, workflow.IsDefault, now, id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated workflow
	var updatedWorkflow models.Workflow
	err = h.db.QueryRow(`
		SELECT id, name, description, is_default, created_at, updated_at
		FROM workflows
		WHERE id = ?
	`, id).Scan(&updatedWorkflow.ID, &updatedWorkflow.Name, &updatedWorkflow.Description,
		&updatedWorkflow.IsDefault, &updatedWorkflow.CreatedAt, &updatedWorkflow.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load transitions
	transitions, err := h.getWorkflowTransitions(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	updatedWorkflow.Transitions = transitions

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionWorkflowUpdate, logger.ResourceWorkflow, &id, workflow.Name)
	}

	// Invalidate initial status cache so new items get the correct initial status
	if h.workflowService != nil {
		h.workflowService.InvalidateInitialStatusCache()
	}

	respondJSONOK(w, updatedWorkflow)
}

func (h *WorkflowHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Check if any configuration sets are using this workflow
	var configCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM configuration_sets WHERE workflow_id = ?", id).Scan(&configCount)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if configCount > 0 {
		respondConflict(w, r, "Cannot delete workflow that is in use by configuration sets")
		return
	}

	// Start transaction to ensure atomic deletion
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Collect every transition id on this workflow so we can cancel any
	// approval_requests pinned to them before the CASCADE-delete chain trips
	// the RESTRICT-FK from approval_requests → approval_set_statuses.
	transitionIDs := []int{}
	{
		rows, qErr := tx.Query("SELECT id FROM workflow_transitions WHERE workflow_id = ?", id)
		if qErr != nil {
			respondInternalError(w, r, qErr)
			return
		}
		for rows.Next() {
			var tid int
			if sErr := rows.Scan(&tid); sErr != nil {
				_ = rows.Close()
				respondInternalError(w, r, sErr)
				return
			}
			transitionIDs = append(transitionIDs, tid)
		}
		if rerr := rows.Err(); rerr != nil {
			_ = rows.Close()
			respondInternalError(w, r, rerr)
			return
		}
		_ = rows.Close()
	}

	cancelledApprovalIDs, err := cancelApprovalRequestsForTransitions(tx, transitionIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete workflow transitions first
	_, err = tx.Exec("DELETE FROM workflow_transitions WHERE workflow_id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete the workflow
	_, err = tx.Exec("DELETE FROM workflows WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		if len(cancelledApprovalIDs) > 0 {
			_ = logger.LogAudit(h.db, logger.AuditEvent{
				UserID:       currentUser.ID,
				Username:     currentUser.Username,
				IPAddress:    utils.GetClientIP(r),
				UserAgent:    r.UserAgent(),
				ActionType:   logger.ActionWorkflowDelete,
				ResourceType: logger.ResourceWorkflow,
				ResourceID:   &id,
				Details: map[string]interface{}{
					"canceled_approval_request_ids": cancelledApprovalIDs,
					"cancellation_reason":           "workflow_deleted",
				},
				Success: true,
			})
		} else {
			logAudit(h.db, r, currentUser, logger.ActionWorkflowDelete, logger.ResourceWorkflow, &id, "")
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetTransitions returns the transitions for a workflow.
func (h *WorkflowHandler) GetTransitions(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	transitions, err := h.getWorkflowTransitions(workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, transitions)
}

func (h *WorkflowHandler) UpdateTransitions(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	transitions, ok := decodeJSON[[]models.WorkflowTransition](w, r)
	if !ok {
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Load current transitions keyed by (from_status_id, to_status_id). We diff
	// the payload against this map below: identity is the status pair, not the
	// row id. Preserving the id of unchanged transitions is what keeps
	// condition_set_transitions / approval_set_statuses references intact and
	// stops cosmetic edits from tripping the CASCADE → RESTRICT chain into
	// approval_requests.
	type oldTransition struct {
		id           int
		displayOrder int
		sourceHandle sql.NullString
		targetHandle sql.NullString
	}
	oldByKey := map[string]oldTransition{}
	{
		oldRows, qErr := tx.Query(
			"SELECT id, from_status_id, to_status_id, display_order, source_handle, target_handle FROM workflow_transitions WHERE workflow_id = ?",
			workflowID,
		)
		if qErr != nil {
			respondInternalError(w, r, qErr)
			return
		}
		for oldRows.Next() {
			var ot oldTransition
			var fromID sql.NullInt64
			var toID int
			if sErr := oldRows.Scan(&ot.id, &fromID, &toID, &ot.displayOrder, &ot.sourceHandle, &ot.targetHandle); sErr != nil {
				_ = oldRows.Close()
				respondInternalError(w, r, sErr)
				return
			}
			oldByKey[transitionKeyStr(fromID, toID)] = ot
		}
		if rerr := oldRows.Err(); rerr != nil {
			_ = oldRows.Close()
			respondInternalError(w, r, rerr)
			return
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
			respondValidationError(w, r, "To status ID is required for all transitions")
			return
		}

		var toStatusExists bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", transition.ToStatusID).Scan(&toStatusExists); err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !toStatusExists {
			respondValidationError(w, r, "To status not found")
			return
		}

		if transition.FromStatusID != nil {
			var fromStatusExists bool
			if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", *transition.FromStatusID).Scan(&fromStatusExists); err != nil {
				respondInternalError(w, r, err)
				return
			}
			if !fromStatusExists {
				respondValidationError(w, r, "From status not found")
				return
			}
		}

		var fromNullInt sql.NullInt64
		if transition.FromStatusID != nil {
			fromNullInt = sql.NullInt64{Int64: int64(*transition.FromStatusID), Valid: true}
		}
		newByKey[transitionKeyStr(fromNullInt, transition.ToStatusID)] = transition
	}

	// Diff: anything in old but not in new is being removed.
	toDeleteIDs := []int{}
	for key, ot := range oldByKey {
		if _, kept := newByKey[key]; !kept {
			toDeleteIDs = append(toDeleteIDs, ot.id)
		}
	}

	// Cancel approval_requests pinned to approval_set_statuses pointing at any
	// transition we are about to delete. See cancelApprovalRequestsForTransitions
	// for the rationale.
	cancelledApprovalIDs, err := cancelApprovalRequestsForTransitions(tx, toDeleteIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if len(toDeleteIDs) > 0 {
		delPlaceholders := make([]string, len(toDeleteIDs))
		delArgs := make([]interface{}, len(toDeleteIDs))
		for i, id := range toDeleteIDs {
			delPlaceholders[i] = "?"
			delArgs[i] = id
		}
		if _, err = tx.Exec(
			fmt.Sprintf("DELETE FROM workflow_transitions WHERE id IN (%s)", strings.Join(delPlaceholders, ",")),
			delArgs...,
		); err != nil {
			respondInternalError(w, r, err)
			return
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
				respondInternalError(w, r, err)
				return
			}
			continue
		}

		if _, err = tx.Exec(`
			INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order, source_handle, target_handle, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, workflowID, fromNullInt, transition.ToStatusID, transition.DisplayOrder, transition.SourceHandle, transition.TargetHandle, time.Now()); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := map[string]interface{}{"update_type": "transitions"}
		if len(cancelledApprovalIDs) > 0 {
			details["canceled_approval_request_ids"] = cancelledApprovalIDs
			details["cancellation_reason"] = "transition_removed"
		}
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionWorkflowUpdate,
			ResourceType: logger.ResourceWorkflow,
			ResourceID:   &workflowID,
			Details:      details,
			Success:      true,
		})
	}

	// Invalidate initial status cache so new items get the correct initial status
	if h.workflowService != nil {
		h.workflowService.InvalidateInitialStatusCache()
	}

	updatedTransitions, err := h.getWorkflowTransitions(workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, updatedTransitions)
}

func (h *WorkflowHandler) GetAvailableTransitions(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	statusID, ok := requireIDParam(w, r, "statusID")
	if !ok {
		return
	}

	query := `
		SELECT wt.id, wt.workflow_id, wt.from_status_id, wt.to_status_id, wt.display_order, wt.created_at,
		       fs.name as from_status_name, ts.name as to_status_name, w.name as workflow_name
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		JOIN workflows w ON wt.workflow_id = w.id
		WHERE wt.workflow_id = ? AND (wt.from_status_id = ? OR wt.from_status_id IS NULL)
		ORDER BY wt.display_order ASC`

	rows, err := h.db.Query(query, workflowID, statusID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var transitions []models.WorkflowTransition
	for rows.Next() {
		var transition models.WorkflowTransition
		var fromStatusID sql.NullInt64
		var fromStatusName sql.NullString

		err := rows.Scan(&transition.ID, &transition.WorkflowID, &fromStatusID, &transition.ToStatusID,
			&transition.DisplayOrder, &transition.CreatedAt, &fromStatusName,
			&transition.ToStatusName, &transition.WorkflowName)
		if err != nil {
			respondInternalError(w, r, err)
			return
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
		respondInternalError(w, r, err)
		return
	}

	// Always return an array, even if empty
	if transitions == nil {
		transitions = []models.WorkflowTransition{}
	}

	respondJSONOK(w, transitions)
}

// Helper function to get workflow transitions
func (h *WorkflowHandler) getWorkflowTransitions(workflowID int) ([]models.WorkflowTransition, error) {
	query := `
		SELECT wt.id, wt.workflow_id, wt.from_status_id, wt.to_status_id, wt.display_order, wt.source_handle, wt.target_handle, wt.created_at,
		       fs.name as from_status_name, ts.name as to_status_name, w.name as workflow_name
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		JOIN workflows w ON wt.workflow_id = w.id
		WHERE wt.workflow_id = ?
		ORDER BY wt.from_status_id NULLS FIRST, wt.display_order ASC`

	rows, err := h.db.Query(query, workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var transitions []models.WorkflowTransition
	for rows.Next() {
		var transition models.WorkflowTransition
		var fromStatusID sql.NullInt64
		var fromStatusName sql.NullString
		var sourceHandle sql.NullString
		var targetHandle sql.NullString

		err := rows.Scan(&transition.ID, &transition.WorkflowID, &fromStatusID, &transition.ToStatusID,
			&transition.DisplayOrder, &sourceHandle, &targetHandle, &transition.CreatedAt, &fromStatusName,
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

// transitionKeyStr creates a unique key for a transition by its from/to status IDs.
func transitionKeyStr(fromStatusID sql.NullInt64, toStatusID int) string {
	if fromStatusID.Valid {
		return fmt.Sprintf("%d:%d", fromStatusID.Int64, toStatusID)
	}
	return fmt.Sprintf("nil:%d", toStatusID)
}

// cancelApprovalRequestsForTransitions hard-deletes approval_requests pinned to
// approval_set_statuses whose approve_transition_id or deny_transition_id is in
// transitionIDs. Returns the deleted request ids so the caller can record a
// single audit_logs entry after the surrounding transaction commits.
//
// Why hard-delete: approval_requests.approval_set_status_id is ON DELETE
// RESTRICT, so the CASCADE-delete chain from workflow_transitions →
// approval_set_statuses → approval_requests would otherwise fail with
// SQLITE_CONSTRAINT_FOREIGNKEY (1811). The soft-archive model on
// approval_set_statuses (is_active=0) keeps RESTRICT-FKs even for completed
// requests, so there is no purely-reconfiguration way out. We sacrifice the
// per-request approval_decisions trail (it CASCADEs away with the request) in
// exchange for letting admins edit the workflow; the durable record is the
// audit_logs row the caller writes.
func cancelApprovalRequestsForTransitions(tx database.Tx, transitionIDs []int) ([]int, error) {
	if len(transitionIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(transitionIDs))
	for i := range transitionIDs {
		placeholders[i] = "?"
	}
	placeholderList := strings.Join(placeholders, ",")

	args := make([]interface{}, 0, len(transitionIDs)*2)
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
	delArgs := make([]interface{}, len(requestIDs))
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
