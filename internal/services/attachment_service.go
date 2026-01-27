package services

import (
	"fmt"

	"windshift/internal/database"
)

// AttachmentService handles attachment record creation in the database.
// File I/O (download, disk write, thumbnails) remains in the handler/caller.
type AttachmentService struct {
	db database.Database
}

// NewAttachmentService creates a new AttachmentService.
func NewAttachmentService(db database.Database) *AttachmentService {
	return &AttachmentService{db: db}
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
