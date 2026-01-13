package logger

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/services"
)

// AuditEvent represents a security or admin event that should be logged
type AuditEvent struct {
	UserID       int                    // User who performed the action
	Username     string                 // Username for quick reference
	IPAddress    string                 // Client IP address
	UserAgent    string                 // Client user agent
	ActionType   string                 // e.g., "user.create", "permission.grant"
	ResourceType string                 // e.g., "user", "workspace", "permission"
	ResourceID   *int                   // ID of the resource (nullable)
	ResourceName string                 // Human-readable resource name
	Details      map[string]interface{} // Additional details (old_value, new_value, etc.)
	Success      bool                   // Whether the operation succeeded
	ErrorMessage string                 // Error message if failed
	Timestamp    time.Time              // When the event occurred (set automatically if zero)
}

// auditLogEntry is the internal representation ready for database insert
type auditLogEntry struct {
	Timestamp    time.Time
	UserID       int
	Username     string
	IPAddress    string
	UserAgent    string
	ActionType   string
	ResourceType string
	ResourceID   *int
	ResourceName string
	DetailsJSON  *string
	Success      bool
	ErrorMessage string
}

// Global audit batcher (nil when using PostgreSQL or not initialized)
var (
	auditBatcher     *services.WriteBatcher[auditLogEntry]
	auditBatcherDB   database.Database
	auditBatcherOnce sync.Once
	auditBatcherMu   sync.RWMutex
)

// InitAuditBatcher initializes the audit log batcher for SQLite.
// Call this at application startup. For PostgreSQL, do not call this function.
func InitAuditBatcher(db database.Database) {
	auditBatcherMu.Lock()
	defer auditBatcherMu.Unlock()

	auditBatcherDB = db
	config := services.DefaultWriteBatcherConfig("audit_logs")
	auditBatcher = services.NewWriteBatcher(config, flushAuditLogs)
	auditBatcher.Start()

	slog.Info("audit log batcher initialized")
}

// StopAuditBatcher stops the batcher and flushes remaining entries.
// Call this during graceful shutdown.
func StopAuditBatcher() {
	auditBatcherMu.Lock()
	defer auditBatcherMu.Unlock()

	if auditBatcher != nil {
		auditBatcher.Stop()
		auditBatcher = nil
		slog.Info("audit log batcher stopped")
	}
}

// flushAuditLogs performs a batch INSERT of audit log entries
func flushAuditLogs(entries []auditLogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	auditBatcherMu.RLock()
	db := auditBatcherDB
	auditBatcherMu.RUnlock()

	if db == nil {
		return fmt.Errorf("audit batcher database not initialized")
	}

	// Build multi-row INSERT statement
	var sb strings.Builder
	sb.WriteString(`INSERT INTO audit_logs (
		timestamp, user_id, username, ip_address, user_agent,
		action_type, resource_type, resource_id, resource_name,
		details, success, error_message
	) VALUES `)

	args := make([]interface{}, 0, len(entries)*12)
	for i, entry := range entries {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			entry.Timestamp,
			entry.UserID,
			entry.Username,
			entry.IPAddress,
			entry.UserAgent,
			entry.ActionType,
			entry.ResourceType,
			entry.ResourceID,
			entry.ResourceName,
			entry.DetailsJSON,
			entry.Success,
			entry.ErrorMessage,
		)
	}

	_, err := db.ExecWrite(sb.String(), args...)
	if err != nil {
		slog.Error("failed to flush audit logs",
			"error", err,
			"count", len(entries),
		)
		return err
	}

	slog.Debug("flushed audit logs", "count", len(entries))
	return nil
}

// LogAudit logs an audit event to the database.
// If the batcher is initialized (SQLite), events are batched for efficiency.
// Otherwise, events are written immediately (PostgreSQL or fallback).
func LogAudit(db database.Database, event AuditEvent) error {
	// Convert details map to JSON
	var detailsJSON *string
	if event.Details != nil && len(event.Details) > 0 {
		detailsBytes, err := json.Marshal(event.Details)
		if err != nil {
			slog.Warn("failed to marshal audit details", "error", err)
		} else {
			detailsStr := string(detailsBytes)
			detailsJSON = &detailsStr
		}
	}

	// Set timestamp
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	// Also log to structured logger for real-time monitoring (always immediate)
	slog.Info("audit_event",
		"user_id", event.UserID,
		"username", event.Username,
		"action_type", event.ActionType,
		"resource_type", event.ResourceType,
		"resource_id", event.ResourceID,
		"resource_name", event.ResourceName,
		"success", event.Success,
	)

	// Check if batcher is available (SQLite mode)
	auditBatcherMu.RLock()
	batcher := auditBatcher
	auditBatcherMu.RUnlock()

	if batcher != nil {
		// Use batcher for efficient batched writes
		entry := auditLogEntry{
			Timestamp:    timestamp,
			UserID:       event.UserID,
			Username:     event.Username,
			IPAddress:    event.IPAddress,
			UserAgent:    event.UserAgent,
			ActionType:   event.ActionType,
			ResourceType: event.ResourceType,
			ResourceID:   event.ResourceID,
			ResourceName: event.ResourceName,
			DetailsJSON:  detailsJSON,
			Success:      event.Success,
			ErrorMessage: event.ErrorMessage,
		}
		batcher.Add(entry)

		slog.Debug("audit event queued for batch write",
			"action_type", event.ActionType,
			"resource_type", event.ResourceType,
			"user_id", event.UserID,
		)
		return nil
	}

	// Fallback: immediate write (PostgreSQL or batcher not initialized)
	query := `
		INSERT INTO audit_logs (
			timestamp, user_id, username, ip_address, user_agent,
			action_type, resource_type, resource_id, resource_name,
			details, success, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.ExecWrite(
		query,
		timestamp,
		event.UserID,
		event.Username,
		event.IPAddress,
		event.UserAgent,
		event.ActionType,
		event.ResourceType,
		event.ResourceID,
		event.ResourceName,
		detailsJSON,
		event.Success,
		event.ErrorMessage,
	)

	if err != nil {
		slog.Error("failed to log audit event",
			"error", err,
			"action_type", event.ActionType,
			"resource_type", event.ResourceType,
			"user_id", event.UserID,
			"resource_id", event.ResourceID,
		)
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	slog.Debug("audit event logged successfully",
		"action_type", event.ActionType,
		"resource_type", event.ResourceType,
		"user_id", event.UserID,
	)

	return nil
}

// Action type constants for common operations
const (
	// User management
	ActionUserCreate        = "user.create"
	ActionUserUpdate        = "user.update"
	ActionUserDelete        = "user.delete"
	ActionUserPasswordReset = "user.password_reset"
	ActionUserActivate      = "user.activate"
	ActionUserDeactivate    = "user.deactivate"

	// Authentication
	ActionLoginSuccess  = "login.success"
	ActionLoginFailure  = "login.failure"
	ActionLogout        = "logout"
	ActionPasswordChange = "password.change"

	// Permission management
	ActionPermissionGrant  = "permission.grant"
	ActionPermissionRevoke = "permission.revoke"

	// Role management
	ActionRoleAssign = "role.assign"
	ActionRoleRevoke = "role.revoke"

	// Workspace management
	ActionWorkspaceCreate = "workspace.create"
	ActionWorkspaceUpdate = "workspace.update"
	ActionWorkspaceDelete = "workspace.delete"

	// Group management
	ActionGroupCreate      = "group.create"
	ActionGroupUpdate      = "group.update"
	ActionGroupDelete      = "group.delete"
	ActionGroupAddMember   = "group.add_member"
	ActionGroupRemoveMember = "group.remove_member"

	// Configuration management
	ActionConfigSetCreate = "config_set.create"
	ActionConfigSetUpdate = "config_set.update"
	ActionConfigSetDelete = "config_set.delete"

	// Workflow management
	ActionWorkflowCreate = "workflow.create"
	ActionWorkflowUpdate = "workflow.update"
	ActionWorkflowDelete = "workflow.delete"

	// Status management
	ActionStatusCategoryCreate = "status_category.create"
	ActionStatusCategoryUpdate = "status_category.update"
	ActionStatusCategoryDelete = "status_category.delete"
	ActionStatusCreate         = "status.create"
	ActionStatusUpdate         = "status.update"
	ActionStatusDelete         = "status.delete"

	// Custom field management
	ActionCustomFieldCreate = "custom_field.create"
	ActionCustomFieldUpdate = "custom_field.update"
	ActionCustomFieldDelete = "custom_field.delete"

	// Item type management
	ActionItemTypeCreate = "item_type.create"
	ActionItemTypeUpdate = "item_type.update"
	ActionItemTypeDelete = "item_type.delete"

	// Screen management
	ActionScreenCreate = "screen.create"
	ActionScreenUpdate = "screen.update"
	ActionScreenDelete = "screen.delete"

	// Theme management
	ActionThemeCreate   = "theme.create"
	ActionThemeUpdate   = "theme.update"
	ActionThemeDelete   = "theme.delete"
	ActionThemeActivate = "theme.activate"

	// Module settings
	ActionModuleEnable  = "module.enable"
	ActionModuleDisable = "module.disable"

	// API Token management
	ActionAPITokenCreate = "api_token.create"
	ActionAPITokenRevoke = "api_token.revoke"

	// Hierarchy level management
	ActionHierarchyLevelCreate = "hierarchy_level.create"
	ActionHierarchyLevelUpdate = "hierarchy_level.update"
	ActionHierarchyLevelDelete = "hierarchy_level.delete"

	// Link type management
	ActionLinkTypeCreate = "link_type.create"
	ActionLinkTypeUpdate = "link_type.update"
	ActionLinkTypeDelete = "link_type.delete"

	// Permission set management
	ActionPermissionSetCreate = "permission_set.create"
	ActionPermissionSetUpdate = "permission_set.update"
	ActionPermissionSetDelete = "permission_set.delete"

	// Notification template management
	ActionNotificationTemplateCreate = "notification_template.create"
	ActionNotificationTemplateUpdate = "notification_template.update"
	ActionNotificationTemplateDelete = "notification_template.delete"

	// Channel management
	ActionChannelCreate       = "channel.create"
	ActionChannelUpdate       = "channel.update"
	ActionChannelDelete       = "channel.delete"
	ActionChannelActivate     = "channel.activate"
	ActionChannelDeactivate   = "channel.deactivate"
	ActionChannelAddManager   = "channel.add_manager"
	ActionChannelRemoveManager = "channel.remove_manager"

	// Attachment settings management
	ActionAttachmentSettingsUpdate = "attachment_settings.update"

	// Time project management
	ActionTimeProjectCreate = "time_project.create"
	ActionTimeProjectUpdate = "time_project.update"
	ActionTimeProjectDelete = "time_project.delete"

	// Milestone management
	ActionMilestoneCreate = "milestone.create"
	ActionMilestoneUpdate = "milestone.update"
	ActionMilestoneDelete = "milestone.delete"

	// Milestone category management
	ActionMilestoneCategoryCreate = "milestone_category.create"
	ActionMilestoneCategoryUpdate = "milestone_category.update"
	ActionMilestoneCategoryDelete = "milestone_category.delete"

	// Project management
	ActionProjectCreate = "project.create"
	ActionProjectUpdate = "project.update"
	ActionProjectDelete = "project.delete"

	// Collection management
	ActionCollectionCreate = "collection.create"
	ActionCollectionUpdate = "collection.update"
	ActionCollectionDelete = "collection.delete"

	// Personal label management
	ActionPersonalLabelCreate = "personal_label.create"
	ActionPersonalLabelUpdate = "personal_label.update"
	ActionPersonalLabelDelete = "personal_label.delete"

	// Test case management
	ActionTestCaseCreate = "test_case.create"
	ActionTestCaseUpdate = "test_case.update"
	ActionTestCaseDelete = "test_case.delete"

	// Test run management
	ActionTestRunCreate = "test_run.create"
	ActionTestRunUpdate = "test_run.update"
	ActionTestRunDelete = "test_run.delete"

	// Test set management
	ActionTestSetCreate = "test_set.create"
	ActionTestSetUpdate = "test_set.update"
	ActionTestSetDelete = "test_set.delete"
)

// Resource type constants
const (
	ResourceUser                 = "user"
	ResourceWorkspace            = "workspace"
	ResourcePermission           = "permission"
	ResourceRole                 = "role"
	ResourceGroup                = "group"
	ResourceConfigurationSet     = "configuration_set"
	ResourceWorkflow             = "workflow"
	ResourceStatusCategory       = "status_category"
	ResourceStatus               = "status"
	ResourceCustomField          = "custom_field"
	ResourceItemType             = "item_type"
	ResourceScreen               = "screen"
	ResourceTheme                = "theme"
	ResourceModule               = "module"
	ResourceAPIToken             = "api_token"
	ResourceHierarchyLevel       = "hierarchy_level"
	ResourceLinkType             = "link_type"
	ResourcePermissionSet        = "permission_set"
	ResourceNotificationTemplate = "notification_template"
	ResourceChannel              = "channel"
	ResourceChannelManager       = "channel_manager"
	ResourceAttachmentSettings   = "attachment_settings"
	ResourceTimeProject          = "time_project"
	ResourceMilestone            = "milestone"
	ResourceMilestoneCategory    = "milestone_category"
	ResourceProject              = "project"
	ResourceCollection           = "collection"
	ResourcePersonalLabel        = "personal_label"
	ResourceTestCase             = "test_case"
	ResourceTestRun              = "test_run"
	ResourceTestSet              = "test_set"
)
