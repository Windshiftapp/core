// Package validation provides field validation for work items and other entities.
// It includes validators for custom fields, required fields, and entity relationships.
package validation

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

// WorkspacePermissionChecker lets the validator ask whether a user holds a
// permission in a specific workspace. Declared as an interface in this package
// to avoid importing services (which imports validation).
// *services.PermissionService satisfies it by duck typing.
type WorkspacePermissionChecker interface {
	HasWorkspacePermission(userID, workspaceID int, permission string) (bool, error)
}

// HierarchyCycleChecker lets the validator ask whether assigning a new parent
// would create a hierarchy cycle. Declared as an interface here for the same
// reason as WorkspacePermissionChecker — avoid importing services.
// *services.HierarchyService satisfies it by duck typing.
type HierarchyCycleChecker interface {
	WouldCreateCycle(ancestorCandidateID, newParentID int) (bool, error)
}

// ProjectAccessChecker lets the validator ask whether a user may assign a
// given time project to an item. Declared as an interface here for the same
// reason as the checkers above — avoid importing services.
// *services.TimePermissionService satisfies it by duck typing.
type ProjectAccessChecker interface {
	CanViewProject(userID, projectID int) (bool, error)
}

// WorkspaceAssigneeChecker reports whether a user can act on items in a
// workspace. *services.WorkspaceUserResolver satisfies it.
type WorkspaceAssigneeChecker interface {
	CanActInWorkspace(userID, workspaceID int) (bool, error)
}

// ItemFieldValidator provides validation for item fields during create/update operations
type ItemFieldValidator struct {
	db              database.Database
	permChecker     WorkspacePermissionChecker
	cycleChecker    HierarchyCycleChecker
	projectChecker  ProjectAccessChecker
	assigneeChecker WorkspaceAssigneeChecker
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

// WithPermissionChecker attaches a workspace permission checker for item
// relationships that may cross workspace boundaries. Returns the receiver for
// chaining.
func (v *ItemFieldValidator) WithPermissionChecker(checker WorkspacePermissionChecker) *ItemFieldValidator {
	v.permChecker = checker
	return v
}

// WithCycleChecker attaches a hierarchy cycle checker so the validator can
// reject parent_id changes that would create a cycle. User-facing callers
// must set this; internal callers that don't mutate parent_id may omit it.
// Returns the receiver for chaining.
func (v *ItemFieldValidator) WithCycleChecker(checker HierarchyCycleChecker) *ItemFieldValidator {
	v.cycleChecker = checker
	return v
}

// WithProjectAccessChecker attaches a time-project access checker so the
// validator can enforce that the caller may assign a given project_id /
// time_project_id. User-facing callers must set this; internal callers that
// don't mutate those fields may omit it. Returns the receiver for chaining.
func (v *ItemFieldValidator) WithProjectAccessChecker(checker ProjectAccessChecker) *ItemFieldValidator {
	v.projectChecker = checker
	return v
}

// WithWorkspaceAssigneeChecker attaches the shared workspace-user resolver.
func (v *ItemFieldValidator) WithWorkspaceAssigneeChecker(checker WorkspaceAssigneeChecker) *ItemFieldValidator {
	v.assigneeChecker = checker
	return v
}

// checkProjectAssignable verifies the caller may attach the given time project
// to an item. Existence is checked first (CanViewProject treats an unrestricted
// non-existent project as viewable), then access. Both the not-found and
// no-access cases return the same "<field> not found" error so the response
// can't be used to enumerate which project IDs exist.
func (v *ItemFieldValidator) checkProjectAssignable(field string, userID, projectID int) error {
	exists, err := v.EntityExists("time_projects", projectID)
	if err != nil {
		return fmt.Errorf("failed to validate project: %w", err)
	}
	if !exists {
		return &ValidationError{Field: field, Message: "Project not found"}
	}
	if v.projectChecker != nil {
		hasAccess, accErr := v.projectChecker.CanViewProject(userID, projectID)
		if accErr != nil {
			return fmt.Errorf("failed to check project access: %w", accErr)
		}
		if !hasAccess {
			// Mirror the not-found message to avoid leaking project existence.
			return &ValidationError{Field: field, Message: "Project not found"}
		}
	}
	return nil
}

// ValidationError represents a field validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// applyDateField applies one date field from updateData onto dst. Accepted
// values: nil (clear), a YYYY-MM-DD string (web handlers decode bodies as raw
// maps), or a time.Time (the REST v1 handlers decode typed DTOs). Any other
// type is a validation error — a recognized key must never be silently
// dropped.
func applyDateField(updateData map[string]any, field string, dst **time.Time) error {
	value, ok := updateData[field]
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case nil:
		*dst = nil
	case string:
		parsedDate, err := time.Parse("2006-01-02", v)
		if err != nil {
			return &ValidationError{Field: field, Message: fmt.Sprintf("Invalid %s format, expected YYYY-MM-DD", field)}
		}
		*dst = &parsedDate
	case time.Time:
		t := v
		*dst = &t
	default:
		return &ValidationError{Field: field, Message: fmt.Sprintf("Invalid %s type, expected a YYYY-MM-DD string or null", field)}
	}
	return nil
}

// ValidateAndApplyUpdates applies all update data to an item with validation
// Returns a list of validation errors if any occur
func (v *ItemFieldValidator) ValidateAndApplyUpdates(
	item *models.Item,
	updateData map[string]any,
	userID int, // for permission checks on personal tasks
) error {
	// Titles use the same normalization as the create and update services.
	if title, ok := updateData["title"].(string); ok {
		title, err := NormalizeTitle(title)
		if err != nil {
			return err
		}
		item.Title = title
	}

	// Description validation preserves accepted source.
	if description, ok := updateData["description"].(string); ok {
		if err := ValidateMarkdownSource("description", description, MarkdownMaxBytes, false); err != nil {
			return err
		}
		item.Description = description
	}

	if isTaskValue, ok := updateData["is_task"]; ok {
		if isTaskBool, ok := isTaskValue.(bool); ok {
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

	// Date validation and parsing (due/start/end)
	if err := applyDateField(updateData, "due_date", &item.DueDate); err != nil {
		return err
	}
	if err := applyDateField(updateData, "start_date", &item.StartDate); err != nil {
		return err
	}
	if err := applyDateField(updateData, "end_date", &item.EndDate); err != nil {
		return err
	}

	// Milestone IDs validation (multi-milestone). Accepts []int / []float64 /
	// []any. nil/missing = no change. Empty slice = clear all. Each
	// referenced milestone must exist.
	if msVal, ok := updateData["milestone_ids"]; ok {
		ids, err := coerceIntSlice(msVal)
		if err != nil {
			return &ValidationError{Field: "milestone_ids", Message: "milestone_ids must be an array of integers"}
		}
		for _, mID := range ids {
			exists, err := v.EntityExists("milestones", mID)
			if err != nil {
				return fmt.Errorf("failed to check milestone existence: %w", err)
			}
			if !exists {
				return &ValidationError{Field: "milestone_ids", Message: fmt.Sprintf("Milestone %d not found", mID)}
			}
		}
		// Stash validated IDs on the item so the calling service can persist
		// them into item_milestones. Hydrated as ID-only Milestone stubs; the
		// handler/loader will refill the full rows on read.
		stubs := make([]models.Milestone, 0, len(ids))
		for _, mID := range ids {
			stubs = append(stubs, models.Milestone{ID: mID})
		}
		item.Milestones = stubs
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
			newProjectID, ok := utils.CoerceInt(projectIDValue)
			if !ok {
				return &ValidationError{Field: "project_id", Message: "Invalid project_id type"}
			}
			if newProjectID > 0 {
				// Validate the project exists AND the caller may assign it.
				if err := v.checkProjectAssignable("project_id", userID, newProjectID); err != nil {
					return err
				}
				item.ProjectID = &newProjectID
				// When setting a direct project, clear inherit flag
				item.InheritProject = false
			}
		}
	}

	// Time-project override validation. time_project_id overrides the project
	// used when logging time on the item; it is independent of inherit_project.
	if timeProjectIDValue, ok := updateData["time_project_id"]; ok {
		if timeProjectIDValue == nil {
			item.TimeProjectID = nil
		} else {
			newTimeProjectID, ok := utils.CoerceInt(timeProjectIDValue)
			if !ok {
				return &ValidationError{Field: "time_project_id", Message: "Invalid time_project_id type"}
			}
			if newTimeProjectID > 0 {
				if err := v.checkProjectAssignable("time_project_id", userID, newTimeProjectID); err != nil {
					return err
				}
				item.TimeProjectID = &newTimeProjectID
			} else {
				// 0 means clear, consistent with the aitools convention.
				item.TimeProjectID = nil
			}
		}
	}

	// Workspace ID validation (if being changed)
	if workspaceIDValue, ok := updateData["workspace_id"]; ok && workspaceIDValue != nil {
		newWorkspaceID, ok := utils.CoerceInt(workspaceIDValue)
		if !ok {
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
	_, priorityChanged := updateData["priority_id"]
	_, workspaceChanged := updateData["workspace_id"]
	if (priorityChanged || workspaceChanged) && item.PriorityID != nil {
		allowed, err := IsPriorityAllowedInWorkspace(v.db, item.WorkspaceID, *item.PriorityID)
		if err != nil {
			return fmt.Errorf("failed to validate workspace priority: %w", err)
		}
		if !allowed {
			return &ValidationError{Field: "priority_id", Message: "Priority is not allowed in this workspace"}
		}
	}

	// Reject inactive and unknown users first, then apply the shared workspace
	// access and ready-binding rules without exposing which check failed.
	if err := v.ValidateNullableActiveUserID(updateData, "assignee_id", &item.AssigneeID, "Assignee user"); err != nil {
		return err
	}
	if _, changed := updateData["assignee_id"]; changed && item.AssigneeID != nil && v.assigneeChecker != nil {
		actionable, err := v.assigneeChecker.CanActInWorkspace(*item.AssigneeID, item.WorkspaceID)
		if err != nil {
			return fmt.Errorf("failed to validate assignee workspace access: %w", err)
		}
		if !actionable {
			return &ValidationError{Field: "assignee_id", Message: "Assignee user not found"}
		}
	}

	// Creator ID validation
	if err := v.ValidateNullableUserID(updateData, "creator_id", &item.CreatorID, "Creator user"); err != nil {
		return err
	}

	// Parent ID validation (with hierarchy level checking)
	if parentIDValue, ok := updateData["parent_id"]; ok {
		if parentIDValue == nil {
			if item.ItemTypeID != nil {
				if err := ValidateParentForItemType(v.db, *item.ItemTypeID, nil); err != nil {
					return err
				}
			}
			item.ParentID = nil
		} else {
			newParentID, ok := utils.CoerceInt(parentIDValue)
			if !ok {
				return &ValidationError{Field: "parent_id", Message: "Invalid parent_id type"}
			}

			// Reject self-parent outright. Catches the common typo/malicious
			// case before we bother the DB with a cycle walk, and ensures
			// items without an item_type_id (which skip hierarchy-level
			// validation below) still can't point at themselves.
			if item.ID != 0 && newParentID == item.ID {
				return &ValidationError{Field: "parent_id", Message: "Item cannot be its own parent"}
			}

			// Validate parent item exists and capture its workspace for the
			// cross-workspace view-permission check below.
			parentWorkspaceID, err := repository.NewItemRepository(v.db).GetWorkspaceID(newParentID)
			if errors.Is(err, repository.ErrNotFound) {
				return &ValidationError{Field: "parent_id", Message: "Parent item not found"}
			}
			if err != nil {
				return fmt.Errorf("failed to validate parent: %w", err)
			}

			// Cross-workspace parents are allowed by design, but only if the
			// caller has view permission on the parent's workspace — otherwise
			// they could link their item to an item whose existence they
			// shouldn't know about.
			if parentWorkspaceID != item.WorkspaceID {
				if v.permChecker == nil {
					// Fail closed: no way to verify the caller's permission.
					return &ValidationError{Field: "parent_id", Message: "Cross-workspace parent requires a permission-checked caller"}
				}
				hasView, permErr := v.permChecker.HasWorkspacePermission(userID, parentWorkspaceID, models.PermissionItemView)
				if permErr != nil {
					return fmt.Errorf("failed to check parent workspace permission: %w", permErr)
				}
				if !hasView {
					// Mimic a 404-style "not found" to avoid leaking existence.
					return &ValidationError{Field: "parent_id", Message: "Parent item not found"}
				}
			}

			// Existing items need a wired cycle checker before parent changes can be validated.
			if item.ID != 0 {
				if v.cycleChecker == nil {
					return &ValidationError{Field: "parent_id", Message: "parent_id changes require a cycle-checked caller"}
				}
				wouldCycle, cycleErr := v.cycleChecker.WouldCreateCycle(item.ID, newParentID)
				if cycleErr != nil {
					return fmt.Errorf("failed to check hierarchy cycle: %w", cycleErr)
				}
				if wouldCycle {
					return &ValidationError{Field: "parent_id", Message: "Parent change would create a hierarchy cycle"}
				}
			}

			// Validate hierarchy levels if item has an item type
			if item.ItemTypeID != nil {
				if err := ValidateParentForItemType(v.db, *item.ItemTypeID, &newParentID); err != nil {
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
			newRelatedWorkItemID, ok := utils.CoerceInt(relatedWorkItemIDValue)
			if !ok {
				return &ValidationError{Field: "related_work_item_id", Message: "Invalid related_work_item_id type"}
			}

			if err := v.ValidateRelatedWorkItem(item.WorkspaceID, userID, newRelatedWorkItemID); err != nil {
				return err
			}

			item.RelatedWorkItemID = &newRelatedWorkItemID
		}
	}

	// Story points validation
	if spValue, ok := updateData["story_points"]; ok {
		if spValue == nil {
			item.StoryPoints = nil
		} else {
			switch v := spValue.(type) {
			case float64:
				if v < 0 {
					return &ValidationError{Field: "story_points", Message: "Story points cannot be negative"}
				}
				item.StoryPoints = &v
			case int:
				f := float64(v)
				if f < 0 {
					return &ValidationError{Field: "story_points", Message: "Story points cannot be negative"}
				}
				item.StoryPoints = &f
			default:
				return &ValidationError{Field: "story_points", Message: "Invalid story_points type"}
			}
		}
	}

	// Estimate minutes validation
	if emValue, ok := updateData["estimate_minutes"]; ok {
		if emValue == nil {
			item.EstimateMinutes = nil
		} else {
			switch v := emValue.(type) {
			case float64:
				if v < 0 {
					return &ValidationError{Field: "estimate_minutes", Message: "Estimate cannot be negative"}
				}
				n := int(v)
				item.EstimateMinutes = &n
			case int:
				if v < 0 {
					return &ValidationError{Field: "estimate_minutes", Message: "Estimate cannot be negative"}
				}
				item.EstimateMinutes = &v
			default:
				return &ValidationError{Field: "estimate_minutes", Message: "Invalid estimate_minutes type"}
			}
		}
	}

	// Custom field values validation
	if customFields, ok := updateData["custom_field_values"]; ok {
		if customFields != nil {
			cfv, ok := customFields.(map[string]any)
			if !ok {
				return &ValidationError{Field: "custom_field_values", Message: "must be a JSON object"}
			}
			// Validate option ids (select/multiselect) + dedupe multiselect
			// arrays. Unknown field ids are accepted here; the async cfv
			// cleanup scheduler is responsible for removing them.
			if err := ValidateAndNormalizeCustomFieldValues(v.db, cfv); err != nil {
				return err
			}
			item.CustomFieldValues = cfv
		} else {
			item.CustomFieldValues = make(map[string]any)
		}
	}
	if err := ValidateTaskState(v.db, item.WorkspaceID, userID, item.IsTask, item.StatusID); err != nil {
		return err
	}

	return nil
}

// ValidateNullableIDField validates a nullable foreign key field
// This eliminates the repetitive pattern used for status_id, priority_id, milestone_id, etc.
func (v *ItemFieldValidator) ValidateNullableIDField(
	updateData map[string]any,
	fieldName string,
	destination **int,
	tableName string,
	entityName string,
) error {
	if value, ok := updateData[fieldName]; ok {
		if value == nil {
			*destination = nil
		} else {
			newID, ok := utils.CoerceInt(value)
			if !ok {
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
	updateData map[string]any,
	fieldName string,
	destination **int,
	entityName string,
) error {
	return v.validateNullableUserID(updateData, fieldName, destination, entityName, false)
}

// ValidateNullableActiveUserID validates a nullable user field against active
// users only. Inactive users are not available to user-facing assignment
// flows and must be indistinguishable from unknown users.
func (v *ItemFieldValidator) ValidateNullableActiveUserID(
	updateData map[string]any,
	fieldName string,
	destination **int,
	entityName string,
) error {
	return v.validateNullableUserID(updateData, fieldName, destination, entityName, true)
}

func (v *ItemFieldValidator) validateNullableUserID(
	updateData map[string]any,
	fieldName string,
	destination **int,
	entityName string,
	activeOnly bool,
) error {
	if value, ok := updateData[fieldName]; ok {
		if value == nil {
			*destination = nil
		} else {
			newID, ok := utils.CoerceInt(value)
			if !ok {
				return &ValidationError{Field: fieldName, Message: fmt.Sprintf("Invalid %s type", entityName)}
			}
			var exists bool
			var err error
			if activeOnly {
				exists, err = repository.NewUserRepository(v.db).ActiveExists(newID)
			} else {
				exists, err = v.EntityExists("users", newID)
			}
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

// coerceIntSlice delegates to the shared integer-array coercion so every
// update field uses identical number handling. The caller wraps failures in
// a field-specific ValidationError, so the underlying message is diagnostic
// only.
func coerceIntSlice(v any) ([]int, error) {
	ids, ok := utils.CoerceIntSlice(v)
	if !ok {
		return nil, fmt.Errorf("unexpected value %T", v)
	}
	return ids, nil
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
	return ValidateParentForItemType(v.db, itemTypeID, &parentID)
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

// ValidateRelatedWorkItem verifies that a personal-workspace item may refer to
// the requested work item without crossing an authorization boundary.
func (v *ItemFieldValidator) ValidateRelatedWorkItem(workspaceID, userID, relatedWorkItemID int) error {
	if err := v.ValidatePersonalWorkspace(workspaceID, userID); err != nil {
		return err
	}

	relatedWorkspaceID, err := repository.NewItemRepository(v.db).GetWorkspaceID(relatedWorkItemID)
	if errors.Is(err, repository.ErrNotFound) {
		return relatedWorkItemNotFoundError()
	}
	if err != nil {
		return fmt.Errorf("failed to validate related work item: %w", err)
	}
	if v.permChecker == nil {
		return relatedWorkItemNotFoundError()
	}

	hasView, err := v.permChecker.HasWorkspacePermission(userID, relatedWorkspaceID, models.PermissionItemView)
	if err != nil {
		return fmt.Errorf("failed to check related work item permission: %w", err)
	}
	if !hasView {
		return relatedWorkItemNotFoundError()
	}

	return nil
}

func relatedWorkItemNotFoundError() *ValidationError {
	return &ValidationError{
		Field:   "related_work_item_id",
		Message: "Related work item not found or access denied",
	}
}

// ConvertCustomFieldValuesToJSON converts custom field values map to JSON for database storage
func ConvertCustomFieldValuesToJSON(customFieldValues map[string]any) (sql.NullString, error) {
	if len(customFieldValues) == 0 {
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
// deadcode-keep: called by core-tests/internal/validation/item_field_validator_test.go
func (v *ItemFieldValidator) ValidateCreateRequest(item *models.Item, userID int) error {
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
	if err := ValidateTaskState(v.db, item.WorkspaceID, userID, item.IsTask, item.StatusID); err != nil {
		return err
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
			if err := ValidateParentForItemType(v.db, *item.ItemTypeID, item.ParentID); err != nil {
				return err
			}
		}
	} else if item.ItemTypeID != nil {
		if err := ValidateParentForItemType(v.db, *item.ItemTypeID, nil); err != nil {
			return err
		}
	}

	return nil
}
