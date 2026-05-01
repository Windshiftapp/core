package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ConditionService evaluates workflow transition conditions.
type ConditionService struct {
	db           database.Database
	permService  *PermissionService
	scriptEngine *ScriptEngine
}

// NewConditionService creates a new condition service.
func NewConditionService(db database.Database, permService *PermissionService, scriptEngine *ScriptEngine) *ConditionService {
	return &ConditionService{
		db:           db,
		permService:  permService,
		scriptEngine: scriptEngine,
	}
}

// conditionRow represents a condition loaded from the database with its parent transition info.
type conditionRow struct {
	TransitionID  int
	LogicMode     string
	ConditionType string
	Config        string
	Mode          string
	ErrorMessage  string
}

// EvaluateTransitionConditions checks if a user is allowed to perform a specific transition
// within a condition set. Only conditions whose mode is listed in `modes` are considered.
// Returns (allowed, failureMessage, error). failureMessage is the error_message from the
// first failing condition (if set).
func (s *ConditionService) EvaluateTransitionConditions(ctx context.Context, conditionSetID, transitionID, userID int, item map[string]interface{}, modes []string) (allowed bool, failureMessage string, err error) {
	if len(modes) == 0 {
		return true, "", nil
	}

	placeholders := strings.Repeat("?,", len(modes))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, 0, 2+len(modes))
	args = append(args, conditionSetID, transitionID)
	for _, m := range modes {
		args = append(args, m)
	}

	query := fmt.Sprintf(`
		SELECT cst.transition_id, cst.logic_mode, c.condition_type, c.config, c.mode, COALESCE(c.error_message, '')
		FROM condition_set_transitions cst
		JOIN conditions c ON c.condition_set_transition_id = cst.id
		WHERE cst.condition_set_id = ? AND cst.transition_id = ? AND c.mode IN (%s)
		ORDER BY c.display_order, c.id
	`, placeholders)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return false, "", fmt.Errorf("failed to load conditions: %w", err)
	}
	defer rows.Close()

	var conditions []conditionRow
	var logicMode string
	for rows.Next() {
		var cr conditionRow
		if err := rows.Scan(&cr.TransitionID, &cr.LogicMode, &cr.ConditionType, &cr.Config, &cr.Mode, &cr.ErrorMessage); err != nil {
			return false, "", fmt.Errorf("failed to scan condition: %w", err)
		}
		logicMode = cr.LogicMode
		conditions = append(conditions, cr)
	}

	// No conditions matching the requested modes for this transition = allowed
	if len(conditions) == 0 {
		return true, "", nil
	}

	return s.evaluateConditions(ctx, conditions, logicMode, userID, item)
}

// FilterTransitionsByConditions filters a list of transitions, returning only those
// the user is allowed to perform given the condition set.
func (s *ConditionService) FilterTransitionsByConditions(ctx context.Context, conditionSetID int, transitions []TransitionWithID, userID int, item map[string]interface{}) ([]TransitionWithID, error) {
	// Load only condition-mode rules (validators are checked at transition time, not filtering)
	rows, err := s.db.Query(`
		SELECT cst.transition_id, cst.logic_mode, c.condition_type, c.config, c.mode, COALESCE(c.error_message, '')
		FROM condition_set_transitions cst
		JOIN conditions c ON c.condition_set_transition_id = cst.id
		WHERE cst.condition_set_id = ? AND c.mode = 'condition'
		ORDER BY cst.transition_id, c.display_order, c.id
	`, conditionSetID)
	if err != nil {
		return nil, fmt.Errorf("failed to load conditions: %w", err)
	}
	defer rows.Close()

	// Group conditions by transition ID
	type transitionConditions struct {
		logicMode  string
		conditions []conditionRow
	}
	condsByTransition := map[int]*transitionConditions{}

	for rows.Next() {
		var cr conditionRow
		if err := rows.Scan(&cr.TransitionID, &cr.LogicMode, &cr.ConditionType, &cr.Config, &cr.Mode, &cr.ErrorMessage); err != nil {
			return nil, fmt.Errorf("failed to scan condition: %w", err)
		}
		tc, ok := condsByTransition[cr.TransitionID]
		if !ok {
			tc = &transitionConditions{logicMode: cr.LogicMode}
			condsByTransition[cr.TransitionID] = tc
		}
		tc.conditions = append(tc.conditions, cr)
	}

	var filtered []TransitionWithID
	for _, t := range transitions {
		tc, hasConds := condsByTransition[t.TransitionID]
		if !hasConds {
			// No conditions for this transition = allowed
			filtered = append(filtered, t)
			continue
		}

		allowed, _, err := s.evaluateConditions(ctx, tc.conditions, tc.logicMode, userID, item)
		if err != nil {
			return nil, err
		}
		if allowed {
			filtered = append(filtered, t)
		}
	}

	return filtered, nil
}

// TransitionWithID carries both the transition ID and the status info for filtering.
type TransitionWithID struct {
	TransitionID  int
	StatusID      int
	StatusName    string
	CategoryColor string
}

func (s *ConditionService) evaluateConditions(ctx context.Context, conditions []conditionRow, logicMode string, userID int, item map[string]interface{}) (allowed bool, failureMessage string, err error) {
	if logicMode == "or" {
		// OR: any condition passing = allowed
		var lastFailMessage string
		for _, c := range conditions {
			result, err := s.evaluateCondition(ctx, c, userID, item)
			if err != nil {
				return false, "", err
			}
			if result {
				return true, "", nil
			}
			if c.ErrorMessage != "" {
				lastFailMessage = c.ErrorMessage
			}
		}
		return false, lastFailMessage, nil
	}

	// AND (default): all conditions must pass
	for _, c := range conditions {
		result, err := s.evaluateCondition(ctx, c, userID, item)
		if err != nil {
			return false, "", err
		}
		if !result {
			return false, c.ErrorMessage, nil
		}
	}
	return true, "", nil
}

func (s *ConditionService) evaluateCondition(ctx context.Context, c conditionRow, userID int, item map[string]interface{}) (bool, error) {
	switch c.ConditionType {
	case models.ConditionTypeUserInRole:
		return s.evaluateUserInRole(c.Config, userID, item)
	case models.ConditionTypeUserInGroup:
		return s.evaluateUserInGroup(c.Config, userID, item)
	case models.ConditionTypeFieldValue:
		return s.evaluateFieldValue(c.Config, item)
	case models.ConditionTypeScript:
		return s.evaluateScript(ctx, c.Config, userID, item)
	default:
		return false, fmt.Errorf("unknown condition type: %s", c.ConditionType)
	}
}

// resolveUserID determines which user to evaluate based on a FieldRef. Source
// vocabulary is shared with approvals: 'current_user' | 'creator' | 'assignee' |
// 'regular_field' | 'custom_field'.
func resolveUserID(ref models.FieldRef, currentUserID int, item map[string]interface{}) (int, error) {
	switch ref.Source {
	case models.ApprovalSourceCurrentUser:
		return currentUserID, nil
	case models.ApprovalSourceCreator:
		id, ok := toInt(item["creator_id"])
		if !ok {
			return 0, fmt.Errorf("item has no creator")
		}
		return id, nil
	case models.ApprovalSourceAssignee:
		id, ok := toInt(item["assignee_id"])
		if !ok {
			return 0, fmt.Errorf("item has no assignee")
		}
		return id, nil
	case models.ApprovalSourceRegularField:
		if _, ok := models.AllowedRegularApproverFields[ref.FieldIdentifier]; !ok {
			return 0, fmt.Errorf("regular_field %q not in user-field whitelist", ref.FieldIdentifier)
		}
		id, ok := toInt(item[ref.FieldIdentifier])
		if !ok {
			return 0, fmt.Errorf("regular field %q is not set or not a user id", ref.FieldIdentifier)
		}
		return id, nil
	case models.ApprovalSourceCustomField:
		if ref.FieldID == nil {
			return 0, fmt.Errorf("field_id required for custom_field source")
		}
		cfv, ok := item["custom_fields"].(map[string]interface{})
		if !ok {
			return 0, fmt.Errorf("no custom fields on item")
		}
		val, exists := cfv[fmt.Sprintf("%d", *ref.FieldID)]
		if !exists {
			return 0, fmt.Errorf("custom field %d not set", *ref.FieldID)
		}
		id, ok := toInt(val)
		if !ok {
			return 0, fmt.Errorf("custom field %d is not a user ID", *ref.FieldID)
		}
		return id, nil
	default:
		return 0, fmt.Errorf("unknown source: %s", ref.Source)
	}
}

func (s *ConditionService) evaluateUserInRole(configJSON string, userID int, item map[string]interface{}) (bool, error) {
	var cfg models.ConditionUserInRoleConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return false, fmt.Errorf("invalid user_in_role config: %w", err)
	}

	evalUserID, err := resolveUserID(cfg.FieldRef, userID, item)
	if err != nil {
		return false, nil //nolint:nilerr // unresolvable user means condition fails
	}

	workspaceID, ok := toInt(item["workspace_id"])
	if !ok {
		return false, fmt.Errorf("item missing workspace_id")
	}

	return s.permService.HasWorkspaceRole(evalUserID, workspaceID, cfg.RoleID)
}

func (s *ConditionService) evaluateUserInGroup(configJSON string, userID int, item map[string]interface{}) (bool, error) {
	var cfg models.ConditionUserInGroupConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return false, fmt.Errorf("invalid user_in_group config: %w", err)
	}

	evalUserID, err := resolveUserID(cfg.FieldRef, userID, item)
	if err != nil {
		return false, nil //nolint:nilerr // unresolvable user means condition fails
	}

	memberships, err := s.permService.GetGroupMemberships(evalUserID)
	if err != nil {
		return false, fmt.Errorf("failed to get group memberships: %w", err)
	}

	for _, gid := range memberships {
		if gid == cfg.GroupID {
			return true, nil
		}
	}
	return false, nil
}

func (s *ConditionService) evaluateFieldValue(configJSON string, item map[string]interface{}) (bool, error) {
	var cfg models.ConditionFieldValueConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return false, fmt.Errorf("invalid field_value config: %w", err)
	}

	fieldValue := fmt.Sprintf("%v", item[cfg.FieldIdentifier])
	re, err := regexp.Compile(cfg.Pattern)
	if err != nil {
		return false, fmt.Errorf("invalid regex pattern: %w", err)
	}

	return re.MatchString(fieldValue), nil
}

func (s *ConditionService) evaluateScript(ctx context.Context, configJSON string, userID int, item map[string]interface{}) (bool, error) {
	var cfg models.ConditionScriptConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return false, fmt.Errorf("invalid script config: %w", err)
	}

	vars := map[string]interface{}{
		"item":    item,
		"user_id": userID,
	}

	return s.scriptEngine.ExecuteBool(ctx, cfg.Script, vars, cfg.TimeoutMs)
}

// GetConditionSetIDForItem returns the condition set ID for an item's workspace/item type,
// using the same fallback chain as workflows: item type override -> config set default -> nil.
func (s *ConditionService) GetConditionSetIDForItem(workspaceID int, itemTypeID *int) (*int, error) {
	// Personal workspaces have no conditions
	var isPersonal bool
	err := s.db.QueryRow(`SELECT is_personal FROM workspaces WHERE id = ?`, workspaceID).Scan(&isPersonal)
	if err == nil && isPersonal {
		return nil, nil
	}

	var conditionSetID *int

	// Try item type override first
	if itemTypeID != nil {
		err = s.db.QueryRow(`
			SELECT COALESCE(csit.condition_set_id, cs.condition_set_id) as condition_set_id
			FROM workspace_configuration_sets wcs
			JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
			LEFT JOIN configuration_set_item_types csit
				ON cs.id = csit.configuration_set_id AND csit.item_type_id = ?
			WHERE wcs.workspace_id = ?
		`, *itemTypeID, workspaceID).Scan(&conditionSetID)

		if err == nil && conditionSetID != nil {
			return conditionSetID, nil
		}
	}

	// Config set default
	err = s.db.QueryRow(`
		SELECT cs.condition_set_id
		FROM workspace_configuration_sets wcs
		JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
		WHERE wcs.workspace_id = ?
	`, workspaceID).Scan(&conditionSetID)

	if err == nil && conditionSetID != nil {
		return conditionSetID, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// Fallback: default config set with item type override
	if itemTypeID != nil {
		err = s.db.QueryRow(`
			SELECT COALESCE(csit.condition_set_id, cs.condition_set_id) as condition_set_id
			FROM configuration_sets cs
			LEFT JOIN configuration_set_item_types csit
				ON cs.id = csit.configuration_set_id AND csit.item_type_id = ?
			WHERE cs.is_default = true
		`, *itemTypeID).Scan(&conditionSetID)

		if err == nil && conditionSetID != nil {
			return conditionSetID, nil
		}
	}

	// Fallback: default config set
	err = s.db.QueryRow(`
		SELECT condition_set_id FROM configuration_sets WHERE is_default = true
	`).Scan(&conditionSetID)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	return conditionSetID, nil
}

// toInt converts an interface{} to int, returning 0 if not possible.
func toInt(v interface{}) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	case nil:
		return 0, false
	default:
		return 0, false
	}
}
