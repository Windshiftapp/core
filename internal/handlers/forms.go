package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// FormHandler handles public form channel submissions
type FormHandler struct {
	db                   database.Database
	sessionManager       *auth.SessionManager
	portalSessionManager *auth.PortalSessionManager
	ipExtractor          *utils.IPExtractor
	portalService        *services.PortalService
}

// NewFormHandler creates a new form handler
func NewFormHandler(db database.Database, sessionManager *auth.SessionManager, portalSessionManager *auth.PortalSessionManager, ipExtractor *utils.IPExtractor) *FormHandler {
	return &FormHandler{
		db:                   db,
		sessionManager:       sessionManager,
		portalSessionManager: portalSessionManager,
		ipExtractor:          ipExtractor,
		portalService:        services.NewPortalService(db),
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

// GetFormChannel returns the form channel configuration for public display
func (h *FormHandler) GetFormChannel(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}

	config := result.config

	// Defense-in-depth: drop a stored redirect_url that isn't http(s) before
	// echoing it to the public form page. Same rationale as SubmitForm — keeps
	// a stale or API-bypassing config from XSS'ing form visitors via
	// window.location.href in FormRenderer.svelte.
	safeRedirectURL := config.FormRedirectURL
	if safeRedirectURL != "" {
		if err := utils.ValidateClientRedirectURL(safeRedirectURL); err != nil {
			slog.Warn("dropped unsafe form_redirect_url from form channel response",
				slog.String("component", "forms"),
				slog.String("slug", slug),
				slog.Any("error", err))
			safeRedirectURL = ""
		}
	}

	respondJSONOK(w, map[string]interface{}{
		"channel_id":      result.channel.ID,
		"name":            result.channel.Name,
		"slug":            config.FormSlug,
		"theme":           config.FormTheme,
		"brand_color":     config.FormBrandColor,
		"logo_url":        config.FormLogoURL,
		"success_message": config.FormSuccessMessage,
		"redirect_url":    safeRedirectURL,
	})
}

// GetForms returns active forms (request types) for a form channel
func (h *FormHandler) GetForms(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}

	query := `
		SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
		       rt.icon, rt.color, rt.display_order, rt.is_active, rt.config,
		       rt.created_at, rt.updated_at,
		       it.name as item_type_name
		FROM request_types rt
		LEFT JOIN item_types it ON rt.item_type_id = it.id
		WHERE rt.channel_id = ? AND rt.is_active = true
		ORDER BY rt.display_order, rt.name`

	rows, err := h.db.QueryContext(ctx, query, result.channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type formInfo struct {
		ID           int                       `json:"id"`
		Name         string                    `json:"name"`
		Description  string                    `json:"description"`
		Icon         string                    `json:"icon"`
		Color        string                    `json:"color"`
		DisplayOrder int                       `json:"display_order"`
		Config       *models.RequestTypeConfig `json:"config,omitempty"`
	}

	var forms []formInfo
	for rows.Next() {
		var rt models.RequestType
		if err := rows.Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
			&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive, &rt.Config,
			&rt.CreatedAt, &rt.UpdatedAt,
			&rt.ItemTypeName); err != nil {
			continue
		}

		fi := formInfo{
			ID:           rt.ID,
			Name:         rt.Name,
			Description:  rt.Description,
			Icon:         rt.Icon,
			Color:        rt.Color,
			DisplayOrder: rt.DisplayOrder,
		}

		// Parse per-form config
		if rt.Config != nil && *rt.Config != "" {
			var rtConfig models.RequestTypeConfig
			if err := json.Unmarshal([]byte(*rt.Config), &rtConfig); err == nil {
				fi.Config = &rtConfig
			}
		}

		forms = append(forms, fi)
	}

	if forms == nil {
		forms = []formInfo{}
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}

	// Verify the form belongs to this channel
	var channelID int
	err = h.db.QueryRowContext(ctx, `SELECT channel_id FROM request_types WHERE id = ? AND is_active = true`, formID).Scan(&channelID)
	if err != nil || channelID != result.channel.ID {
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}

	customFields, err := h.portalService.GetCustomFieldsForChannel(ctx, result.channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, customFields)
}

// SubmitForm handles form submissions
func (h *FormHandler) SubmitForm(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by form slug
	chResult, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}
	channel := chResult.channel
	config := chResult.config

	// Parse submission
	var submission struct {
		RequestTypeID *int                   `json:"request_type_id"`
		Title         string                 `json:"title"`
		Description   string                 `json:"description"`
		CustomFields  map[string]interface{} `json:"custom_fields"`
	}

	if err = json.NewDecoder(r.Body).Decode(&submission); err != nil {
		respondBadRequest(w, r, "Invalid submission")
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
	submission.Title = utils.StripHTMLTags(submission.Title)
	submission.Description = utils.SanitizeCommentContent(submission.Description)

	// Check if this form requires auth and belongs to this channel.
	var rtChannelID int
	var rtConfigStr sql.NullString
	err = h.db.QueryRowContext(ctx, `SELECT channel_id, config FROM request_types WHERE id = ? AND is_active = true`, *submission.RequestTypeID).Scan(&rtChannelID, &rtConfigStr)
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
			// Invalid JSON in config — treat as empty config rather than failing the submission.
			rtConfig = models.RequestTypeConfig{}
		}
	}
	if rtConfig.RequireAuth {
		userID, customerID := h.getAuthFromContext(r)
		if userID == nil && customerID == nil {
			respondForbidden(w, r)
			return
		}
	}

	// Get auth info (may be nil for anonymous submissions)
	authenticatedUserID, portalCustomerID := h.getAuthFromContext(r)

	// Validate and separate fields (reuse portal logic)
	validationResult, err := validateAndSeparateFields(ctx, h.db, submission.RequestTypeID, submission.Title, submission.Description, submission.CustomFields)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Get target workspace
	if len(config.FormWorkspaceIDs) == 0 {
		respondInternalError(w, r, fmt.Errorf("form channel has no configured workspaces"))
		return
	}
	targetWorkspaceID := config.FormWorkspaceIDs[0]

	// Determine initial status
	initialStatus := defaultItemStatus
	if validationResult.itemTypeID != nil {
		status, err := services.GetInitialStatusForItemType(h.db, *validationResult.itemTypeID)
		if err != nil {
			slog.Warn("could not determine initial status for item type", slog.String("component", "forms"), slog.Int("item_type_id", *validationResult.itemTypeID), slog.Any("error", err))
		} else {
			initialStatus = status
		}
	}

	// Create item
	itemID, err := services.CreateItem(h.db, services.ItemCreationParams{
		WorkspaceID:             targetWorkspaceID,
		Title:                   submission.Title,
		Description:             submission.Description,
		Status:                  initialStatus,
		ItemTypeID:              validationResult.itemTypeID,
		Priority:                "medium",
		CreatorID:               authenticatedUserID,
		CreatorPortalCustomerID: portalCustomerID,
		ChannelID:               &channel.ID,
		RequestTypeID:           submission.RequestTypeID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Store custom and virtual field values
	storeCustomFieldValues(ctx, h.db, "forms", itemID, validationResult.customFieldValues)
	storeVirtualFieldValues(ctx, h.db, "forms", itemID, validationResult.virtualFieldValues)

	// Update channel last activity
	if _, err := h.db.ExecWriteContext(ctx, `UPDATE channels SET last_activity = ? WHERE id = ?`, time.Now(), channel.ID); err != nil {
		slog.Warn("failed to update channel last_activity", slog.String("component", "forms"), slog.Int("channel_id", channel.ID), slog.Any("error", err))
	}

	// Build response with per-form config overrides
	const defaultSuccessMessage = "Submission received successfully"
	response := map[string]interface{}{
		"success":         true,
		"item_id":         itemID,
		"success_message": defaultSuccessMessage,
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

	var exists bool
	if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM request_types WHERE id = ?)", id).Scan(&exists); err != nil || !exists {
		respondNotFound(w, r, "request_type")
		return
	}

	var rtConfig models.RequestTypeConfig
	if err := json.NewDecoder(r.Body).Decode(&rtConfig); err != nil {
		respondBadRequest(w, r, "Invalid request body")
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

	respondJSONOK(w, rtConfig)
}
