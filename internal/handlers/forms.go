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

// formChannelResult contains the result of finding a form channel
type formChannelResult struct {
	channel models.Channel
	config  models.ChannelConfig
}

// findChannelByFormSlug finds and validates a form channel by slug
func (h *FormHandler) findChannelByFormSlug(ctx context.Context, slug string) (*formChannelResult, error) {
	query := `
		SELECT id, name, type, config, status
		FROM channels
		WHERE type = 'form'
		ORDER BY created_at DESC
	`

	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query form channels: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var channel models.Channel
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Config, &channel.Status); err != nil {
			continue
		}

		var config models.ChannelConfig
		if channel.Config != "" {
			if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
				continue
			}
		}

		if config.FormSlug == slug && channel.Status == "enabled" {
			return &formChannelResult{
				channel: channel,
				config:  config,
			}, nil
		}
	}

	return nil, fmt.Errorf("form channel not found")
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

	respondJSONOK(w, map[string]interface{}{
		"channel_id":      result.channel.ID,
		"name":            result.channel.Name,
		"slug":            config.FormSlug,
		"theme":           config.FormTheme,
		"brand_color":     config.FormBrandColor,
		"logo_url":        config.FormLogoURL,
		"success_message": config.FormSuccessMessage,
		"redirect_url":    config.FormRedirectURL,
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
	channelResult, err := h.findChannelByFormSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "form_channel")
		return
	}
	channel := channelResult.channel
	config := channelResult.config

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

	// Sanitize user input
	submission.Title = utils.StripHTMLTags(submission.Title)
	submission.Description = utils.SanitizeCommentContent(submission.Description)

	// Check if this form requires auth
	if submission.RequestTypeID != nil {
		var rtConfigStr sql.NullString
		err = h.db.QueryRowContext(ctx, `SELECT config FROM request_types WHERE id = ? AND is_active = true`, *submission.RequestTypeID).Scan(&rtConfigStr)
		if err != nil {
			respondBadRequest(w, r, "Request type not found or inactive")
			return
		}

		if rtConfigStr.Valid && rtConfigStr.String != "" {
			var rtConfig models.RequestTypeConfig
			if err := json.Unmarshal([]byte(rtConfigStr.String), &rtConfig); err == nil {
				if rtConfig.RequireAuth {
					// Check if user is authenticated
					userID, customerID := h.getAuthFromContext(r)
					if userID == nil && customerID == nil {
						respondForbidden(w, r)
						return
					}
				}
			}
		}

		// Verify the request type belongs to this channel
		var rtChannelID int
		err = h.db.QueryRowContext(ctx, `SELECT channel_id FROM request_types WHERE id = ?`, *submission.RequestTypeID).Scan(&rtChannelID)
		if err != nil || rtChannelID != channel.ID {
			respondBadRequest(w, r, "Request type does not belong to this form channel")
			return
		}
	}

	// Get auth info (may be nil for anonymous submissions)
	authenticatedUserID, portalCustomerID := h.getAuthFromContext(r)

	// Validate and separate fields (reuse portal logic)
	validationResult, err := h.validateAndSeparateFields(ctx, submission.RequestTypeID, submission.Title, submission.Description, submission.CustomFields)
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
	h.storeCustomFieldValues(ctx, itemID, validationResult.customFieldValues)
	h.storeVirtualFieldValues(ctx, itemID, validationResult.virtualFieldValues)

	// Update channel last activity
	if _, err := h.db.ExecWriteContext(ctx, `UPDATE channels SET last_activity = ? WHERE id = ?`, time.Now(), channel.ID); err != nil {
		slog.Warn("failed to update channel last_activity", slog.String("component", "forms"), slog.Int("channel_id", channel.ID), slog.Any("error", err))
	}

	// Build response with per-form config overrides
	response := map[string]interface{}{
		"success": true,
		"item_id": itemID,
		"message": "Submission received successfully",
	}

	// Check for per-form success message or redirect
	if submission.RequestTypeID != nil {
		var rtConfigStr sql.NullString
		_ = h.db.QueryRowContext(ctx, `SELECT config FROM request_types WHERE id = ?`, *submission.RequestTypeID).Scan(&rtConfigStr)
		if rtConfigStr.Valid && rtConfigStr.String != "" {
			var rtConfig models.RequestTypeConfig
			if err := json.Unmarshal([]byte(rtConfigStr.String), &rtConfig); err == nil {
				if rtConfig.SuccessMessage != "" {
					response["message"] = rtConfig.SuccessMessage
				}
				if rtConfig.RedirectURL != "" {
					response["redirect_url"] = rtConfig.RedirectURL
				}
			}
		}
	}

	// Fall back to channel-level overrides
	if _, ok := response["redirect_url"]; !ok && config.FormRedirectURL != "" {
		response["redirect_url"] = config.FormRedirectURL
	}
	if config.FormSuccessMessage != "" {
		if msg, ok := response["message"].(string); ok && msg == "Submission received successfully" {
			response["message"] = config.FormSuccessMessage
		}
	}

	respondJSONCreated(w, response)
}

// validateAndSeparateFields validates request type fields and separates virtual from custom fields
func (h *FormHandler) validateAndSeparateFields(ctx context.Context, requestTypeID *int, title, description string, customFields map[string]interface{}) (*requestTypeValidationResult, error) {
	result := &requestTypeValidationResult{}

	if requestTypeID == nil {
		if title == "" {
			return nil, fmt.Errorf("title is required")
		}
		return result, nil
	}

	var rtID int
	var rtName string
	var itemTypeID int
	err := h.db.QueryRowContext(ctx, `SELECT id, name, item_type_id FROM request_types WHERE id = ? AND is_active = true`, *requestTypeID).Scan(
		&rtID, &rtName, &itemTypeID,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid request type (ID: %d): %w", *requestTypeID, err)
	}
	result.itemTypeID = &itemTypeID

	virtualFieldIDs := make(map[string]bool)
	rows, err := h.db.QueryContext(ctx, `SELECT field_identifier, field_type, is_required FROM request_type_fields WHERE request_type_id = ? ORDER BY display_order`, *requestTypeID)
	if err != nil {
		return nil, fmt.Errorf("failed to load request type fields: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var fieldID, fieldType string
		var isRequired bool
		if err := rows.Scan(&fieldID, &fieldType, &isRequired); err != nil {
			continue
		}

		if fieldType == "virtual" {
			virtualFieldIDs[fieldID] = true
		}

		if isRequired {
			switch fieldType {
			case "default":
				if fieldID == "title" && title == "" {
					return nil, fmt.Errorf("title is required")
				}
				if fieldID == "description" && description == "" {
					return nil, fmt.Errorf("description is required")
				}
			case "custom", "virtual":
				if customFields == nil || customFields[fieldID] == nil || customFields[fieldID] == "" {
					return nil, fmt.Errorf("field %s is required", fieldID)
				}
			}
		}
	}

	if len(virtualFieldIDs) > 0 && customFields != nil {
		result.virtualFieldValues = make(map[string]interface{})
		result.customFieldValues = make(map[string]interface{})

		for fieldID, value := range customFields {
			if virtualFieldIDs[fieldID] {
				result.virtualFieldValues[fieldID] = value
			} else {
				result.customFieldValues[fieldID] = value
			}
		}
	} else {
		result.customFieldValues = customFields
	}

	return result, nil
}

// storeCustomFieldValues stores custom field values for an item
func (h *FormHandler) storeCustomFieldValues(ctx context.Context, itemID int64, customFields map[string]interface{}) {
	if len(customFields) == 0 {
		return
	}

	now := time.Now()
	for fieldIDStr, value := range customFields {
		if value == nil || value == "" {
			continue
		}

		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		case float64:
			valueStr = fmt.Sprintf("%v", v)
		case bool:
			valueStr = fmt.Sprintf("%v", v)
		default:
			valueBytes, err := json.Marshal(v)
			if err == nil {
				valueStr = string(valueBytes)
			}
		}

		if valueStr != "" {
			if _, err := h.db.ExecWriteContext(ctx, `
				INSERT INTO custom_field_values (item_id, custom_field_id, value, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(item_id, custom_field_id) DO UPDATE SET value = ?, updated_at = ?
			`, itemID, fieldIDStr, valueStr, now, now, valueStr, now); err != nil {
				slog.Warn("failed to save custom field value", slog.String("component", "forms"), slog.Int64("item_id", itemID), slog.String("field_id", fieldIDStr), slog.Any("error", err))
			}
		}
	}

	customFieldsJSON, err := json.Marshal(customFields)
	if err == nil {
		if _, err := h.db.ExecWriteContext(ctx, `UPDATE items SET custom_field_values = ? WHERE id = ?`, string(customFieldsJSON), itemID); err != nil {
			slog.Warn("failed to update item custom_field_values", slog.String("component", "forms"), slog.Int64("item_id", itemID), slog.Any("error", err))
		}
	}
}

// storeVirtualFieldValues stores virtual field values for an item
func (h *FormHandler) storeVirtualFieldValues(ctx context.Context, itemID int64, virtualFields map[string]interface{}) {
	if len(virtualFields) == 0 {
		return
	}

	virtualFieldsJSON, err := json.Marshal(virtualFields)
	if err != nil {
		slog.Warn("failed to marshal virtual field values", slog.String("component", "forms"), slog.Int64("item_id", itemID), slog.Any("error", err))
		return
	}

	if _, err := h.db.ExecWriteContext(ctx, `UPDATE items SET virtual_field_data = ? WHERE id = ?`, string(virtualFieldsJSON), itemID); err != nil {
		slog.Warn("failed to update item virtual_field_data", slog.String("component", "forms"), slog.Int64("item_id", itemID), slog.Any("error", err))
	}
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
