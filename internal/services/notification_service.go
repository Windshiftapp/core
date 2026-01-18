package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// NotificationManager interface for adding notifications
type NotificationManager interface {
	AddNotification(notification models.Notification) error
}

// NotificationEvent represents an event that should trigger notifications
type NotificationEvent struct {
	EventType    string                 // e.g., "item.created", "comment.added"
	WorkspaceID  int                    // Workspace where event occurred
	ActorUserID  int                    // User who triggered the event
	ItemID       int                    // Work item ID (for action URL)
	AssigneeID   *int                   // Current assignee (if applicable)
	CreatorID    *int                   // Item creator (if applicable)
	Title        string                 // Event title
	TemplateData map[string]interface{} // Data for template substitution
}

// RuleCache stores cached notification rules for fast lookup
type RuleCache struct {
	WorkspaceConfigSets map[int]int                            // workspace_id -> config_set_id
	EventRules          map[int][]models.NotificationEventRule // config_set_id -> rules
	Templates           map[string]string                      // template_name -> content
	LastRefreshed       time.Time
}

// NotificationServiceConfig represents configuration for the notification service
type NotificationServiceConfig struct {
	RefreshInterval time.Duration // How often to refresh rules cache
	EventBufferSize int           // Size of event channel buffer
}

// DefaultNotificationServiceConfig returns default configuration
func DefaultNotificationServiceConfig() NotificationServiceConfig {
	return NotificationServiceConfig{
		RefreshInterval: 5 * time.Minute,
		EventBufferSize: 1000,
	}
}

// NotificationService handles asynchronous notification creation
type NotificationService struct {
	db                  database.Database
	notificationManager NotificationManager
	config              NotificationServiceConfig

	// Rule cache
	ruleCache *RuleCache
	cacheMu   sync.RWMutex

	// Event processing
	eventChan chan *NotificationEvent
	stopChan  chan struct{}
	wg        sync.WaitGroup

	// Statistics
	eventsProcessed int64
	cacheHits       int64
	cacheMisses     int64
	errors          int64
}

// NewNotificationService creates a new notification service
func NewNotificationService(db database.Database, notificationManager NotificationManager, config NotificationServiceConfig) *NotificationService {
	service := &NotificationService{
		db:                  db,
		notificationManager: notificationManager,
		config:              config,
		ruleCache: &RuleCache{
			WorkspaceConfigSets: make(map[int]int),
			EventRules:          make(map[int][]models.NotificationEventRule),
			Templates:           make(map[string]string),
			LastRefreshed:       time.Time{},
		},
		eventChan: make(chan *NotificationEvent, config.EventBufferSize),
		stopChan:  make(chan struct{}),
	}

	// Load initial cache
	if err := service.refreshRuleCache(); err != nil {
		slog.Warn("failed to load initial notification rule cache", slog.String("component", "notifications"), slog.Any("error", err))
	}

	// Start background workers
	service.wg.Add(2)
	go service.eventProcessor()
	go service.cacheRefresher()

	slog.Debug("notification service initialized", slog.String("component", "notifications"), slog.Duration("refresh_interval", config.RefreshInterval))

	return service
}

// EmitEvent sends an event to be processed asynchronously (non-blocking)
func (ns *NotificationService) EmitEvent(event *NotificationEvent) {
	slog.Debug("queuing event", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("workspace_id", event.WorkspaceID), slog.Int("actor_user_id", event.ActorUserID), slog.Int("item_id", event.ItemID))

	select {
	case ns.eventChan <- event:
		// Event queued successfully
		slog.Debug("event queued successfully", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("item_id", event.ItemID))
	default:
		// Channel full, log warning but don't block
		slog.Warn("event channel full, dropping event", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("workspace_id", event.WorkspaceID))
		atomic.AddInt64(&ns.errors, 1)
	}
}

// eventProcessor runs in background and processes events from the channel
func (ns *NotificationService) eventProcessor() {
	defer ns.wg.Done()

	for {
		select {
		case event := <-ns.eventChan:
			if err := ns.processEvent(event); err != nil {
				slog.Error("failed to process notification event", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Any("error", err))
				atomic.AddInt64(&ns.errors, 1)
			} else {
				atomic.AddInt64(&ns.eventsProcessed, 1)
			}
		case <-ns.stopChan:
			slog.Debug("stopping notification event processor", slog.String("component", "notifications"))
			// Drain remaining events
			for len(ns.eventChan) > 0 {
				event := <-ns.eventChan
				if err := ns.processEvent(event); err != nil {
					slog.Error("failed to process notification event during shutdown", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Any("error", err))
				}
			}
			return
		}
	}
}

// cacheRefresher runs in background and periodically refreshes the rule cache
func (ns *NotificationService) cacheRefresher() {
	defer ns.wg.Done()

	ticker := time.NewTicker(ns.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := ns.refreshRuleCache(); err != nil {
				slog.Error("failed to refresh notification rule cache", slog.String("component", "notifications"), slog.Any("error", err))
			}
		case <-ns.stopChan:
			slog.Debug("stopping notification cache refresher", slog.String("component", "notifications"))
			return
		}
	}
}

// processEvent processes a single notification event
func (ns *NotificationService) processEvent(event *NotificationEvent) error {
	slog.Debug("processing event", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("workspace_id", event.WorkspaceID), slog.Int("item_id", event.ItemID))

	// Get configuration set for workspace
	configSetID, err := ns.getConfigSetForWorkspace(event.WorkspaceID)
	if err != nil || configSetID == 0 {
		// No configuration set, skip notifications
		slog.Debug("no config set for workspace, skipping notifications", slog.String("component", "notifications"), slog.Int("workspace_id", event.WorkspaceID))
		return nil
	}

	slog.Debug("found config set for workspace", slog.String("component", "notifications"), slog.Int("config_set_id", configSetID), slog.Int("workspace_id", event.WorkspaceID))

	// Get event rules for this config set
	rules := ns.getEventRules(configSetID, event.EventType)
	if len(rules) == 0 {
		// No rules for this event type
		slog.Debug("no rules for event type in config set", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("config_set_id", configSetID))
		return nil
	}

	slog.Debug("found rules for event type", slog.String("component", "notifications"), slog.Int("rule_count", len(rules)), slog.String("event_type", event.EventType))

	// Process each rule
	for _, rule := range rules {
		if !rule.IsEnabled {
			slog.Debug("rule is disabled, skipping", slog.String("component", "notifications"), slog.Int("rule_id", rule.ID))
			continue
		}

		slog.Debug("processing rule for event", slog.String("component", "notifications"), slog.Int("rule_id", rule.ID), slog.String("event_type", event.EventType))

		// Determine recipients
		recipients := ns.determineRecipients(event, &rule)
		if len(recipients) == 0 {
			slog.Debug("no recipients for rule", slog.String("component", "notifications"), slog.Int("rule_id", rule.ID))
			continue
		}

		slog.Debug("determined recipients for rule", slog.String("component", "notifications"), slog.Int("recipient_count", len(recipients)), slog.Int("rule_id", rule.ID), slog.Any("recipients", recipients))

		// Generate notification message
		title, message := ns.generateNotificationMessage(event, &rule)

		// Create notification for each recipient
		for _, userID := range recipients {
			// Skip if recipient is the actor (don't notify yourself)
			if userID == event.ActorUserID {
				slog.Debug("skipping notification for actor", slog.String("component", "notifications"), slog.Int("user_id", userID))
				continue
			}

			notification := models.Notification{
				UserID:    userID,
				Title:     title,
				Message:   message,
				Type:      ns.getNotificationType(event.EventType),
				Timestamp: time.Now(),
				Read:      false,
				ActionURL: fmt.Sprintf("/workspaces/%d/items/%d", event.WorkspaceID, event.ItemID),
			}

			slog.Debug("creating notification for user", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.String("notification_type", notification.Type), slog.String("title", notification.Title))

			// Add to notification manager (BigCache)
			if err := ns.notificationManager.AddNotification(notification); err != nil {
				slog.Error("failed to add notification for user", slog.String("component", "notifications"), slog.Int("user_id", userID), slog.Any("error", err))
				return err
			}

			slog.Debug("successfully created notification for user", slog.String("component", "notifications"), slog.Int("user_id", userID))
		}
	}

	slog.Debug("completed processing event", slog.String("component", "notifications"), slog.String("event_type", event.EventType), slog.Int("item_id", event.ItemID))
	return nil
}

// refreshRuleCache reloads notification rules from database
func (ns *NotificationService) refreshRuleCache() error {
	ns.cacheMu.Lock()
	defer ns.cacheMu.Unlock()

	newCache := &RuleCache{
		WorkspaceConfigSets: make(map[int]int),
		EventRules:          make(map[int][]models.NotificationEventRule),
		Templates:           make(map[string]string),
		LastRefreshed:       time.Now(),
	}

	// Load workspace -> config_set mappings
	rows, err := ns.db.Query(`
		SELECT workspace_id, configuration_set_id
		FROM workspace_configuration_sets
	`)
	if err != nil {
		return fmt.Errorf("failed to load workspace configuration sets: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var workspaceID, configSetID int
		if err := rows.Scan(&workspaceID, &configSetID); err != nil {
			slog.Error("failed to scan workspace config set", slog.String("component", "notifications"), slog.Any("error", err))
			continue
		}
		newCache.WorkspaceConfigSets[workspaceID] = configSetID
	}

	// Load event rules for each config set with notification settings
	ruleRows, err := ns.db.Query(`
		SELECT
			ner.id, ner.notification_setting_id, ner.event_type, ner.is_enabled,
			ner.notify_assignee, ner.notify_creator, ner.notify_watchers, ner.notify_workspace_admins,
			ner.custom_recipients, ner.message_template,
			csns.configuration_set_id
		FROM notification_event_rules ner
		JOIN notification_settings ns ON ns.id = ner.notification_setting_id
		JOIN configuration_set_notification_settings csns ON csns.notification_setting_id = ns.id
		WHERE ns.is_active = true AND ner.is_enabled = true
	`)
	if err != nil {
		return fmt.Errorf("failed to load notification event rules: %w", err)
	}
	defer ruleRows.Close()

	for ruleRows.Next() {
		var rule models.NotificationEventRule
		var configSetID int
		var customRecipients, messageTemplate *string

		if err := ruleRows.Scan(
			&rule.ID, &rule.NotificationSettingID, &rule.EventType, &rule.IsEnabled,
			&rule.NotifyAssignee, &rule.NotifyCreator, &rule.NotifyWatchers, &rule.NotifyWorkspaceAdmins,
			&customRecipients, &messageTemplate,
			&configSetID,
		); err != nil {
			slog.Error("failed to scan notification event rule", slog.String("component", "notifications"), slog.Any("error", err))
			continue
		}

		if customRecipients != nil {
			rule.CustomRecipients = *customRecipients
		}
		if messageTemplate != nil {
			rule.MessageTemplate = *messageTemplate
		}

		newCache.EventRules[configSetID] = append(newCache.EventRules[configSetID], rule)
	}

	// Load notification templates
	templateRows, err := ns.db.Query(`
		SELECT name, content
		FROM notification_templates
		WHERE is_active = true AND template_type = 'notification_type'
	`)
	if err != nil {
		slog.Warn("failed to load notification templates", slog.String("component", "notifications"), slog.Any("error", err))
	} else {
		defer templateRows.Close()
		for templateRows.Next() {
			var name, content string
			if err := templateRows.Scan(&name, &content); err != nil {
				slog.Error("failed to scan template", slog.String("component", "notifications"), slog.Any("error", err))
				continue
			}
			newCache.Templates[name] = content
		}
	}

	// Swap cache
	ns.ruleCache = newCache

	slog.Debug("notification rule cache refreshed", slog.String("component", "notifications"), slog.Int("workspace_count", len(newCache.WorkspaceConfigSets)), slog.Int("config_set_count", len(newCache.EventRules)), slog.Int("template_count", len(newCache.Templates)))

	return nil
}

// getConfigSetForWorkspace retrieves config set ID for a workspace (cached)
func (ns *NotificationService) getConfigSetForWorkspace(workspaceID int) (int, error) {
	ns.cacheMu.RLock()
	defer ns.cacheMu.RUnlock()

	if configSetID, exists := ns.ruleCache.WorkspaceConfigSets[workspaceID]; exists {
		atomic.AddInt64(&ns.cacheHits, 1)
		return configSetID, nil
	}

	atomic.AddInt64(&ns.cacheMisses, 1)
	return 0, nil
}

// getEventRules retrieves event rules for a config set and event type (cached)
func (ns *NotificationService) getEventRules(configSetID int, eventType string) []models.NotificationEventRule {
	ns.cacheMu.RLock()
	defer ns.cacheMu.RUnlock()

	allRules, exists := ns.ruleCache.EventRules[configSetID]
	if !exists {
		return nil
	}

	// Filter rules for this specific event type
	var matchingRules []models.NotificationEventRule
	for _, rule := range allRules {
		if rule.EventType == eventType {
			matchingRules = append(matchingRules, rule)
		}
	}

	return matchingRules
}

// determineRecipients determines who should receive notifications based on rule configuration
func (ns *NotificationService) determineRecipients(event *NotificationEvent, rule *models.NotificationEventRule) []int {
	recipientSet := make(map[int]bool)

	// Add assignee
	if rule.NotifyAssignee && event.AssigneeID != nil && *event.AssigneeID > 0 {
		recipientSet[*event.AssigneeID] = true
	}

	// Add creator
	if rule.NotifyCreator && event.CreatorID != nil && *event.CreatorID > 0 {
		recipientSet[*event.CreatorID] = true
	}

	// Add workspace admins
	if rule.NotifyWorkspaceAdmins {
		adminIDs := ns.getWorkspaceAdmins(event.WorkspaceID)
		for _, adminID := range adminIDs {
			recipientSet[adminID] = true
		}
	}

	// Add custom recipients
	if rule.CustomRecipients != "" {
		var customIDs []int
		if err := json.Unmarshal([]byte(rule.CustomRecipients), &customIDs); err == nil {
			for _, userID := range customIDs {
				recipientSet[userID] = true
			}
		}
	}

	// Add watchers
	if rule.NotifyWatchers {
		watcherIDs := ns.getItemWatchers(event.ItemID)
		for _, watcherID := range watcherIDs {
			recipientSet[watcherID] = true
		}
	}

	// Convert set to slice
	recipients := make([]int, 0, len(recipientSet))
	for userID := range recipientSet {
		recipients = append(recipients, userID)
	}

	return recipients
}

// getWorkspaceAdmins retrieves admin user IDs for a workspace
func (ns *NotificationService) getWorkspaceAdmins(workspaceID int) []int {
	rows, err := ns.db.Query(`
		SELECT DISTINCT user_id
		FROM workspace_members
		WHERE workspace_id = ? AND role IN ('admin', 'owner')
	`, workspaceID)
	if err != nil {
		slog.Error("failed to fetch workspace admins", slog.String("component", "notifications"), slog.Int("workspace_id", workspaceID), slog.Any("error", err))
		return nil
	}
	defer rows.Close()

	var adminIDs []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err == nil {
			adminIDs = append(adminIDs, userID)
		}
	}

	return adminIDs
}

// getItemWatchers retrieves active watcher user IDs for an item
func (ns *NotificationService) getItemWatchers(itemID int) []int {
	rows, err := ns.db.Query(`
		SELECT DISTINCT user_id
		FROM item_watches
		WHERE item_id = ? AND is_active = true
	`, itemID)
	if err != nil {
		slog.Error("failed to fetch item watchers", slog.String("component", "notifications"), slog.Int("item_id", itemID), slog.Any("error", err))
		return nil
	}
	defer rows.Close()

	var watcherIDs []int
	for rows.Next() {
		var userID int
		if err := rows.Scan(&userID); err == nil {
			watcherIDs = append(watcherIDs, userID)
		}
	}

	return watcherIDs
}

// generateNotificationMessage generates title and message for a notification
func (ns *NotificationService) generateNotificationMessage(event *NotificationEvent, rule *models.NotificationEventRule) (string, string) {
	// Use custom template if provided
	if rule.MessageTemplate != "" {
		return ns.applyTemplate(event, rule.MessageTemplate)
	}

	// Check for DB template
	ns.cacheMu.RLock()
	template, hasTemplate := ns.ruleCache.Templates[event.EventType]
	ns.cacheMu.RUnlock()

	if hasTemplate {
		return ns.applyTemplate(event, template)
	}

	// Fall back to default templates
	return ns.getDefaultMessage(event)
}

// applyTemplate applies template variables to a template string
func (ns *NotificationService) applyTemplate(event *NotificationEvent, template string) (string, string) {
	// Simple variable substitution
	message := template
	for key, value := range event.TemplateData {
		placeholder := fmt.Sprintf("{%s}", key)
		message = strings.ReplaceAll(message, placeholder, fmt.Sprintf("%v", value))
	}

	// Extract title from message (first line) or use event title
	lines := strings.SplitN(message, "\n", 2)
	title := event.Title
	if len(lines) > 1 {
		message = lines[1]
	}

	return title, message
}

// getDefaultMessage generates default notification message based on event type
func (ns *NotificationService) getDefaultMessage(event *NotificationEvent) (string, string) {
	title := event.Title

	var message string
	data := event.TemplateData

	// Helper function to get item identifier - prefer key over title
	getItemIdentifier := func() string {
		if itemKey, ok := data["item.key"]; ok && itemKey != nil && itemKey != "" {
			return fmt.Sprintf("%v", itemKey)
		}
		if itemTitle, ok := data["item.title"]; ok && itemTitle != nil {
			return fmt.Sprintf("%v", itemTitle)
		}
		return "Unknown Item"
	}

	switch event.EventType {
	case models.EventItemCreated:
		message = fmt.Sprintf("New work item created: %s", getItemIdentifier())
	case models.EventItemUpdated:
		message = fmt.Sprintf("Work item updated: %s", getItemIdentifier())
	case models.EventItemDeleted:
		message = fmt.Sprintf("Work item deleted: %s", getItemIdentifier())
	case models.EventItemAssigned:
		message = fmt.Sprintf("You have been assigned to: %s", getItemIdentifier())
	case models.EventStatusChanged:
		message = fmt.Sprintf("Status changed to %s for: %s", data["status.name"], getItemIdentifier())
	case models.EventCommentCreated:
		message = fmt.Sprintf("New comment added by %s on: %s", data["user.name"], getItemIdentifier())
	case models.EventCommentUpdated:
		message = fmt.Sprintf("Comment updated by %s on: %s", data["user.name"], getItemIdentifier())
	case models.EventCommentDeleted:
		message = fmt.Sprintf("Comment deleted by %s on: %s", data["user.name"], getItemIdentifier())
	case models.EventItemLinked:
		message = fmt.Sprintf("Work items linked: %s", getItemIdentifier())
	case models.EventItemUnlinked:
		message = fmt.Sprintf("Work item link removed: %s", getItemIdentifier())
	case models.EventMention:
		actorName := "Someone"
		if name, ok := data["actor.name"]; ok && name != nil && name != "" {
			actorName = fmt.Sprintf("%v", name)
		}
		sourceType := "content"
		if st, ok := data["source.type"]; ok && st != nil {
			sourceType = fmt.Sprintf("%v", st)
		}
		message = fmt.Sprintf("%s mentioned you in %s on %s", actorName, sourceType, getItemIdentifier())
	default:
		message = fmt.Sprintf("Event: %s", event.EventType)
	}

	return title, message
}

// getNotificationType maps event types to notification types for UI display
func (ns *NotificationService) getNotificationType(eventType string) string {
	switch eventType {
	case models.EventItemAssigned:
		return "assignment"
	case models.EventCommentCreated, models.EventCommentUpdated, models.EventCommentDeleted:
		return "comment"
	case models.EventStatusChanged:
		return "status_change"
	case models.EventMention:
		return "mention"
	default:
		return "info"
	}
}

// Close gracefully shuts down the notification service
func (ns *NotificationService) Close() error {
	slog.Debug("closing notification service", slog.String("component", "notifications"))

	// Stop background workers
	close(ns.stopChan)
	ns.wg.Wait()

	slog.Debug("notification service closed successfully", slog.String("component", "notifications"))
	return nil
}

// GetStats returns service statistics
func (ns *NotificationService) GetStats() map[string]int64 {
	ns.cacheMu.RLock()
	lastRefreshed := ns.ruleCache.LastRefreshed
	ns.cacheMu.RUnlock()

	return map[string]int64{
		"events_processed":  atomic.LoadInt64(&ns.eventsProcessed),
		"cache_hits":        atomic.LoadInt64(&ns.cacheHits),
		"cache_misses":      atomic.LoadInt64(&ns.cacheMisses),
		"errors":            atomic.LoadInt64(&ns.errors),
		"cache_age_seconds": int64(time.Since(lastRefreshed).Seconds()),
		"pending_events":    int64(len(ns.eventChan)),
	}
}

// ForceRefreshCache manually refreshes the notification rule cache
// This is useful for admins to force cache refresh after configuration changes
func (ns *NotificationService) ForceRefreshCache() error {
	slog.Debug("force refreshing notification rule cache", slog.String("component", "notifications"))
	return ns.refreshRuleCache()
}
