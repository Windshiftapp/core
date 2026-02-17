package services

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"windshift/internal/database"
)

// initialStatusCacheEntry holds a cached initial status ID with expiry
type initialStatusCacheEntry struct {
	statusID  *int
	expiresAt time.Time
}

const initialStatusCacheTTL = 5 * time.Minute

// WorkflowService provides centralized workflow lookup logic with proper fallback chain
type WorkflowService struct {
	db                 database.Database
	initialStatusCache sync.Map // key: string "ws:{id}:it:{id|nil}" → value: *initialStatusCacheEntry
}

// NewWorkflowService creates a new workflow service
func NewWorkflowService(db database.Database) *WorkflowService {
	return &WorkflowService{db: db}
}

// GetInitialStatusIDCached returns the initial status ID for a workspace+itemType,
// using an in-memory cache to avoid repeated DB lookups.
func (s *WorkflowService) GetInitialStatusIDCached(workspaceID int, itemTypeID *int) (*int, error) {
	key := initialStatusCacheKey(workspaceID, itemTypeID)

	// Check cache
	if val, ok := s.initialStatusCache.Load(key); ok {
		entry := val.(*initialStatusCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.statusID, nil
		}
		// Expired, delete and fall through
		s.initialStatusCache.Delete(key)
	}

	// Cache miss: resolve via DB
	workflowID, err := s.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}

	var statusID *int
	if workflowID != nil {
		statusID, err = s.GetInitialStatusID(*workflowID)
		if err != nil {
			return nil, err
		}
	}

	// Store in cache
	s.initialStatusCache.Store(key, &initialStatusCacheEntry{
		statusID:  statusID,
		expiresAt: time.Now().Add(initialStatusCacheTTL),
	})

	return statusID, nil
}

// InvalidateInitialStatusCache clears the initial status cache.
// Call this when workflow configuration changes.
func (s *WorkflowService) InvalidateInitialStatusCache() {
	s.initialStatusCache.Range(func(key, _ any) bool {
		s.initialStatusCache.Delete(key)
		return true
	})
}

func initialStatusCacheKey(workspaceID int, itemTypeID *int) string {
	if itemTypeID != nil {
		return fmt.Sprintf("ws:%d:it:%d", workspaceID, *itemTypeID)
	}
	return fmt.Sprintf("ws:%d:it:nil", workspaceID)
}

// GetWorkflowIDForItem returns workflow ID with proper fallback chain:
// 1. Item type-specific override (configuration_set_item_types.workflow_id) - can be NULL
// 2. Config set default (configuration_sets.workflow_id) - can be NULL
// 3. Global default workflow (workflows.is_default = true) - final fallback
//
// Returns nil only if NO workflow exists at any level
func (s *WorkflowService) GetWorkflowIDForItem(workspaceID int, itemTypeID *int) (*int, error) {
	// Personal workspaces are not bound by workflow rules
	var isPersonal bool
	err := s.db.QueryRow(`SELECT is_personal FROM workspaces WHERE id = ?`, workspaceID).Scan(&isPersonal)
	if err == nil && isPersonal {
		return nil, nil
	}

	var workflowID *int

	// Try item type + config set workflow (COALESCE handles item type workflow being NULL)
	if itemTypeID != nil {
		err = s.db.QueryRow(`
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
	err = s.db.QueryRow(`
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

// ========================================
// Read Operations for V1 API
// ========================================

// WorkflowResult represents a workflow for listing/reading.
type WorkflowResult struct {
	ID          int
	Name        string
	Description string
	IsDefault   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkflowTransitionResult represents a workflow transition.
type WorkflowTransitionResult struct {
	ID                int
	FromStatusID      *int
	FromStatusName    string
	FromCategoryName  string
	FromCategoryColor string
	ToStatusID        int
	ToStatusName      string
	ToCategoryName    string
	ToCategoryColor   string
}

// List retrieves all workflows.
func (s *WorkflowService) List() ([]WorkflowResult, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, is_default, created_at, updated_at
		FROM workflows
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}
	defer rows.Close()

	var workflows []WorkflowResult
	for rows.Next() {
		var wf WorkflowResult
		var description sql.NullString
		err := rows.Scan(&wf.ID, &wf.Name, &description, &wf.IsDefault, &wf.CreatedAt, &wf.UpdatedAt)
		if err != nil {
			continue
		}
		wf.Description = description.String
		workflows = append(workflows, wf)
	}

	if workflows == nil {
		workflows = []WorkflowResult{}
	}

	return workflows, nil
}

// GetByID retrieves a workflow by ID.
func (s *WorkflowService) GetByID(id int) (*WorkflowResult, error) {
	var wf WorkflowResult
	var description sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, description, is_default, created_at, updated_at
		FROM workflows WHERE id = ?
	`, id).Scan(&wf.ID, &wf.Name, &description, &wf.IsDefault, &wf.CreatedAt, &wf.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}

	wf.Description = description.String
	return &wf, nil
}

// Exists checks if a workflow exists.
func (s *WorkflowService) Exists(id int) (bool, error) {
	var exists int
	err := s.db.QueryRow("SELECT 1 FROM workflows WHERE id = ?", id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check workflow: %w", err)
	}
	return true, nil
}

// GetTransitions retrieves all transitions for a workflow.
func (s *WorkflowService) GetTransitions(workflowID int) ([]WorkflowTransitionResult, error) {
	rows, err := s.db.Query(`
		SELECT wt.id, wt.from_status_id, wt.to_status_id,
		       fs.name as from_status_name, ts.name as to_status_name,
		       fsc.name as from_category_name, fsc.color as from_category_color,
		       tsc.name as to_category_name, tsc.color as to_category_color
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		LEFT JOIN status_categories fsc ON fs.category_id = fsc.id
		JOIN status_categories tsc ON ts.category_id = tsc.id
		WHERE wt.workflow_id = ?
		ORDER BY wt.display_order
	`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transitions: %w", err)
	}
	defer rows.Close()

	var transitions []WorkflowTransitionResult
	for rows.Next() {
		var t WorkflowTransitionResult
		var fromStatusID sql.NullInt64
		var fromStatusName, fromCategoryName, fromCategoryColor sql.NullString

		err := rows.Scan(&t.ID, &fromStatusID, &t.ToStatusID,
			&fromStatusName, &t.ToStatusName,
			&fromCategoryName, &fromCategoryColor,
			&t.ToCategoryName, &t.ToCategoryColor)
		if err != nil {
			continue
		}

		if fromStatusID.Valid {
			id := int(fromStatusID.Int64)
			t.FromStatusID = &id
			t.FromStatusName = fromStatusName.String
			t.FromCategoryName = fromCategoryName.String
			t.FromCategoryColor = fromCategoryColor.String
		}

		transitions = append(transitions, t)
	}

	if transitions == nil {
		transitions = []WorkflowTransitionResult{}
	}

	return transitions, nil
}

// GetTransitionsFromStatus retrieves available transitions from a given status ID.
// This queries transitions where from_status_id matches the given status OR from_status_id IS NULL (initial transitions).
// Used by the V1 API to show available status transitions for an item.
func (s *WorkflowService) GetTransitionsFromStatus(statusID int) ([]WorkflowTransitionResult, error) {
	rows, err := s.db.Query(`
		SELECT wt.id, wt.from_status_id, wt.to_status_id,
		       fs.name as from_status_name, ts.name as to_status_name,
		       fsc.name as from_category_name, fsc.color as from_category_color,
		       tsc.name as to_category_name, tsc.color as to_category_color
		FROM workflow_transitions wt
		LEFT JOIN statuses fs ON wt.from_status_id = fs.id
		JOIN statuses ts ON wt.to_status_id = ts.id
		LEFT JOIN status_categories fsc ON fs.category_id = fsc.id
		JOIN status_categories tsc ON ts.category_id = tsc.id
		WHERE wt.from_status_id = ? OR wt.from_status_id IS NULL
		ORDER BY wt.display_order
	`, statusID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transitions from status: %w", err)
	}
	defer rows.Close()

	var transitions []WorkflowTransitionResult
	for rows.Next() {
		var t WorkflowTransitionResult
		var fromStatusID sql.NullInt64
		var fromStatusName, fromCategoryName, fromCategoryColor sql.NullString

		err := rows.Scan(&t.ID, &fromStatusID, &t.ToStatusID,
			&fromStatusName, &t.ToStatusName,
			&fromCategoryName, &fromCategoryColor,
			&t.ToCategoryName, &t.ToCategoryColor)
		if err != nil {
			continue
		}

		if fromStatusID.Valid {
			id := int(fromStatusID.Int64)
			t.FromStatusID = &id
			t.FromStatusName = fromStatusName.String
			t.FromCategoryName = fromCategoryName.String
			t.FromCategoryColor = fromCategoryColor.String
		}

		transitions = append(transitions, t)
	}

	if transitions == nil {
		transitions = []WorkflowTransitionResult{}
	}

	return transitions, nil
}
