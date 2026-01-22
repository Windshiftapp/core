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
	"strconv"
	"strings"
	"time"
	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/models"
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

// NewPortalHandler creates a new portal handler
func NewPortalHandler(db database.Database, sessionManager *auth.SessionManager, portalSessionManager *auth.PortalSessionManager, ipExtractor *utils.IPExtractor) *PortalHandler {
	return &PortalHandler{
		db:                   db,
		sessionManager:       sessionManager,
		portalSessionManager: portalSessionManager,
		ipExtractor:          ipExtractor,
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
	defer rows.Close()

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

		if config.PortalSlug == slug && config.PortalEnabled {
			return &portalChannelResult{
				channel: channel,
				config:  config,
			}, nil
		}
	}

	return nil, fmt.Errorf("portal not found")
}

// getOrCreatePortalCustomer finds or creates a portal customer for the given user or email
func (h *PortalHandler) getOrCreatePortalCustomer(ctx context.Context, userID *int, name, email string) (int, error) {
	now := time.Now()

	if userID != nil {
		// Authenticated user - find or create linked portal customer
		var customerID int
		err := h.db.QueryRowContext(ctx, `SELECT id FROM portal_customers WHERE user_id = ?`, *userID).Scan(&customerID)

		if err == sql.ErrNoRows {
			// Get user details to create portal customer
			var userName, userEmail string
			err := h.db.QueryRowContext(ctx, `SELECT first_name || ' ' || last_name, email FROM users WHERE id = ?`, *userID).Scan(&userName, &userEmail)
			if err != nil {
				return 0, fmt.Errorf("failed to get user details: %w", err)
			}

			result, err := h.db.ExecWriteContext(ctx, `
				INSERT INTO portal_customers (name, email, user_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?)
			`, userName, userEmail, *userID, now, now)
			if err != nil {
				return 0, fmt.Errorf("failed to create portal customer: %w", err)
			}

			customerIDInt64, err := result.LastInsertId()
			if err != nil {
				return 0, fmt.Errorf("failed to get customer ID: %w", err)
			}
			return int(customerIDInt64), nil
		} else if err != nil {
			return 0, fmt.Errorf("failed to find portal customer: %w", err)
		}

		return customerID, nil
	}

	// Anonymous user - find or create by email
	var customerID int
	err := h.db.QueryRowContext(ctx, `SELECT id FROM portal_customers WHERE email = ?`, email).Scan(&customerID)

	if err == sql.ErrNoRows {
		result, err := h.db.ExecWriteContext(ctx, `
			INSERT INTO portal_customers (name, email, created_at, updated_at)
			VALUES (?, ?, ?, ?)
		`, name, email, now, now)
		if err != nil {
			return 0, fmt.Errorf("failed to create portal customer: %w", err)
		}

		customerIDInt64, err := result.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("failed to get customer ID: %w", err)
		}
		return int(customerIDInt64), nil
	} else if err != nil {
		return 0, fmt.Errorf("failed to find portal customer: %w", err)
	}

	return customerID, nil
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
	defer rows.Close()

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
			if fieldType == "default" {
				if fieldID == "title" && title == "" {
					return nil, fmt.Errorf("title is required")
				}
				if fieldID == "description" && description == "" {
					return nil, fmt.Errorf("description is required")
				}
			} else if fieldType == "custom" || fieldType == "virtual" {
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
	if customFields == nil || len(customFields) == 0 {
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
	if virtualFields == nil || len(virtualFields) == 0 {
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
		http.Error(w, "Portal not found", http.StatusNotFound)
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
			http.Error(w, "Portal workspace not found", http.StatusNotFound)
			return
		}
	}

	// Return portal info with customization settings
	response := map[string]interface{}{
		"channel_id":     channel.ID,
		"slug":           config.PortalSlug,
		"title":          config.PortalTitle,
		"description":    config.PortalDescription,
		"workspace_ids":  config.PortalWorkspaceIDs,
		"workspace_id":   workspaceID, // First workspace for backward compatibility
		"workspace":      workspace,
		// Customization fields
		"gradient":                config.PortalGradient,
		"theme":                   config.PortalTheme,
		"search_placeholder":      config.PortalSearchPlaceholder,
		"search_hint":             config.PortalSearchHint,
		"footer_columns":          config.PortalFooterColumns,
		"sections":                config.PortalSections,
		"knowledge_base_share_link": config.KnowledgeBaseShareLink,
		"knowledge_base_url":      config.KnowledgeBaseURL,
		"knowledge_base_share_id": config.KnowledgeBaseShareID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SubmitToPortal handles public item submissions
func (h *PortalHandler) SubmitToPortal(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}
	channel := portalResult.channel
	config := portalResult.config

	// Parse submission
	var submission struct {
		RequestTypeID *int                   `json:"request_type_id"`
		Title         string                 `json:"title"`
		Description   string                 `json:"description"`
		Name          string                 `json:"name"`  // Required for anonymous submissions
		Email         string                 `json:"email"` // Required for anonymous submissions
		CustomFields  map[string]interface{} `json:"custom_fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		http.Error(w, "Invalid submission", http.StatusBadRequest)
		return
	}

	// Sanitize user input to prevent XSS (portal accepts external/unauthenticated input)
	submission.Title = utils.StripHTMLTags(submission.Title)
	submission.Description = utils.StripHTMLTags(submission.Description)
	submission.Name = utils.StripHTMLTags(submission.Name)

	// Check if user is authenticated
	var authenticatedUserID *int
	sessionToken, err := h.sessionManager.GetSessionFromRequest(r)
	if err == nil {
		clientIP := h.getClientIP(r)
		session, err := h.sessionManager.ValidateSession(sessionToken, clientIP)
		if err == nil && session != nil {
			slog.Debug("user authenticated", slog.String("component", "portal"), slog.Int("user_id", session.UserID))
			authenticatedUserID = &session.UserID
		}
	}

	// Handle portal customer for authenticated vs anonymous users
	if authenticatedUserID == nil {
		// Anonymous submission: require name and email
		if strings.TrimSpace(submission.Name) == "" {
			http.Error(w, "Name is required for portal submissions", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(submission.Email) == "" {
			http.Error(w, "Email is required for portal submissions", http.StatusBadRequest)
			return
		}
	}

	// Get or create portal customer
	customerID, err := h.getOrCreatePortalCustomer(ctx, authenticatedUserID, submission.Name, submission.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Grant channel access
	h.grantChannelAccess(ctx, customerID, channel.ID)

	// Validate and separate fields
	validationResult, err := h.validateAndSeparateFields(ctx, submission.RequestTypeID, submission.Title, submission.Description, submission.CustomFields)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get target workspace (use first workspace for submission)
	if len(config.PortalWorkspaceIDs) == 0 {
		http.Error(w, "Portal has no configured workspaces", http.StatusInternalServerError)
		return
	}
	targetWorkspaceID := config.PortalWorkspaceIDs[0]

	// Determine initial status from workflow if item type is specified
	initialStatus := defaultItemStatus // Default fallback status
	if validationResult.itemTypeID != nil {
		status, err := services.GetInitialStatusForItemType(h.db, *validationResult.itemTypeID)
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
		CreatorPortalCustomerID: &customerID,
		ChannelID:               &channel.ID,
		RequestTypeID:           submission.RequestTypeID,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create item: %v", err), http.StatusInternalServerError)
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
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
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}
	defer rows.Close()

	var found bool
	var config models.ChannelConfig
	for rows.Next() {
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Config, &channel.Status); err != nil {
			continue
		}

		// Parse config to check slug
		if channel.Config != "" {
			if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
				continue
			}
		}

		if config.PortalSlug == slug && config.PortalEnabled {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}

	// Check if knowledge base is configured
	if config.KnowledgeBaseURL == "" || config.KnowledgeBaseShareID == "" {
		http.Error(w, "Knowledge base not configured for this portal", http.StatusNotFound)
		return
	}

	// Parse search request
	var searchRequest struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&searchRequest); err != nil {
		http.Error(w, "Invalid search request", http.StatusBadRequest)
		return
	}

	if searchRequest.Query == "" {
		http.Error(w, "Search query is required", http.StatusBadRequest)
		return
	}

	// Prepare Docmost API request
	docmostURL := fmt.Sprintf("%s/api/search/share-search", config.KnowledgeBaseURL)
	requestBody, err := json.Marshal(map[string]string{
		"query":   searchRequest.Query,
		"shareId": config.KnowledgeBaseShareID,
	})
	if err != nil {
		http.Error(w, "Failed to prepare search request", http.StatusInternalServerError)
		return
	}

	// Make request to Docmost
	req, err := http.NewRequestWithContext(ctx, "POST", docmostURL, bytes.NewBuffer(requestBody))
	if err != nil {
		http.Error(w, "Failed to create search request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to search knowledge base: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read search results", http.StatusInternalServerError)
		return
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("Knowledge base search failed: %s", string(body)), http.StatusBadGateway)
		return
	}

	// Forward response to client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// GetMyRequests returns all requests submitted by the authenticated portal customer through this portal
func (h *PortalHandler) GetMyRequests(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}
	defer rows.Close()

	var found bool
	for rows.Next() {
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Config, &channel.Status); err != nil {
			continue
		}

		// Parse config to check slug
		var config models.ChannelConfig
		if channel.Config != "" {
			if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
				continue
			}
		}

		if config.PortalSlug == slug && config.PortalEnabled {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}

	// Get portal customer ID (supports both portal session and internal user session)
	portalCustomerID, err := h.getPortalCustomerID(ctx, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Get all requests submitted by this portal customer through this channel
	requestsQuery := `
		SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
			i.status_id, i.priority_id, i.created_at, i.updated_at,
			i.channel_id, i.request_type_id,
			w.name AS workspace_name,
			w.key AS workspace_key,
			rt.name AS request_type_name,
			rt.icon AS request_type_icon,
			rt.color AS request_type_color,
			(SELECT COUNT(*) FROM comments WHERE item_id = i.id) AS comment_count
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN request_types rt ON i.request_type_id = rt.id
		WHERE i.creator_portal_customer_id = ? AND i.channel_id = ?
		ORDER BY i.created_at DESC
	`

	requestRows, err := h.db.QueryContext(ctx, requestsQuery, portalCustomerID, channel.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch requests: %v", err), http.StatusInternalServerError)
		return
	}
	defer requestRows.Close()

	type RequestSummary struct {
		ID                  int     `json:"id"`
		WorkspaceID         int     `json:"workspace_id"`
		WorkspaceItemNumber int     `json:"workspace_item_number"`
		WorkspaceName       string  `json:"workspace_name"`
		WorkspaceKey        string  `json:"workspace_key"`
		Title               string  `json:"title"`
		Description         string  `json:"description"`
		Status              string  `json:"status"`
		Priority            string  `json:"priority"`
		CreatedAt           string  `json:"created_at"`
		UpdatedAt           string  `json:"updated_at"`
		ChannelID           *int    `json:"channel_id"`
		RequestTypeID       *int    `json:"request_type_id"`
		RequestTypeName     *string `json:"request_type_name"`
		RequestTypeIcon     *string `json:"request_type_icon"`
		RequestTypeColor    *string `json:"request_type_color"`
		CommentCount        int     `json:"comment_count"`
	}

	var requests []RequestSummary
	for requestRows.Next() {
		var req RequestSummary
		var requestTypeName, requestTypeIcon, requestTypeColor sql.NullString
		err := requestRows.Scan(
			&req.ID, &req.WorkspaceID, &req.WorkspaceItemNumber, &req.Title, &req.Description,
			&req.Status, &req.Priority, &req.CreatedAt, &req.UpdatedAt,
			&req.ChannelID, &req.RequestTypeID,
			&req.WorkspaceName, &req.WorkspaceKey,
			&requestTypeName, &requestTypeIcon, &requestTypeColor,
			&req.CommentCount,
		)
		if err != nil {
			continue
		}

		if requestTypeName.Valid {
			req.RequestTypeName = &requestTypeName.String
		}
		if requestTypeIcon.Valid {
			req.RequestTypeIcon = &requestTypeIcon.String
		}
		if requestTypeColor.Valid {
			req.RequestTypeColor = &requestTypeColor.String
		}

		requests = append(requests, req)
	}

	if requests == nil {
		requests = []RequestSummary{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(requests)
}

// GetRequestDetail returns detailed information about a specific request
func (h *PortalHandler) GetRequestDetail(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	itemIDStr := r.PathValue("itemId")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

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
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}
	defer rows.Close()

	var found bool
	for rows.Next() {
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Config, &channel.Status); err != nil {
			continue
		}

		// Parse config to check slug
		var config models.ChannelConfig
		if channel.Config != "" {
			if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
				continue
			}
		}

		if config.PortalSlug == slug && config.PortalEnabled {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}

	// Get portal customer ID (supports both portal session and internal user session)
	portalCustomerIDPtr, err := h.getPortalCustomerID(ctx, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	portalCustomerID := *portalCustomerIDPtr

	// Get the request details and verify ownership
	detailQuery := `
		SELECT
			i.id, i.workspace_id, i.workspace_item_number, i.title, i.description,
			i.status_id, i.priority_id, i.created_at, i.updated_at,
			i.channel_id, i.request_type_id, i.creator_portal_customer_id,
			w.name AS workspace_name,
			w.key AS workspace_key,
			rt.name AS request_type_name,
			rt.icon AS request_type_icon,
			rt.color AS request_type_color
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN request_types rt ON i.request_type_id = rt.id
		WHERE i.id = ?
	`

	var item struct {
		ID                      int
		WorkspaceID             int
		WorkspaceItemNumber     int
		Title                   string
		Description             string
		Status                  string
		Priority                string
		CreatedAt               string
		UpdatedAt               string
		ChannelID               *int
		RequestTypeID           *int
		CreatorPortalCustomerID *int
		WorkspaceName           string
		WorkspaceKey            string
		RequestTypeName         sql.NullString
		RequestTypeIcon         sql.NullString
		RequestTypeColor        sql.NullString
	}

	err = h.db.QueryRowContext(ctx, detailQuery, itemID).Scan(
		&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &item.Title, &item.Description,
		&item.Status, &item.Priority, &item.CreatedAt, &item.UpdatedAt,
		&item.ChannelID, &item.RequestTypeID, &item.CreatorPortalCustomerID,
		&item.WorkspaceName, &item.WorkspaceKey,
		&item.RequestTypeName, &item.RequestTypeIcon, &item.RequestTypeColor,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch request: %v", err), http.StatusInternalServerError)
		return
	}

	// Verify that this request was submitted by the authenticated portal customer
	if item.CreatorPortalCustomerID == nil || *item.CreatorPortalCustomerID != portalCustomerID {
		http.Error(w, "You do not have permission to view this request", http.StatusForbidden)
		return
	}

	// Verify that this request was submitted through this portal
	if item.ChannelID == nil || *item.ChannelID != channel.ID {
		http.Error(w, "Request not found in this portal", http.StatusNotFound)
		return
	}

	// Build response
	response := map[string]interface{}{
		"id":                    item.ID,
		"workspace_id":          item.WorkspaceID,
		"workspace_item_number": item.WorkspaceItemNumber,
		"workspace_name":        item.WorkspaceName,
		"workspace_key":         item.WorkspaceKey,
		"title":                 item.Title,
		"description":           item.Description,
		"status":                item.Status,
		"priority":              item.Priority,
		"created_at":            item.CreatedAt,
		"updated_at":            item.UpdatedAt,
		"channel_id":            item.ChannelID,
		"request_type_id":       item.RequestTypeID,
	}

	if item.RequestTypeName.Valid {
		response["request_type_name"] = item.RequestTypeName.String
	}
	if item.RequestTypeIcon.Valid {
		response["request_type_icon"] = item.RequestTypeIcon.String
	}
	if item.RequestTypeColor.Valid {
		response["request_type_color"] = item.RequestTypeColor.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetRequestComments returns comments for a specific request
func (h *PortalHandler) GetRequestComments(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	itemIDStr := r.PathValue("itemId")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

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
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}
	defer rows.Close()

	var found bool
	for rows.Next() {
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Config, &channel.Status); err != nil {
			continue
		}

		// Parse config to check slug
		var config models.ChannelConfig
		if channel.Config != "" {
			if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
				continue
			}
		}

		if config.PortalSlug == slug && config.PortalEnabled {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}

	// Get portal customer ID (supports both portal session and internal user session)
	portalCustomerIDPtr, err := h.getPortalCustomerID(ctx, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	portalCustomerID := *portalCustomerIDPtr

	// Verify the item belongs to this portal customer and was submitted through this channel
	verifyQuery := `
		SELECT creator_portal_customer_id, channel_id
		FROM items
		WHERE id = ?
	`
	var creatorPortalCustomerID *int
	var itemChannelID *int
	err = h.db.QueryRowContext(ctx, verifyQuery, itemID).Scan(&creatorPortalCustomerID, &itemChannelID)
	if err == sql.ErrNoRows {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to verify request", http.StatusInternalServerError)
		return
	}

	// Check ownership and channel
	if creatorPortalCustomerID == nil || *creatorPortalCustomerID != portalCustomerID {
		http.Error(w, "You do not have permission to view comments for this request", http.StatusForbidden)
		return
	}
	if itemChannelID == nil || *itemChannelID != channel.ID {
		http.Error(w, "Request not found in this portal", http.StatusNotFound)
		return
	}

	// Get comments
	commentsQuery := `
		SELECT
			c.id, c.item_id, c.author_id, c.portal_customer_id, c.content, c.created_at, c.updated_at,
			COALESCE(u.first_name || ' ' || u.last_name, pc.name, 'Unknown') AS author_name,
			COALESCE(u.email, pc.email, '') AS author_email
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		LEFT JOIN portal_customers pc ON c.portal_customer_id = pc.id
		WHERE c.item_id = ?
		ORDER BY c.created_at ASC
	`

	commentRows, err := h.db.QueryContext(ctx, commentsQuery, itemID)
	if err != nil {
		http.Error(w, "Failed to fetch comments", http.StatusInternalServerError)
		return
	}
	defer commentRows.Close()

	type Comment struct {
		ID               int    `json:"id"`
		ItemID           int    `json:"item_id"`
		AuthorID         *int   `json:"author_id,omitempty"`
		PortalCustomerID *int   `json:"portal_customer_id,omitempty"`
		Content          string `json:"content"`
		CreatedAt        string `json:"created_at"`
		UpdatedAt        string `json:"updated_at"`
		AuthorName       string `json:"author_name"`
		AuthorEmail      string `json:"author_email"`
	}

	var comments []Comment
	for commentRows.Next() {
		var comment Comment
		var authorID, portalCustomerID sql.NullInt64
		err := commentRows.Scan(
			&comment.ID, &comment.ItemID, &authorID, &portalCustomerID, &comment.Content,
			&comment.CreatedAt, &comment.UpdatedAt, &comment.AuthorName, &comment.AuthorEmail,
		)
		if err != nil {
			continue
		}
		if authorID.Valid {
			id := int(authorID.Int64)
			comment.AuthorID = &id
		}
		if portalCustomerID.Valid {
			id := int(portalCustomerID.Int64)
			comment.PortalCustomerID = &id
		}
		comments = append(comments, comment)
	}

	if comments == nil {
		comments = []Comment{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comments)
}

// AddRequestComment adds a comment to a request from a portal customer
func (h *PortalHandler) AddRequestComment(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	itemIDStr := r.PathValue("itemId")
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

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
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}
	defer rows.Close()

	var found bool
	for rows.Next() {
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Config, &channel.Status); err != nil {
			continue
		}

		// Parse config to check slug
		var config models.ChannelConfig
		if channel.Config != "" {
			if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
				continue
			}
		}

		if config.PortalSlug == slug && config.PortalEnabled {
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}

	// Get portal customer ID (supports both portal session and internal user session)
	portalCustomerIDPtr, err := h.getPortalCustomerID(ctx, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	portalCustomerID := *portalCustomerIDPtr

	// Verify the item belongs to this portal customer and was submitted through this channel
	verifyQuery := `
		SELECT creator_portal_customer_id, channel_id
		FROM items
		WHERE id = ?
	`
	var creatorPortalCustomerID *int
	var itemChannelID *int
	err = h.db.QueryRowContext(ctx, verifyQuery, itemID).Scan(&creatorPortalCustomerID, &itemChannelID)
	if err == sql.ErrNoRows {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to verify request", http.StatusInternalServerError)
		return
	}

	// Check ownership and channel
	if creatorPortalCustomerID == nil || *creatorPortalCustomerID != portalCustomerID {
		http.Error(w, "You do not have permission to comment on this request", http.StatusForbidden)
		return
	}
	if itemChannelID == nil || *itemChannelID != channel.ID {
		http.Error(w, "Request not found in this portal", http.StatusNotFound)
		return
	}

	// Parse comment content
	var commentData struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&commentData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(commentData.Content) == "" {
		http.Error(w, "Comment content is required", http.StatusBadRequest)
		return
	}

	// Sanitize comment content to prevent XSS
	sanitizedContent := utils.StripHTMLTags(commentData.Content)

	// Insert comment with portal_customer_id
	now := time.Now()
	insertQuery := `
		INSERT INTO comments (item_id, portal_customer_id, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := h.db.ExecWriteContext(ctx, insertQuery, itemID, portalCustomerID, sanitizedContent, now, now)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to add comment: %v", err), http.StatusInternalServerError)
		return
	}

	commentID, err := result.LastInsertId()
	if err != nil {
		http.Error(w, "Failed to get comment ID", http.StatusInternalServerError)
		return
	}

	// Return the created comment
	response := map[string]interface{}{
		"id":                  commentID,
		"item_id":             itemID,
		"portal_customer_id":  portalCustomerID,
		"content":             commentData.Content,
		"created_at":          now,
		"updated_at":          now,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}
