package services

import (
	"errors"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/validation"
)

// ItemValidationParams contains parameters for validating item creation
type ItemValidationParams struct {
	WorkspaceID       int
	Title             string
	ItemTypeID        *int
	ParentID          *int
	RelatedWorkItemID *int
	UserID            int // User creating the item (for personal workspace validation)
	// PermService, when set, enforces that the caller has view permission on a
	// cross-workspace parent's workspace before allowing the link — otherwise a
	// user could discover (and attach to) items in workspaces they can't see.
	// User-facing create handlers set it; internal callers may omit it.
	PermService *PermissionService
}

// ItemValidationResult contains the result of validation
type ItemValidationResult struct {
	Valid bool
	Error string
}

// IsItemTypeAllowedInWorkspace checks whether the given item type is allowed
// in the workspace's configuration set. Returns true when:
//   - the workspace has no configuration set (all types allowed), or
//   - the item type appears in configuration_set_item_types for that config set.
func IsItemTypeAllowedInWorkspace(db database.Database, workspaceID, itemTypeID int) (bool, error) {
	exists, err := repository.NewConfigurationSetRepository(db).ItemTypeAllowed(workspaceID, itemTypeID)
	if err != nil {
		return false, fmt.Errorf("failed to check item type in config set: %w", err)
	}
	return exists, nil
}

// IsPriorityAllowedInWorkspace verifies that the priority exists and is
// available through the workspace's configuration set. Workspaces without a
// configuration set, or whose configuration set has no explicit priority
// assignments, use the default global priority catalog.
func IsPriorityAllowedInWorkspace(db database.Database, workspaceID, priorityID int) (bool, error) {
	return validation.IsPriorityAllowedInWorkspace(db, workspaceID, priorityID)
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

	// Validate parent item if specified
	if params.ParentID != nil && *params.ParentID != 0 {
		result := validateParentHierarchy(db, params.ParentID, params.ItemTypeID, params.WorkspaceID, params.UserID, params.PermService)
		if !result.Valid {
			return result
		}
	} else if params.ItemTypeID != nil && *params.ItemTypeID != 0 {
		if err := validation.ValidateParentForItemType(db, *params.ItemTypeID, nil); err != nil {
			return &ItemValidationResult{Valid: false, Error: err.Error()}
		}
	}

	// Validate related_work_item_id if provided
	if params.RelatedWorkItemID != nil {
		result := validateRelatedWorkItem(db, params.WorkspaceID, params.UserID, *params.RelatedWorkItemID, params.PermService)
		if !result.Valid {
			return result
		}
	}

	return &ItemValidationResult{Valid: true}
}

// validateParentHierarchy validates the parent-child hierarchy relationship.
// When permService is non-nil and the parent lives in a different workspace
// than the new item, the caller must hold view permission on the parent's
// workspace; otherwise the parent is reported as "not found" so its existence
// (and hierarchy level) isn't leaked across a workspace boundary.
func validateParentHierarchy(db database.Database, parentID, itemTypeID *int, workspaceID, userID int, permService *PermissionService) *ItemValidationResult {
	repo := repository.NewItemRepository(db)

	// Cross-workspace parent: gate on view permission before revealing anything
	// about the parent (mirrors the update-path check in the field validator).
	if permService != nil {
		parentWorkspaceID, wsErr := repo.GetWorkspaceID(*parentID)
		if errors.Is(wsErr, repository.ErrNotFound) {
			return &ItemValidationResult{Valid: false, Error: "Parent item not found"}
		}
		if wsErr != nil {
			return &ItemValidationResult{Valid: false, Error: fmt.Sprintf("Failed to validate parent: %v", wsErr)}
		}
		if parentWorkspaceID != workspaceID {
			hasView, permErr := permService.HasWorkspacePermission(userID, parentWorkspaceID, models.PermissionItemView)
			if permErr != nil {
				return &ItemValidationResult{Valid: false, Error: fmt.Sprintf("Failed to validate parent: %v", permErr)}
			}
			if !hasView {
				return &ItemValidationResult{Valid: false, Error: "Parent item not found"}
			}
		}
	}

	// Validate hierarchy relationship if item type is specified
	if itemTypeID != nil && *itemTypeID != 0 {
		if err := validation.ValidateParentForItemType(db, *itemTypeID, parentID); err != nil {
			return &ItemValidationResult{Valid: false, Error: err.Error()}
		}
	}

	return &ItemValidationResult{Valid: true}
}

// validateRelatedWorkItem applies the shared relationship validation used by
// item updates so create and update paths enforce the same permission rule.
func validateRelatedWorkItem(
	db database.Database,
	workspaceID, userID, relatedWorkItemID int,
	permService *PermissionService,
) *ItemValidationResult {
	validator := validation.NewItemFieldValidator(db)
	if permService != nil {
		validator.WithPermissionChecker(permService)
	}
	if err := validator.ValidateRelatedWorkItem(workspaceID, userID, relatedWorkItemID); err != nil {
		return &ItemValidationResult{Valid: false, Error: err.Error()}
	}
	return &ItemValidationResult{Valid: true}
}
