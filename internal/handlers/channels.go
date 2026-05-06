package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"windshift/internal/email"
	"windshift/internal/logger"
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
	channelRepo       *repository.ChannelRepository
	userRepo          *repository.UserRepository
	auditor           *logger.Auditor
	permissionService *services.PermissionService
	webhookSender     *webhook.WebhookSender
	emailScheduler    *scheduler.EmailScheduler
	encryption        email.Encryptor
	baseURL           string
	smtpSender        *windshiftsmtp.NotificationSMTPSender
	service           *services.ChannelService
	credManager       *email.CredentialManager
}

// NewChannelHandler creates a new channel handler
func NewChannelHandler(
	channelRepo *repository.ChannelRepository,
	userRepo *repository.UserRepository,
	channelService *services.ChannelService,
	permissionService *services.PermissionService,
	webhookSender *webhook.WebhookSender,
	auditor *logger.Auditor,
) *ChannelHandler {
	return &ChannelHandler{
		channelRepo:       channelRepo,
		userRepo:          userRepo,
		auditor:           auditor,
		permissionService: permissionService,
		webhookSender:     webhookSender,
		service:           channelService,
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

// SetCredentialManager wires the email credential manager used during the
// channel-level OAuth callback to persist refreshed tokens. Set after
// construction so server.New's wiring sequence stays linear.
func (h *ChannelHandler) SetCredentialManager(cm *email.CredentialManager) {
	h.credManager = cm
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

	respondJSONOK(w, channels)
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
		h.auditor.Log(r, currentUser, logger.ActionChannelCreate, logger.ResourceChannel, &channelID, channel.Name)
	}

	respondJSONCreated(w, channel)
}

// GetChannel returns a specific channel by ID
func (h *ChannelHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
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

	respondJSONOK(w, channel)
}

// UpdateChannel updates an existing channel
func (h *ChannelHandler) UpdateChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	updates, ok := decodeJSON[models.Channel](w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	existing, err := h.service.GetByID(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if existing == nil {
		respondNotFound(w, r, "channel")
		return
	}

	isPluginManaged, err := h.service.IsPluginManaged(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if isPluginManaged {
		respondForbidden(w, r)
		return
	}

	// Preserve config when not supplied — config edits go through
	// UpdateChannelConfig, which merges. GetByID returns a scrubbed config so
	// we read the raw value separately.
	configToWrite := updates.Config
	if configToWrite == "" {
		raw, err := h.service.GetConfig(ctx, id)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		configToWrite = raw
	}

	updated, err := h.service.Update(ctx, id, services.ChannelUpdateRequest{
		Name:        updates.Name,
		Description: updates.Description,
		Status:      existing.Status, // status is changed via ToggleChannel only
		IsDefault:   updates.IsDefault,
		Config:      configToWrite,
		CategoryID:  updates.CategoryID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionChannelUpdate, logger.ResourceChannel, &id, updates.Name)
	}

	respondJSONOK(w, updated)
}

// DeleteChannel deletes a channel
func (h *ChannelHandler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	channel, err := h.service.GetByID(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}
	if channel.IsDefault {
		respondValidationError(w, r, "Cannot delete default channel")
		return
	}
	if channel.PluginName != nil && *channel.PluginName != "" {
		respondForbidden(w, r)
		return
	}

	err = h.service.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "channel")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionChannelDelete, logger.ResourceChannel, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// ToggleChannel toggles a channel's enabled/disabled status
func (h *ChannelHandler) ToggleChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	channel, err := h.service.GetByID(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}

	isPluginManaged, err := h.service.IsPluginManaged(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if isPluginManaged {
		respondForbidden(w, r)
		return
	}

	currentStatus := channel.Status
	newStatus := "enabled"
	if currentStatus == "enabled" {
		newStatus = "disabled"
	}

	if err := h.service.SetStatus(ctx, id, newStatus); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		actionType := logger.ActionChannelActivate
		if newStatus == "disabled" {
			actionType = logger.ActionChannelDeactivate
		}
		h.auditor.LogWithDetails(r, currentUser,
			actionType, logger.ResourceChannel,
			&id, channel.Name,
			map[string]interface{}{
				"old_status": currentStatus,
				"new_status": newStatus,
			},
		)
	}

	updated, err := h.service.GetByID(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, updated)
}

// TestChannel tests a channel configuration by sending a test email
func (h *ChannelHandler) TestChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

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

	got, err := h.service.GetByID(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if got == nil {
		respondNotFound(w, r, "channel")
		return
	}
	// Send-side credentials live in the scrubbed fields, so re-fetch raw.
	rawConfig, err := h.service.GetConfig(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	channel := *got
	channel.Config = rawConfig

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

	respondJSONOK(w, result)
}

// TestChannelConfig tests a channel configuration without saving it
func (h *ChannelHandler) TestChannelConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	var testData struct {
		Config models.ChannelConfig `json:"config"`
	}
	if err = json.NewDecoder(r.Body).Decode(&testData); err != nil {
		respondValidationError(w, r, "Invalid JSON")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // Longer timeout for network operations
	defer cancel()

	channel, err := h.service.GetByID(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}
	channelType := channel.Type

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

	respondJSONOK(w, result)
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

// testSMTPConfig tests SMTP configuration directly. Dial goes through
// utils.SafeNetDialer so a channel manager cannot use this endpoint to
// port-scan loopback / private-IP / link-local services.
func (h *ChannelHandler) testSMTPConfig(config models.ChannelConfig) bool {
	if config.SMTPHost == "" || config.SMTPPort == 0 {
		return false
	}

	addr := net.JoinHostPort(config.SMTPHost, strconv.Itoa(config.SMTPPort))

	conn, err := utils.SafeNetDialer(10*time.Second).Dial("tcp", addr)
	if err != nil {
		logger.Get().Debug("SMTP connection failed", "error", err)
		return false
	}
	defer func() { _ = conn.Close() }() //nolint:gocritic // defer ensures cleanup even on panic

	return true
}

// updateChannelActivity updates the last_activity timestamp for a channel
func (h *ChannelHandler) updateChannelActivity(ctx context.Context, channelID int) {
	_ = h.service.UpdateLastActivity(ctx, channelID)
}

// UpdateChannelConfig updates only the configuration of a channel
func (h *ChannelHandler) UpdateChannelConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

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

	// Encrypt basic-auth secrets in the incoming payload before merging so
	// they land on disk as AES-GCM ciphertext rather than plaintext JSON. We
	// touch only fields the caller actually sent — omitted keys keep their
	// existing (already-encrypted) value through the merge below. Empty
	// strings are passed through so the caller can clear a credential.
	for _, key := range []string{"smtp_password", "imap_password", "webhook_secret"} {
		raw, ok := incomingConfig[key]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok || s == "" {
			continue
		}
		if h.encryption == nil {
			continue
		}
		ciphertext, encErr := h.encryption.Encrypt(s)
		if encErr != nil {
			respondInternalError(w, r, fmt.Errorf("encrypt %s: %w", key, encErr))
			return
		}
		incomingConfig[key] = ciphertext
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	existingConfigJSON, err := h.service.GetConfig(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "channel")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	isPluginManaged, err := h.service.IsPluginManaged(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if isPluginManaged {
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

	// Validate webhook URL at write time as defense-in-depth: the webhook
	// sender already rejects private targets at send time via
	// webhook.ValidateWebhookURL, but failing fast here gives the admin a
	// clear error in the UI instead of a silent send-time skip later.
	if finalConfig.WebhookURL != "" {
		if err := webhook.ValidateWebhookURL(finalConfig.WebhookURL); err != nil {
			respondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Webhook URL must target a public host"))
			return
		}
	}

	if err := h.service.UpdateConfig(ctx, id, string(configJSON)); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Channel configuration updated successfully",
	})
}

// GetChannelManagers returns all managers for a channel
func (h *ChannelHandler) GetChannelManagers(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, err := h.service.Exists(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "channel")
		return
	}

	managers, err := h.service.GetManagers(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, managers)
}

// AddChannelManager adds managers to a channel
func (h *ChannelHandler) AddChannelManager(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	channelID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	request, ok := decodeJSON[models.ChannelManagerRequest](w, r)
	if !ok {
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

	channel, err := h.service.GetByID(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}

	for _, managerID := range request.ManagerIDs {
		err := h.service.AddManager(ctx, channelID, request.ManagerType, managerID, user.ID)
		if err != nil {
			// Foreign-key violation = referenced user/group doesn't exist.
			// fmt.Errorf wraps preserve the driver string, so substring check still works.
			if strings.Contains(err.Error(), "FOREIGN KEY") || strings.Contains(err.Error(), "foreign key") {
				respondValidationError(w, r, fmt.Sprintf("Invalid %s ID: %d does not exist", request.ManagerType, managerID))
				return
			}
			respondInternalError(w, r, err)
			return
		}

		var managerName string
		switch request.ManagerType {
		case "user":
			managerName, _ = h.userRepo.GetFullName(ctx, managerID)
		case "group":
			managerName, _ = h.channelRepo.GetGroupName(ctx, managerID)
		}

		h.auditor.LogWithDetails(r, user,
			logger.ActionChannelAddManager, logger.ResourceChannelManager,
			&channelID, channel.Name,
			map[string]interface{}{
				"manager_type": request.ManagerType,
				"manager_id":   managerID,
				"manager_name": managerName,
			},
		)
	}

	respondJSONCreated(w, map[string]interface{}{
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

	channelID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	managerID, ok := requireIDParam(w, r, "managerId")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	channel, err := h.service.GetByID(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}

	managerType, actualManagerID, err := h.service.LookupManagerRow(ctx, managerID, channelID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "manager")
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var managerName string
	switch managerType {
	case "user":
		managerName, _ = h.userRepo.GetFullName(ctx, actualManagerID)
	case "group":
		managerName, _ = h.channelRepo.GetGroupName(ctx, actualManagerID)
	}

	removed, err := h.service.RemoveManager(ctx, managerID, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !removed {
		respondNotFound(w, r, "manager")
		return
	}

	h.auditor.LogWithDetails(r, user,
		logger.ActionChannelRemoveManager, logger.ResourceChannelManager,
		&channelID, channel.Name,
		map[string]interface{}{
			"manager_type": managerType,
			"manager_id":   actualManagerID,
			"manager_name": managerName,
		},
	)

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	channel, err := h.service.GetByID(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}
	if channel.Type != "email" || channel.Direction != "inbound" {
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

	respondJSONOK(w, map[string]interface{}{
		"success":    true,
		"channel_id": channelID,
		"message":    "Email processing triggered",
	})
}

// GetEmailLog returns the email processing log for a channel
// GET /channels/{id}/email-log?page=1&page_size=50
func (h *ChannelHandler) GetEmailLog(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

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

	channel, err := h.service.GetByID(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return
	}
	if channel.Type != "email" {
		respondValidationError(w, r, "Channel is not an email channel")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get channel state. ErrNotFound just means "fresh channel, no state yet".
	type emailChannelState struct {
		LastCheckedAt *time.Time `json:"last_checked_at"`
		LastUID       int        `json:"last_uid"`
		ErrorCount    int        `json:"error_count"`
		LastError     string     `json:"last_error"`
	}
	var state emailChannelState
	if got, err := h.channelRepo.GetEmailChannelState(ctx, id); err == nil {
		state.LastUID = got.LastUID
		state.LastCheckedAt = got.LastCheckedAt
		state.ErrorCount = got.ErrorCount
		state.LastError = got.LastError
	} else if !errors.Is(err, repository.ErrNotFound) {
		respondInternalError(w, r, err)
		return
	}

	total, err := h.channelRepo.CountEmailMessages(ctx, id, search)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rows, err := h.channelRepo.ListEmailMessages(ctx, id, search, page, pageSize)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

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

	// Collect distinct workspace IDs so we batch the permission checks.
	workspaceIDs := map[int]bool{}
	for _, m := range rows {
		if m.WorkspaceID != nil {
			workspaceIDs[*m.WorkspaceID] = true
		}
	}
	allowedWS := map[int]bool{}
	for wsID := range workspaceIDs {
		allowed, permErr := h.permissionService.HasWorkspacePermission(user.ID, wsID, models.PermissionItemView)
		if permErr != nil {
			respondInternalError(w, r, permErr)
			return
		}
		allowedWS[wsID] = allowed
	}

	messages := make([]emailMessage, 0, len(rows))
	for _, m := range rows {
		msg := emailMessage{
			ID:                  m.ID,
			FromEmail:           m.FromEmail,
			FromName:            m.FromName,
			Subject:             m.Subject,
			ItemID:              m.ItemID,
			CommentID:           m.CommentID,
			ProcessedAt:         m.ProcessedAt,
			WorkspaceKey:        m.WorkspaceKey,
			WorkspaceItemNumber: m.WorkspaceItemNumber,
		}
		// Only surface workspace identifiers when the user can see that workspace.
		if m.WorkspaceID != nil && !allowedWS[*m.WorkspaceID] {
			msg.WorkspaceKey = ""
			msg.WorkspaceItemNumber = 0
		}
		messages = append(messages, msg)
	}

	respondJSONOK(w, map[string]interface{}{
		"state":     state,
		"messages":  messages,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
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
	user, ok := RequireAuth(w, r)
	if !ok {
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

	got, err := h.service.GetByID(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if got == nil {
		respondNotFound(w, r, "channel")
		return
	}
	if got.Type != "email" {
		respondValidationError(w, r, "Channel is not an email channel")
		return
	}
	configJSON, err := h.service.GetConfig(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
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

	// Store state in database (expires in 5 minutes). The repo records this
	// in email_oauth_state with provider_id = 0 to distinguish channel-flow
	// from provider-flow.
	expiresAt := time.Now().Add(5 * time.Minute)
	if err = h.channelRepo.CreateOAuthState(ctx, state, channelID, user.ID, expiresAt); err != nil {
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

	respondJSONOK(w, map[string]string{
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

	// Validate state, get associated channel ID, and delete the state row in one call.
	// providerID and userID are recorded by the start-flow but unused here.
	_, channelID, _, err := h.channelRepo.ConsumeOAuthState(ctx, state)
	if errors.Is(err, repository.ErrNotFound) {
		respondValidationError(w, r, "Invalid or expired state")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	configJSON, err := h.service.GetConfig(ctx, channelID)
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

	// Save tokens to channel config via the injected credential manager.
	err = h.credManager.SaveOAuthTokens(ctx, channelID, tokens, userEmail)
	if err != nil {
		slog.Error("failed to save tokens", "error", err)
		http.Redirect(w, r, "/channels?oauth_error=save_failed", http.StatusFound)
		return
	}

	slog.Info("OAuth completed for email channel (inline credentials)",
		"channel_id", channelID,
		"email", userEmail,
	)

	// Redirect back to channel config
	http.Redirect(w, r, fmt.Sprintf("/channels/%d?oauth_success=true", channelID), http.StatusFound)
}
