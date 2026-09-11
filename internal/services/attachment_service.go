package services

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// AttachmentService handles attachment record creation in the database.
// File I/O (download, disk write, thumbnails) remains in the handler/caller.
type AttachmentService struct {
	db                database.Database
	permissionService *PermissionService
}

// NewAttachmentService creates a new AttachmentService.
func NewAttachmentService(db database.Database) *AttachmentService {
	return &AttachmentService{db: db}
}

// NewAttachmentServiceWithPermissions creates a new AttachmentService with permission checking.
func NewAttachmentServiceWithPermissions(db database.Database, permService *PermissionService) *AttachmentService {
	return &AttachmentService{
		db:                db,
		permissionService: permService,
	}
}

// CreateAttachmentParams contains the parameters for inserting an attachment record.
type CreateAttachmentParams struct {
	ItemID           int
	EntityType       string // e.g. "item", "test_case", "avatar"
	Filename         string // stored filename (unique)
	OriginalFilename string
	FilePath         string
	MimeType         string
	FileSize         int64
	UploadedBy       *int
	HasThumbnail     bool
	ThumbnailPath    string
	Category         string // e.g. "avatar", "" for regular attachments
}

// CanModifyItemAttachment checks if a user can upload/delete attachments on an item.
// For internal users: requires item.edit permission in the workspace.
// For portal customers: can only modify attachments on items they created.
func (s *AttachmentService) CanModifyItemAttachment(userID, portalCustomerID *int, itemID int) (bool, error) {
	// Get item's workspace_id and creator_portal_customer_id
	item, err := repository.NewItemRepository(s.db).FindByID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil // Item not found
		}
		return false, fmt.Errorf("failed to get item for permission check: %w", err)
	}
	workspaceID := item.WorkspaceID

	// Portal customer: can only access their own items
	if portalCustomerID != nil {
		if item.CreatorPortalCustomerID != nil && *item.CreatorPortalCustomerID == *portalCustomerID {
			return true, nil
		}
		return false, nil
	}

	// Internal user: check item.edit permission in the workspace
	if userID != nil && s.permissionService != nil {
		canEdit, err := s.permissionService.HasWorkspacePermission(*userID, workspaceID, models.PermissionItemEdit)
		if err != nil {
			return false, fmt.Errorf("failed to check workspace permission: %w", err)
		}
		return canEdit, nil
	}

	// If no permission service, allow (backwards compatibility)
	if userID != nil {
		return true, nil
	}

	return false, nil
}

func (s *AttachmentService) CanViewItemAttachment(userID, itemID int) (bool, error) {
	item, err := repository.NewItemRepository(s.db).FindByID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	if s.permissionService == nil {
		return true, nil
	}
	return s.permissionService.HasWorkspacePermission(userID, item.WorkspaceID, models.PermissionItemView)
}

// AttachmentDetails contains attachment info needed for deletion
type AttachmentDetails struct {
	FilePath         string
	ItemID           *int
	OriginalFilename string
	EntityType       string
	UploadedBy       *int
}

// GetAttachmentDetails returns attachment details needed for deletion.
// Returns repository.ErrNotFound if the attachment doesn't exist.
func (s *AttachmentService) GetAttachmentDetails(attachmentID int) (*AttachmentDetails, error) {
	var filePath string
	var itemID sql.NullInt64
	var originalFilename string
	var entityType string
	var uploadedBy sql.NullInt64

	err := s.db.QueryRow(`
		SELECT file_path, item_id, original_filename, COALESCE(entity_type, 'item'), uploaded_by
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(&filePath, &itemID, &originalFilename, &entityType, &uploadedBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, repository.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get attachment details: %w", err)
	}

	details := &AttachmentDetails{
		FilePath:         filePath,
		OriginalFilename: originalFilename,
		EntityType:       entityType,
	}

	if itemID.Valid {
		id := int(itemID.Int64)
		details.ItemID = &id
	}
	if uploadedBy.Valid {
		uid := int(uploadedBy.Int64)
		details.UploadedBy = &uid
	}

	return details, nil
}

// DeleteRecord deletes an attachment record from the database.
// Returns the number of rows affected.
func (s *AttachmentService) DeleteRecord(attachmentID int) (int64, error) {
	result, err := s.db.ExecWrite("DELETE FROM attachments WHERE id = ?", attachmentID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete attachment record: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to verify deletion: %w", err)
	}

	return rowsAffected, nil
}

// CreateRecord inserts a new attachment row and returns the new attachment ID.
func (s *AttachmentService) CreateRecord(params CreateAttachmentParams) (int64, error) {
	// item_id is polymorphic by entity_type. For non-item entity types the
	// real "owner" lives on a different table (workspaces, teams, customers,
	// portals, hubs, users) and must be stored as NULL here — otherwise a
	// numeric collision with a real item id can leak the row via the
	// items-scoped GetByItem query. See WI-46.
	var itemID any
	switch params.EntityType {
	case "avatar",
		"workspace_avatar", "workspace_background",
		"team_avatar", "customer_avatar",
		"portal_background", "portal_logo", "hub_logo":
		itemID = nil
	default:
		itemID = params.ItemID
	}

	var attachmentID int64
	err := s.db.QueryRow(`
		INSERT INTO attachments (item_id, entity_type, filename, original_filename, file_path, mime_type, file_size, uploaded_by, has_thumbnail, thumbnail_path, category)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, itemID, params.EntityType, params.Filename, params.OriginalFilename, params.FilePath,
		params.MimeType, params.FileSize, params.UploadedBy,
		params.HasThumbnail, params.ThumbnailPath, params.Category).Scan(&attachmentID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert attachment record: %w", err)
	}

	return attachmentID, nil
}

// RecordItemHistory appends an item_history row for an attachment lifecycle
// event (upload/delete). It is the single source of truth for attachment
// history shared by the cookie-auth handler and the bearer-token v1 item
// service, so both surfaces emit identical entries. Recording is best-effort
// and non-transactional: callers log and swallow the error so it never fails
// an otherwise-successful upload or delete. A nil userID is a no-op (no actor
// to attribute). new_value encodes the attachment id for uploads
// ("attachment:<id>:<filename>") and the bare filename otherwise, matching the
// format the activity feed already consumes.
func (s *AttachmentService) RecordItemHistory(itemID int, userID *int, action string, oldValue *string, attachmentID int64, filename string) error {
	if userID == nil {
		return nil
	}
	value := filename
	if action == "attachment_uploaded" {
		value = fmt.Sprintf("attachment:%d:%s", attachmentID, filename)
	}
	entry := repository.HistoryEntry{
		ItemID:       itemID,
		UserID:       *userID,
		FieldName:    action,
		OldValueNull: oldValue == nil,
		NewValue:     value,
		ChangedAt:    time.Now(),
	}
	if oldValue != nil {
		entry.OldValue = *oldValue
	}
	return repository.NewItemRepository(s.db).RecordHistory(s.db, entry)
}
