package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// formSubmissionMaxBytes caps the public form submission body. A submission
// can carry a description plus custom-field values, so it gets 1 MiB of
// headroom while still bounding per-request memory on this public endpoint.
const (
	formSubmissionMaxBytes       = 1 << 20
	publicFormMultipartMaxBytes  = 32 << 20
	publicFormMaxAttachmentCount = 5
	publicFormMaxAttachmentBytes = 5 << 20
)

type PublicFormAttachmentConfig struct {
	Enabled          bool     `json:"enabled"`
	MaxFileSize      int64    `json:"max_file_size"`
	MaxFiles         int      `json:"max_files"`
	AllowedMimeTypes []string `json:"allowed_mime_types,omitempty"`
}

// PublicFormChannel is the public, sanitized channel configuration used by
// both the granular compatibility endpoint and the aggregate bootstrap.
type PublicFormChannel struct {
	ChannelID      int                        `json:"channel_id"`
	Name           string                     `json:"name"`
	Slug           string                     `json:"slug"`
	Theme          string                     `json:"theme"`
	BrandColor     string                     `json:"brand_color"`
	LogoURL        string                     `json:"logo_url"`
	SuccessMessage string                     `json:"success_message"`
	RedirectURL    string                     `json:"redirect_url"`
	Attachments    PublicFormAttachmentConfig `json:"attachments"`
}

type PublicFormInfo struct {
	ID            int                       `json:"id"`
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	Icon          string                    `json:"icon"`
	Color         string                    `json:"color"`
	DisplayOrder  int                       `json:"display_order"`
	WorkspaceID   *int                      `json:"workspace_id,omitempty"`
	WorkspaceName string                    `json:"workspace_name,omitempty"`
	WorkspaceKey  string                    `json:"workspace_key,omitempty"`
	Config        *models.RequestTypeConfig `json:"config,omitempty"`
}

type PublicFormDetail struct {
	FormID                 int                            `json:"form_id"`
	Fields                 []services.RequestTypeField    `json:"fields"`
	CustomFieldDefinitions []models.CustomFieldDefinition `json:"custom_field_definitions"`
}

type PublicFormBootstrapResponse struct {
	Channel    PublicFormChannel `json:"channel"`
	Forms      []PublicFormInfo  `json:"forms"`
	FormDetail *PublicFormDetail `json:"form_detail,omitempty"`
}

// FormHandler handles public form channel submissions
type FormHandler struct {
	db                   database.Database
	sessionManager       *auth.SessionManager
	portalSessionManager *auth.PortalSessionManager
	ipExtractor          *utils.IPExtractor
	portalService        *services.PortalService
	channelService       *services.ChannelService
	eventCoordinator     *services.EventCoordinator
	itemAttachments      *services.ItemAttachmentService
	auditor              *logger.Auditor
}

// SetItemAttachmentService enables the trusted item-attachment path used only
// after SubmitForm has validated the channel/request type and created its item.
func (h *FormHandler) SetItemAttachmentService(service *services.ItemAttachmentService) {
	h.itemAttachments = service
}

// SetEventCoordinator wires the shared item-created side-effect pipeline.
func (h *FormHandler) SetEventCoordinator(ec *services.EventCoordinator) {
	h.eventCoordinator = ec
}

// NewFormHandler creates a new form handler
func NewFormHandler(db database.Database, sessionManager *auth.SessionManager, portalSessionManager *auth.PortalSessionManager, ipExtractor *utils.IPExtractor, channelService *services.ChannelService) *FormHandler {
	return &FormHandler{
		db:                   db,
		sessionManager:       sessionManager,
		portalSessionManager: portalSessionManager,
		ipExtractor:          ipExtractor,
		portalService:        services.NewPortalService(db),
		channelService:       channelService,
		auditor:              logger.NewAuditor(db),
	}
}

// findChannelByFormSlug finds and validates a form channel by slug.
func (h *FormHandler) findChannelByFormSlug(ctx context.Context, slug string) (*channelResult, error) {
	return findChannelBySlug(ctx, h.db, "form", slug, func(c *models.ChannelConfig) string { return c.FormSlug })
}

// getAuthFromContext extracts auth info from context (set by RequirePortalAuth middleware)
func (h *FormHandler) getAuthFromContext(r *http.Request) (userID, customerID *int) {
	ctx := r.Context()

	if session, ok := ctx.Value(middleware.ContextKeySession).(*auth.Session); ok && session != nil {
		return &session.UserID, nil
	}

	if portalCustomerID, ok := ctx.Value(middleware.ContextKeyPortalCustomerID).(int); ok {
		return nil, &portalCustomerID
	}

	return nil, nil
}

// GetBootstrap returns the public channel and active form catalog together.
// When the channel has exactly one form, its complete render data is embedded
// so the common public-form entry path needs one browser request instead of
// two request waterfalls totaling four GETs.
func (h *FormHandler) GetBootstrap(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}
	forms, err := h.loadPublicForms(ctx, result.channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := PublicFormBootstrapResponse{
		Channel: h.publicFormChannel(slug, result),
		Forms:   forms,
	}
	if len(forms) == 1 {
		detail, err := h.loadPublicFormDetail(ctx, result.channel.ID, forms[0].ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		response.FormDetail = &detail
	}
	respondJSONOK(w, response)
}

// GetFormDetail returns the two datasets needed to render a selected form in
// one request. Multi-form channels use this after a visitor chooses a form.
func (h *FormHandler) GetFormDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	formID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondBadRequest(w, r, "Invalid form ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}
	detail, err := h.loadPublicFormDetail(ctx, result.channel.ID, formID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "form")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, detail)
}

func (h *FormHandler) publicFormChannel(slug string, result *channelResult) PublicFormChannel {
	safeRedirectURL := result.config.FormRedirectURL
	if safeRedirectURL != "" {
		if err := utils.ValidateClientRedirectURL(safeRedirectURL); err != nil {
			slog.Warn("dropped unsafe form_redirect_url from form channel response",
				slog.String("component", "forms"),
				slog.String("slug", slug),
				slog.Any("error", err))
			safeRedirectURL = ""
		}
	}
	return PublicFormChannel{
		ChannelID:      result.channel.ID,
		Name:           result.channel.Name,
		Slug:           result.config.FormSlug,
		Theme:          result.config.FormTheme,
		BrandColor:     result.config.FormBrandColor,
		LogoURL:        result.config.FormLogoURL,
		SuccessMessage: result.config.FormSuccessMessage,
		RedirectURL:    safeRedirectURL,
		Attachments:    h.publicFormAttachmentConfig(),
	}
}

func (h *FormHandler) publicFormAttachmentConfig() PublicFormAttachmentConfig {
	config := PublicFormAttachmentConfig{MaxFiles: publicFormMaxAttachmentCount}
	if h.itemAttachments == nil {
		return config
	}
	policy, err := h.itemAttachments.UploadPolicy()
	if err != nil {
		slog.Warn("failed to load public form attachment policy", slog.String("component", "forms"), slog.Any("error", err))
		return config
	}
	config.Enabled = policy.Enabled
	config.MaxFileSize = policy.MaxFileSize
	if config.MaxFileSize > publicFormMaxAttachmentBytes {
		config.MaxFileSize = publicFormMaxAttachmentBytes
	}
	config.AllowedMimeTypes = policy.AllowedMimeTypes
	return config
}

type publicFormSubmission struct {
	RequestTypeID *int           `json:"request_type_id"`
	Title         string         `json:"title"`
	Description   string         `json:"description"`
	CustomFields  map[string]any `json:"custom_fields"`
}

func (h *FormHandler) parsePublicFormSubmission(w http.ResponseWriter, r *http.Request) (publicFormSubmission, []services.ItemAttachmentUploadInput, bool) {
	var submission publicFormSubmission
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if !strings.HasPrefix(mediaType, "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, formSubmissionMaxBytes)
		if err := newJSONDecoder(w, r).Decode(&submission); err != nil {
			if isRequestBodyTooLarge(err) {
				respondRequestTooLarge(w, r)
			} else {
				respondBadRequest(w, r, "Invalid submission")
			}
			return submission, nil, false
		}
		return submission, nil, true
	}

	r.Body = http.MaxBytesReader(w, r.Body, publicFormMultipartMaxBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil { //nolint:gosec // body is bounded by MaxBytesReader above
		if isRequestBodyTooLarge(err) {
			respondRequestTooLarge(w, r)
		} else {
			respondBadRequest(w, r, "Invalid multipart submission")
		}
		return submission, nil, false
	}
	if r.MultipartForm != nil {
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	}
	if err := json.Unmarshal([]byte(r.FormValue("submission")), &submission); err != nil {
		respondBadRequest(w, r, "Invalid submission")
		return submission, nil, false
	}
	fileHeaders := r.MultipartForm.File["attachments"]
	if len(fileHeaders) > publicFormMaxAttachmentCount {
		respondValidationError(w, r, fmt.Sprintf("at most %d attachments are allowed", publicFormMaxAttachmentCount))
		return submission, nil, false
	}
	if len(fileHeaders) > 0 && h.itemAttachments == nil {
		respondServiceUnavailable(w, r, "Attachments are not enabled on this server")
		return submission, nil, false
	}
	attachmentConfig := h.publicFormAttachmentConfig()
	if len(fileHeaders) > 0 && !attachmentConfig.Enabled {
		respondServiceUnavailable(w, r, "Attachments are not enabled on this server")
		return submission, nil, false
	}
	attachments := make([]services.ItemAttachmentUploadInput, 0, len(fileHeaders))
	for _, header := range fileHeaders {
		if header.Size > attachmentConfig.MaxFileSize {
			respondValidationError(w, r, fmt.Sprintf("attachment %s exceeds the %d byte size limit", header.Filename, attachmentConfig.MaxFileSize))
			return submission, nil, false
		}
		file, err := header.Open()
		if err != nil {
			respondBadRequest(w, r, "Failed to read attachment")
			return submission, nil, false
		}
		data, readErr := io.ReadAll(file)
		_ = file.Close()
		if readErr != nil {
			respondBadRequest(w, r, "Failed to read attachment")
			return submission, nil, false
		}
		if int64(len(data)) > attachmentConfig.MaxFileSize {
			respondValidationError(w, r, fmt.Sprintf("attachment %s exceeds the %d byte size limit", header.Filename, attachmentConfig.MaxFileSize))
			return submission, nil, false
		}
		input := services.ItemAttachmentUploadInput{
			OriginalFilename: header.Filename,
			FileData:         data,
			FileSize:         int64(len(data)),
		}
		if err := h.itemAttachments.ValidatePublicFormAttachment(input); err != nil {
			h.respondPublicFormAttachmentError(w, r, err)
			return submission, nil, false
		}
		attachments = append(attachments, input)
	}
	return submission, attachments, true
}

func (h *FormHandler) respondPublicFormAttachmentError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, services.ErrItemAttachmentDisabled):
		respondServiceUnavailable(w, r, "Attachments are not enabled on this server")
	case errors.Is(err, services.ErrItemAttachmentInvalid):
		respondValidationError(w, r, err.Error())
	default:
		respondInternalError(w, r, err)
	}
}

func (h *FormHandler) loadPublicForms(ctx context.Context, channelID int) ([]PublicFormInfo, error) {
	query := `
		SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
		       rt.icon, rt.color, rt.display_order, rt.is_active, rt.config,
		       rt.created_at, rt.updated_at,
		       it.name as item_type_name,
		       rt.workspace_id, ws.name as workspace_name, ws.key as workspace_key
		FROM request_types rt
		LEFT JOIN item_types it ON rt.item_type_id = it.id
		LEFT JOIN workspaces ws ON rt.workspace_id = ws.id
		WHERE rt.channel_id = ? AND rt.is_active = true
		ORDER BY rt.display_order, rt.name`

	rows, err := h.db.QueryContext(ctx, query, channelID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	forms := []PublicFormInfo{}
	for rows.Next() {
		var rt models.RequestType
		var workspaceID sql.NullInt64
		var workspaceName, workspaceKey sql.NullString
		if err := rows.Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
			&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive, &rt.Config,
			&rt.CreatedAt, &rt.UpdatedAt,
			&rt.ItemTypeName,
			&workspaceID, &workspaceName, &workspaceKey); err != nil {
			return nil, err
		}

		form := PublicFormInfo{
			ID:            rt.ID,
			Name:          rt.Name,
			Description:   rt.Description,
			Icon:          rt.Icon,
			Color:         rt.Color,
			DisplayOrder:  rt.DisplayOrder,
			WorkspaceName: workspaceName.String,
			WorkspaceKey:  workspaceKey.String,
		}
		if workspaceID.Valid {
			id := int(workspaceID.Int64)
			form.WorkspaceID = &id
		}
		if rt.Config != nil && *rt.Config != "" {
			var config models.RequestTypeConfig
			if err := json.Unmarshal([]byte(*rt.Config), &config); err == nil {
				form.Config = &config
			}
		}
		forms = append(forms, form)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return forms, nil
}

func (h *FormHandler) loadPublicFormDetail(ctx context.Context, channelID, formID int) (PublicFormDetail, error) {
	belongs, err := h.portalService.ValidateRequestTypeBelongsToChannel(ctx, formID, channelID)
	if err != nil {
		return PublicFormDetail{}, err
	}
	if !belongs {
		return PublicFormDetail{}, repository.ErrNotFound
	}

	detail := PublicFormDetail{
		FormID:                 formID,
		Fields:                 []services.RequestTypeField{},
		CustomFieldDefinitions: []models.CustomFieldDefinition{},
	}
	detail.Fields, detail.CustomFieldDefinitions, err = h.portalService.GetRequestTypeForm(ctx, formID)
	if err != nil {
		return PublicFormDetail{}, err
	}
	return detail, nil
}

// GetFormChannel returns the form channel configuration for public display
func (h *FormHandler) GetFormChannel(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}

	respondJSONOK(w, h.publicFormChannel(slug, result))
}

// GetForms returns active forms (request types) for a form channel
func (h *FormHandler) GetForms(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}

	forms, err := h.loadPublicForms(ctx, result.channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, forms)
}

// GetFormFields returns fields for a specific form
func (h *FormHandler) GetFormFields(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	formID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondBadRequest(w, r, "Invalid form ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}

	belongs, err := h.portalService.ValidateRequestTypeBelongsToChannel(ctx, formID, result.channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !belongs {
		respondNotFound(w, r, "form")
		return
	}

	// Get form fields with custom field names
	fields, err := h.portalService.GetRequestTypeFields(ctx, formID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}

// GetCustomFields returns custom field definitions used by forms in this channel
func (h *FormHandler) GetCustomFields(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}

	// Public-form endpoint has no authenticated viewer — visibility filtering
	// (a portal-only construct) does not apply. Treat as admin so request
	// types' field metadata still resolves; any visibility restrictions on
	// form-channel request types should keep them off the form channel
	// entirely, not be enforced at the field-listing layer here.
	customFields, err := h.portalService.GetCustomFieldsForChannel(ctx, result.channel.ID, nil, nil, true)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, customFields)
}

// SubmitForm handles form submissions
func (h *FormHandler) SubmitForm(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Find channel by form slug
	chResult, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}
	channel := chResult.channel
	config := chResult.config

	// Parse either the legacy JSON contract or multipart JSON + attachments.
	submission, attachments, ok := h.parsePublicFormSubmission(w, r)
	if !ok {
		return
	}

	// Every form submission must target a specific form (request type). Without
	// one we cannot enforce per-form require_auth, validate fields, or resolve
	// the item type. Reject early instead of silently creating a generic item.
	if submission.RequestTypeID == nil {
		respondBadRequest(w, r, "request_type_id is required")
		return
	}

	// Sanitize user input
	submission.Title = sanitize.PlainTextField.Sanitize(submission.Title)
	submission.Description = sanitize.Comment.Sanitize(submission.Description)

	// Check if this form requires auth and belongs to this channel.
	var rtChannelID, rtID int
	var rtName, rtTitleTemplate string
	var rtConfigStr sql.NullString
	err = h.db.QueryRowContext(ctx, `SELECT id, channel_id, name, title_template, config FROM request_types WHERE id = ? AND is_active = true`, *submission.RequestTypeID).Scan(&rtID, &rtChannelID, &rtName, &rtTitleTemplate, &rtConfigStr)
	if err != nil {
		respondBadRequest(w, r, "Request type not found or inactive")
		return
	}

	if rtChannelID != channel.ID {
		respondBadRequest(w, r, "Request type does not belong to this form channel")
		return
	}

	var rtConfig models.RequestTypeConfig
	if rtConfigStr.Valid && rtConfigStr.String != "" {
		if err := json.Unmarshal([]byte(rtConfigStr.String), &rtConfig); err != nil {
			// require_auth lives in this blob. Treating malformed JSON as empty
			// would turn a protected form into an anonymous one, so fail closed.
			respondInternalError(w, r, fmt.Errorf("request type %d has invalid config: %w", rtID, err))
			return
		}
	}
	if rtConfig.RequireAuth {
		userID, customerID := h.getAuthFromContext(r)
		if userID == nil && customerID == nil {
			respondForbidden(w, r)
			return
		}
	}
	if len(attachments) > 0 && !rtConfig.AllowAttachments {
		respondValidationError(w, r, "Attachments are not enabled for this form")
		return
	}

	// Get auth info (may be nil for anonymous submissions)
	authenticatedUserID, portalCustomerID := h.getAuthFromContext(r)

	// Validate and separate fields (reuse portal logic)
	validationResult, err := services.ValidateAndSeparateRequestFields(ctx, h.db, submission.RequestTypeID, submission.Title, submission.Description, submission.CustomFields)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	if !validationResult.TitleFieldInForm {
		requestType := &models.RequestType{ID: rtID, ChannelID: rtChannelID, Name: rtName, TitleTemplate: rtTitleTemplate}
		rendered := renderSubmissionTitle(ctx, h.portalService, requestType, submission.Description, validationResult.CustomFieldValues, authenticatedUserID, portalCustomerID)
		if rendered == "" {
			respondValidationError(w, r, "request type is misconfigured: title field is hidden but no title template is set")
			return
		}
		submission.Title = sanitize.PlainTextField.Sanitize(rendered)
	}

	// Resolve the target workspace. The request type pins its workspace at
	// creation and that binding is the source of truth; there is no implicit
	// fallback to the channel's first workspace.
	if len(config.FormWorkspaceIDs) == 0 {
		respondInternalError(w, r, fmt.Errorf("form channel has no configured workspaces"))
		return
	}
	if validationResult.WorkspaceID == nil {
		respondValidationError(w, r, "request type is misconfigured: no target workspace")
		return
	}
	targetWorkspaceID := *validationResult.WorkspaceID
	// The request type's workspace must be one the form channel serves; a
	// mismatch means the channel's workspace list drifted away from the
	// request type's routing target.
	if !containsID(config.FormWorkspaceIDs, targetWorkspaceID) {
		respondValidationError(w, r, "request type is misconfigured: its workspace is not served by this form channel")
		return
	}

	// Determine initial status
	initialStatus := defaultItemStatus
	if validationResult.ItemTypeID != nil {
		status, err := services.GetInitialStatusForItemType(h.db, *validationResult.ItemTypeID)
		if err != nil {
			slog.Warn("could not determine initial status for item type", slog.String("component", "forms"), slog.Int("item_type_id", *validationResult.ItemTypeID), slog.Any("error", err))
		} else {
			initialStatus = status
		}
	}
	customFieldsJSON, err := json.Marshal(validationResult.CustomFieldValues)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	virtualFieldsJSON, err := json.Marshal(validationResult.VirtualFieldValues)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	eventMetadata := itemevents.System("form")
	if portalCustomerID != nil {
		eventMetadata = itemevents.PortalCustomer(*portalCustomerID, "form")
	} else if authenticatedUserID != nil {
		eventMetadata = itemevents.User(*authenticatedUserID, "form")
	}
	// Create item
	itemID, err := services.CreateItem(h.db, services.ItemCreationParams{
		WorkspaceID:             targetWorkspaceID,
		Title:                   submission.Title,
		Description:             submission.Description,
		Status:                  initialStatus,
		ItemTypeID:              validationResult.ItemTypeID,
		Priority:                "medium",
		CreatorID:               authenticatedUserID,
		CreatorPortalCustomerID: portalCustomerID,
		ChannelID:               &channel.ID,
		RequestTypeID:           submission.RequestTypeID,
		CustomFieldValuesJSON:   string(customFieldsJSON),
		VirtualFieldDataJSON:    string(virtualFieldsJSON),
		EventMetadata:           eventMetadata,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	for i := range attachments {
		attachments[i].ItemID = int(itemID)
		if authenticatedUserID != nil {
			attachments[i].UploaderID = *authenticatedUserID
		}
		if portalCustomerID != nil {
			attachments[i].UploaderPortalCustomerID = portalCustomerID
		}
		if _, uploadErr := h.itemAttachments.UploadPublicFormAttachment(r.Context(), attachments[i]); uploadErr != nil {
			if rollbackErr := h.itemAttachments.RollbackPublicFormItem(r.Context(), int(itemID)); rollbackErr != nil {
				respondInternalError(w, r, fmt.Errorf("upload public form attachment: %v; rollback item: %w", uploadErr, rollbackErr))
				return
			}
			h.respondPublicFormAttachmentError(w, r, uploadErr)
			return
		}
	}

	if h.eventCoordinator != nil {
		fullItem, fetchErr := repository.NewItemRepository(h.db).FindByIDWithDetailsContext(ctx, int(itemID))
		if fetchErr != nil {
			slog.Error("failed to hydrate form-created item for side effects", slog.Int64("item_id", itemID), slog.Any("error", fetchErr))
		} else {
			actorID := 0
			if authenticatedUserID != nil {
				actorID = *authenticatedUserID
			}
			h.eventCoordinator.EmitItemCreated(fullItem, actorID)
		}
	}

	// Update channel last activity
	if _, err := h.db.ExecWriteContext(ctx, `UPDATE channels SET last_activity = ? WHERE id = ?`, time.Now(), channel.ID); err != nil {
		slog.Warn("failed to update channel last_activity", slog.String("component", "forms"), slog.Int("channel_id", channel.ID), slog.Any("error", err))
	}

	// Build response with per-form config overrides
	const defaultSuccessMessage = "Submission received successfully"
	response := map[string]any{
		"success":          true,
		"item_id":          itemID,
		"success_message":  defaultSuccessMessage,
		"attachment_count": len(attachments),
	}

	// Per-form config overrides (rtConfig parsed earlier in the handler).
	// Defense-in-depth: re-validate redirect_url here in case stale or
	// API-bypassing writes seeded a non-http(s) URL into the DB. We omit
	// rather than fail so a single bad-config form doesn't break submission.
	if rtConfig.SuccessMessage != "" {
		response["success_message"] = rtConfig.SuccessMessage
	}
	if rtConfig.RedirectURL != "" {
		if err := utils.ValidateClientRedirectURL(rtConfig.RedirectURL); err != nil {
			slog.Warn("dropped unsafe redirect_url from form response",
				slog.String("component", "forms"),
				slog.Int("request_type_id", *submission.RequestTypeID),
				slog.Any("error", err))
		} else {
			response["redirect_url"] = rtConfig.RedirectURL
		}
	}

	// Fall back to channel-level overrides (same defense-in-depth check).
	if _, ok := response["redirect_url"]; !ok && config.FormRedirectURL != "" {
		if err := utils.ValidateClientRedirectURL(config.FormRedirectURL); err != nil {
			slog.Warn("dropped unsafe form_redirect_url from channel config",
				slog.String("component", "forms"),
				slog.Int("channel_id", channel.ID),
				slog.Any("error", err))
		} else {
			response["redirect_url"] = config.FormRedirectURL
		}
	}
	if config.FormSuccessMessage != "" {
		if msg, ok := response["success_message"].(string); ok && msg == defaultSuccessMessage {
			response["success_message"] = config.FormSuccessMessage
		}
	}

	respondJSONCreated(w, response)
}

// UpdateRequestTypeConfig updates the config for a specific request type (form settings)
func (h *FormHandler) UpdateRequestTypeConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Look up the channel this request type belongs to so we can gate on
	// channel-management. Without this, any auth'd user could rewrite a
	// public form's config — see bughunt2.md Run 6 finding #1.
	var channelID sql.NullInt64
	err := h.db.QueryRow("SELECT channel_id FROM request_types WHERE id = ?", id).Scan(&channelID)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !channelID.Valid {
		// Request type with no channel — refuse to mutate config (no scope to authorize against).
		respondNotFound(w, r, "request_type")
		return
	}

	canManage, err := h.channelService.UserCanManage(r.Context(), user.ID, int(channelID.Int64))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canManage {
		respondNotFound(w, r, "request_type")
		return
	}

	var rtConfig models.RequestTypeConfig
	if !decodeChannelRequest(w, r, &rtConfig, false) {
		return
	}

	// redirect_url ends up at window.location.href in the submitter's browser.
	// Reject non-http(s) schemes (javascript:, data:, vbscript:) at write time
	// to keep an admin from XSS-ing form submitters via the redirect.
	if err := utils.ValidateClientRedirectURL(rtConfig.RedirectURL); err != nil {
		respondValidationError(w, r, "redirect_url must be an http(s) URL")
		return
	}

	configJSON, err := json.Marshal(rtConfig)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	configStr := string(configJSON)
	now := time.Now()
	if _, err := h.db.ExecWrite(`UPDATE request_types SET config = ?, updated_at = ? WHERE id = ?`, configStr, now, id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if user != nil {
		h.auditor.Log(r, user, logger.ActionRequestTypeConfigUpdate, logger.ResourceRequestTypeConfig, &id, "")
	}
	respondJSONOK(w, rtConfig)
}
