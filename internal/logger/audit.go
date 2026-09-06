package logger

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// Auditor records HTTP-driven audit events. Handlers depend on this type
// instead of database.Database so they can stop importing the database
// package just to call the audit log helper.
type Auditor struct {
	db database.Database
}

// NewAuditor returns an Auditor that persists events through the given DB.
func NewAuditor(db database.Database) *Auditor {
	return &Auditor{db: db}
}

// Log records a successful resource action. IP/User-Agent are extracted
// from r; Success is implicitly true (use LogWithDetails for richer events
// or future failure paths).
func (a *Auditor) Log(r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName string) {
	_ = LogAudit(a.db, NewRequestAuditEvent(r, user, actionType, resourceType, resourceID, resourceName, nil))
}

// LogEvent persists a fully-constructed audit event. It exists for callers
// (e.g. SCIM, which authenticates via token rather than a user session) that
// need to control every field of the event instead of deriving the actor from
// an authenticated *models.User.
func (a *Auditor) LogEvent(event AuditEvent) {
	_ = LogAudit(a.db, event)
}

// LogWithDetails records a successful resource action with extra structured
// details (serialized to JSON in the audit log row).
func (a *Auditor) LogWithDetails(r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName string, details map[string]any) {
	_ = LogAudit(a.db, NewRequestAuditEvent(r, user, actionType, resourceType, resourceID, resourceName, details))
}

// LogFailure records an attempted resource action that failed after the caller
// was identified. It is intended for security-relevant mutation failures where
// operators need visibility into attempted actions as well as successful ones.
func (a *Auditor) LogFailure(r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName, errorMessage string, details map[string]any) {
	userID, username := auditActor(user)
	_ = LogAudit(a.db, AuditEvent{
		UserID:       userID,
		Username:     username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   actionType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      details,
		Success:      false,
		ErrorMessage: errorMessage,
	})
}

// LogDenied records an authorization/permission denial for an attempted action.
func (a *Auditor) LogDenied(r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName, requiredPermission string, details map[string]any) {
	if details == nil {
		details = map[string]any{}
	}
	if requiredPermission != "" {
		details["required_permission"] = requiredPermission
	}
	details["method"] = r.Method
	details["route"] = r.URL.Path
	a.LogFailure(r, user, actionType, resourceType, resourceID, resourceName, "permission_denied", details)
}

func auditActor(user *models.User) (userID int, username string) {
	if user == nil {
		return 0, "unknown"
	}
	return user.ID, user.Username
}

// NewRequestAuditEvent builds a successful HTTP-driven audit event.
func NewRequestAuditEvent(r *http.Request, user *models.User, actionType, resourceType string, resourceID *int, resourceName string, details map[string]any) AuditEvent {
	return AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   actionType,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      details,
		Success:      true,
	}
}

// AuditEvent represents a security or admin event that should be logged
type AuditEvent struct {
	UserID       int            // User who performed the action
	Username     string         // Username for quick reference
	IPAddress    string         // Client IP address
	UserAgent    string         // Client user agent
	ActionType   string         // e.g., "user.create", "permission.grant"
	ResourceType string         // e.g., "user", "workspace", "permission"
	ResourceID   *int           // ID of the resource (nullable)
	ResourceName string         // Human-readable resource name
	Details      map[string]any // Additional details (old_value, new_value, etc.)
	Success      bool           // Whether the operation succeeded
	ErrorMessage string         // Error message if failed
	Timestamp    time.Time      // When the event occurred (set automatically if zero)
}

const (
	auditDetailsRedactedValue = "[REDACTED]"
	auditDetailsMaxBytes      = 32 * 1024
)

// LogAudit logs an audit event to the database (immediate write).
func LogAudit(db database.Database, event AuditEvent) error {
	// Convert details map to JSON. On marshal failure we still persist
	// the audit row, but with a sentinel { "details_marshal_error": ... }
	// so the trail records *that* an event occurred and that its details
	// were lost — better than silently storing NULL while callers (~80 of
	// 81 sites discard LogAudit's error) believe rich details were saved.
	var detailsJSON *string
	if len(event.Details) > 0 {
		safeDetails, _ := sanitizeAuditDetails(event.Details).(map[string]any)
		detailsBytes, err := json.Marshal(safeDetails)
		if err != nil {
			slog.Warn("failed to marshal audit details", "error", err)
			sentinel, mErr := json.Marshal(map[string]string{"details_marshal_error": err.Error()})
			if mErr == nil {
				s := string(sentinel)
				detailsJSON = &s
			} else {
				s := `{"details_marshal_error":"unknown"}`
				detailsJSON = &s
			}
		} else {
			if len(detailsBytes) > auditDetailsMaxBytes {
				detailsBytes, _ = json.Marshal(map[string]any{
					"details_truncated": true,
					"original_bytes":    len(detailsBytes),
				})
			}
			detailsStr := string(detailsBytes)
			detailsJSON = &detailsStr
		}
	}

	// Set timestamp
	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	// Convert UserID 0 to nil for SQL NULL (SCIM/system operations have no user)
	var userIDPtr *int
	if event.UserID != 0 {
		userIDPtr = &event.UserID
	}

	// Determine if this is a SCIM operation (UserID 0 or action type starts with "scim.")
	isSCIMOperation := event.UserID == 0 || strings.HasPrefix(event.ActionType, "scim.")

	// Also log to structured logger for real-time monitoring (always immediate)
	if isSCIMOperation {
		slog.Info("audit_event",
			"source", "SCIM",
			"username", event.Username,
			"action_type", event.ActionType,
			"resource_type", event.ResourceType,
			"resource_id", event.ResourceID,
			"resource_name", event.ResourceName,
			"success", event.Success,
		)
	} else {
		slog.Info("audit_event",
			"user_id", event.UserID,
			"username", event.Username,
			"action_type", event.ActionType,
			"resource_type", event.ResourceType,
			"resource_id", event.ResourceID,
			"resource_name", event.ResourceName,
			"success", event.Success,
		)
	}

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
		userIDPtr, // nil for SCIM/system operations (becomes SQL NULL)
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
			"is_scim", isSCIMOperation,
			"resource_id", event.ResourceID,
		)
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	slog.Debug("audit event logged successfully",
		"action_type", event.ActionType,
		"resource_type", event.ResourceType,
		"is_scim", isSCIMOperation,
	)

	return nil
}

func sanitizeAuditDetails(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if isSensitiveAuditKey(k) {
				out[k] = auditDetailsRedactedValue
				continue
			}
			out[k] = sanitizeAuditDetails(val)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if isSensitiveAuditKey(k) {
				out[k] = auditDetailsRedactedValue
				continue
			}
			out[k] = val
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = sanitizeAuditDetails(val)
		}
		return out
	case []map[string]any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = sanitizeAuditDetails(val)
		}
		return out
	default:
		return v
	}
}

func isSensitiveAuditKey(key string) bool {
	k := strings.ToLower(key)
	for _, marker := range []string{
		"password",
		"secret",
		"authorization",
		"private_key",
		"api_key",
		"client_secret",
		"refresh_token",
		"access_token",
		"oauth_code",
	} {
		if strings.Contains(k, marker) {
			return true
		}
	}

	// Redact fields that hold a token/credential value without redacting safe
	// aggregate keys such as revoked_api_tokens or credential_id.
	return k == "token" || strings.HasSuffix(k, "_token") ||
		k == "credential" || strings.HasSuffix(k, "_credential")
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
	ActionLoginSuccess   = "login.success"
	ActionLoginFailure   = "login.failure"
	ActionLogout         = "logout"
	ActionPasswordChange = "password.change"

	// Permission management
	ActionPermissionGrant  = "permission.grant"
	ActionPermissionRevoke = "permission.revoke"

	// Role management
	ActionRoleAssign = "role.assign"
	ActionRoleRevoke = "role.revoke"

	// Workspace role management.
	ActionWorkspaceRoleCreate = "workspace_role.create"
	ActionWorkspaceRoleUpdate = "workspace_role.update"
	ActionWorkspaceRoleDelete = "workspace_role.delete"

	// Workspace management
	ActionWorkspaceCreate             = "workspace.create"
	ActionWorkspaceCreateFromTemplate = "workspace.create_from_template"
	ActionWorkspaceUpdate             = "workspace.update"
	ActionWorkspaceDelete             = "workspace.delete"

	// Group management
	ActionGroupCreate       = "group.create"
	ActionGroupUpdate       = "group.update"
	ActionGroupDelete       = "group.delete"
	ActionGroupAddMember    = "group.add_member"
	ActionGroupRemoveMember = "group.remove_member"

	// Configuration management
	ActionConfigSetCreate               = "config_set.create"
	ActionConfigSetUpdate               = "config_set.update"
	ActionConfigSetDelete               = "config_set.delete"
	ActionConfigSetExport               = "config_set.export"
	ActionConfigSetImport               = "config_set.import"
	ActionConfigSetNotificationAssign   = "config_set.notification_assign"
	ActionConfigSetNotificationUnassign = "config_set.notification_unassign"

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

	// Work item lifecycle. Routine create/update/transition changes are also
	// captured in work item history; central audit starts with destructive gaps.
	ActionItemDelete        = "item.delete"
	ActionItemDeleteCascade = "item.delete_cascade"
	ActionItemMoveWorkspace = "item.move_workspace"
	ActionCommentDelete     = "comment.delete"
	ActionAttachmentDelete  = "attachment.delete"

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
	ActionModuleSettingsUpdate    = "module_settings.update"
	ActionWorkItemStalenessUpdate = "work_item_staleness.update"

	// API Token management
	ActionAPITokenCreate      = "api_token.create"
	ActionAPITokenRevoke      = "api_token.revoke"
	ActionAPITokenAdminRevoke = "api_token.admin_revoke"
	ActionAPITokenCleanup     = "api_token.cleanup_expired"
	ActionAPITokenAutoRevoke  = "api_token.auto_revoke" //nolint:gosec // audit action name, not a credential

	// OAuth client management
	ActionOAuthClientCreate       = "oauth_client.create"
	ActionOAuthClientUpdate       = "oauth_client.update"
	ActionOAuthClientRotateSecret = "oauth_client.rotate_secret" //nolint:gosec // audit action name, not a credential
	ActionOAuthClientDelete       = "oauth_client.delete"

	// Agent management (user-managed agents, distinct from admin-provisioned service users)
	ActionAgentCreate     = "agent.create"
	ActionAgentUpdate     = "agent.update"
	ActionAgentDelete     = "agent.delete"
	ActionAgentDeactivate = "agent.deactivate"

	// SCIM provisioning
	ActionSCIMUserCreate        = "scim.user.create"
	ActionSCIMUserUpdate        = "scim.user.update"
	ActionSCIMUserDelete        = "scim.user.delete"
	ActionSCIMGroupCreate       = "scim.group.create"
	ActionSCIMGroupUpdate       = "scim.group.update"
	ActionSCIMGroupDelete       = "scim.group.delete"
	ActionSCIMGroupAddMember    = "scim.group.add_member"
	ActionSCIMGroupRemoveMember = "scim.group.remove_member"
	// ActionSCIMUserAgentImpact is the aggregate row emitted when a SCIM
	// deactivation cascades to owned agents/tokens. Per-agent and per-token
	// rows are also emitted (ActionAgentDeactivate, ActionAPITokenAutoRevoke).
	// Operators watch this row to know integrations may have just lost
	// credentials and need to be re-pointed.
	ActionSCIMUserAgentImpact = "scim.user.agent_impact"
	ActionSCIMTokenCreate     = "scim.token.create" //nolint:gosec // G101 false positive: audit action constant, not a credential
	ActionSCIMTokenRevoke     = "scim.token.revoke" //nolint:gosec // G101 false positive: audit action constant, not a credential

	// Hierarchy level management
	ActionHierarchyLevelCreate = "hierarchy_level.create"
	ActionHierarchyLevelUpdate = "hierarchy_level.update"
	ActionHierarchyLevelDelete = "hierarchy_level.delete"

	// Link type management
	ActionLinkTypeCreate = "link_type.create"
	ActionLinkTypeUpdate = "link_type.update"
	ActionLinkTypeDelete = "link_type.delete"

	// Permission set management
	ActionPermissionSetCreate           = "permission_set.create"
	ActionPermissionSetUpdate           = "permission_set.update"
	ActionPermissionSetDelete           = "permission_set.delete"
	ActionPermissionSetAssignmentCreate = "permission_set.assignment_create"
	ActionPermissionSetAssignmentDelete = "permission_set.assignment_delete"

	// Email template management (admin-edited transactional email bodies)
	ActionEmailTemplateUpdate = "email_template.update"

	// Channel category management
	ActionChannelCategoryCreate = "channel_category.create"
	ActionChannelCategoryUpdate = "channel_category.update"
	ActionChannelCategoryDelete = "channel_category.delete"

	// Channel management
	ActionChannelCreate        = "channel.create"
	ActionChannelUpdate        = "channel.update"
	ActionChannelDelete        = "channel.delete"
	ActionChannelActivate      = "channel.activate"
	ActionChannelDeactivate    = "channel.deactivate"
	ActionChannelAddManager    = "channel.add_manager"
	ActionChannelRemoveManager = "channel.remove_manager"

	// Attachment settings management
	ActionAttachmentSettingsUpdate = "attachment_settings.update"

	// Time project management
	ActionTimeProjectCreate = "time_project.create"
	ActionTimeProjectUpdate = "time_project.update"
	ActionTimeProjectDelete = "time_project.delete"

	// Milestone management
	ActionMilestoneCreate  = "milestone.create"
	ActionMilestoneUpdate  = "milestone.update"
	ActionMilestoneDelete  = "milestone.delete"
	ActionMilestoneReorder = "milestone.reorder"

	// Milestone category management
	ActionMilestoneCategoryCreate = "milestone_category.create"
	ActionMilestoneCategoryUpdate = "milestone_category.update"
	ActionMilestoneCategoryDelete = "milestone_category.delete"

	// Collection management
	ActionCollectionCreate = "collection.create"
	ActionCollectionUpdate = "collection.update"
	ActionCollectionDelete = "collection.delete"

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

	// SSO Provider management
	ActionSSOProviderCreate = "sso_provider.create"
	ActionSSOProviderUpdate = "sso_provider.update"
	ActionSSOProviderDelete = "sso_provider.delete"

	// LDAP Config management
	ActionLDAPConfigCreate = "ldap_config.create"
	ActionLDAPConfigUpdate = "ldap_config.update"
	ActionLDAPConfigDelete = "ldap_config.delete"

	// Security Settings
	ActionSecuritySettingsUpdate = "security_settings.update"
	ActionAuthPolicyUpdate       = "auth_policy.update"
	ActionAIFeaturesConfigUpdate = "ai_features_config.update"
	ActionHubConfigUpdate        = "hub_config.update"

	// Configuration-set migration execution
	ActionConfigSetMigrationExecute              = "config_set.migration_execute"
	ActionConfigSetComprehensiveMigrationExecute = "config_set.comprehensive_migration_execute"

	// Admin diagnostics / retention maintenance
	ActionDiagnosticsWebhookDeliveriesPurge     = "diagnostics.webhook_deliveries.purge"
	ActionDiagnosticsSchedulerRunsPurge         = "diagnostics.scheduler_runs.purge"
	ActionDiagnosticsRecurrenceVolumeUpdate     = "diagnostics.recurrence_volume.update"
	ActionDiagnosticsGlobalRankMigrationControl = "diagnostics.global_rank_migration.control"

	// LLM Connections
	ActionLLMConnectionCreate = "llm_connection.create"
	ActionLLMConnectionUpdate = "llm_connection.update"
	ActionLLMConnectionDelete = "llm_connection.delete"

	// Integration Providers
	ActionIntegrationProviderCreate             = "integration_provider.create"
	ActionIntegrationProviderUpdate             = "integration_provider.update"
	ActionIntegrationProviderDelete             = "integration_provider.delete"
	ActionIntegrationItemLinkCreate             = "integration_item_link.create"
	ActionIntegrationItemLinkDelete             = "integration_item_link.delete"
	ActionZammadConnectionCreate                = "zammad_connection.create"
	ActionZammadConnectionUpdate                = "zammad_connection.update"
	ActionZammadConnectionDelete                = "zammad_connection.delete"
	ActionZammadConnectionTest                  = "zammad_connection.test"
	ActionIntegrationProviderOAuthCredentialSet = "integration_provider.oauth_credential_set" // #nosec G101 -- audit action name, not a credential
	ActionZammadTicketCreate                    = "zammad_ticket.create"
	ActionZammadTicketLinkExisting              = "zammad_ticket.link_existing"
	ActionZammadTicketUpdate                    = "zammad_ticket.update"
	ActionZammadTicketUnlink                    = "zammad_ticket.unlink"
	ActionZammadTicketDetachLocal               = "zammad_ticket.detach_local"
	ActionZammadTicketRefresh                   = "zammad_ticket.refresh"
	ActionZammadTicketRefreshAll                = "zammad_ticket.refresh_all"

	// Issue sync configuration
	ActionIssueSyncConfigCreate = "issue_sync_config.create"
	ActionIssueSyncConfigUpdate = "issue_sync_config.update"
	ActionIssueSyncConfigDelete = "issue_sync_config.delete"
	ActionIssueSyncTrigger      = "issue_sync.trigger"

	// Email Providers
	ActionEmailProviderCreate = "email_provider.create"
	ActionEmailProviderUpdate = "email_provider.update"
	ActionEmailProviderDelete = "email_provider.delete"

	// WebAuthn
	ActionWebAuthnRegister = "webauthn.register"
	ActionWebAuthnRemove   = "webauthn.remove"

	// User credentials (SSH keys, legacy credentials)
	ActionCredentialCreate = "credential.create" // #nosec G101 -- audit action name, not a credential
	ActionCredentialRemove = "credential.remove" // #nosec G101 -- audit action name, not a credential

	// Plugins
	ActionPluginUpload  = "plugin.upload"
	ActionPluginDelete  = "plugin.delete"
	ActionPluginEnable  = "plugin.enable"
	ActionPluginDisable = "plugin.disable"
	ActionPluginReload  = "plugin.reload"

	// Jira Integration
	ActionJiraConnect          = "jira.connect"
	ActionJiraDisconnect       = "jira.disconnect"
	ActionJiraImport           = "jira.import"
	ActionJiraImportDeleteData = "jira.import.delete_data"

	// Iteration management
	ActionIterationCreate = "iteration.create"
	ActionIterationUpdate = "iteration.update"
	ActionIterationDelete = "iteration.delete"

	// Iteration type management
	ActionIterationTypeCreate = "iteration_type.create"
	ActionIterationTypeUpdate = "iteration_type.update"
	ActionIterationTypeDelete = "iteration_type.delete"

	// Automation/Actions management
	ActionAutomationCreate           = "automation.create"
	ActionAutomationUpdate           = "automation.update"
	ActionAutomationDelete           = "automation.delete"
	ActionAutomationToggle           = "automation.toggle"
	ActionAutomationSetActor         = "automation.set_actor" // Granted action.set_actor permission used to impersonate
	ActionAutomationCapabilityCreate = "automation_capability.create"
	ActionAutomationCapabilityUpdate = "automation_capability.update"
	ActionAutomationCapabilityDelete = "automation_capability.delete"
	ActionActionCredentialCreate     = "action_credential.create"
	ActionActionCredentialUpdate     = "action_credential.update"
	ActionActionCredentialRotate     = "action_credential.rotate"
	ActionActionCredentialDelete     = "action_credential.delete"

	// Test Folder management
	ActionTestFolderCreate = "test_folder.create"
	ActionTestFolderUpdate = "test_folder.update"
	ActionTestFolderDelete = "test_folder.delete"

	// Time category management
	ActionTimeCategoryCreate = "time_category.create"
	ActionTimeCategoryUpdate = "time_category.update"
	ActionTimeCategoryDelete = "time_category.delete"

	// Time customer management
	ActionTimeCustomerCreate = "time_customer.create"
	ActionTimeCustomerUpdate = "time_customer.update"
	ActionTimeCustomerDelete = "time_customer.delete"

	// Portal customer (contact) management
	ActionPortalCustomerCreate    = "portal_customer.create"
	ActionPortalCustomerUpdate    = "portal_customer.update"
	ActionPortalCustomerDelete    = "portal_customer.delete"
	ActionPortalCustomerUpdateOrg = "portal_customer.update_organisation" //nolint:misspell // British spelling

	// Time project permission management
	ActionTimeProjectAddManager    = "time_project.add_manager"
	ActionTimeProjectRemoveManager = "time_project.remove_manager"
	ActionTimeProjectAddMember     = "time_project.add_member"
	ActionTimeProjectRemoveMember  = "time_project.remove_member"

	// Customer organisation permission management
	ActionCustomerOrgAddManager    = "customer_organisation.add_manager"
	ActionCustomerOrgRemoveManager = "customer_organisation.remove_manager"
	ActionCustomerOrgAddMember     = "customer_organisation.add_member"
	ActionCustomerOrgRemoveMember  = "customer_organisation.remove_member"

	// Label management (workspace-level)
	ActionLabelCreate = "label.create"
	ActionLabelUpdate = "label.update"
	ActionLabelDelete = "label.delete"

	// Work item template management (workspace-level, WI-438)
	ActionTemplateCreate = "item_template.create"
	ActionTemplateUpdate = "item_template.update"
	ActionTemplateDelete = "item_template.delete"

	// Page label management (workspace-level, applies to wiki pages)
	ActionPageLabelCreate = "page_label.create"
	ActionPageLabelUpdate = "page_label.update"
	ActionPageLabelDelete = "page_label.delete"

	// Knowledge page lifecycle and ACL management.
	ActionPageCreate           = "page.create"
	ActionPageUpdate           = "page.update"
	ActionPageMove             = "page.move"
	ActionPageArchive          = "page.archive"
	ActionPageRestore          = "page.restore"
	ActionPagePermissionGrant  = "page.permission_grant"
	ActionPagePermissionRevoke = "page.permission_revoke"
	ActionPageInheritanceSet   = "page.inheritance_set"
	ActionPageUnarchive        = "page.unarchive"

	// Asset management
	ActionAssetCreate = "asset.create"
	ActionAssetUpdate = "asset.update"
	ActionAssetDelete = "asset.delete"

	// Asset type management
	ActionAssetTypeCreate = "asset_type.create"
	ActionAssetTypeUpdate = "asset_type.update"
	ActionAssetTypeDelete = "asset_type.delete"

	// Asset status management
	ActionAssetStatusCreate = "asset_status.create"
	ActionAssetStatusUpdate = "asset_status.update"
	ActionAssetStatusDelete = "asset_status.delete"

	// Asset category management
	ActionAssetCategoryCreate = "asset_category.create"
	ActionAssetCategoryUpdate = "asset_category.update"
	ActionAssetCategoryDelete = "asset_category.delete"

	// Asset set management
	ActionAssetSetCreate = "asset_set.create"
	ActionAssetSetUpdate = "asset_set.update"
	ActionAssetSetDelete = "asset_set.delete"

	// Asset set role management
	ActionAssetSetRoleAssign = "asset_set_role.assign"
	ActionAssetSetRoleRevoke = "asset_set_role.revoke"

	// Notification setting management
	ActionNotificationSettingCreate = "notification_setting.create"
	ActionNotificationSettingUpdate = "notification_setting.update"
	ActionNotificationSettingDelete = "notification_setting.delete"

	// Contact role management
	ActionContactRoleCreate = "contact_role.create"
	ActionContactRoleUpdate = "contact_role.update"
	ActionContactRoleDelete = "contact_role.delete"

	// Collection category management
	ActionCollectionCategoryCreate = "collection_category.create"
	ActionCollectionCategoryUpdate = "collection_category.update"
	ActionCollectionCategoryDelete = "collection_category.delete"

	// SCM provider management
	ActionSCMProviderCreate          = "scm_provider.create"
	ActionSCMProviderUpdate          = "scm_provider.update"
	ActionSCMProviderDelete          = "scm_provider.delete"
	ActionSCMProviderAllowlistAdd    = "scm_provider_allowlist.add"
	ActionSCMProviderAllowlistRemove = "scm_provider_allowlist.remove"
	ActionSCMProviderAllowlistUpdate = "scm_provider_allowlist.update"

	// Milestone release
	ActionMilestoneRelease = "milestone.release"

	// Team management
	ActionTeamCreate       = "team.create"
	ActionTeamUpdate       = "team.update"
	ActionTeamDelete       = "team.delete"
	ActionTeamAddMember    = "team.add_member"
	ActionTeamRemoveMember = "team.remove_member"
	ActionTeamAddGroup     = "team.add_group"
	ActionTeamRemoveGroup  = "team.remove_group"

	// Condition set management
	ActionConditionSetCreate = "condition_set.create"
	ActionConditionSetUpdate = "condition_set.update"
	ActionConditionSetDelete = "condition_set.delete"

	// Approval set management
	ActionApprovalSetCreate = "approval_set.create"
	ActionApprovalSetUpdate = "approval_set.update"
	ActionApprovalSetDelete = "approval_set.delete"

	// Approval runtime (request lifecycle)
	ActionApprovalDecide   = "approval.decide"
	ActionApprovalCancel   = "approval.cancel"
	ActionApprovalDelegate = "approval.delegate"
	ActionApprovalRefresh  = "approval.refresh_approvers"
	ActionApprovalEscalate = "approval.escalate"

	// Recurrence rules
	ActionRecurrenceCreate        = "recurrence.create"
	ActionRecurrenceUpdate        = "recurrence.update"
	ActionRecurrenceDelete        = "recurrence.delete"
	ActionRecurrenceForceGenerate = "recurrence.force_generate"

	// Team membership
	ActionTeamUpdateMemberRole = "team.update_member_role"

	// On-call administration (operational swap/incident user activity is kept
	// in its domain history rather than the administrative audit log).
	ActionOnCallScheduleCreate  = "oncall.schedule.create"
	ActionOnCallScheduleUpdate  = "oncall.schedule.update"
	ActionOnCallScheduleDelete  = "oncall.schedule.delete"
	ActionOnCallLayerCreate     = "oncall.layer.create"
	ActionOnCallLayerUpdate     = "oncall.layer.update"
	ActionOnCallLayerDelete     = "oncall.layer.delete"
	ActionOnCallSetLayerMembers = "oncall.layer.set_members"
	ActionOnCallOverrideCreate  = "oncall.override.create"
	ActionOnCallOverrideDelete  = "oncall.override.delete"
	ActionOnCallPolicyCreate    = "oncall.policy.create"
	ActionOnCallPolicyUpdate    = "oncall.policy.update"
	ActionOnCallPolicyDelete    = "oncall.policy.delete"
	ActionOnCallPolicySetRules  = "oncall.policy.set_rules"

	// Board configuration
	ActionBoardConfigCreate = "board_configuration.create"
	ActionBoardConfigUpdate = "board_configuration.update"
	ActionBoardConfigDelete = "board_configuration.delete"

	// Webhook dispatch
	ActionWebhookTrigger = "webhook.trigger"

	// Public form administration
	ActionRequestTypeConfigUpdate = "request_type_config.update"
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
	ResourceItem                 = "item"
	ResourceComment              = "comment"
	ResourceAttachment           = "attachment"
	ResourceItemType             = "item_type"
	ResourceScreen               = "screen"
	ResourceTheme                = "theme"
	ResourceModule               = "module"
	ResourceAPIToken             = "api_token"
	ResourceOAuthClient          = "oauth_client"
	ResourceHierarchyLevel       = "hierarchy_level"
	ResourceLinkType             = "link_type"
	ResourcePermissionSet        = "permission_set"
	ResourceEmailTemplate        = "email_template"
	ResourceChannel              = "channel"
	ResourceChannelManager       = "channel_manager"
	ResourceAttachmentSettings   = "attachment_settings"
	ResourceTimeProject          = "time_project"
	ResourceMilestone            = "milestone"
	ResourceMilestoneCategory    = "milestone_category"
	ResourceCollection           = "collection"
	ResourceTestCase             = "test_case"
	ResourceTestRun              = "test_run"
	ResourceTestSet              = "test_set"
	ResourceSCIMToken            = "scim_token"
	ResourceSSOProvider          = "sso_provider"
	ResourceLDAPConfig           = "ldap_config"
	ResourceSecuritySettings     = "security_settings"
	ResourceAuthPolicy           = "auth_policy"
	ResourceAIFeaturesConfig     = "ai_features_config"
	ResourceHubConfig            = "hub_config"
	ResourceDiagnostics          = "diagnostics"
	ResourceLLMConnection        = "llm_connection"
	ResourceIntegrationProvider  = "integration_provider"
	ResourceIntegrationItemLink  = "integration_item_link"
	ResourceZammadConnection     = "zammad_connection"
	ResourceZammadTicket         = "zammad_ticket"
	ResourceIssueSyncConfig      = "issue_sync_config"
	ResourceWorkItemStaleness    = "work_item_staleness"
	ResourceEmailProvider        = "email_provider"
	ResourceWebAuthn             = "webauthn"
	ResourceCredential           = "credential"
	ResourcePlugin               = "plugin"
	ResourceJiraImport           = "jira_import"
	ResourceIteration            = "iteration"
	ResourceAutomation           = "automation"
	ResourceAutomationCapability = "automation_capability"
	ResourceActionCredential     = "action_credential"
	ResourceTestFolder           = "test_folder"
	ResourceTimeCategory         = "time_category"
	ResourceTimeCustomer         = "time_customer"
	ResourcePortalCustomer       = "portal_customer"
	ResourceLabel                = "label"
	ResourceItemTemplate         = "item_template"
	ResourcePage                 = "page"
	ResourcePageLabel            = "page_label"
	ResourceAsset                = "asset"
	ResourceAssetType            = "asset_type"
	ResourceAssetStatus          = "asset_status"
	ResourceAssetCategory        = "asset_category"
	ResourceAssetSet             = "asset_set"
	ResourceAssetSetRole         = "asset_set_role"
	ResourceNotificationSetting  = "notification_setting"
	ResourceIterationType        = "iteration_type"
	ResourceChannelCategory      = "channel_category"
	ResourceContactRole          = "contact_role"
	ResourceCollectionCategory   = "collection_category"
	ResourceSCMProvider          = "scm_provider"
	ResourceSCMProviderAllowlist = "scm_provider_allowlist"
	ResourceTeam                 = "team"
	ResourceConditionSet         = "condition_set"
	ResourceApprovalSet          = "approval_set"
	ResourceApprovalRequest      = "approval_request"
	ResourceRecurrenceRule       = "recurrence_rule"
	ResourceOnCallSchedule       = "oncall_schedule"
	ResourceOnCallLayer          = "oncall_layer"
	ResourceOnCallOverride       = "oncall_override"
	ResourceOnCallPolicy         = "oncall_policy"
	ResourceBoardConfiguration   = "board_configuration"
	ResourceWebhook              = "webhook"
	ResourceRequestTypeConfig    = "request_type_config"
)
