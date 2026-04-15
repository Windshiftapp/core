package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// Portal constants
const (
	defaultItemStatus = "open" // Default status for new portal submissions
)

// PortalHandler handles public portal submissions
type PortalHandler struct {
	db                   database.Database
	sessionManager       *auth.SessionManager
	portalSessionManager *auth.PortalSessionManager
	ipExtractor          *utils.IPExtractor
	portalService        *services.PortalService
	attachmentPath       string
}

// getClientIP extracts the client IP with proxy validation
func (h *PortalHandler) getClientIP(r *http.Request) string {
	return h.ipExtractor.GetClientIP(r)
}

// getPortalCustomerID attempts to get the portal customer ID from either:
// 1. A direct portal customer session (magic link auth)
// 2. An internal user session with a linked portal customer (backward compatible)
// Returns the portal customer ID and an error if authentication fails
func (h *PortalHandler) getPortalCustomerID(ctx context.Context, r *http.Request) (*int, error) {
	clientIP := h.getClientIP(r)

	// First, try portal customer session (direct magic link auth)
	if h.portalSessionManager != nil {
		portalToken, err := h.portalSessionManager.GetPortalSessionFromRequest(r)
		if err == nil && portalToken != "" {
			portalSession, err := h.portalSessionManager.ValidatePortalSession(portalToken)
			if err == nil && portalSession != nil {
				slog.Debug("portal customer authenticated via portal session", slog.String("component", "portal"), slog.Int("portal_customer_id", portalSession.PortalCustomerID))
				return &portalSession.PortalCustomerID, nil
			}
		}
	}

	// Fall back to internal user session (backward compatible)
	sessionToken, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		return nil, fmt.Errorf("authentication required")
	}

	session, err := h.sessionManager.ValidateSession(sessionToken, clientIP)
	if err != nil || session == nil {
		return nil, fmt.Errorf("invalid or expired session")
	}

	// Get portal customer ID from the user's internal session
	customerQuery := `SELECT id FROM portal_customers WHERE user_id = ?`
	var customerID int
	err = h.db.QueryRowContext(ctx, customerQuery, session.UserID).Scan(&customerID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no portal customer found for this user")
	} else if err != nil {
		return nil, fmt.Errorf("failed to find portal customer: %w", err)
	}

	slog.Debug("portal customer authenticated via internal user session", slog.String("component", "portal"), slog.Int("portal_customer_id", customerID), slog.Int("user_id", session.UserID))
	return &customerID, nil
}

// getInternalUserGroupIDs returns the group IDs for an internal user
// Returns nil if not an internal user or if no groups found
func (h *PortalHandler) getInternalUserGroupIDs(ctx context.Context, r *http.Request) []int {
	clientIP := h.getClientIP(r)
	sessionToken, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		return nil
	}

	session, err := h.sessionManager.ValidateSession(sessionToken, clientIP)
	if err != nil || session == nil {
		return nil
	}

	// Get user's group memberships
	rows, err := h.db.QueryContext(ctx, `SELECT group_id FROM group_members WHERE user_id = ?`, session.UserID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var groupIDs []int
	for rows.Next() {
		var groupID int
		if err := rows.Scan(&groupID); err != nil {
			continue
		}
		groupIDs = append(groupIDs, groupID)
	}
	return groupIDs
}

// getAuthFromContext extracts auth info from context (set by RequirePortalAuth middleware)
// Returns (internalUserID, portalCustomerID) - one will be set, the other nil
func (h *PortalHandler) getAuthFromContext(r *http.Request) (userID, customerID *int) {
	ctx := r.Context()

	// Check for internal user (set by middleware)
	if session, ok := ctx.Value(middleware.ContextKeySession).(*auth.Session); ok && session != nil {
		return &session.UserID, nil
	}

	// Check for portal customer (set by middleware)
	if portalCustomerID, ok := ctx.Value(middleware.ContextKeyPortalCustomerID).(int); ok {
		return nil, &portalCustomerID
	}

	return nil, nil
}

// getPortalCustomerOrgID returns the customer organisation ID for a portal customer
// Returns nil if no organisation is associated
//
//nolint:misspell // organisation is used in database column names (customer_organisation_id)
func (h *PortalHandler) getPortalCustomerOrgID(ctx context.Context, portalCustomerID int) *int {
	var orgID sql.NullInt64
	err := h.db.QueryRowContext(ctx, `SELECT customer_organisation_id FROM portal_customers WHERE id = ?`, portalCustomerID).Scan(&orgID)
	if err != nil || !orgID.Valid {
		return nil
	}
	result := int(orgID.Int64)
	return &result
}

// getRequestTypeWithVisibility loads a request type and deserializes its visibility fields
func (h *PortalHandler) getRequestTypeWithVisibility(ctx context.Context, requestTypeID int) (*models.RequestType, error) {
	var rt models.RequestType
	var visibilityGroupIDs, visibilityOrgIDs sql.NullString
	err := h.db.QueryRowContext(ctx, `
		SELECT id, channel_id, name, description, item_type_id, icon, color, display_order, is_active,
		       visibility_group_ids, visibility_org_ids, created_at, updated_at
		FROM request_types WHERE id = ? AND is_active = true
	`, requestTypeID).Scan(
		&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID, &rt.Icon, &rt.Color,
		&rt.DisplayOrder, &rt.IsActive, &visibilityGroupIDs, &visibilityOrgIDs,
		&rt.CreatedAt, &rt.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Deserialize visibility arrays
	if visibilityGroupIDs.Valid && visibilityGroupIDs.String != "" {
		var ids []int
		if err := json.Unmarshal([]byte(visibilityGroupIDs.String), &ids); err == nil {
			rt.VisibilityGroupIDs = ids
		}
	}
	if visibilityOrgIDs.Valid && visibilityOrgIDs.String != "" {
		var ids []int
		if err := json.Unmarshal([]byte(visibilityOrgIDs.String), &ids); err == nil {
			rt.VisibilityOrgIDs = ids
		}
	}

	return &rt, nil
}

// NewPortalHandler creates a new portal handler
func NewPortalHandler(db database.Database, sessionManager *auth.SessionManager, portalSessionManager *auth.PortalSessionManager, ipExtractor *utils.IPExtractor, attachmentPath string) *PortalHandler {
	return &PortalHandler{
		db:                   db,
		sessionManager:       sessionManager,
		portalSessionManager: portalSessionManager,
		ipExtractor:          ipExtractor,
		portalService:        services.NewPortalService(db),
		attachmentPath:       attachmentPath,
	}
}

// portalChannelResult contains the result of finding a portal channel
type portalChannelResult struct {
	channel models.Channel
	config  models.ChannelConfig
}

// findChannelByPortalSlug finds and validates a portal channel by slug
func (h *PortalHandler) findChannelByPortalSlug(ctx context.Context, slug string) (*portalChannelResult, error) {
	query := `
		SELECT id, name, type, config, status
		FROM channels
		WHERE type = 'portal'
		ORDER BY created_at DESC
	`

	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query portals: %w", err)
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

		if config.PortalSlug == slug && channel.Status == "enabled" {
			return &portalChannelResult{
				channel: channel,
				config:  config,
			}, nil
		}
	}

	return nil, fmt.Errorf("portal not found")
}

// grantChannelAccess grants a portal customer access to a channel if not already granted
func (h *PortalHandler) grantChannelAccess(ctx context.Context, customerID, channelID int) {
	var accessID int
	err := h.db.QueryRowContext(ctx, `SELECT id FROM portal_customer_channels WHERE portal_customer_id = ? AND channel_id = ?`, customerID, channelID).Scan(&accessID)

	if err == sql.ErrNoRows {
		if _, err := h.db.ExecWriteContext(ctx, `
			INSERT INTO portal_customer_channels (portal_customer_id, channel_id, created_at)
			VALUES (?, ?, ?)
		`, customerID, channelID, time.Now()); err != nil {
			slog.Warn("failed to grant channel access to portal customer", slog.String("component", "portal"), slog.Int("customer_id", customerID), slog.Int("channel_id", channelID), slog.Any("error", err))
		}
	}
}

// requestTypeValidationResult contains the result of request type field validation
type requestTypeValidationResult struct {
	itemTypeID         *int
	virtualFieldValues map[string]interface{}
	customFieldValues  map[string]interface{}
}

// validateAndSeparateFields validates request type fields and separates virtual from custom fields
func (h *PortalHandler) validateAndSeparateFields(ctx context.Context, requestTypeID *int, title, description string, customFields map[string]interface{}) (*requestTypeValidationResult, error) {
	result := &requestTypeValidationResult{}

	if requestTypeID == nil {
		// Legacy validation for submissions without request type
		if title == "" {
			return nil, fmt.Errorf("title is required")
		}
		return result, nil
	}

	// Look up request type to get item_type_id
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

	// Load request type fields for validation
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

	// Separate virtual fields from custom fields
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
func (h *PortalHandler) storeCustomFieldValues(ctx context.Context, itemID int64, customFields map[string]interface{}) {
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
				slog.Warn("failed to save custom field value", slog.String("component", "portal"), slog.Int64("item_id", itemID), slog.String("field_id", fieldIDStr), slog.Any("error", err))
			}
		}
	}

	// Update item's custom_field_values JSON column
	customFieldsJSON, err := json.Marshal(customFields)
	if err == nil {
		if _, err := h.db.ExecWriteContext(ctx, `UPDATE items SET custom_field_values = ? WHERE id = ?`, string(customFieldsJSON), itemID); err != nil {
			slog.Warn("failed to update item custom_field_values", slog.String("component", "portal"), slog.Int64("item_id", itemID), slog.Any("error", err))
		}
	}
}

// storeVirtualFieldValues stores virtual field values for an item
func (h *PortalHandler) storeVirtualFieldValues(ctx context.Context, itemID int64, virtualFields map[string]interface{}) {
	if len(virtualFields) == 0 {
		return
	}

	virtualFieldsJSON, err := json.Marshal(virtualFields)
	if err != nil {
		slog.Warn("failed to marshal virtual field values", slog.String("component", "portal"), slog.Int64("item_id", itemID), slog.Any("error", err))
		return
	}

	if _, err := h.db.ExecWriteContext(ctx, `UPDATE items SET virtual_field_data = ? WHERE id = ?`, string(virtualFieldsJSON), itemID); err != nil {
		slog.Warn("failed to update item virtual_field_data", slog.String("component", "portal"), slog.Int64("item_id", itemID), slog.Any("error", err))
	}
}

// GetPortal returns the portal configuration for public display
func (h *PortalHandler) GetPortal(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	channel := portalResult.channel
	config := portalResult.config

	// Get workspace info (use first workspace for backward compatibility)
	var workspace models.Workspace
	var workspaceID int
	if len(config.PortalWorkspaceIDs) > 0 {
		workspaceID = config.PortalWorkspaceIDs[0]
	}

	if workspaceID > 0 {
		err = h.db.QueryRowContext(ctx, `SELECT id, name, key FROM workspaces WHERE id = ?`, workspaceID).Scan(
			&workspace.ID, &workspace.Name, &workspace.Key,
		)
		if err != nil {
			respondNotFound(w, r, "workspace")
			return
		}
	}

	// Get hub logo as fallback (for portals without their own logo)
	var hubLogoURL string
	var hubConfigJSON string
	err = h.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = 'portal_hub_config'`).Scan(&hubConfigJSON)
	if err == nil && hubConfigJSON != "" {
		var hubConfig models.PortalHubConfig
		if err := json.Unmarshal([]byte(hubConfigJSON), &hubConfig); err == nil {
			hubLogoURL = hubConfig.LogoURL
		}
	}

	// Return portal info with customization settings
	response := map[string]interface{}{
		"channel_id":    channel.ID,
		"slug":          config.PortalSlug,
		"title":         config.PortalTitle,
		"description":   config.PortalDescription,
		"workspace_ids": config.PortalWorkspaceIDs,
		"workspace_id":  workspaceID, // First workspace for backward compatibility
		"workspace":     workspace,
		// Customization fields
		"gradient":                  config.PortalGradient,
		"theme":                     config.PortalTheme,
		"search_placeholder":        config.PortalSearchPlaceholder,
		"search_hint":               config.PortalSearchHint,
		"footer_columns":            config.PortalFooterColumns,
		"sections":                  config.PortalSections,
		"knowledge_base_share_link": config.KnowledgeBaseShareLink,
		"knowledge_base_url":        config.KnowledgeBaseURL,
		"knowledge_base_share_id":   config.KnowledgeBaseShareID,
		"background_image_url":      config.PortalBackgroundImageURL,
		"logo_url":                  config.PortalLogoURL,
		"hub_logo_url":              hubLogoURL,
	}

	respondJSONOK(w, response)
}

// GetRequestTypes returns request types for a portal, filtered by visibility
// For admin users viewing in customize mode, returns all request types
// For portal customers and regular users, filters by visibility rules
func (h *PortalHandler) GetRequestTypes(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	channel := portalResult.channel

	// Query all request types for this channel
	query := `
		SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
		       rt.icon, rt.color, rt.display_order, rt.is_active,
		       rt.visibility_group_ids, rt.visibility_org_ids,
		       rt.created_at, rt.updated_at,
		       it.name as item_type_name
		FROM request_types rt
		LEFT JOIN item_types it ON rt.item_type_id = it.id
		WHERE rt.channel_id = ? AND rt.is_active = true
		ORDER BY rt.display_order, rt.name`

	rows, err := h.db.QueryContext(ctx, query, channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	// Get user context for visibility filtering
	userGroupIDs := h.getInternalUserGroupIDs(ctx, r)

	// Get portal customer org ID if authenticated as portal customer
	var customerOrgID *int
	if h.portalSessionManager != nil {
		portalToken, err := h.portalSessionManager.GetPortalSessionFromRequest(r)
		if err == nil && portalToken != "" {
			portalSession, err := h.portalSessionManager.ValidatePortalSession(portalToken)
			if err == nil && portalSession != nil {
				customerOrgID = h.getPortalCustomerOrgID(ctx, portalSession.PortalCustomerID)
			}
		}
	}

	// Check if this is an admin viewing for customization (has internal session)
	isAdmin := false
	if sessionToken, err := h.sessionManager.GetSessionFromRequest(r); err == nil {
		clientIP := h.getClientIP(r)
		if session, err := h.sessionManager.ValidateSession(sessionToken, clientIP); err == nil && session != nil {
			// Check if user has system admin or channel management permission
			var hasPermission bool
			err := h.db.QueryRowContext(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM user_permissions up
					JOIN permissions p ON up.permission_id = p.id
					WHERE up.user_id = ? AND p.name IN ('system.admin', 'channels.manage')
				)
			`, session.UserID).Scan(&hasPermission)
			if err == nil && hasPermission {
				isAdmin = true
			}
		}
	}

	var requestTypes []models.RequestType
	for rows.Next() {
		var rt models.RequestType
		var visibilityGroupIDs, visibilityOrgIDs sql.NullString
		err := rows.Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
			&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive,
			&visibilityGroupIDs, &visibilityOrgIDs,
			&rt.CreatedAt, &rt.UpdatedAt,
			&rt.ItemTypeName)
		if err != nil {
			continue
		}

		// Deserialize visibility arrays
		if visibilityGroupIDs.Valid && visibilityGroupIDs.String != "" {
			var ids []int
			if err := json.Unmarshal([]byte(visibilityGroupIDs.String), &ids); err == nil {
				rt.VisibilityGroupIDs = ids
			}
		}
		if visibilityOrgIDs.Valid && visibilityOrgIDs.String != "" {
			var ids []int
			if err := json.Unmarshal([]byte(visibilityOrgIDs.String), &ids); err == nil {
				rt.VisibilityOrgIDs = ids
			}
		}

		// Admin users see all request types; others see only visible ones
		if isAdmin || rt.IsVisibleTo(userGroupIDs, customerOrgID) {
			requestTypes = append(requestTypes, rt)
		}
	}

	if requestTypes == nil {
		requestTypes = []models.RequestType{}
	}

	respondJSONOK(w, requestTypes)
}

// SubmitToPortal handles portal item submissions (requires authentication)
func (h *PortalHandler) SubmitToPortal(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	channel := portalResult.channel
	config := portalResult.config

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

	// Sanitize user input to prevent XSS
	submission.Title = utils.StripHTMLTags(submission.Title)
	submission.Description = utils.SanitizeCommentContent(submission.Description)

	// Get auth info from context (middleware already validated)
	authenticatedUserID, portalCustomerID := h.getAuthFromContext(r)

	// For portal customers, grant channel access
	// Internal users don't need portal customer records - they're tracked via user_id
	if portalCustomerID != nil {
		h.grantChannelAccess(ctx, *portalCustomerID, channel.ID)
	}

	// Validate request type visibility (security check)
	if submission.RequestTypeID != nil {
		var requestType *models.RequestType
		requestType, err = h.getRequestTypeWithVisibility(ctx, *submission.RequestTypeID)
		if err != nil {
			respondBadRequest(w, r, "Request type not found or inactive")
			return
		}

		// Verify the request type belongs to this channel
		if requestType.ChannelID != channel.ID {
			respondBadRequest(w, r, "Request type does not belong to this portal")
			return
		}

		// Get user context for visibility check
		userGroupIDs := h.getInternalUserGroupIDs(ctx, r)
		var customerOrgID *int
		if portalCustomerID != nil {
			customerOrgID = h.getPortalCustomerOrgID(ctx, *portalCustomerID)
		}

		// Check visibility
		if !requestType.IsVisibleTo(userGroupIDs, customerOrgID) {
			respondForbidden(w, r)
			return
		}
	}

	// Validate and separate fields
	validationResult, err := h.validateAndSeparateFields(ctx, submission.RequestTypeID, submission.Title, submission.Description, submission.CustomFields)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Get target workspace (use first workspace for submission)
	if len(config.PortalWorkspaceIDs) == 0 {
		respondInternalError(w, r, fmt.Errorf("portal has no configured workspaces"))
		return
	}
	targetWorkspaceID := config.PortalWorkspaceIDs[0]

	// Determine initial status from workflow if item type is specified
	initialStatus := defaultItemStatus // Default fallback status
	if validationResult.itemTypeID != nil {
		var status string
		status, err = services.GetInitialStatusForItemType(h.db, *validationResult.itemTypeID)
		if err != nil {
			slog.Warn("could not determine initial status for item type", slog.String("component", "portal"), slog.Int("item_type_id", *validationResult.itemTypeID), slog.Any("error", err))
		} else {
			initialStatus = status
		}
	}

	// Create item using centralized service
	itemID, err := services.CreateItem(h.db, services.ItemCreationParams{
		WorkspaceID:             targetWorkspaceID,
		Title:                   submission.Title,
		Description:             submission.Description,
		Status:                  initialStatus,
		ItemTypeID:              validationResult.itemTypeID,
		Priority:                "medium",
		CreatorID:               authenticatedUserID,
		CreatorPortalCustomerID: portalCustomerID, // nil for internal users, set for portal customers
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
		slog.Warn("failed to update channel last_activity", slog.String("component", "portal"), slog.Int("channel_id", channel.ID), slog.Any("error", err))
	}

	// Return success response
	respondJSONCreated(w, map[string]interface{}{
		"success": true,
		"item_id": itemID,
		"message": "Submission received successfully",
	})
}

// SearchKnowledgeBase proxies knowledge base search requests to Docmost
func (h *PortalHandler) SearchKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	var channel models.Channel
	query := `
		SELECT id, name, type, config, status
		FROM channels
		WHERE type = 'portal'
		ORDER BY created_at DESC
	`

	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	defer func() { _ = rows.Close() }()

	var found bool
	var config models.ChannelConfig
	for rows.Next() {
		if err = rows.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Config, &channel.Status); err != nil {
			continue
		}

		// Parse config to check slug
		if channel.Config != "" {
			if err = json.Unmarshal([]byte(channel.Config), &config); err != nil {
				continue
			}
		}

		if config.PortalSlug == slug && channel.Status == "enabled" {
			found = true
			break
		}
	}

	if !found {
		respondNotFound(w, r, "portal")
		return
	}

	// Check if knowledge base is configured
	if config.KnowledgeBaseURL == "" || config.KnowledgeBaseShareID == "" {
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Knowledge base not configured for this portal"))
		return
	}

	// Parse search request
	var searchRequest struct {
		Query string `json:"query"`
	}

	if err = json.NewDecoder(r.Body).Decode(&searchRequest); err != nil {
		respondBadRequest(w, r, "Invalid search request")
		return
	}

	if searchRequest.Query == "" {
		respondValidationError(w, r, "Search query is required")
		return
	}

	// Defense in depth: re-validate the URL before making the request
	if err := utils.ValidateExternalURL(config.KnowledgeBaseURL); err != nil {
		respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, "BAD_GATEWAY", "Failed to connect to knowledge base"))
		return
	}

	// Prepare Docmost API request
	docmostURL := fmt.Sprintf("%s/api/search/share-search", config.KnowledgeBaseURL)
	requestBody, err := json.Marshal(map[string]string{
		"query":   searchRequest.Query,
		"shareId": config.KnowledgeBaseShareID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Make request to Docmost
	req, err := http.NewRequestWithContext(ctx, "POST", docmostURL, bytes.NewBuffer(requestBody))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := utils.NewSSRFSafeHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, "BAD_GATEWAY", "Failed to connect to knowledge base"))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, "BAD_GATEWAY", "Knowledge base search failed"))
		return
	}

	// Forward response to client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// DownloadPortalAttachment serves portal branding attachments (logos, backgrounds) without authentication
func (h *PortalHandler) DownloadPortalAttachment(w http.ResponseWriter, r *http.Request) {
	attachmentIDStr := r.PathValue("id")
	attachmentID, err := strconv.Atoi(attachmentIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get attachment info including category
	var filePath, mimeType, originalFilename, category string
	var fileSize int64
	err = h.db.QueryRowContext(ctx, `
		SELECT file_path, mime_type, original_filename, file_size, COALESCE(category, '') as category
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(&filePath, &mimeType, &originalFilename, &fileSize, &category)

	if err == sql.ErrNoRows {
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Attachment not found"))
		return
	}
	if err != nil {
		slog.Error("failed to query attachment", slog.String("component", "portal"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Security check: Only allow portal branding attachments (logos, backgrounds)
	allowedCategories := map[string]bool{
		"portal_logo":       true,
		"portal_background": true,
		"hub_logo":          true,
	}

	if !allowedCategories[category] {
		// Return 404 to prevent enumeration of non-portal attachments
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Attachment not found"))
		return
	}

	// Validate file path is within attachment directory (prevent path traversal)
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		slog.Error("failed to resolve file path", slog.String("component", "portal"), slog.Any("error", err))
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "File not found"))
		return
	}
	absBasePath, _ := filepath.Abs(h.attachmentPath)
	if !strings.HasPrefix(absPath, absBasePath+string(os.PathSeparator)) {
		slog.Warn("path traversal attempt detected", slog.String("component", "portal"), slog.String("file_path", filePath))
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "File not found"))
		return
	}

	// Open and serve the file
	file, err := os.Open(filePath) //nolint:gosec // G304 — filePath validated via filepath.Abs prefix check above
	if err != nil {
		slog.Error("failed to open attachment file", slog.String("component", "portal"), slog.String("path", filePath), slog.Any("error", err))
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "File not found"))
		return
	}
	defer func() { _ = file.Close() }()

	// Set headers
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", originalFilename))

	// Serve file
	_, _ = io.Copy(w, file)
}
