package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// AttachmentDownloadRecord is the persistence projection needed to authorize
// and stream an item-scoped attachment download.
type AttachmentDownloadRecord struct {
	WorkspaceID      int
	Filename         string
	OriginalFilename string
	FilePath         string
	MimeType         string
	FileSize         int64
}

// AttachmentRepository owns attachment metadata lookups.
type AttachmentRepository struct {
	db database.Database
}

// ListItem returns a stable, newest-first page of item attachment metadata.
func (r *AttachmentRepository) ListItem(itemID, limit, offset int) ([]models.Attachment, int, error) {
	return r.listItem(itemID, limit, offset, false)
}

// ListPortalItem returns the customer-visible attachment page for one portal
// request. Internal attachments (for example files attached to agent-only JSM
// notes) are excluded so they never surface in the portal.
func (r *AttachmentRepository) ListPortalItem(itemID, limit, offset int) ([]models.Attachment, int, error) {
	return r.listItem(itemID, limit, offset, true)
}

func (r *AttachmentRepository) listItem(itemID, limit, offset int, portalOnly bool) ([]models.Attachment, int, error) {
	internalFilter := ""
	if portalOnly {
		internalFilter = " AND COALESCE(is_internal, false) = false"
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM attachments WHERE item_id = ? AND COALESCE(entity_type, 'item') = 'item'`+internalFilter, itemID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count item attachments: %w", err)
	}
	rows, err := r.db.Query(`
		SELECT a.id, a.item_id, a.filename, a.original_filename, a.mime_type, a.file_size,
		       a.uploaded_by, a.uploaded_by_portal_customer_id, a.has_thumbnail, a.created_at,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.username, ''),
		       COALESCE(u.email, ''),
		       COALESCE(NULLIF(TRIM(pc.name), ''), pc.email, '')
		FROM attachments a
		LEFT JOIN users u ON u.id = a.uploaded_by
		LEFT JOIN portal_customers pc ON pc.id = a.uploaded_by_portal_customer_id
		WHERE a.item_id = ? AND COALESCE(a.entity_type, 'item') = 'item'`+internalFilter+`
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT ? OFFSET ?
	`, itemID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list item attachments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	result := make([]models.Attachment, 0, limit)
	for rows.Next() {
		var attachment models.Attachment
		var rowItemID, uploadedBy, uploadedByCustomer sql.NullInt64
		if err := rows.Scan(&attachment.ID, &rowItemID, &attachment.Filename, &attachment.OriginalFilename,
			&attachment.MimeType, &attachment.FileSize, &uploadedBy, &uploadedByCustomer, &attachment.HasThumbnail,
			&attachment.CreatedAt, &attachment.UploaderName, &attachment.UploaderEmail,
			&attachment.UploaderPortalCustomerName); err != nil {
			return nil, 0, fmt.Errorf("scan item attachment: %w", err)
		}
		if rowItemID.Valid {
			id := int(rowItemID.Int64)
			attachment.ItemID = &id
		}
		if uploadedBy.Valid {
			id := int(uploadedBy.Int64)
			attachment.UploadedBy = &id
		}
		if uploadedByCustomer.Valid {
			id := int(uploadedByCustomer.Int64)
			attachment.UploadedByPortalCustomerID = &id
		}
		result = append(result, attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate item attachments: %w", err)
	}
	return result, total, nil
}

func (r *AttachmentRepository) GetItem(id int) (*models.Attachment, error) {
	var attachment models.Attachment
	var itemID, uploadedBy, uploadedByCustomer sql.NullInt64
	err := r.db.QueryRow(`
		SELECT a.id, a.item_id, a.filename, a.original_filename, a.mime_type, a.file_size,
		       a.uploaded_by, a.uploaded_by_portal_customer_id, a.has_thumbnail, a.created_at,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.username, ''),
		       COALESCE(u.email, ''),
		       COALESCE(NULLIF(TRIM(pc.name), ''), pc.email, '')
		FROM attachments a
		LEFT JOIN users u ON u.id = a.uploaded_by
		LEFT JOIN portal_customers pc ON pc.id = a.uploaded_by_portal_customer_id
		WHERE a.id = ? AND a.item_id IS NOT NULL AND COALESCE(a.entity_type, 'item') = 'item'
	`, id).Scan(
		&attachment.ID, &itemID, &attachment.Filename, &attachment.OriginalFilename,
		&attachment.MimeType, &attachment.FileSize, &uploadedBy, &uploadedByCustomer, &attachment.HasThumbnail,
		&attachment.CreatedAt, &attachment.UploaderName, &attachment.UploaderEmail,
		&attachment.UploaderPortalCustomerName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get item attachment: %w", err)
	}
	item := int(itemID.Int64)
	attachment.ItemID = &item
	if uploadedBy.Valid {
		uploader := int(uploadedBy.Int64)
		attachment.UploadedBy = &uploader
	}
	if uploadedByCustomer.Valid {
		id := int(uploadedByCustomer.Int64)
		attachment.UploadedByPortalCustomerID = &id
	}
	return &attachment, nil
}

// AttachmentThumbnailRecord is the persistence projection needed to authorize
// and stream an item-scoped attachment thumbnail.
type AttachmentThumbnailRecord struct {
	WorkspaceID   int
	ThumbnailPath string
	HasThumbnail  bool
}

// PageAttachmentRecord is the persistence projection used by PageDiagram and
// Page-scoped bearer reads. PageID and WorkspaceID are selected through the
// owning page so callers cannot confuse a numerically colliding item row.
type PageAttachmentRecord struct {
	ID               int
	PageID           int
	WorkspaceID      int
	Filename         string
	OriginalFilename string
	FilePath         string
	ThumbnailPath    string
	MimeType         string
	FileSize         int64
	UploadedBy       *int
	CreatedAt        time.Time
}

type CaptureAttachment struct {
	ID               int
	OriginalFilename string
	MimeType         string
	FileSize         int64
	UploaderUsername string
}

// ListCaptureAttachments returns mapped item attachments for export verification.
func (r *AttachmentRepository) ListCaptureAttachments(itemID int, attachmentIDs []int) ([]CaptureAttachment, error) {
	if len(attachmentIDs) == 0 {
		return []CaptureAttachment{}, nil
	}
	placeholders, idArgs := inPlaceholders(attachmentIDs)
	args := append([]any{itemID}, idArgs...)
	rows, err := r.db.Query(`
		SELECT a.id, COALESCE(a.original_filename, ''), COALESCE(a.mime_type, ''),
		       COALESCE(a.file_size, 0), COALESCE(u.username, '')
		FROM attachments a
		LEFT JOIN users u ON u.id = a.uploaded_by
		WHERE a.item_id = ? AND a.entity_type = 'item' AND a.id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list capture attachments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []CaptureAttachment{}
	for rows.Next() {
		var attachment CaptureAttachment
		if err := rows.Scan(&attachment.ID, &attachment.OriginalFilename, &attachment.MimeType, &attachment.FileSize, &attachment.UploaderUsername); err != nil {
			return nil, fmt.Errorf("scan capture attachment: %w", err)
		}
		out = append(out, attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate capture attachments: %w", err)
	}
	return out, nil
}

// NewAttachmentRepository creates an attachment repository.
func NewAttachmentRepository(db database.Database) *AttachmentRepository {
	return &AttachmentRepository{db: db}
}

// GetItemDownloadRecord returns download metadata for an item-scoped
// attachment. Non-item attachments are deliberately hidden from the items-token
// route and return ErrNotFound.
func (r *AttachmentRepository) GetItemDownloadRecord(id int) (*AttachmentDownloadRecord, error) {
	var rec AttachmentDownloadRecord
	err := r.db.QueryRow(`
		SELECT i.workspace_id, a.filename, a.original_filename, a.file_path, a.mime_type, a.file_size
		FROM attachments a
		JOIN items i ON i.id = a.item_id
		WHERE a.id = ?
		  AND a.item_id IS NOT NULL
		  AND (a.entity_type IS NULL OR a.entity_type = '' OR a.entity_type = 'item')
	`, id).Scan(&rec.WorkspaceID, &rec.Filename, &rec.OriginalFilename, &rec.FilePath, &rec.MimeType, &rec.FileSize)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get attachment download: %w", err)
	}
	return &rec, nil
}

// GetPortalRequestAttachmentRecord returns download metadata for an
// item-scoped attachment bound to one specific request (item). The itemID
// comes from the caller's portal ownership resolution, so the query refuses
// attachments that belong to any other item or entity type.
func (r *AttachmentRepository) GetPortalRequestAttachmentRecord(attachmentID, itemID int) (*AttachmentDownloadRecord, error) {
	var rec AttachmentDownloadRecord
	err := r.db.QueryRow(`
		SELECT i.workspace_id, a.filename, a.original_filename, a.file_path, a.mime_type, a.file_size
		FROM attachments a
		JOIN items i ON i.id = a.item_id
		WHERE a.id = ? AND a.item_id = ?
		  AND (a.entity_type IS NULL OR a.entity_type = '' OR a.entity_type = 'item')
		  AND COALESCE(a.is_internal, false) = false
	`, attachmentID, itemID).Scan(&rec.WorkspaceID, &rec.Filename, &rec.OriginalFilename, &rec.FilePath, &rec.MimeType, &rec.FileSize)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get portal request attachment: %w", err)
	}
	return &rec, nil
}

// GetItemThumbnailRecord returns thumbnail metadata for an item-scoped
// attachment. Missing thumbnails and non-item attachments are deliberately
// hidden from the items-token route and return ErrNotFound.
func (r *AttachmentRepository) GetItemThumbnailRecord(id int) (*AttachmentThumbnailRecord, error) {
	var rec AttachmentThumbnailRecord
	err := r.db.QueryRow(`
		SELECT i.workspace_id, a.has_thumbnail, a.thumbnail_path
		FROM attachments a
		JOIN items i ON i.id = a.item_id
		WHERE a.id = ?
		  AND a.item_id IS NOT NULL
		  AND (a.entity_type IS NULL OR a.entity_type = '' OR a.entity_type = 'item')
		  AND a.has_thumbnail = true
		  AND a.thumbnail_path IS NOT NULL
		  AND a.thumbnail_path <> ''
	`, id).Scan(&rec.WorkspaceID, &rec.HasThumbnail, &rec.ThumbnailPath)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get attachment thumbnail: %w", err)
	}
	return &rec, nil
}

// pageAttachmentSelect resolves a page-scoped attachment through its owning
// page so callers receive the page and workspace ids needed for authorization.
const pageAttachmentSelect = `
	SELECT a.id, p.id, p.workspace_id, a.filename, a.original_filename,
	       a.file_path, COALESCE(a.thumbnail_path, ''), a.mime_type, a.file_size, a.uploaded_by, a.created_at
	FROM attachments a
	JOIN pages p ON p.id = a.item_id
`

func scanPageAttachmentRecord(row interface{ Scan(dest ...any) error }) (*PageAttachmentRecord, error) {
	var rec PageAttachmentRecord
	var uploadedBy sql.NullInt64
	if err := row.Scan(
		&rec.ID, &rec.PageID, &rec.WorkspaceID, &rec.Filename,
		&rec.OriginalFilename, &rec.FilePath, &rec.ThumbnailPath, &rec.MimeType, &rec.FileSize,
		&uploadedBy, &rec.CreatedAt,
	); err != nil {
		return nil, err
	}
	if uploadedBy.Valid {
		id := int(uploadedBy.Int64)
		rec.UploadedBy = &id
	}
	return &rec, nil
}

// GetPageAttachmentRecord returns attachment metadata only when the row is
// explicitly page-scoped and owned by the requested Page.
func (r *AttachmentRepository) GetPageAttachmentRecord(pageID, attachmentID int) (*PageAttachmentRecord, error) {
	rec, err := scanPageAttachmentRecord(r.db.QueryRow(pageAttachmentSelect+`
		WHERE a.id = ?
		  AND p.id = ?
		  AND a.entity_type = 'page'
	`, attachmentID, pageID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get page attachment %d for page %d: %w", attachmentID, pageID, err)
	}
	return rec, nil
}

// GetPageAttachmentRecordByID returns a page-scoped attachment by its own id,
// resolving the owning page and workspace so the caller can enforce page-view
// permission before streaming the blob.
func (r *AttachmentRepository) GetPageAttachmentRecordByID(attachmentID int) (*PageAttachmentRecord, error) {
	rec, err := scanPageAttachmentRecord(r.db.QueryRow(pageAttachmentSelect+`
		WHERE a.id = ?
		  AND a.entity_type = 'page'
	`, attachmentID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get page attachment %d: %w", attachmentID, err)
	}
	return rec, nil
}

// UpdateCreatedAt preserves the source timestamp for an imported attachment.
func (r *AttachmentRepository) UpdateCreatedAt(attachmentID int64, createdAt time.Time) error {
	result, err := r.db.ExecWrite(
		"UPDATE attachments SET created_at = ? WHERE id = ?",
		createdAt,
		attachmentID,
	)
	if err != nil {
		return fmt.Errorf("update attachment created timestamp: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return ErrNotFound
	}
	return nil
}
