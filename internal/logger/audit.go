package logger

import (
	"context"
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
	// Source overrides the subsystem label used in the structured log. When
	// empty it is derived from the action type and actor (see auditSource).
	Source string
}

// Audit event sources for the structured log's "source" field. They describe
// which subsystem produced the event, not who the actor was.
const (
	AuditSourceUser   = "USER"   // Authenticated user session or API request
	AuditSourceSCIM   = "SCIM"   // SCIM provisioning via token auth
	AuditSourceSystem = "SYSTEM" // Unauthenticated or background actor with no user
)

const (
	auditDetailsRedactedValue  = "[REDACTED]"
	auditDetailsTruncatedValue = "[TRUNCATED]"
	auditDetailsMaxBytes       = 32 * 1024
	// Bound individual strings and slices before falling back to a
	// scalars-only payload, so a single oversized field cannot evict the
	// small summary fields an operator needs.
	auditDetailsMaxStringBytes = 512
	auditDetailsMaxSliceItems  = 100
	// auditStructuredDetailsMaxBytes bounds the details JSON mirrored to the
	// live log line so a large payload cannot flood the log stream.
	auditStructuredDetailsMaxBytes = 8 * 1024
)

// auditSource resolves the event's source. An explicit Source wins; otherwise
// a scim.* action type marks SCIM provisioning and a missing user (failed
// login, rejected SSH key, scheduler work) marks a system actor. A missing
// user must not be conflated with SCIM.
func auditSource(event AuditEvent) string {
	if event.Source != "" {
		return event.Source
	}
	if strings.HasPrefix(event.ActionType, "scim.") {
		return AuditSourceSCIM
	}
	if event.UserID == 0 {
		return AuditSourceSystem
	}
	return AuditSourceUser
}

const auditInsertQuery = `
		INSERT INTO audit_logs (
			timestamp, user_id, username, ip_address, user_agent,
			action_type, resource_type, resource_id, resource_name,
			details, success, error_message, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

// auditInsert holds the column values for one audit row so LogAudit and
// LogAuditTx share the same sanitization and marshaling.
type auditInsert struct {
	args []any
}

// LogAudit logs an audit event to the database (immediate write).
func LogAudit(db database.Database, event AuditEvent) error {
	insert, source, detailsJSON := prepareAuditEvent(event)
	emitAuditStructuredLog(event, source, detailsJSON)

	if _, err := db.ExecWrite(auditInsertQuery, insert.args...); err != nil {
		logAuditFailure(event, source, err)
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	slog.Debug("audit event logged successfully",
		"action_type", event.ActionType,
		"resource_type", event.ResourceType,
		"source", source,
	)

	return nil
}

// LogAuditTx writes an audit event inside an existing transaction so it
// commits or rolls back atomically with the mutation it describes. Callers
// that need that coupling use this instead of LogAudit.
func LogAuditTx(ctx context.Context, tx database.Tx, event AuditEvent) error {
	insert, source, detailsJSON := prepareAuditEvent(event)
	emitAuditStructuredLog(event, source, detailsJSON)

	if _, err := tx.ExecWriteContext(ctx, auditInsertQuery, insert.args...); err != nil {
		logAuditFailure(event, source, err)
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	slog.Debug("audit event logged successfully",
		"action_type", event.ActionType,
		"resource_type", event.ResourceType,
		"source", source,
	)

	return nil
}

func logAuditFailure(event AuditEvent, source string, err error) {
	slog.Error("failed to log audit event",
		"error", err,
		"action_type", event.ActionType,
		"resource_type", event.ResourceType,
		"source", source,
		"resource_id", event.ResourceID,
	)
}

// prepareAuditEvent sanitizes the details and resolves the insert values. It
// is shared by LogAudit and LogAuditTx so both paths store identical shapes.
func prepareAuditEvent(event AuditEvent) (insert auditInsert, source string, detailsJSON *string) {
	// Convert details map to JSON. On marshal failure we still persist
	// the audit row, but with a sentinel { "details_marshal_error": ... }
	// so the trail records *that* an event occurred and that its details
	// were lost — better than silently storing NULL while callers (~80 of
	// 81 sites discard LogAudit's error) believe rich details were saved.
	var detailsJSONOut *string
	if len(event.Details) > 0 {
		safeDetails, _ := sanitizeAuditDetails(event.Details).(map[string]any)
		detailsBytes, err := json.Marshal(safeDetails)
		if err != nil {
			slog.Warn("failed to marshal audit details", "error", err)
			sentinel, mErr := json.Marshal(map[string]string{"details_marshal_error": err.Error()})
			if mErr == nil {
				s := string(sentinel)
				detailsJSONOut = &s
			} else {
				s := `{"details_marshal_error":"unknown"}`
				detailsJSONOut = &s
			}
		} else {
			if len(detailsBytes) > auditDetailsMaxBytes {
				detailsBytes = truncateAuditDetails(safeDetails, len(detailsBytes))
			}
			detailsStr := string(detailsBytes)
			detailsJSONOut = &detailsStr
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

	source = auditSource(event)
	return auditInsert{args: []any{
		timestamp,
		userIDPtr, // nil for SCIM/system operations (becomes SQL NULL)
		event.Username,
		event.IPAddress,
		event.UserAgent,
		event.ActionType,
		event.ResourceType,
		event.ResourceID,
		event.ResourceName,
		detailsJSONOut,
		event.Success,
		event.ErrorMessage,
		source,
	}}, source, detailsJSONOut
}

// emitAuditStructuredLog mirrors the event to the structured logger for
// real-time monitoring (always immediate, independent of the DB write). The
// details attribute carries the same redacted, size-bounded payload that is
// persisted, so a failed event is diagnosable from the live log alone.
func emitAuditStructuredLog(event AuditEvent, source string, detailsJSON *string) {
	attrs := []any{
		"source", source,
		"username", event.Username,
		"action_type", event.ActionType,
		"resource_type", event.ResourceType,
		"resource_id", event.ResourceID,
		"resource_name", event.ResourceName,
		"success", event.Success,
	}
	if event.UserID != 0 {
		attrs = append(attrs, "user_id", event.UserID)
	}
	if event.ErrorMessage != "" {
		attrs = append(attrs, "error_message", event.ErrorMessage)
	}
	if detailsJSON != nil && *detailsJSON != "" {
		if len(*detailsJSON) <= auditStructuredDetailsMaxBytes {
			attrs = append(attrs, "details", *detailsJSON)
		} else {
			attrs = append(attrs, "details", auditDetailsTruncatedValue)
		}
	}
	slog.Info("audit_event", attrs...)
}

func sanitizeAuditDetails(v any) any {
	switch x := v.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return v
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if isSensitiveAuditKey(k) && !isBooleanAuditValue(val) {
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
		return sanitizeAuditJSONValue(x)
	}
}

// sanitizeAuditJSONValue redacts typed containers (structs, typed slices and
// maps) that the direct type switch cannot traverse. It round-trips through
// JSON so struct tags and exported-field names drive the redaction keys, then
// sanitizes the generic result. Without this a value such as
// `[]models.KnowledgeBasePageSource` would be marshaled unredacted.
func sanitizeAuditJSONValue(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return v
	}
	return sanitizeAuditDetails(decoded)
}

// isBooleanAuditValue reports whether a value is a bool. A sensitive-looking
// boolean flag such as has_secret reveals no secret material, so keeping it
// preserves useful audit metadata that a substring match on the key would
// otherwise destroy.
func isBooleanAuditValue(v any) bool {
	_, ok := v.(bool)
	return ok
}

// truncateAuditDetails shrinks an over-limit details map while preserving
// scalar summary fields. It first trims long strings and long slices; if the
// payload is still too large it keeps only scalars and replaces complex
// values with a marker. Either way the result records that it was truncated
// and how large the original was.
func truncateAuditDetails(details map[string]any, originalBytes int) []byte {
	shrunk, _ := shrinkAuditValue(details, auditDetailsMaxStringBytes, auditDetailsMaxSliceItems).(map[string]any)
	if b, err := json.Marshal(shrunk); err == nil && len(b) <= auditDetailsMaxBytes {
		return markAuditDetailsTruncated(shrunk, originalBytes)
	}

	scalars := make(map[string]any, len(details))
	for k, v := range details {
		switch v.(type) {
		case map[string]any, map[string]string, []any, []map[string]any:
			scalars[k] = auditDetailsTruncatedValue
		default:
			scalars[k] = v
		}
	}
	return markAuditDetailsTruncated(scalars, originalBytes)
}

// shrinkAuditValue recursively caps string length and slice length.
func shrinkAuditValue(v any, maxString, maxItems int) any {
	switch x := v.(type) {
	case string:
		if len(x) > maxString {
			return x[:maxString] + "…" + auditDetailsTruncatedValue
		}
		return x
	case []any:
		out := make([]any, 0, len(x))
		for i, item := range x {
			if i >= maxItems {
				out = append(out, auditDetailsTruncatedValue)
				break
			}
			out = append(out, shrinkAuditValue(item, maxString, maxItems))
		}
		return out
	case []map[string]any:
		out := make([]any, 0, len(x))
		for i, item := range x {
			if i >= maxItems {
				out = append(out, auditDetailsTruncatedValue)
				break
			}
			out = append(out, shrinkAuditValue(item, maxString, maxItems))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = shrinkAuditValue(val, maxString, maxItems)
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = shrinkAuditValue(val, maxString, maxItems)
		}
		return out
	default:
		return v
	}
}

func markAuditDetailsTruncated(details map[string]any, originalBytes int) []byte {
	details["details_truncated"] = true
	details["original_bytes"] = originalBytes
	b, err := json.Marshal(details)
	if err != nil {
		b, _ = json.Marshal(map[string]any{
			"details_truncated": true,
			"original_bytes":    originalBytes,
		})
	}
	return b
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
	ActionUserErase         = "user.erase"

	// Authentication
	ActionLoginSuccess   = "login.success"
	ActionLoginFailure   = "login.failure"
	ActionLogout         = "logout"
	ActionPasswordChange = "password.change"

	// Authorization
	// ActionAccessDenied records a rejected authorization attempt for a
	// mutating request; the required permission and route live in details.
	ActionAccessDenied = "access.denied"

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
	ActionConfigSetWorkspaceAssign      = "config_set.workspace_assign"
	ActionConfigSetDelete               = "config_set.delete"
	ActionConfigSetExport               = "config_set.export"
	ActionConfigSetImport               = "config_set.import"
	ActionConfigSetConformanceCheck     = "config_set.conformance_check"
	ActionConfigSetConformanceRepair    = "config_set.conformance_repair"
	ActionWorkspaceBundleExport         = "workspace.bundle_export"
	ActionWorkspaceBundleImport         = "workspace.bundle_import"
	ActionPackApply                     = "pack.apply"
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
	ActionItemDelete         = "item.delete"
	ActionItemDeleteCascade  = "item.delete_cascade"
	ActionItemMoveWorkspace  = "item.move_workspace"
	ActionItemAssign         = "item.assign"
	ActionItemMerge          = "item.merge"
	ActionItemSplit          = "item.split"
	ActionItemBulkUpdate     = "item.bulk_update"
	ActionItemBulkTransition = "item.bulk_transition"
	ActionItemBulkLabels     = "item.bulk_labels"
	ActionCommentDelete      = "comment.delete"
	ActionCommentCreate      = "comment.create"
	ActionAttachmentDelete   = "attachment.delete"

	// External request participants (WI-1136).
	ActionItemParticipantAdd    = "item.participant_add"
	ActionItemParticipantRemove = "item.participant_remove"

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

	// OAuth authorization decisions
	ActionOAuthApprove = "oauth.approve"
	ActionOAuthDeny    = "oauth.deny"

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
	ActionAgentChatTurn   = "agent.chat.turn"

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
	ActionSCIMDisconnect      = "scim.disconnect"

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
	ActionChannelKBPublish     = "channel.kb_publish"   // Portal knowledge base publish scope changed
	ActionCustomerView         = "portal_customer.view" // Customer-data detail view
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
	// ActionLLMConnectionRefreshModels records a provider model-catalog
	// refresh, which is not a connection edit.
	ActionLLMConnectionRefreshModels = "llm_connection.refresh_models"

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
	ActionIntegrationProviderOAuthStart         = "integration_provider.oauth_start"
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
	ActionJiraConnectionUpdate = "jira.connection.update"
	ActionJiraDisconnect       = "jira.disconnect"
	ActionJiraImport           = "jira.import"
	ActionJiraImportDeleteData = "jira.import.delete_data"
	ActionJiraImportHide       = "jira.import.hide"

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
	ActionAutomationExecute          = "automation.execute"
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
	ActionPortalCustomerCreate      = "portal_customer.create"
	ActionPortalCustomerUpdate      = "portal_customer.update"
	ActionPortalCustomerUpdateOrg   = "portal_customer.update_organisation" //nolint:misspell // British spelling
	ActionPortalCustomerErase       = "portal_customer.erase"
	ActionPortalCustomerDataExport  = "portal_customer.data_export"
	ActionPortalCustomerDeactivate  = "portal_customer.deactivate"
	ActionPortalCustomerActivate    = "portal_customer.activate"
	ActionPortalCustomerBulkCleanup = "portal_customer.bulk_cleanup"

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

	// Canned response management (WI-1138, workspace-level reply snippets)
	ActionCannedResponseCreate = "canned_response.create"
	ActionCannedResponseUpdate = "canned_response.update"
	ActionCannedResponseDelete = "canned_response.delete"

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

	// Asset set portal exposure
	ActionAssetSetPortalAccessGrant  = "asset_set_portal_access.grant"
	ActionAssetSetPortalAccessRevoke = "asset_set_portal_access.revoke"

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

	// Asset report management
	ActionAssetReportCreate           = "asset_report.create"
	ActionAssetReportUpdate           = "asset_report.update"
	ActionAssetReportDelete           = "asset_report.delete"
	ActionAssetReportVisibilityUpdate = "asset_report.visibility_update"
	ActionAssetReportFieldsUpdate     = "asset_report.fields_update"

	// Request type management (public intake forms)
	ActionRequestTypeCreate           = "request_type.create"
	ActionRequestTypeUpdate           = "request_type.update"
	ActionRequestTypeDelete           = "request_type.delete"
	ActionRequestTypeFieldsUpdate     = "request_type.fields_update"
	ActionRequestTypeVisibilityUpdate = "request_type.visibility_update"

	// Channel intake management
	ActionChannelIntakeCreate = "channel_intake.create"
	ActionChannelIntakeUpdate = "channel_intake.update"
	ActionChannelIntakeDelete = "channel_intake.delete"

	// Workspace agent bindings and runner registration
	ActionAgentProfileCreate          = "agent_profile.create"
	ActionAgentProfileUpdate          = "agent_profile.update"
	ActionAgentProfileMigrateRunner   = "agent_profile.migrate_runner"
	ActionAgentProfileConnectRunner   = "agent_profile.connect_runner"
	ActionAgentProfileTest            = "agent_profile.test"
	ActionAgentProfileReady           = "agent_profile.ready"
	ActionAgentRunnerSetupTokenMint   = "agent_runner_setup.token_mint"
	ActionAgentRunnerSetupTokenRevoke = "agent_runner_setup.token_revoke"
	ActionAgentBindingCreate          = "agent_binding.create"
	ActionAgentBindingUpdate          = "agent_binding.update"
	ActionAgentBindingUpdateConfig    = "agent_binding.update_config"
	ActionAgentBindingArchive         = "agent_binding.archive"
	ActionAgentBindingRestore         = "agent_binding.restore"
	ActionAgentBindingTestRun         = "agent_binding.test_run"
	ActionAgentSkillCreate            = "agent_skill.create"
	ActionAgentSkillUpdate            = "agent_skill.update"
	ActionAgentSkillDelete            = "agent_skill.delete"

	// Agent security administration
	ActionAgentSecurityFlagSet         = "agent_security.flag.set"
	ActionAgentSecurityAllowlistAdd    = "agent_security.allowlist.add"
	ActionAgentSecurityAllowlistRemove = "agent_security.allowlist.remove"

	// Agent template catalog administration
	ActionAgentTemplateCatalogCreate = "agent_template_catalog.create"
	ActionAgentTemplateCatalogUpdate = "agent_template_catalog.update"
	ActionAgentTemplateCatalogDelete = "agent_template_catalog.delete"

	// CLI onboarding
	ActionCLIOnboardingApprove = "cli_onboarding.approve"
	ActionCLIOnboardingDeny    = "cli_onboarding.deny"

	// LDAP synchronization
	ActionLDAPSync = "ldap.sync"

	// Catalog priority management
	ActionPriorityCreate = "priority.create"
	ActionPriorityUpdate = "priority.update"
	ActionPriorityDelete = "priority.delete"

	// Bulk import
	ActionAssetImport  = "asset_import"
	ActionTicketImport = "ticket_import"

	// SLA configuration management
	ActionSLACalendarCreate         = "sla_calendar.create"
	ActionSLACalendarUpdate         = "sla_calendar.update"
	ActionSLACalendarDelete         = "sla_calendar.delete"
	ActionSLAWarningThresholdCreate = "sla_warning_threshold.create"
	ActionSLAWarningThresholdUpdate = "sla_warning_threshold.update"
	ActionSLAWarningThresholdDelete = "sla_warning_threshold.delete"
	ActionSLAMetricCreate           = "sla_metric.create"
	ActionSLAMetricUpdate           = "sla_metric.update"
	ActionSLAMetricDelete           = "sla_metric.delete"
	ActionSLATeamBindingCreate      = "sla_team_binding.create"
	ActionSLATeamBindingDelete      = "sla_team_binding.delete"

	// Object translation administration
	ActionObjectTranslationUpdate = "object_translation.update"
	ActionObjectTranslationDelete = "object_translation.delete"

	// User avatar
	ActionUserAvatarUpdate = "user.avatar_update"

	// Audit log access (reads and SIEM streaming are security-relevant)
	ActionAuditLogRead   = "audit_log.read"
	ActionAuditLogStream = "audit_log.stream"
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
	ResourceAgentSession         = "agent_session"
	ResourceRoute                = "route"
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
	ResourceCustomerOrganisation = "customer_organisation"
	ResourceAgent                = "agent"
	ResourceRequestType          = "request_type"
	ResourceJiraConnection       = "jira_connection"
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
	ResourceCannedResponse       = "canned_response"
	ResourcePage                 = "page"
	ResourcePageLabel            = "page_label"
	ResourceAsset                = "asset"
	ResourceAssetType            = "asset_type"
	ResourceAssetStatus          = "asset_status"
	ResourceAssetCategory        = "asset_category"
	ResourceAssetSet             = "asset_set"
	ResourceAssetSetRole         = "asset_set_role"
	ResourceAssetSetPortalAccess = "asset_set_portal_access"
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
	ResourceAssetReport          = "asset_report"
	ResourceChannelIntake        = "channel_intake"

	// Workspace agent binding and runner registration resources
	ResourceWorkspaceAgentBinding   = "workspace_agent_binding"
	ResourceRunnerRegistrationToken = "runner_registration_token"
	ResourceAgentSkill              = "workspace_agent_skill"
	ResourceAgentSecurityFlag       = "agent_security_flag"
	ResourceAgentSecurityAllowlist  = "agent_security_allowlist"
	ResourceAgentTemplateCatalog    = "agent_template_catalog"

	// Catalog priority resource
	ResourcePriority = "priority"

	// Bulk import resources
	ResourceAssetImport = "asset_import"

	// SLA configuration resources
	ResourceSLACalendar         = "sla_calendar"
	ResourceSLAWarningThreshold = "sla_warning_threshold"
	ResourceSLAMetric           = "sla_metric"
	ResourceSLATeamBinding      = "sla_team_binding"

	// Object translation administration
	ResourceObjectTranslation = "object_translation"

	// Audit log read surface
	ResourceAuditLog = "audit_log"
)
