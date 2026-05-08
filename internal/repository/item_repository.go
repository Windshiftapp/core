package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
	var itemTypeID, parentID, statusID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID, creatorPortalCustomerID, relatedWorkItemID sql.NullInt64
	var dueDate, startDate, endDate sql.NullTime
	var storyPoints sql.NullFloat64

	err := r.db.QueryRow(`
		SELECT id, workspace_id, workspace_item_number, item_type_id, title, description, status_id,
		       priority_id, due_date, start_date, end_date, is_task, iteration_id, project_id, inherit_project,
		       assignee_id, creator_id, creator_portal_customer_id, custom_field_values, parent_id, related_work_item_id,
		       story_points, frac_index, created_at, updated_at
		FROM items WHERE id = ?
	`, id).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &iterationID,
		&projectID, &item.InheritProject, &assigneeID, &creatorID, &creatorPortalCustomerID, &customFieldValuesJSON, &parentID,
		&relatedWorkItemID, &storyPoints, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
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
	assignNullableInt(&item.IterationID, iterationID)
	assignNullableInt(&item.ProjectID, projectID)
	assignNullableInt(&item.AssigneeID, assigneeID)
	assignNullableInt(&item.CreatorID, creatorID)
	assignNullableInt(&item.CreatorPortalCustomerID, creatorPortalCustomerID)
	assignNullableInt(&item.RelatedWorkItemID, relatedWorkItemID)

	assignNullableTime(&item.DueDate, dueDate)
	assignNullableTime(&item.StartDate, startDate)
	assignNullableTime(&item.EndDate, endDate)
	assignNullableFloat64(&item.StoryPoints, storyPoints)

	item.CustomFieldValues = parseCustomFieldsJSON(customFieldValuesJSON)

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
	var itemTypeID, parentID, statusID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID, timeProjectID sql.NullInt64
	var dueDate, startDate, endDate sql.NullTime
	var workspaceActive bool

	// Joined data
	var projectName, iterationName, timeProjectName, parentTitle sql.NullString
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
		       i.status_id, i.priority_id, i.due_date, i.start_date, i.end_date, i.is_task, i.iteration_id,
		       i.project_id, i.inherit_project, i.time_project_id, i.assignee_id, i.creator_id, i.custom_field_values,
		       i.parent_id, i.story_points, i.frac_index, i.created_at, i.updated_at,
		       i.creator_portal_customer_id, i.channel_id, i.request_type_id,
		       w.name as workspace_name, w.key as workspace_key, w.active as workspace_active,
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
		&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &iterationID,
		&projectID, &item.InheritProject, &timeProjectID, &assigneeID, &creatorID, &customFieldValuesJSON,
		&parentID, &storyPoints, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
		&creatorPortalCustomerID, &channelID, &requestTypeID,
		&item.WorkspaceName, &item.WorkspaceKey, &workspaceActive,
		&iterationName, &projectName, &timeProjectName, &parentTitle,
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

	if errors.Is(err, sql.ErrNoRows) {
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
	assignNullableInt(&item.IterationID, iterationID)
	assignNullableInt(&item.ProjectID, projectID)
	assignNullableInt(&item.TimeProjectID, timeProjectID)
	assignNullableInt(&item.AssigneeID, assigneeID)
	assignNullableInt(&item.CreatorID, creatorID)

	// Portal-specific fields
	assignNullableInt(&item.CreatorPortalCustomerID, creatorPortalCustomerID)
	assignNullableInt(&item.ChannelID, channelID)
	assignNullableInt(&item.RequestTypeID, requestTypeID)

	assignNullableTime(&item.DueDate, dueDate)
	assignNullableTime(&item.StartDate, startDate)
	assignNullableTime(&item.EndDate, endDate)
	assignNullableFloat64(&item.StoryPoints, storyPoints)

	// Handle nullable string fields from joins
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

	item.CustomFieldValues = parseCustomFieldsJSON(customFieldValuesJSON)

	// Eager-load milestones so callers (REST mappers, ai tools, etc.) don't
	// each have to remember to attach them after the fact.
	holder := []models.Item{item}
	if err := NewMilestoneAttachRepository(r.db).LoadForItems(holder); err == nil {
		item = holder[0]
	}

	return &ItemWithWorkspaceStatus{Item: &item, WorkspaceActive: workspaceActive}, nil
}

// GetWorkspaceID returns just the workspace_id for an item (frequently needed for permission checks)
func (r *ItemRepository) GetWorkspaceID(itemID int) (int, error) {
	var workspaceID int
	err := r.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
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
	placeholders, args := inPlaceholders(itemIDs)
	rows, err := r.db.Query(
		`SELECT id, title FROM items WHERE id IN (`+placeholders+`)`,
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
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
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
			priority_id, due_date, start_date, end_date, is_task, iteration_id, project_id, inherit_project,
			assignee_id, creator_id, custom_field_values, parent_id, related_work_item_id,
			story_points, frac_index, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		item.WorkspaceID, item.WorkspaceItemNumber, item.ItemTypeID, item.Title, item.Description,
		item.StatusID, item.PriorityID, item.DueDate, item.StartDate, item.EndDate, item.IsTask,
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
		    due_date = ?, start_date = ?, end_date = ?, iteration_id = ?, project_id = ?, inherit_project = ?,
		    assignee_id = ?, creator_id = ?, custom_field_values = ?, parent_id = ?,
		    related_work_item_id = ?, story_points = ?, updated_at = ?
		WHERE id = ?
	`,
		item.WorkspaceID, item.Title, item.Description, item.StatusID, item.PriorityID,
		item.DueDate, item.StartDate, item.EndDate, item.IterationID, item.ProjectID, item.InheritProject,
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
	"iteration_id": true, "project_id": true, "inherit_project": true,
	"assignee_id": true, "creator_id": true, "custom_field_values": true,
	"parent_id": true, "related_work_item_id": true, "item_type_id": true,
	"frac_index": true, "is_task": true, "time_project_id": true,
	"story_points": true,
}

// IsAllowedItemColumn reports whether col names a column on the items table
// that is safe to interpolate into a query (used by callers that must build
// dynamic SELECT/UPDATE statements, e.g., action nodes).
func IsAllowedItemColumn(col string) bool {
	return allowedItemColumns[col]
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

// GetItemCustomFieldValue reads the current value for a single custom field
// out of an item's custom_field_values JSON blob. Returns nil if the item has
// no entry for that field. The value is returned as decoded from JSON
// (string/float64/bool/map/slice).
func (r *ItemRepository) GetItemCustomFieldValue(itemID, customFieldID int) (interface{}, error) {
	var raw sql.NullString
	if err := r.db.QueryRow(`SELECT custom_field_values FROM items WHERE id = ?`, itemID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("load item custom_field_values: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var values map[string]interface{}
	if err := json.Unmarshal([]byte(raw.String), &values); err != nil {
		return nil, nil //nolint:nilerr // treat malformed blob as "no value present"
	}
	return values[strconv.Itoa(customFieldID)], nil
}

// SetItemCustomFieldValue sets a single custom field entry inside an item's
// custom_field_values JSON blob, preserving other entries. The caller is
// responsible for validating that customFieldID refers to an existing field;
// this method verifies existence to fail fast for obviously bogus IDs.
func (r *ItemRepository) SetItemCustomFieldValue(tx database.Tx, itemID, customFieldID int, value interface{}) error {
	var exists int
	if err := tx.QueryRow(`SELECT 1 FROM custom_field_definitions WHERE id = ?`, customFieldID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("custom field %d not found", customFieldID)
		}
		return fmt.Errorf("check custom field: %w", err)
	}

	var raw sql.NullString
	if err := tx.QueryRow(`SELECT custom_field_values FROM items WHERE id = ?`, itemID).Scan(&raw); err != nil {
		return fmt.Errorf("load item custom_field_values: %w", err)
	}

	values := make(map[string]interface{})
	if raw.Valid && raw.String != "" {
		_ = json.Unmarshal([]byte(raw.String), &values)
	}
	values[strconv.Itoa(customFieldID)] = value

	updated, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal custom_field_values: %w", err)
	}

	if _, err := tx.Exec(`UPDATE items SET custom_field_values = ?, updated_at = ? WHERE id = ?`, string(updated), time.Now(), itemID); err != nil {
		return fmt.Errorf("update item custom_field_values: %w", err)
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
	if errors.Is(err, sql.ErrNoRows) {
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

// GetParentIDTx returns the parent_id for an item using the supplied
// transaction, locking the row with FOR UPDATE on Postgres so a concurrent
// writer cannot change the parent between the cycle check and the subsequent
// update in the same transaction.
func (r *ItemRepository) GetParentIDTx(tx database.Tx, itemID int) (*int, error) {
	query := `SELECT parent_id FROM items WHERE id = ?`
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	var parentID sql.NullInt64
	err := tx.QueryRow(query, itemID).Scan(&parentID)
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
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
	if errors.Is(err, sql.ErrNoRows) {
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
	placeholders, args := inPlaceholders(workspaceIDs)
	query := fmt.Sprintf(`
		SELECT id, COALESCE(custom_field_values, '{}') as cfv
		FROM items
		WHERE workspace_id IN (%s)
	`, placeholders)

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
	if errors.Is(err, sql.ErrNoRows) {
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
	assignNullableFloat64(&p.StoryPoints, storyPoints)
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
	placeholders, args := inPlaceholders(keys)
	query := `SELECT w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.id, i.workspace_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE w.key || '-' || CAST(i.workspace_item_number AS TEXT) IN (` + placeholders + `)`
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
	placeholders, args := inPlaceholders(workspaceIDs)
	query := fmt.Sprintf(`
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
		       i.status_id, i.priority_id, i.assignee_id, i.creator_id,
		       i.calendar_data, i.due_date, i.created_at, i.updated_at,
		       w.name, w.key, w.is_personal
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.calendar_data IS NOT NULL AND i.calendar_data != ''
		  AND i.workspace_id IN (%s)
	`, placeholders)

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
		assignNullableTime(&item.DueDate, dueDate)

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
// activity widget needs. MilestoneIDs is the full set of milestones the item
// belongs to, so the caller can aggregate by milestone without a second round
// trip.
type HomepageItemSummary struct {
	ItemID              int
	WorkspaceID         int
	WorkspaceItemNumber int
	Title               string
	Status              string
	StatusColor         *string
	PriorityID          *int
	PriorityName        *string
	PriorityColor       *string
	WorkspaceKey        string
	MilestoneIDs        []int
}

// ListHomepageItemSummaries returns the homepage widget's item summaries for
// the given item IDs. Missing IDs are silently omitted. The result order is
// not guaranteed — callers index by ItemID.
func (r *ItemRepository) ListHomepageItemSummaries(itemIDs []int) ([]HomepageItemSummary, error) {
	if len(itemIDs) == 0 {
		return []HomepageItemSummary{}, nil
	}
	placeholders, args := inPlaceholders(itemIDs)
	query := `
		SELECT i.id, i.workspace_id, i.workspace_item_number, i.title,
		       COALESCE(s.name, 'Unknown') as status,
		       sc.color as status_color,
		       i.priority_id, p.name as priority_name, p.color as priority_color,
		       w.key as workspace_key
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		WHERE i.id IN (` + placeholders + `)`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list homepage items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []HomepageItemSummary{}
	resultIdx := map[int]int{}
	for rows.Next() {
		var s HomepageItemSummary
		var priorityID sql.NullInt64
		var statusColor, priorityName, priorityColor sql.NullString
		if err := rows.Scan(
			&s.ItemID, &s.WorkspaceID, &s.WorkspaceItemNumber, &s.Title,
			&s.Status, &statusColor,
			&priorityID, &priorityName, &priorityColor,
			&s.WorkspaceKey,
		); err != nil {
			return nil, fmt.Errorf("scan homepage item: %w", err)
		}
		if statusColor.Valid {
			v := statusColor.String
			s.StatusColor = &v
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
		resultIdx[s.ItemID] = len(results)
		results = append(results, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(results) > 0 {
		msPlaceholders, msArgs := inPlaceholders(itemIDs)
		msRows, err := r.db.Query(`
			SELECT item_id, milestone_id
			FROM item_milestones
			WHERE item_id IN (`+msPlaceholders+`)
			ORDER BY milestone_id
		`, msArgs...)
		if err != nil {
			return nil, fmt.Errorf("list homepage item milestones: %w", err)
		}
		defer func() { _ = msRows.Close() }()
		for msRows.Next() {
			var itemID, mID int
			if err := msRows.Scan(&itemID, &mID); err != nil {
				return nil, fmt.Errorf("scan homepage item milestone: %w", err)
			}
			if idx, ok := resultIdx[itemID]; ok {
				results[idx].MilestoneIDs = append(results[idx].MilestoneIDs, mID)
			}
		}
		if err := msRows.Err(); err != nil {
			return nil, err
		}
	}

	return results, nil
}

// TopMilestoneIDsForItems returns the `limit` most frequently referenced
// milestone_id values among the given item IDs, ordered by frequency desc,
// then ascending milestone_id for stability. With multi-milestone, every
// (item, milestone) row in item_milestones counts once.
func (r *ItemRepository) TopMilestoneIDsForItems(itemIDs []int, limit int) ([]int, error) {
	if len(itemIDs) == 0 || limit <= 0 {
		return []int{}, nil
	}
	placeholders, args := inPlaceholders(itemIDs)
	query := `
		SELECT milestone_id, COUNT(*) as freq
		FROM item_milestones
		WHERE item_id IN (` + placeholders + `)
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

// HomepageMilestoneProgress is the per-milestone progress row returned by
// HomepageMilestoneProgressByIDs. TargetDate and CategoryColor may be empty
// when the milestone lacks a target date or category.
type HomepageMilestoneProgress struct {
	MilestoneID   int
	MilestoneName string
	TargetDate    *string
	CategoryColor string
	TotalItems    int
	DoneItems     int
}

// HomepageMilestoneProgressByIDs returns per-milestone completion stats for
// the given milestone IDs, joining items/statuses/status_categories to derive
// done counts. Missing IDs are silently omitted; results are ordered by
// milestone ID ascending.
func (r *ItemRepository) HomepageMilestoneProgressByIDs(milestoneIDs []int) ([]HomepageMilestoneProgress, error) {
	if len(milestoneIDs) == 0 {
		return []HomepageMilestoneProgress{}, nil
	}
	placeholders, args := inPlaceholders(milestoneIDs)
	query := `
		SELECT
			m.id,
			m.name,
			m.target_date,
			mc.color,
			COUNT(i.id) as total_items,
			SUM(CASE WHEN COALESCE(sc.is_completed, FALSE) = TRUE THEN 1 ELSE 0 END) as done_items
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN item_milestones im ON im.milestone_id = m.id
		LEFT JOIN items i ON i.id = im.item_id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE m.id IN (` + placeholders + `)
		GROUP BY m.id, m.name, m.target_date, mc.color
		ORDER BY m.id`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query milestone progress: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []HomepageMilestoneProgress{}
	for rows.Next() {
		var progress HomepageMilestoneProgress
		var targetDate, categoryColor sql.NullString
		var doneItems sql.NullInt64
		if err := rows.Scan(
			&progress.MilestoneID,
			&progress.MilestoneName,
			&targetDate,
			&categoryColor,
			&progress.TotalItems,
			&doneItems,
		); err != nil {
			return nil, fmt.Errorf("scan milestone progress: %w", err)
		}
		if targetDate.Valid {
			v := targetDate.String
			progress.TargetDate = &v
		}
		if categoryColor.Valid {
			progress.CategoryColor = categoryColor.String
		}
		if doneItems.Valid {
			progress.DoneItems = int(doneItems.Int64)
		}
		results = append(results, progress)
	}
	return results, rows.Err()
}
