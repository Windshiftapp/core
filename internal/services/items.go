package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
)

// mapTextStatusToID maps legacy text status values to status IDs
// Returns nil if the status cannot be mapped
// Default status IDs from database setup:
// 1: "Open", 2: "To Do", 3: "In Progress", 4: "Under Review", 5: "Done", 6: "Closed"
func mapTextStatusToID(status string) *int {
	normalized := strings.ToLower(strings.TrimSpace(status))
	normalized = strings.ReplaceAll(normalized, "_", " ") // Handle in_progress -> in progress

	switch normalized {
	case "open":
		id := 1
		return &id
	case "to do", "todo":
		id := 2
		return &id
	case "in progress", "inprogress", "in-progress":
		id := 3
		return &id
	case "under review", "review":
		id := 4
		return &id
	case "done", "completed":
		id := 5
		return &id
	case "closed":
		id := 6
		return &id
	default:
		return nil
	}
}

// mapTextPriorityToID maps legacy text priority values to priority IDs
// Returns nil if the priority cannot be mapped
// Default priority IDs from database setup:
// 1: "Low", 2: "Medium", 3: "High", 4: "Critical"
func mapTextPriorityToID(priority string) *int {
	normalized := strings.ToLower(strings.TrimSpace(priority))

	switch normalized {
	case "low":
		id := 1
		return &id
	case "medium":
		id := 2
		return &id
	case "high":
		id := 3
		return &id
	case "critical", "urgent":
		id := 4
		return &id
	default:
		return nil
	}
}

// ItemCreationParams contains all parameters for creating an item
type ItemCreationParams struct {
	WorkspaceID             int
	Title                   string
	Description             string
	Status                  string // Text status (legacy) - mapped to StatusID if StatusID is nil
	StatusID                *int   // Direct status ID - takes precedence over Status text
	ItemTypeID              *int
	Priority                string // Text priority (legacy) - mapped to PriorityID if PriorityID is nil
	PriorityID              *int   // Direct priority ID - takes precedence over Priority text
	IsTask                  bool
	ParentID                *int
	MilestoneID             *int
	IterationID             *int
	ProjectID               *int
	InheritProject          bool
	TimeProjectID           *int
	AssigneeID              *int
	ReporterID              *int   // Reporter/submitter of the item
	CreatorID               *int
	CreatorPortalCustomerID *int
	ChannelID               *int       // Portal-specific: track portal/channel
	RequestTypeID           *int       // Portal-specific: track request type
	DueDate                 *time.Time // Due date for the item
	RelatedWorkItemID       *int       // For personal tasks: related work item
	CustomFieldValuesJSON   string     // JSON string of custom field values
}

// CreateItem creates a new item with proper transaction handling and number generation
// This centralizes the item creation logic used by normal creation, portal submissions, and copying
func CreateItem(db database.Database, params ItemCreationParams) (int64, error) {
	now := time.Now()

	// Generate fractional index for manual ordering
	fracIndex, err := GenerateFracIndexForNewItem(db.GetDB(), params.WorkspaceID, params.ParentID)
	if err != nil {
		return 0, fmt.Errorf("failed to generate frac_index: %w", err)
	}

	// Start transaction for atomic item creation
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Get next workspace-specific item number (within transaction to prevent race conditions)
	var nextWorkspaceItemNumber int
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(workspace_item_number), 0) + 1
		FROM items
		WHERE workspace_id = ?
	`, params.WorkspaceID).Scan(&nextWorkspaceItemNumber)
	if err != nil {
		return 0, fmt.Errorf("failed to generate workspace item number: %w", err)
	}

	// Resolve status ID: direct ID takes precedence, then text mapping, then workflow initial status
	var statusID *int
	if params.StatusID != nil {
		statusID = params.StatusID
	} else if params.Status != "" {
		statusID = mapTextStatusToID(params.Status)
	}

	// If status is still nil, resolve from workflow initial status
	if statusID == nil {
		workflowService := NewWorkflowService(db)
		workflowID, err := workflowService.GetWorkflowIDForItem(params.WorkspaceID, params.ItemTypeID)
		if err == nil && workflowID != nil {
			initialStatusID, err := workflowService.GetInitialStatusID(*workflowID)
			if err == nil && initialStatusID != nil {
				statusID = initialStatusID
			}
		}
	}

	// Resolve priority ID: direct ID takes precedence, then text mapping, then default priority
	var priorityID *int
	if params.PriorityID != nil {
		priorityID = params.PriorityID
	} else if params.Priority != "" {
		priorityID = mapTextPriorityToID(params.Priority)
	}

	// If priority is still nil, get the default priority
	if priorityID == nil {
		var defaultPriorityID int
		err := db.QueryRow("SELECT id FROM priorities WHERE is_default = true LIMIT 1").Scan(&defaultPriorityID)
		if err == nil {
			priorityID = &defaultPriorityID
		}
	}

	// Insert item with all fields
	// Note: Uses RETURNING id for both SQLite (3.35+) and PostgreSQL
	insertQuery := `
		INSERT INTO items (
			workspace_id, workspace_item_number, item_type_id, title, description, status_id, priority_id, is_task,
			milestone_id, iteration_id, project_id, inherit_project, time_project_id, assignee_id, reporter_id, creator_id, creator_portal_customer_id,
			channel_id, request_type_id, due_date, related_work_item_id,
			custom_field_values, parent_id,
			frac_index, created_at, updated_at, path
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`

	var itemID int64
	err = tx.QueryRow(insertQuery,
		params.WorkspaceID,
		nextWorkspaceItemNumber,
		params.ItemTypeID,
		params.Title,
		params.Description,
		statusID,
		priorityID,
		params.IsTask,
		params.MilestoneID,
		params.IterationID,
		params.ProjectID,
		params.InheritProject,
		params.TimeProjectID,
		params.AssigneeID,
		params.ReporterID,
		params.CreatorID,
		params.CreatorPortalCustomerID,
		params.ChannelID,
		params.RequestTypeID,
		params.DueDate,
		params.RelatedWorkItemID,
		nullString(params.CustomFieldValuesJSON),
		params.ParentID,
		fracIndex,
		now,
		now,
		"/", // Initial path, will be updated below
	).Scan(&itemID)

	if err != nil {
		return 0, fmt.Errorf("failed to insert item: %w", err)
	}

	// Update item path to include its own ID
	path := fmt.Sprintf("/%d/", itemID)
	_, err = tx.Exec(`UPDATE items SET path = ? WHERE id = ?`, path, itemID)
	if err != nil {
		return 0, fmt.Errorf("failed to update item path: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Record item creation history if a creator is specified
	if params.CreatorID != nil {
		updateService := NewItemUpdateService(db)
		if err := updateService.recordItemCreationHistory(db, int(itemID), *params.CreatorID); err != nil {
			// Log error but don't fail the request
			// This is a non-critical operation
		}
	}

	return itemID, nil
}

// GetInitialStatusForItemType determines the initial status for an item type
// by querying the workflow assigned to the item type. Uses a two-tier override system:
// 1. First checks if there's a workflow override for this specific item type
// 2. Falls back to the configuration set's default workflow if no override exists
// Returns the status name of the first status in the workflow (where from_status_id IS NULL).
// Returns an error if the item type, configuration set, workflow, or initial status cannot be found.
func GetInitialStatusForItemType(db database.Database, itemTypeID int) (string, error) {
	// First, get the workflow ID with fallback logic:
	// 1. Check for item-type-specific override in configuration_set_item_types
	// 2. If NULL, use configuration set default workflow
	workflowQuery := `
		SELECT COALESCE(csit.workflow_id, cs.workflow_id) as workflow_id
		FROM item_types it
		LEFT JOIN configuration_set_item_types csit ON it.id = csit.item_type_id
		JOIN configuration_sets cs ON (
			csit.configuration_set_id = cs.id OR
			(csit.configuration_set_id IS NULL AND it.configuration_set_id = cs.id)
		)
		WHERE it.id = ?
		LIMIT 1
	`

	var workflowID *int
	err := db.QueryRow(workflowQuery, itemTypeID).Scan(&workflowID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no configuration set or workflow found for item type %d", itemTypeID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query workflow: %w", err)
	}
	if workflowID == nil {
		return "", fmt.Errorf("no workflow assigned for item type %d", itemTypeID)
	}

	// Now get the initial status from the workflow
	// The initial status is identified by from_status_id IS NULL in workflow_transitions
	statusQuery := `
		SELECT s.name
		FROM workflow_transitions wt
		JOIN statuses s ON wt.to_status_id = s.id
		WHERE wt.workflow_id = ?
		  AND wt.from_status_id IS NULL
		ORDER BY wt.display_order ASC
		LIMIT 1
	`

	var statusName string
	err = db.QueryRow(statusQuery, *workflowID).Scan(&statusName)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no initial status found for workflow %d (workflow may not be configured)", *workflowID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query initial status: %w", err)
	}

	return statusName, nil
}

// nullString converts an empty string to sql.NullString
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}
