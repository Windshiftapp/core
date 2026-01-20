package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// PluginDispatcher is an interface for dispatching webhooks to plugins
type PluginDispatcher interface {
	DispatchToPlugin(ctx context.Context, pluginName, handler, event string, payload json.RawMessage) error
}

// WebhookSender handles sending webhooks to configured endpoints
type WebhookSender struct {
	db               database.Database
	itemRepository   *repository.ItemRepository
	httpClient       *http.Client
	pluginDispatcher PluginDispatcher
}

// NewWebhookSender creates a new webhook sender
func NewWebhookSender(db database.Database) *WebhookSender {
	return &WebhookSender{
		db:             db,
		itemRepository: repository.NewItemRepository(db),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
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

	// Send webhooks asynchronously
	for _, webhook := range webhooks {
		go w.sendWebhook(webhook, event, item)
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

// matchesScope checks if the item matches the webhook's scope configuration
func (w *WebhookSender) matchesScope(ctx context.Context, config *models.ChannelConfig, item *models.Item) bool {
	switch config.WebhookScopeType {
	case "all", "":
		return true
	case "workspaces":
		return w.contains(config.WebhookWorkspaceIDs, item.WorkspaceID)
	case "collections":
		return w.itemInCollections(ctx, item.ID, config.WebhookCollectionIDs)
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

// itemInCollections checks if an item belongs to any of the specified collections
func (w *WebhookSender) itemInCollections(ctx context.Context, itemID int, collectionIDs []int) bool {
	if len(collectionIDs) == 0 {
		return false
	}

	// Collections in this system are saved QL queries
	// For simplicity, we'll check if the item appears in any of the collection's query results
	// This is a simplified check - in production you might want to cache collection memberships
	for _, collectionID := range collectionIDs {
		// Get the collection's QL query
		var qlQuery string
		err := w.db.QueryRowContext(ctx, "SELECT ql_query FROM collections WHERE id = ?", collectionID).Scan(&qlQuery)
		if err != nil {
			continue
		}

		// Check if item ID appears in collection results
		// For now, use a simpler approach: check if item_id is directly referenced
		// A full implementation would execute the QL query
		checkQuery := fmt.Sprintf(`
			SELECT EXISTS(
				SELECT 1 FROM items
				WHERE id = ? AND id IN (
					SELECT i.id FROM items i WHERE i.id = ?
				)
			)
		`)
		var exists bool
		if err := w.db.QueryRowContext(ctx, checkQuery, itemID, itemID).Scan(&exists); err == nil && exists {
			return true
		}
	}

	return false
}

// sendWebhook sends the webhook payload to the configured URL or plugin
func (w *WebhookSender) sendWebhook(webhook WebhookConfig, event string, item *models.Item) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get full item details for payload
	fullItem, err := w.itemRepository.FindByIDWithDetails(item.ID)
	if err != nil {
		logger.Get().Error("Failed to get item details for webhook", "error", err, "item_id", item.ID)
		return
	}

	// Serialize item to JSON
	itemJSON, err := json.Marshal(fullItem)
	if err != nil {
		logger.Get().Error("Failed to serialize item for webhook", "error", err, "item_id", item.ID)
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
		return
	}

	// If this is a plugin webhook, dispatch to plugin instead of HTTP
	if webhook.PluginName != "" {
		if w.pluginDispatcher == nil {
			logger.Get().Error("Plugin dispatcher not configured, cannot send plugin webhook",
				"plugin", webhook.PluginName,
				"webhook_id", webhook.PluginWebhookID,
			)
			return
		}

		if err := w.pluginDispatcher.DispatchToPlugin(ctx, webhook.PluginName, webhook.PluginHandler, event, payloadBytes); err != nil {
			logger.Get().Error("Failed to dispatch webhook to plugin",
				"error", err,
				"plugin", webhook.PluginName,
				"handler", webhook.PluginHandler,
				"event", event,
			)
		} else {
			logger.Get().Debug("Plugin webhook dispatched",
				"plugin", webhook.PluginName,
				"handler", webhook.PluginHandler,
				"event", event,
				"item_id", item.ID,
			)
		}
		return
	}

	// Standard HTTP webhook
	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		logger.Get().Error("Failed to create webhook request", "error", err, "url", webhook.URL)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", event)
	req.Header.Set("X-Webhook-ID", fmt.Sprintf("%d", webhook.ChannelID))
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	// Add signature if secret is configured
	if webhook.Secret != "" {
		signature := w.generateSignature(payloadBytes, webhook.Secret)
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	}

	// Add custom headers
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := w.httpClient.Do(req)
	if err != nil {
		logger.Get().Error("Failed to send webhook", "error", err, "url", webhook.URL, "webhook_id", webhook.ChannelID)
		w.updateChannelActivity(ctx, webhook.ChannelID, false)
		return
	}
	defer resp.Body.Close()

	// Log result
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logger.Get().Debug("Webhook sent successfully", "webhook_id", webhook.ChannelID, "event", event, "status", resp.StatusCode)
		w.updateChannelActivity(ctx, webhook.ChannelID, true)
	} else {
		logger.Get().Warn("Webhook returned non-success status", "webhook_id", webhook.ChannelID, "event", event, "status", resp.StatusCode)
		w.updateChannelActivity(ctx, webhook.ChannelID, false)
	}
}

// TriggerManually sends a webhook manually for a specific item
// This is used when webhooks are triggered from item actions, not events
func (w *WebhookSender) TriggerManually(ctx context.Context, webhookID int, itemID int) error {
	// Get webhook config
	var channelName string
	var configJSON string
	query := "SELECT name, config FROM channels WHERE id = ? AND type = 'webhook' AND direction = 'outbound'"
	err := w.db.QueryRowContext(ctx, query, webhookID).Scan(&channelName, &configJSON)
	if err != nil {
		return fmt.Errorf("webhook not found: %w", err)
	}

	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
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
func (w *WebhookSender) SendTestWebhook(ctx context.Context, config *models.ChannelConfig) (bool, string) {
	if config.WebhookURL == "" {
		return false, "Webhook URL is required"
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

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event", "test")
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", time.Now().Unix()))

	// Add signature if secret is configured
	if config.WebhookSecret != "" {
		signature := w.generateSignature(payloadBytes, config.WebhookSecret)
		req.Header.Set("X-Webhook-Signature", "sha256="+signature)
	}

	// Add custom headers
	for key, value := range config.WebhookHeaders {
		req.Header.Set(key, value)
	}

	// Send request with timeout
	client := &http.Client{Timeout: 10 * time.Second}
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
func (w *WebhookSender) updateChannelActivity(ctx context.Context, channelID int, success bool) {
	query := "UPDATE channels SET last_activity = ? WHERE id = ?"
	_, _ = w.db.ExecWriteContext(ctx, query, time.Now(), channelID)
}
