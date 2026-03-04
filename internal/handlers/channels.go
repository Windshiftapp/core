package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/scheduler"
	"windshift/internal/services"
	windshiftsmtp "windshift/internal/smtp"
	"windshift/internal/utils"
	"windshift/internal/webhook"
)

// ChannelHandler handles HTTP requests for channels
type ChannelHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	webhookSender     *webhook.WebhookSender
	emailScheduler    *scheduler.EmailScheduler
	encryption        email.Encryptor
	baseURL           string
	smtpSender        *windshiftsmtp.NotificationSMTPSender
	service           *services.ChannelService
}

// NewChannelHandler creates a new channel handler
func NewChannelHandler(db database.Database, permissionService *services.PermissionService, webhookSender *webhook.WebhookSender) *ChannelHandler {
	return &ChannelHandler{
		db:                db,
		permissionService: permissionService,
		webhookSender:     webhookSender,
		service:           services.NewChannelService(db, permissionService),
	}
}

// SetEncryption sets the encryption service for OAuth credential handling
func (h *ChannelHandler) SetEncryption(enc email.Encryptor) {
	h.encryption = enc
}

// SetBaseURL sets the base URL for OAuth callbacks
func (h *ChannelHandler) SetBaseURL(baseURL string) {
	h.baseURL = baseURL
}

// SetEmailScheduler sets the email scheduler (used to avoid circular dependencies)
func (h *ChannelHandler) SetEmailScheduler(es *scheduler.EmailScheduler) {
	h.emailScheduler = es
}

// SetSMTPSender sets the SMTP sender for sending test emails
func (h *ChannelHandler) SetSMTPSender(sender *windshiftsmtp.NotificationSMTPSender) {
	h.smtpSender = sender
}

// GetChannels returns all channels (admins) or only managed channels (non-admins)
func (h *ChannelHandler) GetChannels(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Parse category_id filter from query params
	categoryFilter := r.URL.Query().Get("category_id")

	var filters services.ChannelListFilters
	if categoryFilter != "" {
		if categoryFilter == "null" {
			val := -1
			filters.CategoryID = &val
		} else if catID, err := strconv.Atoi(categoryFilter); err == nil {
			filters.CategoryID = &catID
		}
	}

	channels, err := h.service.List(ctx, user.ID, filters)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(channels)
}

// CreateChannel creates a new channel
func (h *ChannelHandler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var req struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Direction   string `json:"direction"`
		Description string `json:"description"`
		Status      string `json:"status"`
		IsDefault   bool   `json:"is_default"`
		Config      string `json:"config"`
		CategoryID  *int   `json:"category_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	channel, err := h.service.Create(ctx, services.ChannelCreateRequest{
		Name:        req.Name,
		Type:        req.Type,
		Direction:   req.Direction,
		Description: req.Description,
		Status:      req.Status,
		IsDefault:   req.IsDefault,
		Config:      req.Config,
		CategoryID:  req.CategoryID,
	})
	if err != nil {
		if err.Error() == "name, type, and direction are required" {
			respondValidationError(w, r, err.Error())
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		channelID := channel.ID
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionChannelCreate,
			ResourceType: logger.ResourceChannel,
			ResourceID:   &channelID,
			ResourceName: channel.Name,
			Success:      true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(channel)
}

// GetChannel returns a specific channel by ID
func (h *ChannelHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	channel, err := h.service.GetByID(ctx, id)
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "channel")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(channel)
}

// UpdateChannel updates an existing channel
func (h *ChannelHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	var updates models.Channel
	if err = json.NewDecoder(r.Body).Decode(&updates); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if channel exists and is not a plugin-managed channel
	var exists bool
	var pluginName *string
	var existingConfig string
	checkQuery := "SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?), (SELECT plugin_name FROM channels WHERE id = ?), (SELECT config FROM channels WHERE id = ?)"
	err = h.db.QueryRowContext(ctx, checkQuery, id, id, id).Scan(&exists, &pluginName, &existingConfig)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "channel")
		return
	}
	if pluginName != nil && *pluginName != "" {
		respondForbidden(w, r)
		return
	}

	updates.UpdatedAt = time.Now()

	// Preserve existing config if not provided or if it looks scrubbed
	// Config updates should go through UpdateChannelConfig which handles merging properly
	if updates.Config == "" {
		updates.Config = existingConfig
	}

	query := `
		UPDATE channels
		SET name = ?, type = ?, direction = ?, description = ?, status = ?,
		    is_default = ?, config = ?, category_id = ?, updated_at = ?
		WHERE id = ?
	`

	_, err = h.db.ExecWriteContext(ctx, query,
		updates.Name, updates.Type, updates.Direction, updates.Description,
		updates.Status, updates.IsDefault, updates.Config, updates.CategoryID, updates.UpdatedAt, id,
	)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionChannelUpdate,
			ResourceType: logger.ResourceChannel,
			ResourceID:   &id,
			ResourceName: updates.Name,
			Success:      true,
		})
	}

	// Return the updated channel
	h.GetChannel(w, r)
}

// DeleteChannel deletes a channel
func (h *ChannelHandler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if channel exists, is not default, and is not plugin-managed
	var exists bool
	var isDefault bool
	var pluginName *string
	checkQuery := `
		SELECT
			EXISTS(SELECT 1 FROM channels WHERE id = ?),
			COALESCE((SELECT is_default FROM channels WHERE id = ?), false),
			(SELECT plugin_name FROM channels WHERE id = ?)
	`
	err = h.db.QueryRowContext(ctx, checkQuery, id, id, id).Scan(&exists, &isDefault, &pluginName)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "channel")
		return
	}
	if isDefault {
		respondValidationError(w, r, "Cannot delete default channel")
		return
	}
	if pluginName != nil && *pluginName != "" {
		respondForbidden(w, r)
		return
	}

	err = h.service.Delete(ctx, id)
	if err != nil {
		if err == repository.ErrNotFound {
			respondNotFound(w, r, "channel")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionChannelDelete,
			ResourceType: logger.ResourceChannel,
			ResourceID:   &id,
			Success:      true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestChannel tests a channel configuration by sending a test email
func (h *ChannelHandler) TestChannel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	// Parse request body for test email address
	var testRequest struct {
		TestEmail string `json:"test_email"`
	}
	if err = json.NewDecoder(r.Body).Decode(&testRequest); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	if testRequest.TestEmail == "" {
		respondValidationError(w, r, "test_email is required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Longer timeout for network operations
	defer cancel()

	// Get channel configuration
	var channel models.Channel
	query := `
		SELECT id, name, type, direction, description, status, is_default, config,
			   plugin_name, plugin_webhook_id, created_at, updated_at, last_activity
		FROM channels
		WHERE id = ?
	`

	err = h.db.QueryRowContext(ctx, query, id).Scan(
		&channel.ID, &channel.Name, &channel.Type, &channel.Direction,
		&channel.Description, &channel.Status, &channel.IsDefault, &channel.Config,
		&channel.PluginName, &channel.PluginWebhookID,
		&channel.CreatedAt, &channel.UpdatedAt, &channel.LastActivity,
	)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "channel")
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Test the channel based on its type
	result := make(map[string]interface{})
	result["channel_id"] = channel.ID
	result["channel_name"] = channel.Name
	result["test_time"] = time.Now()
	result["test_email"] = testRequest.TestEmail

	switch channel.Type {
	case "smtp":
		success, message := h.testSMTPChannelWithEmail(channel, testRequest.TestEmail)
		result["success"] = success
		result["message"] = message
		if success {
			// Update last activity
			h.updateChannelActivity(ctx, channel.ID)
		}
	default:
		result["success"] = false
		result["message"] = fmt.Sprintf("Testing not implemented for channel type: %s", channel.Type)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// TestChannelConfig tests a channel configuration without saving it
func (h *ChannelHandler) TestChannelConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	var testData struct {
		Config models.ChannelConfig `json:"config"`
	}
	if err = json.NewDecoder(r.Body).Decode(&testData); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Longer timeout for network operations
	defer cancel()

	// Get channel type from database
	var channelType string
	query := "SELECT type FROM channels WHERE id = ?"
	err = h.db.QueryRowContext(ctx, query, id).Scan(&channelType)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "channel")
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Test the configuration based on channel type
	result := make(map[string]interface{})
	result["channel_id"] = id
	result["test_time"] = time.Now()

	switch channelType {
	case "smtp":
		result["success"] = h.testSMTPConfig(testData.Config)
		if ok := result["success"].(bool); ok { //nolint:errcheck // type assertion always succeeds for bool
			result["message"] = "SMTP connection successful"
		} else {
			result["message"] = "SMTP connection failed"
		}
	case "webhook":
		if h.webhookSender != nil {
			success, message := h.webhookSender.SendTestWebhook(ctx, &testData.Config)
			result["success"] = success
			result["message"] = message
		} else {
			result["success"] = false
			result["message"] = "Webhook sender not configured"
		}
	default:
		result["success"] = false
		result["message"] = fmt.Sprintf("Testing not supported for channel type: %s", channelType)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// testSMTPChannelWithEmail tests an SMTP channel by sending a test email
func (h *ChannelHandler) testSMTPChannelWithEmail(channel models.Channel, testEmail string) (success bool, message string) {
	// Parse SMTP configuration
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return false, "Failed to parse SMTP configuration: " + err.Error()
	}

	// Basic validation
	if config.SMTPHost == "" {
		return false, "SMTP host is not configured"
	}
	if config.SMTPPort == 0 {
		return false, "SMTP port is not configured"
	}
	if config.SMTPFromEmail == "" {
		return false, "From email is not configured"
	}

	// Create a test email
	subject := "Windshift SMTP Test Email"
	htmlBody := `<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<title>Windshift SMTP Test</title>
	<style>
		body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background-color: #f5f5f5; }
		.container { max-width: 600px; margin: 0 auto; background-color: white; border-radius: 8px; padding: 24px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
		.header { text-align: center; color: #2563eb; margin-bottom: 24px; }
		.content { color: #374151; line-height: 1.6; }
		.success { background-color: #dcfce7; border: 1px solid #16a34a; color: #15803d; padding: 12px; border-radius: 6px; margin: 16px 0; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Windshift SMTP Test</h1>
		</div>
		<div class="content">
			<div class="success">
				<strong>Success!</strong> Your SMTP configuration is working correctly.
			</div>
			<p>This test email was sent from Windshift to verify your SMTP settings.</p>
			<p><strong>Channel:</strong> ` + channel.Name + `</p>
			<p><strong>Test Time:</strong> ` + time.Now().Format("January 2, 2006 at 3:04 PM MST") + `</p>
			<p>If you received this email, your SMTP configuration is ready to send notifications.</p>
		</div>
	</div>
</body>
</html>`

	textBody := `Windshift SMTP Test Email

Success! Your SMTP configuration is working correctly.

This test email was sent from Windshift to verify your SMTP settings.

Channel: ` + channel.Name + `
Test Time: ` + time.Now().Format("January 2, 2006 at 3:04 PM MST") + `

If you received this email, your SMTP configuration is ready to send notifications.`

	// Check if SMTP sender is configured
	if h.smtpSender == nil {
		return false, "SMTP sender not configured"
	}

	// Send the test email using the shared SMTP sender
	err := h.smtpSender.SendEmailWithConfig(&config, testEmail, subject, htmlBody, textBody)
	if err != nil {
		// Provide more specific error guidance based on common SMTP errors
		errorMsg := err.Error()
		switch {
		case strings.Contains(errorMsg, "502"):
			return false, fmt.Sprintf("SMTP server error (502): %s. This usually means the server doesn't support the requested command. Try checking your server settings or use a different encryption method.", errorMsg)
		case strings.Contains(errorMsg, "530"):
			return false, fmt.Sprintf("Authentication failed (530): %s. Please check your username and password.", errorMsg)
		case strings.Contains(errorMsg, "535"):
			return false, fmt.Sprintf("Authentication credentials invalid (535): %s. Please verify your username and password are correct.", errorMsg)
		case strings.Contains(errorMsg, "connection refused"), strings.Contains(errorMsg, "no such host"):
			return false, fmt.Sprintf("Connection failed: %s. Please check your SMTP host and port settings.", errorMsg)
		default:
			return false, "Failed to send test email: " + errorMsg
		}
	}

	return true, "Test email sent successfully to " + testEmail
}

// testSMTPConfig tests SMTP configuration directly
func (h *ChannelHandler) testSMTPConfig(config models.ChannelConfig) bool {
	// Basic validation
	if config.SMTPHost == "" || config.SMTPPort == 0 {
		return false
	}

	// Test actual SMTP connection
	addr := net.JoinHostPort(config.SMTPHost, strconv.Itoa(config.SMTPPort))

	// Set connection timeout
	timeout := 10 * time.Second

	// Attempt to connect to SMTP server
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		// Log SMTP connection failure
		logger.Get().Debug("SMTP connection failed", "error", err)
		return false
	}
	defer func() { _ = conn.Close() }() //nolint:gocritic // defer ensures cleanup even on panic

	return true // Connection test successful
}

// updateChannelActivity updates the last_activity timestamp for a channel
func (h *ChannelHandler) updateChannelActivity(ctx context.Context, channelID int) {
	query := "UPDATE channels SET last_activity = ? WHERE id = ?"
	_, _ = h.db.ExecWriteContext(ctx, query, time.Now(), channelID)
}

// UpdateChannelConfig updates only the configuration of a channel
func (h *ChannelHandler) UpdateChannelConfig(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	var rawRequest map[string]json.RawMessage
	if err = json.NewDecoder(r.Body).Decode(&rawRequest); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	rawConfig, ok := rawRequest["config"]
	if !ok {
		respondValidationError(w, r, "Missing config field")
		return
	}

	var incomingConfig map[string]interface{}
	if err = json.Unmarshal(rawConfig, &incomingConfig); err != nil {
		respondValidationError(w, r, "Invalid config JSON")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get current channel status, name, existing config, and plugin info for audit logging and merging
	var oldStatus string
	var channelName string
	var existingConfigJSON string
	var pluginName *string
	checkQuery := "SELECT status, name, config, plugin_name FROM channels WHERE id = ?"
	err = h.db.QueryRowContext(ctx, checkQuery, id).Scan(&oldStatus, &channelName, &existingConfigJSON, &pluginName)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "channel")
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Prevent modification of plugin-managed channels
	if pluginName != nil && *pluginName != "" {
		respondForbidden(w, r)
		return
	}

	// Merge existing config with new config to preserve unmodified fields
	var mergedConfig map[string]interface{}

	// Unmarshal existing config into map
	if existingConfigJSON != "" {
		if err = json.Unmarshal([]byte(existingConfigJSON), &mergedConfig); err != nil {
			// If existing config is invalid, start with empty map
			mergedConfig = make(map[string]interface{})
		}
	} else {
		mergedConfig = make(map[string]interface{})
	}

	// Merge: incoming config overwrites existing config for keys that are present
	for key, value := range incomingConfig {
		mergedConfig[key] = value
	}

	// Convert merged config back to JSON
	configJSON, err := json.Marshal(mergedConfig)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Unmarshal merged config into ChannelConfig struct for validation
	var finalConfig models.ChannelConfig
	if err = json.Unmarshal(configJSON, &finalConfig); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Validate knowledge base URL if set
	if finalConfig.KnowledgeBaseURL != "" {
		if err := utils.ValidateExternalURL(finalConfig.KnowledgeBaseURL); err != nil {
			respondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Knowledge base URL must be a valid public HTTPS URL"))
			return
		}
	}

	// Determine status based solely on whether the feature is enabled
	// Frontend is responsible for validating required fields before submission
	newStatus := "disabled"

	// Check if portal is enabled
	if finalConfig.PortalEnabled {
		newStatus = "enabled"
	}

	// Check if email channel is enabled
	if finalConfig.EmailEnabled {
		newStatus = "enabled"
	}

	// Check if SMTP is configured (has host set)
	if finalConfig.SMTPHost != "" {
		newStatus = "enabled"
	}

	query := `
		UPDATE channels
		SET config = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	_, err = h.db.ExecWriteContext(ctx, query, string(configJSON), newStatus, time.Now(), id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event if status changed (activation or deactivation)
	if oldStatus != newStatus {
		var actionType string
		if newStatus == "enabled" && (oldStatus == "disabled" || oldStatus == "" || oldStatus == "pending" || oldStatus == "configured") {
			actionType = logger.ActionChannelActivate
		} else if newStatus == "disabled" && (oldStatus == "enabled" || oldStatus == "configured") {
			actionType = logger.ActionChannelDeactivate
		}

		if actionType != "" {
			_ = logger.LogAudit(h.db, logger.AuditEvent{
				UserID:       user.ID,
				Username:     user.Username,
				IPAddress:    r.RemoteAddr,
				UserAgent:    r.UserAgent(),
				ActionType:   actionType,
				ResourceType: logger.ResourceChannel,
				ResourceID:   &id,
				ResourceName: channelName,
				Details: map[string]interface{}{
					"old_status": oldStatus,
					"new_status": newStatus,
				},
				Success: true,
			})
		}
	}

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": "Channel configuration updated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// GetChannelManagers returns all managers for a channel
func (h *ChannelHandler) GetChannelManagers(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify channel exists
	exists, err := h.service.Exists(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "channel")
		return
	}

	// Get managers with joined information
	query := `
		SELECT
			cm.id, cm.channel_id, cm.manager_type, cm.manager_id,
			cm.added_by, cm.created_at, cm.updated_at,
			CASE
				WHEN cm.manager_type = 'user' THEN (u.first_name || ' ' || u.last_name)
				WHEN cm.manager_type = 'group' THEN g.name
				ELSE NULL
			END as manager_name,
			CASE
				WHEN cm.manager_type = 'user' THEN u.email
				ELSE NULL
			END as manager_email,
			(added_by_user.first_name || ' ' || added_by_user.last_name) as added_by_name
		FROM channel_managers cm
		LEFT JOIN users u ON cm.manager_type = 'user' AND cm.manager_id = u.id
		LEFT JOIN groups g ON cm.manager_type = 'group' AND cm.manager_id = g.id
		LEFT JOIN users added_by_user ON cm.added_by = added_by_user.id
		WHERE cm.channel_id = ?
		ORDER BY cm.created_at ASC
	`

	rows, err := h.db.QueryContext(ctx, query, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var managers []models.ChannelManager
	for rows.Next() {
		var manager models.ChannelManager
		var addedBy sql.NullInt64
		var managerName sql.NullString
		var managerEmail sql.NullString
		var addedByName sql.NullString

		err = rows.Scan(
			&manager.ID, &manager.ChannelID, &manager.ManagerType, &manager.ManagerID,
			&addedBy, &manager.CreatedAt, &manager.UpdatedAt,
			&managerName, &managerEmail, &addedByName,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if addedBy.Valid {
			val := int(addedBy.Int64)
			manager.AddedBy = &val
		}
		if managerName.Valid {
			manager.ManagerName = managerName.String
		}
		if managerEmail.Valid {
			manager.ManagerEmail = managerEmail.String
		}
		if addedByName.Valid {
			manager.AddedByName = addedByName.String
		}

		managers = append(managers, manager)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(managers)
}

// AddChannelManager adds managers to a channel
func (h *ChannelHandler) AddChannelManager(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	channelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	var request models.ChannelManagerRequest
	if err = json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	// Validate request
	if request.ManagerType != "user" && request.ManagerType != "group" {
		respondValidationError(w, r, "manager_type must be 'user' or 'group'")
		return
	}
	if len(request.ManagerIDs) == 0 {
		respondValidationError(w, r, "manager_ids must contain at least one ID")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get channel name for audit logging
	var channelName string
	nameQuery := "SELECT name FROM channels WHERE id = ?"
	err = h.db.QueryRowContext(ctx, nameQuery, channelID).Scan(&channelName)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "channel")
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Set added_by to current user
	addedBy := user.ID

	// Insert managers
	insertQuery := `
		INSERT OR IGNORE INTO channel_managers (channel_id, manager_type, manager_id, added_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	for _, managerID := range request.ManagerIDs {
		_, err := h.db.ExecWriteContext(ctx, insertQuery,
			channelID, request.ManagerType, managerID, addedBy, now, now,
		)
		if err != nil {
			// Check if it's a foreign key violation (user/group doesn't exist)
			if strings.Contains(err.Error(), "FOREIGN KEY") || strings.Contains(err.Error(), "foreign key") {
				respondValidationError(w, r, fmt.Sprintf("Invalid %s ID: %d does not exist", request.ManagerType, managerID))
				return
			}
			respondInternalError(w, r, err)
			return
		}

		// Get manager name for audit log
		var managerName string
		switch request.ManagerType {
		case "user":
			var firstName, lastName string
			nameQuery := "SELECT first_name, last_name FROM users WHERE id = ?"
			err = h.db.QueryRowContext(ctx, nameQuery, managerID).Scan(&firstName, &lastName)
			if err == nil {
				managerName = fmt.Sprintf("%s %s", firstName, lastName)
			}
		case "group":
			nameQuery := "SELECT name FROM groups WHERE id = ?"
			_ = h.db.QueryRowContext(ctx, nameQuery, managerID).Scan(&managerName)
		}

		// Log audit event
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       user.ID,
			Username:     user.Username,
			IPAddress:    r.RemoteAddr,
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionChannelAddManager,
			ResourceType: logger.ResourceChannelManager,
			ResourceID:   &channelID,
			ResourceName: channelName,
			Details: map[string]interface{}{
				"manager_type": request.ManagerType,
				"manager_id":   managerID,
				"manager_name": managerName,
			},
			Success: true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Added %d manager(s) to channel", len(request.ManagerIDs)),
	})
}

// RemoveChannelManager removes a manager from a channel
func (h *ChannelHandler) RemoveChannelManager(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	channelID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	managerID, err := strconv.Atoi(r.PathValue("managerId"))
	if err != nil {
		respondInvalidID(w, r, "manager ID")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get channel name for audit logging
	var channelName string
	nameQuery := "SELECT name FROM channels WHERE id = ?"
	err = h.db.QueryRowContext(ctx, nameQuery, channelID).Scan(&channelName)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "channel")
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get manager info before deleting for audit log
	var managerType string
	var actualManagerID int
	managerInfoQuery := "SELECT manager_type, manager_id FROM channel_managers WHERE id = ? AND channel_id = ?"
	err = h.db.QueryRowContext(ctx, managerInfoQuery, managerID, channelID).Scan(&managerType, &actualManagerID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "manager")
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get manager name for audit log
	var managerName string
	switch managerType {
	case "user":
		var firstName, lastName string
		nameQuery := "SELECT first_name, last_name FROM users WHERE id = ?"
		err = h.db.QueryRowContext(ctx, nameQuery, actualManagerID).Scan(&firstName, &lastName)
		if err == nil {
			managerName = fmt.Sprintf("%s %s", firstName, lastName)
		}
	case "group":
		nameQuery := "SELECT name FROM groups WHERE id = ?"
		_ = h.db.QueryRowContext(ctx, nameQuery, actualManagerID).Scan(&managerName)
	}

	// Delete the manager
	deleteQuery := "DELETE FROM channel_managers WHERE id = ? AND channel_id = ?"
	result, err := h.db.ExecWriteContext(ctx, deleteQuery, managerID, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "manager")
		return
	}

	// Log audit event
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    r.RemoteAddr,
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionChannelRemoveManager,
		ResourceType: logger.ResourceChannelManager,
		ResourceID:   &channelID,
		ResourceName: channelName,
		Details: map[string]interface{}{
			"manager_type": managerType,
			"manager_id":   actualManagerID,
			"manager_name": managerName,
		},
		Success: true,
	})

	w.WriteHeader(http.StatusNoContent)
}

// ProcessEmailsNow triggers immediate processing of emails for an inbound email channel.
// This is primarily used for testing to avoid waiting for the scheduler interval.
// POST /api/channels/{id}/process-emails
func (h *ChannelHandler) ProcessEmailsNow(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check if user is a system admin
	isSystemAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil || !isSystemAdmin {
		respondAdminRequired(w, r)
		return
	}

	// Get channel ID from path
	channelIDStr := r.PathValue("id")
	channelID, err := strconv.Atoi(channelIDStr)
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	// Verify channel exists and is an inbound email channel
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var channelType, direction string
	err = h.db.QueryRowContext(ctx, `
		SELECT type, direction FROM channels WHERE id = ?
	`, channelID).Scan(&channelType, &direction)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "channel")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if channelType != "email" || direction != "inbound" {
		respondValidationError(w, r, "Channel is not an inbound email channel")
		return
	}

	// Check if email scheduler is available
	if h.emailScheduler == nil {
		respondError(w, r, &restapi.APIError{
			StatusCode: http.StatusServiceUnavailable,
			Code:       "SERVICE_UNAVAILABLE",
			Message:    "Email scheduler not available",
		})
		return
	}

	// Trigger processing
	err = h.emailScheduler.ProcessChannelNow(channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"channel_id": channelID,
		"message":    "Email processing triggered",
	})
}

// GetEmailLog returns the email processing log for a channel
// GET /channels/{id}/email-log?page=1&page_size=50
func (h *ChannelHandler) GetEmailLog(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	// Parse pagination params
	page := 1
	pageSize := 50
	if p := r.URL.Query().Get("page"); p != "" {
		var v int
		if v, err = strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		var v int
		if v, err = strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
			pageSize = v
		}
	}
	search := r.URL.Query().Get("search")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Verify channel exists and is an email channel
	var channelType string
	err = h.db.QueryRowContext(ctx, "SELECT type FROM channels WHERE id = ?", id).Scan(&channelType)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "channel")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channelType != "email" {
		respondValidationError(w, r, "Channel is not an email channel")
		return
	}

	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	// Get channel state
	type emailChannelState struct {
		LastCheckedAt *time.Time `json:"last_checked_at"`
		LastUID       int        `json:"last_uid"`
		ErrorCount    int        `json:"error_count"`
		LastError     string     `json:"last_error"`
	}

	var state emailChannelState
	var lastCheckedAt sql.NullTime
	var lastError sql.NullString
	err = h.db.QueryRowContext(ctx,
		"SELECT last_uid, last_checked_at, error_count, last_error FROM email_channel_state WHERE channel_id = ?",
		id,
	).Scan(&state.LastUID, &lastCheckedAt, &state.ErrorCount, &lastError)
	if err != nil && err != sql.ErrNoRows {
		respondInternalError(w, r, err)
		return
	}
	if lastCheckedAt.Valid {
		state.LastCheckedAt = &lastCheckedAt.Time
	}
	if lastError.Valid {
		state.LastError = lastError.String
	}

	// Build WHERE clause with optional search filter
	whereClause := "WHERE emt.channel_id = ?"
	args := []interface{}{id}
	if search != "" {
		searchPattern := "%" + search + "%"
		whereClause += " AND (emt.from_email LIKE ? OR emt.from_name LIKE ? OR emt.subject LIKE ?)"
		args = append(args, searchPattern, searchPattern, searchPattern)
	}

	// Get total count
	var total int
	err = h.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM email_message_tracking emt "+whereClause,
		args...,
	).Scan(&total)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get paginated messages
	offset := (page - 1) * pageSize
	queryArgs := append([]interface{}{}, args...) //nolint:gocritic // intentionally creating new slice to add pagination params
	queryArgs = append(queryArgs, pageSize, offset)
	rows, err := h.db.QueryContext(ctx,
		"SELECT emt.id, emt.from_email, emt.from_name, emt.subject, emt.item_id, emt.comment_id, emt.processed_at, i.workspace_item_number, i.workspace_id, w.key as workspace_key FROM email_message_tracking emt LEFT JOIN items i ON emt.item_id = i.id LEFT JOIN workspaces w ON i.workspace_id = w.id "+whereClause+" ORDER BY emt.processed_at DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type emailMessage struct {
		ID                  int       `json:"id"`
		FromEmail           string    `json:"from_email"`
		FromName            string    `json:"from_name"`
		Subject             string    `json:"subject"`
		ItemID              *int      `json:"item_id"`
		CommentID           *int      `json:"comment_id"`
		ProcessedAt         time.Time `json:"processed_at"`
		WorkspaceKey        string    `json:"workspace_key,omitempty"`
		WorkspaceItemNumber int       `json:"workspace_item_number,omitempty"`
	}

	type scannedMessage struct {
		msg         emailMessage
		workspaceID sql.NullInt64
	}

	var scannedMessages []scannedMessage
	workspaceIDs := map[int]bool{}
	for rows.Next() {
		var sm scannedMessage
		var itemID, commentID sql.NullInt64
		var fromName sql.NullString
		var workspaceItemNumber sql.NullInt64
		var workspaceKey sql.NullString
		err = rows.Scan(&sm.msg.ID, &sm.msg.FromEmail, &fromName, &sm.msg.Subject, &itemID, &commentID, &sm.msg.ProcessedAt, &workspaceItemNumber, &sm.workspaceID, &workspaceKey)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if fromName.Valid {
			sm.msg.FromName = fromName.String
		}
		if itemID.Valid {
			v := int(itemID.Int64)
			sm.msg.ItemID = &v
		}
		if commentID.Valid {
			v := int(commentID.Int64)
			sm.msg.CommentID = &v
		}
		if workspaceItemNumber.Valid {
			sm.msg.WorkspaceItemNumber = int(workspaceItemNumber.Int64)
		}
		if workspaceKey.Valid {
			sm.msg.WorkspaceKey = workspaceKey.String
		}
		if sm.workspaceID.Valid {
			workspaceIDs[int(sm.workspaceID.Int64)] = true
		}
		scannedMessages = append(scannedMessages, sm)
	}
	if err = rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check workspace permissions for all unique workspaces
	allowedWS := map[int]bool{}
	for wsID := range workspaceIDs {
		allowed, err := h.permissionService.HasWorkspacePermission(user.ID, wsID, models.PermissionItemView)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		allowedWS[wsID] = allowed
	}

	// Build final messages, only including workspace key if user has permission
	var messages []emailMessage
	for _, sm := range scannedMessages {
		msg := sm.msg
		if sm.workspaceID.Valid && !allowedWS[int(sm.workspaceID.Int64)] {
			msg.WorkspaceKey = ""
			msg.WorkspaceItemNumber = 0
		}
		messages = append(messages, msg)
	}

	if messages == nil {
		messages = []emailMessage{}
	}

	response := map[string]interface{}{
		"state":     state,
		"messages":  messages,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// Default OAuth scopes for email providers
var defaultEmailOAuthScopes = map[string][]string{
	"microsoft": {
		"https://outlook.office365.com/IMAP.AccessAsUser.All",
		"https://outlook.office365.com/SMTP.Send",
		"openid",
		"profile",
		"email",
		"offline_access",
	},
	"google": {
		"https://mail.google.com/",
		"openid",
		"email",
		"profile",
	},
}

// StartChannelEmailOAuth initiates OAuth flow using channel's inline credentials
// POST /api/channels/{id}/email-oauth/start
func (h *ChannelHandler) StartChannelEmailOAuth(w http.ResponseWriter, r *http.Request) {
	// Get user ID
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	// Get channel ID
	channelIDStr := r.PathValue("id")
	channelID, err := strconv.Atoi(channelIDStr)
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get channel config
	var configJSON string
	var channelType string
	err = h.db.QueryRowContext(ctx, `
		SELECT config, type FROM channels WHERE id = ?
	`, channelID).Scan(&configJSON, &channelType)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "channel")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if channelType != "email" {
		respondValidationError(w, r, "Channel is not an email channel")
		return
	}

	// Parse config
	var config models.ChannelConfig
	if configJSON != "" {
		if err = json.Unmarshal([]byte(configJSON), &config); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Validate inline OAuth credentials
	if config.EmailOAuthProviderType == "" {
		respondValidationError(w, r, "OAuth provider type not configured")
		return
	}
	if config.EmailOAuthClientID == "" {
		respondValidationError(w, r, "OAuth client ID not configured")
		return
	}
	if config.EmailOAuthClientSecret == "" {
		respondValidationError(w, r, "OAuth client secret not configured")
		return
	}

	// Decrypt client secret
	var clientSecret string
	if h.encryption != nil {
		clientSecret, err = h.encryption.Decrypt(config.EmailOAuthClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	} else {
		clientSecret = config.EmailOAuthClientSecret
	}

	// Generate state token
	stateBytes := make([]byte, 32)
	if _, err = rand.Read(stateBytes); err != nil {
		respondInternalError(w, r, err)
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Store state in database (expires in 5 minutes)
	// Reuse the email_oauth_state table but with provider_id = 0 for inline OAuth
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = h.db.ExecWriteContext(ctx, `
		INSERT INTO email_oauth_state (provider_id, channel_id, state, user_id, expires_at)
		VALUES (0, ?, ?, ?, ?)
	`, channelID, state, user.ID, expiresAt)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Build redirect URI
	redirectURI := fmt.Sprintf("%s/api/channels/inline-oauth/callback", h.baseURL)

	// Get OAuth URL based on provider type
	var authURL string
	scopes := defaultEmailOAuthScopes[config.EmailOAuthProviderType]

	switch config.EmailOAuthProviderType {
	case "microsoft":
		tenant := config.EmailOAuthTenantID
		if tenant == "" {
			tenant = "common"
		}
		p := email.NewMicrosoftProvider(config.EmailOAuthClientID, clientSecret, tenant, scopes)
		authURL = p.GetOAuthURL(state, redirectURI)
	case "google":
		p := email.NewGoogleProvider(config.EmailOAuthClientID, clientSecret, scopes)
		authURL = p.GetOAuthURL(state, redirectURI)
	default:
		respondValidationError(w, r, "Unsupported OAuth provider type")
		return
	}

	slog.Info("starting inline OAuth flow for email channel",
		"channel_id", channelID,
		"provider_type", config.EmailOAuthProviderType,
		"user_id", user.ID,
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"auth_url": authURL,
	})
}

// ChannelEmailOAuthCallback handles the OAuth callback for channel-level OAuth
// GET /api/channels/email-oauth/callback
func (h *ChannelHandler) ChannelEmailOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		slog.Error("OAuth error", "error", errorParam, "description", errorDesc)
		// URL-encode the error parameter to prevent open redirect attacks
		http.Redirect(w, r, "/channels?oauth_error="+url.QueryEscape(errorParam), http.StatusFound)
		return
	}

	if code == "" || state == "" {
		respondValidationError(w, r, "Missing code or state parameter")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Validate state and get associated data
	var providerID, channelID, userID int
	err := h.db.QueryRowContext(ctx, `
		SELECT provider_id, channel_id, user_id
		FROM email_oauth_state
		WHERE state = ? AND expires_at > CURRENT_TIMESTAMP
	`, state).Scan(&providerID, &channelID, &userID)
	if err == sql.ErrNoRows {
		respondValidationError(w, r, "Invalid or expired state")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete used state
	_, _ = h.db.ExecWriteContext(ctx, `DELETE FROM email_oauth_state WHERE state = ?`, state)

	// Get channel config
	var configJSON string
	err = h.db.QueryRowContext(ctx, `SELECT config FROM channels WHERE id = ?`, channelID).Scan(&configJSON)
	if err != nil {
		slog.Error("failed to get channel config", "error", err, "channel_id", channelID)
		http.Redirect(w, r, "/channels?oauth_error=channel_not_found", http.StatusFound)
		return
	}

	var config models.ChannelConfig
	if configJSON != "" {
		if err = json.Unmarshal([]byte(configJSON), &config); err != nil {
			http.Redirect(w, r, "/channels?oauth_error=invalid_config", http.StatusFound)
			return
		}
	}

	// Decrypt client secret
	var clientSecret string
	if h.encryption != nil && config.EmailOAuthClientSecret != "" {
		clientSecret, _ = h.encryption.Decrypt(config.EmailOAuthClientSecret)
	}

	// Build redirect URI (must match the one used in StartOAuth)
	redirectURI := fmt.Sprintf("%s/api/channels/inline-oauth/callback", h.baseURL)

	// Exchange code for tokens
	var tokens *email.OAuthTokens
	var userEmail string
	scopes := defaultEmailOAuthScopes[config.EmailOAuthProviderType]

	switch config.EmailOAuthProviderType {
	case "microsoft":
		tenant := config.EmailOAuthTenantID
		if tenant == "" {
			tenant = "common"
		}
		p := email.NewMicrosoftProvider(config.EmailOAuthClientID, clientSecret, tenant, scopes)
		tokens, err = p.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			slog.Error("failed to exchange code", "error", err)
			http.Redirect(w, r, "/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}
		userEmail, _ = p.GetUserEmail(ctx, tokens.AccessToken)

	case "google":
		p := email.NewGoogleProvider(config.EmailOAuthClientID, clientSecret, scopes)
		tokens, err = p.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			slog.Error("failed to exchange code", "error", err)
			http.Redirect(w, r, "/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}
		userEmail, _ = p.GetUserEmail(ctx, tokens.AccessToken)

	default:
		http.Redirect(w, r, "/channels?oauth_error=unsupported_provider", http.StatusFound)
		return
	}

	// Save tokens to channel config
	credManager := email.NewCredentialManager(h.db, h.encryption)
	err = credManager.SaveOAuthTokens(ctx, channelID, tokens, userEmail)
	if err != nil {
		slog.Error("failed to save tokens", "error", err)
		http.Redirect(w, r, "/channels?oauth_error=save_failed", http.StatusFound)
		return
	}

	slog.Info("OAuth completed for email channel (inline credentials)",
		"channel_id", channelID,
		"email", userEmail,
		"user_id", userID,
	)

	// Redirect back to channel config
	http.Redirect(w, r, fmt.Sprintf("/channels/%d?oauth_success=true", channelID), http.StatusFound)
}
