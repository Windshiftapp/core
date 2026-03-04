package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif" // Register GIF decoder
	"image/jpeg"
	_ "image/png" // Register PNG decoder
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"

	"golang.org/x/image/draw"
)

type AttachmentHandler struct {
	db                database.Database
	attachmentPath    string
	permissionService *services.PermissionService
	attachmentService *services.AttachmentService
}

func NewAttachmentHandler(db database.Database, attachmentPath string, permissionService *services.PermissionService) *AttachmentHandler {
	return &AttachmentHandler{
		db:                db,
		attachmentPath:    attachmentPath,
		permissionService: permissionService,
		attachmentService: services.NewAttachmentServiceWithPermissions(db, permissionService),
	}
}

// checkItemAttachmentPermission checks if the user can modify attachments on an item
// Internal users need item.edit permission in the workspace
// Portal customers can only modify attachments on items they created
func (h *AttachmentHandler) checkItemAttachmentPermission(r *http.Request, itemID int) (bool, error) {
	// Get user ID if internal user
	var userID *int
	if user := utils.GetCurrentUser(r); user != nil {
		userID = &user.ID
	}

	// Get portal customer ID if portal customer
	var portalCustomerID *int
	if pcID, ok := r.Context().Value(middleware.ContextKeyPortalCustomerID).(int); ok {
		portalCustomerID = &pcID
	}

	return h.attachmentService.CanModifyItemAttachment(userID, portalCustomerID, itemID)
}

// IsEnabled checks if attachments are enabled (attachment path is set)
func (h *AttachmentHandler) IsEnabled() bool {
	return h.attachmentPath != ""
}

// Upload handles file upload to an item
func (h *AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	slog.Debug("upload request received", slog.String("component", "attachments"))

	if !h.IsEnabled() {
		slog.Warn("upload failed: attachments not enabled", slog.String("component", "attachments"))
		respondServiceUnavailable(w, r, "Attachments are not enabled on this server")
		return
	}

	// Parse form data (32MB max)
	slog.Debug("parsing multipart form", slog.String("component", "attachments"))
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		slog.Error("failed to parse form data", slog.String("component", "attachments"), slog.Any("error", err))
		respondBadRequest(w, r, "Failed to parse form data: "+err.Error())
		return
	}

	// Get entity info from form
	// Support both old (item_id) and new (entity_type + entity_id) parameters
	entityIDStr := r.FormValue("entity_id")
	if entityIDStr == "" {
		entityIDStr = r.FormValue("item_id") // Backwards compatibility
	}
	entityType := r.FormValue("entity_type")
	category := r.FormValue("category")

	// Determine entity type from category for backwards compatibility
	if entityType == "" {
		switch category {
		case "avatar":
			entityType = "avatar"
		case "workspace_avatar":
			entityType = "workspace_avatar"
		case "customer_avatar":
			entityType = "customer_avatar"
		case "workspace_background":
			entityType = "workspace_background"
		case "portal_background":
			entityType = "portal_background"
		case "portal_logo":
			entityType = "portal_logo"
		case "hub_logo":
			entityType = "hub_logo"
		default:
			entityType = "item" // Default to item for backwards compatibility
		}
	}

	slog.Debug("entity info received", slog.String("component", "attachments"), slog.String("entity_id", entityIDStr), slog.String("entity_type", entityType), slog.String("category", category))

	// Handle avatar uploads differently (they don't need a real entity)
	isAvatar := entityType == "avatar"
	isWorkspaceAvatar := entityType == "workspace_avatar"
	isCustomerAvatar := entityType == "customer_avatar"
	isWorkspaceBackground := entityType == "workspace_background"
	isPortalBackground := entityType == "portal_background"
	isPortalLogo := entityType == "portal_logo"
	isHubLogo := entityType == "hub_logo"

	// Validate entity_id is provided (except for avatars, backgrounds, and logos)
	if entityIDStr == "" && !isAvatar && !isWorkspaceAvatar && !isCustomerAvatar && !isWorkspaceBackground && !isPortalBackground && !isPortalLogo && !isHubLogo {
		slog.Debug("missing entity_id in form", slog.String("component", "attachments"))
		respondValidationError(w, r, "entity_id is required")
		return
	}

	var entityID int
	if entityIDStr != "" {
		entityID, err = strconv.Atoi(entityIDStr)
		if err != nil {
			slog.Error("invalid entity_id", slog.String("component", "attachments"), slog.Any("error", err))
			respondInvalidID(w, r, "entity_id")
			return
		}
	}
	slog.Debug("uploading to entity", slog.String("component", "attachments"), slog.String("entity_type", entityType), slog.Int("entity_id", entityID))

	// Verify entity exists based on type
	if !isAvatar && !isWorkspaceAvatar && !isCustomerAvatar && !isWorkspaceBackground && !isPortalBackground && !isPortalLogo && !isHubLogo {
		var exists bool
		var checkQuery string

		switch entityType {
		case "item":
			checkQuery = "SELECT EXISTS(SELECT 1 FROM items WHERE id = ?)"
		case "test_case":
			checkQuery = "SELECT EXISTS(SELECT 1 FROM test_cases WHERE id = ?)"
		default:
			slog.Debug("unknown entity type", slog.String("component", "attachments"), slog.String("entity_type", entityType))
			respondValidationError(w, r, "Unknown entity type")
			return
		}

		slog.Debug("verifying entity exists", slog.String("component", "attachments"), slog.String("entity_type", entityType), slog.Int("entity_id", entityID))
		err = h.db.QueryRow(checkQuery, entityID).Scan(&exists)
		if err != nil {
			slog.Error("database error checking entity existence", slog.String("component", "attachments"), slog.Any("error", err))
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			slog.Debug("entity not found", slog.String("component", "attachments"), slog.String("entity_type", entityType), slog.Int("entity_id", entityID))
			respondNotFound(w, r, entityType)
			return
		}
		slog.Debug("entity exists", slog.String("component", "attachments"), slog.String("entity_type", entityType), slog.Int("entity_id", entityID))

		// Check permission for item attachments
		if entityType == "item" {
			var canModify bool
			canModify, err = h.checkItemAttachmentPermission(r, entityID)
			if err != nil {
				slog.Error("failed to check attachment permission", slog.String("component", "attachments"), slog.Any("error", err))
				respondInternalError(w, r, err)
				return
			}
			if !canModify {
				slog.Debug("user lacks permission to upload attachment to item", slog.String("component", "attachments"), slog.Int("entity_id", entityID))
				respondForbidden(w, r)
				return
			}
		}
	} else {
		slog.Debug("skipping entity existence check for avatar upload", slog.String("component", "attachments"))
	}

	// Get file from form
	slog.Debug("getting file from form", slog.String("component", "attachments"))
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		slog.Error("failed to get file from form", slog.String("component", "attachments"), slog.Any("error", err))
		respondBadRequest(w, r, "Failed to get file from form: "+err.Error())
		return
	}
	defer func() { _ = file.Close() }()
	slog.Debug("file received", slog.String("component", "attachments"), slog.String("filename", fileHeader.Filename), slog.Int64("size", fileHeader.Size), slog.String("content_type", fileHeader.Header.Get("Content-Type")))

	// Read entire file into memory to avoid multipart.File seek issues
	slog.Debug("reading file into memory", slog.String("component", "attachments"))
	fileData, err := io.ReadAll(file)
	if err != nil {
		slog.Error("failed to read file data", slog.String("component", "attachments"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to read file data: %w", err))
		return
	}
	slog.Debug("file data read", slog.String("component", "attachments"), slog.Int("bytes", len(fileData)))

	// SECURITY: Validate file extension against dangerous extensions blacklist
	slog.Debug("validating file extension", slog.String("component", "attachments"))
	if err = h.validateFileExtension(fileHeader.Filename); err != nil {
		slog.Warn("extension validation failed", slog.String("component", "attachments"), slog.Any("error", err))
		respondValidationError(w, r, err.Error())
		return
	}

	// SECURITY: Verify actual file content matches extension
	slog.Debug("verifying file content", slog.String("component", "attachments"))
	detectedMimeType, err := h.verifyFileContentFromBytes(fileData, fileHeader.Filename)
	if err != nil {
		slog.Warn("content verification failed", slog.String("component", "attachments"), slog.Any("error", err))
		respondValidationError(w, r, "File content validation failed: "+err.Error())
		return
	}
	slog.Debug("content verified", slog.String("component", "attachments"), slog.String("mime_type", detectedMimeType))

	// Get attachment settings for validation
	slog.Debug("getting attachment settings", slog.String("component", "attachments"))
	settings, err := h.getAttachmentSettings()
	if err != nil {
		slog.Error("failed to get attachment settings", slog.String("component", "attachments"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to get attachment settings: %w", err))
		return
	}
	slog.Debug("attachment settings loaded", slog.String("component", "attachments"), slog.Bool("enabled", settings.Enabled), slog.Int64("max_size", settings.MaxFileSize))

	if !settings.Enabled {
		respondServiceUnavailable(w, r, "Attachments are disabled")
		return
	}

	// Validate file size
	if fileHeader.Size > settings.MaxFileSize {
		respondValidationError(w, r, fmt.Sprintf("File too large. Maximum size: %d bytes", settings.MaxFileSize))
		return
	}

	// Validate MIME type against allowed types (if restrictions are set)
	// Use the detected MIME type from content verification (not client header)
	if settings.AllowedMimeTypes != "" {
		var allowedTypes []string
		if err = json.Unmarshal([]byte(settings.AllowedMimeTypes), &allowedTypes); err == nil {
			if len(allowedTypes) > 0 {
				allowed := false
				for _, allowedType := range allowedTypes {
					if strings.HasPrefix(detectedMimeType, allowedType) {
						allowed = true
						break
					}
				}

				if !allowed {
					respondValidationError(w, r, fmt.Sprintf("File type %s not allowed by server configuration", detectedMimeType))
					return
				}
			}
		}
	}

	// Generate unique filename
	slog.Debug("generating unique filename", slog.String("component", "attachments"), slog.String("original_filename", fileHeader.Filename))
	uniqueFilename, err := h.generateUniqueFilename(fileHeader.Filename)
	if err != nil {
		slog.Error("failed to generate filename", slog.String("component", "attachments"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to generate filename: %w", err))
		return
	}
	slog.Debug("generated filename", slog.String("component", "attachments"), slog.String("unique_filename", uniqueFilename))

	// Ensure attachment directory exists based on entity type
	var itemDir string
	switch entityType {
	case "avatar":
		itemDir = filepath.Join(h.attachmentPath, "avatars")
	case "workspace_avatar":
		itemDir = filepath.Join(h.attachmentPath, "workspace_avatars")
	case "customer_avatar":
		itemDir = filepath.Join(h.attachmentPath, "customer_avatars")
	case "workspace_background":
		itemDir = filepath.Join(h.attachmentPath, "workspace_backgrounds")
	case "portal_background":
		itemDir = filepath.Join(h.attachmentPath, "portal_backgrounds")
	case "portal_logo":
		itemDir = filepath.Join(h.attachmentPath, "portal_logos")
	case "hub_logo":
		itemDir = filepath.Join(h.attachmentPath, "hub_logos")
	case "test_case":
		itemDir = filepath.Join(h.attachmentPath, "test_cases", strconv.Itoa(entityID))
	default: // "item"
		itemDir = filepath.Join(h.attachmentPath, "items", strconv.Itoa(entityID))
	}
	slog.Debug("creating directory", slog.String("component", "attachments"), slog.String("path", itemDir))
	if err = os.MkdirAll(itemDir, 0o750); err != nil { //nolint:gosec // G703: path built from hardcoded strings + strconv.Itoa(entityID)
		slog.Error("failed to create directory", slog.String("component", "attachments"), slog.String("path", itemDir), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to create attachment directory: %w", err))
		return
	}

	// Create file path
	filePath := filepath.Join(itemDir, uniqueFilename)
	slog.Debug("creating file", slog.String("component", "attachments"), slog.String("path", filePath))

	// Write file data directly (already in memory from earlier read)
	slog.Debug("writing file data", slog.String("component", "attachments"))
	err = os.WriteFile(filePath, fileData, 0o600) //nolint:gosec // G703: path from hardcoded base + strconv.Itoa
	if err != nil {
		slog.Error("failed to write file", slog.String("component", "attachments"), slog.String("path", filePath), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to save file: %w", err))
		return
	}
	fileSize := int64(len(fileData))
	slog.Debug("file saved", slog.String("component", "attachments"), slog.Int64("bytes", fileSize))

	// Get uploader ID from context/session
	var uploaderID *int
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			uploaderID = &u.ID
		}
	}

	// Save attachment record to database
	// Use the detected MIME type from content verification (not client header)
	mimeType := detectedMimeType

	// Generate thumbnail for images
	hasThumbnail := false
	var thumbnailPath string
	if strings.HasPrefix(mimeType, "image/") {
		slog.Debug("generating thumbnail for image", slog.String("component", "attachments"), slog.String("filename", uniqueFilename))
		thumbnailPath, err = h.generateThumbnail(filePath, uniqueFilename)
		if err == nil {
			hasThumbnail = true
			slog.Debug("thumbnail generated", slog.String("component", "attachments"), slog.String("thumbnail_path", thumbnailPath))
		} else {
			slog.Warn("failed to generate thumbnail", slog.String("component", "attachments"), slog.String("filename", uniqueFilename), slog.Any("error", err))
		}
	} else {
		slog.Debug("skipping thumbnail generation for non-image", slog.String("component", "attachments"), slog.String("mime_type", mimeType))
	}

	slog.Debug("saving attachment record to database", slog.String("component", "attachments"))

	// Add entity_type column if it doesn't exist (for polymorphic attachment support)
	_, err = h.db.ExecWrite("ALTER TABLE attachments ADD COLUMN entity_type TEXT DEFAULT 'item'")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") && !strings.Contains(err.Error(), "already exists") {
		slog.Warn("failed to add entity_type column (may already exist)", slog.String("component", "attachments"), slog.Any("error", err))
	}

	// Add category column if it doesn't exist (for avatar support)
	_, err = h.db.ExecWrite("ALTER TABLE attachments ADD COLUMN category TEXT DEFAULT ''")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") && !strings.Contains(err.Error(), "already exists") {
		slog.Warn("failed to add category column (may already exist)", slog.String("component", "attachments"), slog.Any("error", err))
	}

	// Insert attachment record via service
	attachmentSvc := services.NewAttachmentService(h.db)
	attachmentID, err := attachmentSvc.CreateRecord(services.CreateAttachmentParams{
		ItemID:           entityID,
		EntityType:       entityType,
		Filename:         uniqueFilename,
		OriginalFilename: fileHeader.Filename,
		FilePath:         filePath,
		MimeType:         mimeType,
		FileSize:         fileSize,
		UploadedBy:       uploaderID,
		HasThumbnail:     hasThumbnail,
		ThumbnailPath:    thumbnailPath,
		Category:         category,
	})
	if err != nil {
		slog.Error("failed to save attachment record", slog.String("component", "attachments"), slog.Any("error", err))
		_ = os.Remove(filePath) //nolint:gosec // G703: cleanup of path already validated
		respondInternalError(w, r, fmt.Errorf("failed to save attachment record: %w", err))
		return
	}

	// For avatar type checks below
	var attachmentEntityID interface{}
	if isAvatar || isWorkspaceAvatar || isCustomerAvatar || isWorkspaceBackground || isPortalBackground || isPortalLogo || isHubLogo {
		attachmentEntityID = nil
	} else {
		attachmentEntityID = entityID
	}
	slog.Debug("attachment saved", slog.String("component", "attachments"), slog.Int64("attachment_id", attachmentID))

	// Record history for item attachments only (not test_case, avatars, etc.)
	if entityType == "item" && attachmentEntityID != nil {
		if entityIDInt, ok := attachmentEntityID.(int); ok {
			if err = h.recordAttachmentHistory(entityIDInt, uploaderID, "attachment_uploaded", nil, attachmentID, fileHeader.Filename); err != nil {
				slog.Warn("failed to record attachment history", slog.String("component", "attachments"), slog.Any("error", err))
				// Don't fail the whole operation if history recording fails
			}
		}
	}

	// For avatars, also update the user's avatar_url with the attachment download URL
	if isAvatar && uploaderID != nil {
		avatarURL := fmt.Sprintf("/api/attachments/%d/download", attachmentID)
		slog.Debug("updating user avatar_url", slog.String("component", "attachments"), slog.Int("user_id", *uploaderID), slog.String("avatar_url", avatarURL))

		_, err = h.db.ExecWrite(`UPDATE users SET avatar_url = ? WHERE id = ?`, avatarURL, *uploaderID)
		if err != nil {
			slog.Warn("failed to update user avatar_url", slog.String("component", "attachments"), slog.Any("error", err))
			// Don't fail the whole operation, avatar was still uploaded
		} else {
			slog.Debug("user avatar updated successfully", slog.String("component", "attachments"))
		}
	}

	// Return success response
	if isAvatar || isWorkspaceAvatar || isCustomerAvatar || isWorkspaceBackground || isPortalBackground || isPortalLogo || isHubLogo {
		// For avatars, backgrounds, and logos, return the appropriate download URL
		// Portal branding (logo, background, hub_logo) uses public endpoint, others use authenticated endpoint
		var downloadURL string
		if isPortalBackground || isPortalLogo || isHubLogo {
			// Public endpoint for portal branding (no auth required)
			downloadURL = fmt.Sprintf("/api/portal-assets/%d", attachmentID)
		} else {
			// Authenticated endpoint for user avatars
			downloadURL = fmt.Sprintf("/api/attachments/%d/download", attachmentID)
		}
		message := "Avatar uploaded successfully"
		urlKey := "avatar_url"
		switch {
		case isWorkspaceAvatar:
			message = "Workspace avatar uploaded successfully"
		case isCustomerAvatar:
			message = "Customer avatar uploaded successfully"
		case isWorkspaceBackground:
			message = "Workspace background uploaded successfully"
			urlKey = "background_url"
		case isPortalBackground:
			message = "Portal background uploaded successfully"
			urlKey = "background_url"
		case isPortalLogo:
			message = "Portal logo uploaded successfully"
			urlKey = "logo_url"
		case isHubLogo:
			message = "Hub logo uploaded successfully"
			urlKey = "logo_url"
		}
		response := map[string]interface{}{
			"success":       true,
			"message":       message,
			urlKey:          downloadURL,
			"attachment_id": attachmentID,
			"filename":      uniqueFilename,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
		return
	} else {
		// For regular attachments, return attachment structure
		attachment := models.Attachment{
			ID:               int(attachmentID),
			ItemID:           &entityID,
			Filename:         uniqueFilename,
			OriginalFilename: fileHeader.Filename,
			MimeType:         mimeType,
			FileSize:         fileSize,
			UploadedBy:       uploaderID,
			CreatedAt:        time.Now(),
		}

		response := models.AttachmentUploadResponse{
			Success:    true,
			Message:    "File uploaded successfully",
			Attachment: attachment,
		}

		slog.Debug("upload completed successfully", slog.String("component", "attachments"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

// GetByItem returns attachments for a specific item with pagination support
func (h *AttachmentHandler) GetByItem(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.Atoi(r.PathValue("itemId"))
	if err != nil {
		respondInvalidID(w, r, "itemId")
		return
	}

	// Get user from context and check permissions
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Look up the item to get its workspace_id for permission check
	var workspaceID int
	err = h.db.QueryRow("SELECT workspace_id FROM items WHERE id = ?", itemID).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check workspace view permission
	if h.permissionService != nil {
		var canView bool
		canView, err = h.permissionService.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemView)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canView {
			respondForbidden(w, r)
			return
		}
	}

	// Parse pagination parameters
	page := 1
	limit := 50     // Default items per page
	maxLimit := 100 // Maximum items that can be returned from API

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		var p int
		if p, err = strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var l int
		if l, err = strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > maxLimit {
				limit = maxLimit
			}
		}
	}

	offset := (page - 1) * limit

	// Get total count first
	var totalCount int
	err = h.db.QueryRow(`
		SELECT COUNT(*) FROM attachments WHERE item_id = ?
	`, itemID).Scan(&totalCount)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Query attachments with uploader info and pagination
	rows, err := h.db.Query(`
		SELECT a.id, a.item_id, a.filename, a.original_filename, a.mime_type, a.file_size,
		       a.uploaded_by, a.has_thumbnail, a.created_at,
		       u.first_name || ' ' || u.last_name as uploader_name, u.email as uploader_email
		FROM attachments a
		LEFT JOIN users u ON a.uploaded_by = u.id
		WHERE a.item_id = ?
		ORDER BY a.created_at DESC
		LIMIT ? OFFSET ?
	`, itemID, limit, offset)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var attachments []models.Attachment
	for rows.Next() {
		var attachment models.Attachment
		var itemID sql.NullInt64
		var uploaderName, uploaderEmail sql.NullString

		err := rows.Scan(
			&attachment.ID, &itemID, &attachment.Filename, &attachment.OriginalFilename,
			&attachment.MimeType, &attachment.FileSize, &attachment.UploadedBy, &attachment.HasThumbnail, &attachment.CreatedAt,
			&uploaderName, &uploaderEmail,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if itemID.Valid {
			id := int(itemID.Int64)
			attachment.ItemID = &id
		}
		if uploaderName.Valid {
			attachment.UploaderName = uploaderName.String
		}
		if uploaderEmail.Valid {
			attachment.UploaderEmail = uploaderEmail.String
		}

		attachments = append(attachments, attachment)
	}

	// Create paginated response
	response := models.PaginatedAttachmentsResponse{
		Attachments: attachments,
		Pagination: models.PaginationMeta{
			Page:       page,
			Limit:      limit,
			Total:      totalCount,
			TotalPages: (totalCount + limit - 1) / limit,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// Download serves a specific attachment file
func (h *AttachmentHandler) Download(w http.ResponseWriter, r *http.Request) {
	slog.Debug("download request received", slog.String("component", "attachments"), slog.String("attachment_id", r.PathValue("id")))

	attachmentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		slog.Error("invalid attachment ID", slog.String("component", "attachments"), slog.Any("error", err))
		respondInvalidID(w, r, "id")
		return
	}

	// Get attachment info
	slog.Debug("getting attachment info", slog.String("component", "attachments"), slog.Int("attachment_id", attachmentID))
	var attachment models.Attachment
	var itemID sql.NullInt64
	err = h.db.QueryRow(`
		SELECT id, item_id, filename, original_filename, file_path, mime_type, file_size
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(
		&attachment.ID, &itemID, &attachment.Filename, &attachment.OriginalFilename,
		&attachment.FilePath, &attachment.MimeType, &attachment.FileSize,
	)

	if err == sql.ErrNoRows {
		slog.Debug("attachment not found in database", slog.String("component", "attachments"), slog.Int("attachment_id", attachmentID))
		respondNotFound(w, r, "attachment")
		return
	}
	if err != nil {
		slog.Error("database error", slog.String("component", "attachments"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}
	slog.Debug("found attachment", slog.String("component", "attachments"), slog.String("original_filename", attachment.OriginalFilename), slog.String("path", attachment.FilePath))

	// Check item permission if attachment is associated with an item
	if attachment.ItemID != nil {
		if !CheckItemPermission(w, r, h.db, h.permissionService, *attachment.ItemID, models.PermissionItemView) {
			return
		}
	}

	// Validate file path is within attachment directory (prevent path traversal)
	absPath, err := filepath.Abs(attachment.FilePath)
	if err != nil {
		slog.Error("failed to resolve file path", slog.String("component", "attachments"), slog.Any("error", err))
		respondBadRequest(w, r, "Invalid file path")
		return
	}
	absBasePath, _ := filepath.Abs(h.attachmentPath)
	if !strings.HasPrefix(absPath, absBasePath+string(os.PathSeparator)) {
		slog.Warn("path traversal attempt detected", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
		respondBadRequest(w, r, "Invalid file path")
		return
	}

	// Check if file exists
	slog.Debug("checking if file exists", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
	if _, err = os.Stat(attachment.FilePath); os.IsNotExist(err) {
		slog.Debug("file not found on disk", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
		respondNotFound(w, r, "file")
		return
	}

	// Open file
	slog.Debug("opening file", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
	file, err := os.Open(attachment.FilePath)
	if err != nil {
		slog.Error("failed to open file", slog.String("component", "attachments"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to open file: %w", err))
		return
	}
	defer func() { _ = file.Close() }()

	// Set headers
	slog.Debug("setting headers and serving file", slog.String("component", "attachments"), slog.String("original_filename", attachment.OriginalFilename))
	w.Header().Set("Content-Type", attachment.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(attachment.FileSize, 10))

	// SECURITY: Add security headers to prevent attacks
	// Prevent browsers from MIME-sniffing the response
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Prevent embedding in iframes
	w.Header().Set("X-Frame-Options", "DENY")
	// Control how the file is displayed/downloaded
	// Force download for potentially dangerous types (HTML, JS, SVG) to prevent XSS
	if strings.HasPrefix(attachment.MimeType, "text/html") ||
		strings.HasPrefix(attachment.MimeType, "application/javascript") ||
		strings.HasPrefix(attachment.MimeType, "text/javascript") ||
		strings.HasPrefix(attachment.MimeType, "image/svg+xml") ||
		strings.Contains(attachment.MimeType, "script") {
		// Force download for dangerous types
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", attachment.OriginalFilename)) //nolint:gocritic // Content-Disposition requires this specific format
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		slog.Debug("forcing download for potentially dangerous file type", slog.String("component", "attachments"), slog.String("mime_type", attachment.MimeType))
	} else {
		// Allow inline display for safe types (images, PDFs, etc.)
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", attachment.OriginalFilename)) //nolint:gocritic // Content-Disposition requires this specific format
	}

	// Serve file
	bytesServed, err := io.Copy(w, file)
	if err != nil {
		slog.Error("error serving file", slog.String("component", "attachments"), slog.Any("error", err))
	}
	slog.Debug("successfully served file", slog.String("component", "attachments"), slog.Int64("bytes_served", bytesServed))
}

// Delete removes an attachment
func (h *AttachmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	attachmentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get user from context for history tracking
	var userID *int
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
		if u, ok := user.(*models.User); ok {
			userID = &u.ID
		}
	}

	// Get attachment details before deletion (for history tracking and permission check)
	details, err := h.attachmentService.GetAttachmentDetails(attachmentID)
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "attachment")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission for item attachments
	if details.EntityType == "item" && details.ItemID != nil {
		var canModify bool
		canModify, err = h.checkItemAttachmentPermission(r, *details.ItemID)
		if err != nil {
			slog.Error("failed to check attachment permission", slog.String("component", "attachments"), slog.Any("error", err))
			respondInternalError(w, r, err)
			return
		}
		if !canModify {
			slog.Debug("user lacks permission to delete attachment from item", slog.String("component", "attachments"), slog.Int("item_id", *details.ItemID))
			respondForbidden(w, r)
			return
		}
	}

	// Record history if attachment is associated with an item
	if details.ItemID != nil && userID != nil {
		if err = h.recordAttachmentHistory(*details.ItemID, userID, "attachment_deleted", &details.OriginalFilename, 0, details.OriginalFilename); err != nil {
			slog.Warn("failed to record attachment deletion history", slog.String("component", "attachments"), slog.Any("error", err))
			// Don't fail the whole operation if history recording fails
		}
	}

	// Delete from database
	rowsAffected, err := h.attachmentService.DeleteRecord(attachmentID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "attachment")
		return
	}

	// Delete physical file
	if err := os.Remove(details.FilePath); err != nil && !os.IsNotExist(err) { //nolint:gosec // G703: path from DB record
		// Log warning but don't fail the request if file removal fails
		slog.Warn("failed to delete attachment file", slog.String("component", "attachments"), slog.String("file_path", details.FilePath), slog.Any("error", err))
	}

	w.WriteHeader(http.StatusNoContent)
}

// Thumbnail serves a thumbnail for an image attachment
func (h *AttachmentHandler) Thumbnail(w http.ResponseWriter, r *http.Request) {
	attachmentID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Get attachment info
	var hasThumbnail bool
	var thumbnailPath string
	var mimeType string
	var thumbItemID sql.NullInt64
	err = h.db.QueryRow(`
		SELECT has_thumbnail, thumbnail_path, mime_type, item_id
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(&hasThumbnail, &thumbnailPath, &mimeType, &thumbItemID)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "attachment")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check item permission if attachment is associated with an item
	if thumbItemID.Valid {
		if !CheckItemPermission(w, r, h.db, h.permissionService, int(thumbItemID.Int64), models.PermissionItemView) {
			return
		}
	}

	if !hasThumbnail || thumbnailPath == "" {
		respondNotFound(w, r, "thumbnail")
		return
	}

	// Check if thumbnail file exists
	if _, err = os.Stat(thumbnailPath); os.IsNotExist(err) {
		respondNotFound(w, r, "thumbnail")
		return
	}

	// Open thumbnail file
	file, err := os.Open(thumbnailPath)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to open thumbnail: %w", err))
		return
	}
	defer func() { _ = file.Close() }()

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get file info: %w", err))
		return
	}

	// Set headers for thumbnail (always JPEG)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year

	// Serve thumbnail
	_, _ = io.Copy(w, file)
}

// verifyFileContentFromBytes detects actual file content from bytes and validates it matches the extension
func (h *AttachmentHandler) verifyFileContentFromBytes(fileData []byte, filename string) (string, error) {
	// Use first 512 bytes for content detection (or less if file is smaller)
	detectSize := 512
	if len(fileData) < detectSize {
		detectSize = len(fileData)
	}

	// Detect actual content type from file content
	detectedType := http.DetectContentType(fileData[:detectSize])

	// Get expected type from file extension
	ext := filepath.Ext(filename)
	expectedType := mime.TypeByExtension(ext)

	// Validate content matches extension (if we have an expected type)
	if expectedType != "" {
		// Extract base type (before semicolon and parameters)
		detectedBase := strings.Split(detectedType, ";")[0]
		expectedBase := strings.Split(expectedType, ";")[0]

		// Allow octet-stream as it's a generic fallback.
		// Allow text/plain when the expected type is a text/* subtype, since
		// http.DetectContentType cannot distinguish between text subtypes
		// (e.g. CSV, XML, YAML are all detected as text/plain).
		if detectedBase != expectedBase && detectedBase != "application/octet-stream" &&
			(detectedBase != "text/plain" || !strings.HasPrefix(expectedBase, "text/")) {
			return "", fmt.Errorf("file content type (%s) doesn't match extension %s (expected %s)", detectedBase, ext, expectedBase)
		}
	}

	slog.Debug("content verification passed", slog.String("component", "attachments"), slog.String("filename", filename), slog.String("detected_type", detectedType))
	return detectedType, nil
}

// validateFileExtension checks if the file extension is allowed (not in dangerous list)
func (h *AttachmentHandler) validateFileExtension(filename string) error {
	// List of dangerous extensions that could be used for attacks
	dangerousExtensions := []string{
		".exe", ".bat", ".cmd", ".com", ".pif", ".scr", ".msi", // Windows executables
		".js", ".jsx", ".ts", ".tsx", // JavaScript/TypeScript (XSS risk)
		".html", ".htm", ".svg", // HTML/SVG (XSS risk)
		".sh", ".bash", ".zsh", ".fish", // Shell scripts
		".py", ".rb", ".pl", ".php", ".asp", ".aspx", ".jsp", // Server-side scripts
		".jar", ".class", ".dex", // Java/Android executables
		".app", ".dmg", ".pkg", // macOS executables/installers
		".deb", ".rpm", // Linux packages
		".apk", ".ipa", // Mobile app packages
	}

	ext := strings.ToLower(filepath.Ext(filename))

	// Check if extension is in the dangerous list
	for _, dangerous := range dangerousExtensions {
		if ext == dangerous {
			return fmt.Errorf("file extension %s is not allowed for security reasons", ext)
		}
	}

	// Additional check: reject files with no extension
	if ext == "" || ext == "." {
		return fmt.Errorf("files without extensions are not allowed")
	}

	slog.Debug("extension validation passed", slog.String("component", "attachments"), slog.String("extension", ext))
	return nil
}

// generateUniqueFilename creates a unique filename while preserving the extension
func (h *AttachmentHandler) generateUniqueFilename(originalFilename string) (string, error) {
	ext := filepath.Ext(originalFilename)

	// Generate random bytes for filename
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

	// Create hex string from random bytes
	randomStr := fmt.Sprintf("%x", randomBytes)

	return randomStr + ext, nil
}

// getAttachmentSettings retrieves current attachment settings
func (h *AttachmentHandler) getAttachmentSettings() (*models.AttachmentSettings, error) {
	settings := &models.AttachmentSettings{
		MaxFileSize:      52428800, // 50MB default
		AllowedMimeTypes: "",
		AttachmentPath:   h.attachmentPath,
		Enabled:          true,
	}

	// Try to get settings from database
	err := h.db.QueryRow(`
		SELECT max_file_size, allowed_mime_types, attachment_path, enabled
		FROM attachment_settings ORDER BY id DESC LIMIT 1
	`).Scan(&settings.MaxFileSize, &settings.AllowedMimeTypes, &settings.AttachmentPath, &settings.Enabled)

	if err == sql.ErrNoRows {
		// No settings in database, use defaults
		return settings, nil
	}
	if err != nil {
		return nil, err
	}

	return settings, nil
}

// generateThumbnail creates a thumbnail for an image file
func (h *AttachmentHandler) generateThumbnail(originalPath, filename string) (string, error) {
	slog.Debug("starting thumbnail generation", slog.String("component", "attachments"), slog.String("original_path", originalPath))

	// Open original image
	file, err := os.Open(originalPath)
	if err != nil {
		slog.Error("failed to open image file", slog.String("component", "attachments"), slog.Any("error", err))
		return "", err
	}
	defer func() { _ = file.Close() }()

	// Decode image
	slog.Debug("decoding image", slog.String("component", "attachments"))
	img, format, err := image.Decode(file)
	if err != nil {
		slog.Error("failed to decode image", slog.String("component", "attachments"), slog.Any("error", err))
		return "", err
	}
	slog.Debug("image decoded successfully", slog.String("component", "attachments"), slog.String("format", format))

	// Calculate thumbnail dimensions (max 200x200, maintaining aspect ratio)
	bounds := img.Bounds()
	origWidth := bounds.Dx()
	origHeight := bounds.Dy()
	slog.Debug("original dimensions", slog.String("component", "attachments"), slog.Int("width", origWidth), slog.Int("height", origHeight))

	maxSize := 200
	var newWidth, newHeight int

	if origWidth > origHeight {
		newWidth = maxSize
		newHeight = (origHeight * maxSize) / origWidth
	} else {
		newHeight = maxSize
		newWidth = (origWidth * maxSize) / origHeight
	}
	slog.Debug("thumbnail dimensions", slog.String("component", "attachments"), slog.Int("width", newWidth), slog.Int("height", newHeight))

	// Create thumbnail image
	slog.Debug("creating thumbnail image", slog.String("component", "attachments"))
	thumbnail := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	slog.Debug("scaling image", slog.String("component", "attachments"))
	draw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), img, bounds, draw.Over, nil)

	// Generate thumbnail filename (remove original extension, add .thumb.jpg)
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	thumbnailFilename := base + ".thumb.jpg"
	slog.Debug("thumbnail filename generated", slog.String("component", "attachments"), slog.String("thumbnail_filename", thumbnailFilename))

	// Create thumbnail path (same directory as original)
	thumbnailPath := filepath.Join(filepath.Dir(originalPath), thumbnailFilename)
	slog.Debug("thumbnail path", slog.String("component", "attachments"), slog.String("thumbnail_path", thumbnailPath))

	// Create thumbnail file
	slog.Debug("creating thumbnail file", slog.String("component", "attachments"))
	thumbnailFile, err := os.Create(thumbnailPath)
	if err != nil {
		slog.Error("failed to create thumbnail file", slog.String("component", "attachments"), slog.Any("error", err))
		return "", err
	}
	defer func() { _ = thumbnailFile.Close() }()

	// Encode as JPEG with good quality
	slog.Debug("encoding thumbnail as JPEG", slog.String("component", "attachments"))
	err = jpeg.Encode(thumbnailFile, thumbnail, &jpeg.Options{Quality: 85})
	if err != nil {
		slog.Error("failed to encode thumbnail", slog.String("component", "attachments"), slog.Any("error", err))
		return "", err
	}

	slog.Debug("thumbnail generation completed successfully", slog.String("component", "attachments"))
	return thumbnailPath, nil
}

// recordAttachmentHistory records attachment-related changes to item history
func (h *AttachmentHandler) recordAttachmentHistory(itemID int, userID *int, action string, oldValue *string, attachmentID int64, filename string) error {
	if userID == nil {
		return nil // Skip if no user context
	}

	var value string
	if action == "attachment_uploaded" {
		value = fmt.Sprintf("attachment:%d:%s", attachmentID, filename)
	} else {
		value = filename
	}

	query := `INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at)
	          VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`

	_, err := h.db.ExecWrite(query, itemID, *userID, action, oldValue, value)
	return err
}
