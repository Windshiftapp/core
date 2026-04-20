package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ItemRepository provides data access methods for items
type ItemRepository struct {
	db database.Database
}

// NewItemRepository creates a new item repository
func NewItemRepository(db database.Database) *ItemRepository {
	return &ItemRepository{db: db}
}

// FindByID loads an item by ID with all fields (no joins)
func (r *ItemRepository) FindByID(id int) (*models.Item, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, milestoneID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID, relatedWorkItemID sql.NullInt64
	var dueDate, startDate, endDate sql.NullTime
	var storyPoints sql.NullFloat64

	err := r.db.QueryRow(`
		SELECT id, workspace_id, workspace_item_number, item_type_id, title, description, status_id,
		       priority_id, due_date, start_date, end_date, is_task, milestone_id, iteration_id, project_id, inherit_project,
		       assignee_id, creator_id, custom_field_values, parent_id, related_work_item_id,
		       story_points, frac_index, created_at, updated_at
		FROM items WHERE id = ?
	`, id).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &milestoneID, &iterationID,
		&projectID, &item.InheritProject, &assigneeID, &creatorID, &customFieldValuesJSON, &parentID,
		&relatedWorkItemID, &storyPoints, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find item: %w", err)
	}

	// Handle nullable fields
	assignNullableInt(&item.ItemTypeID, itemTypeID)
	assignNullableInt(&item.ParentID, parentID)
	assignNullableInt(&item.StatusID, statusID)
	assignNullableInt(&item.PriorityID, priorityID)
	assignNullableInt(&item.MilestoneID, milestoneID)
	assignNullableInt(&item.IterationID, iterationID)
	assignNullableInt(&item.ProjectID, projectID)
	assignNullableInt(&item.AssigneeID, assigneeID)
	assignNullableInt(&item.CreatorID, creatorID)
	assignNullableInt(&item.RelatedWorkItemID, relatedWorkItemID)

	if dueDate.Valid {
		item.DueDate = &dueDate.Time
	}
	if startDate.Valid {
		item.StartDate = &startDate.Time
	}
	if endDate.Valid {
		item.EndDate = &endDate.Time
	}
	if storyPoints.Valid {
		item.StoryPoints = &storyPoints.Float64
	}

	// Parse custom field values
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
			item.CustomFieldValues = make(map[string]interface{})
		}
	} else {
		item.CustomFieldValues = make(map[string]interface{})
	}

	return &item, nil
}

// ItemWithWorkspaceStatus includes workspace active status for permission checks
type ItemWithWorkspaceStatus struct {
	*models.Item
	WorkspaceActive bool
}

// FindByIDWithDetails loads an item with all joined data
// This is the consolidated method for the ~30 duplicate JOIN queries throughout items.go
func (r *ItemRepository) FindByIDWithDetails(id int) (*models.Item, error) {
	result, err := r.FindByIDWithWorkspaceStatus(id)
	if err != nil {
		return nil, err
	}
	return result.Item, nil
}

// FindByIDWithWorkspaceStatus loads an item with all joined data including workspace active status
func (r *ItemRepository) FindByIDWithWorkspaceStatus(id int) (*ItemWithWorkspaceStatus, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, milestoneID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID, timeProjectID sql.NullInt64
	var dueDate, startDate, endDate sql.NullTime
	var workspaceActive bool

	// Joined data
	var milestoneName, projectName, iterationName, timeProjectName, parentTitle sql.NullString
	var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail sql.NullString
	var priorityName, priorityIcon, priorityColor sql.NullString
	var statusName sql.NullString
	var itemTypeName sql.NullString
	// Related work item data (for personal tasks)
	var relatedWorkItemID sql.NullInt64
	var relatedWorkItemTitle, relatedWorkItemWorkspaceKey sql.NullString
	var relatedWorkItemWorkspaceID, relatedWorkItemNumber sql.NullInt64
	// Portal-specific fields
	var creatorPortalCustomerID, channelID, requestTypeID sql.NullInt64

	var storyPoints sql.NullFloat64

	err := r.db.QueryRow(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
		       i.status_id, i.priority_id, i.due_date, i.start_date, i.end_date, i.is_task, i.milestone_id, i.iteration_id,
		       i.project_id, i.inherit_project, i.time_project_id, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.story_points, i.frac_index, i.created_at, i.updated_at,
		       i.creator_portal_customer_id, i.channel_id, i.request_type_id,
		       w.name as workspace_name, w.key as workspace_key, w.active as workspace_active,
		       m.name as milestone_name,
		       iter.name as iteration_name,
		       proj.name as project_name,
		       tp.name as time_project_name,
		       p.title as parent_title,
		       assignee.first_name || ' ' || assignee.last_name as assignee_name, assignee.email as assignee_email, assignee.avatar_url as assignee_avatar,
		       creator.first_name || ' ' || creator.last_name as creator_name, creator.email as creator_email,
		       pri.name as priority_name, pri.icon as priority_icon, pri.color as priority_color,
		       s.name as status_name,
		       it.name as item_type_name,
		       i.related_work_item_id,
		       rw.title as related_work_item_title,
		       rw_ws.key as related_work_item_workspace_key,
		       rw.workspace_id as related_work_item_workspace_id,
		       rw.workspace_item_number as related_work_item_number
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN items p ON i.parent_id = p.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN items rw ON i.related_work_item_id = rw.id
		LEFT JOIN workspaces rw_ws ON rw.workspace_id = rw_ws.id
		WHERE i.id = ?
	`, id).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &milestoneID, &iterationID,
		&projectID, &item.InheritProject, &timeProjectID, &assigneeID, &creatorID, &customFieldValuesJSON,
		&parentID, &storyPoints, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
		&creatorPortalCustomerID, &channelID, &requestTypeID,
		&item.WorkspaceName, &item.WorkspaceKey, &workspaceActive,
		&milestoneName, &iterationName, &projectName, &timeProjectName, &parentTitle,
		&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail,
		&priorityName, &priorityIcon, &priorityColor,
		&statusName,
		&itemTypeName,
		&relatedWorkItemID,
		&relatedWorkItemTitle,
		&relatedWorkItemWorkspaceKey,
		&relatedWorkItemWorkspaceID,
		&relatedWorkItemNumber,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find item with details: %w", err)
	}

	// Handle nullable ID fields
	assignNullableInt(&item.ItemTypeID, itemTypeID)
	assignNullableInt(&item.ParentID, parentID)
	assignNullableInt(&item.StatusID, statusID)
	assignNullableInt(&item.PriorityID, priorityID)
	assignNullableInt(&item.MilestoneID, milestoneID)
	assignNullableInt(&item.IterationID, iterationID)
	assignNullableInt(&item.ProjectID, projectID)
	assignNullableInt(&item.TimeProjectID, timeProjectID)
	assignNullableInt(&item.AssigneeID, assigneeID)
	assignNullableInt(&item.CreatorID, creatorID)

	// Portal-specific fields
	assignNullableInt(&item.CreatorPortalCustomerID, creatorPortalCustomerID)
	assignNullableInt(&item.ChannelID, channelID)
	assignNullableInt(&item.RequestTypeID, requestTypeID)

	if dueDate.Valid {
		item.DueDate = &dueDate.Time
	}
	if startDate.Valid {
		item.StartDate = &startDate.Time
	}
	if endDate.Valid {
		item.EndDate = &endDate.Time
	}
	if storyPoints.Valid {
		item.StoryPoints = &storyPoints.Float64
	}

	// Handle nullable string fields from joins
	assignNullableString(&item.MilestoneName, milestoneName)
	assignNullableString(&item.IterationName, iterationName)
	assignNullableString(&item.ProjectName, projectName)
	assignNullableString(&item.TimeProjectName, timeProjectName)
	assignNullableString(&item.ParentTitle, parentTitle)
	assignNullableString(&item.AssigneeName, assigneeName)
	assignNullableString(&item.AssigneeEmail, assigneeEmail)
	assignNullableString(&item.AssigneeAvatar, assigneeAvatar)
	assignNullableString(&item.CreatorName, creatorName)
	assignNullableString(&item.CreatorEmail, creatorEmail)
	assignNullableString(&item.PriorityName, priorityName)
	assignNullableString(&item.PriorityIcon, priorityIcon)
	assignNullableString(&item.PriorityColor, priorityColor)
	assignNullableString(&item.StatusName, statusName)
	assignNullableString(&item.ItemTypeName, itemTypeName)

	// Handle related work item fields (for personal tasks)
	assignNullableInt(&item.RelatedWorkItemID, relatedWorkItemID)
	assignNullableString(&item.RelatedWorkItemTitle, relatedWorkItemTitle)
	assignNullableString(&item.RelatedWorkItemWorkspaceKey, relatedWorkItemWorkspaceKey)
	if relatedWorkItemWorkspaceID.Valid {
		item.RelatedWorkItemWorkspaceID = int(relatedWorkItemWorkspaceID.Int64)
	}
	if relatedWorkItemNumber.Valid {
		item.RelatedWorkItemNumber = int(relatedWorkItemNumber.Int64)
	}

	// Parse custom field values
	if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
			item.CustomFieldValues = make(map[string]interface{})
		}
	} else {
		item.CustomFieldValues = make(map[string]interface{})
	}

	return &ItemWithWorkspaceStatus{Item: &item, WorkspaceActive: workspaceActive}, nil
}

// GetWorkspaceID returns just the workspace_id for an item (frequently needed for permission checks)
func (r *ItemRepository) GetWorkspaceID(itemID int) (int, error) {
	var workspaceID int
	err := r.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace id: %w", err)
	}
	return workspaceID, nil
}

// GetWorkspaceIDCtx is the context-aware variant of GetWorkspaceID. Used by
// handlers that already hold a request-scoped context with timeouts.
func (r *ItemRepository) GetWorkspaceIDCtx(ctx context.Context, itemID int) (int, error) {
	var workspaceID int
	err := r.db.QueryRowContext(ctx, "SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get workspace id: %w", err)
	}
	return workspaceID, nil
}

// ListChildTitles returns the titles of items whose parent_id equals the given
// id. Used for generating prompts that reference existing children.
func (r *ItemRepository) ListChildTitles(parentID int) ([]string, error) {
	rows, err := r.db.Query(`SELECT title FROM items WHERE parent_id = ?`, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list child titles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	titles := []string{}
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, fmt.Errorf("failed to scan child title: %w", err)
		}
		titles = append(titles, title)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate child titles: %w", err)
	}
	return titles, nil
}

// GetTitle returns an item's title (used by link/attachment handlers that need
// a label for notifications without fetching the whole row).
func (r *ItemRepository) GetTitle(itemID int) (string, error) {
	var title string
	err := r.db.QueryRow("SELECT title FROM items WHERE id = ?", itemID).Scan(&title)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to get title: %w", err)
	}
	return title, nil
}

// GetTitles returns a map of itemID → title for all IDs that exist. Missing
// IDs are simply omitted from the result; there is no ErrNotFound signal for
// a missing ID because callers are typically hydrating a mixed set of entity
// IDs and expect lookups to fail silently.
func (r *ItemRepository) GetTitles(itemIDs []int) (map[int]string, error) {
	if len(itemIDs) == 0 {
		return map[int]string{}, nil
	}
	placeholders := make([]string, len(itemIDs))
	args := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := r.db.Query(
		`SELECT id, title FROM items WHERE id IN (`+strings.Join(placeholders, ",")+`)`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get titles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[int]string, len(itemIDs))
	for rows.Next() {
		var id int
		var title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, fmt.Errorf("scan title: %w", err)
		}
		out[id] = title
	}
	return out, rows.Err()
}

// ItemRefByCustomField is one row from ListItemsReferencingAssetInCustomField
// — a single item that references the given asset via its custom_field_values
// JSON blob. Used by the asset-link custom-field reference walker.
type ItemRefByCustomField struct {
	ID          int
	Title       string
	WorkspaceID int
}

// ListItemsReferencingAssetInCustomField scans items whose
// custom_field_values JSON contains the given asset ID, either as a plain int
// value or as an object with an `id` key, for the given custom-field key.
// fieldKey and assetIDStr must already be stringified by the caller.
func (r *ItemRepository) ListItemsReferencingAssetInCustomField(fieldKey, assetIDStr string) ([]ItemRefByCustomField, error) {
	query := fmt.Sprintf(`
		SELECT i.id, i.title, i.workspace_id
		FROM items i
		WHERE (
			CAST(NULLIF(i.custom_field_values,'') ->> '$."%s"' AS TEXT) = ?
			OR CAST(NULLIF(i.custom_field_values,'') ->> '$."%s".id' AS TEXT) = ?
		)
	`, fieldKey, fieldKey)
	rows, err := r.db.Query(query, assetIDStr, assetIDStr)
	if err != nil {
		return nil, fmt.Errorf("list items referencing asset in custom field: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ItemRefByCustomField
	for rows.Next() {
		var ref ItemRefByCustomField
		if err := rows.Scan(&ref.ID, &ref.Title, &ref.WorkspaceID); err != nil {
			return nil, fmt.Errorf("scan item ref: %w", err)
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// ItemGraphMetadata is the subset of item fields used to label a node in the
// asset-link graph view — workspace key + item number for display, workspace
// id for permission checks, and the current status name (empty string if
// unset).
type ItemGraphMetadata struct {
	WorkspaceKey        string
	WorkspaceItemNumber int
	WorkspaceID         int
	StatusName          string
}

// GetItemGraphMetadata returns the minimal per-item metadata the asset-link
// graph view needs. Returns ErrNotFound if the item does not exist.
func (r *ItemRepository) GetItemGraphMetadata(itemID int) (*ItemGraphMetadata, error) {
	var meta ItemGraphMetadata
	err := r.db.QueryRow(`
		SELECT w.key, i.workspace_item_number, i.workspace_id, COALESCE(s.name, '')
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE i.id = ?
	`, itemID).Scan(&meta.WorkspaceKey, &meta.WorkspaceItemNumber, &meta.WorkspaceID, &meta.StatusName)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get item graph metadata: %w", err)
	}
	return &meta, nil
}

// GetCustomFieldValuesRaw returns the raw JSON-encoded custom_field_values
// payload for an item. Migration/analysis handlers consume this directly
// without parsing into a map.
func (r *ItemRepository) GetCustomFieldValuesRaw(itemID int) (sql.NullString, error) {
	var data sql.NullString
	err := r.db.QueryRow("SELECT custom_field_values FROM items WHERE id = ?", itemID).Scan(&data)
	if err == sql.ErrNoRows {
		return sql.NullString{}, ErrNotFound
	}
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to get custom field values: %w", err)
	}
	return data, nil
}

// ListCustomFieldValuesByWorkspace streams every (item_id, custom_field_values)
// pair in a workspace. Used by the migration analyzer which has to inspect
// every item's stored field payload. The returned RowsIterator mirrors
// database/sql's Rows so callers can scan without re-implementing streaming.
func (r *ItemRepository) ListCustomFieldValuesByWorkspace(workspaceID int) (*sql.Rows, error) {
	rows, err := r.db.Query(
		`SELECT id, custom_field_values FROM items WHERE workspace_id = ?`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list custom field values: %w", err)
	}
	return rows, nil
}

// GetNextWorkspaceItemNumber returns the next item number for a workspace (atomic increment)
func (r *ItemRepository) GetNextWorkspaceItemNumber(tx database.Tx, workspaceID int) (int, error) {
	var nextNumber int
	err := tx.QueryRow(`
		SELECT COALESCE(MAX(workspace_item_number), 0) + 1
		FROM items
		WHERE workspace_id = ?
	`, workspaceID).Scan(&nextNumber)
	if err != nil {
		return 0, fmt.Errorf("failed to get next item number: %w", err)
	}
	return nextNumber, nil
}

// Create inserts a new item and returns its ID
func (r *ItemRepository) Create(tx database.Tx, item *models.Item) (int, error) {
	customFieldValuesJSON, err := marshalCustomFields(item.CustomFieldValues)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	var id int64
	err = tx.QueryRow(`
		INSERT INTO items (
			workspace_id, workspace_item_number, item_type_id, title, description, status_id,
			priority_id, due_date, start_date, end_date, is_task, milestone_id, iteration_id, project_id, inherit_project,
			assignee_id, creator_id, custom_field_values, parent_id, related_work_item_id,
			story_points, frac_index, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		item.WorkspaceID, item.WorkspaceItemNumber, item.ItemTypeID, item.Title, item.Description,
		item.StatusID, item.PriorityID, item.DueDate, item.StartDate, item.EndDate, item.IsTask, item.MilestoneID,
		item.IterationID, item.ProjectID, item.InheritProject, item.AssigneeID, item.CreatorID,
		customFieldValuesJSON, item.ParentID, item.RelatedWorkItemID,
		item.StoryPoints, item.FracIndex, now, now,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create item: %w", err)
	}

	return int(id), nil
}

// Update updates an existing item
func (r *ItemRepository) Update(tx database.Tx, item *models.Item) error {
	customFieldValuesJSON, err := marshalCustomFields(item.CustomFieldValues)
	if err != nil {
		return err
	}

	now := time.Now()
	_, err = tx.Exec(`
		UPDATE items
		SET workspace_id = ?, title = ?, description = ?, status_id = ?, priority_id = ?,
		    due_date = ?, start_date = ?, end_date = ?, milestone_id = ?, iteration_id = ?, project_id = ?, inherit_project = ?,
		    assignee_id = ?, creator_id = ?, custom_field_values = ?, parent_id = ?,
		    related_work_item_id = ?, story_points = ?, updated_at = ?
		WHERE id = ?
	`,
		item.WorkspaceID, item.Title, item.Description, item.StatusID, item.PriorityID,
		item.DueDate, item.StartDate, item.EndDate, item.MilestoneID, item.IterationID, item.ProjectID, item.InheritProject,
		item.AssigneeID, item.CreatorID, customFieldValuesJSON, item.ParentID,
		item.RelatedWorkItemID, item.StoryPoints, now, item.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	return nil
}

// allowedItemColumns is the whitelist of columns that UpdateFields may touch.
var allowedItemColumns = map[string]bool{
	"title": true, "description": true, "status_id": true, "priority_id": true,
	"due_date": true, "start_date": true, "end_date": true,
	"milestone_id": true, "iteration_id": true, "project_id": true, "inherit_project": true,
	"assignee_id": true, "creator_id": true, "custom_field_values": true,
	"parent_id": true, "related_work_item_id": true, "item_type_id": true,
	"frac_index": true, "is_task": true, "time_project_id": true,
	"story_points": true,
}

// UpdateFields updates only the specified columns of an item.
// Keys must be valid item column names; unknown keys return an error.
func (r *ItemRepository) UpdateFields(tx database.Tx, itemID int, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}

	setClauses := make([]string, 0, len(fields)+1)
	args := make([]interface{}, 0, len(fields)+2)

	for col, val := range fields {
		if !allowedItemColumns[col] {
			return fmt.Errorf("unknown item column: %s", col)
		}
		setClauses = append(setClauses, col+" = ?")
		args = append(args, val)
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, time.Now())
	args = append(args, itemID)

	query := "UPDATE items SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	_, err := tx.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update item fields: %w", err)
	}
	return nil
}

// Delete removes an item by ID
func (r *ItemRepository) Delete(tx database.Tx, id int) error {
	_, err := tx.Exec("DELETE FROM items WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}
	return nil
}

// DeleteItemLinks removes all links where the item is source or target
func (r *ItemRepository) DeleteItemLinks(tx database.Tx, itemID int) error {
	_, err := tx.Exec(`
		DELETE FROM item_links
		WHERE (source_type = 'item' AND source_id = ?) OR (target_type = 'item' AND target_id = ?)
	`, itemID, itemID)
	if err != nil {
		return fmt.Errorf("failed to delete item links: %w", err)
	}
	return nil
}

// ClearWorklogItemReferences clears item references from worklogs
func (r *ItemRepository) ClearWorklogItemReferences(tx database.Tx, itemID int) error {
	_, err := tx.Exec("UPDATE time_worklogs SET item_id = NULL WHERE item_id = ?", itemID)
	if err != nil {
		return fmt.Errorf("failed to clear worklog references: %w", err)
	}
	return nil
}

// GetParentID returns the parent_id for an item
func (r *ItemRepository) GetParentID(itemID int) (*int, error) {
	var parentID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT parent_id FROM items WHERE id = ?
	`, itemID).Scan(&parentID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get parent id: %w", err)
	}

	var result *int
	if parentID.Valid {
		val := int(parentID.Int64)
		result = &val
	}
	return result, nil
}

// Exists checks if an item exists
func (r *ItemRepository) Exists(id int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM items WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check item existence: %w", err)
	}
	return exists, nil
}

// GetCalendarData returns an item's calendar_data payload along with its workspace id,
// which callers need to authorize the caller against the item's workspace.
// A row with NULL calendar_data returns a non-valid sql.NullString.
func (r *ItemRepository) GetCalendarData(itemID int) (sql.NullString, int, error) {
	var data sql.NullString
	var workspaceID int
	err := r.db.QueryRow(
		"SELECT calendar_data, workspace_id FROM items WHERE id = ?",
		itemID,
	).Scan(&data, &workspaceID)
	if err == sql.ErrNoRows {
		return sql.NullString{}, 0, ErrNotFound
	}
	if err != nil {
		return sql.NullString{}, 0, fmt.Errorf("failed to get calendar data: %w", err)
	}
	return data, workspaceID, nil
}

// itemCountableColumns is the allow-list of columns the CountByField method
// may filter on. This keeps the column name out of dynamic SQL construction
// (structurally prevents injection) while covering the handful of "is this
// still in use?" checks that handlers need before delete/demote.
var itemCountableColumns = map[string]bool{
	"status_id":       true,
	"priority_id":     true,
	"item_type_id":    true,
	"milestone_id":    true,
	"iteration_id":    true,
	"project_id":      true,
	"time_project_id": true,
	"assignee_id":     true,
	"workspace_id":    true,
}

// CountByField returns the number of items where the given column equals value.
// The column name must be in itemCountableColumns — unknown columns produce an
// error rather than a dynamic query, so SQL injection is structurally impossible.
func (r *ItemRepository) CountByField(column string, value interface{}) (int, error) {
	if !itemCountableColumns[column] {
		return 0, fmt.Errorf("CountByField: column %q is not in the allow-list", column)
	}
	var count int
	// The column name is validated against a fixed allow-list above, so the
	// fmt.Sprintf here cannot splice attacker-controlled input.
	query := fmt.Sprintf("SELECT COUNT(*) FROM items WHERE %s = ?", column)
	if err := r.db.QueryRow(query, value).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count items by %s: %w", column, err)
	}
	return count, nil
}

// FindIDByKeyAndNumber resolves a "KEY-NUMBER" item reference (e.g. "TEST-42")
// to an internal item ID. The workspace key comparison is case-insensitive.
func (r *ItemRepository) FindIDByKeyAndNumber(workspaceKey string, itemNumber int) (int, error) {
	var id int
	err := r.db.QueryRow(
		"SELECT i.id FROM items i JOIN workspaces w ON i.workspace_id = w.id WHERE UPPER(w.key) = UPPER(?) AND i.workspace_item_number = ?",
		workspaceKey, itemNumber,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to resolve item by key: %w", err)
	}
	return id, nil
}

// GetFracIndex returns the fractional-index (used for drag-and-drop ordering)
// of an item. Returns nil when the column is NULL; ErrNotFound when the item
// doesn't exist.
func (r *ItemRepository) GetFracIndex(itemID int) (*string, error) {
	var fracIndex sql.NullString
	err := r.db.QueryRow("SELECT frac_index FROM items WHERE id = ?", itemID).Scan(&fracIndex)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get frac_index: %w", err)
	}
	if !fracIndex.Valid {
		return nil, nil
	}
	return &fracIndex.String, nil
}

// HubInboxFilter captures the user-scoped filters the hub inbox endpoint
// supports: an optional portal_id, an optional status name, and pagination.
type HubInboxFilter struct {
	UserID       int
	PortalID     *int   // nil = no portal filter
	StatusFilter string // empty = no status filter
	PerPage      int
	Offset       int
}

// ListHubInboxItems returns the items submitted via portal by the given user,
// the total row count (ignoring pagination but honoring portal + status
// filters), and the distinct status-name/color facets across the user's
// submissions (computed without the status filter so the UI dropdown keeps
// every option visible).
func (r *ItemRepository) ListHubInboxItems(ctx context.Context, f HubInboxFilter) ([]models.HubInboxItem, int, []models.HubInboxStatusFacet, error) {
	baseFrom := `
		FROM items i
		JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN channels c ON i.channel_id = c.id
		LEFT JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
		WHERE c.type = 'portal' AND i.creator_id = ?
	`
	facetArgs := []interface{}{f.UserID}
	if f.PortalID != nil {
		baseFrom += " AND c.id = ?"
		facetArgs = append(facetArgs, *f.PortalID)
	}

	baseQuery := baseFrom
	args := append([]interface{}{}, facetArgs...)
	if f.StatusFilter != "" {
		baseQuery += " AND s.name = ?"
		args = append(args, f.StatusFilter)
	}

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT i.id) "+baseQuery, args...).Scan(&total); err != nil {
		return nil, 0, nil, fmt.Errorf("hub inbox count: %w", err)
	}

	itemArgs := append([]interface{}{}, args...)
	itemArgs = append(itemArgs, f.PerPage, f.Offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			i.id, i.title, COALESCE(i.description, ''), i.created_at,
			s.name, COALESCE(sc.color, '#6b7280'),
			w.key, i.workspace_item_number,
			COALESCE(c.name, ''), COALESCE(JSON_EXTRACT(c.config, '$.portal_slug'), ''),
			pc.name, pc.email
	`+baseQuery+`
		ORDER BY i.created_at DESC
		LIMIT ? OFFSET ?
	`, itemArgs...)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("hub inbox list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []models.HubInboxItem{}
	for rows.Next() {
		var item models.HubInboxItem
		var submitterName, submitterEmail sql.NullString
		if err := rows.Scan(
			&item.ID, &item.Title, &item.Description, &item.CreatedAt,
			&item.StatusName, &item.StatusColor,
			&item.WorkspaceKey, &item.WorkspaceItemNumber,
			&item.PortalName, &item.PortalSlug,
			&submitterName, &submitterEmail,
		); err != nil {
			return nil, 0, nil, fmt.Errorf("scan hub inbox row: %w", err)
		}
		if submitterName.Valid {
			item.SubmitterName = &submitterName.String
		}
		if submitterEmail.Valid {
			item.SubmitterEmail = &submitterEmail.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, err
	}

	facets := []models.HubInboxStatusFacet{}
	facetRows, err := r.db.QueryContext(ctx, "SELECT DISTINCT s.name, COALESCE(sc.color, '#6b7280') "+baseFrom+" ORDER BY s.name ASC", facetArgs...)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("hub inbox facets: %w", err)
	}
	defer func() { _ = facetRows.Close() }()
	for facetRows.Next() {
		var f models.HubInboxStatusFacet
		if err := facetRows.Scan(&f.Name, &f.Color); err != nil {
			return nil, 0, nil, err
		}
		facets = append(facets, f)
	}
	return items, total, facets, facetRows.Err()
}

// FindHubInboxItem returns a single hub-inbox item (portal submission) owned
// by the given user. ErrNotFound when the row doesn't exist or belongs to
// someone else.
func (r *ItemRepository) FindHubInboxItem(ctx context.Context, userID, itemID int) (*models.HubInboxItem, error) {
	var item models.HubInboxItem
	var submitterName, submitterEmail sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT
			i.id, i.title, COALESCE(i.description, ''), i.created_at,
			s.name, COALESCE(sc.color, '#6b7280'),
			w.key, i.workspace_item_number,
			COALESCE(c.name, ''), COALESCE(JSON_EXTRACT(c.config, '$.portal_slug'), ''),
			pc.name, pc.email
		FROM items i
		JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN channels c ON i.channel_id = c.id
		LEFT JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
		WHERE i.id = ? AND c.type = 'portal' AND i.creator_id = ?
	`, itemID, userID).Scan(
		&item.ID, &item.Title, &item.Description, &item.CreatedAt,
		&item.StatusName, &item.StatusColor,
		&item.WorkspaceKey, &item.WorkspaceItemNumber,
		&item.PortalName, &item.PortalSlug,
		&submitterName, &submitterEmail,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find hub inbox item: %w", err)
	}
	if submitterName.Valid {
		item.SubmitterName = &submitterName.String
	}
	if submitterEmail.Valid {
		item.SubmitterEmail = &submitterEmail.String
	}
	return &item, nil
}

// ListItemsLinkedToTestResult returns the items associated with a given test
// result, ordered by most recently linked first. Scoped to the workspace via
// the test run so a stale result ID from a different workspace can't leak.
func (r *ItemRepository) ListItemsLinkedToTestResult(resultID, workspaceID int) ([]models.Item, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.workspace_item_number, i.title, i.item_type_id, i.status_id, i.created_at
		FROM items i
		JOIN test_result_items tri ON i.id = tri.item_id
		JOIN test_results tr ON tri.test_result_id = tr.id
		JOIN test_runs run ON tr.run_id = run.id
		WHERE tri.test_result_id = ? AND run.workspace_id = ?
		ORDER BY tri.created_at DESC
	`, resultID, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list items linked to test result: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]models.Item, 0)
	for rows.Next() {
		var item models.Item
		if err := rows.Scan(&item.ID, &item.WorkspaceItemNumber, &item.Title, &item.ItemTypeID, &item.StatusID, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan item linked to test result: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ItemCustomFields is the projection returned by ListItemCustomFieldsTx —
// each row is an item id with its raw JSON-encoded custom_field_values.
type ItemCustomFields struct {
	ID      int
	CFVJSON string
}

// ListItemCustomFieldsTx streams (id, custom_field_values) rows for every item
// in the given workspaces within the provided transaction. NULL payloads are
// coerced to "{}" so callers can unmarshal without an extra branch.
func (r *ItemRepository) ListItemCustomFieldsTx(tx database.Tx, workspaceIDs []int) ([]ItemCustomFields, error) {
	if len(workspaceIDs) == 0 {
		return []ItemCustomFields{}, nil
	}
	placeholders := make([]string, len(workspaceIDs))
	args := make([]interface{}, len(workspaceIDs))
	for i, id := range workspaceIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT id, COALESCE(custom_field_values, '{}') as cfv
		FROM items
		WHERE workspace_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list item custom fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []ItemCustomFields{}
	for rows.Next() {
		var row ItemCustomFields
		if err := rows.Scan(&row.ID, &row.CFVJSON); err != nil {
			return nil, fmt.Errorf("scan item custom fields row: %w", err)
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

// SearchLinkableItems returns a set of items whose title or description
// matches the substring, scoped to the given workspaces and optionally
// restricted to the given item types. Used by the link picker.
func (r *ItemRepository) SearchLinkableItems(query string, workspaceIDs, itemTypeIDs []int, limit int) ([]models.LinkableItem, error) {
	if len(workspaceIDs) == 0 {
		return []models.LinkableItem{}, nil
	}
	wsPlaceholders := make([]string, len(workspaceIDs))
	args := []interface{}{}
	args = append(args, "%"+query+"%", "%"+query+"%")
	for i, id := range workspaceIDs {
		wsPlaceholders[i] = "?"
		args = append(args, id)
	}

	itemTypeFilter := ""
	if len(itemTypeIDs) > 0 {
		itPlaceholders := make([]string, len(itemTypeIDs))
		for i, id := range itemTypeIDs {
			itPlaceholders[i] = "?"
			args = append(args, id)
		}
		itemTypeFilter = fmt.Sprintf(" AND i.item_type_id IN (%s)", strings.Join(itPlaceholders, ","))
	}
	args = append(args, limit)

	sqlQuery := fmt.Sprintf(`
		SELECT
			i.id,
			i.title,
			COALESCE(i.description, '') AS description,
			i.workspace_id,
			w.name AS workspace_name,
			COALESCE(s.name, '') AS status_name,
			COALESCE(p.name, '') AS priority_name,
			i.item_type_id,
			COALESCE(it.name, '') AS item_type_name
		FROM items i
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE (i.title LIKE ? OR i.description LIKE ?)
		  AND i.workspace_id IN (%s)%s
		ORDER BY i.title
		LIMIT ?
	`, strings.Join(wsPlaceholders, ","), itemTypeFilter)

	rows, err := r.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := []models.LinkableItem{}
	for rows.Next() {
		var item models.LinkableItem
		var description, workspaceName, statusName, priorityName, itemTypeName sql.NullString
		var workspaceID, itemTypeID sql.NullInt64

		if err := rows.Scan(
			&item.ID, &item.Title, &description,
			&workspaceID, &workspaceName,
			&statusName, &priorityName,
			&itemTypeID, &itemTypeName,
		); err != nil {
			return nil, err
		}

		item.Description = description.String
		if workspaceID.Valid {
			v := int(workspaceID.Int64)
			item.WorkspaceID = &v
		}
		item.WorkspaceName = workspaceName.String
		item.Status = statusName.String
		item.Priority = priorityName.String
		if itemTypeID.Valid {
			v := int(itemTypeID.Int64)
			item.ItemTypeID = &v
		}
		item.ItemTypeName = itemTypeName.String
		item.Type = "item"
		items = append(items, item)
	}
	return items, rows.Err()
}

// PublicItem is the projection returned by FindPublicItemByKeyAndNumber. It
// carries the visual metadata (status category color, item-type icon/color)
// the public board view needs but FindByIDWithDetails doesn't surface on the
// regular Item model.
type PublicItem struct {
	ID             int
	Title          string
	Description    string
	StatusName     string
	StatusColor    string
	PriorityName   string
	PriorityIcon   string
	PriorityColor  string
	ItemTypeName   string
	ItemTypeIcon   string
	ItemTypeColor  string
	AssigneeName   string
	AssigneeAvatar string
	DueDate        string // nullable — empty string when unset
	StoryPoints    *float64
	CreatedAt      string
}

// FindPublicItemByKeyAndNumber returns the visual projection of an item
// identified by (workspace_key, workspace_item_number). Used by the public
// board view which has its own color/icon needs.
func (r *ItemRepository) FindPublicItemByKeyAndNumber(workspaceKey string, itemNumber int) (*PublicItem, error) {
	var p PublicItem
	var statusName, statusColor sql.NullString
	var priorityName, priorityIcon, priorityColor sql.NullString
	var itemTypeName, itemTypeIcon, itemTypeColor sql.NullString
	var assigneeName, assigneeAvatar sql.NullString
	var dueDate sql.NullString
	var storyPoints sql.NullFloat64

	err := r.db.QueryRow(`
		SELECT i.id, i.title, COALESCE(i.description, ''),
		       s.name, sc.color,
		       p.name, p.icon, p.color,
		       it.name, it.icon, it.color,
		       COALESCE(u.first_name || ' ' || u.last_name, ''), u.avatar_url,
		       i.due_date, i.story_points, i.created_at
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE w.key = ? AND i.workspace_item_number = ?
	`, workspaceKey, itemNumber).Scan(
		&p.ID, &p.Title, &p.Description,
		&statusName, &statusColor,
		&priorityName, &priorityIcon, &priorityColor,
		&itemTypeName, &itemTypeIcon, &itemTypeColor,
		&assigneeName, &assigneeAvatar,
		&dueDate, &storyPoints, &p.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find public item: %w", err)
	}

	p.StatusName = statusName.String
	p.StatusColor = statusColor.String
	p.PriorityName = priorityName.String
	p.PriorityIcon = priorityIcon.String
	p.PriorityColor = priorityColor.String
	p.ItemTypeName = itemTypeName.String
	p.ItemTypeIcon = itemTypeIcon.String
	p.ItemTypeColor = itemTypeColor.String
	p.AssigneeName = assigneeName.String
	p.AssigneeAvatar = assigneeAvatar.String
	p.DueDate = dueDate.String
	if storyPoints.Valid {
		p.StoryPoints = &storyPoints.Float64
	}
	return &p, nil
}

// KeyReference maps an "ITEMKEY-NUMBER" string to its item id and workspace id.
type KeyReference struct {
	ItemKey     string
	ItemID      int
	WorkspaceID int
}

// ResolveItemKeyReferences resolves a set of "ITEMKEY-NUMBER" strings to the
// underlying item ids and workspace ids. Unknown keys are silently omitted.
// Returns an empty slice when keys is empty.
func (r *ItemRepository) ResolveItemKeyReferences(keys []string) ([]KeyReference, error) {
	if len(keys) == 0 {
		return []KeyReference{}, nil
	}
	placeholders := make([]string, len(keys))
	args := make([]interface{}, len(keys))
	for i, k := range keys {
		placeholders[i] = "?"
		args[i] = k
	}
	query := `SELECT w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.id, i.workspace_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE w.key || '-' || CAST(i.workspace_item_number AS TEXT) IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve item keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []KeyReference{}
	for rows.Next() {
		var ref KeyReference
		if err := rows.Scan(&ref.ItemKey, &ref.ItemID, &ref.WorkspaceID); err != nil {
			return nil, fmt.Errorf("scan key reference: %w", err)
		}
		results = append(results, ref)
	}
	return results, rows.Err()
}

// CandidateItem is the projection returned by ListOpenCandidatesInWorkspace —
// the fields the AI link-suggestion prompt needs to describe a candidate.
type CandidateItem struct {
	ID          int
	ItemKey     string
	Title       string
	StatusName  string
	Description string
}

// ListOpenCandidatesInWorkspace returns up to `limit` non-completed items in
// the given workspace, excluding excludeID. Ordered by creation time desc so
// the most recently created items surface first.
func (r *ItemRepository) ListOpenCandidatesInWorkspace(workspaceID, excludeID, limit int) ([]CandidateItem, error) {
	rows, err := r.db.Query(
		`SELECT i.id, w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.title,
		        COALESCE(s.name, '') as status_name, COALESCE(i.description, '') as description
		 FROM items i
		 JOIN workspaces w ON i.workspace_id = w.id
		 LEFT JOIN statuses s ON i.status_id = s.id
		 LEFT JOIN status_categories sc ON s.category_id = sc.id
		 WHERE i.workspace_id = ? AND i.id != ?
		   AND COALESCE(sc.is_completed, FALSE) = FALSE
		 ORDER BY i.created_at DESC LIMIT ?`,
		workspaceID, excludeID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list open candidates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []CandidateItem{}
	for rows.Next() {
		var c CandidateItem
		if err := rows.Scan(&c.ID, &c.ItemKey, &c.Title, &c.StatusName, &c.Description); err != nil {
			return nil, fmt.Errorf("scan candidate row: %w", err)
		}
		results = append(results, c)
	}
	return results, rows.Err()
}

// IterationItemInfo is the projection returned by ListIterationItems — the
// fields used by the AI dependency analyzer prompt.
type IterationItemInfo struct {
	ID            int
	Key           string
	Title         string
	Description   string
	StatusName    string
	PriorityName  string
	ItemTypeName  string
	AssigneeName  string
	WorkspaceID   int
	WorkspaceKey  string
	WorkspaceName string
	IterationID   int
}

// ListIterationItems returns up to 100 items that live in any of the given
// iterations and workspaces, ordered for deterministic prompt generation.
func (r *ItemRepository) ListIterationItems(iterationIDs, workspaceIDs []int) ([]IterationItemInfo, error) {
	if len(iterationIDs) == 0 || len(workspaceIDs) == 0 {
		return []IterationItemInfo{}, nil
	}
	iterPlaceholders := make([]string, len(iterationIDs))
	iterArgs := make([]interface{}, len(iterationIDs))
	for i, id := range iterationIDs {
		iterPlaceholders[i] = "?"
		iterArgs[i] = id
	}
	wsPlaceholders := make([]string, len(workspaceIDs))
	wsArgs := make([]interface{}, len(workspaceIDs))
	for i, id := range workspaceIDs {
		wsPlaceholders[i] = "?"
		wsArgs[i] = id
	}
	query := fmt.Sprintf(`
		SELECT i.id, CONCAT(w.key, '-', i.workspace_item_number) as item_key,
		       i.title, COALESCE(i.description, '') as description,
		       COALESCE(s.name, '') as status_name,
		       COALESCE(p.name, '') as priority_name,
		       COALESCE(it.name, '') as item_type_name,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
		       i.workspace_id, w.key as workspace_key, w.name as workspace_name,
		       i.iteration_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.iteration_id IN (%s)
		  AND i.workspace_id IN (%s)
		ORDER BY i.iteration_id, i.workspace_id, i.workspace_item_number
		LIMIT 100`,
		strings.Join(iterPlaceholders, ","),
		strings.Join(wsPlaceholders, ","))

	args := make([]interface{}, 0, len(iterArgs)+len(wsArgs))
	args = append(args, iterArgs...)
	args = append(args, wsArgs...)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list iteration items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []IterationItemInfo{}
	for rows.Next() {
		var item IterationItemInfo
		if err := rows.Scan(&item.ID, &item.Key, &item.Title, &item.Description,
			&item.StatusName, &item.PriorityName, &item.ItemTypeName, &item.AssigneeName,
			&item.WorkspaceID, &item.WorkspaceKey, &item.WorkspaceName,
			&item.IterationID); err != nil {
			return nil, fmt.Errorf("scan iteration item row: %w", err)
		}
		results = append(results, item)
	}
	return results, rows.Err()
}

// ItemWithCalendar augments a bare Item with the parsed calendar schedule
// entries and the owning workspace's is_personal flag — the shape needed by
// the scheduled-items endpoint.
type ItemWithCalendar struct {
	Item            models.Item
	CalendarEntries []models.CalendarScheduleEntry
	IsPersonal      bool
}

// ListItemsWithCalendarData returns every item in the given workspaces whose
// calendar_data column is populated, each with parsed entries and the
// workspace's is_personal flag. Items with malformed calendar JSON are
// skipped (logged by the caller, not at the repository boundary).
func (r *ItemRepository) ListItemsWithCalendarData(workspaceIDs []int) ([]ItemWithCalendar, error) {
	if len(workspaceIDs) == 0 {
		return []ItemWithCalendar{}, nil
	}
	placeholders := make([]string, len(workspaceIDs))
	args := make([]interface{}, len(workspaceIDs))
	for i, id := range workspaceIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
		       i.status_id, i.priority_id, i.assignee_id, i.creator_id,
		       i.calendar_data, i.due_date, i.created_at, i.updated_at,
		       w.name, w.key, w.is_personal
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.calendar_data IS NOT NULL AND i.calendar_data != ''
		  AND i.workspace_id IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list items with calendar data: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []ItemWithCalendar{}
	for rows.Next() {
		var item models.Item
		var description, calendarDataJSON sql.NullString
		var statusID, priorityID, assigneeID, creatorID sql.NullInt64
		var dueDate sql.NullTime
		var isPersonal bool

		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &item.Title, &description,
			&statusID, &priorityID, &assigneeID, &creatorID,
			&calendarDataJSON, &dueDate, &item.CreatedAt, &item.UpdatedAt,
			&item.WorkspaceName, &item.WorkspaceKey, &isPersonal,
		); err != nil {
			return nil, fmt.Errorf("scan scheduled item row: %w", err)
		}

		item.Description = description.String
		assignNullableInt(&item.StatusID, statusID)
		assignNullableInt(&item.PriorityID, priorityID)
		assignNullableInt(&item.AssigneeID, assigneeID)
		assignNullableInt(&item.CreatorID, creatorID)
		if dueDate.Valid {
			item.DueDate = &dueDate.Time
		}

		var entries []models.CalendarScheduleEntry
		if calendarDataJSON.Valid && calendarDataJSON.String != "" {
			if err := json.Unmarshal([]byte(calendarDataJSON.String), &entries); err != nil {
				// Malformed JSON — skip this row but continue the iteration.
				continue
			}
		}

		results = append(results, ItemWithCalendar{
			Item:            item,
			CalendarEntries: entries,
			IsPersonal:      isPersonal,
		})
	}
	return results, rows.Err()
}

// FindByIDsWithDetails returns items for a set of IDs, each populated the same
// way FindByIDWithDetails populates a single item. IDs not found are silently
// omitted from the result. Returns an empty slice (not nil) when ids is empty.
func (r *ItemRepository) FindByIDsWithDetails(ids []int) ([]*models.Item, error) {
	if len(ids) == 0 {
		return []*models.Item{}, nil
	}
	items := make([]*models.Item, 0, len(ids))
	for _, id := range ids {
		item, err := r.FindByIDWithDetails(id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// Helper functions

func assignNullableInt(dest **int, src sql.NullInt64) {
	if src.Valid {
		val := int(src.Int64)
		*dest = &val
	}
}

func assignNullableString(dest *string, src sql.NullString) {
	if src.Valid {
		*dest = src.String
	}
}

func marshalCustomFields(customFields map[string]interface{}) (sql.NullString, error) {
	if len(customFields) == 0 {
		return sql.NullString{Valid: false}, nil
	}

	data, err := json.Marshal(customFields)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to marshal custom fields: %w", err)
	}

	return sql.NullString{String: string(data), Valid: true}, nil
}

// --- Homepage aggregations --------------------------------------------------

// CountActiveNonPersonalItems returns the number of items in active, non-
// personal workspaces. Used by the homepage onboarding banner.
func (r *ItemRepository) CountActiveNonPersonalItems() (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE (w.is_personal = false OR w.is_personal IS NULL)
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count non-personal items: %w", err)
	}
	return count, nil
}

// HomepageItemSummary carries the subset of item metadata the homepage
// activity widget needs. Milestone ID is included so the caller can
// aggregate by milestone without a second round trip.
type HomepageItemSummary struct {
	ItemID              int
	WorkspaceID         int
	WorkspaceItemNumber int
	Title               string
	Status              string
	PriorityID          *int
	PriorityName        *string
	PriorityColor       *string
	WorkspaceKey        string
	MilestoneID         *int
}

// ListHomepageItemSummaries returns the homepage widget's item summaries for
// the given item IDs. Missing IDs are silently omitted. The result order is
// not guaranteed — callers index by ItemID.
func (r *ItemRepository) ListHomepageItemSummaries(itemIDs []int) ([]HomepageItemSummary, error) {
	if len(itemIDs) == 0 {
		return []HomepageItemSummary{}, nil
	}
	placeholders := make([]string, len(itemIDs))
	args := make([]interface{}, len(itemIDs))
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title,
		       COALESCE(s.name, 'Unknown') as status,
		       i.priority_id, p.name as priority_name, p.color as priority_color,
		       w.key as workspace_key, i.milestone_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		WHERE i.id IN (` + strings.Join(placeholders, ",") + `)`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list homepage items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []HomepageItemSummary{}
	for rows.Next() {
		var s HomepageItemSummary
		var priorityID, milestoneID sql.NullInt64
		var priorityName, priorityColor sql.NullString
		if err := rows.Scan(
			&s.ItemID, &s.WorkspaceID, &s.WorkspaceItemNumber, &s.Title,
			&s.Status,
			&priorityID, &priorityName, &priorityColor,
			&s.WorkspaceKey, &milestoneID,
		); err != nil {
			return nil, fmt.Errorf("scan homepage item: %w", err)
		}
		if priorityID.Valid {
			v := int(priorityID.Int64)
			s.PriorityID = &v
		}
		if priorityName.Valid {
			v := priorityName.String
			s.PriorityName = &v
		}
		if priorityColor.Valid {
			v := priorityColor.String
			s.PriorityColor = &v
		}
		if milestoneID.Valid {
			v := int(milestoneID.Int64)
			s.MilestoneID = &v
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

// TopMilestoneIDsForItems returns the `limit` most frequently referenced
// milestone_id values among the given item IDs, ordered by frequency desc,
// then ascending milestone_id for stability. NULL milestone_ids are ignored.
func (r *ItemRepository) TopMilestoneIDsForItems(itemIDs []int, limit int) ([]int, error) {
	if len(itemIDs) == 0 || limit <= 0 {
		return []int{}, nil
	}
	placeholders := make([]string, len(itemIDs))
	args := make([]interface{}, len(itemIDs), len(itemIDs)+1)
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `
		SELECT milestone_id, COUNT(*) as freq
		FROM items
		WHERE id IN (` + strings.Join(placeholders, ",") + `)
		  AND milestone_id IS NOT NULL
		GROUP BY milestone_id
		ORDER BY freq DESC, milestone_id ASC
		LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("top milestone frequencies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int
	for rows.Next() {
		var id int
		var freq int
		if err := rows.Scan(&id, &freq); err != nil {
			return nil, fmt.Errorf("scan milestone freq: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// --- Workspace statistics aggregations --------------------------------------

// WorkspaceStatsAssignment is one row of the assignment-distribution facet
// returned by ComputeWorkspaceItemStats. UserID is nil for unassigned items.
type WorkspaceStatsAssignment struct {
	UserID    *int
	UserName  string
	FirstName string
	LastName  string
	ItemCount int
}

// WorkspaceStatsProject is one row of the project-statistics facet.
type WorkspaceStatsProject struct {
	ProjectID      *int
	ProjectName    string
	ProjectColor   string
	ItemCount      int
	CompletedCount int
}

// WorkspaceItemStats bundles the five item-scoped aggregations the workspace
// stats endpoint needs. Collections count and milestone progress live
// elsewhere and are composed into the handler's response separately.
type WorkspaceItemStats struct {
	TotalItems             int
	ItemsByStatusCategory  map[string]int
	AssignmentDistribution []WorkspaceStatsAssignment
	ProjectStatistics      []WorkspaceStatsProject
	PriorityBreakdown      map[string]int
}

// ComputeWorkspaceItemStats runs the five item aggregations the workspace
// stats dashboard depends on (total, status-category breakdown, assignment
// distribution over `since`, project statistics over `since`, priority
// breakdown over `since`). All five share the workspace-id filter and the
// optional VQL-derived `filterSQL` + `filterArgs`.
func (r *ItemRepository) ComputeWorkspaceItemStats(workspaceID int, filterSQL string, filterArgs []interface{}, since time.Time) (*WorkspaceItemStats, error) {
	stats := &WorkspaceItemStats{
		ItemsByStatusCategory:  make(map[string]int),
		AssignmentDistribution: []WorkspaceStatsAssignment{},
		ProjectStatistics:      []WorkspaceStatsProject{},
		PriorityBreakdown:      make(map[string]int),
	}

	// 1. Total items
	totalQuery := `
		SELECT COUNT(*)
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.workspace_id = ?`
	totalArgs := []interface{}{workspaceID}
	if filterSQL != "" {
		totalQuery += " AND (" + filterSQL + ")"
		totalArgs = append(totalArgs, filterArgs...)
	}
	if err := r.db.QueryRow(totalQuery, totalArgs...).Scan(&stats.TotalItems); err != nil {
		return nil, fmt.Errorf("count workspace items: %w", err)
	}

	// 2. Items by status category
	statusQuery := `
		SELECT sc.name, COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?`
	statusArgs := []interface{}{workspaceID}
	if filterSQL != "" {
		statusQuery += " AND (" + filterSQL + ")"
		statusArgs = append(statusArgs, filterArgs...)
	}
	statusQuery += ` GROUP BY sc.name`
	rows, err := r.db.Query(statusQuery, statusArgs...)
	if err != nil {
		return nil, fmt.Errorf("status category breakdown: %w", err)
	}
	for rows.Next() {
		var categoryName sql.NullString
		var count int
		if err := rows.Scan(&categoryName, &count); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan status breakdown: %w", err)
		}
		if categoryName.Valid {
			stats.ItemsByStatusCategory[categoryName.String] = count
		}
	}
	_ = rows.Close()

	// 3. Assignment distribution (since cutoff)
	assignmentQuery := `
		SELECT
			i.assignee_id,
			COALESCE(u.username, 'Unassigned') as user_name,
			COALESCE(u.first_name, '') as first_name,
			COALESCE(u.last_name, '') as last_name,
			COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`
	assignmentArgs := []interface{}{workspaceID, since}
	if filterSQL != "" {
		assignmentQuery += " AND (" + filterSQL + ")"
		assignmentArgs = append(assignmentArgs, filterArgs...)
	}
	assignmentQuery += `
		GROUP BY i.assignee_id, u.username, u.first_name, u.last_name
		ORDER BY item_count DESC
		LIMIT 10`
	assignRows, err := r.db.Query(assignmentQuery, assignmentArgs...)
	if err != nil {
		return nil, fmt.Errorf("assignment distribution: %w", err)
	}
	for assignRows.Next() {
		var row WorkspaceStatsAssignment
		var assigneeID sql.NullInt64
		if err := assignRows.Scan(&assigneeID, &row.UserName, &row.FirstName, &row.LastName, &row.ItemCount); err != nil {
			_ = assignRows.Close()
			return nil, fmt.Errorf("scan assignment row: %w", err)
		}
		if assigneeID.Valid {
			id := int(assigneeID.Int64)
			row.UserID = &id
		}
		stats.AssignmentDistribution = append(stats.AssignmentDistribution, row)
	}
	_ = assignRows.Close()

	// 4. Project statistics (since cutoff)
	projectQuery := `
		SELECT
			tp.id,
			tp.name,
			tp.color,
			COUNT(i.id) as item_count,
			SUM(CASE WHEN COALESCE(sc.is_completed, FALSE) = TRUE THEN 1 ELSE 0 END) as completed_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?
		  AND i.time_project_id IS NOT NULL`
	projectArgs := []interface{}{workspaceID, since}
	if filterSQL != "" {
		projectQuery += " AND (" + filterSQL + ")"
		projectArgs = append(projectArgs, filterArgs...)
	}
	projectQuery += `
		GROUP BY tp.id, tp.name, tp.color
		ORDER BY item_count DESC
		LIMIT 10`
	projectRows, err := r.db.Query(projectQuery, projectArgs...)
	if err != nil {
		return nil, fmt.Errorf("project statistics: %w", err)
	}
	for projectRows.Next() {
		var row WorkspaceStatsProject
		var projectID sql.NullInt64
		var projectColor sql.NullString
		if err := projectRows.Scan(&projectID, &row.ProjectName, &projectColor, &row.ItemCount, &row.CompletedCount); err != nil {
			_ = projectRows.Close()
			return nil, fmt.Errorf("scan project row: %w", err)
		}
		if projectID.Valid {
			id := int(projectID.Int64)
			row.ProjectID = &id
		}
		row.ProjectColor = projectColor.String
		stats.ProjectStatistics = append(stats.ProjectStatistics, row)
	}
	_ = projectRows.Close()

	// 5. Priority breakdown (since cutoff)
	priorityQuery := `
		SELECT
			COALESCE(pri.name, 'None') as priority,
			COUNT(i.id) as item_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`
	priorityArgs := []interface{}{workspaceID, since}
	if filterSQL != "" {
		priorityQuery += " AND (" + filterSQL + ")"
		priorityArgs = append(priorityArgs, filterArgs...)
	}
	priorityQuery += ` GROUP BY pri.name`
	priorityRows, err := r.db.Query(priorityQuery, priorityArgs...)
	if err != nil {
		return nil, fmt.Errorf("priority breakdown: %w", err)
	}
	for priorityRows.Next() {
		var priority string
		var count int
		if err := priorityRows.Scan(&priority, &count); err != nil {
			_ = priorityRows.Close()
			return nil, fmt.Errorf("scan priority row: %w", err)
		}
		stats.PriorityBreakdown[priority] = count
	}
	_ = priorityRows.Close()

	return stats, nil
}

// --- Personal-workspace item helpers ----------------------------------------

// ListRelatedPersonalItems returns items in personalWorkspaceID that are linked
// via related_work_item_id to the given work item, hydrated with workspace,
// item_type, status, priority, and assignee names used by the personal-tasks
// widget. Results are ordered newest-first.
func (r *ItemRepository) ListRelatedPersonalItems(relatedWorkItemID, personalWorkspaceID int) ([]models.Item, error) {
	query := `
		SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
			i.status_id, i.priority_id, i.is_task, i.milestone_id,
			i.project_id, i.inherit_project, i.time_project_id, i.assignee_id, i.creator_id,
			i.calendar_data, i.parent_id,
			i.frac_index, i.related_work_item_id,
			i.created_at, i.updated_at,
			w.name AS workspace_name, w.key AS workspace_key,
			it.name AS item_type_name,
			st.name AS status_name,
			pri.name AS priority_name, pri.icon AS priority_icon, pri.color AS priority_color,
			assignee.first_name || ' ' || assignee.last_name AS assignee_name,
			assignee.email AS assignee_email,
			assignee.avatar_url AS assignee_avatar
		FROM items i
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		WHERE i.related_work_item_id = ? AND i.workspace_id = ?
		ORDER BY i.created_at DESC`

	rows, err := r.db.Query(query, relatedWorkItemID, personalWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("list related personal items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []models.Item
	for rows.Next() {
		var item models.Item
		var calendarDataJSON sql.NullString
		var itemTypeName, statusName, priorityName, priorityIcon, priorityColor sql.NullString
		var assigneeName, assigneeEmail, assigneeAvatar sql.NullString

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &item.ItemTypeID, &item.Title, &item.Description,
			&item.StatusID, &item.PriorityID, &item.IsTask, &item.MilestoneID,
			&item.ProjectID, &item.InheritProject, &item.TimeProjectID, &item.AssigneeID, &item.CreatorID,
			&calendarDataJSON, &item.ParentID,
			&item.FracIndex, &item.RelatedWorkItemID,
			&item.CreatedAt, &item.UpdatedAt,
			&item.WorkspaceName, &item.WorkspaceKey,
			&itemTypeName,
			&statusName,
			&priorityName, &priorityIcon, &priorityColor,
			&assigneeName, &assigneeEmail, &assigneeAvatar,
		)
		if err != nil {
			return nil, fmt.Errorf("scan related personal item: %w", err)
		}

		if itemTypeName.Valid {
			item.ItemTypeName = itemTypeName.String
		}
		if statusName.Valid {
			item.StatusName = statusName.String
		}
		if priorityName.Valid {
			item.PriorityName = priorityName.String
		}
		if priorityIcon.Valid {
			item.PriorityIcon = priorityIcon.String
		}
		if priorityColor.Valid {
			item.PriorityColor = priorityColor.String
		}
		if assigneeName.Valid {
			item.AssigneeName = assigneeName.String
		}
		if assigneeEmail.Valid {
			item.AssigneeEmail = assigneeEmail.String
		}
		if assigneeAvatar.Valid {
			item.AssigneeAvatar = assigneeAvatar.String
		}

		if calendarDataJSON.Valid && calendarDataJSON.String != "" {
			if err := json.Unmarshal([]byte(calendarDataJSON.String), &item.CalendarData); err != nil {
				item.CalendarData = []models.CalendarScheduleEntry{}
			}
		} else {
			item.CalendarData = []models.CalendarScheduleEntry{}
		}

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate related personal items: %w", err)
	}
	return items, nil
}

// ItemWorkspaceOwnership describes the workspace an item belongs to, whether
// that workspace is personal, and who owns it. Returned by
// GetItemWorkspaceOwnership; callers use this to enforce personal-workspace
// access rules.
type ItemWorkspaceOwnership struct {
	WorkspaceID int
	IsPersonal  bool
	OwnerID     *int
}

// GetItemWorkspaceOwnership returns the workspace ID for an item along with
// is_personal and owner_id of that workspace. Returns ErrNotFound if the item
// does not exist.
func (r *ItemRepository) GetItemWorkspaceOwnership(itemID int) (*ItemWorkspaceOwnership, error) {
	var out ItemWorkspaceOwnership
	var ownerID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT i.workspace_id, w.is_personal, w.owner_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, itemID).Scan(&out.WorkspaceID, &out.IsPersonal, &ownerID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get item workspace ownership: %w", err)
	}
	if ownerID.Valid {
		v := int(ownerID.Int64)
		out.OwnerID = &v
	}
	return &out, nil
}

// --- Portal customer ticket lookups -----------------------------------------

// PortalCustomerSubmission is one row returned by
// ListPortalCustomerSubmissions — a ticket a portal customer created, with
// workspace + status metadata for display in the customer profile.
type PortalCustomerSubmission struct {
	ID            int
	WorkspaceID   int
	WorkspaceName string
	WorkspaceKey  string
	Title         string
	Description   string
	StatusName    string
	StatusColor   string
	CreatedAt     string
}

// ListPortalCustomerSubmissions returns all items created by the given portal
// customer, newest-first, hydrated with workspace name/key and status
// name/color (falling back to empty string / neutral color for NULLs).
func (r *ItemRepository) ListPortalCustomerSubmissions(customerID int) ([]PortalCustomerSubmission, error) {
	rows, err := r.db.Query(`
		SELECT
			i.id, i.workspace_id, i.title, i.description,
			COALESCE(s.name, ''), COALESCE(sc.color, '#6b7280'),
			i.created_at,
			w.name AS workspace_name,
			w.key AS workspace_key
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.creator_portal_customer_id = ?
		ORDER BY i.created_at DESC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("list portal customer submissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []PortalCustomerSubmission
	for rows.Next() {
		var s PortalCustomerSubmission
		if err := rows.Scan(
			&s.ID, &s.WorkspaceID, &s.Title, &s.Description,
			&s.StatusName, &s.StatusColor, &s.CreatedAt,
			&s.WorkspaceName, &s.WorkspaceKey,
		); err != nil {
			return nil, fmt.Errorf("scan portal customer submission: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// OrganisationTicket is one row returned by ListOrganisationTickets — a
// ticket raised by any contact of a customer organisation, with workspace,
// status, and creator-contact metadata.
type OrganisationTicket struct {
	ID                  int
	WorkspaceID         int
	WorkspaceItemNumber int
	Title               string
	CreatedAt           string
	WorkspaceName       string
	WorkspaceKey        string
	StatusName          string
	StatusColor         string
	CreatorContactName  string
	CreatorContactEmail string
}

// ListOrganisationTickets returns all tickets raised by portal customers
// belonging to the given customer_organisation_id, restricted to the given
// workspace IDs. Returns an empty slice if workspaceIDs is empty.
func (r *ItemRepository) ListOrganisationTickets(orgID int, workspaceIDs []int) ([]OrganisationTicket, error) {
	if len(workspaceIDs) == 0 {
		return []OrganisationTicket{}, nil
	}
	placeholders := make([]string, len(workspaceIDs))
	args := make([]interface{}, 0, len(workspaceIDs)+1)
	args = append(args, orgID)
	for i, id := range workspaceIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title, i.created_at,
		       w.name, w.key,
		       COALESCE(s.name, ''), COALESCE(sc.color, '#6b7280'),
		       pc.name, pc.email
		FROM items i
		JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE pc.customer_organisation_id = ?
		  AND i.workspace_id IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY i.created_at DESC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list organisation tickets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []OrganisationTicket
	for rows.Next() {
		var t OrganisationTicket
		if err := rows.Scan(
			&t.ID, &t.WorkspaceID, &t.WorkspaceItemNumber, &t.Title, &t.CreatedAt,
			&t.WorkspaceName, &t.WorkspaceKey,
			&t.StatusName, &t.StatusColor,
			&t.CreatorContactName, &t.CreatorContactEmail,
		); err != nil {
			return nil, fmt.Errorf("scan organisation ticket: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- Configuration-set migration aggregations ------------------------------

// ItemStatusCount is one row of a (status_id, status_name, item_count)
// aggregation. Rows with no status yield status_id=0, status_name="" because
// the underlying queries use COALESCE.
type ItemStatusCount struct {
	StatusID   int
	StatusName string
	ItemCount  int
}

// ItemTypeStatusCount is one row of a
// (item_type_id, item_type_name, status_id, status_name, count) aggregation
// used when migration analysis differentiates by item type. ItemTypeID is nil
// when the item has no item_type_id; StatusID is 0 and StatusName is "" when
// the item has no status.
type ItemTypeStatusCount struct {
	ItemTypeID   *int
	ItemTypeName string
	StatusID     int
	StatusName   string
	ItemCount    int
}

// ListStatusCountsForWorkspaces groups items in the given workspaces by
// status_id / status_name, returning COUNT(*) per group. Used by migration
// analyzers to enumerate the statuses in use. Returns an empty slice if
// workspaceIDs is empty.
func (r *ItemRepository) ListStatusCountsForWorkspaces(workspaceIDs []int) ([]ItemStatusCount, error) {
	if len(workspaceIDs) == 0 {
		return []ItemStatusCount{}, nil
	}
	placeholders := make([]string, len(workspaceIDs))
	args := make([]interface{}, len(workspaceIDs))
	for i, id := range workspaceIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `
		SELECT COALESCE(s.id, 0) as status_id, COALESCE(s.name, '') as status_name, COUNT(*) as item_count
		FROM items i
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE i.workspace_id IN (` + strings.Join(placeholders, ",") + `)
		GROUP BY s.id, s.name
		ORDER BY s.name`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list status counts for workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ItemStatusCount
	for rows.Next() {
		var c ItemStatusCount
		if err := rows.Scan(&c.StatusID, &c.StatusName, &c.ItemCount); err != nil {
			return nil, fmt.Errorf("scan status count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListItemTypeStatusCountsForWorkspaces groups items in the given workspaces
// by (item_type_id, status_id), returning COUNT(*) per group. Used by the
// per-item-type migration analyzer.
func (r *ItemRepository) ListItemTypeStatusCountsForWorkspaces(workspaceIDs []int) ([]ItemTypeStatusCount, error) {
	if len(workspaceIDs) == 0 {
		return []ItemTypeStatusCount{}, nil
	}
	placeholders := make([]string, len(workspaceIDs))
	args := make([]interface{}, len(workspaceIDs))
	for i, id := range workspaceIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `
		SELECT i.item_type_id, COALESCE(it.name, '') as item_type_name,
		       COALESCE(s.id, 0) as status_id, COALESCE(s.name, '') as status_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE i.workspace_id IN (` + strings.Join(placeholders, ",") + `)
		GROUP BY i.item_type_id, it.name, s.id, s.name
		ORDER BY it.name, s.name`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list item-type/status counts for workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ItemTypeStatusCount
	for rows.Next() {
		var c ItemTypeStatusCount
		var itemTypeID sql.NullInt64
		if err := rows.Scan(&itemTypeID, &c.ItemTypeName, &c.StatusID, &c.StatusName, &c.ItemCount); err != nil {
			return nil, fmt.Errorf("scan item-type/status count: %w", err)
		}
		if itemTypeID.Valid {
			v := int(itemTypeID.Int64)
			c.ItemTypeID = &v
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ItemTypeCount is one row of (item_type_id, item_type_name, count). TypeID=0
// and TypeName="(No Type)" for items with no item_type_id, courtesy of COALESCE.
type ItemTypeCount struct {
	TypeID    int
	TypeName  string
	ItemCount int
}

// ListItemTypeCountsForWorkspace groups a single workspace's items by
// item_type_id, returning COUNT(*) per group. Used by the item-type migration
// analyzer.
func (r *ItemRepository) ListItemTypeCountsForWorkspace(workspaceID int) ([]ItemTypeCount, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(i.item_type_id, 0) as type_id,
		       COALESCE(it.name, '(No Type)') as type_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.workspace_id = ?
		GROUP BY i.item_type_id, it.name
		ORDER BY it.name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list item-type counts for workspace: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ItemTypeCount
	for rows.Next() {
		var c ItemTypeCount
		if err := rows.Scan(&c.TypeID, &c.TypeName, &c.ItemCount); err != nil {
			return nil, fmt.Errorf("scan item-type count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PriorityCount is one row of (priority_id, priority_name, count).
// PriorityID=0 and PriorityName="(No Priority)" for items with no priority_id.
type PriorityCount struct {
	PriorityID   int
	PriorityName string
	ItemCount    int
}

// ListPriorityCountsForWorkspace groups a single workspace's items by
// priority_id, returning COUNT(*) per group.
func (r *ItemRepository) ListPriorityCountsForWorkspace(workspaceID int) ([]PriorityCount, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(i.priority_id, 0) as priority_id,
		       COALESCE(p.name, '(No Priority)') as priority_name,
		       COUNT(*) as item_count
		FROM items i
		LEFT JOIN priorities p ON i.priority_id = p.id
		WHERE i.workspace_id = ?
		GROUP BY i.priority_id, p.name
		ORDER BY p.name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list priority counts for workspace: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PriorityCount
	for rows.Next() {
		var c PriorityCount
		if err := rows.Scan(&c.PriorityID, &c.PriorityName, &c.ItemCount); err != nil {
			return nil, fmt.Errorf("scan priority count: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ListNonEmptyCustomFieldJSONForWorkspace returns the raw custom_field_values
// JSON strings for every item in the given workspace whose value is non-NULL,
// non-empty, and not the literal "{}". Used by the custom-field migration
// analyzer to count how many items reference each field.
func (r *ItemRepository) ListNonEmptyCustomFieldJSONForWorkspace(workspaceID int) ([]string, error) {
	rows, err := r.db.Query(`
		SELECT custom_field_values FROM items
		WHERE workspace_id = ?
		  AND custom_field_values IS NOT NULL
		  AND custom_field_values != ''
		  AND custom_field_values != '{}'
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list custom field JSON: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var cfvJSON string
		if err := rows.Scan(&cfvJSON); err != nil {
			return nil, fmt.Errorf("scan custom field JSON: %w", err)
		}
		out = append(out, cfvJSON)
	}
	return out, rows.Err()
}
