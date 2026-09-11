package services

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

type attachmentFileInput struct {
	originalFilename string
	data             []byte
	validationSize   int64
}

type attachmentValidationErrors struct {
	disabled error
	invalid  error
}

type storedAttachmentFile struct {
	filename      string
	path          string
	mimeType      string
	size          int64
	hasThumbnail  bool
	thumbnailPath string
}

func validateAttachmentUpload(db database.Database, attachmentPath string, input attachmentFileInput, uploadErrors attachmentValidationErrors) (string, error) {
	if attachmentPath == "" {
		return "", uploadErrors.disabled
	}
	if strings.TrimSpace(input.originalFilename) == "" {
		return "", fmt.Errorf("%w: filename is required", uploadErrors.invalid)
	}
	if len(input.data) == 0 {
		return "", fmt.Errorf("%w: file is empty", uploadErrors.invalid)
	}
	if err := validateAttachmentFileExtension(input.originalFilename); err != nil {
		return "", fmt.Errorf("%w: %s", uploadErrors.invalid, err.Error())
	}
	detectedMimeType, err := verifyAttachmentFileContent(input.data, input.originalFilename)
	if err != nil {
		return "", fmt.Errorf("%w: File content validation failed: %s", uploadErrors.invalid, err.Error())
	}
	settings, err := loadAttachmentSettings(db, attachmentPath)
	if err != nil {
		return "", fmt.Errorf("get attachment settings: %w", err)
	}
	if !settings.Enabled {
		return "", uploadErrors.disabled
	}
	if input.validationSize > settings.MaxFileSize {
		return "", fmt.Errorf("%w: File too large. Maximum size: %d bytes", uploadErrors.invalid, settings.MaxFileSize)
	}
	if message := disallowedAttachmentMimeMessage(settings.AllowedMimeTypes, detectedMimeType); message != "" {
		return "", fmt.Errorf("%w: %s", uploadErrors.invalid, message)
	}
	return detectedMimeType, nil
}

func storeAttachmentFile(attachmentPath, collection, entityType string, entityID int, input attachmentFileInput, mimeType string) (storedAttachmentFile, error) {
	filename, err := generateAttachmentFilename(input.originalFilename)
	if err != nil {
		return storedAttachmentFile{}, fmt.Errorf("generate filename: %w", err)
	}
	dir := filepath.Join(attachmentPath, collection, strconv.Itoa(entityID))
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // trusted root and collection, numeric id
		return storedAttachmentFile{}, fmt.Errorf("create attachment directory: %w", err)
	}
	stored := storedAttachmentFile{
		filename: filename,
		path:     filepath.Join(dir, filename),
		mimeType: mimeType,
		size:     int64(len(input.data)),
	}
	if err := os.WriteFile(stored.path, input.data, 0o600); err != nil { //nolint:gosec // generated filename under trusted directory
		return storedAttachmentFile{}, fmt.Errorf("save file: %w", err)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return stored, nil
	}
	thumbnailPath, err := generateAttachmentThumbnail(stored.path, filename)
	if err != nil {
		slog.Warn("failed to generate attachment thumbnail", slog.String("component", "attachments"), slog.String("entity_type", entityType), slog.Any("error", err))
		return stored, nil
	}
	stored.hasThumbnail = true
	stored.thumbnailPath = thumbnailPath
	return stored, nil
}

func removeStoredAttachmentFile(stored storedAttachmentFile) {
	_ = os.Remove(stored.path) //nolint:gosec // server-managed attachment path
	if stored.thumbnailPath != "" {
		_ = os.Remove(stored.thumbnailPath) //nolint:gosec // server-managed thumbnail path
	}
}

func newAttachmentUploadResponse(attachmentID int64, entityID int, originalFilename string, uploaderID *int, stored storedAttachmentFile) models.AttachmentUploadResponse {
	return models.AttachmentUploadResponse{
		Success: true,
		Message: "File uploaded successfully",
		Attachment: models.Attachment{
			ID:               int(attachmentID),
			ItemID:           &entityID,
			Filename:         stored.filename,
			OriginalFilename: originalFilename,
			MimeType:         stored.mimeType,
			FileSize:         stored.size,
			UploadedBy:       uploaderID,
			CreatedAt:        time.Now(),
		},
	}
}
