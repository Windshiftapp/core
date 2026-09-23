package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
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
	"windshift/internal/sanitize"
	"windshift/internal/scheduler"
	"windshift/internal/services"
	windshiftsmtp "windshift/internal/smtp"
	"windshift/internal/utils"
	"windshift/internal/webhook"
)

const (
	channelRequestBodyMaxBytes = 1 << 20
	channelManagerBatchMax     = 100
)

// decodeChannelRequest bounds channel-management JSON before decoding and
// rejects concatenated values after the first document. Channel configs can
// contain form/portal layout data, so the cap is deliberately roomy while
// still preventing an authenticated manager from forcing unbounded reads.
func decodeChannelRequest(w http.ResponseWriter, r *http.Request, target any, optional bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, channelRequestBodyMaxBytes)
	decoder := newJSONDecoder(w, r)
	if err := decoder.Decode(target); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return true
		}
		if isRequestBodyTooLarge(err) {
			respondRequestTooLarge(w, r)
		} else {
			respondValidationError(w, r, "Invalid JSON")
		}
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if isRequestBodyTooLarge(err) {
			respondRequestTooLarge(w, r)
		} else {
			respondValidationError(w, r, "Invalid JSON")
		}
		return false
	}
	return true
}

func decodeChannelJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var value T
	ok := decodeChannelRequest(w, r, &value, false)
	return value, ok
}

// bareEmailAddress accepts only a mailbox address, not a display-name form.
// SMTP envelope recipients must not carry header syntax or control characters.
func bareEmailAddress(raw string) (string, bool) {
	address := strings.TrimSpace(raw)
	parsed, err := mail.ParseAddress(address)
	if err != nil || parsed.Address == "" || parsed.Address != address {
		return "", false
	}
	return address, true
}

// ChannelHandler handles HTTP requests for channels
type ChannelHandler struct {
	channelRepo       *repository.ChannelRepository
	userRepo          *repository.UserRepository
	auditor           *logger.Auditor
	permissionService *services.PermissionService
	webhookSender     *webhook.WebhookSender
	emailScheduler    *scheduler.EmailScheduler
	emailReplies      *services.EmailReplyService
	encryption        email.Encryptor
	baseURL           string
	smtpSender        *windshiftsmtp.NotificationSMTPSender
	service           *services.ChannelService
	configUpdate      *services.ChannelConfigUpdateService
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
	handler := &ChannelHandler{
		channelRepo:       channelRepo,
		userRepo:          userRepo,
		auditor:           auditor,
		permissionService: permissionService,
		webhookSender:     webhookSender,
		service:           channelService,
		configUpdate:      services.NewChannelConfigUpdateService(channelService, permissionService),
	}
	handler.configUpdate.SetURLValidator(webhook.ValidateWebhookURL)
	handler.configUpdate.SetEmailConfigValidator(email.ValidateConfigForEnable)
	handler.configUpdate.SetSubscriptionInvalidator(handler.invalidateWebhookSubscriptions)
	return handler
}

// SetEncryption sets the encryption service for OAuth credential handling
func (h *ChannelHandler) SetEncryption(enc email.Encryptor) {
	h.encryption = enc
	h.configUpdate.SetSecretEncryptor(func(secret string) (string, error) {
		return email.EncryptSecret(enc, secret)
	})
}

// SetBaseURL sets the base URL for OAuth callbacks
func (h *ChannelHandler) SetBaseURL(baseURL string) {
	h.baseURL = baseURL
}

// SetKnowledgeBasePageValidator wires the page-existence check used when a
// portal wires a page subtree into its knowledge base.
func (h *ChannelHandler) SetKnowledgeBasePageValidator(validate func(workspaceID, pageID int) error) {
	h.configUpdate.SetKnowledgeBasePageValidator(validate)
}

// SetEmailScheduler sets the email scheduler (used to avoid circular dependencies)
func (h *ChannelHandler) SetEmailScheduler(es *scheduler.EmailScheduler) {
	h.emailScheduler = es
}

// SetEmailReplyService wires the outbound customer-reply service so the
// channel surface can list, retry, and discard queued replies.
func (h *ChannelHandler) SetEmailReplyService(rs *services.EmailReplyService) {
	h.emailReplies = rs
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

func (h *ChannelHandler) invalidateWebhookSubscriptions() {
	if h.webhookSender != nil {
		h.webhookSender.InvalidateSubscriptions()
	}
}

// requireChannelManageAccess is defense-in-depth on top of the
// RequireChannelManagement route middleware: writes 401 when unauthenticated,
// 403 when the authenticated user is not a manager, or 500 on lookup error.
// The route middleware already limits this per-record decision to users with
// at least one channel-management assignment. Returns the user on success so
// callers can reuse it for audit logging.
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
		respondForbidden(w, r)
		return nil, false
	}
	return user, true
}

func (h *ChannelHandler) canCompleteEmailOAuth(ctx context.Context, userID, channelID int) (bool, error) {
	canManage, err := h.service.UserCanManage(ctx, userID, channelID)
	if err != nil || !canManage {
		return canManage, err
	}
	channel, err := h.service.GetByID(ctx, channelID)
	if err != nil {
		return false, err
	}
	if channel == nil || channel.Type != "email" || channel.Direction != "inbound" {
		return false, nil
	}
	if !channel.IsDefault {
		return true, nil
	}
	return h.service.UserIsSystemAdmin(userID)
}

// GetChannels returns all channels (admins) or only managed channels (non-admins)
func (h *ChannelHandler) GetChannels(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

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
		CategoryID  *int   `json:"category_id"`
		Slug        string `json:"slug"`
	}
	if !decodeChannelRequest(w, r, &req, false) {
		return
	}
	if req.IsDefault {
		respondValidationError(w, r, "Default channels cannot be created through this endpoint")
		return
	}
	// Sanitize user-facing text; service validation owns enum and config fields.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &req.Description, Policy: sanitize.RichText, Label: "Description"},
	)
	config := map[string]any{}
	req.Slug = strings.TrimSpace(req.Slug)
	if (req.Type == "portal" || req.Type == "form") && req.Slug != "" {
		if !slugFormatOK(req.Slug) {
			respondValidationError(w, r, "slug must be 3-64 chars: lowercase letters, digits, or hyphens (no leading/trailing hyphen)")
			return
		}
		inUse, slugErr := h.channelRepo.SlugInUse(ctx, req.Type, req.Slug, 0)
		if slugErr != nil {
			respondInternalError(w, r, slugErr)
			return
		}
		if inUse {
			respondConflict(w, r, fmt.Sprintf("Slug %q is already in use by another %s channel", req.Slug, req.Type))
			return
		}
		if req.Type == "portal" {
			config["portal_slug"] = req.Slug
			config["portal_title"] = req.Name
		} else {
			config["form_slug"] = req.Slug
		}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if req.CategoryID != nil {
		exists, categoryErr := h.channelRepo.CategoryExists(ctx, *req.CategoryID)
		if categoryErr != nil {
			respondInternalError(w, r, categoryErr)
			return
		}
		if !exists {
			respondValidationError(w, r, "Channel category not found")
			return
		}
	}

	channel, err := h.service.Create(ctx, services.ChannelCreateRequest{
		Name:        req.Name,
		Type:        req.Type,
		Direction:   req.Direction,
		Description: req.Description,
		Status:      req.Status,
		IsDefault:   req.IsDefault,
		Config:      string(configJSON),
		CategoryID:  req.CategoryID,
	})
	if err != nil {
		if errors.Is(err, repository.ErrChannelSlugConflict) {
			respondConflict(w, r, "That public channel slug was claimed by another request; choose a different slug")
			return
		}
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
	h.invalidateWebhookSubscriptions()

	respondJSONCreated(w, struct {
		*models.Channel
		Warnings []string `json:"warnings,omitempty"`
	}{channel, warnings})
}

// GetChannel returns a specific channel by ID
func (h *ChannelHandler) GetChannel(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Match the manager gate used by the collection endpoint.
	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
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

	var updates models.Channel
	if !decodeChannelRequest(w, r, &updates, false) {
		return
	}
	// Sanitize the two user-facing text fields before updating the channel.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &updates.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &updates.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
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

	// Default status changes go through the dedicated atomic workflow.
	if updates.IsDefault != existing.IsDefault {
		respondValidationError(w, r, "Default-channel status cannot be changed through the metadata endpoint")
		return
	}
	if updates.CategoryID != nil {
		exists, categoryErr := h.channelRepo.CategoryExists(ctx, *updates.CategoryID)
		if categoryErr != nil {
			respondInternalError(w, r, categoryErr)
			return
		}
		if !exists {
			respondValidationError(w, r, "Channel category not found")
			return
		}
	}

	updated, err := h.service.Update(ctx, id, services.ChannelUpdateRequest{
		Name:        updates.Name,
		Description: updates.Description,
		Status:      existing.Status, // status is changed via ToggleChannel only
		IsDefault:   updates.IsDefault,
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
	h.invalidateWebhookSubscriptions()

	respondJSONOK(w, struct {
		*models.Channel
		Warnings []string `json:"warnings,omitempty"`
	}{updated, warnings})
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
		if errors.Is(err, repository.ErrDefaultChannel) {
			respondValidationError(w, r, "Cannot delete default channel")
			return
		}
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
	h.invalidateWebhookSubscriptions()

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

	// Enabling validates the stored configuration first so the operator gets
	// a precise error instead of a later scheduler failure, then the status
	// transition is compare-and-swapped against that same configuration so a
	// concurrent edit cannot bypass validation.
	validatedConfig, err := h.configUpdate.PrepareEnable(ctx, user.ID, id)
	if err != nil {
		var configErr *services.ChannelConfigError
		if errors.As(err, &configErr) {
			switch configErr.Kind {
			case services.ChannelConfigInvalid:
				respondValidationError(w, r, configErr.Message)
			case services.ChannelConfigForbidden:
				respondForbidden(w, r)
			case services.ChannelConfigWorkspaceForbidden:
				respondError(w, r, restapi.NewAPIError(http.StatusForbidden, restapi.ErrCodeInsufficientPermission, configErr.Message))
			case services.ChannelConfigConflict:
				respondConflict(w, r, configErr.Message)
			}
			return
		}
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "channel")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if validatedConfig != "" {
		updated, statusErr := h.service.SetStatusIfConfigUnchanged(ctx, id, newStatus, validatedConfig)
		if statusErr != nil {
			respondInternalError(w, r, statusErr)
			return
		}
		if !updated {
			respondConflict(w, r, "Channel configuration changed while it was being enabled; review it and try again")
			return
		}
	} else if err := h.service.SetStatus(ctx, id, newStatus); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.invalidateWebhookSubscriptions()

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		actionType := logger.ActionChannelActivate
		if newStatus == "disabled" {
			actionType = logger.ActionChannelDeactivate
		}
		h.auditor.LogWithDetails(r, currentUser,
			actionType, logger.ResourceChannel,
			&id, channel.Name,
			map[string]any{
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

	var testRequest struct {
		TestEmail string `json:"test_email"`
	}
	if !decodeChannelRequest(w, r, &testRequest, false) {
		return
	}

	if testRequest.TestEmail == "" {
		respondValidationError(w, r, "test_email is required")
		return
	}
	testEmail, valid := bareEmailAddress(testRequest.TestEmail)
	if !valid {
		respondValidationError(w, r, "test_email must be a valid bare email address")
		return
	}
	testRequest.TestEmail = testEmail

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

	result := make(map[string]any)
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
	if !decodeChannelRequest(w, r, &testData, false) {
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
	if channelType == "smtp" && testData.Config.SMTPPassword == "" {
		rawConfig, configErr := h.service.GetConfig(ctx, id)
		if configErr != nil {
			respondInternalError(w, r, configErr)
			return
		}
		var stored models.ChannelConfig
		if configErr = json.Unmarshal([]byte(rawConfig), &stored); configErr != nil {
			respondInternalError(w, r, configErr)
			return
		}
		// Preserve an omitted password for tests just as config updates do. The
		// SMTP sender owns decryption immediately before authentication.
		testData.Config.SMTPPassword = stored.SMTPPassword
	}
	if channelType == "webhook" && testData.Config.WebhookSecret == "" {
		rawConfig, configErr := h.service.GetConfig(ctx, id)
		if configErr != nil {
			respondInternalError(w, r, configErr)
			return
		}
		var stored models.ChannelConfig
		if configErr = json.Unmarshal([]byte(rawConfig), &stored); configErr != nil {
			respondInternalError(w, r, configErr)
			return
		}
		secret, decryptErr := email.DecryptOrLegacy(h.encryption, stored.WebhookSecret)
		if decryptErr != nil {
			respondInternalError(w, r, decryptErr)
			return
		}
		testData.Config.WebhookSecret = secret
	}

	result := make(map[string]any)
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
	htmlBody, textBody := buildSMTPTestEmailBodies(channel.Name, time.Now())

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

func buildSMTPTestEmailBodies(channelName string, testTime time.Time) (htmlBody, textBody string) {
	htmlChannelName := html.EscapeString(channelName)
	htmlBody = `<!DOCTYPE html>
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
			<p><strong>Channel:</strong> ` + htmlChannelName + `</p>
			<p><strong>Test Time:</strong> ` + testTime.Format("January 2, 2006 at 3:04 PM MST") + `</p>
			<p>If you received this email, your SMTP configuration is ready to send notifications.</p>
		</div>
	</div>
</body>
</html>`

	textBody = `Windshift SMTP Test Email

Success! Your SMTP configuration is working correctly.

This test email was sent from Windshift to verify your SMTP settings.

Channel: ` + channelName + `
Test Time: ` + testTime.Format("January 2, 2006 at 3:04 PM MST") + `

If you received this email, your SMTP configuration is ready to send notifications.`

	return htmlBody, textBody
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

// channelSlugRegex accepts routable public portal/form slugs: lowercase 3–64
// alphanumerics with internal hyphens. It evolves independently from collections.
var channelSlugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

// slugFormatOK reports whether s is a valid portal/form slug.
func slugFormatOK(s string) bool {
	return channelSlugRegex.MatchString(s)
}

// UpdateChannelConfig updates only the configuration of a channel
func (h *ChannelHandler) UpdateChannelConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var rawRequest map[string]json.RawMessage
	if !decodeChannelRequest(w, r, &rawRequest, false) {
		return
	}
	rawConfig, ok := rawRequest["config"]
	if !ok {
		respondValidationError(w, r, "Missing config field")
		return
	}

	var incomingConfig map[string]any
	if err := json.Unmarshal(rawConfig, &incomingConfig); err != nil {
		respondValidationError(w, r, "Invalid config JSON")
		return
	}
	if incomingConfig == nil {
		respondValidationError(w, r, "Config must be a JSON object")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	updated, err := h.configUpdate.Update(ctx, user.ID, id, incomingConfig)
	if err != nil {
		var configErr *services.ChannelConfigError
		if errors.As(err, &configErr) {
			switch configErr.Kind {
			case services.ChannelConfigInvalid:
				respondValidationError(w, r, configErr.Message)
			case services.ChannelConfigForbidden:
				respondForbidden(w, r)
			case services.ChannelConfigWorkspaceForbidden:
				respondError(w, r, restapi.NewAPIError(http.StatusForbidden, restapi.ErrCodeInsufficientPermission, configErr.Message))
			case services.ChannelConfigConflict:
				respondConflict(w, r, configErr.Message)
			}
			return
		}
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "channel")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	resourceName := ""
	if channel, lookupErr := h.service.GetByID(ctx, id); lookupErr == nil && channel != nil {
		resourceName = channel.Name
	}
	h.auditor.LogWithDetails(r, user,
		logger.ActionChannelUpdate, logger.ResourceChannel,
		&id, resourceName,
		map[string]any{"change_type": "configuration"},
	)

	respondJSONOK(w, map[string]any{
		"success": updated,
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

	var request models.ChannelManagerRequest
	if !decodeChannelRequest(w, r, &request, false) {
		return
	}

	if request.ManagerType != "user" && request.ManagerType != "group" {
		respondValidationError(w, r, "manager_type must be 'user' or 'group'")
		return
	}
	if len(request.ManagerIDs) == 0 {
		respondValidationError(w, r, "manager_ids must contain at least one ID")
		return
	}
	if len(request.ManagerIDs) > channelManagerBatchMax {
		respondValidationError(w, r, fmt.Sprintf("manager_ids must contain at most %d IDs", channelManagerBatchMax))
		return
	}
	uniqueManagerIDs := make([]int, 0, len(request.ManagerIDs))
	seenManagerIDs := make(map[int]struct{}, len(request.ManagerIDs))
	for _, managerID := range request.ManagerIDs {
		if managerID <= 0 {
			respondValidationError(w, r, "manager_ids must contain positive IDs")
			return
		}
		if _, duplicate := seenManagerIDs[managerID]; duplicate {
			continue
		}
		seenManagerIDs[managerID] = struct{}{}
		uniqueManagerIDs = append(uniqueManagerIDs, managerID)
	}
	request.ManagerIDs = uniqueManagerIDs

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

	managerNames := make(map[int]string, len(request.ManagerIDs))
	for _, managerID := range request.ManagerIDs {
		// channel_managers.manager_id is polymorphic (user or group) and has
		// no FK, so existence has to be enforced in app code. Reject up front
		// rather than relying on the FK-violation string from the driver
		// which differs between SQLite and Postgres.
		var exists bool
		switch request.ManagerType {
		case "user":
			exists, err = h.userRepo.ActiveExists(managerID)
		case "group":
			exists, err = h.channelRepo.GroupExists(ctx, managerID)
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondValidationError(w, r, fmt.Sprintf("Invalid %s ID: %d does not exist or is inactive", request.ManagerType, managerID))
			return
		}

		var managerName string
		switch request.ManagerType {
		case "user":
			managerName, _ = h.userRepo.GetFullName(ctx, managerID)
		case "group":
			managerName, _ = h.channelRepo.GetGroupName(ctx, managerID)
		}
		managerNames[managerID] = managerName
	}

	insertedManagerIDs, err := h.service.AddManagers(ctx, channelID, request.ManagerType, request.ManagerIDs, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	for _, managerID := range insertedManagerIDs {
		h.auditor.LogWithDetails(r, user,
			logger.ActionChannelAddManager, logger.ResourceChannelManager,
			&channelID, channel.Name,
			map[string]any{
				"manager_type": request.ManagerType,
				"manager_id":   managerID,
				"manager_name": managerNames[managerID],
			},
		)
	}

	respondJSONCreated(w, map[string]any{
		"success": true,
		"message": fmt.Sprintf("Added %d manager(s) to channel", len(insertedManagerIDs)),
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
	if errors.Is(err, repository.ErrNotFound) {
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
		map[string]any{
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
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	isSystemAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil || !isSystemAdmin {
		respondAdminRequired(w, r)
		return
	}

	channelIDStr := r.PathValue("id")
	channelID, err := strconv.Atoi(channelIDStr)
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
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

	if h.emailScheduler == nil {
		respondError(w, r, &restapi.APIError{
			StatusCode: http.StatusServiceUnavailable,
			Code:       "SERVICE_UNAVAILABLE",
			Message:    "Email scheduler not available",
		})
		return
	}

	err = h.emailScheduler.ProcessChannelNow(ctx, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]any{
		"success":    true,
		"channel_id": channelID,
		"message":    "Email processing triggered",
	})
}

// RequeueRateLimitedEmails rewinds the channel's IMAP watermark to the
// earliest flood-declined message so the next poll retries it. Rate-limited
// mail was left in the mailbox, so nothing is re-downloaded from the sender:
// recovery only re-reads what is already there. After the tracking row's
// claim goes stale the message is re-evaluated against the current cap.
// POST /channels/{id}/email/requeue-rate-limited
func (h *ChannelHandler) RequeueRateLimitedEmails(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
		return
	}
	if !h.requireInboundEmailChannel(ctx, w, r, id) {
		return
	}

	state, err := h.channelRepo.GetEmailChannelState(ctx, id)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		respondInternalError(w, r, err)
		return
	}
	var uidValidity uint32
	if state != nil {
		uidValidity = state.UIDValidity
	}

	earliestUID, found, err := h.channelRepo.GetEarliestRateLimitedUID(ctx, id, uidValidity)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !found {
		respondJSONOK(w, map[string]any{
			"requeued":   false,
			"channel_id": id,
			"message":    "No rate-limited messages to requeue",
		})
		return
	}

	if err := h.channelRepo.ResetEmailWatermarkToUID(ctx, id, earliestUID-1); err != nil {
		respondInternalError(w, r, err)
		return
	}

	slog.Info("requeued rate-limited email messages",
		"channel_id", id,
		"from_uid", earliestUID,
	)
	respondJSONOK(w, map[string]any{
		"requeued":   true,
		"channel_id": id,
		"from_uid":   earliestUID,
		"message":    "Rate-limited messages requeued for reprocessing",
	})
}

// requireInboundEmailChannel is the shared channel lookup + shape check for
// the per-channel email endpoints.
func (h *ChannelHandler) requireInboundEmailChannel(ctx context.Context, w http.ResponseWriter, r *http.Request, id int) bool {
	channel, err := h.service.GetByID(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if channel == nil {
		respondNotFound(w, r, "channel")
		return false
	}
	if channel.Type != "email" || channel.Direction != "inbound" {
		respondValidationError(w, r, "Channel is not an inbound email channel")
		return false
	}
	return true
}

// ListEmailReplies returns the channel's outbound customer-reply queue:
// envelope metadata and delivery state, never message bodies.
// GET /channels/{id}/email/replies?status=pending&page=1&page_size=50
func (h *ChannelHandler) ListEmailReplies(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	user, ok := h.requireChannelManageAccess(ctx, w, r, id)
	if !ok {
		return
	}
	if !h.requireInboundEmailChannel(ctx, w, r, id) {
		return
	}

	status := repository.EmailReplyOutboxPending
	if s := r.URL.Query().Get("status"); s != "" {
		switch repository.EmailReplyOutboxStatus(s) {
		case repository.EmailReplyOutboxPending, repository.EmailReplyOutboxDelivered,
			repository.EmailReplyOutboxDiscarded, repository.EmailReplyOutboxAll:
			status = repository.EmailReplyOutboxStatus(s)
		default:
			respondValidationError(w, r, "status must be pending, delivered, discarded, or all")
			return
		}
	}

	page, pageSize := 1, 50
	if p := r.URL.Query().Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
			pageSize = v
		}
	}

	isSystemAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	total, err := h.channelRepo.CountEmailReplies(ctx, id, status)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	rows, err := h.channelRepo.ListEmailReplies(ctx, id, status, page, pageSize)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Same workspace-scoped redaction as the inbound log: the recipient and
	// subject stay hidden from channel managers without item-view on the
	// ticket's workspace. Bodies are never returned regardless.
	workspaceIDs := map[int]bool{}
	for _, row := range rows {
		if row.WorkspaceID != nil {
			workspaceIDs[*row.WorkspaceID] = true
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

	type emailReply struct {
		CommentID     int        `json:"comment_id"`
		ItemID        int        `json:"item_id"`
		ToEmail       string     `json:"to_email"`
		ToName        string     `json:"to_name"`
		Subject       string     `json:"subject"`
		AttemptCount  int        `json:"attempt_count"`
		NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
		LastError     string     `json:"last_error,omitempty"`
		DeliveredAt   *time.Time `json:"delivered_at,omitempty"`
		DiscardedAt   *time.Time `json:"discarded_at,omitempty"`
		CreatedAt     time.Time  `json:"created_at"`
		WorkspaceKey  string     `json:"workspace_key,omitempty"`
		Redacted      bool       `json:"redacted,omitempty"`
	}

	replies := make([]emailReply, 0, len(rows))
	for _, row := range rows {
		reply := emailReply{
			CommentID:    row.CommentID,
			ItemID:       row.ItemID,
			ToEmail:      row.ToEmail,
			ToName:       row.ToName,
			Subject:      row.Subject,
			AttemptCount: row.AttemptCount,
			LastError:    row.LastError.String,
			CreatedAt:    row.CreatedAt,
			WorkspaceKey: row.WorkspaceKey,
		}
		if row.NextAttemptAt.Valid {
			t := row.NextAttemptAt.Time
			reply.NextAttemptAt = &t
		}
		if row.DeliveredAt.Valid {
			t := row.DeliveredAt.Time
			reply.DeliveredAt = &t
		}
		if row.DiscardedAt.Valid {
			t := row.DiscardedAt.Time
			reply.DiscardedAt = &t
		}
		if !isSystemAdmin && (row.WorkspaceID == nil || !allowedWS[*row.WorkspaceID]) {
			reply.ToEmail = "[redacted]"
			reply.ToName = ""
			reply.Subject = "[redacted]"
			reply.WorkspaceKey = ""
			reply.Redacted = true
		}
		replies = append(replies, reply)
	}

	respondJSONOK(w, map[string]any{
		"replies":   replies,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// RetryEmailReply attempts immediate delivery of a queued customer reply on
// behalf of an operator, bypassing the backoff schedule once.
// POST /channels/{id}/email/replies/{commentId}/retry
func (h *ChannelHandler) RetryEmailReply(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	commentID, ok := requireIDParam(w, r, "commentId")
	if !ok {
		return
	}
	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
		return
	}
	if !h.requireInboundEmailChannel(ctx, w, r, id) {
		return
	}
	if h.emailReplies == nil {
		respondError(w, r, &restapi.APIError{
			StatusCode: http.StatusServiceUnavailable,
			Code:       "SERVICE_UNAVAILABLE",
			Message:    "Email reply service not available",
		})
		return
	}

	delivered, err := h.emailReplies.RetryPendingReply(id, commentID)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		respondNotFound(w, r, "email reply")
		return
	case errors.Is(err, services.ErrEmailReplyNotRetryable):
		respondError(w, r, &restapi.APIError{
			StatusCode: http.StatusConflict,
			Code:       "EMAIL_REPLY_NOT_RETRYABLE",
			Message:    "Reply is already delivered or discarded",
		})
		return
	case errors.Is(err, services.ErrSMTPNotConfigured):
		respondError(w, r, &restapi.APIError{
			StatusCode: http.StatusConflict,
			Code:       "SMTP_NOT_CONFIGURED",
			Message:    "Outbound SMTP is not configured; configure it first or discard the reply",
		})
		return
	case err != nil:
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]any{
		"delivered":  delivered,
		"comment_id": commentID,
		"message":    "Retry attempted; see the queue for the current state",
	})
}

// DiscardEmailReply marks a queued customer reply as never-send. The row
// stays visible for audit and stops retrying.
// POST /channels/{id}/email/replies/{commentId}/discard
func (h *ChannelHandler) DiscardEmailReply(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	commentID, ok := requireIDParam(w, r, "commentId")
	if !ok {
		return
	}
	if _, ok := h.requireChannelManageAccess(ctx, w, r, id); !ok {
		return
	}
	if !h.requireInboundEmailChannel(ctx, w, r, id) {
		return
	}

	discarded, err := h.channelRepo.DiscardEmailReply(ctx, id, commentID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !discarded {
		respondError(w, r, &restapi.APIError{
			StatusCode: http.StatusConflict,
			Code:       "EMAIL_REPLY_NOT_DISCARDABLE",
			Message:    "Reply is already delivered, discarded, or does not exist",
		})
		return
	}

	slog.Info("discarded queued email reply", "channel_id", id, "comment_id", commentID)
	respondJSONOK(w, map[string]any{
		"discarded":  true,
		"comment_id": commentID,
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
	isAdmin, err := h.permissionService.IsSystemAdmin(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	// Searching raw sender/subject columns would expose a count oracle for
	// workspace-redacted rows. System administrators can search the full audit
	// log; channel managers can browse the same page with protected rows redacted.
	if search != "" && !isAdmin {
		respondForbidden(w, r)
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
		// Healthy is derived: a non-empty last_error means the last poll either
		// failed or dropped a poison message, so the channel needs attention.
		Healthy bool `json:"healthy"`
		// RateLimitedCount is how many messages flood protection declined and
		// are waiting for operator requeue.
		RateLimitedCount int `json:"rate_limited_count"`
	}
	state := emailChannelState{Healthy: true}
	if got, err := h.channelRepo.GetEmailChannelState(ctx, id); err == nil {
		state.LastUID = got.LastUID
		state.LastCheckedAt = got.LastCheckedAt
		state.ErrorCount = got.ErrorCount
		state.LastError = got.LastError
		state.Healthy = got.LastError == ""
	} else if !errors.Is(err, repository.ErrNotFound) {
		respondInternalError(w, r, err)
		return
	}
	rateLimitedCount, err := h.channelRepo.CountRateLimitedEmails(ctx, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	state.RateLimitedCount = rateLimitedCount

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
		ID                  int        `json:"id"`
		FromEmail           string     `json:"from_email"`
		FromName            string     `json:"from_name"`
		Subject             string     `json:"subject"`
		ItemID              *int       `json:"item_id"`
		CommentID           *int       `json:"comment_id"`
		ProcessedAt         time.Time  `json:"processed_at"`
		RateLimitedAt       *time.Time `json:"rate_limited_at,omitempty"`
		WorkspaceKey        string     `json:"workspace_key,omitempty"`
		WorkspaceItemNumber int        `json:"workspace_item_number,omitempty"`
		Redacted            bool       `json:"redacted,omitempty"`
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
			RateLimitedAt:       m.RateLimitedAt,
			WorkspaceKey:        m.WorkspaceKey,
			WorkspaceItemNumber: m.WorkspaceItemNumber,
		}
		// Redact sender/subject PII for rows whose target workspace the channel
		// manager can't view. Channel-management permission alone is not enough
		// to read inbound customer email contents when the resulting item lives
		// in a workspace the manager has no item-view on. Redacted=true lets
		// the UI render a placeholder rather than blanks.
		if !isAdmin && (m.WorkspaceID == nil || !allowedWS[*m.WorkspaceID]) {
			msg.FromEmail = "[redacted]"
			msg.FromName = ""
			msg.Subject = "[redacted]"
			msg.WorkspaceKey = ""
			msg.WorkspaceItemNumber = 0
			msg.Redacted = true
		}
		messages = append(messages, msg)
	}

	respondJSONOK(w, map[string]any{
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
	var startRequest struct {
		RestoreChannelEnabled bool `json:"restore_channel_enabled"`
	}
	if !decodeChannelRequest(w, r, &startRequest, true) {
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

	// Generate a one-time state token bound to the exact config used to build
	// this authorization request. A callback from a stale tab must not attach
	// tokens after another manager changes the channel.
	state, err := newEmailOAuthState(configJSON)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Store state in database (expires in 5 minutes). The repo records this
	// in email_oauth_state with a NULL provider_id to distinguish channel-flow
	// from provider-flow.
	expiresAt := time.Now().Add(5 * time.Minute)
	if err = h.channelRepo.CreateOAuthState(ctx, state, channelID, user.ID, startRequest.RestoreChannelEnabled, expiresAt); err != nil {
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
		if state != "" {
			_, channelID, _, restoreEnabled, consumeErr := h.channelRepo.ConsumeOAuthState(r.Context(), state, false)
			if consumeErr == nil {
				h.restoreEmailChannelAfterOAuth(r.Context(), channelID, restoreEnabled, state, "")
			}
		}
		// URL-encode the error parameter to prevent open redirect attacks
		http.Redirect(w, r, "/admin/channels?oauth_error="+url.QueryEscape(errorParam), http.StatusFound)
		return
	}

	if code == "" || state == "" {
		respondValidationError(w, r, "Missing code or state parameter")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Validate state, get associated channel ID, and delete the state row in one call.
	// providerID is NULL for this flow. The captured user is re-authorized
	// before credentials are changed, so permission revocation takes effect.
	_, channelID, stateUserID, restoreEnabled, err := h.channelRepo.ConsumeOAuthState(ctx, state, false)
	if errors.Is(err, repository.ErrNotFound) {
		respondValidationError(w, r, "Invalid or expired state")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	var savedConfigJSON string
	defer func() {
		h.restoreEmailChannelAfterOAuth(context.Background(), channelID, restoreEnabled, state, savedConfigJSON)
	}()
	canManage, err := h.canCompleteEmailOAuth(ctx, stateUserID, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canManage {
		http.Redirect(w, r, "/admin/channels?oauth_error=authorization_failed", http.StatusFound)
		return
	}

	configJSON, err := h.service.GetConfig(ctx, channelID)
	if err != nil {
		slog.Error("failed to get channel config", "error", err, "channel_id", channelID)
		http.Redirect(w, r, "/admin/channels?oauth_error=channel_not_found", http.StatusFound)
		return
	}
	if !emailOAuthStateMatchesConfig(state, configJSON) {
		http.Redirect(w, r, "/admin/channels?oauth_error=config_changed", http.StatusFound)
		return
	}

	var config models.ChannelConfig
	if configJSON != "" {
		if err = json.Unmarshal([]byte(configJSON), &config); err != nil {
			http.Redirect(w, r, "/admin/channels?oauth_error=invalid_config", http.StatusFound)
			return
		}
	}

	// Decrypt client secret. DecryptOrLegacy returns legacy plaintext rows
	// unchanged so an in-flight migration of email_oauth_client_secret does
	// not break the callback for channels saved before this change.
	clientSecret, err := email.DecryptOrLegacy(h.encryption, config.EmailOAuthClientSecret)
	if err != nil {
		slog.Error("failed to decrypt client secret", "error", err, "channel_id", channelID)
		http.Redirect(w, r, "/admin/channels?oauth_error=decrypt_failed", http.StatusFound)
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
			http.Redirect(w, r, "/admin/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}
		userEmail, err = p.GetUserEmail(ctx, tokens.AccessToken)
		if err != nil || strings.TrimSpace(userEmail) == "" {
			slog.Error("failed to get Microsoft mailbox identity", "error", err)
			http.Redirect(w, r, "/admin/channels?oauth_error=identity_failed", http.StatusFound)
			return
		}

	case "google":
		p := email.NewGoogleProvider(config.EmailOAuthClientID, clientSecret, scopes)
		tokens, err = p.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			slog.Error("failed to exchange code", "error", err)
			http.Redirect(w, r, "/admin/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}
		userEmail, err = p.GetUserEmail(ctx, tokens.AccessToken)
		if err != nil || strings.TrimSpace(userEmail) == "" {
			slog.Error("failed to get Google mailbox identity", "error", err)
			http.Redirect(w, r, "/admin/channels?oauth_error=identity_failed", http.StatusFound)
			return
		}

	default:
		http.Redirect(w, r, "/admin/channels?oauth_error=unsupported_provider", http.StatusFound)
		return
	}

	// Save tokens to channel config via the injected credential manager.
	canManage, err = h.canCompleteEmailOAuth(ctx, stateUserID, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canManage {
		http.Redirect(w, r, "/admin/channels?oauth_error=authorization_failed", http.StatusFound)
		return
	}
	savedConfigJSON, err = h.credManager.SaveOAuthTokens(ctx, channelID, tokens, userEmail, &config)
	if err != nil {
		slog.Error("failed to save tokens", "error", err)
		http.Redirect(w, r, "/admin/channels?oauth_error=save_failed", http.StatusFound)
		return
	}
	slog.Info("OAuth completed for email channel (inline credentials)",
		"channel_id", channelID,
		"email", userEmail,
	)

	// Redirect back to channel config
	// #nosec G710 -- local relative URL built from a server-side int (channelID); no caller-controlled component reaches the destination
	http.Redirect(w, r, fmt.Sprintf("/admin/channels/%d?oauth_success=true", channelID), http.StatusFound)
}

func (h *ChannelHandler) restoreEmailChannelAfterOAuth(ctx context.Context, channelID int, restore bool, state, savedConfigJSON string) {
	if !restore {
		return
	}
	// The request context is commonly canceled as soon as the callback returns.
	// Give restoration a small independent budget, then revalidate the exact
	// config we are enabling. This matters when the OAuth identity was changed:
	// a denied/failed flow has no tokens for the new identity and must stay
	// disabled, while a reconnect that retained valid tokens can safely recover.
	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	channel, err := h.service.GetByID(restoreCtx, channelID)
	if err != nil {
		slog.Error("failed to restore email channel after OAuth", "channel_id", channelID, "error", err)
		return
	}
	configJSON, err := h.service.GetConfig(restoreCtx, channelID)
	if err != nil {
		slog.Error("failed to load email channel config for OAuth restoration", "channel_id", channelID, "error", err)
		return
	}
	if savedConfigJSON == "" {
		if !emailOAuthStateMatchesConfig(state, configJSON) {
			slog.Warn("email channel remains disabled because its config changed during OAuth", "channel_id", channelID)
			return
		}
	} else if configJSON != savedConfigJSON {
		slog.Warn("email channel remains disabled because its config changed after OAuth credentials were saved", "channel_id", channelID)
		return
	}
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		slog.Error("invalid email channel config during OAuth restoration", "channel_id", channelID, "error", err)
		return
	}
	if err := email.ValidateConfigForEnable(channel, &config); err != nil {
		slog.Warn("email channel remains disabled after incomplete OAuth", "channel_id", channelID, "error", err)
		return
	}
	updated, err := h.service.SetStatusIfConfigUnchanged(restoreCtx, channelID, "enabled", configJSON)
	if err != nil {
		slog.Error("failed to restore email channel after OAuth", "channel_id", channelID, "error", err)
	} else if !updated {
		slog.Warn("email channel config changed during OAuth restoration; leaving disabled", "channel_id", channelID)
	}
}
