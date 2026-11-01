package handlers

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"windshift/internal/blobstore"
	"windshift/internal/database"
	"windshift/internal/fileserve"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type AttachmentHandler struct {
	db                    database.Database
	storage               *blobstore.Store
	permissionService     *services.PermissionService
	channelService        *services.ChannelService
	attachmentService     *services.AttachmentService
	approvalService       *services.ApprovalService       // for approver-derived item.view fallback (optional, may be nil)
	pagePermissionService *services.PagePermissionService // for entity_type='page' uploads (optional)
}

func NewAttachmentHandler(db database.Database, storage *blobstore.Store, permissionService *services.PermissionService) *AttachmentHandler {
	return &AttachmentHandler{
		db:                db,
		storage:           storage,
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
	return h.storage.Enabled()
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

// attachmentKind is the single classification of an upload target. It replaces
// the string comparisons and per-type switches that used to be spread across
// Upload: collection path, entity-id requirement, image-only policy, public
// asset URL, and response shape.
type attachmentKind struct {
	entityType       string
	collection       string
	scopedByEntity   bool
	domain           bool
	imageOnly        bool
	requiresEntityID bool
	publicAsset      bool
	urlKey           string
	message          string
}

var errUnknownAttachmentType = errors.New("unknown attachment entity type")

func classifyAttachment(entityType, category string) (attachmentKind, error) {
	if entityType == "" {
		switch category {
		case "avatar", "workspace_avatar", "team_avatar", "customer_avatar",
			"workspace_background", "portal_background", "portal_logo",
			"hub_logo", "theme_logo":
			entityType = category
		default:
			entityType = "item" // legacy default
		}
	}

	switch entityType {
	case "item":
		return attachmentKind{entityType: "item", collection: "items", scopedByEntity: true, domain: true, requiresEntityID: true}, nil
	case "page":
		return attachmentKind{entityType: "page", collection: "pages", scopedByEntity: true, domain: true, requiresEntityID: true}, nil
	case "test_case":
		return attachmentKind{entityType: "test_case", collection: "test_cases", scopedByEntity: true, requiresEntityID: true}, nil
	case "test_result":
		return attachmentKind{entityType: "test_result", collection: "test_results", scopedByEntity: true, requiresEntityID: true}, nil
	case "avatar":
		return attachmentKind{entityType: "avatar", collection: "avatars", imageOnly: true, urlKey: "avatar_url", message: "Avatar uploaded successfully"}, nil
	case "workspace_avatar":
		return attachmentKind{entityType: "workspace_avatar", collection: "workspace_avatars", imageOnly: true, requiresEntityID: true, urlKey: "avatar_url", message: "Workspace avatar uploaded successfully"}, nil
	case "team_avatar":
		return attachmentKind{entityType: "team_avatar", collection: "team_avatars", imageOnly: true, requiresEntityID: true, urlKey: "avatar_url", message: "Team avatar uploaded successfully"}, nil
	case "customer_avatar":
		return attachmentKind{entityType: "customer_avatar", collection: "customer_avatars", imageOnly: true, requiresEntityID: true, urlKey: "avatar_url", message: "Customer avatar uploaded successfully"}, nil
	case "workspace_background":
		return attachmentKind{entityType: "workspace_background", collection: "workspace_backgrounds", imageOnly: true, requiresEntityID: true, urlKey: "background_url", message: "Workspace background uploaded successfully"}, nil
	case "portal_background":
		return attachmentKind{entityType: "portal_background", collection: "portal_backgrounds", imageOnly: true, requiresEntityID: true, publicAsset: true, urlKey: "background_url", message: "Portal background uploaded successfully"}, nil
	case "portal_logo":
		return attachmentKind{entityType: "portal_logo", collection: "portal_logos", imageOnly: true, requiresEntityID: true, publicAsset: true, urlKey: "logo_url", message: "Portal logo uploaded successfully"}, nil
	case "hub_logo":
		return attachmentKind{entityType: "hub_logo", collection: "hub_logos", imageOnly: true, publicAsset: true, urlKey: "logo_url", message: "Hub logo uploaded successfully"}, nil
	case "theme_logo":
		return attachmentKind{entityType: "theme_logo", collection: "theme_logos", imageOnly: true, publicAsset: true, urlKey: "logo_url", message: "Theme logo uploaded successfully"}, nil
	default:
		return attachmentKind{}, fmt.Errorf("%w: %q", errUnknownAttachmentType, entityType)
	}
}

// kindPath resolves the blob path for a classified upload. Store.Path is
// variadic, so each shape passes only the segments it needs.
func (h *AttachmentHandler) kindPath(kind attachmentKind, entityID int, name string) string {
	if kind.scopedByEntity {
		return h.storage.Path(kind.collection, strconv.Itoa(entityID), name)
	}
	return h.storage.Path(kind.collection, name)
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

	kind, err := classifyAttachment(entityType, category)
	if err != nil {
		slog.Warn("unknown attachment entity type", slog.String("component", "attachments"), slog.String("entity_type", entityType))
		respondValidationError(w, r, "Unknown entity type")
		return
	}
	entityType = kind.entityType

	// category is an older image-asset discriminator used by /api/portal-assets.
	// Keep it in lockstep with entity_type so callers cannot make an item/test
	// attachment public by tagging it as portal_logo/portal_background.
	if category != "" && category != entityType {
		respondValidationError(w, r, "category must match entity_type")
		return
	}
	if category == "" && kind.imageOnly {
		category = entityType
	}

	slog.Debug("entity info received", slog.String("component", "attachments"), slog.String("entity_id", entityIDStr), slog.String("entity_type", entityType), slog.String("category", category))

	// entity_id is required for every entity type that has an owner row. The
	// global uploads (user avatar, hub logo, theme logo) have no owner id.
	if entityIDStr == "" && kind.requiresEntityID {
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

	if kind.domain {
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

	if kind.imageOnly && !isAllowedImageExtension(fileHeader.Filename) {
		respondValidationError(w, r, fmt.Sprintf(
			"File extension %s is not allowed for %s uploads. Only image files are accepted",
			strings.ToLower(filepath.Ext(fileHeader.Filename)), entityType))
		return
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

	filePath := h.kindPath(kind, entityID, uniqueFilename)
	fileSize := int64(len(fileData))
	if err = h.storage.Put(r.Context(), filePath, bytes.NewReader(fileData), fileSize, detectedMimeType); err != nil {
		slog.Error("failed to write file", slog.String("component", "attachments"), slog.String("path", filePath), slog.Any("error", err))
		respondInternalError(w, r, fmt.Errorf("failed to save file: %w", err))
		return
	}
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
		// Shared with the item/page upload services so local and S3 share one
		// implementation; the uploaded bytes are already in memory.
		thumbnail, thumbErr := services.GenerateAttachmentThumbnailJPEG(fileData)
		if thumbErr != nil {
			slog.Warn("failed to generate thumbnail", slog.String("component", "attachments"), slog.String("filename", uniqueFilename), slog.Any("error", thumbErr))
		} else {
			thumbnailName := strings.TrimSuffix(uniqueFilename, filepath.Ext(uniqueFilename)) + ".thumb.jpg"
			thumbnailPath = h.kindPath(kind, entityID, thumbnailName)
			if putErr := h.storage.Put(r.Context(), thumbnailPath, bytes.NewReader(thumbnail), int64(len(thumbnail)), "image/jpeg"); putErr != nil {
				slog.Warn("failed to store thumbnail", slog.String("component", "attachments"), slog.String("filename", uniqueFilename), slog.Any("error", putErr))
				thumbnailPath = ""
			} else {
				hasThumbnail = true
			}
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
		_ = h.storage.Remove(r.Context(), filePath)
		if thumbnailPath != "" {
			_ = h.storage.Remove(r.Context(), thumbnailPath)
		}
		respondInternalError(w, r, fmt.Errorf("failed to save attachment record: %w", err))
		return
	}

	slog.Debug("attachment saved", slog.String("component", "attachments"), slog.Int64("attachment_id", attachmentID))

	if kind.entityType == "item" {
		if err = h.attachmentService.RecordItemHistory(entityID, services.AttachmentHistoryActor{UserID: uploaderID}, "attachment_uploaded", nil, attachmentID, fileHeader.Filename); err != nil {
			slog.Warn("failed to record attachment history", slog.String("component", "attachments"), slog.Any("error", err))
			// Don't fail the whole operation if history recording fails
		}
	}

	if kind.entityType == "avatar" && uploaderID != nil {
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

	if kind.imageOnly {
		downloadURL := fmt.Sprintf("/api/attachments/%d/download", attachmentID)
		if kind.publicAsset {
			downloadURL = fmt.Sprintf("/api/portal-assets/%d", attachmentID)
		}
		respondJSONOK(w, map[string]any{
			"success":       true,
			"message":       kind.message,
			kind.urlKey:     downloadURL,
			"attachment_id": attachmentID,
			"filename":      uniqueFilename,
		})
		return
	}

	// For regular attachments, return the attachment structure.
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
		response, err = services.NewItemAttachmentService(h.db, h.storage, h.permissionService).UploadItemAttachment(r.Context(), services.ItemAttachmentUploadInput{
			ItemID: entityID, UploaderID: user.ID, OriginalFilename: header.Filename,
			FileData: data, FileSize: int64(len(data)),
		})
	case "page":
		response, err = services.NewPageAttachmentUploadService(h.db, h.storage, h.permissionService, h.pagePermissionService).UploadPageAttachment(r.Context(), services.PageAttachmentUploadInput{
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

	// Open the blob through the configured backend. The local backend keeps the
	// historical os.Root confinement; S3 maps the stored path to an object key.
	// Escapes and missing objects both return 404 to avoid disclosing storage
	// details.
	slog.Debug("opening attachment blob", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
	file, err := h.storage.Open(r.Context(), attachment.FilePath)
	if err != nil {
		if errors.Is(err, blobstore.ErrOutsideRoot) {
			slog.Warn("path traversal attempt blocked", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
			respondNotFound(w, r, "file")
			return
		}
		if errors.Is(err, blobstore.ErrNotFound) {
			slog.Debug("file not found in storage", slog.String("component", "attachments"), slog.String("file_path", attachment.FilePath))
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
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size(), 10))

	// Prevent MIME sniffing and embedding.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	// Inline only allowlisted types; MIME may be remote-declared. Other files
	// download with a restrictive CSP to prevent app-origin execution.
	if fileserve.IsInlineSafeMimeType(attachment.MimeType) {
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
		itemAttachments := services.NewItemAttachmentService(h.db, h.storage, h.permissionService)
		if err := itemAttachments.DeleteItemAttachment(r.Context(), attachmentID, currentUser.ID); err != nil {
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

	// Blob-first: remove the file and thumbnail before the row. A removal
	// failure keeps the row so the caller can retry; Store.Remove treats a
	// missing object as success. A path outside the storage root is refused
	// (never touched) but does not block removing the corrupt row.
	for _, path := range []string{details.FilePath, details.ThumbnailPath} {
		if path == "" {
			continue
		}
		if err := h.storage.Remove(r.Context(), path); err != nil {
			if errors.Is(err, blobstore.ErrOutsideRoot) {
				slog.Warn("refusing to delete attachment outside storage root", slog.String("component", "attachments"), slog.String("file_path", path))
				continue
			}
			slog.Error("failed to delete attachment blob", slog.String("component", "attachments"), slog.String("file_path", path), slog.Any("error", err))
			respondInternalError(w, r, fmt.Errorf("failed to delete attachment file: %w", err))
			return
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

	if currentUser != nil {
		logAuditWithDetails(h.db, r, currentUser, logger.ActionAttachmentDelete, logger.ResourceAttachment, &attachmentID, details.OriginalFilename, map[string]any{
			"entity_type":       details.EntityType,
			"item_id":           details.ItemID,
			"original_filename": details.OriginalFilename,
		})
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

	// Open the thumbnail through the configured backend (see Download for the
	// rationale). Escapes and missing objects both surface as 404.
	file, err := h.storage.Open(r.Context(), thumbnailPath)
	if err != nil {
		if errors.Is(err, blobstore.ErrOutsideRoot) {
			slog.Warn("thumbnail path traversal attempt blocked", slog.String("component", "attachments"), slog.String("thumbnail_path", thumbnailPath))
			respondNotFound(w, r, "thumbnail")
			return
		}
		if errors.Is(err, blobstore.ErrNotFound) {
			respondNotFound(w, r, "thumbnail")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to open thumbnail: %w", err))
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(file.Size(), 10))
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
		AttachmentPath:   h.storage.Describe(),
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
