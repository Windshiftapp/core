// Package webhook provides webhook delivery and management functionality.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/utils"
)

// newSSRFSafeWebhookClient returns an http.Client whose transport refuses
// to dial loopback / RFC1918 / link-local / CGNAT addresses. Used for both
// the long-lived production webhook client and the per-test client so the
// validate-then-dial gap (DNS rebinding between ValidateWebhookURL and the
// actual HTTP request) is closed at the dialer.
func newSSRFSafeWebhookClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: utils.SafeNetDialer(timeout).DialContext,
		},
	}
}

// PluginDispatcher is an interface for dispatching webhooks to plugins
type PluginDispatcher interface {
	DispatchToPlugin(ctx context.Context, pluginName, handler, event string, payload json.RawMessage) error
}

// dispatchConcurrency caps the number of simultaneous outbound webhook
// deliveries per WebhookSender. Without a cap, a burst of item events with
// many configured webhooks could spawn an unbounded number of goroutines and
// outbound connections. 16 matches a typical hosted-process budget and is
// well below the default SSRF-safe client's per-host connection limit.
const dispatchConcurrency = 16

// WebhookSender handles sending webhooks to configured endpoints
type WebhookSender struct {
	db               database.Database
	itemRepository   *repository.ItemRepository
	deliveryRepo     *repository.WebhookDeliveryRepository
	httpClient       *http.Client
	pluginDispatcher PluginDispatcher
	// dispatchSem caps concurrent sendWebhook goroutines. See dispatchConcurrency.
	dispatchSem chan struct{}
}

// NewWebhookSender creates a new webhook sender
func NewWebhookSender(db database.Database) *WebhookSender {
	return &WebhookSender{
		db:             db,
		itemRepository: repository.NewItemRepository(db),
		deliveryRepo:   repository.NewWebhookDeliveryRepository(db),
		httpClient:     newSSRFSafeWebhookClient(30 * time.Second),
		dispatchSem:    make(chan struct{}, dispatchConcurrency),
	}
}

// recordDelivery persists a delivery row. Failures here are logged but never
// propagated — recording must not block actual webhook dispatch.
func (w *WebhookSender) recordDelivery(ctx context.Context, d *models.WebhookDelivery) {
	if err := w.deliveryRepo.Insert(ctx, d); err != nil {
		logger.Get().Warn("Failed to record webhook delivery", "error", err, "channel_id", d.ChannelID)
	}
}

// attemptTypeFor returns "manual" for the literal "manual" event (TriggerManually
// passes that value), "automatic" otherwise.
func attemptTypeFor(event string) string {
	if event == "manual" {
		return "manual"
	}
	return "automatic"
}

// SetPluginDispatcher sets the plugin dispatcher for handling plugin webhooks
func (w *WebhookSender) SetPluginDispatcher(dispatcher PluginDispatcher) {
	w.pluginDispatcher = dispatcher
}

// WebhookPayload represents the payload sent to webhook endpoints
type WebhookPayload struct {
	Event     string          `json:"event"`
	Timestamp time.Time       `json:"timestamp"`
	WebhookID int             `json:"webhook_id"`
	Item      json.RawMessage `json:"item"`
}

// WebhookConfig represents a webhook configuration from the channels table
type WebhookConfig struct {
	ChannelID        int
	Name             string
	URL              string
	Secret           string
	Headers          map[string]string
	ScopeType        string // "all", "workspaces", "collections"
	WorkspaceIDs     []int
	CollectionIDs    []int
	AutoTrigger      bool
	SubscribedEvents []string
	// Plugin webhook fields
	PluginName      string // Non-empty if this is a plugin webhook
	PluginWebhookID string // Plugin's webhook identifier
	PluginHandler   string // Plugin's handler function name
}

// DispatchEvent sends webhook for an event if matching webhooks exist
// This is called from item/comment handlers when events occur
func (w *WebhookSender) DispatchEvent(event string, item *models.Item) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Get all webhooks that should fire for this event
	webhooks, err := w.GetMatchingWebhooks(ctx, event, item)
	if err != nil {
		logger.Get().Error("Failed to get matching webhooks", "error", err, "event", event, "item_id", item.ID)
		return
	}

	// Send webhooks asynchronously, capped at dispatchConcurrency in flight.
	// The semaphore is per-WebhookSender so independent processes don't
	// share state, but within one process a burst of events can't fan out
	// without bound.
	for _, webhook := range webhooks {
		w.dispatchSem <- struct{}{}
		go func(wh WebhookConfig) {
			defer func() { <-w.dispatchSem }()
			w.sendWebhook(wh, event, item)
		}(webhook)
	}
}

// GetMatchingWebhooks returns all webhooks that should fire for this event and item
func (w *WebhookSender) GetMatchingWebhooks(ctx context.Context, event string, item *models.Item) ([]WebhookConfig, error) {
	// Query all active webhook channels, including plugin webhooks
	query := `
		SELECT id, name, config, plugin_name, plugin_webhook_id
		FROM channels
		WHERE type = 'webhook' AND direction = 'outbound' AND status = 'enabled'
	`

	rows, err := w.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query webhooks: %w", err)
	}
	defer rows.Close()

	var matchingWebhooks []WebhookConfig
	for rows.Next() {
		var channelID int
		var channelName string
		var configJSON string
		var pluginName, pluginWebhookID *string

		if err := rows.Scan(&channelID, &channelName, &configJSON, &pluginName, &pluginWebhookID); err != nil {
			logger.Get().Error("Failed to scan webhook channel", "error", err)
			continue
		}

		// Parse config
		var config models.ChannelConfig
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			logger.Get().Error("Failed to parse webhook config", "error", err, "channel_id", channelID)
			continue
		}

		// Skip if auto trigger is disabled
		if !config.WebhookAutoTrigger {
			continue
		}

		// Skip if event is not subscribed
		if !w.isEventSubscribed(event, config.WebhookSubscribedEvents) {
			continue
		}

		// Check scope matching
		if !w.matchesScope(ctx, &config, item) {
			continue
		}

		// Build webhook config
		webhook := WebhookConfig{
			ChannelID:        channelID,
			Name:             channelName,
			URL:              config.WebhookURL,
			Secret:           config.WebhookSecret,
			Headers:          config.WebhookHeaders,
			ScopeType:        config.WebhookScopeType,
			WorkspaceIDs:     config.WebhookWorkspaceIDs,
			CollectionIDs:    config.WebhookCollectionIDs,
			AutoTrigger:      config.WebhookAutoTrigger,
			SubscribedEvents: config.WebhookSubscribedEvents,
			PluginHandler:    config.WebhookPluginHandler,
		}

		// Set plugin fields if this is a plugin webhook
		if pluginName != nil && *pluginName != "" {
			webhook.PluginName = *pluginName
		}
		if pluginWebhookID != nil && *pluginWebhookID != "" {
			webhook.PluginWebhookID = *pluginWebhookID
		}

		matchingWebhooks = append(matchingWebhooks, webhook)
	}

	return matchingWebhooks, nil
}

// isEventSubscribed checks if the event is in the subscribed events list
func (w *WebhookSender) isEventSubscribed(event string, subscribedEvents []string) bool {
	for _, e := range subscribedEvents {
		if e == event {
			return true
		}
	}
	return false
}

// matchesScope checks if the item matches the webhook's scope configuration.
// "collections" scope is intentionally treated as never-matching: the prior
// implementation read the collection's QL query but didn't execute it, so it
// effectively returned true for any item. Disabling the scope until a proper
// QL evaluator is wired is the safer default — better to under-deliver than
// to fire webhooks for unintended items.
// TODO(channel-bughunt #11): wire collection QL evaluation and restore.
func (w *WebhookSender) matchesScope(ctx context.Context, config *models.ChannelConfig, item *models.Item) bool {
	_ = ctx
	switch config.WebhookScopeType {
	case "all", "":
		return true
	case "workspaces":
		return w.contains(config.WebhookWorkspaceIDs, item.WorkspaceID)
	case "collections":
		slog.Warn("webhook collection scope is not yet evaluated; treating as no-match",
			"item_id", item.ID,
			"collection_ids", config.WebhookCollectionIDs,
		)
		return false
	}
	return false
}

// contains checks if a slice contains a value
func (w *WebhookSender) contains(slice []int, value int) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}

// ValidateWebhookURL checks that a webhook URL is safe to call (not targeting internal networks).
// Exported so that admin endpoints (e.g. handlers.UpdateChannelConfig) can
// reject SSRF-shaped URLs at write time, rather than only at send time.
func ValidateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https scheme, got %q", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL must have a host")
	}

	// Reject localhost and common loopback names
	lower := net.ParseIP(host)
	if lower == nil {
		// It's a hostname, resolve it
		if host == "localhost" || host == "ip6-localhost" || host == "ip6-loopback" {
			return fmt.Errorf("webhook URL must not target localhost")
		}
		ips, err := net.LookupHost(host)
		if err != nil {
			return fmt.Errorf("cannot resolve webhook host %q: %w", host, err)
		}
		for _, ipStr := range ips {
			if ip := net.ParseIP(ipStr); ip != nil && isPrivateIP(ip) {
				return fmt.Errorf("webhook URL %q resolves to private IP %s", host, ipStr)
			}
		}
	} else if isPrivateIP(lower) {
		return fmt.Errorf("webhook URL must not target private IP %s", host)
	}

	return nil
}

// isPrivateIP returns true for loopback, private, and link-local addresses.
func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// sendWebhook sends the webhook payload to the configured URL or plugin.
//
// Records one delivery row for every attempt — success, non-2xx response, hard
// error, or plugin dispatch — so the admin Diagnostics page can surface health
// per channel. Recording errors are logged but do not block dispatch.
func (w *WebhookSender) sendWebhook(webhook WebhookConfig, event string, item *models.Item) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	delivery := &models.WebhookDelivery{
		ChannelID:   webhook.ChannelID,
		ItemID:      &item.ID,
		EventType:   event,
		AttemptType: attemptTypeFor(event),
		Transport:   "http",
		RequestedAt: time.Now().UTC(),
	}

	// Get full item details for payload
	fullItem, err := w.itemRepository.FindByIDWithDetails(item.ID)
	if err != nil {
		logger.Get().Error("Failed to get item details for webhook", "error", err, "item_id", item.ID)
		delivery.ErrorMessage = "failed to load item: " + err.Error()
		w.recordDelivery(ctx, delivery)
		return
	}

	// Serialize item using REST API v1 DTO for consistent payload structure
	itemResponse := dto.MapItemToResponse(fullItem, "")
	itemJSON, err := json.Marshal(itemResponse)
	if err != nil {
		logger.Get().Error("Failed to serialize item for webhook", "error", err, "item_id", item.ID)
		delivery.ErrorMessage = "failed to serialize item: " + err.Error()
		w.recordDelivery(ctx, delivery)
		return
	}

	// Build payload
	payload := WebhookPayload{
		Event:     event,
		Timestamp: time.Now().UTC(),
		WebhookID: webhook.ChannelID,
		Item:      itemJSON,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Get().Error("Failed to serialize webhook payload", "error", err)
		delivery.ErrorMessage = "failed to serialize payload: " + err.Error()
		w.recordDelivery(ctx, delivery)
		return
	}

	// If this is a plugin webhook, dispatch to plugin instead of HTTP
	if webhook.PluginName != "" {
		delivery.Transport = "plugin"
		delivery.RequestURL = "" // not meaningful for plugin transport

		if w.pluginDispatcher == nil {
			logger.Get().Error("Plugin dispatcher not configured, cannot send plugin webhook",
				"plugin", webhook.PluginName,
				"webhook_id", webhook.PluginWebhookID,
			)
			delivery.ErrorMessage = "plugin dispatcher not configured"
			w.recordDelivery(ctx, delivery)
			return
		}

		pluginStart := time.Now()
		if err = w.pluginDispatcher.DispatchToPlugin(ctx, webhook.PluginName, webhook.PluginHandler, event, payloadBytes); err != nil {
			logger.Get().Error("Failed to dispatch webhook to plugin",
				"error", err,
				"plugin", webhook.PluginName,
				"handler", webhook.PluginHandler,
				"event", event,
			)
			delivery.ErrorMessage = err.Error()
		} else {
			logger.Get().Debug("Plugin webhook dispatched",
				"plugin", webhook.PluginName,
				"handler", webhook.PluginHandler,
				"event", event,
				"item_id", item.ID,
			)
			delivery.Success = true
		}
		ms := int(time.Since(pluginStart).Milliseconds())
		delivery.ResponseTimeMs = &ms
		w.recordDelivery(ctx, delivery)
		return
	}

	// Standard HTTP webhook
	delivery.RequestURL = webhook.URL

	// Validate URL to prevent SSRF
	if err := ValidateWebhookURL(webhook.URL); err != nil {
		logger.Get().Error("Webhook URL validation failed", "error", err, "url", webhook.URL, "webhook_id", webhook.ChannelID)
		delivery.ErrorMessage = "URL validation failed: " + err.Error()
		w.recordDelivery(ctx, delivery)
		return
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		logger.Get().Error("Failed to create webhook request", "error", err, "url", webhook.URL)
		delivery.ErrorMessage = "failed to create request: " + err.Error()
		w.recordDelivery(ctx, delivery)
		return
	}

	// Apply custom headers FIRST so reserved Windshift headers (Content-Type,
	// X-Webhook-*) overwrite any collision below. Previously the order was
	// reversed and a channel manager could supply an X-Webhook-Signature
	// custom header that overrode the computed HMAC.
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// Set reserved headers — these take precedence over custom headers.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", event)
	req.Header.Set("X-Webhook-ID", fmt.Sprintf("%d", webhook.ChannelID))
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	// Add signature if secret is configured
	if webhook.Secret != "" {
		signature := w.generateSignature(payloadBytes, webhook.Secret)
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	}

	// Send request
	httpStart := time.Now()
	resp, err := w.httpClient.Do(req)
	elapsedMs := int(time.Since(httpStart).Milliseconds())
	delivery.ResponseTimeMs = &elapsedMs

	if err != nil {
		logger.Get().Error("Failed to send webhook", "error", err, "url", webhook.URL, "webhook_id", webhook.ChannelID)
		w.updateChannelActivity(ctx, webhook.ChannelID, false)
		delivery.ErrorMessage = err.Error()
		w.recordDelivery(ctx, delivery)
		return
	}
	defer resp.Body.Close()

	delivery.ResponseStatusCode = &resp.StatusCode

	// Log result
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logger.Get().Debug("Webhook sent successfully", "webhook_id", webhook.ChannelID, "event", event, "status", resp.StatusCode)
		w.updateChannelActivity(ctx, webhook.ChannelID, true)
		delivery.Success = true
	} else {
		logger.Get().Warn("Webhook returned non-success status", "webhook_id", webhook.ChannelID, "event", event, "status", resp.StatusCode)
		w.updateChannelActivity(ctx, webhook.ChannelID, false)
		delivery.ErrorMessage = fmt.Sprintf("non-2xx status: %d", resp.StatusCode)
		// Capture a bounded slice of the response body so operators can see
		// the receiver's error verbatim (e.g. "invalid signature", "rate
		// limited") instead of just the status code. LimitReader caps memory
		// at 4 KiB even when a misbehaving receiver streams gigabytes.
		preview, perr := io.ReadAll(io.LimitReader(resp.Body, webhookResponsePreviewBytes))
		if perr == nil && len(preview) > 0 {
			delivery.ResponsePreview = string(preview)
		}
	}
	w.recordDelivery(ctx, delivery)
}

// webhookResponsePreviewBytes caps how much of a non-2xx response body we
// store on each delivery row. 4 KiB is enough for typical error messages and
// stays well clear of TEXT/BLOB column overhead on both backends.
const webhookResponsePreviewBytes = 4 * 1024

// TriggerManually sends a webhook manually for a specific item
// This is used when webhooks are triggered from item actions, not events
// ErrWebhookDisabled is returned by TriggerManually when the target webhook
// channel exists but is currently in 'disabled' status. Callers map this to
// a 400 so the operator sees the precise reason instead of "not found".
var ErrWebhookDisabled = fmt.Errorf("webhook is disabled")

func (w *WebhookSender) TriggerManually(ctx context.Context, webhookID, itemID int) error {
	// Get webhook config. Status is loaded so disabled webhooks fail loudly
	// instead of silently delivering; GetMatchingWebhooks already filters
	// the automatic path on status, but the manual path bypassed it before.
	var (
		channelName string
		status      string
		configJSON  string
	)
	query := "SELECT name, status, config FROM channels WHERE id = ? AND type = 'webhook' AND direction = 'outbound'"
	err := w.db.QueryRowContext(ctx, query, webhookID).Scan(&channelName, &status, &configJSON)
	if err != nil {
		return fmt.Errorf("webhook not found: %w", err)
	}
	if status != "enabled" {
		return ErrWebhookDisabled
	}

	var config models.ChannelConfig
	if err = json.Unmarshal([]byte(configJSON), &config); err != nil {
		return fmt.Errorf("failed to parse webhook config: %w", err)
	}

	// Get item
	item, err := w.itemRepository.FindByIDWithDetails(itemID)
	if err != nil {
		return fmt.Errorf("item not found: %w", err)
	}

	// Check scope matching
	if !w.matchesScope(ctx, &config, item) {
		return fmt.Errorf("item does not match webhook scope")
	}

	// Build webhook config
	webhook := WebhookConfig{
		ChannelID: webhookID,
		Name:      channelName,
		URL:       config.WebhookURL,
		Secret:    config.WebhookSecret,
		Headers:   config.WebhookHeaders,
	}

	// Send synchronously for manual triggers
	w.sendWebhook(webhook, "manual", item)

	return nil
}

// SendTestWebhook sends a test webhook to verify configuration
func (w *WebhookSender) SendTestWebhook(ctx context.Context, config *models.ChannelConfig) (success bool, message string) {
	if config.WebhookURL == "" {
		return false, "Webhook URL is required"
	}

	// Validate URL to prevent SSRF
	if err := ValidateWebhookURL(config.WebhookURL); err != nil {
		return false, fmt.Sprintf("Invalid webhook URL: %v", err)
	}

	// Create test payload
	testPayload := map[string]interface{}{
		"event":     "test",
		"timestamp": time.Now().UTC(),
		"message":   "This is a test webhook from Windshift",
		"item": map[string]interface{}{
			"id":    0,
			"title": "Test Item",
			"workspace": map[string]interface{}{
				"id":   0,
				"name": "Test Workspace",
				"key":  "TEST",
			},
		},
	}

	payloadBytes, err := json.Marshal(testPayload)
	if err != nil {
		return false, "Failed to create test payload"
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", config.WebhookURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return false, fmt.Sprintf("Failed to create request: %v", err)
	}

	// Apply custom headers FIRST so reserved Windshift headers overwrite any
	// collision (especially X-Webhook-Signature). See sendWebhook for rationale.
	for key, value := range config.WebhookHeaders {
		req.Header.Set(key, value)
	}

	// Set reserved headers — these take precedence over custom headers.
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", "test")
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	// Add signature if secret is configured
	if config.WebhookSecret != "" {
		signature := w.generateSignature(payloadBytes, config.WebhookSecret)
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	}

	// Send request through an SSRF-safe client (validate-then-dial gap closed
	// at the dialer; URL form already validated by ValidateWebhookURL above).
	client := newSSRFSafeWebhookClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("Failed to send webhook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, fmt.Sprintf("Test webhook sent successfully (status: %d)", resp.StatusCode)
	}

	return false, fmt.Sprintf("Webhook returned non-success status: %d", resp.StatusCode)
}

// generateSignature creates HMAC-SHA256 signature for webhook payload
func (w *WebhookSender) generateSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return hex.EncodeToString(h.Sum(nil))
}

// updateChannelActivity updates the last_activity timestamp for a channel
func (w *WebhookSender) updateChannelActivity(ctx context.Context, channelID int, _ bool) {
	query := "UPDATE channels SET last_activity = ? WHERE id = ?"
	_, _ = w.db.ExecWriteContext(ctx, query, time.Now(), channelID)
}
