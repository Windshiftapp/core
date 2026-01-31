package services

import (
	"database/sql"
	"fmt"
	"strings"

	"windshift/internal/constants"
	"windshift/internal/database"
)

// ItemValidationParams contains parameters for validating item creation
type ItemValidationParams struct {
	WorkspaceID       int
	Title             string
	ItemTypeID        *int
	ParentID          *int
	StatusID          *int
	IsTask            bool
	RelatedWorkItemID *int
	UserID            int // User creating the item (for personal workspace validation)
}

// ItemValidationResult contains the result of validation
type ItemValidationResult struct {
	Valid bool
	Error string
}

// ValidateItemCreation validates all parameters for creating an item
// Returns a validation result indicating success or failure with error message
func ValidateItemCreation(db database.Database, params ItemValidationParams) *ItemValidationResult {
	// Validate required fields
	if strings.TrimSpace(params.Title) == "" {
		return &ItemValidationResult{Valid: false, Error: "Title is required"}
	}

	// Validate workspace exists
	var workspaceExists bool
	err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", params.WorkspaceID).Scan(&workspaceExists)
	if err != nil {
		return &ItemValidationResult{Valid: false, Error: fmt.Sprintf("Failed to validate workspace: %v", err)}
	}
	if !workspaceExists {
		return &ItemValidationResult{Valid: false, Error: "Workspace not found"}
	}

	// Task-specific validation
	if params.IsTask {
		// Tasks can only have status_id Open or Done
		if params.StatusID != nil && *params.StatusID != constants.StatusIDOpen && *params.StatusID != constants.StatusIDDone {
			return &ItemValidationResult{Valid: false, Error: "Tasks can only have status 'Open' or 'Done'"}
		}
	}

	// Validate parent item if specified
	if params.ParentID != nil && *params.ParentID != 0 {
		result := validateParentHierarchy(db, params.ParentID, params.ItemTypeID)
		if !result.Valid {
			return result
		}
	}

	// Validate related_work_item_id if provided
	if params.RelatedWorkItemID != nil {
		result := validateRelatedWorkItem(db, params.WorkspaceID, params.UserID, *params.RelatedWorkItemID)
		if !result.Valid {
			return result
		}
	}

	return &ItemValidationResult{Valid: true}
}

// validateParentHierarchy validates the parent-child hierarchy relationship
func validateParentHierarchy(db database.Database, parentID *int, itemTypeID *int) *ItemValidationResult {
	var parentItemTypeID sql.NullInt64
	var parentItemTypeHierarchyLevel int
	err := db.QueryRow(`
		SELECT i.item_type_id, COALESCE(it.hierarchy_level, 0)
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.id = ?
	`, *parentID).Scan(&parentItemTypeID, &parentItemTypeHierarchyLevel)

	if err == sql.ErrNoRows {
		return &ItemValidationResult{Valid: false, Error: "Parent item not found"}
	}
	if err != nil {
		return &ItemValidationResult{Valid: false, Error: fmt.Sprintf("Failed to validate parent: %v", err)}
	}

	// Validate hierarchy relationship if item type is specified
	if itemTypeID != nil && *itemTypeID != 0 {
		var itemTypeHierarchyLevel int
		var itemTypeName string
		err := db.QueryRow(`
			SELECT hierarchy_level, name FROM item_types
			WHERE id = ?
		`, *itemTypeID).Scan(&itemTypeHierarchyLevel, &itemTypeName)

		if err == sql.ErrNoRows {
			return &ItemValidationResult{Valid: false, Error: "Item type not found"}
		}
		if err != nil {
			return &ItemValidationResult{Valid: false, Error: fmt.Sprintf("Failed to validate item type: %v", err)}
		}

		// Check if child hierarchy level is exactly one more than parent
		if itemTypeHierarchyLevel != parentItemTypeHierarchyLevel+1 {
			return &ItemValidationResult{
				Valid: false,
				Error: fmt.Sprintf("Item type '%s' (hierarchy level %d) cannot be a child of an item at hierarchy level %d",
					itemTypeName, itemTypeHierarchyLevel, parentItemTypeHierarchyLevel),
			}
		}
	}

	return &ItemValidationResult{Valid: true}
}

// validateRelatedWorkItem validates that the related work item exists and is in a valid workspace
func validateRelatedWorkItem(db database.Database, workspaceID int, userID int, relatedWorkItemID int) *ItemValidationResult {
	// Verify workspace is personal and belongs to the user
	var isPersonal bool
	var ownerID *int
	err := db.QueryRow(`
		SELECT is_personal, owner_id FROM workspaces WHERE id = ?
	`, workspaceID).Scan(&isPersonal, &ownerID)

	if err != nil {
		return &ItemValidationResult{Valid: false, Error: fmt.Sprintf("Failed to validate workspace: %v", err)}
	}

	if !isPersonal || ownerID == nil || *ownerID != userID {
		return &ItemValidationResult{Valid: false, Error: "Personal tasks must be created in your own personal workspace"}
	}

	// Verify the related work item exists
	var relatedWorkspaceID int
	err = db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", relatedWorkItemID).Scan(&relatedWorkspaceID)
	if err != nil {
		return &ItemValidationResult{Valid: false, Error: "Related work item not found or access denied"}
	}

	return &ItemValidationResult{Valid: true}
}
