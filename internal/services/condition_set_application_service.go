package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

type ConditionSetPatch struct {
	Name, Description    *string
	WorkflowID           *int
	TransitionConditions *[]models.TransitionCondition
}

type ConditionSetApplicationService struct {
	db          database.Database
	permissions *PermissionService
}

func NewConditionSetApplicationService(db database.Database, permissions *PermissionService) *ConditionSetApplicationService {
	return &ConditionSetApplicationService{db: db, permissions: permissions}
}

func (s *ConditionSetApplicationService) List(workflowID *int) ([]models.ConditionSet, error) {
	query := `SELECT cs.id, cs.name, cs.description, cs.workflow_id, cs.created_at, cs.updated_at, w.name FROM condition_sets cs JOIN workflows w ON cs.workflow_id = w.id`
	args := []any{}
	if workflowID != nil {
		query += " WHERE cs.workflow_id = ?"
		args = append(args, *workflowID)
	}
	query += " ORDER BY cs.name"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanConditionSets(rows)
	if err != nil {
		return nil, err
	}
	if err := s.attachGatedTransitions(items); err != nil {
		return nil, err
	}
	if items == nil {
		items = []models.ConditionSet{}
	}
	return items, nil
}

func (s *ConditionSetApplicationService) Get(id int) (*models.ConditionSet, error) { return s.load(id) }

func (s *ConditionSetApplicationService) Create(actor AuditActor, input models.ConditionSet) (*models.ConditionSet, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	prepareConditionSet(&input)
	if input.Name == "" {
		return nil, governanceValidation("name is required")
	}
	if input.WorkflowID <= 0 {
		return nil, governanceValidation("workflow_id is required")
	}
	var exists bool
	if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workflows WHERE id = ?)", input.WorkflowID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, governanceValidation("workflow was not found")
	}
	if err := s.validateTransitions(input.WorkflowID, input.TransitionConditions); err != nil {
		return nil, err
	}
	id, err := database.WithTxResult(s.db, func(tx database.Tx) (int, error) {
		var id int
		now := time.Now()
		if err := tx.QueryRow(`INSERT INTO condition_sets (name, description, workflow_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?) RETURNING id`, input.Name, input.Description, input.WorkflowID, now, now).Scan(&id); err != nil {
			return 0, err
		}
		return id, saveTransitionConditions(tx, id, input.TransitionConditions)
	})
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionConditionSetCreate, logger.ResourceConditionSet, &id, input.Name, nil)
	return s.load(id)
}

func (s *ConditionSetApplicationService) Patch(actor AuditActor, id int, patch ConditionSetPatch) (*models.ConditionSet, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	current, err := s.load(id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	if patch.WorkflowID != nil && *patch.WorkflowID != current.WorkflowID {
		return nil, governanceValidation("workflow_id cannot be changed")
	}
	if patch.TransitionConditions != nil {
		current.TransitionConditions = *patch.TransitionConditions
	}
	prepareConditionSet(current)
	if current.Name == "" {
		return nil, governanceValidation("name is required")
	}
	if err := s.validateTransitions(current.WorkflowID, current.TransitionConditions); err != nil {
		return nil, err
	}
	err = database.WithTx(s.db, func(tx database.Tx) error {
		if _, err := tx.Exec("UPDATE condition_sets SET name = ?, description = ?, updated_at = ? WHERE id = ?", current.Name, current.Description, time.Now(), id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM condition_set_transitions WHERE condition_set_id = ?", id); err != nil {
			return err
		}
		return saveTransitionConditions(tx, id, current.TransitionConditions)
	})
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionConditionSetUpdate, logger.ResourceConditionSet, &id, current.Name, nil)
	return s.load(id)
}

func (s *ConditionSetApplicationService) Delete(actor AuditActor, id int) error {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return err
	}
	var name string
	if err := s.db.QueryRow("SELECT name FROM condition_sets WHERE id = ?", id).Scan(&name); errors.Is(err, sql.ErrNoRows) {
		return repository.ErrNotFound
	} else if err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM (SELECT id FROM configuration_sets WHERE condition_set_id = ? UNION ALL SELECT id FROM configuration_set_item_types WHERE condition_set_id = ?)`, id, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return ErrGovernanceConflict
	}
	if _, err := s.db.ExecWrite("DELETE FROM condition_sets WHERE id = ?", id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionConditionSetDelete, logger.ResourceConditionSet, &id, name, nil)
	return nil
}

func (s *ConditionSetApplicationService) requireAdmin(userID int) error {
	allowed, err := s.permissions.IsSystemAdmin(userID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrGovernanceForbidden
	}
	return nil
}

func (s *ConditionSetApplicationService) load(id int) (*models.ConditionSet, error) {
	var item models.ConditionSet
	var description sql.NullString
	err := s.db.QueryRow(`SELECT cs.id, cs.name, cs.description, cs.workflow_id, cs.created_at, cs.updated_at, w.name FROM condition_sets cs JOIN workflows w ON cs.workflow_id = w.id WHERE cs.id = ?`, id).Scan(&item.ID, &item.Name, &description, &item.WorkflowID, &item.CreatedAt, &item.UpdatedAt, &item.WorkflowName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item.Description = description.String
	rows, err := s.db.Query(`SELECT cst.id, cst.condition_set_id, cst.transition_id, cst.logic_mode, fs.name, ts.name FROM condition_set_transitions cst JOIN workflow_transitions wt ON cst.transition_id = wt.id LEFT JOIN statuses fs ON wt.from_status_id = fs.id JOIN statuses ts ON wt.to_status_id = ts.id WHERE cst.condition_set_id = ? ORDER BY cst.id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	transitions := []models.TransitionCondition{}
	for rows.Next() {
		var transition models.TransitionCondition
		var from sql.NullString
		if err := rows.Scan(&transition.ID, &transition.ConditionSetID, &transition.TransitionID, &transition.LogicMode, &from, &transition.ToStatusName); err != nil {
			return nil, err
		}
		transition.FromStatusName = from.String
		transitions = append(transitions, transition)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range transitions {
		conditionRows, err := s.db.Query(`SELECT id, condition_set_transition_id, condition_type, config, display_order, mode, COALESCE(error_message, '') FROM conditions WHERE condition_set_transition_id = ? ORDER BY display_order, id`, transitions[i].ID)
		if err != nil {
			return nil, err
		}
		conditions := []models.Condition{}
		for conditionRows.Next() {
			var condition models.Condition
			var config string
			if err := conditionRows.Scan(&condition.ID, &condition.ConditionSetTransitionID, &condition.ConditionType, &config, &condition.DisplayOrder, &condition.Mode, &condition.ErrorMessage); err != nil {
				conditionRows.Close()
				return nil, err
			}
			condition.Config = json.RawMessage(config)
			conditions = append(conditions, condition)
		}
		if err := conditionRows.Err(); err != nil {
			conditionRows.Close()
			return nil, err
		}
		conditionRows.Close()
		transitions[i].Conditions = conditions
	}
	item.TransitionConditions = transitions
	return &item, nil
}

func (s *ConditionSetApplicationService) attachGatedTransitions(items []models.ConditionSet) error {
	if len(items) == 0 {
		return nil
	}
	byID := make(map[int]*models.ConditionSet, len(items))
	placeholders := make([]string, len(items))
	args := make([]any, len(items))
	for i := range items {
		byID[items[i].ID] = &items[i]
		placeholders[i] = "?"
		args[i] = items[i].ID
	}
	query := fmt.Sprintf(`SELECT cst.condition_set_id, cst.transition_id, fs.name, ts.name FROM condition_set_transitions cst JOIN workflow_transitions wt ON wt.id = cst.transition_id LEFT JOIN statuses fs ON fs.id = wt.from_status_id JOIN statuses ts ON ts.id = wt.to_status_id WHERE cst.condition_set_id IN (%s) ORDER BY cst.condition_set_id, cst.id`, strings.Join(placeholders, ","))
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var setID int
		var summary models.ConditionSetTransitionSummary
		var from sql.NullString
		if err := rows.Scan(&setID, &summary.TransitionID, &from, &summary.ToStatusName); err != nil {
			return err
		}
		summary.FromStatusName = from.String
		byID[setID].GatedTransitions = append(byID[setID].GatedTransitions, summary)
	}
	return rows.Err()
}

func (s *ConditionSetApplicationService) validateTransitions(workflowID int, transitions []models.TransitionCondition) error {
	for _, transition := range transitions {
		if transition.TransitionID <= 0 {
			return governanceValidation("transition_id is required")
		}
		if transition.LogicMode != "and" && transition.LogicMode != "or" {
			return governanceValidation("logic_mode must be and or or")
		}
		var belongs bool
		if err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workflow_transitions WHERE id = ? AND workflow_id = ?)", transition.TransitionID, workflowID).Scan(&belongs); err != nil {
			return err
		}
		if !belongs {
			return governanceValidation("transition does not belong to workflow")
		}
		for _, condition := range transition.Conditions {
			if err := validateConditionInput(condition); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateConditionInput(condition models.Condition) error {
	if condition.Mode != "" && condition.Mode != models.ConditionModeCondition && condition.Mode != models.ConditionModeValidator {
		return governanceValidation("condition mode must be condition or validator")
	}
	switch condition.ConditionType {
	case models.ConditionTypeUserInRole:
		var config models.ConditionUserInRoleConfig
		if json.Unmarshal(condition.Config, &config) != nil {
			return governanceValidation("invalid user_in_role config")
		}
		if err := validateConditionFieldRef(config.FieldRef, "user_in_role"); err != nil {
			return err
		}
		if config.RoleID == 0 {
			return governanceValidation("user_in_role requires role_id")
		}
	case models.ConditionTypeUserInGroup:
		var config models.ConditionUserInGroupConfig
		if json.Unmarshal(condition.Config, &config) != nil {
			return governanceValidation("invalid user_in_group config")
		}
		if err := validateConditionFieldRef(config.FieldRef, "user_in_group"); err != nil {
			return err
		}
		if config.GroupID == 0 {
			return governanceValidation("user_in_group requires group_id")
		}
	case models.ConditionTypeFieldValue:
		var config models.ConditionFieldValueConfig
		if json.Unmarshal(condition.Config, &config) != nil || config.FieldIdentifier == "" || config.Pattern == "" {
			return governanceValidation("field_value requires field_identifier and pattern")
		}
	case models.ConditionTypeScript:
		var config models.ConditionScriptConfig
		if json.Unmarshal(condition.Config, &config) != nil || config.Script == "" {
			return governanceValidation("script condition requires script")
		}
		if len(config.Script) > 10240 {
			return governanceValidation("script exceeds 10 KiB")
		}
	default:
		return governanceValidation("unknown condition type: " + condition.ConditionType)
	}
	return nil
}

func validateConditionFieldRef(ref models.FieldRef, conditionType string) error {
	switch ref.Source {
	case models.ApprovalSourceCurrentUser, models.ApprovalSourceCreator, models.ApprovalSourceAssignee:
	case models.ApprovalSourceRegularField:
		if _, ok := models.AllowedRegularApproverFields[ref.FieldIdentifier]; !ok {
			return governanceValidation(conditionType + ": invalid regular field")
		}
	case models.ApprovalSourceCustomField:
		if ref.FieldID == nil || *ref.FieldID == 0 {
			return governanceValidation(conditionType + ": custom field requires field_id")
		}
	default:
		return governanceValidation(conditionType + ": invalid source")
	}
	return nil
}

func saveTransitionConditions(tx database.Tx, conditionSetID int, transitions []models.TransitionCondition) error {
	for _, transition := range transitions {
		var transitionID int
		if err := tx.QueryRow(`INSERT INTO condition_set_transitions (condition_set_id, transition_id, logic_mode, created_at) VALUES (?, ?, ?, ?) RETURNING id`, conditionSetID, transition.TransitionID, transition.LogicMode, time.Now()).Scan(&transitionID); err != nil {
			return err
		}
		for _, condition := range transition.Conditions {
			config, err := json.Marshal(condition.Config)
			if err != nil {
				return err
			}
			mode := condition.Mode
			if mode == "" {
				mode = models.ConditionModeCondition
			}
			var message *string
			if condition.ErrorMessage != "" {
				message = &condition.ErrorMessage
			}
			if _, err := tx.Exec(`INSERT INTO conditions (condition_set_transition_id, condition_type, config, display_order, mode, error_message, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, transitionID, condition.ConditionType, string(config), condition.DisplayOrder, mode, message, time.Now()); err != nil {
				return err
			}
		}
	}
	return nil
}

func scanConditionSets(rows *sql.Rows) ([]models.ConditionSet, error) {
	items := []models.ConditionSet{}
	for rows.Next() {
		var item models.ConditionSet
		var description sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &description, &item.WorkflowID, &item.CreatedAt, &item.UpdatedAt, &item.WorkflowName); err != nil {
			return nil, err
		}
		item.Description = description.String
		items = append(items, item)
	}
	return items, rows.Err()
}
func prepareConditionSet(item *models.ConditionSet) {
	sanitize.ApplyAll(sanitize.Pair{Target: &item.Name, Policy: sanitize.PlainTextField}, sanitize.Pair{Target: &item.Description, Policy: sanitize.RichText})
}
