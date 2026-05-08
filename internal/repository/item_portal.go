package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"windshift/internal/models"
)

// --- Personal-workspace item helpers ----------------------------------------

// ListRelatedPersonalItems returns items in personalWorkspaceID that are linked
// via related_work_item_id to the given work item, hydrated with workspace,
// item_type, status, priority, and assignee names used by the personal-tasks
// widget. Results are ordered newest-first.
func (r *ItemRepository) ListRelatedPersonalItems(relatedWorkItemID, personalWorkspaceID int) ([]models.Item, error) {
	query := `
		SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, i.description,
			i.status_id, i.priority_id, i.is_task,
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
			&item.StatusID, &item.PriorityID, &item.IsTask,
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

		assignNullableString(&item.ItemTypeName, itemTypeName)
		assignNullableString(&item.StatusName, statusName)
		assignNullableString(&item.PriorityName, priorityName)
		assignNullableString(&item.PriorityIcon, priorityIcon)
		assignNullableString(&item.PriorityColor, priorityColor)
		assignNullableString(&item.AssigneeName, assigneeName)
		assignNullableString(&item.AssigneeEmail, assigneeEmail)
		assignNullableString(&item.AssigneeAvatar, assigneeAvatar)

		item.CalendarData = []models.CalendarScheduleEntry{}
		if calendarDataJSON.Valid && calendarDataJSON.String != "" {
			if err := json.Unmarshal([]byte(calendarDataJSON.String), &item.CalendarData); err != nil {
				item.CalendarData = []models.CalendarScheduleEntry{}
			}
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
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get item workspace ownership: %w", err)
	}
	assignNullableInt(&out.OwnerID, ownerID)
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
	placeholders, wsArgs := inPlaceholders(workspaceIDs)
	args := append([]interface{}{orgID}, wsArgs...)

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
		  AND i.workspace_id IN (` + placeholders + `)
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
