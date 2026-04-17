package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
)

// ConditionSetHandler handles CRUD for condition sets
type ConditionSetHandler struct {
	db database.Database
}

// NewConditionSetHandler creates a new condition set handler
func NewConditionSetHandler(db database.Database) *ConditionSetHandler {
	return &ConditionSetHandler{db: db}
}

// GetAll returns all condition sets, optionally filtered by workflow_id
func (h *ConditionSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	query := `
		SELECT cs.id, cs.name, cs.description, cs.workflow_id, cs.created_at, cs.updated_at,
		       w.name as workflow_name
		FROM condition_sets cs
		JOIN workflows w ON cs.workflow_id = w.id`

	var args []interface{}
	if workflowIDStr := r.URL.Query().Get("workflow_id"); workflowIDStr != "" {
		workflowID, err := strconv.Atoi(workflowIDStr)
		if err != nil {
			respondValidationError(w, r, "Invalid workflow_id")
			return
		}
		query += " WHERE cs.workflow_id = ?"
		args = append(args, workflowID)
	}

	query += " ORDER BY cs.name"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	conditionSets, err := scanConditionSets(rows)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if conditionSets == nil {
		conditionSets = []models.ConditionSet{}
	}

	respondJSONOK(w, conditionSets)
}

// Get returns a single condition set with all transition conditions
func (h *ConditionSetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	_, ok = RequireAuth(w, r)
	if !ok {
		return
	}

	cs, err := h.loadConditionSet(id)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "Condition set")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, cs)
}

// Create creates a new condition set with transition conditions
func (h *ConditionSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, input, ok := h.getConditionSetForEdit(w, r)
	if !ok {
		return
	}

	if input.WorkflowID == 0 {
		respondValidationError(w, r, "Workflow ID is required")
		return
	}

	// Validate workflow exists
	var workflowExists bool
	if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workflows WHERE id = ?)", input.WorkflowID).Scan(&workflowExists); err != nil || !workflowExists {
		respondValidationError(w, r, "Workflow not found")
		return
	}

	// Validate transition conditions
	if err := h.validateTransitionConditions(input.WorkflowID, input.TransitionConditions); err != nil {
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
	result, err := tx.Exec(`
		INSERT INTO condition_sets (name, description, workflow_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, input.Name, input.Description, input.WorkflowID, now, now)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	csID64, _ := result.LastInsertId()
	csID := int(csID64)

	if err := h.saveTransitionConditions(tx, csID, input.TransitionConditions); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, user, logger.ActionConditionSetCreate, logger.ResourceConditionSet, &csID, input.Name)

	cs, err := h.loadConditionSet(csID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, cs)
}

// Update replaces a condition set and its transition conditions
func (h *ConditionSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, input, ok := h.getConditionSetForEdit(w, r)
	if !ok {
		return
	}

	// Verify condition set exists and get its workflow_id
	var existingWorkflowID int
	err := h.db.QueryRow("SELECT workflow_id FROM condition_sets WHERE id = ?", id).Scan(&existingWorkflowID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "Condition set")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// workflow_id cannot change
	if input.WorkflowID != 0 && input.WorkflowID != existingWorkflowID {
		respondValidationError(w, r, "Cannot change workflow_id of an existing condition set")
		return
	}

	if err := h.validateTransitionConditions(existingWorkflowID, input.TransitionConditions); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`
		UPDATE condition_sets SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, input.Name, input.Description, time.Now(), id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete existing transition conditions and re-create
	_, err = tx.Exec("DELETE FROM condition_set_transitions WHERE condition_set_id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.saveTransitionConditions(tx, id, input.TransitionConditions); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, user, logger.ActionConditionSetUpdate, logger.ResourceConditionSet, &id, input.Name)

	cs, err := h.loadConditionSet(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, cs)
}

// Delete deletes a condition set
func (h *ConditionSetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get name for audit log before deleting
	var name string
	err := h.db.QueryRow("SELECT name FROM condition_sets WHERE id = ?", id).Scan(&name)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "Condition set")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check if in use by any configuration set
	var inUseCount int
	err = h.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT id FROM configuration_sets WHERE condition_set_id = ?
			UNION ALL
			SELECT id FROM configuration_set_item_types WHERE condition_set_id = ?
		)
	`, id, id).Scan(&inUseCount)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if inUseCount > 0 {
		respondValidationError(w, r, "Cannot delete condition set: it is in use by one or more configuration sets")
		return
	}

	_, err = h.db.Exec("DELETE FROM condition_sets WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, user, logger.ActionConditionSetDelete, logger.ResourceConditionSet, &id, name)

	w.WriteHeader(http.StatusNoContent)
}

// GetByWorkflow returns condition sets for a specific workflow
func (h *ConditionSetHandler) GetByWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	_, ok = RequireAuth(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT cs.id, cs.name, cs.description, cs.workflow_id, cs.created_at, cs.updated_at,
		       w.name as workflow_name
		FROM condition_sets cs
		JOIN workflows w ON cs.workflow_id = w.id
		WHERE cs.workflow_id = ?
		ORDER BY cs.name
	`, workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	conditionSets, err := scanConditionSets(rows)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if conditionSets == nil {
		conditionSets = []models.ConditionSet{}
	}

	respondJSONOK(w, conditionSets)
}

// scanConditionSets scans rows of condition sets and returns the slice.
func scanConditionSets(rows *sql.Rows) ([]models.ConditionSet, error) {
	var conditionSets []models.ConditionSet
	for rows.Next() {
		var cs models.ConditionSet
		var description sql.NullString
		err := rows.Scan(&cs.ID, &cs.Name, &description, &cs.WorkflowID,
			&cs.CreatedAt, &cs.UpdatedAt, &cs.WorkflowName)
		if err != nil {
			return nil, err
		}
		cs.Description = description.String
		conditionSets = append(conditionSets, cs)
	}
	return conditionSets, nil
}

// getConditionSetForEdit authenticates the request, decodes the JSON body
// into a ConditionSet, and validates that Name is present.
func (h *ConditionSetHandler) getConditionSetForEdit(w http.ResponseWriter, r *http.Request) (*models.User, models.ConditionSet, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, models.ConditionSet{}, false
	}

	var input models.ConditionSet
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid request body")
		return nil, models.ConditionSet{}, false
	}

	if input.Name == "" {
		respondValidationError(w, r, "Name is required")
		return nil, models.ConditionSet{}, false
	}

	return user, input, true
}

// --- internal helpers ---

func (h *ConditionSetHandler) loadConditionSet(id int) (*models.ConditionSet, error) {
	var cs models.ConditionSet
	var description sql.NullString
	err := h.db.QueryRow(`
		SELECT cs.id, cs.name, cs.description, cs.workflow_id, cs.created_at, cs.updated_at,
		       w.name as workflow_name
		FROM condition_sets cs
		JOIN workflows w ON cs.workflow_id = w.id
		WHERE cs.id = ?
	`, id).Scan(&cs.ID, &cs.Name, &description, &cs.WorkflowID,
		&cs.CreatedAt, &cs.UpdatedAt, &cs.WorkflowName)
	if err != nil {
		return nil, err
	}
	cs.Description = description.String

	// Load transition conditions
	tcRows, err := h.db.Query(`
		SELECT cst.id, cst.condition_set_id, cst.transition_id, cst.logic_mode,
		       fs.name as from_status_name, ts.name as to_status_name
		FROM condition_set_transitions cst
		JOIN workflow_transitions wt ON cst.transition_id = wt.id
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		WHERE cst.condition_set_id = ?
		ORDER BY cst.id
	`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tcRows.Close() }()

	var tcs []models.TransitionCondition
	for tcRows.Next() {
		var tc models.TransitionCondition
		var fromName sql.NullString
		if err := tcRows.Scan(&tc.ID, &tc.ConditionSetID, &tc.TransitionID, &tc.LogicMode,
			&fromName, &tc.ToStatusName); err != nil {
			continue
		}
		tc.FromStatusName = fromName.String
		tcs = append(tcs, tc)
	}

	// Load conditions for each transition condition
	for i := range tcs {
		condRows, err := h.db.Query(`
			SELECT id, condition_set_transition_id, condition_type, config, display_order, mode, COALESCE(error_message, '')
			FROM conditions
			WHERE condition_set_transition_id = ?
			ORDER BY display_order, id
		`, tcs[i].ID)
		if err != nil {
			return nil, err
		}

		var conditions []models.Condition
		for condRows.Next() {
			var c models.Condition
			var configStr string
			if err := condRows.Scan(&c.ID, &c.ConditionSetTransitionID, &c.ConditionType, &configStr, &c.DisplayOrder, &c.Mode, &c.ErrorMessage); err != nil {
				_ = condRows.Close()
				return nil, fmt.Errorf("scanning condition row: %w", err)
			}
			c.Config = json.RawMessage(configStr)
			conditions = append(conditions, c)
		}
		_ = condRows.Close()

		if conditions == nil {
			conditions = []models.Condition{}
		}
		tcs[i].Conditions = conditions
	}

	if tcs == nil {
		tcs = []models.TransitionCondition{}
	}
	cs.TransitionConditions = tcs

	return &cs, nil
}

func (h *ConditionSetHandler) validateTransitionConditions(workflowID int, tcs []models.TransitionCondition) error {
	for _, tc := range tcs {
		if tc.TransitionID == 0 {
			return &validationErr{msg: "Transition ID is required for each transition condition"}
		}
		if tc.LogicMode != "and" && tc.LogicMode != "or" {
			return &validationErr{msg: "Logic mode must be 'and' or 'or'"}
		}

		// Verify transition belongs to the workflow
		var belongs bool
		if err := h.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM workflow_transitions WHERE id = ? AND workflow_id = ?)",
			tc.TransitionID, workflowID,
		).Scan(&belongs); err != nil || !belongs {
			return &validationErr{msg: "Transition does not belong to the specified workflow"}
		}

		// Validate each condition
		for _, c := range tc.Conditions {
			if err := validateCondition(c); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCondition(c models.Condition) error {
	// Validate mode if provided
	if c.Mode != "" && c.Mode != models.ConditionModeCondition && c.Mode != models.ConditionModeValidator {
		return &validationErr{msg: "Condition mode must be 'condition' or 'validator'"}
	}

	switch c.ConditionType {
	case models.ConditionTypeUserInRole:
		var cfg models.ConditionUserInRoleConfig
		if err := json.Unmarshal(c.Config, &cfg); err != nil {
			return &validationErr{msg: "Invalid user_in_role config"}
		}
		validSources := map[string]bool{"current_user": true, "creator": true, "assignee": true, "field": true}
		if !validSources[cfg.UserSource] {
			return &validationErr{msg: "user_in_role user_source must be 'current_user', 'creator', 'assignee', or 'field'"}
		}
		if cfg.UserSource == "field" && cfg.FieldID == nil {
			return &validationErr{msg: "user_in_role requires field_id when user_source is 'field'"}
		}
		if cfg.RoleID == 0 {
			return &validationErr{msg: "user_in_role requires a role_id"}
		}
	case models.ConditionTypeUserInGroup:
		var cfg models.ConditionUserInGroupConfig
		if err := json.Unmarshal(c.Config, &cfg); err != nil {
			return &validationErr{msg: "Invalid user_in_group config"}
		}
		validSources := map[string]bool{"current_user": true, "creator": true, "assignee": true, "field": true}
		if !validSources[cfg.UserSource] {
			return &validationErr{msg: "user_in_group user_source must be 'current_user', 'creator', 'assignee', or 'field'"}
		}
		if cfg.UserSource == "field" && cfg.FieldID == nil {
			return &validationErr{msg: "user_in_group requires field_id when user_source is 'field'"}
		}
		if cfg.GroupID == 0 {
			return &validationErr{msg: "user_in_group requires a group_id"}
		}
	case models.ConditionTypeFieldValue:
		var cfg models.ConditionFieldValueConfig
		if err := json.Unmarshal(c.Config, &cfg); err != nil {
			return &validationErr{msg: "Invalid field_value config"}
		}
		if cfg.FieldIdentifier == "" || cfg.Pattern == "" {
			return &validationErr{msg: "field_value requires field_identifier and pattern"}
		}
	case models.ConditionTypeScript:
		var cfg models.ConditionScriptConfig
		if err := json.Unmarshal(c.Config, &cfg); err != nil {
			return &validationErr{msg: "Invalid script config"}
		}
		if cfg.Script == "" {
			return &validationErr{msg: "script condition requires a script"}
		}
		if len(cfg.Script) > 10240 {
			return &validationErr{msg: "Script exceeds maximum length of 10KB"}
		}
	default:
		return &validationErr{msg: "Unknown condition type: " + c.ConditionType}
	}
	return nil
}

func (h *ConditionSetHandler) saveTransitionConditions(tx database.Tx, conditionSetID int, tcs []models.TransitionCondition) error {
	for _, tc := range tcs {
		result, err := tx.Exec(`
			INSERT INTO condition_set_transitions (condition_set_id, transition_id, logic_mode, created_at)
			VALUES (?, ?, ?, ?)
		`, conditionSetID, tc.TransitionID, tc.LogicMode, time.Now())
		if err != nil {
			return err
		}

		cstID64, _ := result.LastInsertId()
		cstID := int(cstID64)

		for _, c := range tc.Conditions {
			configBytes, err := json.Marshal(c.Config)
			if err != nil {
				return err
			}
			var errMsg *string
			if c.ErrorMessage != "" {
				errMsg = &c.ErrorMessage
			}
			mode := c.Mode
			if mode == "" {
				mode = models.ConditionModeCondition
			}
			_, err = tx.Exec(`
				INSERT INTO conditions (condition_set_transition_id, condition_type, config, display_order, mode, error_message, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, cstID, c.ConditionType, string(configBytes), c.DisplayOrder, mode, errMsg, time.Now())
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// validationErr is a simple error type for validation errors
type validationErr struct {
	msg string
}

func (e *validationErr) Error() string {
	return e.msg
}
