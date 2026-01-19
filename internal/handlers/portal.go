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

// GetPortal returns the portal configuration for public display
func (h *PortalHandler) GetPortal(w http.ResponseWriter, r *http.Request) {
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

	// Parse config
	var config models.ChannelConfig
	if channel.Config != "" {
		if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
			http.Error(w, "Invalid portal configuration", http.StatusInternalServerError)
			return
		}
	}

	// Get workspace info (use first workspace for backward compatibility)
	var workspace models.Workspace
	var workspaceID int
	if len(config.PortalWorkspaceIDs) > 0 {
		workspaceID = config.PortalWorkspaceIDs[0]
	}

	if workspaceID > 0 {
		workspaceQuery := `SELECT id, name, key FROM workspaces WHERE id = ?`
		err = h.db.QueryRowContext(ctx, workspaceQuery, workspaceID).Scan(
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

	// Initialize timestamp for use in customer creation and item creation
	now := time.Now()

	// Track virtual field values for storage after item creation
	var virtualFieldValues map[string]interface{}

	// Check if user is authenticated
	var authenticatedUserID *int
	var portalCustomerID *int

	// Try to get session from request
	sessionToken, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		slog.Debug("no session token found", slog.String("component", "portal"), slog.Any("error", err))
	} else {
		slog.Debug("session token found", slog.String("component", "portal"))
		// Validate session
		clientIP := h.getClientIP(r)
		session, err := h.sessionManager.ValidateSession(sessionToken, clientIP)
		if err != nil {
			slog.Debug("session validation failed", slog.String("component", "portal"), slog.Any("error", err))
		} else if session == nil {
			slog.Debug("session is nil", slog.String("component", "portal"))
		} else {
			// User is authenticated
			slog.Debug("user authenticated", slog.String("component", "portal"), slog.Int("user_id", session.UserID))
			authenticatedUserID = &session.UserID
		}
	}

	// Handle portal customer for authenticated vs anonymous users
	if authenticatedUserID == nil {
		// Anonymous submission: require name and email, and create/find portal customer
		if strings.TrimSpace(submission.Name) == "" {
			http.Error(w, "Name is required for portal submissions", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(submission.Email) == "" {
			http.Error(w, "Email is required for portal submissions", http.StatusBadRequest)
			return
		}

		// Find or create portal customer
		var customerID int
		findCustomerQuery := `SELECT id FROM portal_customers WHERE email = ?`
		err := h.db.QueryRowContext(ctx, findCustomerQuery, submission.Email).Scan(&customerID)

		if err == sql.ErrNoRows {
			// Create new portal customer
			insertCustomerQuery := `
				INSERT INTO portal_customers (name, email, created_at, updated_at)
				VALUES (?, ?, ?, ?)
			`
			result, err := h.db.ExecWriteContext(ctx, insertCustomerQuery, submission.Name, submission.Email, now, now)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to create portal customer: %v", err), http.StatusInternalServerError)
				return
			}

			customerIDInt64, err := result.LastInsertId()
			if err != nil {
				http.Error(w, "Failed to get customer ID", http.StatusInternalServerError)
				return
			}
			customerID = int(customerIDInt64)
		} else if err != nil {
			http.Error(w, fmt.Sprintf("Failed to find portal customer: %v", err), http.StatusInternalServerError)
			return
		}

		portalCustomerID = &customerID

		// Grant channel access if not already granted
		var accessID int
		checkAccessQuery := `SELECT id FROM portal_customer_channels WHERE portal_customer_id = ? AND channel_id = ?`
		err = h.db.QueryRowContext(ctx, checkAccessQuery, customerID, channel.ID).Scan(&accessID)

		if err == sql.ErrNoRows {
			// Grant access
			insertAccessQuery := `
				INSERT INTO portal_customer_channels (portal_customer_id, channel_id, created_at)
				VALUES (?, ?, ?)
			`
			_, _ = h.db.ExecWriteContext(ctx, insertAccessQuery, customerID, channel.ID, now)
		}
	} else {
		// Authenticated submission: find or create portal customer linked to user
		var customerID int
		findCustomerQuery := `SELECT id FROM portal_customers WHERE user_id = ?`
		err := h.db.QueryRowContext(ctx, findCustomerQuery, *authenticatedUserID).Scan(&customerID)

		if err == sql.ErrNoRows {
			// Get user details to create portal customer
			var userName, userEmail string
			userQuery := `SELECT first_name || ' ' || last_name, email FROM users WHERE id = ?`
			err := h.db.QueryRowContext(ctx, userQuery, *authenticatedUserID).Scan(&userName, &userEmail)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to get user details: %v", err), http.StatusInternalServerError)
				return
			}

			// Create portal customer linked to user
			insertCustomerQuery := `
				INSERT INTO portal_customers (name, email, user_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?)
			`
			result, err := h.db.ExecWriteContext(ctx, insertCustomerQuery, userName, userEmail, *authenticatedUserID, now, now)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to create portal customer: %v", err), http.StatusInternalServerError)
				return
			}

			customerIDInt64, err := result.LastInsertId()
			if err != nil {
				http.Error(w, "Failed to get customer ID", http.StatusInternalServerError)
				return
			}
			customerID = int(customerIDInt64)
		} else if err != nil {
			http.Error(w, fmt.Sprintf("Failed to find portal customer: %v", err), http.StatusInternalServerError)
			return
		}

		portalCustomerID = &customerID

		// Grant channel access if not already granted
		var accessID int
		checkAccessQuery := `SELECT id FROM portal_customer_channels WHERE portal_customer_id = ? AND channel_id = ?`
		err = h.db.QueryRowContext(ctx, checkAccessQuery, customerID, channel.ID).Scan(&accessID)

		if err == sql.ErrNoRows {
			// Grant access
			insertAccessQuery := `
				INSERT INTO portal_customer_channels (portal_customer_id, channel_id, created_at)
				VALUES (?, ?, ?)
			`
			_, _ = h.db.ExecWriteContext(ctx, insertAccessQuery, customerID, channel.ID, now)
		}
	}

	// Get item type ID from request type if provided
	var itemTypeID *int
	if submission.RequestTypeID != nil {
		// Look up request type to get item_type_id
		var requestType models.RequestType
		rtQuery := `SELECT id, name, item_type_id FROM request_types WHERE id = ? AND is_active = true`
		err := h.db.QueryRowContext(ctx, rtQuery, *submission.RequestTypeID).Scan(
			&requestType.ID, &requestType.Name, &requestType.ItemTypeID,
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid request type (ID: %d): %v", *submission.RequestTypeID, err), http.StatusBadRequest)
			return
		}
		itemTypeID = &requestType.ItemTypeID

		// Load request type fields for validation
		// Track which fields are virtual for later separation
		virtualFieldIDs := make(map[string]bool)
		fieldsQuery := `SELECT field_identifier, field_type, is_required FROM request_type_fields WHERE request_type_id = ? ORDER BY display_order`
		rows, err := h.db.QueryContext(ctx, fieldsQuery, *submission.RequestTypeID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var fieldID string
				var fieldType string
				var isRequired bool
				if err := rows.Scan(&fieldID, &fieldType, &isRequired); err != nil {
					continue
				}

				// Track virtual fields for later separation
				if fieldType == "virtual" {
					virtualFieldIDs[fieldID] = true
				}

				// Validate required fields
				if isRequired {
					if fieldType == "default" {
						if fieldID == "title" && submission.Title == "" {
							http.Error(w, "Title is required", http.StatusBadRequest)
							return
						}
						if fieldID == "description" && submission.Description == "" {
							http.Error(w, "Description is required", http.StatusBadRequest)
							return
						}
					} else if fieldType == "custom" || fieldType == "virtual" {
						if submission.CustomFields == nil || submission.CustomFields[fieldID] == nil || submission.CustomFields[fieldID] == "" {
							http.Error(w, fmt.Sprintf("Field %s is required", fieldID), http.StatusBadRequest)
							return
						}
					}
				}
			}
		}

		// Separate virtual fields from custom fields
		if len(virtualFieldIDs) > 0 && submission.CustomFields != nil {
			virtualFieldValues = make(map[string]interface{})
			customFieldValues := make(map[string]interface{})

			for fieldID, value := range submission.CustomFields {
				if virtualFieldIDs[fieldID] {
					virtualFieldValues[fieldID] = value
				} else {
					customFieldValues[fieldID] = value
				}
			}

			// Update submission.CustomFields to only contain custom fields (not virtual)
			submission.CustomFields = customFieldValues
		}
	} else {
		// Legacy validation for submissions without request type
		if submission.Title == "" {
			http.Error(w, "Title is required", http.StatusBadRequest)
			return
		}
	}

	// Get target workspace (use first workspace for submission)
	if len(config.PortalWorkspaceIDs) == 0 {
		http.Error(w, "Portal has no configured workspaces", http.StatusInternalServerError)
		return
	}
	targetWorkspaceID := config.PortalWorkspaceIDs[0]

	// Determine initial status from workflow if item type is specified
	initialStatus := "open" // Default fallback status
	if itemTypeID != nil {
		status, err := services.GetInitialStatusForItemType(h.db, *itemTypeID)
		if err != nil {
			// Log the error but continue with default status
			slog.Warn("could not determine initial status for item type", slog.String("component", "portal"), slog.Int("item_type_id", *itemTypeID), slog.Any("error", err))
		} else {
			initialStatus = status
		}
	}

	// Create item using centralized service (handles transaction, numbering, frac_index, etc.)
	itemID, err := services.CreateItem(h.db, services.ItemCreationParams{
		WorkspaceID:              targetWorkspaceID,
		Title:                    submission.Title,
		Description:              submission.Description,
		Status:                   initialStatus,
		ItemTypeID:               itemTypeID,
		Priority:                 "medium", // Not customizable yet
		CreatorID:                authenticatedUserID,
		CreatorPortalCustomerID:  portalCustomerID,
		ChannelID:                &channel.ID,          // Track which portal/channel this came from
		RequestTypeID:            submission.RequestTypeID, // Track which request type was used
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create item: %v", err), http.StatusInternalServerError)
		return
	}

	// Store custom field values (after transaction commit)
	if submission.CustomFields != nil && len(submission.CustomFields) > 0 {
		for fieldIDStr, value := range submission.CustomFields {
			// Skip empty values
			if value == nil || value == "" {
				continue
			}

			// Convert value to string for storage
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
				// Insert custom field value
				cfvQuery := `
					INSERT INTO custom_field_values (item_id, custom_field_id, value, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?)
					ON CONFLICT(item_id, custom_field_id) DO UPDATE SET value = ?, updated_at = ?
				`
				_, _ = h.db.ExecWriteContext(ctx, cfvQuery, itemID, fieldIDStr, valueStr, now, now, valueStr, now)
			}
		}

		// Also update the item's custom_field_values JSON column for retrieval compatibility
		customFieldsJSON, err := json.Marshal(submission.CustomFields)
		if err == nil {
			updateItemQuery := `UPDATE items SET custom_field_values = ? WHERE id = ?`
			_, _ = h.db.ExecWriteContext(ctx, updateItemQuery, string(customFieldsJSON), itemID)
		}
	}

	// Store virtual field values (separate from custom fields)
	if virtualFieldValues != nil && len(virtualFieldValues) > 0 {
		virtualFieldsJSON, err := json.Marshal(virtualFieldValues)
		if err == nil {
			updateVirtualFieldsQuery := `UPDATE items SET virtual_field_data = ? WHERE id = ?`
			_, _ = h.db.ExecWriteContext(ctx, updateVirtualFieldsQuery, string(virtualFieldsJSON), itemID)
		}
	}

	// Note: Creator information is now properly tracked via creator_id or creator_portal_customer_id
	// No need to add email as a comment anymore

	// Update channel last activity
	updateChannelQuery := `UPDATE channels SET last_activity = ? WHERE id = ?`
	_, _ = h.db.ExecWriteContext(ctx, updateChannelQuery, now, channel.ID)

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"item_id": itemID,
		"message": "Submission received successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
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
