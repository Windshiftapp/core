package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type WorkflowHandler struct {
	db database.Database
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

	if err == sql.ErrNoRows {
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
	var workflow models.Workflow
	if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
		respondBadRequest(w, r, err.Error())
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
	result, err := h.db.ExecWrite(`
		INSERT INTO workflows (name, description, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, workflow.Name, workflow.Description, workflow.IsDefault, now, now)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	id, err := result.LastInsertId()
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

	respondJSONCreated(w, createdWorkflow)
}

func (h *WorkflowHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var workflow models.Workflow
	if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
		respondBadRequest(w, r, err.Error())
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

	var transitions []models.WorkflowTransition
	if err := json.NewDecoder(r.Body).Decode(&transitions); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	// Start transaction for atomic updates
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing transitions for this workflow
	_, err = tx.Exec("DELETE FROM workflow_transitions WHERE workflow_id = ?", workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Insert new transitions
	for _, transition := range transitions {
		// Validate required fields
		if transition.ToStatusID <= 0 {
			respondValidationError(w, r, "To status ID is required for all transitions")
			return
		}

		// Validate that statuses exist
		var toStatusExists bool
		err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", transition.ToStatusID).Scan(&toStatusExists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !toStatusExists {
			respondValidationError(w, r, "To status not found")
			return
		}

		if transition.FromStatusID != nil {
			var fromStatusExists bool
			err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM statuses WHERE id = ?)", *transition.FromStatusID).Scan(&fromStatusExists)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			if !fromStatusExists {
				respondValidationError(w, r, "From status not found")
				return
			}
		}

		_, err = tx.Exec(`
			INSERT INTO workflow_transitions (workflow_id, from_status_id, to_status_id, display_order, source_handle, target_handle, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, workflowID, transition.FromStatusID, transition.ToStatusID, transition.DisplayOrder, transition.SourceHandle, transition.TargetHandle, time.Now())

		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return updated transitions
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

	// Always return an array, even if empty
	if transitions == nil {
		transitions = []models.WorkflowTransition{}
	}

	return transitions, nil
}
