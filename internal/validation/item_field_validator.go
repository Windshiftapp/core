package validation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// ItemFieldValidator provides validation for item fields during create/update operations
type ItemFieldValidator struct {
	db database.Database
}

// allowedEntityTables is a whitelist of valid table names for EntityExists checks
// This prevents SQL injection via dynamic table names
var allowedEntityTables = map[string]bool{
	"items":         true,
	"users":         true,
	"workspaces":    true,
	"milestones":    true,
	"iterations":    true,
	"time_projects": true,
	"item_types":    true,
	"statuses":      true,
	"priorities":    true,
}

// NewItemFieldValidator creates a new item field validator
func NewItemFieldValidator(db database.Database) *ItemFieldValidator {
	return &ItemFieldValidator{db: db}
}

// ValidationError represents a field validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateAndApplyUpdates applies all update data to an item with validation
// Returns a list of validation errors if any occur
func (v *ItemFieldValidator) ValidateAndApplyUpdates(
	item *models.Item,
	updateData map[string]interface{},
	userID int, // for permission checks on personal tasks
) error {
	// Title validation and sanitization
	if title, ok := updateData["title"].(string); ok {
		sanitizedTitle := utils.SanitizeTitle(title)
		if strings.TrimSpace(sanitizedTitle) == "" {
			return &ValidationError{Field: "title", Message: "Title is required"}
		}
		item.Title = sanitizedTitle
	}

	// Description validation and sanitization
	if description, ok := updateData["description"].(string); ok {
		item.Description = utils.SanitizeDescription(description)
	}

	// is_task validation - can only be true for personal workspaces
	if isTaskValue, ok := updateData["is_task"]; ok {
		if isTaskBool, ok := isTaskValue.(bool); ok {
			if err := v.ValidateIsTask(item.WorkspaceID, isTaskBool); err != nil {
				return err
			}
			item.IsTask = isTaskBool
		}
	}

	// Status ID validation
	if err := v.ValidateNullableIDField(updateData, "status_id", &item.StatusID, "statuses", "Status"); err != nil {
		return err
	}

	// Priority ID validation
	if err := v.ValidateNullableIDField(updateData, "priority_id", &item.PriorityID, "priorities", "Priority"); err != nil {
		return err
	}

	// Due date validation and parsing
	if dueDateValue, ok := updateData["due_date"]; ok {
		if dueDateValue == nil {
			item.DueDate = nil
		} else if dueDateStr, ok := dueDateValue.(string); ok {
			parsedDate, err := time.Parse("2006-01-02", dueDateStr)
			if err != nil {
				return &ValidationError{Field: "due_date", Message: "Invalid due_date format, expected YYYY-MM-DD"}
			}
			item.DueDate = &parsedDate
		}
	}

	// Milestone ID validation
	if err := v.ValidateNullableIDField(updateData, "milestone_id", &item.MilestoneID, "milestones", "Milestone"); err != nil {
		return err
	}

	// Iteration ID validation
	if err := v.ValidateNullableIDField(updateData, "iteration_id", &item.IterationID, "iterations", "Iteration"); err != nil {
		return err
	}

	// Project inheritance logic
	if inheritProjectValue, ok := updateData["inherit_project"]; ok {
		if inheritProjectBool, ok := inheritProjectValue.(bool); ok {
			item.InheritProject = inheritProjectBool
			// If setting to inherit, clear project_id
			if inheritProjectBool {
				item.ProjectID = nil
			}
		}
	}

	// Project ID validation with inheritance logic
	if projectIDValue, ok := updateData["project_id"]; ok {
		if projectIDValue == nil {
			item.ProjectID = nil
			// When clearing project_id, only clear inherit flag if inherit_project wasn't explicitly set to true
			if inheritProjectValue, hasInheritProject := updateData["inherit_project"]; !hasInheritProject || inheritProjectValue != true {
				item.InheritProject = false
			}
		} else {
			var newProjectID int
			switch v := projectIDValue.(type) {
			case float64:
				newProjectID = int(v)
			case int:
				newProjectID = v
			default:
				return &ValidationError{Field: "project_id", Message: "Invalid project_id type"}
			}
			if newProjectID > 0 {
				// Validate project exists
				exists, err := v.EntityExists("time_projects", newProjectID)
				if err != nil {
					return fmt.Errorf("failed to validate project: %w", err)
				}
				if !exists {
					return &ValidationError{Field: "project_id", Message: "Project not found"}
				}
				item.ProjectID = &newProjectID
				// When setting a direct project, clear inherit flag
				item.InheritProject = false
			}
		}
	}

	// Workspace ID validation (if being changed)
	if workspaceIDValue, ok := updateData["workspace_id"]; ok && workspaceIDValue != nil {
		var newWorkspaceID int
		switch v := workspaceIDValue.(type) {
		case float64:
			newWorkspaceID = int(v)
		case int:
			newWorkspaceID = v
		default:
			return &ValidationError{Field: "workspace_id", Message: "Invalid workspace_id type"}
		}
		exists, err := v.EntityExists("workspaces", newWorkspaceID)
		if err != nil {
			return fmt.Errorf("failed to validate workspace: %w", err)
		}
		if !exists {
			return &ValidationError{Field: "workspace_id", Message: "Workspace not found"}
		}
		item.WorkspaceID = newWorkspaceID
	}

	// Assignee ID validation
	if err := v.ValidateNullableUserID(updateData, "assignee_id", &item.AssigneeID, "Assignee user"); err != nil {
		return err
	}

	// Creator ID validation
	if err := v.ValidateNullableUserID(updateData, "creator_id", &item.CreatorID, "Creator user"); err != nil {
		return err
	}

	// Parent ID validation (with hierarchy level checking)
	if parentIDValue, ok := updateData["parent_id"]; ok {
		if parentIDValue == nil {
			item.ParentID = nil
		} else {
			var newParentID int
			switch v := parentIDValue.(type) {
			case float64:
				newParentID = int(v)
			case int:
				newParentID = v
			default:
				return &ValidationError{Field: "parent_id", Message: "Invalid parent_id type"}
			}

			// Validate parent item exists
			exists, err := v.EntityExists("items", newParentID)
			if err != nil {
				return fmt.Errorf("failed to validate parent: %w", err)
			}
			if !exists {
				return &ValidationError{Field: "parent_id", Message: "Parent item not found"}
			}

			// Validate hierarchy levels if item has an item type
			if item.ItemTypeID != nil {
				if err := v.ValidateHierarchyLevels(item.ID, *item.ItemTypeID, newParentID); err != nil {
					return err
				}
			}

			item.ParentID = &newParentID
		}
	}

	// Related work item ID validation (for personal tasks)
	if relatedWorkItemIDValue, ok := updateData["related_work_item_id"]; ok {
		if relatedWorkItemIDValue == nil {
			item.RelatedWorkItemID = nil
		} else {
			var newRelatedWorkItemID int
			switch v := relatedWorkItemIDValue.(type) {
			case float64:
				newRelatedWorkItemID = int(v)
			case int:
				newRelatedWorkItemID = v
			default:
				return &ValidationError{Field: "related_work_item_id", Message: "Invalid related_work_item_id type"}
			}

			// Validate workspace is personal and belongs to the user
			if err := v.ValidatePersonalWorkspace(item.WorkspaceID, userID); err != nil {
				return err
			}

			// Verify the related work item exists
			var relatedWorkspaceID int
			err := v.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", newRelatedWorkItemID).Scan(&relatedWorkspaceID)
			if err != nil {
				return &ValidationError{Field: "related_work_item_id", Message: "Related work item not found or access denied"}
			}

			item.RelatedWorkItemID = &newRelatedWorkItemID
		}
	}

	// Custom field values validation
	if customFields, ok := updateData["custom_field_values"]; ok {
		if customFields != nil {
			item.CustomFieldValues = customFields.(map[string]interface{})
		} else {
			item.CustomFieldValues = make(map[string]interface{})
		}
	}

	return nil
}

// ValidateNullableIDField validates a nullable foreign key field
// This eliminates the repetitive pattern used for status_id, priority_id, milestone_id, etc.
func (v *ItemFieldValidator) ValidateNullableIDField(
	updateData map[string]interface{},
	fieldName string,
	destination **int,
	tableName string,
	entityName string,
) error {
	if value, ok := updateData[fieldName]; ok {
		if value == nil {
			*destination = nil
		} else {
			var newID int
			switch val := value.(type) {
			case float64:
				newID = int(val)
			case int:
				newID = val
			default:
				return &ValidationError{Field: fieldName, Message: fmt.Sprintf("Invalid %s type", entityName)}
			}
			// Validate entity exists
			exists, err := v.EntityExists(tableName, newID)
			if err != nil {
				return fmt.Errorf("failed to validate %s: %w", entityName, err)
			}
			if !exists {
				return &ValidationError{Field: fieldName, Message: fmt.Sprintf("%s not found", entityName)}
			}
			*destination = &newID
		}
	}
	return nil
}

// ValidateNullableUserID validates a user ID field (assignee_id, creator_id, etc.)
func (v *ItemFieldValidator) ValidateNullableUserID(
	updateData map[string]interface{},
	fieldName string,
	destination **int,
	entityName string,
) error {
	if value, ok := updateData[fieldName]; ok {
		if value == nil {
			*destination = nil
		} else {
			var newID int
			switch val := value.(type) {
			case float64:
				newID = int(val)
			case int:
				newID = val
			default:
				return &ValidationError{Field: fieldName, Message: fmt.Sprintf("Invalid %s type", entityName)}
			}
			// Validate user exists
			exists, err := v.EntityExists("users", newID)
			if err != nil {
				return fmt.Errorf("failed to validate user: %w", err)
			}
			if !exists {
				return &ValidationError{Field: fieldName, Message: fmt.Sprintf("%s not found", entityName)}
			}
			*destination = &newID
		}
	}
	return nil
}

// EntityExists checks if an entity with the given ID exists in the specified table
func (v *ItemFieldValidator) EntityExists(tableName string, id int) (bool, error) {
	if !allowedEntityTables[tableName] {
		return false, fmt.Errorf("invalid table name: %s", tableName)
	}
	var exists bool
	query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s WHERE id = ?)", tableName)
	err := v.db.QueryRow(query, id).Scan(&exists)
	return exists, err
}

// ValidateHierarchyLevels validates that parent-child hierarchy levels are correct
// Child hierarchy level must be exactly one more than parent hierarchy level
func (v *ItemFieldValidator) ValidateHierarchyLevels(itemID, itemTypeID, parentID int) error {
	// Get the item's current item type hierarchy level
	var itemTypeHierarchyLevel int
	var itemTypeName string
	err := v.db.QueryRow(`
		SELECT it.hierarchy_level, it.name
		FROM item_types it
		WHERE it.id = ?
	`, itemTypeID).Scan(&itemTypeHierarchyLevel, &itemTypeName)
	if err != nil {
		return fmt.Errorf("failed to get item type hierarchy level: %w", err)
	}

	// Get the parent's item type hierarchy level
	var parentItemTypeHierarchyLevel int
	err = v.db.QueryRow(`
		SELECT it.hierarchy_level
		FROM item_types it
		JOIN items i ON i.item_type_id = it.id
		WHERE i.id = ?
	`, parentID).Scan(&parentItemTypeHierarchyLevel)
	if err != nil {
		return fmt.Errorf("failed to get parent item type hierarchy level: %w", err)
	}

	// Check if child hierarchy level is exactly one more than parent
	if itemTypeHierarchyLevel != parentItemTypeHierarchyLevel+1 {
		return &ValidationError{
			Field: "parent_id",
			Message: fmt.Sprintf(
				"Item type '%s' (hierarchy level %d) cannot be a child of an item at hierarchy level %d",
				itemTypeName, itemTypeHierarchyLevel, parentItemTypeHierarchyLevel,
			),
		}
	}

	return nil
}

// IsPersonalWorkspace checks if a workspace is a personal workspace
func (v *ItemFieldValidator) IsPersonalWorkspace(workspaceID int) (bool, error) {
	var isPersonal bool
	err := v.db.QueryRow(`
		SELECT is_personal FROM workspaces WHERE id = ?
	`, workspaceID).Scan(&isPersonal)
	if err != nil {
		return false, fmt.Errorf("failed to check workspace: %w", err)
	}
	return isPersonal, nil
}

// ValidatePersonalWorkspace validates that a workspace is personal and belongs to the user
func (v *ItemFieldValidator) ValidatePersonalWorkspace(workspaceID, userID int) error {
	isPersonal, err := v.IsPersonalWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if !isPersonal {
		return &ValidationError{
			Field:   "related_work_item_id",
			Message: "Personal tasks must be created in your own personal workspace",
		}
	}

	// Also check ownership
	var ownerID *int
	err = v.db.QueryRow(`SELECT owner_id FROM workspaces WHERE id = ?`, workspaceID).Scan(&ownerID)
	if err != nil {
		return fmt.Errorf("failed to validate workspace owner: %w", err)
	}

	if ownerID == nil || *ownerID != userID {
		return &ValidationError{
			Field:   "related_work_item_id",
			Message: "Personal tasks must be created in your own personal workspace",
		}
	}

	return nil
}

// ValidateIsTask validates that is_task can only be true for personal workspaces
func (v *ItemFieldValidator) ValidateIsTask(workspaceID int, isTask bool) error {
	if !isTask {
		return nil // is_task: false is always allowed
	}

	isPersonal, err := v.IsPersonalWorkspace(workspaceID)
	if err != nil {
		return err
	}

	if !isPersonal {
		return &ValidationError{
			Field:   "is_task",
			Message: "Tasks can only be created in personal workspaces",
		}
	}

	return nil
}

// ConvertCustomFieldValuesToJSON converts custom field values map to JSON for database storage
func ConvertCustomFieldValuesToJSON(customFieldValues map[string]interface{}) (sql.NullString, error) {
	if customFieldValues == nil || len(customFieldValues) == 0 {
		return sql.NullString{Valid: false}, nil
	}

	customFieldValuesBytes, err := json.Marshal(customFieldValues)
	if err != nil {
		return sql.NullString{}, &ValidationError{
			Field:   "custom_field_values",
			Message: "Invalid custom field values",
		}
	}

	return sql.NullString{String: string(customFieldValuesBytes), Valid: true}, nil
}

// ValidateCreateRequest validates required fields for item creation
func (v *ItemFieldValidator) ValidateCreateRequest(item *models.Item) error {
	// Title is required
	if strings.TrimSpace(item.Title) == "" {
		return &ValidationError{Field: "title", Message: "Title is required"}
	}

	// Workspace must exist
	exists, err := v.EntityExists("workspaces", item.WorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to validate workspace: %w", err)
	}
	if !exists {
		return &ValidationError{Field: "workspace_id", Message: "Workspace not found"}
	}

	// Validate is_task can only be true for personal workspaces
	if item.IsTask {
		if err := v.ValidateIsTask(item.WorkspaceID, item.IsTask); err != nil {
			return err
		}
	}

	// Validate item type if provided
	if item.ItemTypeID != nil {
		exists, err := v.EntityExists("item_types", *item.ItemTypeID)
		if err != nil {
			return fmt.Errorf("failed to validate item type: %w", err)
		}
		if !exists {
			return &ValidationError{Field: "item_type_id", Message: "Item type not found"}
		}
	}

	// Validate parent if provided
	if item.ParentID != nil {
		exists, err := v.EntityExists("items", *item.ParentID)
		if err != nil {
			return fmt.Errorf("failed to validate parent: %w", err)
		}
		if !exists {
			return &ValidationError{Field: "parent_id", Message: "Parent item not found"}
		}

		// Validate hierarchy levels
		if item.ItemTypeID != nil {
			if err := v.ValidateHierarchyLevels(0, *item.ItemTypeID, *item.ParentID); err != nil {
				return err
			}
		}
	}

	return nil
}
