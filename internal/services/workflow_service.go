package services

import (
	"database/sql"
	"fmt"

	"windshift/internal/database"
)

// WorkflowService provides centralized workflow lookup logic with proper fallback chain
type WorkflowService struct {
	db database.Database
}

// NewWorkflowService creates a new workflow service
func NewWorkflowService(db database.Database) *WorkflowService {
	return &WorkflowService{db: db}
}

// GetWorkflowIDForItem returns workflow ID with proper fallback chain:
// 1. Item type-specific override (configuration_set_item_types.workflow_id) - can be NULL
// 2. Config set default (configuration_sets.workflow_id) - can be NULL
// 3. Global default workflow (workflows.is_default = true) - final fallback
//
// Returns nil only if NO workflow exists at any level
func (s *WorkflowService) GetWorkflowIDForItem(workspaceID int, itemTypeID *int) (*int, error) {
	var workflowID *int

	// Try item type + config set workflow (COALESCE handles item type workflow being NULL)
	if itemTypeID != nil {
		err := s.db.QueryRow(`
			SELECT COALESCE(csit.workflow_id, cs.workflow_id) as workflow_id
			FROM workspace_configuration_sets wcs
			JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
			LEFT JOIN configuration_set_item_types csit
				ON cs.id = csit.configuration_set_id AND csit.item_type_id = ?
			WHERE wcs.workspace_id = ?
		`, *itemTypeID, workspaceID).Scan(&workflowID)

		if err == nil && workflowID != nil {
			return workflowID, nil
		}
		// Continue to fallback if error or NULL result
	}

	// Try config set default workflow (no item type consideration)
	err := s.db.QueryRow(`
		SELECT cs.workflow_id
		FROM workspace_configuration_sets wcs
		JOIN configuration_sets cs ON wcs.configuration_set_id = cs.id
		WHERE wcs.workspace_id = ?
	`, workspaceID).Scan(&workflowID)

	if err == nil && workflowID != nil {
		return workflowID, nil
	}

	// Final fallback: global default workflow
	var defaultID int
	err = s.db.QueryRow(`SELECT id FROM workflows WHERE is_default = true LIMIT 1`).Scan(&defaultID)
	if err == nil {
		return &defaultID, nil
	}

	// No workflow configured anywhere
	return nil, nil
}

// IsValidStatusTransition checks if a status transition is allowed by the workflow
// Uses the full fallback chain to determine the correct workflow
func (s *WorkflowService) IsValidStatusTransition(workspaceID int, itemTypeID *int, fromStatusID, toStatusID int64) (bool, error) {
	// Same status is always valid
	if fromStatusID == toStatusID {
		return true, nil
	}

	// Get the workflow using proper fallback chain
	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return false, err
	}

	// No workflow configured - allow any transition
	if workflowID == nil {
		return true, nil
	}

	// Check if the transition exists in the workflow
	var exists bool
	err = s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM workflow_transitions
			WHERE workflow_id = ? AND from_status_id = ? AND to_status_id = ?
		)
	`, *workflowID, fromStatusID, toStatusID).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("failed to check transition: %w", err)
	}

	return exists, nil
}

// GetAvailableTransitions returns all valid status transitions from the current status
// Uses the full fallback chain to determine the correct workflow
func (s *WorkflowService) GetAvailableTransitions(workspaceID int, itemTypeID *int, currentStatusID int64) ([]StatusTransition, error) {
	// Get the workflow using proper fallback chain
	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}

	// No workflow configured - return empty (caller should handle this)
	if workflowID == nil {
		return []StatusTransition{}, nil
	}

	// Get valid transitions from current status
	rows, err := s.db.Query(`
		SELECT s.id, s.name, sc.color
		FROM workflow_transitions wt
		JOIN statuses s ON wt.to_status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE wt.workflow_id = ? AND wt.from_status_id = ?
	`, *workflowID, currentStatusID)

	if err != nil {
		return nil, fmt.Errorf("failed to query transitions: %w", err)
	}
	defer rows.Close()

	var transitions []StatusTransition
	for rows.Next() {
		var t StatusTransition
		var color sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &color); err != nil {
			continue
		}
		if color.Valid {
			t.CategoryColor = color.String
		}
		transitions = append(transitions, t)
	}

	return transitions, nil
}

// StatusTransition represents a valid status transition
type StatusTransition struct {
	ID            int
	Name          string
	CategoryColor string
}

// GetInitialStatusID returns the initial status ID for a workflow
// The initial status is identified by from_status_id IS NULL in workflow_transitions
func (s *WorkflowService) GetInitialStatusID(workflowID int) (*int, error) {
	var statusID int
	err := s.db.QueryRow(`
		SELECT wt.to_status_id
		FROM workflow_transitions wt
		WHERE wt.workflow_id = ?
		  AND wt.from_status_id IS NULL
		ORDER BY wt.display_order ASC
		LIMIT 1
	`, workflowID).Scan(&statusID)

	if err == sql.ErrNoRows {
		return nil, nil // No initial status configured for this workflow
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query initial status: %w", err)
	}

	return &statusID, nil
}
