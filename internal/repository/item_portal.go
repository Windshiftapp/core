package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

// PersonalWorkspaceTask is the minimal projection of a personal-workspace task
// the Todoist sync engine reconciles against. Completed is derived from the
// status's category (is_completed), the same signal the UI uses to render a
// task as done.
type PersonalWorkspaceTask struct {
	ItemID      int
	Title       string
	Description string
	DueDate     *time.Time
	ParentID    *int
	Completed   bool
	UpdatedAt   time.Time
}

// ListPersonalWorkspaceTasks returns every task item in a personal workspace,
// projected for Todoist sync. is_task filters out any non-task rows so the sync
// mirrors the personal to-do list rather than arbitrary items.
func (r *ItemRepository) ListPersonalWorkspaceTasks(workspaceID int) ([]PersonalWorkspaceTask, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.title, i.description, i.due_date, i.parent_id, i.updated_at,
		       COALESCE(sc.is_completed, false) AS completed
		FROM items i
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ? AND i.is_task = true
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list personal workspace tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tasks []PersonalWorkspaceTask
	for rows.Next() {
		var t PersonalWorkspaceTask
		var dueDate sql.NullTime
		var parentID sql.NullInt64
		if err := rows.Scan(&t.ItemID, &t.Title, &t.Description, &dueDate, &parentID, &t.UpdatedAt, &t.Completed); err != nil {
			return nil, fmt.Errorf("scan personal workspace task: %w", err)
		}
		if dueDate.Valid {
			t.DueDate = &dueDate.Time
		}
		assignNullableInt(&t.ParentID, parentID)
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate personal workspace tasks: %w", err)
	}
	return tasks, nil
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

// --- Portal channel request lookups ------------------------------------------

// PortalRequestRow is one row returned by the portal channel request queries —
// an item submitted through a portal channel, hydrated with workspace,
// request-type, status, and comment-count metadata.
type PortalRequestRow struct {
	ID                      int
	WorkspaceID             int
	WorkspaceItemNumber     int
	Title                   string
	Description             string
	StatusName              string
	PriorityName            string
	CreatedAt               string
	UpdatedAt               string
	ChannelID               *int
	IntakeID                *int
	RequestTypeID           *int
	CreatorID               *int
	CreatorPortalCustomerID *int
	WorkspaceName           string
	WorkspaceKey            string
	RequestTypeName         *string
	RequestTypeIcon         *string
	RequestTypeColor        *string
	CommentCount            int
	StatusCategoryColor     *string
	StatusIsCompleted       bool
	// MergedIntoItemID is the canonical ticket when this request was folded
	// into another. The portal redirects to it instead of showing a stale
	// duplicate thread (WI-1528).
	MergedIntoItemID *int
}

const portalRequestSelect = `
	SELECT
		i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
		COALESCE(s.name, 'Unknown') AS status, COALESCE(p.name, '') AS priority,
		i.created_at, i.updated_at,
		i.channel_id, i.intake_id, i.request_type_id, i.creator_id, i.creator_portal_customer_id,
		w.name AS workspace_name,
		w.key AS workspace_key,
		rt.name AS request_type_name,
		rt.icon AS request_type_icon,
		rt.color AS request_type_color,
		(SELECT COUNT(*) FROM comments WHERE item_id = i.id AND (is_private = false OR is_private IS NULL)) AS comment_count,
		sc.color AS status_category_color,
		COALESCE(sc.is_completed, false) AS status_is_completed,
		i.merged_into_item_id
	FROM items i
	JOIN workspaces w ON i.workspace_id = w.id
	LEFT JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
	LEFT JOIN request_types rt ON i.request_type_id = rt.id
	LEFT JOIN statuses s ON i.status_id = s.id
	LEFT JOIN status_categories sc ON s.category_id = sc.id
	LEFT JOIN priorities p ON i.priority_id = p.id
`

func scanPortalRequestRow(scanner interface {
	Scan(dest ...any) error
}) (PortalRequestRow, error) {
	var row PortalRequestRow
	var channelID, intakeID, requestTypeID, creatorID, creatorPortalCustomerID, mergedIntoItemID sql.NullInt64
	var requestTypeName, requestTypeIcon, requestTypeColor, statusCategoryColor sql.NullString
	err := scanner.Scan(
		&row.ID, &row.WorkspaceID, &row.WorkspaceItemNumber, &row.Title, &row.Description,
		&row.StatusName, &row.PriorityName, &row.CreatedAt, &row.UpdatedAt,
		&channelID, &intakeID, &requestTypeID, &creatorID, &creatorPortalCustomerID,
		&row.WorkspaceName, &row.WorkspaceKey,
		&requestTypeName, &requestTypeIcon, &requestTypeColor,
		&row.CommentCount,
		&statusCategoryColor, &row.StatusIsCompleted,
		&mergedIntoItemID,
	)
	if err != nil {
		return row, err
	}
	assignNullableInt(&row.ChannelID, channelID)
	assignNullableInt(&row.IntakeID, intakeID)
	assignNullableInt(&row.RequestTypeID, requestTypeID)
	assignNullableInt(&row.CreatorID, creatorID)
	assignNullableInt(&row.CreatorPortalCustomerID, creatorPortalCustomerID)
	assignNullableInt(&row.MergedIntoItemID, mergedIntoItemID)
	assignNullableStringPtr(&row.RequestTypeName, requestTypeName)
	assignNullableStringPtr(&row.RequestTypeIcon, requestTypeIcon)
	assignNullableStringPtr(&row.RequestTypeColor, requestTypeColor)
	assignNullableStringPtr(&row.StatusCategoryColor, statusCategoryColor)
	return row, nil
}

// portalMergedDuplicateVisible keeps a merged duplicate in a requester's list
// only when it is a cross-requester merge: the source's files and original
// request stay with its own requester (WI-1566), so hiding it would remove the
// customer's own ticket. A same-requester merge moves the thread to the
// canonical, so the duplicate is hidden as a redirect. "Same requester" is the
// portal-customer identity, matching ItemLifecycleService.sameRequester, with
// COALESCE so a NULL (internal) creator compares deterministically.
const portalMergedDuplicateVisible = `(i.merged_into_item_id IS NULL OR EXISTS (
		SELECT 1 FROM items canonical
		WHERE canonical.id = i.merged_into_item_id
		  AND COALESCE(canonical.creator_portal_customer_id, 0) <> COALESCE(i.creator_portal_customer_id, 0)
	))`

// PortalRequestVisibility describes which items a portal exposes to its
// requesters: the portal's own channel, any enabled intake email channel
// linked to it via email_connected_portal_id, and — as defense-in-depth —
// only items in workspaces the portal serves (empty = no restriction, for
// legacy portals).
type PortalRequestVisibility struct {
	PortalChannelID int
	// LinkedIntakeIDs are the enabled intakes linked to the portal. Their items
	// are visible even though their channel_id is the mailbox, so linking one
	// intake never exposes a sibling internal intake on the same mailbox.
	LinkedIntakeIDs []int
	// LinkedEmailChannelIDs are legacy email channels linked through
	// email_connected_portal_id without an intake row.
	LinkedEmailChannelIDs []int
	ServedWorkspaceIDs    []int
}

// PortalOrgShare describes the organisation-sharing that applies to one portal
// viewer. The service resolves it from the organisation's settings and the
// viewer's contact levels (settings live in JSON, so they are parsed in Go),
// leaving the repository to express only the resulting per-item condition. The
// zero value disables organisation sharing.
type PortalOrgShare struct {
	// OrganisationID is the viewer's organisation when sharing applies to them.
	OrganisationID int
	// Mode is models.OrgRequestSharingAutomatic or
	// models.OrgRequestSharingRequesterChoice. Empty means no org sharing.
	Mode string
}

// ListChannelRequestsByCreator returns the newest 500 requests an internal
// user submitted through the given portal (own channel plus linked intake
// channels).
func (r *ItemRepository) ListChannelRequestsByCreator(creatorID int, visibility PortalRequestVisibility) ([]PortalRequestRow, error) {
	return r.listChannelRequests("i.creator_id = ?", []any{creatorID}, visibility)
}

// ListChannelRequestsByPortalCustomer returns the newest 500 requests a portal
// customer submitted through the given portal (own channel plus linked intake
// channels), plus any requests where the customer is an external participant
// (WI-1136), plus organisation-shared requests when orgShare applies (WI-1139).
func (r *ItemRepository) ListChannelRequestsByPortalCustomer(customerID int, orgShare PortalOrgShare, visibility PortalRequestVisibility) ([]PortalRequestRow, error) {
	ownerClause := `(i.creator_portal_customer_id = ? OR EXISTS (
		SELECT 1 FROM item_participants p
		WHERE p.item_id = i.id AND p.portal_customer_id = ?
	))`
	ownerArgs := []any{customerID, customerID}
	switch {
	case orgShare.OrganisationID == 0:
		// Org sharing does not apply to this viewer.
	case orgShare.Mode == models.OrgRequestSharingAutomatic:
		ownerClause = "(" + ownerClause + " OR pc.customer_organisation_id = ?)"
		ownerArgs = append(ownerArgs, orgShare.OrganisationID)
	case orgShare.Mode == models.OrgRequestSharingRequesterChoice:
		ownerClause = "(" + ownerClause + " OR (pc.customer_organisation_id = ? AND i.portal_org_shared = true))"
		ownerArgs = append(ownerArgs, orgShare.OrganisationID)
	}
	return r.listChannelRequests(ownerClause, ownerArgs, visibility)
}

// PortalRequestOrgShare returns the creator's organisation and whether the item
// carries the organisation-share flag, for the portal detail gate. Returns
// ErrNotFound when the item does not exist.
func (r *ItemRepository) PortalRequestOrgShare(itemID int) (creatorOrgID *int, shared bool, err error) {
	var orgID sql.NullInt64
	var sharedFlag sql.NullBool
	err = r.db.QueryRow(`
		SELECT pc.customer_organisation_id, i.portal_org_shared
		FROM items i
		LEFT JOIN portal_customers pc ON i.creator_portal_customer_id = pc.id
		WHERE i.id = ?
	`, itemID).Scan(&orgID, &sharedFlag)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("get portal request organisation share: %w", err)
	}
	if orgID.Valid {
		id := int(orgID.Int64)
		creatorOrgID = &id
	}
	return creatorOrgID, sharedFlag.Valid && sharedFlag.Bool, nil
}

func (r *ItemRepository) listChannelRequests(ownerClause string, ownerArgs []any, visibility PortalRequestVisibility) ([]PortalRequestRow, error) {
	args := append([]any{}, ownerArgs...)
	channelFilter := "i.channel_id = ?"
	args = append(args, visibility.PortalChannelID)
	for _, id := range visibility.LinkedEmailChannelIDs {
		channelFilter += " OR i.channel_id = ?"
		args = append(args, id)
	}
	if len(visibility.LinkedIntakeIDs) > 0 {
		placeholders := strings.Repeat("?,", len(visibility.LinkedIntakeIDs))
		placeholders = placeholders[:len(placeholders)-1]
		channelFilter += " OR i.intake_id IN (" + placeholders + ")"
		for _, id := range visibility.LinkedIntakeIDs {
			args = append(args, id)
		}
	}
	where := ownerClause + " AND " + portalMergedDuplicateVisible + " AND (" + channelFilter + ")"
	if len(visibility.ServedWorkspaceIDs) > 0 {
		placeholders := strings.Repeat("?,", len(visibility.ServedWorkspaceIDs))
		placeholders = placeholders[:len(placeholders)-1]
		where += " AND i.workspace_id IN (" + placeholders + ")"
		for _, id := range visibility.ServedWorkspaceIDs {
			args = append(args, id)
		}
	}
	rows, err := r.db.Query(portalRequestSelect+`
		WHERE `+where+`
		ORDER BY i.created_at DESC
		LIMIT 500
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list channel requests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []PortalRequestRow{}
	for rows.Next() {
		row, err := scanPortalRequestRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan channel request: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// GetPortalRequest returns a single item with portal request metadata.
// Returns ErrNotFound if the item does not exist.
func (r *ItemRepository) GetPortalRequest(itemID int) (*PortalRequestRow, error) {
	row, err := scanPortalRequestRow(r.db.QueryRow(portalRequestSelect+" WHERE i.id = ?", itemID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get portal request %d: %w", itemID, err)
	}
	return &row, nil
}

// GetPortalCustomerEmailForItem returns the email of the portal customer who
// created the item, regardless of which channel it originated on. Returns
// ErrNotFound when the item does not exist or was not created by a portal
// customer. Channel-agnostic on purpose: email replies quoting an item's
// Message-IDs must resolve the creator even when they arrive on a different
// intake channel than the one that owns the ticket (WI-1546).
func (r *ItemRepository) GetPortalCustomerEmailForItem(itemID int) (string, error) {
	var email sql.NullString
	err := r.db.QueryRow(`
		SELECT pc.email
		FROM items i
		JOIN portal_customers pc ON pc.id = i.creator_portal_customer_id
		WHERE i.id = ?
	`, itemID).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !email.Valid) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get portal customer email for item: %w", err)
	}
	return email.String, nil
}

// --- Portal customer ticket lookups -----------------------------------------

// PortalCustomerSubmission is one row returned by
// ListPortalCustomerSubmissions — a ticket a portal customer created, with
// workspace + status metadata for display in the customer profile.
type PortalCustomerSubmission struct {
	ID                  int
	WorkspaceID         int
	WorkspaceItemNumber int
	WorkspaceName       string
	WorkspaceKey        string
	Title               string
	Description         string
	StatusName          string
	StatusColor         string
	CreatedAt           string
}

// ListPortalCustomerSubmissions returns all items created by the given portal
// customer, newest-first, hydrated with workspace name/key and status
// name/color (falling back to empty string / neutral color for NULLs).
func (r *ItemRepository) ListPortalCustomerSubmissions(customerID int) ([]PortalCustomerSubmission, error) {
	rows, err := r.db.Query(`
		SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
			COALESCE(s.name, ''), COALESCE(sc.color, '#6b7280'),
			i.created_at,
			w.name AS workspace_name,
			w.key AS workspace_key
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.creator_portal_customer_id = ? AND `+portalMergedDuplicateVisible+`
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
			&s.ID, &s.WorkspaceID, &s.WorkspaceItemNumber, &s.Title, &s.Description,
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
// belonging to the given customer_organisation_id. When workspaceIDs is nil
// the result is not workspace-scoped — the caller has already established
// that org-level ACLs authorize reading every ticket in the org. When
// workspaceIDs is non-nil but empty, no tickets match.
func (r *ItemRepository) ListOrganisationTickets(orgID int, workspaceIDs []int) ([]OrganisationTicket, error) {
	args := []any{orgID}
	workspaceClause := ""
	if workspaceIDs != nil {
		if len(workspaceIDs) == 0 {
			return []OrganisationTicket{}, nil
		}
		placeholders, wsArgs := inPlaceholders(workspaceIDs)
		workspaceClause = "  AND i.workspace_id IN (" + placeholders + ")\n"
		args = append(args, wsArgs...)
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
		WHERE pc.customer_organisation_id = ? AND ` + portalMergedDuplicateVisible + `
` + workspaceClause + `		ORDER BY i.created_at DESC`

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

// RequesterOpenTicketRow is one duplicate-candidate entry: an open ticket
// created by the same portal customer as another item.
type RequesterOpenTicketRow struct {
	ID                  int
	WorkspaceID         int
	WorkspaceKey        string
	WorkspaceItemNumber int
	Title               string
	StatusName          string
	UpdatedAt           time.Time
}

// RequesterCustomerID returns the portal customer who created the item, or
// nil when the item has no portal-customer creator.
func (r *ItemRepository) RequesterCustomerID(itemID int) (*int, error) {
	var customerID *int
	err := r.db.QueryRow(`SELECT creator_portal_customer_id FROM items WHERE id = ?`, itemID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get requester customer id: %w", err)
	}
	return customerID, nil
}

// RequesterOpenTickets lists the requester's other open tickets (status not
// in a completed category), newest activity first, across all workspaces —
// callers filter rows to workspaces the agent can view.
func (r *ItemRepository) RequesterOpenTickets(requesterCustomerID, excludeItemID, limit int) ([]RequesterOpenTicketRow, error) {
	rows, err := r.db.Query(`
		SELECT i.id, i.workspace_id, w.key, i.workspace_item_number, i.title,
		       COALESCE(s.name, ''), i.updated_at
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.creator_portal_customer_id = ?
		  AND i.merged_into_item_id IS NULL
		  AND i.id != ?
		  AND (sc.is_completed = false OR sc.is_completed IS NULL)
		ORDER BY i.updated_at DESC
		LIMIT ?
	`, requesterCustomerID, excludeItemID, limit)
	if err != nil {
		return nil, fmt.Errorf("list requester open tickets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []RequesterOpenTicketRow{}
	for rows.Next() {
		var row RequesterOpenTicketRow
		if err := rows.Scan(&row.ID, &row.WorkspaceID, &row.WorkspaceKey, &row.WorkspaceItemNumber, &row.Title, &row.StatusName, &row.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan requester open ticket: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
