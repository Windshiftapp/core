package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// WebhookDispatcher handles dispatching webhook events to plugins
type WebhookDispatcher struct {
	manager *Manager
	db      database.Database
}

// NewWebhookDispatcher creates a new webhook dispatcher
func NewWebhookDispatcher(manager *Manager, db database.Database) *WebhookDispatcher {
	return &WebhookDispatcher{
		manager: manager,
		db:      db,
	}
}

// DispatchToPlugin sends a webhook event to a plugin's handler function
func (d *WebhookDispatcher) DispatchToPlugin(ctx context.Context, pluginName, handler, event string, payload json.RawMessage) error {
	// Build webhook request payload
	webhookRequest := struct {
		Event   string          `json:"event"`
		Payload json.RawMessage `json:"payload"`
	}{
		Event:   event,
		Payload: payload,
	}

	// Use CallPluginFunction which handles plugin context setup for host functions
	_, err := d.manager.CallPluginFunction(pluginName, handler, webhookRequest)
	return err
}

// RegisterPluginWebhooks creates channel entries for all webhooks declared in a plugin's manifest
func (m *Manager) RegisterPluginWebhooks(ctx context.Context, db database.Database, plugin *LoadedPlugin) error {
	if len(plugin.Manifest.Webhooks) == 0 {
		return nil
	}

	for _, webhook := range plugin.Manifest.Webhooks {
		// Check if this webhook already exists
		var existingID int
		err := db.QueryRowContext(ctx,
			"SELECT id FROM channels WHERE plugin_name = ? AND plugin_webhook_id = ?",
			plugin.Manifest.Name, webhook.ID,
		).Scan(&existingID)

		if err == nil {
			// Already exists, skip
			m.logger.Debug("plugin webhook already registered",
				"plugin", plugin.Manifest.Name,
				"webhook_id", webhook.ID,
			)
			continue
		}

		// Create channel entry for this plugin webhook
		config := models.ChannelConfig{
			WebhookSubscribedEvents: webhook.Events,
			WebhookAutoTrigger:      true,
			WebhookScopeType:        "all",
			WebhookPluginHandler:    webhook.Handler,
		}
		configJSON, _ := json.Marshal(config)

		now := time.Now()
		_, err = db.ExecWriteContext(ctx, `
			INSERT INTO channels (name, type, direction, description, status, is_default, config, plugin_name, plugin_webhook_id, created_at, updated_at)
			VALUES (?, 'webhook', 'outbound', ?, 'enabled', false, ?, ?, ?, ?, ?)
		`,
			fmt.Sprintf("%s: %s", plugin.Manifest.Name, webhook.ID),
			fmt.Sprintf("Plugin webhook for %s (handler: %s)", plugin.Manifest.Name, webhook.Handler),
			string(configJSON),
			plugin.Manifest.Name,
			webhook.ID,
			now, now,
		)
		if err != nil {
			m.logger.Error("failed to register plugin webhook",
				"plugin", plugin.Manifest.Name,
				"webhook_id", webhook.ID,
				"error", err,
			)
			continue
		}

		m.logger.Info("registered plugin webhook",
			"plugin", plugin.Manifest.Name,
			"webhook_id", webhook.ID,
			"events", webhook.Events,
		)
	}

	return nil
}

// UnregisterPluginWebhooks removes all channel entries for a plugin's webhooks
func (m *Manager) UnregisterPluginWebhooks(ctx context.Context, db database.Database, pluginName string) error {
	result, err := db.ExecWriteContext(ctx,
		"DELETE FROM channels WHERE plugin_name = ?",
		pluginName,
	)
	if err != nil {
		return fmt.Errorf("failed to delete plugin webhooks: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		m.logger.Info("unregistered plugin webhooks",
			"plugin", pluginName,
			"count", rowsAffected,
		)
	}

	return nil
}

// GetPluginWebhookHandler returns the handler function name for a plugin webhook
func GetPluginWebhookHandler(ctx context.Context, db database.Database, pluginName, webhookID string) (string, error) {
	var configJSON string
	err := db.QueryRowContext(ctx,
		"SELECT config FROM channels WHERE plugin_name = ? AND plugin_webhook_id = ?",
		pluginName, webhookID,
	).Scan(&configJSON)
	if err != nil {
		return "", fmt.Errorf("webhook not found: %w", err)
	}

	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return "", fmt.Errorf("failed to parse webhook config: %w", err)
	}

	if config.WebhookPluginHandler == "" {
		return "", fmt.Errorf("no handler defined for webhook")
	}

	return config.WebhookPluginHandler, nil
}
