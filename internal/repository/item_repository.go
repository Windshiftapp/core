package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/models"
)

type ItemRepository struct {
	db database.Database
}

func NewItemRepository(db database.Database) *ItemRepository {
	return &ItemRepository{db: db}
}

// GetDetailPanelAvailability reports whether the item detail surface has SCM
// context or agent runs to show. Keeping this query in the repository preserves
// the handler boundary while allowing both flags to share one database round trip.
func (r *ItemRepository) GetDetailPanelAvailability(workspaceID, itemID int) (scmAvailable, hasAgentRuns bool, err error) {
	err = r.db.QueryRow(`
		SELECT
			EXISTS(
				SELECT 1 FROM workspace_repositories wr
				JOIN workspace_scm_connections wsc ON wsc.id = wr.workspace_scm_connection_id
				WHERE wsc.workspace_id = ? AND wr.is_active = true AND wsc.enabled = true
			),
			EXISTS(SELECT 1 FROM agent_runs WHERE item_id = ?)
	`, workspaceID, itemID).Scan(&scmAvailable, &hasAgentRuns)
	return scmAvailable, hasAgentRuns, err
}

const itemBaseColumns = `id, workspace_id, workspace_item_number, item_type_id, title, description, status_id,
       priority_id, due_date, start_date, end_date, is_task, iteration_id, project_id, inherit_project,
       time_project_id, assignee_id, creator_id, creator_portal_customer_id, custom_field_values, parent_id, related_work_item_id,
       story_points, estimate_minutes, frac_index, created_at, updated_at`

func scanItemBase(scanner interface {
	Scan(dest ...any) error
}) (*models.Item, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var itemTypeID, parentID, statusID, iterationID, projectID, priorityID sql.NullInt64
	var timeProjectID, assigneeID, creatorID, creatorPortalCustomerID, relatedWorkItemID sql.NullInt64
	var dueDate, startDate, endDate sql.NullTime
	var storyPoints sql.NullFloat64
	var estimateMinutes sql.NullInt64
	var fracIndex sql.NullString

	err := scanner.Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &iterationID,
		&projectID, &item.InheritProject, &timeProjectID, &assigneeID, &creatorID, &creatorPortalCustomerID, &customFieldValuesJSON, &parentID,
		&relatedWorkItemID, &storyPoints, &estimateMinutes, &fracIndex, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	assignNullableInt(&item.ItemTypeID, itemTypeID)
	assignNullableInt(&item.ParentID, parentID)
	assignNullableInt(&item.StatusID, statusID)
	assignNullableInt(&item.PriorityID, priorityID)
	assignNullableInt(&item.IterationID, iterationID)
	assignNullableInt(&item.ProjectID, projectID)
	assignNullableInt(&item.TimeProjectID, timeProjectID)
	assignNullableInt(&item.AssigneeID, assigneeID)
	assignNullableInt(&item.CreatorID, creatorID)
	assignNullableInt(&item.CreatorPortalCustomerID, creatorPortalCustomerID)
	assignNullableInt(&item.RelatedWorkItemID, relatedWorkItemID)
	assignNullableTime(&item.DueDate, dueDate)
	assignNullableTime(&item.StartDate, startDate)
	assignNullableTime(&item.EndDate, endDate)
	assignNullableFloat64(&item.StoryPoints, storyPoints)
	assignNullableInt(&item.EstimateMinutes, estimateMinutes)
	assignNullableStringPtr(&item.FracIndex, fracIndex)
	item.CustomFieldValues = parseCustomFieldsJSON(customFieldValuesJSON)

	return &item, nil
}

func assignNullableStringPtr(dest **string, src sql.NullString) {
	if src.Valid {
		val := src.String
		*dest = &val
	}
}

// mapItemErr keeps single-item accessors consistent: missing rows become
// ErrNotFound and other errors are wrapped with the operation name.
func mapItemErr(err error, op string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("failed to %s: %w", op, err)
}

// scanItemColumn reads one allowlisted item column and maps missing rows to
// ErrNotFound. column and op come only from typed accessors.
func (r *ItemRepository) scanItemColumn(itemID int, column, op string, dest any) error {
	return mapItemErr(r.db.QueryRow("SELECT "+column+" FROM items WHERE id = ?", itemID).Scan(dest), op)
}

func (r *ItemRepository) FindByID(id int) (*models.Item, error) {
	return r.FindByIDContext(context.Background(), id)
}

func (r *ItemRepository) FindByIDContext(ctx context.Context, id int) (*models.Item, error) {
	item, err := scanItemBase(r.db.QueryRowContext(ctx, `SELECT `+itemBaseColumns+` FROM items WHERE id = ?`, id))
	if err != nil {
		return nil, mapItemErr(err, "find item")
	}
	return item, nil
}

// ItemCaptureSnapshot is the stable item projection used by the Jira capture
// export verifier.
type ItemCaptureSnapshot struct {
	Title, Description, StatusName, ItemTypeName, PriorityName string
	AssigneeUsername, ReporterUsername, CreatorUsername        string
	StoryPoints                                                *float64
	DueDate, CreatedAt, UpdatedAt, CustomFieldValues           string
}

// GetCaptureSnapshot returns the item fields required by capture verification.
func (r *ItemRepository) GetCaptureSnapshot(itemID int) (*ItemCaptureSnapshot, error) {
	var out ItemCaptureSnapshot
	var description, statusName, itemTypeName, priorityName sql.NullString
	var assignee, reporter, creator, dueDate, createdAt, updatedAt, customFields sql.NullString
	err := r.db.QueryRow(`
		SELECT i.title, i.description, s.name, t.name, p.name,
		       ua.username, ur.username, uc.username, i.story_points,
		       CAST(i.due_date AS TEXT), CAST(i.created_at AS TEXT),
		       CAST(i.updated_at AS TEXT), CAST(i.custom_field_values AS TEXT)
		FROM items i
		LEFT JOIN statuses s ON s.id = i.status_id
		LEFT JOIN item_types t ON t.id = i.item_type_id
		LEFT JOIN priorities p ON p.id = i.priority_id
		LEFT JOIN users ua ON ua.id = i.assignee_id
		LEFT JOIN users ur ON ur.id = i.reporter_id
		LEFT JOIN users uc ON uc.id = i.creator_id
		WHERE i.id = ?
	`, itemID).Scan(
		&out.Title, &description, &statusName, &itemTypeName, &priorityName,
		&assignee, &reporter, &creator, &out.StoryPoints,
		&dueDate, &createdAt, &updatedAt, &customFields,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get item capture snapshot: %w", err)
	}
	out.Description = description.String
	out.StatusName = statusName.String
	out.ItemTypeName = itemTypeName.String
	out.PriorityName = priorityName.String
	out.AssigneeUsername = assignee.String
	out.ReporterUsername = reporter.String
	out.CreatorUsername = creator.String
	out.DueDate = dueDate.String
	out.CreatedAt = createdAt.String
	out.UpdatedAt = updatedAt.String
	out.CustomFieldValues = customFields.String
	return &out, nil
}

// FindByIDForUpdate locks the row on Postgres; SQLite uses a plain select.
func (r *ItemRepository) FindByIDForUpdate(tx database.Tx, id int) (*models.Item, error) {
	query := `SELECT ` + itemBaseColumns + ` FROM items WHERE id = ?`
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	item, err := scanItemBase(tx.QueryRow(query, id))
	if err != nil {
		return nil, mapItemErr(err, "find item for update")
	}
	return item, nil
}

// FindByIDsForUpdateContext locks a bounded set in stable ID order. Missing
// IDs are omitted so callers can fail the operation without exposing which one.
func (r *ItemRepository) FindByIDsForUpdateContext(ctx context.Context, tx database.Tx, ids []int) ([]*models.Item, error) {
	if len(ids) == 0 {
		return []*models.Item{}, nil
	}
	placeholders, args := inPlaceholders(ids)
	query := `SELECT ` + itemBaseColumns + ` FROM items WHERE id IN (` + placeholders + `) ORDER BY id`
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("find items for bulk update: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]*models.Item, 0, len(ids))
	for rows.Next() {
		item, scanErr := scanItemBase(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan item for bulk update: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate items for bulk update: %w", err)
	}
	return items, nil
}

// LockWorkspaceItemsTx prevents new foreign-key references from being added to
// the workspace's current items while a destructive workspace operation runs.
// The caller must lock the workspace row first so concurrent item creation is
// fenced as well.
func (r *ItemRepository) LockWorkspaceItemsTx(tx database.Tx, workspaceID int) error {
	query := "SELECT id FROM items WHERE workspace_id = ? ORDER BY id"
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	rows, err := tx.Query(query, workspaceID)
	if err != nil {
		return fmt.Errorf("lock workspace items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var itemID int
		if err := rows.Scan(&itemID); err != nil {
			return fmt.Errorf("scan locked workspace item: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate locked workspace items: %w", err)
	}
	return nil
}

type ItemWithWorkspaceStatus struct {
	*models.Item
	WorkspaceActive bool
}

func (r *ItemRepository) FindByIDWithDetails(id int) (*models.Item, error) {
	return r.FindByIDWithDetailsContext(context.Background(), id)
}

func (r *ItemRepository) FindByIDWithDetailsContext(ctx context.Context, id int) (*models.Item, error) {
	result, err := r.FindByIDWithWorkspaceStatusContext(ctx, id)
	if err != nil {
		return nil, err
	}
	return result.Item, nil
}

// itemDetailsSelectBody is the shared joined projection for single and batch
// loaders. Callers append the WHERE clause and use scanItemDetailsRow.
const itemDetailsSelectBody = `
	SELECT i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
	       i.status_id, i.priority_id, i.due_date, i.start_date, i.end_date, i.is_task, i.iteration_id,
	       i.project_id, i.inherit_project, i.time_project_id, i.assignee_id, i.creator_id, i.custom_field_values,
	       i.virtual_field_data,
	       i.parent_id, i.story_points, i.estimate_minutes, i.frac_index, i.created_at, i.updated_at,
	       i.creator_portal_customer_id, i.channel_id, i.request_type_id,
	       w.name as workspace_name, w.key as workspace_key, w.active as workspace_active,
	       iter.name as iteration_name,
	       proj.name as project_name,
	       tp.name as time_project_name,
	       p.title as parent_title,
	       p.workspace_item_number as parent_workspace_item_number,
	       assignee.first_name || ' ' || assignee.last_name as assignee_name, assignee.email as assignee_email, assignee.avatar_url as assignee_avatar,
	       creator.first_name || ' ' || creator.last_name as creator_name, creator.email as creator_email,
	       pri.name as priority_name, COALESCE(pri.builtin_key, '') as priority_builtin_key, pri.icon as priority_icon, pri.color as priority_color,
	       s.name as status_name, COALESCE(s.builtin_key, '') as status_builtin_key,
	       it.name as item_type_name, COALESCE(it.builtin_key, '') as item_type_builtin_key,
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
	LEFT JOIN workspaces rw_ws ON rw.workspace_id = rw_ws.id`

// scanItemDetailsRow scans the shared projection. Milestones are attached by
// the caller to avoid a per-row query.
func scanItemDetailsRow(scanner rowScanner) (models.Item, bool, error) {
	var item models.Item
	var customFieldValuesJSON sql.NullString
	var virtualFieldDataJSON sql.NullString
	var itemTypeID, parentID, statusID, iterationID, projectID, priorityID sql.NullInt64
	var assigneeID, creatorID, timeProjectID sql.NullInt64
	var dueDate, startDate, endDate sql.NullTime
	var workspaceActive bool

	var projectName, iterationName, timeProjectName, parentTitle sql.NullString
	var parentWorkspaceItemNumber sql.NullInt64
	var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail sql.NullString
	var priorityName, priorityBuiltinKey, priorityIcon, priorityColor sql.NullString
	var statusName, statusBuiltinKey sql.NullString
	var itemTypeName, itemTypeBuiltinKey sql.NullString
	var relatedWorkItemID sql.NullInt64
	var relatedWorkItemTitle, relatedWorkItemWorkspaceKey sql.NullString
	var relatedWorkItemWorkspaceID, relatedWorkItemNumber sql.NullInt64
	var creatorPortalCustomerID, channelID, requestTypeID sql.NullInt64

	var storyPoints sql.NullFloat64
	var estimateMinutes sql.NullInt64

	err := scanner.Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
		&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &iterationID,
		&projectID, &item.InheritProject, &timeProjectID, &assigneeID, &creatorID, &customFieldValuesJSON,
		&virtualFieldDataJSON,
		&parentID, &storyPoints, &estimateMinutes, &item.FracIndex, &item.CreatedAt, &item.UpdatedAt,
		&creatorPortalCustomerID, &channelID, &requestTypeID,
		&item.WorkspaceName, &item.WorkspaceKey, &workspaceActive,
		&iterationName, &projectName, &timeProjectName, &parentTitle, &parentWorkspaceItemNumber,
		&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail,
		&priorityName, &priorityBuiltinKey, &priorityIcon, &priorityColor,
		&statusName, &statusBuiltinKey,
		&itemTypeName, &itemTypeBuiltinKey,
		&relatedWorkItemID,
		&relatedWorkItemTitle,
		&relatedWorkItemWorkspaceKey,
		&relatedWorkItemWorkspaceID,
		&relatedWorkItemNumber,
	)
	if err != nil {
		return models.Item{}, false, err
	}

	assignNullableInt(&item.ItemTypeID, itemTypeID)
	assignNullableInt(&item.ParentID, parentID)
	assignNullableInt(&item.StatusID, statusID)
	assignNullableInt(&item.PriorityID, priorityID)
	assignNullableInt(&item.IterationID, iterationID)
	assignNullableInt(&item.ProjectID, projectID)
	assignNullableInt(&item.TimeProjectID, timeProjectID)
	assignNullableInt(&item.AssigneeID, assigneeID)
	assignNullableInt(&item.CreatorID, creatorID)

	assignNullableInt(&item.CreatorPortalCustomerID, creatorPortalCustomerID)
	assignNullableInt(&item.ChannelID, channelID)
	assignNullableInt(&item.RequestTypeID, requestTypeID)

	assignNullableTime(&item.DueDate, dueDate)
	assignNullableTime(&item.StartDate, startDate)
	assignNullableTime(&item.EndDate, endDate)
	assignNullableFloat64(&item.StoryPoints, storyPoints)
	assignNullableInt(&item.EstimateMinutes, estimateMinutes)

	assignNullableString(&item.IterationName, iterationName)
	assignNullableString(&item.ProjectName, projectName)
	assignNullableString(&item.TimeProjectName, timeProjectName)
	assignNullableString(&item.ParentTitle, parentTitle)
	assignNullableInt(&item.ParentWorkspaceItemNumber, parentWorkspaceItemNumber)
	assignNullableString(&item.AssigneeName, assigneeName)
	assignNullableString(&item.AssigneeEmail, assigneeEmail)
	assignNullableString(&item.AssigneeAvatar, assigneeAvatar)
	assignNullableString(&item.CreatorName, creatorName)
	assignNullableString(&item.CreatorEmail, creatorEmail)
	assignNullableString(&item.PriorityName, priorityName)
	assignNullableString(&item.PriorityBuiltinKey, priorityBuiltinKey)
	assignNullableString(&item.PriorityIcon, priorityIcon)
	assignNullableString(&item.PriorityColor, priorityColor)
	assignNullableString(&item.StatusName, statusName)
	assignNullableString(&item.StatusBuiltinKey, statusBuiltinKey)
	assignNullableString(&item.ItemTypeName, itemTypeName)
	assignNullableString(&item.ItemTypeBuiltinKey, itemTypeBuiltinKey)

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
	item.VirtualFieldData = parseCustomFieldsJSON(virtualFieldDataJSON)

	return item, workspaceActive, nil
}

func (r *ItemRepository) FindByIDWithWorkspaceStatus(id int) (*ItemWithWorkspaceStatus, error) {
	return r.FindByIDWithWorkspaceStatusContext(context.Background(), id)
}

func (r *ItemRepository) FindByIDWithWorkspaceStatusContext(ctx context.Context, id int) (*ItemWithWorkspaceStatus, error) {
	item, workspaceActive, err := scanItemDetailsRow(r.db.QueryRowContext(ctx, itemDetailsSelectBody+"\n\t\tWHERE i.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find item with details: %w", err)
	}

	// Attach milestones once so callers receive complete item details.
	holder := []models.Item{item}
	if err := NewMilestoneAttachRepository(r.db).LoadForItemsContext(ctx, holder); err == nil {
		item = holder[0]
	} else if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return &ItemWithWorkspaceStatus{Item: &item, WorkspaceActive: workspaceActive}, nil
}

func (r *ItemRepository) GetWorkspaceID(itemID int) (int, error) {
	var workspaceID int
	if err := r.scanItemColumn(itemID, "workspace_id", "get workspace id", &workspaceID); err != nil {
		return 0, err
	}
	return workspaceID, nil
}

func (r *ItemRepository) GetWorkspaceIDCtx(ctx context.Context, itemID int) (int, error) {
	var workspaceID int
	err := r.db.QueryRowContext(ctx, "SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err != nil {
		return 0, mapItemErr(err, "get workspace id")
	}
	return workspaceID, nil
}

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

// GetTitles omits missing IDs because callers hydrate mixed entity sets.
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

// ItemRefByCustomField identifies an item referencing an asset in custom fields.
type ItemRefByCustomField struct {
	ID          int
	Title       string
	WorkspaceID int
}

// ListItemsReferencingAssetInCustomField scans scalar, object, and array asset
// references for a field key. Callers provide stringified values.
func (r *ItemRepository) ListItemsReferencingAssetInCustomField(fieldKey, assetIDStr string) ([]ItemRefByCustomField, error) {
	var query string
	if r.db.GetDriverName() == "postgres" {
		directExpr := fmt.Sprintf("i.custom_field_values->>'%s'", fieldKey)
		nestedExpr := fmt.Sprintf("i.custom_field_values->'%s'->>'id'", fieldKey)
		arrayExpr := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM jsonb_array_elements(CASE
				WHEN jsonb_typeof(i.custom_field_values->'%s') = 'array' THEN i.custom_field_values->'%s'
				ELSE '[]'::jsonb
			END) AS elem
			WHERE elem #>> '{}' = ? OR elem->>'id' = ?
		)`, fieldKey, fieldKey)
		query = fmt.Sprintf(`
			SELECT i.id, i.title, i.workspace_id
			FROM items i
			WHERE (%s = ? OR %s = ? OR %s)
		`, directExpr, nestedExpr, arrayExpr)
	} else {
		directExpr := fmt.Sprintf(`NULLIF(i.custom_field_values,'') ->> '$."%s"'`, fieldKey)    //nolint:gocritic // SQL JSON path, not Go quoting
		nestedExpr := fmt.Sprintf(`NULLIF(i.custom_field_values,'') ->> '$."%s".id'`, fieldKey) //nolint:gocritic // SQL JSON path, not Go quoting
		arrayExpr := fmt.Sprintf(`EXISTS (
			SELECT 1 FROM json_each(NULLIF(i.custom_field_values,'') -> '$."%s"') AS elem
			WHERE CAST(elem.value AS TEXT) = ? OR elem.value ->> '$.id' = ?
		)`, fieldKey) //nolint:gocritic // SQL JSON path, not Go quoting
		query = fmt.Sprintf(`
			SELECT i.id, i.title, i.workspace_id
			FROM items i
			WHERE (%s = ? OR %s = ? OR %s)
		`, directExpr, nestedExpr, arrayExpr)
	}
	rows, err := r.db.Query(query, assetIDStr, assetIDStr, assetIDStr, assetIDStr)
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

// ItemGraphMetadata contains display, permission, and status data for a graph node.
type ItemGraphMetadata struct {
	WorkspaceKey        string
	WorkspaceItemNumber int
	WorkspaceID         int
	StatusName          string
}

func (r *ItemRepository) GetItemGraphMetadata(itemID int) (*ItemGraphMetadata, error) {
	var meta ItemGraphMetadata
	err := r.db.QueryRow(`
		SELECT w.key, i.workspace_item_number, i.workspace_id, COALESCE(s.name, '')
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE i.id = ?
	`, itemID).Scan(&meta.WorkspaceKey, &meta.WorkspaceItemNumber, &meta.WorkspaceID, &meta.StatusName)
	if err != nil {
		return nil, mapItemErr(err, "get item graph metadata")
	}
	return &meta, nil
}

func (r *ItemRepository) GetCustomFieldValuesRaw(itemID int) (sql.NullString, error) {
	var data sql.NullString
	if err := r.scanItemColumn(itemID, "custom_field_values", "get custom field values", &data); err != nil {
		return sql.NullString{}, err
	}
	return data, nil
}

func (r *ItemRepository) GetCustomFieldValuesRawTx(ctx context.Context, tx database.Tx, itemID int) (sql.NullString, error) {
	var data sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT custom_field_values FROM items WHERE id = ?`, itemID).Scan(&data); err != nil {
		return sql.NullString{}, mapItemErr(err, "get custom field values")
	}
	return data, nil
}

func (r *ItemRepository) SetCustomFieldValuesRaw(ctx context.Context, itemID int, raw string) error {
	// An emptied cfv comes through as "". Write NULL rather than the empty
	// string: the column treats empty/NULL identically, but on Postgres the
	// JSONB column rejects '' ("invalid input syntax for type json").
	if raw == "" {
		if _, err := r.db.ExecWriteContext(ctx, `UPDATE items SET custom_field_values = NULL WHERE id = ?`, itemID); err != nil {
			return fmt.Errorf("set custom field values: %w", err)
		}
		return nil
	}
	if _, err := r.db.ExecWriteContext(ctx, `UPDATE items SET custom_field_values = ? WHERE id = ?`, raw, itemID); err != nil {
		return fmt.Errorf("set custom field values: %w", err)
	}
	return nil
}

func (r *ItemRepository) SetVirtualFieldDataRaw(ctx context.Context, itemID int, raw string) error {
	if _, err := r.db.ExecWriteContext(ctx, `UPDATE items SET virtual_field_data = ? WHERE id = ?`, raw, itemID); err != nil {
		return fmt.Errorf("set virtual field data: %w", err)
	}
	return nil
}

// UpdateDescription replaces only an item's description. Import workflows use
// this after attachment mappings make rich-text media links resolvable.
func (r *ItemRepository) UpdateDescription(itemID int, description string) error {
	result, err := r.db.ExecWrite(
		"UPDATE items SET description = ? WHERE id = ?",
		description,
		itemID,
	)
	if err != nil {
		return fmt.Errorf("update item description: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ItemCFVRow is one (id, custom_field_values) pair from a paged scan.
type ItemCFVRow struct {
	ID  int
	CFV string
}

func (r *ItemRepository) ListCustomFieldValuesPageByKey(afterID int, fieldKey string, limit int) ([]ItemCFVRow, error) {
	// CAST(... AS TEXT) so the LIKE key prefilter works on both SQLite (TEXT
	// column) and Postgres (JSONB column — a bare LIKE errors with "jsonb ~~").
	rows, err := r.db.Query(
		`SELECT id, custom_field_values
		   FROM items
		  WHERE id > ?
		    AND custom_field_values IS NOT NULL
		    AND CAST(custom_field_values AS TEXT) != ''
		    AND CAST(custom_field_values AS TEXT) LIKE ?
		  ORDER BY id ASC
		  LIMIT ?`,
		afterID, `%"`+fieldKey+`"%`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list custom field values page: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ItemCFVRow
	for rows.Next() {
		var row ItemCFVRow
		if err := rows.Scan(&row.ID, &row.CFV); err != nil {
			return nil, fmt.Errorf("scan custom field values row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

var itemRemapColumns = map[string]bool{
	"status_id":    true,
	"priority_id":  true,
	"item_type_id": true,
}

// RemapFieldForWorkspacesTx remaps one allowlisted reference column in a
// transaction and optionally restricts by item type.
func (r *ItemRepository) RemapFieldForWorkspacesTx(tx database.Tx, column string, fromID *int, toID int, itemTypeID *int, workspaceIDs []int, now time.Time) (int, error) {
	if !itemRemapColumns[column] {
		return 0, fmt.Errorf("RemapFieldForWorkspacesTx: column %q is not in the allow-list", column)
	}
	if len(workspaceIDs) == 0 {
		return 0, nil
	}
	// The column name is validated against the fixed allow-list above, so the
	// fmt.Sprintf cannot splice attacker-controlled input.
	query := fmt.Sprintf("UPDATE items SET %s = ?, updated_at = ?", column)
	args := []any{toID, now}
	if fromID == nil {
		query += fmt.Sprintf(" WHERE %s IS NULL", column)
	} else {
		query += fmt.Sprintf(" WHERE %s = ?", column)
		args = append(args, *fromID)
	}
	if itemTypeID != nil {
		query += " AND item_type_id = ?"
		args = append(args, *itemTypeID)
	}
	ph, wsArgs := inPlaceholders(workspaceIDs)
	query += " AND workspace_id IN (" + ph + ")"
	args = append(args, wsArgs...)

	res, err := tx.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("remap %s: %w", column, err)
	}
	rows, _ := res.RowsAffected()
	return int(rows), nil
}

func (r *ItemRepository) SetCustomFieldValuesRawTx(tx database.Tx, itemID int, raw string, now time.Time) error {
	if _, err := tx.Exec(`UPDATE items SET custom_field_values = ?, updated_at = ? WHERE id = ?`, raw, now, itemID); err != nil {
		return fmt.Errorf("set custom field values: %w", err)
	}
	return nil
}

func (r *ItemRepository) DeleteByWorkspaceTx(tx database.Tx, workspaceID int) error {
	if _, err := tx.Exec(`DELETE FROM items WHERE workspace_id = ?`, workspaceID); err != nil {
		return fmt.Errorf("delete items by workspace: %w", err)
	}
	return nil
}

func (r *ItemRepository) ClearAssigneeForUserTx(tx database.Tx, userID int) error {
	if _, err := tx.Exec(`UPDATE items SET assignee_id = NULL WHERE assignee_id = ?`, userID); err != nil {
		return fmt.Errorf("clear assignee for user: %w", err)
	}
	return nil
}

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

const itemNumberAdvisoryLockClass = 0x4954 // 'IT'

// GetNextWorkspaceItemNumber serializes Postgres allocation per workspace with
// an advisory lock; SQLite's writer lock already provides the needed isolation.
func (r *ItemRepository) GetNextWorkspaceItemNumber(tx database.Tx, workspaceID int) (int, error) {
	// Serialize per-workspace number assignment on Postgres. MAX+1 alone races:
	// `ORDER BY ... DESC LIMIT 1 FOR UPDATE` has no row to lock on a fresh
	// workspace (and a concurrently-inserted higher row isn't visible to an
	// already-planned query), so N concurrent inserts all compute the same
	// number. Only one wins per unique-violation retry round, so a batch of 10
	// creates blows through the bounded retry budget and 500s. Unlike
	// frac_index, workspace_item_number must be dense, so the jitter trick can't
	// apply. A transaction-scoped advisory lock keyed on the workspace
	// serializes just this numbering step — it's released on commit/rollback and
	// leaves cross-workspace creates fully parallel. SQLite already serializes
	// writers at the database level, so MAX+1 is safe there and needs no lock.
	if r.db.GetDriverName() == "postgres" {
		if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(?, ?)`, itemNumberAdvisoryLockClass, workspaceID); err != nil {
			return 0, fmt.Errorf("failed to acquire item-number lock: %w", err)
		}
	}
	// Read each indexed maximum independently instead of materializing and
	// sorting the union of every item and reservation in the workspace.
	var maxItem, maxReservation sql.NullInt64
	if err := tx.QueryRow(
		`SELECT MAX(workspace_item_number) FROM items WHERE workspace_id = ?`,
		workspaceID,
	).Scan(&maxItem); err != nil {
		return 0, fmt.Errorf("failed to get max item number: %w", err)
	}
	if err := tx.QueryRow(
		`SELECT MAX(workspace_item_number) FROM item_key_reservations WHERE workspace_id = ?`,
		workspaceID,
	).Scan(&maxReservation); err != nil {
		return 0, fmt.Errorf("failed to get max reserved item number: %w", err)
	}
	maxNumber := int64(0)
	if maxItem.Valid && maxItem.Int64 > maxNumber {
		maxNumber = maxItem.Int64
	}
	if maxReservation.Valid && maxReservation.Int64 > maxNumber {
		maxNumber = maxReservation.Int64
	}
	return int(maxNumber + 1), nil
}

// Create inserts an item in a caller-owned transaction. Production creation
// flows should use CreateWithRetry so rank and item-number conflicts restart
// the full transaction.
func (r *ItemRepository) Create(tx database.Tx, item *models.Item) (int, error) {
	if err := acquireGlobalRankMutationLock(tx, r.db.GetDriverName()); err != nil {
		return 0, err
	}
	customFieldValuesJSON, err := marshalCustomFields(item.CustomFieldValues)
	if err != nil {
		return 0, err
	}
	fracIndex := item.FracIndex
	if fracIndex == nil || *fracIndex == "" {
		generated, err := GenerateFracIndexForNewItem(tx, r.db.GetDriverName())
		if err != nil {
			return 0, fmt.Errorf("failed to generate frac_index: %w", err)
		}
		fracIndex = &generated
	}

	now := time.Now()
	var id int64
	err = tx.QueryRow(`
		INSERT INTO items (
			workspace_id, workspace_item_number, item_type_id, title, description, status_id,
			priority_id, due_date, start_date, end_date, is_task, iteration_id, project_id, inherit_project,
			assignee_id, creator_id, custom_field_values, parent_id, related_work_item_id,
			story_points, estimate_minutes, frac_index, created_at, updated_at, last_active_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		item.WorkspaceID, item.WorkspaceItemNumber, item.ItemTypeID, item.Title, item.Description,
		item.StatusID, item.PriorityID, item.DueDate, item.StartDate, item.EndDate, item.IsTask,
		item.IterationID, item.ProjectID, item.InheritProject, item.AssigneeID, item.CreatorID,
		customFieldValuesJSON, item.ParentID, item.RelatedWorkItemID,
		item.StoryPoints, item.EstimateMinutes, fracIndex, now, now, now,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create item: %w", err)
	}

	return int(id), nil
}

func (r *ItemRepository) Update(tx database.Tx, item *models.Item) error {
	customFieldValuesJSON, err := marshalCustomFields(item.CustomFieldValues)
	if err != nil {
		return err
	}

	now := time.Now()
	_, err = tx.Exec(`
		UPDATE items
		SET workspace_id = ?, title = ?, description = ?, status_id = ?, priority_id = ?,
		    due_date = ?, start_date = ?, end_date = ?, is_task = ?, iteration_id = ?, project_id = ?, inherit_project = ?,
		    time_project_id = ?, assignee_id = ?, creator_id = ?, custom_field_values = ?, parent_id = ?,
		    related_work_item_id = ?, story_points = ?, estimate_minutes = ?, updated_at = ?, last_active_at = ?
		WHERE id = ?
	`,
		item.WorkspaceID, item.Title, item.Description, item.StatusID, item.PriorityID,
		item.DueDate, item.StartDate, item.EndDate, item.IsTask, item.IterationID, item.ProjectID, item.InheritProject,
		item.TimeProjectID, item.AssigneeID, item.CreatorID, customFieldValuesJSON, item.ParentID,
		item.RelatedWorkItemID, item.StoryPoints, item.EstimateMinutes, now, now, item.ID,
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
	"story_points": true, "estimate_minutes": true,
}

// IsAllowedItemColumn reports whether col is safe for dynamic item queries.
func IsAllowedItemColumn(col string) bool {
	return allowedItemColumns[col]
}

// GetAllowedColumnValue reads one allowlisted item column.
func (r *ItemRepository) GetAllowedColumnValue(itemID int, col string) (any, error) {
	if !allowedItemColumns[col] {
		return nil, fmt.Errorf("unknown item column: %s", col)
	}
	var val any
	if err := r.db.QueryRow(`SELECT `+col+` FROM items WHERE id = ?`, itemID).Scan(&val); err != nil {
		return nil, fmt.Errorf("get item column %s: %w", col, err)
	}
	return val, nil
}

// UpdateFields updates only allowlisted item columns.
func (r *ItemRepository) UpdateFields(tx database.Tx, itemID int, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	if _, changesRank := fields["frac_index"]; changesRank {
		if err := acquireGlobalRankMutationLock(tx, r.db.GetDriverName()); err != nil {
			return err
		}
	}

	setClauses := make([]string, 0, len(fields)+1)
	args := make([]any, 0, len(fields)+2)

	for col, val := range fields {
		if !allowedItemColumns[col] {
			return fmt.Errorf("unknown item column: %s", col)
		}
		setClauses = append(setClauses, col+" = ?")
		args = append(args, val)
	}

	// Any field edit (incl. status transitions, which flow through here) counts
	// as activity for the board's Bubble Mode recency sort. Manual reorders write
	// frac_index via a raw UPDATE in fracindex.go, not UpdateFields, so they are
	// intentionally excluded from this bump.
	now := time.Now()
	setClauses = append(setClauses, "updated_at = ?", "last_active_at = ?")
	args = append(args, now, now)
	args = append(args, itemID)

	query := "UPDATE items SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
	_, err := tx.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update item fields: %w", err)
	}
	return nil
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// TouchActivity updates Bubble Mode recency without changing updated_at.
func (r *ItemRepository) TouchActivity(exec execer, itemID int, now time.Time) error {
	if _, err := exec.Exec("UPDATE items SET last_active_at = ? WHERE id = ?", now, itemID); err != nil {
		return fmt.Errorf("failed to touch item activity: %w", err)
	}
	return nil
}

// TouchChanged marks a user-visible item change for delta sync and recency.
func (r *ItemRepository) TouchChanged(exec execer, itemID int, now time.Time) error {
	if _, err := exec.Exec(
		"UPDATE items SET updated_at = ?, last_active_at = ? WHERE id = ?",
		now, now, itemID,
	); err != nil {
		return fmt.Errorf("failed to touch changed item: %w", err)
	}
	return nil
}

// GetItemCustomFieldValue returns a decoded field value, or nil when absent.
func (r *ItemRepository) GetItemCustomFieldValue(itemID, customFieldID int) (any, error) {
	var raw sql.NullString
	if err := r.db.QueryRow(`SELECT custom_field_values FROM items WHERE id = ?`, itemID).Scan(&raw); err != nil {
		return nil, fmt.Errorf("load item custom_field_values: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw.String), &values); err != nil {
		return nil, nil //nolint:nilerr // treat malformed blob as "no value present"
	}
	return values[strconv.Itoa(customFieldID)], nil
}

// SetItemCustomFieldValue updates one JSON field while preserving the others.
func (r *ItemRepository) SetItemCustomFieldValue(tx database.Tx, itemID, customFieldID int, value any) error {
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

	values := make(map[string]any)
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

func (r *ItemRepository) Delete(tx database.Tx, id int) error {
	_, err := tx.Exec("DELETE FROM items WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}
	return nil
}

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

func (r *ItemRepository) ClearWorklogItemReferences(tx database.Tx, itemID int) error {
	_, err := tx.Exec("UPDATE time_worklogs SET item_id = NULL WHERE item_id = ?", itemID)
	if err != nil {
		return fmt.Errorf("failed to clear worklog references: %w", err)
	}
	return nil
}

func (r *ItemRepository) GetParentID(itemID int) (*int, error) {
	var parentID sql.NullInt64
	if err := r.scanItemColumn(itemID, "parent_id", "get parent id", &parentID); err != nil {
		return nil, err
	}
	return nullIntPtr(parentID), nil
}

// GetParentIDAndHierarchyLevel returns the stored parent and hierarchy level.
func (r *ItemRepository) GetParentIDAndHierarchyLevel(itemID int) (parentID, hierarchyLevel *int, err error) {
	var parent, level sql.NullInt64
	err = r.db.QueryRow(`
		SELECT i.parent_id, it.hierarchy_level
		FROM items i
		LEFT JOIN item_types it ON i.item_type_id = it.id
		WHERE i.id = ?
	`, itemID).Scan(&parent, &level)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get parent and hierarchy level: %w", err)
	}
	assignNullableInt(&parentID, parent)
	assignNullableInt(&hierarchyLevel, level)
	return parentID, hierarchyLevel, nil
}

// GetParentIDTx locks and reads the parent inside the supplied transaction.
func (r *ItemRepository) GetParentIDTx(tx database.Tx, itemID int) (*int, error) {
	query := `SELECT parent_id FROM items WHERE id = ?`
	if r.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	var parentID sql.NullInt64
	if err := tx.QueryRow(query, itemID).Scan(&parentID); err != nil {
		return nil, mapItemErr(err, "get parent id")
	}
	return nullIntPtr(parentID), nil
}

func (r *ItemRepository) Exists(id int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM items WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check item existence: %w", err)
	}
	return exists, nil
}

// GetCalendarData returns calendar data and its workspace for authorization.
func (r *ItemRepository) GetCalendarData(itemID int) (sql.NullString, int, error) {
	var data sql.NullString
	var workspaceID int
	err := r.db.QueryRow(
		"SELECT calendar_data, workspace_id FROM items WHERE id = ?",
		itemID,
	).Scan(&data, &workspaceID)
	if err != nil {
		return sql.NullString{}, 0, mapItemErr(err, "get calendar data")
	}
	return data, workspaceID, nil
}

func (r *ItemRepository) UpdateCalendarData(itemID int, data string, updatedAt time.Time) error {
	result, err := r.db.ExecWrite("UPDATE items SET calendar_data = ?, updated_at = ? WHERE id = ?", data, updatedAt, itemID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}

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

// CountByField counts items matching an allowlisted column value.
func (r *ItemRepository) CountByField(column string, value any) (int, error) {
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

// FindIDByKeyAndNumber resolves a case-insensitive KEY-NUMBER reference.
func (r *ItemRepository) FindIDByKeyAndNumber(workspaceKey string, itemNumber int) (int, error) {
	var id int
	err := r.db.QueryRow(
		"SELECT i.id FROM items i JOIN workspaces w ON i.workspace_id = w.id WHERE UPPER(w.key) = UPPER(?) AND i.workspace_item_number = ?",
		workspaceKey, itemNumber,
	).Scan(&id)
	if err != nil {
		return 0, mapItemErr(err, "resolve item by key")
	}
	return id, nil
}

// GetItemKey returns an item's KEY-NUMBER display key.
func (r *ItemRepository) GetItemKey(itemID int) (string, error) {
	var workspaceKey string
	var itemNumber int
	err := r.db.QueryRow(`
		SELECT w.key, i.workspace_item_number
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, itemID).Scan(&workspaceKey, &itemNumber)
	if err != nil {
		return "", mapItemErr(err, "get item key")
	}
	return fmt.Sprintf("%s-%d", workspaceKey, itemNumber), nil
}

var itemUserRefColumns = map[string]bool{
	"assignee_id": true,
	"creator_id":  true,
	"reporter_id": true,
}

// GetUserFieldTx reads an allowlisted user-reference column inside a transaction.
func (r *ItemRepository) GetUserFieldTx(ctx context.Context, tx database.Tx, itemID int, col string) (*int, error) {
	if !itemUserRefColumns[col] {
		return nil, fmt.Errorf("unknown item user column: %s", col)
	}
	var nid sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT `+col+` FROM items WHERE id = ?`, itemID).Scan(&nid); err != nil {
		return nil, fmt.Errorf("get item column %s: %w", col, err)
	}
	return nullIntPtr(nid), nil
}

// FindIDByKeyAndNumberInWorkspace constrains KEY-NUMBER lookup to one workspace.
func (r *ItemRepository) FindIDByKeyAndNumberInWorkspace(workspaceID int, workspaceKey string, itemNumber int) (int, error) {
	var id int
	err := r.db.QueryRow(
		"SELECT i.id FROM items i JOIN workspaces w ON i.workspace_id = w.id WHERE i.workspace_id = ? AND i.workspace_item_number = ? AND UPPER(w.key) = UPPER(?)",
		workspaceID, itemNumber, workspaceKey,
	).Scan(&id)
	if err != nil {
		return 0, mapItemErr(err, "resolve item by key in workspace")
	}
	return id, nil
}

// GetFracIndex returns an item's ordering key, or nil when unset.
func (r *ItemRepository) GetFracIndex(itemID int) (*string, error) {
	var fracIndex sql.NullString
	if err := r.scanItemColumn(itemID, "frac_index", "get frac_index", &fracIndex); err != nil {
		return nil, err
	}
	return nullStrPtr(fracIndex), nil
}

// ListItemsLinkedToTestResult scopes links through the test run's workspace.
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

type ItemCustomFields struct {
	ID      int
	CFVJSON string
}

// ListItemCustomFieldsTx streams item custom-field JSON, coercing NULL to "{}".
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

// SearchLinkableItems searches linkable items within the given workspaces.
func (r *ItemRepository) SearchLinkableItems(query string, workspaceIDs, itemTypeIDs []int, limit int) ([]models.LinkableItem, error) {
	if len(workspaceIDs) == 0 {
		return []models.LinkableItem{}, nil
	}
	wsPlaceholders := make([]string, len(workspaceIDs))
	args := []any{}
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
			w.key AS workspace_key,
			i.workspace_item_number,
			COALESCE(s.name, '') AS status_name,
			COALESCE(p.name, '') AS priority_name,
			i.item_type_id,
			COALESCE(it.name, '') AS item_type_name,
			COALESCE(it.icon, '') AS item_type_icon,
			COALESCE(it.color, '') AS item_type_color
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
		var description, workspaceName, workspaceKey, statusName, priorityName, itemTypeName, itemTypeIcon, itemTypeColor sql.NullString
		var workspaceID, workspaceItemNumber, itemTypeID sql.NullInt64

		if err := rows.Scan(
			&item.ID, &item.Title, &description,
			&workspaceID, &workspaceName, &workspaceKey, &workspaceItemNumber,
			&statusName, &priorityName,
			&itemTypeID, &itemTypeName, &itemTypeIcon, &itemTypeColor,
		); err != nil {
			return nil, err
		}

		item.Description = description.String
		if workspaceID.Valid {
			v := int(workspaceID.Int64)
			item.WorkspaceID = &v
		}
		item.WorkspaceName = workspaceName.String
		item.WorkspaceKey = workspaceKey.String
		if workspaceItemNumber.Valid {
			v := int(workspaceItemNumber.Int64)
			item.WorkspaceItemNumber = &v
		}
		item.Status = statusName.String
		item.Priority = priorityName.String
		if itemTypeID.Valid {
			v := int(itemTypeID.Int64)
			item.ItemTypeID = &v
		}
		item.ItemTypeName = itemTypeName.String
		item.ItemTypeIcon = itemTypeIcon.String
		item.ItemTypeColor = itemTypeColor.String
		item.Type = "item"
		items = append(items, item)
	}
	return items, rows.Err()
}

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

type KeyReference struct {
	ItemKey     string
	ItemID      int
	WorkspaceID int
}

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

func (r *ItemRepository) ResolveItemKeyReferencesInWorkspaces(keys []string, workspaceIDs []int) ([]KeyReference, error) {
	if len(keys) == 0 || len(workspaceIDs) == 0 {
		return []KeyReference{}, nil
	}
	keyPlaceholders, args := inPlaceholders(keys)
	workspacePlaceholders, workspaceArgs := inPlaceholders(workspaceIDs)
	args = append(args, workspaceArgs...)
	query := `SELECT w.key || '-' || CAST(i.workspace_item_number AS TEXT) as item_key, i.id, i.workspace_id
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		WHERE w.key || '-' || CAST(i.workspace_item_number AS TEXT) IN (` + keyPlaceholders + `)
		  AND i.workspace_id IN (` + workspacePlaceholders + `)`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve scoped item keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := []KeyReference{}
	for rows.Next() {
		var ref KeyReference
		if err := rows.Scan(&ref.ItemKey, &ref.ItemID, &ref.WorkspaceID); err != nil {
			return nil, fmt.Errorf("scan scoped key reference: %w", err)
		}
		results = append(results, ref)
	}
	return results, rows.Err()
}

type CandidateItem struct {
	ID          int
	ItemKey     string
	Title       string
	StatusName  string
	Description string
}

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

func (r *ItemRepository) ListIterationItems(iterationIDs, workspaceIDs []int) ([]IterationItemInfo, error) {
	if len(iterationIDs) == 0 || len(workspaceIDs) == 0 {
		return []IterationItemInfo{}, nil
	}
	iterPlaceholders := make([]string, len(iterationIDs))
	iterArgs := make([]any, len(iterationIDs))
	for i, id := range iterationIDs {
		iterPlaceholders[i] = "?"
		iterArgs[i] = id
	}
	wsPlaceholders := make([]string, len(workspaceIDs))
	wsArgs := make([]any, len(workspaceIDs))
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

	args := make([]any, 0, len(iterArgs)+len(wsArgs))
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

type ItemWithCalendar struct {
	Item            models.Item
	CalendarEntries []models.CalendarScheduleEntry
	IsPersonal      bool
}

// ListItemsWithCalendarData returns parsed schedules for populated calendar rows.
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

// FindByIDsWithDetails loads joined items in one query plus a batched milestone
// attach. Missing IDs are omitted and result order is unspecified.
func (r *ItemRepository) FindByIDsWithDetails(ids []int) ([]*models.Item, error) {
	if len(ids) == 0 {
		return []*models.Item{}, nil
	}
	placeholders, args := inPlaceholders(ids)
	rows, err := r.db.Query(itemDetailsSelectBody+"\n\t\tWHERE i.id IN ("+placeholders+")", args...)
	if err != nil {
		return nil, fmt.Errorf("find items with details: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Materialize values before the batched milestone attach populates pointers.
	scanned := make([]models.Item, 0, len(ids))
	for rows.Next() {
		item, _, err := scanItemDetailsRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan item with details: %w", err)
		}
		scanned = append(scanned, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate items with details: %w", err)
	}

	// Attach milestones in one round trip for the full result set.
	if err := NewMilestoneAttachRepository(r.db).LoadForItems(scanned); err != nil {
		slog.Warn("failed to attach milestones for batch items", slog.Any("error", err))
	}

	items := make([]*models.Item, len(scanned))
	for i := range scanned {
		items[i] = &scanned[i]
	}
	return items, nil
}

// Homepage aggregations.

// CountNonPersonalByWorkspaceIDs returns the non-personal item count in the
// given workspaces.
func (r *ItemRepository) CountNonPersonalByWorkspaceIDs(workspaceIDs []int) (int, error) {
	if len(workspaceIDs) == 0 {
		return 0, nil
	}
	placeholders, args := inPlaceholders(workspaceIDs)
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM items i
		JOIN workspaces w ON w.id = i.workspace_id
		WHERE i.workspace_id IN (`+placeholders+`)
		  AND (w.is_personal = false OR w.is_personal IS NULL)
	`, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count visible items: %w", err)
	}
	return count, nil
}

// ClearRelatedWorkItem removes a personal task's related work item reference.
func (r *ItemRepository) ClearRelatedWorkItem(itemID int) error {
	res, err := r.db.ExecWrite(`
		UPDATE items
		SET related_work_item_id = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, itemID)
	if err != nil {
		return fmt.Errorf("clear related work item for item %d: %w", itemID, err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetHistoryWithApprovals returns item history plus approval decision events as a single chronological feed.
func (r *ItemRepository) GetHistoryWithApprovals(itemID int, includeAgentOwner bool) ([]models.ItemHistory, error) {
	query := `
		SELECT
			ih.id, ih.item_id, ih.user_id, ih.changed_at, ih.field_name, ih.old_value, ih.new_value,
			COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as user_name,
			COALESCE(u.email, '') as user_email,
			COALESCE(u.is_agent, FALSE) AS is_agent,
			COALESCE(NULLIF(TRIM(COALESCE(owner.first_name, '') || ' ' || COALESCE(owner.last_name, '')), ''), owner.username, '') AS agent_owner_name
		FROM item_history ih
		LEFT JOIN users u ON ih.user_id = u.id
		LEFT JOIN users owner ON owner.id = u.agent_owner_user_id
		WHERE ih.item_id = ?
		UNION ALL
		SELECT
			-d.id AS id,
			ar.item_id,
			COALESCE(d.actor_user_id, 0) AS user_id,
			d.created_at AS changed_at,
			'approval_' || d.decision AS field_name,
			NULL AS old_value,
			d.comment AS new_value,
			COALESCE(u.first_name || ' ' || u.last_name, u.username, 'System') AS user_name,
			COALESCE(u.email, '') AS user_email,
			COALESCE(u.is_agent, FALSE) AS is_agent,
			COALESCE(NULLIF(TRIM(COALESCE(owner.first_name, '') || ' ' || COALESCE(owner.last_name, '')), ''), owner.username, '') AS agent_owner_name
		FROM approval_decisions d
		JOIN approval_requests ar ON ar.id = d.approval_request_id
		LEFT JOIN users u ON u.id = d.actor_user_id
		LEFT JOIN users owner ON owner.id = u.agent_owner_user_id
		WHERE ar.item_id = ?
		ORDER BY changed_at DESC
	`

	rows, err := r.db.Query(query, itemID, itemID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	history := []models.ItemHistory{}
	for rows.Next() {
		var entry models.ItemHistory
		if err := rows.Scan(&entry.ID, &entry.ItemID, &entry.UserID, &entry.ChangedAt, &entry.FieldName, &entry.OldValue, &entry.NewValue, &entry.UserName, &entry.UserEmail, &entry.IsAgent, &entry.AgentOwnerName); err != nil {
			return nil, err
		}
		if !includeAgentOwner {
			entry.AgentOwnerName = ""
		}
		history = append(history, entry)
	}
	return history, rows.Err()
}

// GetCQLCustomFieldMap returns a lowercase-name → {ID, Kind, ...} map of every
// custom field definition. Item custom fields are global (custom_field_definitions
// and request_types are not workspace-scoped in the schema), so the map is
// returned in full and used by CQL to resolve UI-supplied names like cf_Severity
// to the numeric JSON key used in items.custom_field_values. For linking fields,
// also reads options to detect mirror fields and target-type constraints.
func (r *ItemRepository) GetCQLCustomFieldMap() (cql.CustomFieldMap, error) {
	return r.GetCQLCustomFieldMapContext(context.Background())
}

// GetCQLCustomFieldMapContext is the request-aware form of GetCQLCustomFieldMap.
func (r *ItemRepository) GetCQLCustomFieldMapContext(ctx context.Context) (cql.CustomFieldMap, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, LOWER(name), field_type, COALESCE(options, '') FROM custom_field_definitions`)
	if err != nil {
		return nil, fmt.Errorf("failed to query item custom fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cfMap := make(cql.CustomFieldMap)
	for rows.Next() {
		var id int
		var name, fieldType, options string
		if err := rows.Scan(&id, &name, &fieldType, &options); err != nil {
			return nil, fmt.Errorf("failed to scan custom field: %w", err)
		}
		info := cql.CustomFieldInfo{
			ID:        id,
			Kind:      cql.ClassifyCustomFieldKind(fieldType),
			FieldType: strings.ToLower(fieldType),
		}
		if info.Kind == cql.CFKindLinking {
			info.MirrorOfFieldID, info.AllowedTargetTypes = cql.LinkingFieldOptions(options)
		}
		cfMap[name] = info
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate custom fields: %w", err)
	}
	return cfMap, nil
}
