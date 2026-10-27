package services

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"windshift/internal/blobstore"
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

func validateAttachmentUpload(db database.Database, enabled bool, input attachmentFileInput, uploadErrors attachmentValidationErrors) (string, error) {
	if !enabled {
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
	settings, err := loadAttachmentSettings(db)
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

func storeAttachmentFile(ctx context.Context, store *blobstore.Store, collection, entityType string, entityID int, input attachmentFileInput, mimeType string) (storedAttachmentFile, error) {
	filename, err := generateAttachmentFilename(input.originalFilename)
	if err != nil {
		return storedAttachmentFile{}, fmt.Errorf("generate filename: %w", err)
	}
	entityPart := strconv.Itoa(entityID)
	storedPath := store.Path(collection, entityPart, filename)
	stored := storedAttachmentFile{
		filename: filename,
		path:     storedPath,
		mimeType: mimeType,
		size:     int64(len(input.data)),
	}

	if err := store.Put(ctx, storedPath, bytes.NewReader(input.data), int64(len(input.data)), mimeType); err != nil {
		return storedAttachmentFile{}, fmt.Errorf("save file: %w", err)
	}
	if !strings.HasPrefix(mimeType, "image/") {
		return stored, nil
	}

	thumbnail, err := GenerateAttachmentThumbnailJPEG(input.data)
	if err != nil {
		slog.Warn("failed to generate attachment thumbnail",
			slog.String("component", "attachments"),
			slog.String("entity_type", entityType),
			slog.Any("error", err))
		return stored, nil
	}
	thumbnailName := strings.TrimSuffix(filename, filepath.Ext(filename)) + ".thumb.jpg"
	thumbnailPath := store.Path(collection, entityPart, thumbnailName)
	if err := store.Put(ctx, thumbnailPath, bytes.NewReader(thumbnail), int64(len(thumbnail)), "image/jpeg"); err != nil {
		slog.Warn("failed to store attachment thumbnail",
			slog.String("component", "attachments"),
			slog.String("entity_type", entityType),
			slog.Any("error", err))
		return stored, nil
	}
	stored.hasThumbnail = true
	stored.thumbnailPath = thumbnailPath
	return stored, nil
}

func removeStoredAttachmentFile(ctx context.Context, store *blobstore.Store, stored storedAttachmentFile) {
	_ = store.Remove(ctx, stored.path)
	if stored.thumbnailPath != "" {
		_ = store.Remove(ctx, stored.thumbnailPath)
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
			HasThumbnail:     stored.hasThumbnail,
			CreatedAt:        time.Now(),
		},
	}
}
