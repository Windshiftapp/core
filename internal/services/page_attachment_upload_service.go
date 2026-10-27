package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"windshift/internal/blobstore"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// maxAttachmentThumbnailSourcePixels bounds the declared dimensions an
// attachment may carry before the thumbnail decoder allocates for it.
const maxAttachmentThumbnailSourcePixels = 25_000_000

var (
	ErrPageAttachmentUploadDisabled = errors.New("page attachment upload disabled")
	ErrPageAttachmentUploadInvalid  = errors.New("page attachment upload invalid")
	ErrPageAttachmentUploadNotFound = errors.New("page attachment upload target not found")
)

// PageAttachmentUploadService owns page-attachment upload validation, storage,
// thumbnail generation, DB insert, and page-edit authorization. Both cookie and
// bearer HTTP adapters can call this use case without sharing handler code.
type PageAttachmentUploadService struct {
	db                    database.Database
	storage               *blobstore.Store
	permissionService     *PermissionService
	pagePermissionService *PagePermissionService
	attachmentService     *AttachmentService
}

// NewPageAttachmentUploadService creates a page attachment upload service.
func NewPageAttachmentUploadService(db database.Database, storage *blobstore.Store, permissionService *PermissionService, pagePermissionService *PagePermissionService) *PageAttachmentUploadService {
	return &PageAttachmentUploadService{
		db:                    db,
		storage:               storage,
		permissionService:     permissionService,
		pagePermissionService: pagePermissionService,
		attachmentService:     NewAttachmentService(db),
	}
}

// PageAttachmentUploadInput contains the validated HTTP upload payload.
type PageAttachmentUploadInput struct {
	PageID           int
	UploaderID       int
	OriginalFilename string
	FileData         []byte
	FileSize         int64
}

// UploadPageAttachment stores a new attachment for a page and returns the same
// response model used by the legacy upload endpoint for regular attachments.
func (s *PageAttachmentUploadService) UploadPageAttachment(ctx context.Context, in PageAttachmentUploadInput) (models.AttachmentUploadResponse, error) {
	if !s.storage.Enabled() {
		return models.AttachmentUploadResponse{}, ErrPageAttachmentUploadDisabled
	}
	if in.PageID <= 0 {
		return models.AttachmentUploadResponse{}, fmt.Errorf("%w: page_id is required", ErrPageAttachmentUploadInvalid)
	}
	if strings.TrimSpace(in.OriginalFilename) == "" {
		return models.AttachmentUploadResponse{}, fmt.Errorf("%w: filename is required", ErrPageAttachmentUploadInvalid)
	}
	if len(in.FileData) == 0 {
		return models.AttachmentUploadResponse{}, fmt.Errorf("%w: file is empty", ErrPageAttachmentUploadInvalid)
	}

	if err := s.authorizePageEdit(in.UploaderID, in.PageID); err != nil {
		return models.AttachmentUploadResponse{}, err
	}

	fileInput := attachmentFileInput{
		originalFilename: in.OriginalFilename,
		data:             in.FileData,
		validationSize:   in.FileSize,
	}
	detectedMimeType, err := validateAttachmentUpload(s.db, s.storage.Enabled(), fileInput, attachmentValidationErrors{
		disabled: ErrPageAttachmentUploadDisabled,
		invalid:  ErrPageAttachmentUploadInvalid,
	})
	if err != nil {
		return models.AttachmentUploadResponse{}, err
	}
	stored, err := storeAttachmentFile(ctx, s.storage, "pages", "page", in.PageID, fileInput, detectedMimeType)
	if err != nil {
		return models.AttachmentUploadResponse{}, err
	}

	uploaderID := in.UploaderID
	attachmentID, err := s.attachmentService.CreateRecord(CreateAttachmentParams{
		ItemID:           in.PageID,
		EntityType:       "page",
		Filename:         stored.filename,
		OriginalFilename: in.OriginalFilename,
		FilePath:         stored.path,
		MimeType:         stored.mimeType,
		FileSize:         stored.size,
		UploadedBy:       &uploaderID,
		HasThumbnail:     stored.hasThumbnail,
		ThumbnailPath:    stored.thumbnailPath,
		Category:         "",
	})
	if err != nil {
		removeStoredAttachmentFile(ctx, s.storage, stored)
		return models.AttachmentUploadResponse{}, fmt.Errorf("save attachment record: %w", err)
	}
	return newAttachmentUploadResponse(attachmentID, in.PageID, in.OriginalFilename, &uploaderID, stored), nil
}

// DeleteUploadedPageAttachment compensates a failed higher-level mutation.
// It is intentionally not exposed as a user-facing delete operation: live and
// historical Page revisions may still reference successful diagram uploads.
func (s *PageAttachmentUploadService) DeleteUploadedPageAttachment(ctx context.Context, pageID, attachmentID int) error {
	rec, err := repository.NewAttachmentRepository(s.db).GetPageAttachmentRecord(pageID, attachmentID)
	if err != nil {
		return err
	}
	// Blob-first: remove the file and thumbnail before the row. A removal
	// failure keeps the row so the delete can be retried; a missing object is
	// already success.
	for _, path := range []string{rec.FilePath, rec.ThumbnailPath} {
		if path == "" {
			continue
		}
		if err := s.storage.Remove(ctx, path); err != nil {
			if errors.Is(err, blobstore.ErrOutsideRoot) {
				continue
			}
			return fmt.Errorf("remove page attachment file: %w", err)
		}
	}
	rows, err := s.attachmentService.DeleteRecord(attachmentID)
	if err != nil {
		return err
	}
	if rows != 1 {
		return repository.ErrNotFound
	}
	return nil
}

func (s *PageAttachmentUploadService) authorizePageEdit(userID, pageID int) error {
	var wsID int
	err := s.db.QueryRow(`SELECT workspace_id FROM pages WHERE id = ?`, pageID).Scan(&wsID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPageAttachmentUploadNotFound
	}
	if err != nil {
		return fmt.Errorf("resolve page workspace: %w", err)
	}
	if s.pagePermissionService != nil {
		can, err := s.pagePermissionService.Can(userID, wsID, pageID, PageOpEdit)
		if err != nil {
			return fmt.Errorf("check page permission: %w", err)
		}
		if !can {
			return ErrPageAttachmentUploadNotFound
		}
		return nil
	}
	if s.permissionService == nil {
		return ErrPageAttachmentUploadNotFound
	}
	allowed, err := s.permissionService.HasWorkspacePermission(userID, wsID, models.PermissionPageEdit)
	if err != nil {
		return fmt.Errorf("check workspace permission: %w", err)
	}
	if !allowed {
		return ErrPageAttachmentUploadNotFound
	}
	return nil
}

// loadAttachmentSettings loads the system-wide attachment settings, falling
// back to permissive defaults when no row exists. Shared by the page and item
// attachment upload services so both surfaces read storage limits identically.
func loadAttachmentSettings(db database.Database) (*models.AttachmentSettings, error) {
	settings := &models.AttachmentSettings{
		MaxFileSize:      52428800, // 50MB default
		AllowedMimeTypes: "",
		Enabled:          true,
	}
	err := db.QueryRow(`
		SELECT max_file_size, allowed_mime_types, attachment_path, enabled
		FROM attachment_settings ORDER BY id DESC LIMIT 1
	`).Scan(&settings.MaxFileSize, &settings.AllowedMimeTypes, &settings.AttachmentPath, &settings.Enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func disallowedAttachmentMimeMessage(allowedMimeTypes, detectedMimeType string) string {
	if allowedMimeTypes == "" {
		return ""
	}
	// Preserve the legacy upload behavior: malformed settings are treated as
	// no MIME restriction rather than blocking all uploads.
	if !json.Valid([]byte(allowedMimeTypes)) {
		return ""
	}
	var allowedTypes []string
	_ = json.Unmarshal([]byte(allowedMimeTypes), &allowedTypes)
	if len(allowedTypes) == 0 {
		return ""
	}
	for _, allowedType := range allowedTypes {
		if strings.HasPrefix(detectedMimeType, allowedType) {
			return ""
		}
	}
	return fmt.Sprintf("File type %s not allowed by server configuration", detectedMimeType)
}

func verifyAttachmentFileContent(fileData []byte, filename string) (string, error) {
	detectSize := 512
	if len(fileData) < detectSize {
		detectSize = len(fileData)
	}
	detectedType := http.DetectContentType(fileData[:detectSize])
	ext := filepath.Ext(filename)
	expectedType := mime.TypeByExtension(ext)
	if expectedType != "" {
		detectedBase := strings.Split(detectedType, ";")[0]
		expectedBase := strings.Split(expectedType, ";")[0]
		if detectedBase != expectedBase && detectedBase != "application/octet-stream" &&
			(detectedBase != "text/plain" || !strings.HasPrefix(expectedBase, "text/")) {
			if detectedBase == "application/zip" && isZipBasedAttachmentMimeType(expectedBase) {
				return expectedType, nil
			}
			if detectedBase == "text/plain" && expectedBase == "application/json" && looksLikeAttachmentJSON(fileData) {
				return expectedType, nil
			}
			return "", fmt.Errorf("file content type (%s) doesn't match extension %s (expected %s)", detectedBase, ext, expectedBase)
		}
	}
	return detectedType, nil
}

func isZipBasedAttachmentMimeType(mimeType string) bool {
	return mimeType == "application/zip" ||
		strings.Contains(mimeType, "openxmlformats") ||
		strings.Contains(mimeType, "opendocument") ||
		mimeType == "application/epub+zip"
}

func looksLikeAttachmentJSON(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return false
	}
	return json.Valid(trimmed)
}

func validateAttachmentFileExtension(filename string) error {
	dangerousExtensions := []string{
		".exe", ".bat", ".cmd", ".com", ".pif", ".scr", ".msi",
		".js", ".jsx", ".ts", ".tsx",
		".html", ".htm", ".svg",
		".sh", ".bash", ".zsh", ".fish",
		".py", ".rb", ".pl", ".php", ".asp", ".aspx", ".jsp",
		".jar", ".class", ".dex",
		".app", ".dmg", ".pkg",
		".deb", ".rpm",
		".apk", ".ipa",
	}
	ext := strings.ToLower(filepath.Ext(filename))
	for _, dangerous := range dangerousExtensions {
		if ext == dangerous {
			return fmt.Errorf("file extension %s is not allowed for security reasons", ext)
		}
	}
	if ext == "" || ext == "." {
		return fmt.Errorf("files without extensions are not allowed")
	}
	return nil
}

func generateAttachmentFilename(originalFilename string) (string, error) {
	ext := filepath.Ext(originalFilename)
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x%s", randomBytes, ext), nil
}
