package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
)

// CustomerDataExportSchemaVersion pins the export payload structure so
// repeated DSAR exports are comparable across executions. Bump on any
// field-level change.
const CustomerDataExportSchemaVersion = 2

// CustomerDataExport is the Article 15/20 export payload: every personal-data
// category Windshift holds about one portal customer. Sections are ordered
// deterministically (every query orders by id) and empty sections marshal as
// [] so repeated exports diff cleanly. Credential material is never exported:
// sessions contribute metadata only (created_at, ip, user agent), never
// tokens, and attachments contribute metadata only, never file contents.
type CustomerDataExport struct {
	SchemaVersion     int                                `json:"schema_version"`
	ExportedAt        string                             `json:"exported_at"`
	Customer          CustomerExportProfile              `json:"customer"`
	RequestedItems    []CustomerExportItem               `json:"requested_items"`
	Drafts            []CustomerExportDraft              `json:"drafts"`
	Comments          []CustomerExportComment            `json:"comments"`
	ApprovalDecisions []CustomerExportApprovalDecision   `json:"approval_decisions"`
	Attachments       []CustomerExportAttachmentMetadata `json:"attachments"`
	EmailTracking     []CustomerExportEmailTracking      `json:"email_tracking"`
	Sessions          []CustomerExportSessionMetadata    `json:"sessions"`
	KBEvents          []CustomerExportKBEvent            `json:"kb_events"`
}

type CustomerExportProfile struct {
	ID                int             `json:"id"`
	Name              string          `json:"name"`
	Email             string          `json:"email"`
	Phone             *string         `json:"phone"`
	CustomFieldValues json.RawMessage `json:"custom_field_values"`
	Organisation      *string         `json:"organisation"` //nolint:misspell // British spelling used throughout codebase
	Roles             []string        `json:"roles"`
	IsPrimary         bool            `json:"is_primary"`
	CreatedAt         *string         `json:"created_at"`
}

type CustomerExportItem struct {
	ID                int             `json:"id"`
	ItemKey           string          `json:"item_key"`
	Title             string          `json:"title"`
	Description       string          `json:"description"`
	Status            *string         `json:"status"`
	WorkspaceKey      string          `json:"workspace_key"`
	ItemNumber        int             `json:"item_number"`
	CustomFieldValues json.RawMessage `json:"custom_field_values"`
	CreatedAt         *string         `json:"created_at"`
}

// CustomerExportDraft is one unfinished portal submission: the personal data a
// customer entered but has not yet submitted.
type CustomerExportDraft struct {
	ID                int             `json:"id"`
	RequestTypeID     int             `json:"request_type_id"`
	Title             string          `json:"title"`
	Description       string          `json:"description"`
	CustomFieldValues json.RawMessage `json:"custom_field_values"`
	CurrentStep       int             `json:"current_step"`
	CreatedAt         *string         `json:"created_at"`
	UpdatedAt         *string         `json:"updated_at"`
}

type CustomerExportComment struct {
	ID        int     `json:"id"`
	ItemID    int     `json:"item_id"`
	Content   string  `json:"content"`
	IsPrivate bool    `json:"is_private"`
	CreatedAt *string `json:"created_at"`
}

type CustomerExportApprovalDecision struct {
	ID                int     `json:"id"`
	ApprovalRequestID int     `json:"approval_request_id"`
	Decision          string  `json:"decision"`
	Comment           *string `json:"comment"`
	CreatedAt         *string `json:"created_at"`
}

type CustomerExportAttachmentMetadata struct {
	ID               int     `json:"id"`
	ItemID           *int    `json:"item_id"`
	Filename         string  `json:"filename"`
	OriginalFilename string  `json:"original_filename"`
	MimeType         string  `json:"mime_type"`
	FileSize         int64   `json:"file_size"`
	CreatedAt        *string `json:"created_at"`
}

type CustomerExportEmailTracking struct {
	ID          int     `json:"id"`
	ChannelID   int     `json:"channel_id"`
	MessageID   string  `json:"message_id"`
	FromEmail   string  `json:"from_email"`
	FromName    *string `json:"from_name"`
	Subject     *string `json:"subject"`
	Direction   *string `json:"direction"`
	ProcessedAt *string `json:"processed_at"`
}

type CustomerExportSessionMetadata struct {
	ID        int     `json:"id"`
	CreatedAt *string `json:"created_at"`
	ExpiresAt *string `json:"expires_at"`
	IPAddress *string `json:"ip_address"`
	UserAgent *string `json:"user_agent"`
}

type CustomerExportKBEvent struct {
	ID        int     `json:"id"`
	ChannelID int     `json:"channel_id"`
	EventType string  `json:"event_type"`
	PageID    *int    `json:"page_id"`
	Source    *string `json:"source"`
	Query     *string `json:"query"`
	CreatedAt *string `json:"created_at"`
}

// ExportCustomerData assembles the personal-data payload for one portal
// customer and records the disclosure in the audit trail (who exported which
// customer, when, and how much was disclosed).
//
// workspaceIDs limits every workspace-scoped section (requested items, drafts,
// comments, approval decisions, attachments) to workspaces the caller may
// read, so an export never discloses content the caller cannot view. nil means
// unscoped; a non-nil empty slice discloses no workspace-scoped rows. Sections
// that are not workspace-bound (email tracking, sessions, KB events) are
// always included.
func ExportCustomerData(db database.Database, customerID int, actor AuditActor, workspaceIDs []int) (CustomerDataExport, error) {
	export := CustomerDataExport{
		SchemaVersion:     CustomerDataExportSchemaVersion,
		ExportedAt:        time.Now().UTC().Format(time.RFC3339),
		RequestedItems:    []CustomerExportItem{},
		Drafts:            []CustomerExportDraft{},
		Comments:          []CustomerExportComment{},
		ApprovalDecisions: []CustomerExportApprovalDecision{},
		Attachments:       []CustomerExportAttachmentMetadata{},
		EmailTracking:     []CustomerExportEmailTracking{},
		Sessions:          []CustomerExportSessionMetadata{},
		KBEvents:          []CustomerExportKBEvent{},
	}

	// wsFilter builds the workspace-visibility predicate for a query column.
	// A nil scope leaves the query unscoped; an empty scope matches nothing.
	wsFilter := func(column string) (string, []any) {
		if workspaceIDs == nil {
			return "", nil
		}
		if len(workspaceIDs) == 0 {
			return " AND 1 = 0", nil
		}
		placeholders, args := inArgsInt(workspaceIDs)
		return " AND " + column + " IN (" + placeholders + ")", args
	}
	// wsFilterNullable keeps rows whose workspace link is NULL (not
	// workspace-bound) alongside rows in the caller's workspaces.
	wsFilterNullable := func(column string) (string, []any) {
		if workspaceIDs == nil {
			return "", nil
		}
		if len(workspaceIDs) == 0 {
			return " AND " + column + " IS NULL", nil
		}
		placeholders, args := inArgsInt(workspaceIDs)
		return " AND (" + column + " IS NULL OR " + column + " IN (" + placeholders + "))", args
	}

	// Profile. The export is meaningless for an unknown customer.
	var customFields, createdAt sql.NullString
	var orgName sql.NullString
	if err := db.QueryRow(`
		SELECT pc.id, pc.name, pc.email, pc.phone, pc.custom_field_values, pc.is_primary,
		       pc.created_at, COALESCE(co.name, '')
		FROM portal_customers pc
		LEFT JOIN customer_organisations co ON co.id = pc.customer_organisation_id
		WHERE pc.id = ?
	`, customerID).Scan(
		&export.Customer.ID, &export.Customer.Name, &export.Customer.Email, &export.Customer.Phone,
		&customFields, &export.Customer.IsPrimary, &createdAt, &orgName,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return export, NewServiceError(404, "Portal customer not found")
		}
		return export, fmt.Errorf("load customer profile: %w", err)
	}
	if customFields.Valid && json.Valid([]byte(customFields.String)) && customFields.String != "" {
		export.Customer.CustomFieldValues = json.RawMessage(customFields.String)
	}
	if orgName.Valid && orgName.String != "" {
		export.Customer.Organisation = &orgName.String //nolint:misspell // British spelling used throughout codebase
	}
	if createdAt.Valid {
		export.Customer.CreatedAt = &createdAt.String
	}

	roles := []string{}
	roleRows, err := db.Query(`
		SELECT cr.name FROM portal_customer_roles pcr
		JOIN contact_roles cr ON cr.id = pcr.contact_role_id
		WHERE pcr.portal_customer_id = ? ORDER BY pcr.id
	`, customerID)
	if err != nil {
		return export, fmt.Errorf("load customer roles: %w", err)
	}
	defer func() { _ = roleRows.Close() }()
	for roleRows.Next() {
		var role string
		if err := roleRows.Scan(&role); err != nil {
			return export, fmt.Errorf("scan customer role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := roleRows.Err(); err != nil {
		return export, fmt.Errorf("iterate customer roles: %w", err)
	}
	export.Customer.Roles = roles

	// Requested items with display keys, status names, submitted bodies and
	// custom-field values.
	itemClause, itemArgs := wsFilter("i.workspace_id")
	itemRows, err := db.Query(`
		SELECT i.id, COALESCE(w.key, ''), i.workspace_item_number, i.title, COALESCE(i.description, ''),
		       i.custom_field_values, COALESCE(s.name, ''), i.created_at
		FROM items i
		JOIN workspaces w ON w.id = i.workspace_id
		LEFT JOIN statuses s ON i.status_id = s.id
		WHERE i.creator_portal_customer_id = ?`+itemClause+` ORDER BY i.id
	`, append([]any{customerID}, itemArgs...)...)
	if err != nil {
		return export, fmt.Errorf("load requested items: %w", err)
	}
	defer func() { _ = itemRows.Close() }()
	for itemRows.Next() {
		var item CustomerExportItem
		var createdAt string
		var customFields sql.NullString
		if err := itemRows.Scan(&item.ID, &item.WorkspaceKey, &item.ItemNumber, &item.Title, &item.Description, &customFields, &item.Status, &createdAt); err != nil {
			return export, fmt.Errorf("scan requested item: %w", err)
		}
		if item.WorkspaceKey != "" {
			item.ItemKey = fmt.Sprintf("%s-%d", item.WorkspaceKey, item.ItemNumber)
		}
		if customFields.Valid && customFields.String != "" && json.Valid([]byte(customFields.String)) {
			item.CustomFieldValues = json.RawMessage(customFields.String)
		}
		item.CreatedAt = &createdAt
		export.RequestedItems = append(export.RequestedItems, item)
	}
	if err := itemRows.Err(); err != nil {
		return export, fmt.Errorf("iterate requested items: %w", err)
	}

	// Unfinished submissions the customer saved as drafts. Drafts are the
	// customer's own personal data, so they are disclosed alongside submitted
	// requests. A draft whose request type is not bound to a workspace is not
	// workspace-scoped and stays visible.
	draftClause, draftArgs := wsFilterNullable("rt.workspace_id")
	draftRows, err := db.Query(`
		SELECT d.id, d.request_type_id, d.title, d.description, d.custom_field_values, d.current_step, d.created_at, d.updated_at
		FROM portal_request_drafts d
		LEFT JOIN request_types rt ON rt.id = d.request_type_id
		WHERE d.portal_customer_id = ?`+draftClause+` ORDER BY d.id
	`, append([]any{customerID}, draftArgs...)...)
	if err != nil {
		return export, fmt.Errorf("load drafts: %w", err)
	}
	defer func() { _ = draftRows.Close() }()
	for draftRows.Next() {
		var draft CustomerExportDraft
		var customFields sql.NullString
		var createdAt, updatedAt string
		if err := draftRows.Scan(&draft.ID, &draft.RequestTypeID, &draft.Title, &draft.Description, &customFields, &draft.CurrentStep, &createdAt, &updatedAt); err != nil {
			return export, fmt.Errorf("scan draft: %w", err)
		}
		if customFields.Valid && customFields.String != "" && json.Valid([]byte(customFields.String)) {
			draft.CustomFieldValues = json.RawMessage(customFields.String)
		}
		draft.CreatedAt = &createdAt
		draft.UpdatedAt = &updatedAt
		export.Drafts = append(export.Drafts, draft)
	}
	if err := draftRows.Err(); err != nil {
		return export, fmt.Errorf("iterate drafts: %w", err)
	}

	commentClause, commentArgs := wsFilter("i.workspace_id")
	commentRows, err := db.Query(`
		SELECT c.id, c.item_id, c.content, c.is_private, c.created_at
		FROM comments c
		JOIN items i ON i.id = c.item_id
		WHERE c.portal_customer_id = ?`+commentClause+` ORDER BY c.id
	`, append([]any{customerID}, commentArgs...)...)
	if err != nil {
		return export, fmt.Errorf("load comments: %w", err)
	}
	defer func() { _ = commentRows.Close() }()
	for commentRows.Next() {
		var c CustomerExportComment
		var createdAt string
		if err := commentRows.Scan(&c.ID, &c.ItemID, &c.Content, &c.IsPrivate, &createdAt); err != nil {
			return export, fmt.Errorf("scan comment: %w", err)
		}
		c.CreatedAt = &createdAt
		export.Comments = append(export.Comments, c)
	}
	if err := commentRows.Err(); err != nil {
		return export, fmt.Errorf("iterate comments: %w", err)
	}

	decisionClause, decisionArgs := wsFilter("i.workspace_id")
	decisionRows, err := db.Query(`
		SELECT d.id, d.approval_request_id, d.decision, d.comment, d.created_at
		FROM approval_decisions d
		JOIN approval_requests ar ON ar.id = d.approval_request_id
		JOIN items i ON i.id = ar.item_id
		WHERE d.actor_portal_customer_id = ?`+decisionClause+` ORDER BY d.id
	`, append([]any{customerID}, decisionArgs...)...)
	if err != nil {
		return export, fmt.Errorf("load approval decisions: %w", err)
	}
	defer func() { _ = decisionRows.Close() }()
	for decisionRows.Next() {
		var d CustomerExportApprovalDecision
		var createdAt string
		if err := decisionRows.Scan(&d.ID, &d.ApprovalRequestID, &d.Decision, &d.Comment, &createdAt); err != nil {
			return export, fmt.Errorf("scan approval decision: %w", err)
		}
		d.CreatedAt = &createdAt
		export.ApprovalDecisions = append(export.ApprovalDecisions, d)
	}
	if err := decisionRows.Err(); err != nil {
		return export, fmt.Errorf("iterate approval decisions: %w", err)
	}

	// Attachments without an item link (for example avatars) are not
	// workspace-bound and stay visible; item attachments follow item visibility.
	attachmentClause, attachmentArgs := wsFilterNullable("i.workspace_id")
	attachmentRows, err := db.Query(`
		SELECT a.id, a.item_id, a.filename, a.original_filename, a.mime_type, a.file_size, a.created_at
		FROM attachments a
		LEFT JOIN items i ON i.id = a.item_id AND a.entity_type = 'item'
		WHERE a.uploaded_by_portal_customer_id = ?`+attachmentClause+` ORDER BY a.id
	`, append([]any{customerID}, attachmentArgs...)...)
	if err != nil {
		return export, fmt.Errorf("load attachment metadata: %w", err)
	}
	defer func() { _ = attachmentRows.Close() }()
	for attachmentRows.Next() {
		var a CustomerExportAttachmentMetadata
		var createdAt string
		if err := attachmentRows.Scan(&a.ID, &a.ItemID, &a.Filename, &a.OriginalFilename, &a.MimeType, &a.FileSize, &createdAt); err != nil {
			return export, fmt.Errorf("scan attachment metadata: %w", err)
		}
		a.CreatedAt = &createdAt
		export.Attachments = append(export.Attachments, a)
	}
	if err := attachmentRows.Err(); err != nil {
		return export, fmt.Errorf("iterate attachment metadata: %w", err)
	}

	// The tracking address is stored as it arrived in the From header, which
	// may differ in case from the profile email, so match case-insensitively.
	trackingRows, err := db.Query(`
		SELECT id, channel_id, message_id, from_email, from_name, subject, direction, processed_at
		FROM email_message_tracking WHERE UPPER(from_email) = UPPER(?) ORDER BY id
	`, export.Customer.Email)
	if err != nil {
		return export, fmt.Errorf("load email tracking: %w", err)
	}
	defer func() { _ = trackingRows.Close() }()
	for trackingRows.Next() {
		var t CustomerExportEmailTracking
		var processedAt string
		if err := trackingRows.Scan(&t.ID, &t.ChannelID, &t.MessageID, &t.FromEmail, &t.FromName, &t.Subject, &t.Direction, &processedAt); err != nil {
			return export, fmt.Errorf("scan email tracking: %w", err)
		}
		t.ProcessedAt = &processedAt
		export.EmailTracking = append(export.EmailTracking, t)
	}
	if err := trackingRows.Err(); err != nil {
		return export, fmt.Errorf("iterate email tracking: %w", err)
	}

	sessionRows, err := db.Query(`
		SELECT id, created_at, expires_at, ip_address, user_agent FROM portal_customer_sessions
		WHERE portal_customer_id = ? ORDER BY id
	`, customerID)
	if err != nil {
		return export, fmt.Errorf("load session metadata: %w", err)
	}
	defer func() { _ = sessionRows.Close() }()
	for sessionRows.Next() {
		var s CustomerExportSessionMetadata
		var createdAt, expiresAt string
		if err := sessionRows.Scan(&s.ID, &createdAt, &expiresAt, &s.IPAddress, &s.UserAgent); err != nil {
			return export, fmt.Errorf("scan session metadata: %w", err)
		}
		s.CreatedAt = &createdAt
		s.ExpiresAt = &expiresAt
		export.Sessions = append(export.Sessions, s)
	}
	if err := sessionRows.Err(); err != nil {
		return export, fmt.Errorf("iterate session metadata: %w", err)
	}

	kbRows, err := db.Query(`
		SELECT id, channel_id, event_type, page_id, source, query, created_at FROM kb_events
		WHERE portal_customer_id = ? ORDER BY id
	`, customerID)
	if err != nil {
		return export, fmt.Errorf("load kb events: %w", err)
	}
	defer func() { _ = kbRows.Close() }()
	for kbRows.Next() {
		var e CustomerExportKBEvent
		var createdAt string
		if err := kbRows.Scan(&e.ID, &e.ChannelID, &e.EventType, &e.PageID, &e.Source, &e.Query, &createdAt); err != nil {
			return export, fmt.Errorf("scan kb event: %w", err)
		}
		e.CreatedAt = &createdAt
		export.KBEvents = append(export.KBEvents, e)
	}
	if err := kbRows.Err(); err != nil {
		return export, fmt.Errorf("iterate kb events: %w", err)
	}

	// The disclosure itself is audit-relevant: record who exported which
	// customer, when, and the section sizes disclosed.
	_ = logger.LogAudit(db, logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		ActionType:   logger.ActionPortalCustomerDataExport,
		ResourceType: logger.ResourcePortalCustomer,
		ResourceID:   &customerID,
		Success:      true,
		Details: map[string]any{
			"schema_version":     CustomerDataExportSchemaVersion,
			"customer_email":     export.Customer.Email,
			"requested_items":    len(export.RequestedItems),
			"comments":           len(export.Comments),
			"approval_decisions": len(export.ApprovalDecisions),
			"attachments":        len(export.Attachments),
			"email_tracking":     len(export.EmailTracking),
			"sessions":           len(export.Sessions),
			"kb_events":          len(export.KBEvents),
		},
	})

	return export, nil
}
