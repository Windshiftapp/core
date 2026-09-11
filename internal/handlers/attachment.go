package handlers

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
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
	"windshift/internal/fileserve"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"

	"golang.org/x/image/draw"
)

type AttachmentHandler struct {
	db                    database.Database
	attachmentPath        string
	permissionService     *services.PermissionService
	channelService        *services.ChannelService
	attachmentService     *services.AttachmentService
	approvalService       *services.ApprovalService       // for approver-derived item.view fallback (optional, may be nil)
	pagePermissionService *services.PagePermissionService // for entity_type='page' uploads (optional)
}

const maxThumbnailSourcePixels = 25_000_000

var errThumbnailSourceTooLarge = errors.New("thumbnail source dimensions exceed the pixel limit")

func NewAttachmentHandler(db database.Database, attachmentPath string, permissionService *services.PermissionService) *AttachmentHandler {
	return &AttachmentHandler{
		db:                db,
		attachmentPath:    attachmentPath,
		permissionService: permissionService,
		attachmentService: services.NewAttachmentServiceWithPermissions(db, permissionService),
	}
}

// SetApprovalService wires the approval service so that attachment list/read
// endpoints fall back to approver-pool membership when the caller lacks
// workspace item.view (mirrors the documented exception in approvals.go's Decide).
func (h *AttachmentHandler) SetApprovalService(ap *services.ApprovalService) {
	h.approvalService = ap
}

// SetPagePermissionService wires the page-permission evaluator so that
// uploads with entity_type='page' run the full ACL walk rather than just
// the workspace page.edit role check. Nil-safe: if unset, the page
// upload branch falls back to workspace page.edit only.
func (h *AttachmentHandler) SetPagePermissionService(ps *services.PagePermissionService) {
	h.pagePermissionService = ps
}

// SetChannelService wires channel-manager authorization for public portal
// branding uploads (portal logo/background). Without this, those uploads fail
// closed except for the system-admin fallback below.
func (h *AttachmentHandler) SetChannelService(cs *services.ChannelService) {
	h.channelService = cs
}

// authorizeTestCaseAttachmentAccess writes a 404 response and returns false
// when the caller cannot access attachments scoped to the given test_case
// (either the row is gone or the user lacks the requested test permission on
// its workspace). Mirrors the 404-not-403 invariant used for item attachments
// to avoid disclosing existence of resources outside the caller's reach.
func (h *AttachmentHandler) authorizeTestCaseAttachmentAccess(w http.ResponseWriter, r *http.Request, testCaseID int, permission string) bool {
	wsID, err := repository.NewTestCaseRepository(h.db).GetWorkspaceID(testCaseID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "attachment")
		return false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return false
	}
	if h.permissionService == nil {
		slog.Error("permission service unavailable, denying test case attachment access", slog.String("component", "attachments"))
		respondNotFound(w, r, "attachment")
		return false
	}
	allowed, err := h.permissionService.HasWorkspacePermission(user.ID, wsID, permission)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !allowed {
		respondNotFound(w, r, "attachment")
		return false
	}
	return true
}

// authorizeTestResultAttachmentAccess writes a 404 response and returns false
// when the caller lacks the requested permission on the test_result's
// workspace. Read paths pass PermissionTestView; the delete path passes
// PermissionTestExecute so that the privilege required to remove a result
// attachment matches the privilege required to upload it.
func (h *AttachmentHandler) authorizeTestResultAttachmentAccess(w http.ResponseWriter, r *http.Request, testResultID int, permission string) bool {
	wsID, err := repository.NewTestRunRepository(h.db).GetWorkspaceIDForResult(testResultID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "attachment")
		return false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return false
	}
	if h.permissionService == nil {
		slog.Error("permission service unavailable, denying test result attachment access", slog.String("component", "attachments"))
		respondNotFound(w, r, "attachment")
		return false
	}
	allowed, err := h.permissionService.HasWorkspacePermission(user.ID, wsID, permission)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !allowed {
		respondNotFound(w, r, "attachment")
		return false
	}
	return true
}

// IsEnabled checks if attachments are enabled (attachment path is set)
func (h *AttachmentHandler) IsEnabled() bool {
	return h.attachmentPath != ""
}

// authorizeUploadEntity verifies the upload target exists and the caller
// can modify it. Returns true if the upload may proceed; otherwise it has
// already written the response and the caller must return immediately.
//
// All failures respond with 404 (no existence disclosure) per the repo's
// item-permission invariant. The route itself is already auth-gated, so
// every branch can assume an authenticated session.
func (h *AttachmentHandler) authorizeUploadEntity(w http.ResponseWriter, r *http.Request, entityType string, entityID int) bool {
	switch entityType {
	case "test_case":
		return h.authorizeTestCaseAttachmentAccess(w, r, entityID, models.PermissionTestManage)

	case "test_result":
		// Resolve workspace via test_results -> test_runs and gate on test.execute.
		wsID, err := repository.NewTestRunRepository(h.db).GetWorkspaceIDForResult(entityID)
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "test_result")
			return false
		}
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		user, ok := RequireAuth(w, r)
		if !ok {
			return false
		}
		allowed, err := h.permissionService.HasWorkspacePermission(user.ID, wsID, models.PermissionTestExecute)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !allowed {
			respondNotFound(w, r, "test_result")
			return false
		}
		return true

	case "workspace_avatar", "workspace_background":
		// entity_id is the workspace id. Require workspace.admin on it. See WI-46.
		var exists bool
		if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", entityID).Scan(&exists); err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !exists {
			respondNotFound(w, r, "workspace")
			return false
		}
		user, ok := RequireAuth(w, r)
		if !ok {
			return false
		}
		allowed, err := h.permissionService.HasWorkspacePermission(user.ID, entityID, models.PermissionWorkspaceAdmin)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !allowed {
			respondNotFound(w, r, "workspace")
			return false
		}
		return true

	case "team_avatar":
		// entity_id is the team id. Mirror canManageTeam in teams.go:
		// teams.manage global perm OR team admin role on this team.
		teamRepo := repository.NewTeamRepository(h.db)
		if _, err := teamRepo.GetByID(entityID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				respondNotFound(w, r, "team")
				return false
			}
			respondInternalError(w, r, err)
			return false
		}
		user, ok := RequireAuth(w, r)
		if !ok {
			return false
		}
		hasPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionTeamsManage)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if hasPerm {
			return true
		}
		isAdmin, err := teamRepo.IsTeamAdmin(entityID, user.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !isAdmin {
			respondNotFound(w, r, "team")
			return false
		}
		return true

	case "customer_avatar":
		// customer_organisations are global (no workspace_id); gate on the
		// global customers.manage permission.
		var exists bool
		if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM customer_organisations WHERE id = ?)", entityID).Scan(&exists); err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !exists {
			respondNotFound(w, r, "customer")
			return false
		}
		user, ok := RequireAuth(w, r)
		if !ok {
			return false
		}
		hasPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionCustomersManage)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !hasPerm {
			respondNotFound(w, r, "customer")
			return false
		}
		return true

	case "portal_background", "portal_logo":
		// Public portal branding URLs are served anonymously via
		// /api/portal-assets/{id}, so the upload itself must be authorized —
		// not just the later bind into channel config. entityID is the channel
		// id supplied by the customization UI.
		user, ok := RequireAuth(w, r)
		if !ok {
			return false
		}
		if entityID <= 0 {
			respondValidationError(w, r, "entity_id is required for portal branding uploads")
			return false
		}
		if h.channelService != nil {
			canManage, err := h.channelService.UserCanManage(r.Context(), user.ID, entityID)
			if err != nil {
				respondInternalError(w, r, err)
				return false
			}
			if !canManage {
				respondNotFound(w, r, "channel")
				return false
			}
			return true
		}
		if h.permissionService != nil {
			isAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
			if err != nil {
				respondInternalError(w, r, err)
				return false
			}
			if isAdmin {
				return true
			}
		}
		respondNotFound(w, r, "channel")
		return false

	case "hub_logo", "theme_logo":
		// Hub and theme branding are system-admin surfaces (PUT /api/hub/config
		// and the admin-only theme routes); keep their public logo uploads
		// aligned with those authorization contracts.
		user, ok := RequireAuth(w, r)
		if !ok {
			return false
		}
		if h.permissionService == nil {
			respondForbidden(w, r)
			return false
		}
		isAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !isAdmin {
			respondForbidden(w, r)
			return false
		}
		return true

	case "avatar":
		// User's own profile picture. Auth-gated at the route.
		if _, ok := RequireAuth(w, r); !ok {
			return false
		}
		return true

	default:
		respondValidationError(w, r, "Unknown entity type")
		return false
	}
}

// Upload handles file upload to an item
func (h *AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	// FIXME: split authorization from storage concerns.
	slog.Debug("upload request received", slog.String("component", "attachments"))

	if !h.IsEnabled() {
		slog.Warn("upload failed: attachments not enabled", slog.String("component", "attachments"))
		respondServiceUnavailable(w, r, "Attachments are not enabled on this server")
		return
	}

	// Cap the request before parsing.
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	slog.Debug("parsing multipart form", slog.String("component", "attachments"))
	// #nosec G120 -- the body is already capped by MaxBytesReader above; the int arg is the in-memory threshold, not the upper bound
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		slog.Error("failed to parse form data", slog.String("component", "attachments"), slog.Any("error", err))
		respondBadRequest(w, r, "Failed to parse form data: "+err.Error())
		return
	}

	// Accept legacy item_id and current entity parameters.
	entityIDStr := r.FormValue("entity_id")
	if entityIDStr == "" {
		entityIDStr = r.FormValue("item_id") // Backwards compatibility
	}
	entityType := r.FormValue("entity_type")
	category := r.FormValue("category")

	// Derive a missing entity type for legacy clients.
	if entityType == "" {
		switch category {
		case "avatar":
			entityType = "avatar"
		case "workspace_avatar":
			entityType = "workspace_avatar"
		case "team_avatar":
			entityType = "team_avatar"
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
		case "theme_logo":
			entityType = "theme_logo"
		default:
			entityType = "item" // Default to item for backwards compatibility
		}
	}

	slog.Debug("entity info received", slog.String("component", "attachments"), slog.String("entity_id", entityIDStr), slog.String("entity_type", entityType), slog.String("category", category))

	// Handle avatar uploads differently (they don't need a real entity)
	isAvatar := entityType == "avatar"
	isWorkspaceAvatar := entityType == "workspace_avatar"
	isTeamAvatar := entityType == "team_avatar"
	isCustomerAvatar := entityType == "customer_avatar"
	isWorkspaceBackground := entityType == "workspace_background"
	isPortalBackground := entityType == "portal_background"
	isPortalLogo := entityType == "portal_logo"
	isHubLogo := entityType == "hub_logo"
	isThemeLogo := entityType == "theme_logo"
	isImageEntityType := isAvatar || isWorkspaceAvatar || isTeamAvatar || isCustomerAvatar || isWorkspaceBackground || isPortalBackground || isPortalLogo || isHubLogo || isThemeLogo

	// category is an older image-asset discriminator used by /api/portal-assets.
	// Keep it in lockstep with entity_type so callers cannot make an item/test
	// attachment public by tagging it as portal_logo/portal_background.
	if category != "" && category != entityType {
		respondValidationError(w, r, "category must match entity_type")
		return
	}
	if category == "" && isImageEntityType {
		category = entityType
	}

	// entity_id is required for every entity type that has an owner row
	// (item, test_case, test_result, workspace/team/customer scoped image
	// assets). Portal logo/background uploads use entity_id as the owning
	// channel id so public URLs cannot be minted by arbitrary users. The truly
	// global uploads — user avatar, hub logo, and theme logo — need no owner
	// id here.
	entityIDRequired := entityType != "avatar" &&
		entityType != "hub_logo" &&
		entityType != "theme_logo"
	if entityIDStr == "" && entityIDRequired {
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

	if entityType == "item" || entityType == "page" {
		h.uploadDomainAttachment(w, r, entityType, entityID)
		return
	}

	if !h.authorizeUploadEntity(w, r, entityType, entityID) {
		return
	}

	slog.Debug("getting file from form", slog.String("component", "attachments"))
	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		slog.Error("failed to get file from form", slog.String("component", "attachments"), slog.Any("error", err))
		respondBadRequest(w, r, "Failed to get file from form: "+err.Error())
		return
	}
	defer func() { _ = file.Close() }()
	slog.Debug("file received", slog.String("component", "attachments"), slog.String("filename", fileHeader.Filename), slog.Int64("size", fileHeader.Size), slog.String("content_type", fileHeader.Header.Get("Content-Type")))

	slog.Debug("reading file into memory", slog.String("component", "attachments"))
	fileData, err := io.ReadAll(file)
	if err != nil {
		slog.Error("failed to read file data", slog.String("component", "attachments"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to read file data: %w", err))
		return
	}
	slog.Debug("file data read", slog.String("component", "attachments"), slog.Int("bytes", len(fileData)))

	// Validate both the filename and detected content before writing anything.
	slog.Debug("validating file extension", slog.String("component", "attachments"))
	if err = h.validateFileExtension(fileHeader.Filename); err != nil {
		slog.Warn("extension validation failed", slog.String("component", "attachments"), slog.Any("error", err))
		respondValidationError(w, r, err.Error())
		return
	}

	slog.Debug("verifying file content", slog.String("component", "attachments"))
	detectedMimeType, err := h.verifyFileContentFromBytes(fileData, fileHeader.Filename)
	if err != nil {
		slog.Warn("content verification failed", slog.String("component", "attachments"), slog.Any("error", err))
		respondValidationError(w, r, "File content validation failed: "+err.Error())
		return
	}
	slog.Debug("content verified", slog.String("component", "attachments"), slog.String("mime_type", detectedMimeType))

	if isImageEntityType {
		if !isAllowedImageExtension(fileHeader.Filename) {
			respondValidationError(w, r, fmt.Sprintf(
				"File extension %s is not allowed for %s uploads. Only image files are accepted",
				strings.ToLower(filepath.Ext(fileHeader.Filename)), entityType))
			return
		}
	}

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

	// Store files under an entity-specific directory with a generated name.
	slog.Debug("generating unique filename", slog.String("component", "attachments"), slog.String("original_filename", fileHeader.Filename))
	uniqueFilename, err := h.generateUniqueFilename(fileHeader.Filename)
	if err != nil {
		slog.Error("failed to generate filename", slog.String("component", "attachments"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to generate filename: %w", err))
		return
	}
	slog.Debug("generated filename", slog.String("component", "attachments"), slog.String("unique_filename", uniqueFilename))

	var itemDir string
	switch entityType {
	case "avatar":
		itemDir = filepath.Join(h.attachmentPath, "avatars")
	case "workspace_avatar":
		itemDir = filepath.Join(h.attachmentPath, "workspace_avatars")
	case "team_avatar":
		itemDir = filepath.Join(h.attachmentPath, "team_avatars")
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
	case "theme_logo":
		itemDir = filepath.Join(h.attachmentPath, "theme_logos")
	case "test_case":
		itemDir = filepath.Join(h.attachmentPath, "test_cases", strconv.Itoa(entityID))
	case "test_result":
		itemDir = filepath.Join(h.attachmentPath, "test_results", strconv.Itoa(entityID))
	default: // "item"
		itemDir = filepath.Join(h.attachmentPath, "items", strconv.Itoa(entityID))
	}
	slog.Debug("creating directory", slog.String("component", "attachments"), slog.String("path", itemDir))
	if err = os.MkdirAll(itemDir, 0o750); err != nil { //nolint:gosec // G703: path built from hardcoded strings + strconv.Itoa(entityID)
		slog.Error("failed to create directory", slog.String("component", "attachments"), slog.String("path", itemDir), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to create attachment directory: %w", err))
		return
	}

	filePath := filepath.Join(itemDir, uniqueFilename)
	slog.Debug("creating file", slog.String("component", "attachments"), slog.String("path", filePath))

	slog.Debug("writing file data", slog.String("component", "attachments"))
	err = os.WriteFile(filePath, fileData, 0o600) //nolint:gosec // G703: path from hardcoded base + strconv.Itoa
	if err != nil {
		slog.Error("failed to write file", slog.String("component", "attachments"), slog.String("path", filePath), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to save file: %w", err))
		return
	}
	fileSize := int64(len(fileData))
	slog.Debug("file saved", slog.String("component", "attachments"), slog.Int64("bytes", fileSize))

	var uploaderID *int
	if user := utils.GetCurrentUser(r); user != nil {
		uploaderID = &user.ID
	}

	// Use the detected MIME type from content verification (not client header)
	mimeType := detectedMimeType

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

	var attachmentEntityID any
	if isImageEntityType {
		attachmentEntityID = nil
	} else {
		attachmentEntityID = entityID
	}
	slog.Debug("attachment saved", slog.String("component", "attachments"), slog.Int64("attachment_id", attachmentID))

	if entityType == "item" && attachmentEntityID != nil {
		if entityIDInt, ok := attachmentEntityID.(int); ok {
			if err = h.attachmentService.RecordItemHistory(entityIDInt, uploaderID, "attachment_uploaded", nil, attachmentID, fileHeader.Filename); err != nil {
				slog.Warn("failed to record attachment history", slog.String("component", "attachments"), slog.Any("error", err))
				// Don't fail the whole operation if history recording fails
			}
		}
	}

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

	if isImageEntityType {
		// Image uploads use public or authenticated download URLs by entity type.
		var downloadURL string
		if isPortalBackground || isPortalLogo || isHubLogo || isThemeLogo {
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
		case isTeamAvatar:
			message = "Team avatar uploaded successfully"
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
		case isThemeLogo:
			message = "Theme logo uploaded successfully"
			urlKey = "logo_url"
		}
		response := map[string]any{
			"success":       true,
			"message":       message,
			urlKey:          downloadURL,
			"attachment_id": attachmentID,
			"filename":      uniqueFilename,
		}
		respondJSONOK(w, response)
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
		respondJSONOK(w, response)
	}
}

func (h *AttachmentHandler) uploadDomainAttachment(w http.ResponseWriter, r *http.Request, entityType string, entityID int) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, r, "Failed to get file from form: "+err.Error())
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(file)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to read file data: %w", err))
		return
	}

	var response models.AttachmentUploadResponse
	switch entityType {
	case "item":
		response, err = services.NewItemAttachmentService(h.db, h.attachmentPath, h.permissionService).UploadItemAttachment(services.ItemAttachmentUploadInput{
			ItemID: entityID, UploaderID: user.ID, OriginalFilename: header.Filename,
			FileData: data, FileSize: int64(len(data)),
		})
	case "page":
		response, err = services.NewPageAttachmentUploadService(h.db, h.attachmentPath, h.permissionService, h.pagePermissionService).UploadPageAttachment(services.PageAttachmentUploadInput{
			PageID: entityID, UploaderID: user.ID, OriginalFilename: header.Filename,
			FileData: data, FileSize: int64(len(data)),
		})
	}
	if err != nil {
		h.respondDomainUploadError(w, r, entityType, err)
		return
	}
	respondJSONOK(w, response)
}

func (h *AttachmentHandler) respondDomainUploadError(w http.ResponseWriter, r *http.Request, entityType string, err error) {
	if errors.Is(err, services.ErrItemAttachmentDisabled) || errors.Is(err, services.ErrPageAttachmentUploadDisabled) {
		respondServiceUnavailable(w, r, "Attachments are not enabled on this server")
		return
	}
	if errors.Is(err, services.ErrItemAttachmentNotFound) || errors.Is(err, services.ErrPageAttachmentUploadNotFound) {
		respondNotFound(w, r, entityType)
		return
	}
	for _, sentinel := range []error{services.ErrItemAttachmentInvalid, services.ErrPageAttachmentUploadInvalid} {
		if errors.Is(err, sentinel) {
			respondValidationError(w, r, strings.TrimPrefix(err.Error(), sentinel.Error()+": "))
			return
		}
	}
	respondInternalError(w, r, err)
}

// GetByItem returns attachments for a specific item with pagination support
func (h *AttachmentHandler) GetByItem(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "itemId")
	if !ok {
		return
	}

	var err error

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	workspaceID, err := repository.NewItemRepository(h.db).GetWorkspaceID(itemID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check workspace view permission. Active approvers without workspace
	// item.view are allowed through so they can browse attachments on the
	// item they're reviewing.
	if h.permissionService != nil {
		var canView bool
		canView, err = userCanViewItemAsActor(r.Context(), user.ID, itemID, workspaceID, h.permissionService, h.approvalService)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !canView {
			respondNotFound(w, r, "item")
			return
		}
	}

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

	var totalCount int
	err = h.db.QueryRow(`
		SELECT COUNT(*) FROM attachments WHERE item_id = ? AND entity_type = 'item'
	`, itemID).Scan(&totalCount)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rows, err := h.db.Query(`
		SELECT a.id, a.item_id, a.filename, a.original_filename, a.mime_type, a.file_size,
		       a.uploaded_by, a.has_thumbnail, a.created_at,
		       u.first_name || ' ' || u.last_name as uploader_name, u.email as uploader_email
		FROM attachments a
		LEFT JOIN users u ON a.uploaded_by = u.id
		WHERE a.item_id = ? AND a.entity_type = 'item'
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
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
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

	respondJSONOK(w, response)
}

// authorizeAttachmentRead gates download/thumbnail access by entity type.
// Denials return 404 to avoid disclosure; callers return after a false result.
func (h *AttachmentHandler) authorizeAttachmentRead(w http.ResponseWriter, r *http.Request, entityType string, itemID *int) bool {
	switch entityType {
	case "test_case":
		if itemID == nil {
			respondNotFound(w, r, "attachment")
			return false
		}
		return h.authorizeTestCaseAttachmentAccess(w, r, *itemID, models.PermissionTestView)
	case "test_result":
		if itemID == nil {
			respondNotFound(w, r, "attachment")
			return false
		}
		return h.authorizeTestResultAttachmentAccess(w, r, *itemID, models.PermissionTestView)
	case "item", "":
		// Empty legacy types are items; missing item IDs are corrupt and never served.
		if itemID == nil {
			respondNotFound(w, r, "attachment")
			return false
		}
		return CheckItemPermissionAsActor(w, r, repository.NewItemRepository(h.db), h.permissionService, h.approvalService, *itemID, models.PermissionItemView)
	case "page":
		// Honor per-page ACLs, falling back to workspace page.view in degraded mode.
		if itemID == nil {
			respondNotFound(w, r, "attachment")
			return false
		}
		var wsID int
		if err := h.db.QueryRow(`SELECT workspace_id FROM pages WHERE id = ?`, *itemID).Scan(&wsID); err != nil {
			respondNotFound(w, r, "attachment")
			return false
		}
		user, ok := RequireAuth(w, r)
		if !ok {
			return false
		}
		if h.pagePermissionService != nil {
			can, perr := h.pagePermissionService.Can(user.ID, wsID, *itemID, services.PageOpView)
			if perr != nil {
				respondInternalError(w, r, perr)
				return false
			}
			if !can {
				respondNotFound(w, r, "attachment")
				return false
			}
			return true
		}
		allowed, perr := h.permissionService.HasWorkspacePermission(user.ID, wsID, models.PermissionPageView)
		if perr != nil || !allowed {
			respondNotFound(w, r, "attachment")
			return false
		}
		return true
	case "avatar",
		"workspace_avatar", "workspace_background",
		"team_avatar", "customer_avatar":
		// Branding / profile assets are non-secret and rendered widely. The
		// route is auth-gated so portal customer sessions can't reach here.
		_, ok := RequireAuth(w, r)
		return ok
	case "portal_background", "portal_logo", "hub_logo":
		// Canonical access route is /api/portal-assets/{id}; refuse to serve
		// portal/hub branding through the cookie-auth path so there is exactly
		// one place that gates them.
		respondNotFound(w, r, "attachment")
		return false
	default:
		respondNotFound(w, r, "attachment")
		return false
	}
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
	var entityType sql.NullString
	err = h.db.QueryRow(`
		SELECT id, item_id, entity_type, filename, original_filename, file_path, mime_type, file_size
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(
		&attachment.ID, &itemID, &entityType, &attachment.Filename, &attachment.OriginalFilename,
		&attachment.FilePath, &attachment.MimeType, &attachment.FileSize,
	)

	if errors.Is(err, sql.ErrNoRows) {
		slog.Debug("attachment not found in database", slog.String("component", "attachments"), slog.Int("attachment_id", attachmentID))
		respondNotFound(w, r, "attachment")
		return
	}
	if err != nil {
		slog.Error("database error", slog.String("component", "attachments"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}
	if itemID.Valid {
		id := int(itemID.Int64)
		attachment.ItemID = &id
	}
	slog.Debug("found attachment", slog.String("component", "attachments"), slog.String("original_filename", attachment.OriginalFilename), slog.String("path", attachment.FilePath))

	// Authorize per entity_type. Before WI-46 the default branch treated a
	// non-NULL item_id as a work-item id and ran CheckItemPermissionAsActor
	// on it, which was wrong for branding rows whose item_id was actually
	// the workspace/portal/hub id. Now every type is explicit.
	if !h.authorizeAttachmentRead(w, r, entityType.String, attachment.ItemID) {
		return
	}

	// Open the file confined to the attachment storage root. os.OpenRoot (used
	// by fileserve.OpenUnderRoot) rejects ".." traversal and symlink escapes,
	// so a malicious stored path or a planted symlink cannot read outside the
	// root. Email ingestion stores relative paths; those resolve against the
	// same root. Escapes and missing files both return 404 to avoid disclosing
	// filesystem details.
	slog.Debug("opening file under attachment root", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
	file, err := fileserve.OpenUnderRoot(h.attachmentPath, attachment.FilePath)
	if err != nil {
		if errors.Is(err, fileserve.ErrOutsideRoot) {
			slog.Warn("path traversal attempt blocked", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
			respondNotFound(w, r, "file")
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			slog.Debug("file not found on disk", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
			respondNotFound(w, r, "file")
			return
		}
		slog.Error("failed to open file", slog.String("component", "attachments"), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to open file: %w", err))
		return
	}
	defer func() { _ = file.Close() }()

	// Set headers
	slog.Debug("setting headers and serving file", slog.String("component", "attachments"), slog.String("original_filename", attachment.OriginalFilename))
	w.Header().Set("Content-Type", attachment.MimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(attachment.FileSize, 10))

	// Prevent MIME sniffing and embedding.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	// Inline only allowlisted types; MIME may be remote-declared. Other files
	// download with a restrictive CSP to prevent app-origin execution.
	if isInlineSafeMimeType(attachment.MimeType) {
		w.Header().Set("Content-Disposition", fileserve.ContentDisposition("inline", attachment.OriginalFilename))
	} else {
		w.Header().Set("Content-Disposition", fileserve.ContentDisposition("attachment", attachment.OriginalFilename))
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		slog.Debug("forcing download for non-allowlisted file type", slog.String("component", "attachments"), slog.String("mime_type", attachment.MimeType))
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
	attachmentID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context for history tracking and audit context.
	currentUser := utils.GetCurrentUser(r)

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
	if details.EntityType == "item" || details.EntityType == "" {
		if currentUser == nil || details.ItemID == nil {
			respondNotFound(w, r, "attachment")
			return
		}
		itemAttachments := services.NewItemAttachmentService(h.db, h.attachmentPath, h.permissionService)
		if err := itemAttachments.DeleteItemAttachment(attachmentID, currentUser.ID); err != nil {
			if errors.Is(err, services.ErrItemAttachmentNotFound) {
				respondNotFound(w, r, "attachment")
				return
			}
			respondInternalError(w, r, err)
			return
		}
		logAuditWithDetails(h.db, r, currentUser, logger.ActionAttachmentDelete, logger.ResourceAttachment, &attachmentID, details.OriginalFilename, map[string]any{
			"entity_type":       details.EntityType,
			"item_id":           details.ItemID,
			"original_filename": details.OriginalFilename,
		})
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Authorize each entity type explicitly. Missing item IDs return 404;
	// parent-owned branding avatars are refused, except uploader-owned profiles.
	switch details.EntityType {
	case "test_case":
		if details.ItemID == nil {
			respondNotFound(w, r, "attachment")
			return
		}
		if !h.authorizeTestCaseAttachmentAccess(w, r, *details.ItemID, models.PermissionTestManage) {
			return
		}
	case "test_result":
		if details.ItemID == nil {
			respondNotFound(w, r, "attachment")
			return
		}
		if !h.authorizeTestResultAttachmentAccess(w, r, *details.ItemID, models.PermissionTestExecute) {
			return
		}
	case "page":
		// Page attachment deletes require page-edit on the owning page.
		if details.ItemID == nil {
			respondNotFound(w, r, "attachment")
			return
		}
		var wsID int
		if err = h.db.QueryRow(`SELECT workspace_id FROM pages WHERE id = ?`, *details.ItemID).Scan(&wsID); err != nil {
			respondNotFound(w, r, "attachment")
			return
		}
		user, ok := RequireAuth(w, r)
		if !ok {
			return
		}
		if h.pagePermissionService != nil {
			can, perr := h.pagePermissionService.Can(user.ID, wsID, *details.ItemID, services.PageOpEdit)
			if perr != nil || !can {
				respondNotFound(w, r, "attachment")
				return
			}
		} else {
			allowed, perr := h.permissionService.HasWorkspacePermission(user.ID, wsID, models.PermissionPageEdit)
			if perr != nil || !allowed {
				respondNotFound(w, r, "attachment")
				return
			}
		}
	case "avatar":
		user, authed := RequireAuth(w, r)
		if !authed {
			return
		}
		if details.UploadedBy == nil || *details.UploadedBy != user.ID {
			respondNotFound(w, r, "attachment")
			return
		}
	default:
		// workspace_avatar, workspace_background, team_avatar,
		// customer_avatar, portal_background, portal_logo, hub_logo,
		// theme_logo, and any unknown entity_type. Refuse — the parent
		// entity owns the lifecycle. Unknown types likewise default-deny so a
		// future entity_type can't accidentally land in a permissive branch.
		respondNotFound(w, r, "attachment")
		return
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

	if currentUser != nil {
		logAuditWithDetails(h.db, r, currentUser, logger.ActionAttachmentDelete, logger.ResourceAttachment, &attachmentID, details.OriginalFilename, map[string]any{
			"entity_type":       details.EntityType,
			"item_id":           details.ItemID,
			"original_filename": details.OriginalFilename,
		})
	}

	// Delete physical file using the same root-confined resolver as downloads.
	// Missing files are tolerated so stale DB rows can still be cleaned up.
	if err := fileserve.RemoveUnderRoot(h.attachmentPath, details.FilePath); err != nil && !os.IsNotExist(err) {
		if errors.Is(err, fileserve.ErrOutsideRoot) {
			slog.Warn("refusing to delete attachment file outside storage root", slog.String("component", "attachments"), slog.String("file_path", details.FilePath), slog.Any("error", err))
		} else {
			// Log warning but don't fail the request if file removal fails.
			slog.Warn("failed to delete attachment file", slog.String("component", "attachments"), slog.String("file_path", details.FilePath), slog.Any("error", err))
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// Thumbnail serves a thumbnail for an image attachment
func (h *AttachmentHandler) Thumbnail(w http.ResponseWriter, r *http.Request) {
	attachmentID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	var hasThumbnail bool
	var thumbnailPath string
	var mimeType string
	var thumbItemID sql.NullInt64
	var thumbEntityType sql.NullString
	err = h.db.QueryRow(`
		SELECT has_thumbnail, thumbnail_path, mime_type, item_id, entity_type
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(&hasThumbnail, &thumbnailPath, &mimeType, &thumbItemID, &thumbEntityType)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "attachment")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Authorize per entity_type — mirrors Download. See WI-46 commit notes.
	var thumbItemPtr *int
	if thumbItemID.Valid {
		id := int(thumbItemID.Int64)
		thumbItemPtr = &id
	}
	if !h.authorizeAttachmentRead(w, r, thumbEntityType.String, thumbItemPtr) {
		return
	}

	if !hasThumbnail || thumbnailPath == "" {
		respondNotFound(w, r, "thumbnail")
		return
	}

	// Open the thumbnail confined to the attachment storage root (see Download
	// for the rationale). Escapes and missing files both surface as 404.
	file, err := fileserve.OpenUnderRoot(h.attachmentPath, thumbnailPath)
	if err != nil {
		if errors.Is(err, fileserve.ErrOutsideRoot) {
			slog.Warn("thumbnail path traversal attempt blocked", slog.String("component", "attachments"), slog.String("thumbnail_path", thumbnailPath))
			respondNotFound(w, r, "thumbnail")
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			respondNotFound(w, r, "thumbnail")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to open thumbnail: %w", err))
		return
	}
	defer func() { _ = file.Close() }()

	fileInfo, err := file.Stat()
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get file info: %w", err))
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year

	_, _ = io.Copy(w, file)
}

// verifyFileContentFromBytes detects actual file content from bytes and validates it matches the extension
func (h *AttachmentHandler) verifyFileContentFromBytes(fileData []byte, filename string) (string, error) {
	detectSize := 512
	if len(fileData) < detectSize {
		detectSize = len(fileData)
	}

	detectedType := http.DetectContentType(fileData[:detectSize])

	ext := filepath.Ext(filename)
	expectedType := mime.TypeByExtension(ext)

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
			// Allow application/zip when the expected type is a known ZIP-based container format.
			// These formats share ZIP magic bytes (PK header), so http.DetectContentType
			// identifies them as application/zip. Return the expected type for correct
			// downstream MIME handling.
			if detectedBase == "application/zip" && isZipBasedMimeType(expectedBase) {
				return expectedType, nil
			}
			// http.DetectContentType has no JSON sniffer, so .json files come
			// back as text/plain. Accept when the payload is structurally JSON.
			if detectedBase == "text/plain" && expectedBase == "application/json" && looksLikeJSON(fileData) {
				return expectedType, nil
			}
			return "", fmt.Errorf("file content type (%s) doesn't match extension %s (expected %s)", detectedBase, ext, expectedBase)
		}
	}

	slog.Debug("content verification passed", slog.String("component", "attachments"), slog.String("filename", filename), slog.String("detected_type", detectedType))
	return detectedType, nil
}

// inlineSafeMimeTypes is the allowlist of Content-Types that may be served with
// an inline Content-Disposition. Every entry is a non-executable media type that
// a browser renders without running embedded script. Anything outside this set —
// including HTML/XHTML/SVG/MathML and any unknown type — is forced to download.
var inlineSafeMimeTypes = map[string]struct{}{
	"image/png":                {},
	"image/jpeg":               {},
	"image/gif":                {},
	"image/webp":               {},
	"image/bmp":                {},
	"image/x-icon":             {},
	"image/vnd.microsoft.icon": {},
	"application/pdf":          {},
	"text/plain":               {},
	"audio/mpeg":               {},
	"audio/ogg":                {},
	"audio/wav":                {},
	"audio/x-wav":              {},
	"audio/webm":               {},
	"video/mp4":                {},
	"video/webm":               {},
	"video/ogg":                {},
}

// isInlineSafeMimeType reports whether a stored Content-Type may be served
// inline. The declared type (before any parameters such as "; charset=") is
// matched case-insensitively against the allowlist; SVG is deliberately absent
// because it can carry script.
func isInlineSafeMimeType(mimeType string) bool {
	base := strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(base, ';'); i >= 0 {
		base = strings.TrimSpace(base[:i])
	}
	_, ok := inlineSafeMimeTypes[base]
	return ok
}

// isZipBasedMimeType returns true if the MIME type is a known ZIP-based container format.
// These formats use ZIP as their container (PK magic bytes), so http.DetectContentType
// will report them as application/zip.
func isZipBasedMimeType(mimeType string) bool {
	return mimeType == "application/zip" ||
		strings.Contains(mimeType, "openxmlformats") ||
		strings.Contains(mimeType, "opendocument") ||
		mimeType == "application/epub+zip"
}

// looksLikeJSON returns true if data parses as JSON. Used to rescue .json
// uploads that http.DetectContentType reports as text/plain.
func looksLikeJSON(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return false
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return false
	}
	return json.Valid(trimmed)
}

// validateFileExtension checks if the file extension is allowed (not in dangerous list)
func (h *AttachmentHandler) validateFileExtension(filename string) error {
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

	for _, dangerous := range dangerousExtensions {
		if ext == dangerous {
			return fmt.Errorf("file extension %s is not allowed for security reasons", ext)
		}
	}

	if ext == "" || ext == "." {
		return fmt.Errorf("files without extensions are not allowed")
	}

	slog.Debug("extension validation passed", slog.String("component", "attachments"), slog.String("extension", ext))
	return nil
}

// isAllowedImageExtension checks if the file extension is a known image format.
// Used for image-only entity types (avatars, backgrounds, logos) as defense-in-depth.
func isAllowedImageExtension(filename string) bool {
	allowed := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".webp": true, ".bmp": true, ".ico": true, ".tiff": true, ".tif": true,
	}
	return allowed[strings.ToLower(filepath.Ext(filename))]
}

// generateUniqueFilename creates a unique filename while preserving the extension
func (h *AttachmentHandler) generateUniqueFilename(originalFilename string) (string, error) {
	ext := filepath.Ext(originalFilename)

	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}

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

	if errors.Is(err, sql.ErrNoRows) {
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
	file, err := os.Open(originalPath) //nolint:gosec // path from server-managed attachment storage
	if err != nil {
		slog.Error("failed to open image file", slog.String("component", "attachments"), slog.Any("error", err))
		return "", err
	}
	defer func() { _ = file.Close() }()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		slog.Error("failed to read image dimensions", slog.String("component", "attachments"), slog.Any("error", err))
		return "", err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxThumbnailSourcePixels/config.Height {
		return "", fmt.Errorf("%w: %dx%d exceeds %d pixels", errThumbnailSourceTooLarge, config.Width, config.Height, maxThumbnailSourcePixels)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind image after dimension check: %w", err)
	}

	// Decode image after its declared dimensions pass the allocation bound.
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
	thumbnailFile, err := os.Create(thumbnailPath) //nolint:gosec // path derived from server-managed attachment storage
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
