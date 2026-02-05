package services

import (
	"database/sql"
	"fmt"

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
	var workspaceID int
	var creatorPortalCustomerID sql.NullInt64
	err := s.db.QueryRow(`
		SELECT workspace_id, creator_portal_customer_id
		FROM items WHERE id = ?
	`, itemID).Scan(&workspaceID, &creatorPortalCustomerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil // Item not found
		}
		return false, fmt.Errorf("failed to get item for permission check: %w", err)
	}

	// Portal customer: can only access their own items
	if portalCustomerID != nil {
		if creatorPortalCustomerID.Valid && int(creatorPortalCustomerID.Int64) == *portalCustomerID {
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

// GetAttachmentItemID returns the item_id and entity_type for an attachment.
// Returns (itemID, entityType, error). itemID may be nil for non-item attachments.
func (s *AttachmentService) GetAttachmentItemID(attachmentID int) (itemID *int, entityType string, err error) {
	var nullItemID sql.NullInt64
	err = s.db.QueryRow(`
		SELECT item_id, COALESCE(entity_type, 'item')
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(&nullItemID, &entityType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, "", fmt.Errorf("attachment not found")
		}
		return nil, "", fmt.Errorf("failed to get attachment: %w", err)
	}

	if nullItemID.Valid {
		id := int(nullItemID.Int64)
		return &id, entityType, nil
	}
	return nil, entityType, nil
}

// AttachmentDetails contains attachment info needed for deletion
type AttachmentDetails struct {
	FilePath         string
	ItemID           *int
	OriginalFilename string
	EntityType       string
}

// GetAttachmentDetails returns attachment details needed for deletion.
// Returns repository.ErrNotFound if the attachment doesn't exist.
func (s *AttachmentService) GetAttachmentDetails(attachmentID int) (*AttachmentDetails, error) {
	var filePath string
	var itemID sql.NullInt64
	var originalFilename string
	var entityType string

	err := s.db.QueryRow(`
		SELECT file_path, item_id, original_filename, COALESCE(entity_type, 'item')
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(&filePath, &itemID, &originalFilename, &entityType)
	if err != nil {
		if err == sql.ErrNoRows {
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
	// For avatars, item_id should be NULL
	var itemID interface{}
	if params.EntityType == "avatar" || params.EntityType == "workspace_avatar" || params.EntityType == "customer_avatar" {
		itemID = nil
	} else {
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
