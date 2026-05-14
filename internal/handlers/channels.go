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
	"regexp"
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

// requireChannelManageAccess is defense-in-depth on top of the
// RequireChannelManagement route middleware: writes 401 when unauthenticated,
// 404 when the user is not a manager (matches the existence-hiding rule from
// CheckItemPermission in base.go), or 500 on lookup error. Returns the user on
// success so callers can reuse it for audit logging.
func (h *ChannelHandler) requireChannelManageAccess(ctx context.Context, w http.ResponseWriter, r *http.Request, channelID int) (*models.User, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, false
	}
	canManage, err := h.service.UserCanManage(ctx, user.ID, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	if !canManage {
		respondNotFound(w, r, "channel")
		return nil, false
	}
	return user, true
}

// GetChannels returns all channels (admins) or only managed channels (non-admins)
func (h *ChannelHandler) GetChannels(w http.ResponseWriter, r *http.Request) {
	// Get current user
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Parse query-string filters. Unknown values are dropped silently — the
	// service layer rejects them via its own validation if it cares.
	q := r.URL.Query()
	categoryFilter := q.Get("category_id")

	var filters services.ChannelListFilters
	if categoryFilter != "" {
		if categoryFilter == "null" {
			val := -1
			filters.CategoryID = &val
		} else if catID, err := strconv.Atoi(categoryFilter); err == nil {
			filters.CategoryID = &catID
		}
	}
	filters.Type = q.Get("type")
	filters.Direction = q.Get("direction")
	filters.Status = q.Get("status")
	if q.Get("include_disabled") == "true" {
		filters.IncludeDisabled = true
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
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
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
		if errors.Is(err, services.ErrInvalidChannelField) ||
			err.Error() == "name, type, and direction are required" {
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

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Gate by manager scope so direct GET /channels/{id} matches the
	// visibility filter already applied by GET /channels. See bughunt2.md
	// Run 6 finding #4.
	canManage, err := h.service.UserCanManage(ctx, user.ID, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canManage {
		respondNotFound(w, r, "channel")
		return
	}

	channel, err := h.service.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user, ok := h.requireChannelManageAccess(ctx, w, r, id)
	if !ok {
		return
	}

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

	// Default-channel status is an admin-only attribute. A non-admin channel
	// manager who could flip is_default would (a) lock themselves out (the
	// manage-default middleware refuses non-admins) and (b) put product
	// semantics tied to "default channel" under non-admin control.
	isAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !isAdmin && updates.IsDefault != existing.IsDefault {
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
		if errors.Is(err, services.ErrInvalidChannelField) {
			respondValidationError(w, r, err.Error())
			return
		}
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
		return
	}

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

// GetChannelDeleteImpact reports row counts for the cascading-or-orphaning
// tables tied to a channel, so the UI's delete-confirmation dialog can warn
// the operator before the cascade fires. Channel-manager gated.
func (h *ChannelHandler) GetChannelDeleteImpact(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
		return
	}

	impact, err := h.channelRepo.GetDeleteImpact(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, impact)
}

// ToggleChannel toggles a channel's enabled/disabled status
func (h *ChannelHandler) ToggleChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
		return
	}

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

	// Block enabling an inbound email channel that's missing required fields.
	// The scheduler would otherwise spin and increment error state on every
	// tick; we'd rather give the operator a precise 400 here.
	if newStatus == "enabled" && channel.Type == "email" && channel.Direction == "inbound" {
		rawConfig, cfgErr := h.service.GetConfig(ctx, id)
		if cfgErr != nil {
			respondInternalError(w, r, cfgErr)
			return
		}
		var cfg models.ChannelConfig
		if rawConfig != "" {
			if jsonErr := json.Unmarshal([]byte(rawConfig), &cfg); jsonErr != nil {
				respondInternalError(w, r, jsonErr)
				return
			}
		}
		if vErr := email.ValidateConfigForEnable(channel, &cfg); vErr != nil {
			respondValidationError(w, r, vErr.Error())
			return
		}
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

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second) // Longer timeout for network operations
	defer cancel()

	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
		return
	}

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

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second) // Longer timeout for network operations
	defer cancel()

	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
		return
	}

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

// channelSlugRegex matches the shape the frontend enforces for portal/form
// slugs: lowercase alphanumerics with optional internal hyphens, 3-64 chars,
// no leading/trailing hyphen. (collections.go has its own copy with the
// same rules — kept separate so the channel-slug constraint can evolve
// without affecting collection slugs.) Slugs land in public URLs, so
// anything outside this set would either fail to route or invite escaping
// bugs in the routing layer.
var channelSlugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// slugFormatOK reports whether s is a valid portal/form slug.
func slugFormatOK(s string) bool {
	return channelSlugRegex.MatchString(s)
}

// emailOAuthAuthFields are config keys only meaningful when EmailAuthMethod
// == "oauth". They include the OAuth app credentials, the tenant ID, and the
// tokens persisted after a successful OAuth flow.
var emailOAuthAuthFields = []string{
	"email_oauth_provider_type",
	"email_oauth_client_id",
	"email_oauth_client_secret",
	"email_oauth_tenant_id",
	"email_oauth_access_token",
	"email_oauth_refresh_token",
	"email_oauth_expires_at",
	"email_oauth_email",
}

// emailBasicAuthFields are config keys only meaningful when EmailAuthMethod
// == "basic". IMAP host/port/encryption come from the generic provider config
// for basic auth; for OAuth providers they're set by the provider itself.
var emailBasicAuthFields = []string{
	"imap_host",
	"imap_port",
	"imap_username",
	"imap_password",
	"imap_encryption",
}

// normalizeEmailAuthConfig strips fields belonging to the inactive auth mode
// from a merged email channel config map. No-op when email_auth_method is
// absent (the caller may be updating non-email-auth fields) or when the
// channel isn't using email auth at all.
func normalizeEmailAuthConfig(cfg map[string]interface{}) {
	rawMethod, ok := cfg["email_auth_method"]
	if !ok {
		return
	}
	method, ok := rawMethod.(string)
	if !ok {
		return
	}
	switch strings.ToLower(method) {
	case "basic":
		for _, k := range emailOAuthAuthFields {
			delete(cfg, k)
		}
	case "oauth":
		for _, k := range emailBasicAuthFields {
			delete(cfg, k)
		}
	}
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
	// email_oauth_client_secret is included so the start-OAuth path can
	// decrypt it consistently with EmailOAuthAccessToken / EmailOAuthRefreshToken.
	for _, key := range []string{"smtp_password", "imap_password", "webhook_secret", "email_oauth_client_secret"} {
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
		return
	}

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

	// For email channels, drop fields from the inactive auth mode so a basic →
	// OAuth switch can't keep the IMAP password around (and vice versa). Stale
	// inline OAuth credentials are especially dangerous: CredentialManager.
	// GetProviderForChannel checks them before falling back to IMAPHost, so a
	// leftover client_id/client_secret can pin a "switched to basic" channel
	// onto OAuth.
	normalizeEmailAuthConfig(mergedConfig)

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

	// Form redirect URL ends up at window.location.href in the form
	// submitter's browser. Reject non-http(s) schemes (javascript:, data:,
	// vbscript:) at write time so an admin can't XSS form visitors.
	if finalConfig.FormRedirectURL != "" {
		if err := utils.ValidateClientRedirectURL(finalConfig.FormRedirectURL); err != nil {
			respondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Form redirect URL must be an http(s) URL"))
			return
		}
	}
	// Same for the form logo, which is rendered as <img src> on the public
	// form page — javascript: in src would be ignored by modern browsers but
	// data: HTML is still a click-through phishing surface.
	if finalConfig.FormLogoURL != "" {
		if err := utils.ValidateClientRedirectURL(finalConfig.FormLogoURL); err != nil {
			respondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Form logo URL must be an http(s) URL"))
			return
		}
	}

	// Portal/form target workspace IDs must reference real, non-personal
	// workspaces. The UI filters them, but a direct API caller can submit
	// bad IDs that would later surface as runtime failures in SubmitForm
	// or GetPortal. Personal workspaces are off-limits for public ingest
	// since they're a single-user scratch space.
	if len(finalConfig.PortalWorkspaceIDs) > 0 || len(finalConfig.FormWorkspaceIDs) > 0 {
		all := append([]int(nil), finalConfig.PortalWorkspaceIDs...)
		all = append(all, finalConfig.FormWorkspaceIDs...)
		bad, wsErr := h.channelRepo.FindBadWorkspaceIDs(all)
		if wsErr != nil {
			respondInternalError(w, r, wsErr)
			return
		}
		if len(bad) > 0 {
			respondValidationError(w, r, fmt.Sprintf("Workspace IDs %v are missing or personal and cannot be used as portal/form targets", bad))
			return
		}
	}

	// Slug format + uniqueness for portal/form channels. Slugs are routed
	// via findChannelBySlug, which scans enabled channels and returns the
	// first match by creation order. Without server-side validation a
	// channel manager could pick a duplicate slug and hijack traffic from
	// an older portal/form. Format regex matches the frontend rules.
	if channel, chErr := h.service.GetByID(ctx, id); chErr == nil && channel != nil {
		var (
			slug      string
			slugLabel string
		)
		switch channel.Type {
		case "portal":
			slug = finalConfig.PortalSlug
			slugLabel = "portal_slug"
		case "form":
			slug = finalConfig.FormSlug
			slugLabel = "form_slug"
		}
		if slug != "" {
			if !slugFormatOK(slug) {
				respondValidationError(w, r, fmt.Sprintf("%s must be 1-64 chars: lowercase letters, digits, or hyphens (no leading/trailing hyphen)", slugLabel))
				return
			}
			inUse, slugErr := h.channelRepo.SlugInUse(ctx, channel.Type, slug, id)
			if slugErr != nil {
				respondInternalError(w, r, slugErr)
				return
			}
			if inUse {
				respondError(w, r, restapi.NewAPIError(http.StatusConflict, restapi.ErrCodeValidationFailed, fmt.Sprintf("%s %q is already in use by another %s channel", slugLabel, slug, channel.Type)))
				return
			}
		}
	}

	// If the channel is already enabled and this is an inbound email channel,
	// the merged config must still satisfy the ingestion-readiness rules.
	// Without this gate, a partial config update (e.g. clearing the OAuth
	// refresh token) would silently leave the channel enabled but broken.
	if channel, chErr := h.service.GetByID(ctx, id); chErr == nil && channel != nil &&
		channel.Status == "enabled" && channel.Type == "email" && channel.Direction == "inbound" {
		if vErr := email.ValidateConfigForEnable(channel, &finalConfig); vErr != nil {
			respondValidationError(w, r, vErr.Error())
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

// GetChannelManagers returns all managers for a channel. Gated by manage
// scope (404-on-deny) because the manager list contains user PII (names,
// emails); any authenticated user could otherwise enumerate channels by ID
// and read the manager directory.
func (h *ChannelHandler) GetChannelManagers(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if _, ok := h.requireChannelManageAccess(ctx, w, r, channelID); !ok {
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user, ok := h.requireChannelManageAccess(ctx, w, r, channelID)
	if !ok {
		return
	}

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
		// channel_managers.manager_id is polymorphic (user or group) and has
		// no FK, so existence has to be enforced in app code. Reject up front
		// rather than relying on the FK-violation string from the driver
		// which differs between SQLite and Postgres.
		var exists bool
		switch request.ManagerType {
		case "user":
			exists, err = h.userRepo.Exists(managerID)
		case "group":
			exists, err = h.channelRepo.GroupExists(ctx, managerID)
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondValidationError(w, r, fmt.Sprintf("Invalid %s ID: %d does not exist", request.ManagerType, managerID))
			return
		}

		err := h.service.AddManager(ctx, channelID, request.ManagerType, managerID, user.ID)
		if err != nil {
			// Belt-and-braces: catch a deferred FK violation from any
			// future schema change that adds a real foreign key.
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
	channelID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	managerID, ok := requireIDParam(w, r, "managerId")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user, ok := h.requireChannelManageAccess(ctx, w, r, channelID)
	if !ok {
		return
	}

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
	if errors.Is(err, sql.ErrNoRows) {
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

	actorIsAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	removed, err := h.service.RemoveManager(ctx, managerID, channelID, actorIsAdmin)
	if err != nil {
		if errors.Is(err, services.ErrLastManager) {
			respondValidationError(w, r, "Cannot remove the last channel manager. Add another manager first or have an admin perform this action.")
			return
		}
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user, ok := h.requireChannelManageAccess(ctx, w, r, id)
	if !ok {
		return
	}

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
		Redacted            bool      `json:"redacted,omitempty"`
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
		// Redact sender/subject PII for rows whose target workspace the channel
		// manager can't view. Channel-management permission alone is not enough
		// to read inbound customer email contents when the resulting item lives
		// in a workspace the manager has no item-view on. Redacted=true lets
		// the UI render a placeholder rather than blanks.
		if m.WorkspaceID != nil && !allowedWS[*m.WorkspaceID] {
			msg.FromEmail = "[redacted]"
			msg.FromName = ""
			msg.Subject = "[redacted]"
			msg.WorkspaceKey = ""
			msg.WorkspaceItemNumber = 0
			msg.Redacted = true
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
	// Get channel ID
	channelIDStr := r.PathValue("id")
	channelID, err := strconv.Atoi(channelIDStr)
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user, ok := h.requireChannelManageAccess(ctx, w, r, channelID)
	if !ok {
		return
	}

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

	// Decrypt client secret — DecryptOrLegacy handles legacy plaintext rows
	// saved before email_oauth_client_secret was added to the encrypt set.
	clientSecret, err := email.DecryptOrLegacy(h.encryption, config.EmailOAuthClientSecret)
	if err != nil {
		respondInternalError(w, r, err)
		return
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

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
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

	// Decrypt client secret. DecryptOrLegacy returns legacy plaintext rows
	// unchanged so an in-flight migration of email_oauth_client_secret does
	// not break the callback for channels saved before this change.
	clientSecret, err := email.DecryptOrLegacy(h.encryption, config.EmailOAuthClientSecret)
	if err != nil {
		slog.Error("failed to decrypt client secret", "error", err, "channel_id", channelID)
		http.Redirect(w, r, "/channels?oauth_error=decrypt_failed", http.StatusFound)
		return
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
	// #nosec G710 -- local relative URL built from a server-side int (channelID); no caller-controlled component reaches the destination
	http.Redirect(w, r, fmt.Sprintf("/channels/%d?oauth_success=true", channelID), http.StatusFound)
}
