package jiraimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// WorkflowGuard is one source condition or validator attached to a transition.
// A guard with a ConditionType is recreated as a Windshift condition; a guard
// without one is unmappable and keeps the transition locked for review.
type WorkflowGuard struct {
	Kind          string         // normalized source kind, for metadata
	Mode          string         // "condition" or "validator"
	ConditionType string         // Windshift condition type when mappable
	Config        map[string]any // Windshift condition config when mappable
	ErrorMessage  string
}

// Mappable reports whether the guard can be recreated faithfully.
func (g WorkflowGuard) Mappable() bool { return g.ConditionType != "" }

type WorkflowEdge struct {
	FromStatusID *int
	ToStatusID   int
	ReviewLocked bool
	// Guards carries the source conditions/validators when the importer could
	// read them (the DC companion plugin). Empty means only counts are known.
	Guards []WorkflowGuard
}

type WorkflowDefinition struct {
	SourceID    string
	Name        string
	Description string
	Edges       []WorkflowEdge
	ItemTypeIDs []int
	Metadata    map[string]any
}

func (s *Service) CreateDetailedWorkflowConfiguration(
	ctx context.Context,
	jobID, projectKey string,
	workspaceID int,
	definitions []WorkflowDefinition,
) (int, error) {
	type createdWorkflow struct {
		id             int
		conditionSetID *int
		itemTypeIDs    []int
	}
	workflows := make([]createdWorkflow, 0, len(definitions))
	for _, definition := range definitions {
		var workflowID int
		if err := s.db.QueryRow(`
			INSERT INTO workflows (name, description, is_default, created_at, updated_at)
			VALUES (?, ?, false, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
		`, definition.Name, definition.Description).Scan(&workflowID); err != nil {
			return 0, fmt.Errorf("create workflow %q: %w", definition.Name, err)
		}
		var conditionSetID *int
		needsConditionSet := false
		for _, edge := range definition.Edges {
			if edge.ReviewLocked || len(edge.Guards) > 0 {
				needsConditionSet = true
				break
			}
		}
		if needsConditionSet {
			id, err := s.CreateWorkflowReviewConditionSet(workflowID, definition.Name)
			if err != nil {
				return 0, err
			}
			conditionSetID = &id
		}
		for index, edge := range definition.Edges {
			var transitionID int
			if err := s.db.QueryRow(`
				INSERT INTO workflow_transitions
					(workflow_id, from_status_id, to_status_id, display_order, source_handle, target_handle, created_at)
				VALUES (?, ?, ?, ?, '', '', CURRENT_TIMESTAMP) RETURNING id
			`, workflowID, edge.FromStatusID, edge.ToStatusID, index+1).Scan(&transitionID); err != nil {
				return 0, fmt.Errorf("create workflow %q transition: %w", definition.Name, err)
			}
			if conditionSetID == nil {
				continue
			}
			switch {
			case len(edge.Guards) > 0:
				if err := s.ApplyWorkflowGuards(*conditionSetID, transitionID, edge.Guards); err != nil {
					return 0, err
				}
			case edge.ReviewLocked:
				// Counts only (no descriptors): keep the whole transition locked.
				if err := s.LockWorkflowTransitionForReview(*conditionSetID, transitionID); err != nil {
					return 0, err
				}
			}
		}
		if err := s.RecordMapping(jobID, "workflow", definition.SourceID, definition.Name, workflowID, definition.Metadata); err != nil {
			return 0, err
		}
		workflows = append(workflows, createdWorkflow{id: workflowID, conditionSetID: conditionSetID, itemTypeIDs: definition.ItemTypeIDs})
	}
	defaultIndex := 0
	for index := range workflows {
		if len(workflows[index].itemTypeIDs) > len(workflows[defaultIndex].itemTypeIDs) {
			defaultIndex = index
		}
	}
	defaultWorkflowID := workflows[defaultIndex].id
	defaultConditionSetID := workflows[defaultIndex].conditionSetID
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	name := projectKey + " Configuration"
	repo := repository.NewConfigurationSetRepository(s.db)
	configSetID64, err := repo.Create(tx, &models.ConfigurationSet{
		Name: name, WorkflowID: &defaultWorkflowID, ConditionSetID: defaultConditionSetID,
		DifferentiateByItemType: len(workflows) > 1,
	})
	if err != nil {
		return 0, err
	}
	configSetID := int(configSetID64)
	var configs []models.ItemTypeConfig
	for _, workflow := range workflows {
		for _, itemTypeID := range workflow.itemTypeIDs {
			config := models.ItemTypeConfig{ItemTypeID: itemTypeID}
			if workflow.id != defaultWorkflowID {
				id := workflow.id
				config.WorkflowID = &id
			}
			if workflow.conditionSetID != nil && (defaultConditionSetID == nil || *workflow.conditionSetID != *defaultConditionSetID) {
				id := *workflow.conditionSetID
				config.ConditionSetID = &id
			}
			configs = append(configs, config)
		}
	}
	if err := repo.SaveItemTypeConfigs(tx, configSetID, configs); err != nil {
		return 0, err
	}
	if err := repo.SaveWorkspaceAssignments(tx, configSetID, []int{workspaceID}); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if err := s.RecordMapping(jobID, "configuration_set", "cs-"+projectKey, name, configSetID, nil); err != nil {
		return 0, err
	}
	return configSetID, nil
}

func (s *Service) CreateWorkflowReviewConditionSet(workflowID int, workflowName string) (int, error) {
	var conditionSetID int
	err := s.db.QueryRow(`
		INSERT INTO condition_sets (name, description, workflow_id, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`, workflowName+" - Jira Rule Review Required",
		"Generated by Jira import. Transitions with exposed Jira conditions or validators stay locked until an operator reviews them; rules the source API cannot identify are not recreated.",
		workflowID).Scan(&conditionSetID)
	return conditionSetID, err
}

func (s *Service) LockWorkflowTransitionForReview(conditionSetID, transitionID int) error {
	return s.ApplyWorkflowGuards(conditionSetID, transitionID, []WorkflowGuard{{Kind: "unknown"}})
}

// ApplyWorkflowGuards recreates the mappable guards as Windshift conditions and,
// when any guard is unmappable, adds a never-passing validator so the transition
// stays locked for review instead of being silently broadened.
func (s *Service) ApplyWorkflowGuards(conditionSetID, transitionID int, guards []WorkflowGuard) error {
	var linkID int
	if err := s.db.QueryRow(`
		INSERT INTO condition_set_transitions (condition_set_id, transition_id, logic_mode, created_at)
		VALUES (?, ?, 'and', CURRENT_TIMESTAMP) RETURNING id
	`, conditionSetID, transitionID).Scan(&linkID); err != nil {
		return err
	}

	order := 1
	hasUnmapped := false
	for _, guard := range guards {
		if !guard.Mappable() {
			hasUnmapped = true
			continue
		}
		configJSON, err := json.Marshal(guard.Config)
		if err != nil {
			return fmt.Errorf("encode workflow guard config: %w", err)
		}
		mode := guard.Mode
		if mode == "" {
			mode = models.ConditionModeCondition
		}
		if _, err := s.db.ExecWrite(`
			INSERT INTO conditions
				(condition_set_transition_id, condition_type, config, display_order, mode, error_message, created_at)
			VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		`, linkID, guard.ConditionType, string(configJSON), order, mode, guard.ErrorMessage); err != nil {
			return err
		}
		order++
	}

	if hasUnmapped {
		if _, err := s.db.ExecWrite(`
			INSERT INTO conditions
				(condition_set_transition_id, condition_type, config, display_order, mode, error_message, created_at)
			VALUES (?, 'field_value', ?, ?, 'validator', ?, CURRENT_TIMESTAMP)
		`, linkID, `{"field_identifier":"title","pattern":"a\\A"}`, order,
			"Imported Jira transition is locked pending review of source conditions and validators that Windshift cannot map."); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ConservativeInitialStatus(statusIDs []int) (int, error) {
	categories, err := s.statuses.CategoryIDs(statusIDs)
	if err != nil {
		return 0, err
	}
	for _, statusID := range statusIDs {
		if categories[statusID] == 1 {
			return statusID, nil
		}
	}
	if len(statusIDs) == 0 {
		return 0, errors.New("cannot create fallback Jira workflow without statuses")
	}
	return statusIDs[0], nil
}

func (s *Service) AssignConfigurationScreens(
	workspaceID int,
	defaultScreens map[string]int,
	itemTypeScreens map[int][3]*int,
) (int, error) {
	configSetID, err := s.WorkspaceConfigurationSetID(workspaceID)
	if err != nil || configSetID == nil {
		return 0, err
	}
	for contextName, screenID := range defaultScreens {
		if _, err := s.db.ExecWrite(`
			INSERT INTO configuration_set_screens (configuration_set_id, screen_id, context, created_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(configuration_set_id, context) DO UPDATE SET screen_id = excluded.screen_id
		`, *configSetID, screenID, contextName); err != nil {
			return 0, err
		}
	}
	for itemTypeID, screens := range itemTypeScreens {
		if _, err := s.db.ExecWrite(`
			UPDATE configuration_set_item_types
			SET create_screen_id = ?, edit_screen_id = ?, view_screen_id = ?
			WHERE configuration_set_id = ? AND item_type_id = ?
		`, screens[0], screens[1], screens[2], *configSetID, itemTypeID); err != nil {
			return 0, err
		}
	}
	return *configSetID, nil
}

func (s *Service) WorkspaceConfigurationSetID(workspaceID int) (*int, error) {
	return repository.NewConfigurationSetRepository(s.db).GetWorkspaceConfigSetID(workspaceID)
}
