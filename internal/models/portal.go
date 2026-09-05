package models

import "time"

// Channel represents an integration channel (inbound/outbound)
type Channel struct {
	ID              int        `json:"id"`
	Name            string     `json:"name"`
	Type            string     `json:"type"`      // smtp, webhook, imap, portal, widget
	Direction       string     `json:"direction"` // inbound, outbound
	Description     string     `json:"description"`
	Status          string     `json:"status"`                      // enabled, disabled
	IsDefault       bool       `json:"is_default"`                  // Default channel for its type
	Config          string     `json:"config"`                      // JSON configuration data
	PluginName      *string    `json:"plugin_name,omitempty"`       // Name of plugin that owns this channel (NULL for user-created)
	PluginWebhookID *string    `json:"plugin_webhook_id,omitempty"` // Plugin's internal webhook identifier
	CategoryID      *int       `json:"category_id,omitempty"`       // Optional category grouping
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastActivity    *time.Time `json:"last_activity,omitempty"` // Last time channel was used
	// Joined fields for API responses
	CategoryName  string           `json:"category_name,omitempty"`  // Category name (from JOIN)
	CategoryColor string           `json:"category_color,omitempty"` // Category color (from JOIN)
	Managers      []ChannelManager `json:"managers,omitempty"`       // Channel managers for detailed views
}

// ChannelCategory represents a grouping for channels
type ChannelCategory struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // Hex color code (e.g., "#3b82f6")
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ChannelConfig represents configuration for different channel types
type ChannelConfig struct {
	// SMTP Configuration
	SMTPHost          string `json:"smtp_host,omitempty"`
	SMTPPort          int    `json:"smtp_port,omitempty"`
	SMTPUsername      string `json:"smtp_username,omitempty"`
	SMTPPassword      string `json:"smtp_password,omitempty"`
	SMTPFromEmail     string `json:"smtp_from_email,omitempty"`
	SMTPFromName      string `json:"smtp_from_name,omitempty"`
	SMTPEncryption    string `json:"smtp_encryption,omitempty"` // tls/starttls, ssl, none
	SMTPSkipTLSVerify bool   `json:"smtp_skip_tls_verify,omitempty"`

	// Webhook Configuration
	WebhookURL              string            `json:"webhook_url,omitempty"`
	WebhookSecret           string            `json:"webhook_secret,omitempty"`
	WebhookHeaders          map[string]string `json:"webhook_headers,omitempty"`
	WebhookScopeType        string            `json:"webhook_scope_type,omitempty"`        // "all", "workspaces", "collections"
	WebhookWorkspaceIDs     []int             `json:"webhook_workspace_ids,omitempty"`     // Workspace IDs when scope is "workspaces"
	WebhookCollectionIDs    []int             `json:"webhook_collection_ids,omitempty"`    // Collection IDs when scope is "collections"
	WebhookAutoTrigger      bool              `json:"webhook_auto_trigger,omitempty"`      // Enable automatic event triggers
	WebhookSubscribedEvents []string          `json:"webhook_subscribed_events,omitempty"` // Events to trigger on (e.g., "item.created")
	WebhookPluginHandler    string            `json:"webhook_plugin_handler,omitempty"`    // Plugin handler function name (for plugin webhooks)

	// IMAP Configuration (for generic basic auth)
	IMAPHost       string `json:"imap_host,omitempty"`
	IMAPPort       int    `json:"imap_port,omitempty"`
	IMAPUsername   string `json:"imap_username,omitempty"`
	IMAPPassword   string `json:"imap_password,omitempty"`
	IMAPEncryption string `json:"imap_encryption,omitempty"`

	// Email Channel Configuration (inbound email to items)
	EmailProviderID *int   `json:"email_provider_id,omitempty"` // Link to email_providers table (legacy)
	EmailAuthMethod string `json:"email_auth_method,omitempty"` // 'oauth' or 'basic'

	// Inline OAuth App Credentials (per-channel)
	EmailOAuthProviderType string `json:"email_oauth_provider_type,omitempty"` // 'microsoft' or 'google'
	EmailOAuthClientID     string `json:"email_oauth_client_id,omitempty"`     // OAuth app client ID
	EmailOAuthClientSecret string `json:"email_oauth_client_secret,omitempty"` // Encrypted client secret
	EmailOAuthTenantID     string `json:"email_oauth_tenant_id,omitempty"`     // Microsoft tenant ID (or 'common')

	// OAuth Tokens (populated after successful OAuth flow)
	EmailOAuthAccessToken      string     `json:"email_oauth_access_token,omitempty"`      // Encrypted OAuth access token
	EmailOAuthRefreshToken     string     `json:"email_oauth_refresh_token,omitempty"`     // Encrypted OAuth refresh token
	EmailOAuthExpiresAt        *time.Time `json:"email_oauth_expires_at,omitempty"`        // Token expiration time
	EmailOAuthEmail            string     `json:"email_oauth_email,omitempty"`             // Connected email address
	EmailWorkspaceID           int        `json:"email_workspace_id,omitempty"`            // Target workspace for items
	EmailItemTypeID            *int       `json:"email_item_type_id,omitempty"`            // Item type to create
	EmailDefaultPriorityID     *int       `json:"email_default_priority_id,omitempty"`     // Default priority for items
	EmailMailbox               string     `json:"email_mailbox,omitempty"`                 // IMAP mailbox (default "INBOX")
	EmailMarkAsRead            bool       `json:"email_mark_as_read,omitempty"`            // Mark processed emails as read
	EmailDeleteAfterProcess    bool       `json:"email_delete_after_process,omitempty"`    // Delete emails after processing
	EmailConnectedPortalID     *int       `json:"email_connected_portal_id,omitempty"`     // Portal for "My Requests" visibility
	EmailTrackingRetentionDays int        `json:"email_tracking_retention_days,omitempty"` // Days to keep processed-email tracking rows; 0 = default (365). Anchor rows (referenced by in_reply_to) are kept regardless.

	// Portal Configuration
	PortalSlug         string `json:"portal_slug,omitempty"`        // URL-friendly identifier (e.g., "support-portal")
	PortalWorkspaceIDs []int  `json:"portal_workspace_ids"`         // Target workspaces for submissions
	PortalTitle        string `json:"portal_title,omitempty"`       // Display title for portal
	PortalDescription  string `json:"portal_description,omitempty"` // Description shown on portal page

	// Portal Registration Controls
	PortalRegistrationMode string   `json:"portal_registration_mode,omitempty"` // "open" (default) or "manual" — manual = only admin-managed customers can sign in
	PortalAllowedDomains   []string `json:"portal_allowed_domains,omitempty"`   // Email domain allow-list (lowercase, e.g. "acme.com"); empty = allow all

	// Portal Customization
	PortalGradient           int    `json:"portal_gradient,omitempty"`             // Selected gradient index (0-17)
	PortalTheme              string `json:"portal_theme,omitempty"`                // Theme mode: "light" or "dark"
	PortalSearchPlaceholder  string `json:"portal_search_placeholder,omitempty"`   // Custom search placeholder text
	PortalSearchHint         string `json:"portal_search_hint,omitempty"`          // Custom search hint text
	PortalBackgroundImageURL string `json:"portal_background_image_url,omitempty"` // Custom background image URL (overrides gradient)
	PortalLogoURL            string `json:"portal_logo_url,omitempty"`             // Custom portal logo URL
	PortalFooterColumns      []struct {
		Title string `json:"title"`
		Links []struct {
			Text string `json:"text"`
			URL  string `json:"url"`
		} `json:"links"`
	} `json:"portal_footer_columns,omitempty"` // 3-column footer with links
	PortalSections []PortalSection `json:"portal_sections,omitempty"` // Configurable content sections

	// Knowledge Base Configuration (Docmost)
	KnowledgeBaseShareLink string `json:"knowledge_base_share_link,omitempty"` // Full Docmost share link
	KnowledgeBaseURL       string `json:"knowledge_base_url,omitempty"`        // Parsed base URL (e.g., https://wiki.realigned.tech)
	KnowledgeBaseShareID   string `json:"knowledge_base_share_id,omitempty"`   // Parsed share ID (e.g., u1gkl0jk1u)

	// Form Channel Configuration
	FormSlug           string `json:"form_slug,omitempty"`            // URL-friendly identifier for form channel
	FormTheme          string `json:"form_theme,omitempty"`           // "light", "dark", "auto"
	FormBrandColor     string `json:"form_brand_color,omitempty"`     // Primary color for form UI
	FormLogoURL        string `json:"form_logo_url,omitempty"`        // Optional logo
	FormWorkspaceIDs   []int  `json:"form_workspace_ids"`             // Target workspaces for submissions
	FormSuccessMessage string `json:"form_success_message,omitempty"` // Default post-submit message
	FormRedirectURL    string `json:"form_redirect_url,omitempty"`    // Optional redirect after submit
}

// ChannelManager represents a user or group that can manage a channel
type ChannelManager struct {
	ID          int       `json:"id"`
	ChannelID   int       `json:"channel_id"`
	ManagerType string    `json:"manager_type"`       // 'user' or 'group'
	ManagerID   int       `json:"manager_id"`         // User ID or Group ID
	AddedBy     *int      `json:"added_by,omitempty"` // User who added this manager
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	ManagerName  string `json:"manager_name,omitempty"`  // User or group name
	ManagerEmail string `json:"manager_email,omitempty"` // Email (for users only)
	AddedByName  string `json:"added_by_name,omitempty"`
	ChannelName  string `json:"channel_name,omitempty"`
}

// ChannelManagerRequest represents the payload for adding/removing channel managers
type ChannelManagerRequest struct {
	ManagerType string `json:"manager_type"` // 'user' or 'group'
	ManagerIDs  []int  `json:"manager_ids"`
}

// PortalSection represents a configurable section on the portal page
type PortalSection struct {
	ID             string `json:"id"`               // UUID for client-side tracking
	Title          string `json:"title"`            // Section title (e.g., "Popular Requests")
	Subtitle       string `json:"subtitle"`         // Section subtitle (optional)
	DisplayOrder   int    `json:"display_order"`    // Order of section on page
	RequestTypeIDs []int  `json:"request_type_ids"` // Ordered list of request type IDs in this section
	AssetReportIDs []int  `json:"asset_report_ids"` // Ordered list of asset report IDs in this section
}

// PortalCustomer represents an individual portal user
type PortalCustomer struct {
	ID                     int            `json:"id"`
	Name                   string         `json:"name"`
	Email                  string         `json:"email"`
	Phone                  string         `json:"phone,omitempty"`
	UserID                 *int           `json:"user_id,omitempty"`                  // Links to internal user if applicable
	CustomerOrganisationID *int           `json:"customer_organisation_id,omitempty"` //nolint:misspell // matches API/database field name
	IsPrimary              bool           `json:"is_primary"`                         // Primary contact for the organization
	CustomFieldValues      map[string]any `json:"custom_field_values,omitempty"`
	CreatedAt              time.Time      `json:"created_at"`
	UpdatedAt              time.Time      `json:"updated_at"`
	// Joined fields for API responses
	UserName                 string        `json:"user_name,omitempty"`
	UserEmail                string        `json:"user_email,omitempty"`
	CustomerOrganisationName string        `json:"customer_organisation_name,omitempty"` //nolint:misspell // matches API/database field name
	Roles                    []ContactRole `json:"roles,omitempty"`                      // Contact roles assigned to this customer
}

// PortalCustomerChannel represents access control for portal customers per channel
type PortalCustomerChannel struct {
	ID               int       `json:"id"`
	PortalCustomerID int       `json:"portal_customer_id"`
	ChannelID        int       `json:"channel_id"`
	CreatedAt        time.Time `json:"created_at"`
	// Joined fields for API responses
	PortalCustomerName  string `json:"portal_customer_name,omitempty"`
	PortalCustomerEmail string `json:"portal_customer_email,omitempty"`
	ChannelName         string `json:"channel_name,omitempty"`
}

// PortalCustomerRole represents the many-to-many relationship between portal customers and contact roles
type PortalCustomerRole struct {
	ID               int       `json:"id"`
	PortalCustomerID int       `json:"portal_customer_id"`
	ContactRoleID    int       `json:"contact_role_id"`
	CreatedAt        time.Time `json:"created_at"`
	// Joined fields for API responses
	RoleName string `json:"role_name,omitempty"`
}

// CustomerOrganisation represents a B2B entity for time tracking
type CustomerOrganisation struct {
	ID                int            `json:"id"`
	Name              string         `json:"name"`
	Email             string         `json:"email"`
	Description       string         `json:"description"`
	Active            bool           `json:"active"`
	AvatarURL         string         `json:"avatar_url,omitempty"`
	CustomFieldValues map[string]any `json:"custom_field_values,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// ContactRole represents a role that can be assigned to portal customers
type ContactRole struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
}

// RequestTypeConfig represents per-form configuration for a request type used as a form
type RequestTypeConfig struct {
	RequireAuth      bool   `json:"require_auth"`
	AllowAttachments bool   `json:"allow_attachments"`
	SuccessMessage   string `json:"success_message,omitempty"`
	SubmitButtonText string `json:"submit_button_text,omitempty"`
	RedirectURL      string `json:"redirect_url,omitempty"`
}

// RequestType represents a portal request type that maps to an item type
type RequestType struct {
	ID                 int       `json:"id"`
	ChannelID          int       `json:"channel_id"` // Scope request type to specific portal/channel
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	ItemTypeID         int       `json:"item_type_id"`                   // n:1 relationship - which item type submissions create
	Icon               string    `json:"icon"`                           // Lucide icon name for visual representation
	Color              string    `json:"color"`                          // Hex color for visual representation
	DisplayOrder       int       `json:"display_order"`                  // Ordering within channel
	IsActive           bool      `json:"is_active"`                      // Enable/disable this request type
	Config             *string   `json:"config,omitempty"`               // JSON configuration for form-specific settings
	WorkspaceID        *int      `json:"workspace_id,omitempty"`         // Workspace for field resolution via config sets
	VisibilityGroupIDs []int     `json:"visibility_group_ids,omitempty"` // Internal groups that can see this request type
	VisibilityOrgIDs   []int     `json:"visibility_org_ids,omitempty"`   // Customer organizations that can see this request type
	TitleTemplate      string    `json:"title_template"`                 // Template used as the item title when the title field is hidden from the request form. Supports {{var}} placeholders (see services/template).
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Joined fields for API responses
	ChannelName  string `json:"channel_name,omitempty"`
	ItemTypeName string `json:"item_type_name,omitempty"`
	// WorkspaceName / WorkspaceKey identify the routing target workspace in
	// listings so admins and customers can tell same-named request types
	// across connected workspaces apart. Empty when workspace_id is NULL.
	WorkspaceName string `json:"workspace_name,omitempty"`
	WorkspaceKey  string `json:"workspace_key,omitempty"`
	// FieldCount is the number of configured form fields in request-type listings.
	FieldCount int `json:"field_count"`
}

// IsVisibleTo checks if this request type is visible to the given user groups and/or customer organization
// Returns true if no restrictions are set, or if the user matches any group OR the customer org matches any org
func (rt *RequestType) IsVisibleTo(userGroupIDs []int, customerOrgID *int) bool {
	// No restrictions = visible to all
	if len(rt.VisibilityGroupIDs) == 0 && len(rt.VisibilityOrgIDs) == 0 {
		return true
	}

	// Check group match (internal users)
	if len(rt.VisibilityGroupIDs) > 0 && len(userGroupIDs) > 0 {
		for _, gid := range rt.VisibilityGroupIDs {
			for _, ug := range userGroupIDs {
				if gid == ug {
					return true
				}
			}
		}
	}

	// Check org match (portal customers)
	if len(rt.VisibilityOrgIDs) > 0 && customerOrgID != nil {
		for _, oid := range rt.VisibilityOrgIDs {
			if oid == *customerOrgID {
				return true
			}
		}
	}

	return false
}

// AssetReport represents a portal asset report that displays filtered assets
type AssetReport struct {
	ID                 int       `json:"id"`
	ChannelID          int       `json:"channel_id"`
	AssetSetID         int       `json:"asset_set_id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	CQLQuery           string    `json:"cql_query"`
	Icon               string    `json:"icon"`
	Color              string    `json:"color"`
	DisplayOrder       int       `json:"display_order"`
	IsActive           bool      `json:"is_active"`
	ColumnConfig       []string  `json:"column_config,omitempty"`
	VisibilityGroupIDs []int     `json:"visibility_group_ids,omitempty"`
	VisibilityOrgIDs   []int     `json:"visibility_org_ids,omitempty"`
	RunMode            string    `json:"run_mode"`               // 'direct' (inline table) or 'form' (launch-as-request)
	ItemTypeID         *int      `json:"item_type_id,omitempty"` // Used in form mode to resolve custom fields
	WorkspaceID        *int      `json:"workspace_id,omitempty"`
	Config             *string   `json:"config,omitempty"` // JSON RequestTypeConfig
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Joined fields for API responses
	ChannelName  string `json:"channel_name,omitempty"`
	AssetSetName string `json:"asset_set_name,omitempty"`
	ItemTypeName string `json:"item_type_name,omitempty"`
}

// PublicAssetReport is the portal-facing contract for an asset report. It
// excludes the internal CQL expression, visibility principal IDs,
// channel/asset-set bindings, lifecycle flags, and timestamps.
type PublicAssetReport struct {
	ID           int                      `json:"id"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description"`
	Icon         string                   `json:"icon"`
	Color        string                   `json:"color"`
	DisplayOrder int                      `json:"display_order"`
	ColumnConfig []string                 `json:"column_config,omitempty"`
	RunMode      string                   `json:"run_mode"`
	ItemTypeID   *int                     `json:"item_type_id,omitempty"`
	WorkspaceID  *int                     `json:"workspace_id,omitempty"`
	Config       *PublicAssetReportConfig `json:"config,omitempty"`
}

// PublicAssetReportConfig contains only copy used by the current portal form
// UI. Internal auth and redirect controls are not part of the guest contract.
type PublicAssetReportConfig struct {
	SuccessMessage   string `json:"success_message,omitempty"`
	SubmitButtonText string `json:"submit_button_text,omitempty"`
}

// AssetReportField represents a field configuration for a form-mode asset report.
// Mirrors RequestTypeField; values collected at submission are substituted into the CQL query.
type AssetReportField struct {
	ID                  int       `json:"id"`
	AssetReportID       int       `json:"asset_report_id"`
	FieldIdentifier     string    `json:"field_identifier"`
	FieldType           string    `json:"field_type"` // 'default', 'custom', 'virtual'
	DisplayOrder        int       `json:"display_order"`
	IsRequired          bool      `json:"is_required"`
	DisplayName         *string   `json:"display_name,omitempty"`
	Description         *string   `json:"description,omitempty"`
	StepNumber          int       `json:"step_number"`
	VirtualFieldType    *string   `json:"virtual_field_type,omitempty"`
	VirtualFieldOptions *string   `json:"virtual_field_options,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	FieldName           string    `json:"field_name,omitempty"`
	FieldLabel          string    `json:"field_label,omitempty"`
}

// IsVisibleTo checks if this asset report is visible to the given user groups and/or customer organization
// Returns true if no restrictions are set, or if the user matches any group OR the customer org matches any org
func (ar *AssetReport) IsVisibleTo(userGroupIDs []int, customerOrgID *int) bool {
	// No restrictions = visible to all
	if len(ar.VisibilityGroupIDs) == 0 && len(ar.VisibilityOrgIDs) == 0 {
		return true
	}

	// Check group match (internal users)
	if len(ar.VisibilityGroupIDs) > 0 && len(userGroupIDs) > 0 {
		for _, gid := range ar.VisibilityGroupIDs {
			for _, ug := range userGroupIDs {
				if gid == ug {
					return true
				}
			}
		}
	}

	// Check org match (portal customers)
	if len(ar.VisibilityOrgIDs) > 0 && customerOrgID != nil {
		for _, oid := range ar.VisibilityOrgIDs {
			if oid == *customerOrgID {
				return true
			}
		}
	}

	return false
}

// RequestTypeField represents a field configuration for a request type
type RequestTypeField struct {
	ID              int    `json:"id"`
	RequestTypeID   int    `json:"request_type_id"`
	FieldIdentifier string `json:"field_identifier"` // Field identifier (e.g., "title", "description", custom field ID, or virtual field ID)
	FieldType       string `json:"field_type"`       // 'default', 'custom', or 'virtual'
	DisplayOrder    int    `json:"display_order"`    // Order in form
	IsRequired      bool   `json:"is_required"`      // Whether field is required
	// Display customization for portal
	DisplayName *string `json:"display_name,omitempty"` // Override label shown in portal
	Description *string `json:"description,omitempty"`  // Help text shown below field
	// Multi-step form support
	StepNumber int `json:"step_number"` // Which step this field appears on (default 1)
	// Virtual field support (only for field_type = 'virtual')
	VirtualFieldType    *string   `json:"virtual_field_type,omitempty"`    // 'text', 'textarea', 'select', 'checkbox'
	VirtualFieldOptions *string   `json:"virtual_field_options,omitempty"` // JSON array for select options
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	// Joined/computed fields for API responses
	FieldName  string `json:"field_name,omitempty"`
	FieldLabel string `json:"field_label,omitempty"` // Uses display_name if set, otherwise field_name
}

// EmailProvider represents an email provider configuration for inbound email channels
type EmailProvider struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Type      string    `json:"type"` // 'microsoft', 'google', 'generic'
	IsEnabled bool      `json:"is_enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// OAuth Configuration (for microsoft/google types)
	OAuthClientID              string `json:"oauth_client_id,omitempty"`
	OAuthClientSecretEncrypted string `json:"-"` // Never expose in JSON
	OAuthScopes                string `json:"oauth_scopes,omitempty"`
	OAuthTenantID              string `json:"oauth_tenant_id,omitempty"` // Microsoft tenant ID or 'common'

	// IMAP Settings (for generic type)
	IMAPHost       string `json:"imap_host,omitempty"`
	IMAPPort       int    `json:"imap_port,omitempty"`
	IMAPEncryption string `json:"imap_encryption,omitempty"` // "ssl" (implicit TLS) or "starttls"; legacy alias "tls" still accepted at connect time
}

// EmailProviderType constants
const (
	EmailProviderTypeMicrosoft = "microsoft"
	EmailProviderTypeGoogle    = "google"
	EmailProviderTypeGeneric   = "generic"
)

// EmailChannelState tracks IMAP sync state for an email channel
type EmailChannelState struct {
	ID            int        `json:"id"`
	ChannelID     int        `json:"channel_id"`
	LastUID       int        `json:"last_uid"`
	UIDValidity   uint32     `json:"uid_validity"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	ErrorCount    int        `json:"error_count"`
	LastError     string     `json:"last_error,omitempty"`
	// FailedMessage* is separate from ErrorCount: connection outages should
	// affect health without making an unrelated email look like poison.
	FailedMessageUID         int       `json:"-"`
	FailedMessageUIDValidity uint32    `json:"-"`
	FailedMessageCount       int       `json:"-"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// EmailMessageTracking records processed emails for deduplication and reply threading
type EmailMessageTracking struct {
	ID          int       `json:"id"`
	ChannelID   int       `json:"channel_id"`
	MessageID   string    `json:"message_id"`  // RFC 5322 Message-ID header
	InReplyTo   string    `json:"in_reply_to"` // For reply threading
	FromEmail   string    `json:"from_email"`
	FromName    string    `json:"from_name,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	ItemID      *int      `json:"item_id,omitempty"`    // Created item (nil if comment)
	CommentID   *int      `json:"comment_id,omitempty"` // Created comment (nil if new item)
	ProcessedAt time.Time `json:"processed_at"`
}

const (
	NotificationScopeLegacy    = "legacy"
	NotificationScopeSystem    = "system"
	NotificationScopeWorkspace = "workspace"
	NotificationScopeAsset     = "asset"
)

// Notification represents a system notification for a user.
type Notification struct {
	ID                            int        `json:"id"`
	UserID                        int        `json:"user_id"`
	Title                         string     `json:"title"`
	Message                       string     `json:"message"`
	Type                          string     `json:"type"` // info, warning, error, success, assignment, comment, status_change, reminder, milestone
	Timestamp                     time.Time  `json:"timestamp"`
	Read                          bool       `json:"read"`
	SeenAt                        *time.Time `json:"seen_at,omitempty"`    // When the user observed the notification in the tray (NULL until then). Distinct from Read: seeing does not suppress email batching.
	SentAt                        *time.Time `json:"sent_at,omitempty"`    // When notification was sent via email (NULL if not sent)
	Avatar                        string     `json:"avatar,omitempty"`     // Initials or avatar identifier
	ActionURL                     string     `json:"action_url,omitempty"` // URL to navigate to when clicked
	Metadata                      string     `json:"metadata,omitempty"`   // JSON for additional data
	AuthorizationScope            string     `json:"authorization_scope"`
	WorkspaceID                   *int       `json:"workspace_id,omitempty"`
	ItemID                        *int       `json:"item_id,omitempty"`
	SourceType                    string     `json:"source_type,omitempty"`
	SourceID                      *int       `json:"source_id,omitempty"`
	ReferencedEntityType          string     `json:"referenced_entity_type,omitempty"`
	ReferencedEntityID            *int       `json:"referenced_entity_id,omitempty"`
	ReferencedWorkspaceID         *int       `json:"referenced_workspace_id,omitempty"`
	ReferencedWorkspacePermission string     `json:"referenced_workspace_permission,omitempty"`
	DeliveryKey                   string     `json:"-"`
	CreatedAt                     time.Time  `json:"created_at"`
	UpdatedAt                     time.Time  `json:"updated_at"`
}

// NotificationCache represents a cached notification for BigCache storage
type NotificationCache struct {
	Notifications []Notification `json:"notifications"`
	LastSynced    time.Time      `json:"last_synced"`
	IsDirty       bool           `json:"is_dirty"` // Indicates if cache needs DB sync
	Complete      bool           `json:"complete"` // True when the cached page contains the full retained inbox
}

// EmailTemplate represents a customizable email template managed from the
// admin UI. The Name acts as the lookup key used by senders (e.g.
// "magic_link", "notification_batch"). Subject, HTMLBody, and TextBody are
// all Go html/template / text/template sources.
type EmailTemplate struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Subject     string    `json:"subject"`
	HTMLBody    string    `json:"html_body"`
	TextBody    string    `json:"text_body"`
	Description string    `json:"description,omitempty"`
	IsActive    bool      `json:"is_active"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// NotificationSetting represents a notification configuration that can be assigned to configuration sets
type NotificationSetting struct {
	ID                 int       `json:"id"`
	BuiltinKey         string    `json:"builtin_key,omitempty"`
	Name               string    `json:"name"` // e.g., "Development Team Notifications"
	DisplayName        string    `json:"display_name"`
	Description        string    `json:"description"` // e.g., "Standard notifications for development workspaces"
	DisplayDescription string    `json:"display_description,omitempty"`
	IsActive           bool      `json:"is_active"`
	CreatedBy          int       `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Joined fields for API responses
	CreatedByName string `json:"created_by_name,omitempty"`
	// Event rules
	EventRules []NotificationEventRule `json:"event_rules,omitempty"`
}

// NotificationEventRule represents a specific notification rule for an event type
type NotificationEventRule struct {
	ID                    int       `json:"id"`
	NotificationSettingID int       `json:"notification_setting_id"`
	EventType             string    `json:"event_type"` // item.created, item.assigned, item.commented, etc.
	IsEnabled             bool      `json:"is_enabled"`
	NotifyAssignee        bool      `json:"notify_assignee"`
	NotifyCreator         bool      `json:"notify_creator"`
	NotifyWatchers        bool      `json:"notify_watchers"`
	NotifyWorkspaceAdmins bool      `json:"notify_workspace_admins"`
	CustomRecipients      string    `json:"custom_recipients"` // JSON array of user IDs
	MessageTemplate       string    `json:"message_template"`  // Custom message template (optional)
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	// Joined fields
	NotificationSettingName string `json:"notification_setting_name,omitempty"`
}

// ConfigurationSetNotificationSetting links notification settings to configuration sets
type ConfigurationSetNotificationSetting struct {
	ID                    int       `json:"id"`
	ConfigurationSetID    int       `json:"configuration_set_id"`
	NotificationSettingID int       `json:"notification_setting_id"`
	CreatedAt             time.Time `json:"created_at"`
	// Joined fields for API responses
	ConfigurationSetName    string `json:"configuration_set_name,omitempty"`
	NotificationSettingName string `json:"notification_setting_name,omitempty"`
}

// NotificationEvent represents the available event types for notifications
type NotificationEvent struct {
	Type        string `json:"type"`        // item.created, item.assigned, etc.
	Name        string `json:"name"`        // "Item Created"
	Description string `json:"description"` // "Triggered when a new work item is created"
	Category    string `json:"category"`    // "item", "comment", "assignment", etc.
}

// Predefined notification event types
const (
	// Item events
	EventItemCreated  = "item.created"
	EventItemUpdated  = "item.updated"
	EventItemDeleted  = "item.deleted"
	EventItemAssigned = "item.assigned"

	// Comment events
	EventCommentCreated = "comment.created"
	EventCommentUpdated = "comment.updated"
	EventCommentDeleted = "comment.deleted"

	// Link events
	EventItemLinked   = "item.linked"
	EventItemUnlinked = "item.unlinked"

	// Status events
	EventStatusChanged = "status.changed"

	// Mention events
	EventMention = "mention.created"

	// Approval events
	EventApprovalRequested   = "approval.requested"
	EventApprovalStepStarted = "approval.step_started"
	EventApprovalDecided     = "approval.decided"
	EventApprovalCompleted   = "approval.completed"
	EventApprovalCancelled   = "approval.cancelled"
	EventApprovalEscalated   = "approval.escalated"
)

// GetAvailableNotificationEvents returns all available notification event types
func GetAvailableNotificationEvents() []NotificationEvent {
	return []NotificationEvent{
		{EventItemCreated, "Item Created", "When a new work item is created", "item"},
		{EventItemUpdated, "Item Updated", "When a work item is updated", "item"},
		{EventItemDeleted, "Item Deleted", "When a work item is deleted", "item"},
		{EventItemAssigned, "Item Assigned", "When a work item is assigned to a user", "assignment"},
		{EventCommentCreated, "Comment Added", "When a comment is added to a work item", "comment"},
		{EventCommentUpdated, "Comment Updated", "When a comment is modified", "comment"},
		{EventCommentDeleted, "Comment Deleted", "When a comment is deleted", "comment"},
		{EventItemLinked, "Item Linked", "When work items are linked together", "link"},
		{EventItemUnlinked, "Item Unlinked", "When work item links are removed", "link"},
		{EventStatusChanged, "Status Changed", "When a work item's status is changed", "status"},
		{EventMention, "User Mentioned", "When a user is @mentioned in a comment or description", "mention"},
		{EventApprovalRequested, "Approval Requested", "When an item enters a status that requires approval", "approval"},
		{EventApprovalStepStarted, "Approval Step Started", "When a new approval step opens for its approvers", "approval"},
		{EventApprovalDecided, "Approval Decision Made", "When an approver approves, rejects, or comments on a request", "approval"},
		{EventApprovalCompleted, "Approval Completed", "When an approval request reaches a final outcome (approved or rejected)", "approval"},
		{EventApprovalCancelled, "Approval Cancelled", "When a pending approval is cancelled (e.g. item left the status)", "approval"}, //nolint:misspell // British spelling consistent with event_type value
		{EventApprovalEscalated, "Approval Escalated", "When a pending approval step is escalated due to timeout or empty pool", "approval"},
	}
}

// ============================================
// Portal Hub Models
// ============================================

// PortalHubConfig represents the configuration for the Portal Hub central page
type PortalHubConfig struct {
	Title             string         `json:"title"`
	Description       string         `json:"description"`
	Gradient          int            `json:"gradient"`
	Theme             string         `json:"theme"`
	SearchPlaceholder string         `json:"search_placeholder"`
	SearchHint        string         `json:"search_hint"`
	LogoURL           string         `json:"logo_url,omitempty"`
	Sections          []HubSection   `json:"sections"`
	FooterColumns     []FooterColumn `json:"footer_columns"`
}

// HubSection represents a customizable section in the Portal Hub
type HubSection struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Visible bool   `json:"visible"`
}

// FooterColumn represents a column in the Portal Hub footer
type FooterColumn struct {
	Title string `json:"title"`
	Links []struct {
		Text string `json:"text"`
		URL  string `json:"url"`
	} `json:"links"`
}

// HubResponse is the API response for the Portal Hub
type HubResponse struct {
	Config           PortalHubConfig `json:"config"`
	Portals          []HubPortalInfo `json:"portals"`
	OpenRequestCount int             `json:"open_request_count"`
}

// HubPortalInfo represents portal information displayed in the hub
type HubPortalInfo struct {
	ID                 int                    `json:"id"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	Status             string                 `json:"status"`
	Slug               string                 `json:"slug"`
	Gradient           int                    `json:"gradient"`
	BackgroundImageURL string                 `json:"background_image_url"`
	RequestTypeCount   int                    `json:"request_type_count"`
	RequestTypes       []HubPortalRequestType `json:"request_types,omitempty"`
}

// HubPortalRequestType represents a request type for hub search
type HubPortalRequestType struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
}

// HubInboxItem represents a request/ticket in the hub inbox
type HubInboxItem struct {
	ID                  int       `json:"id"`
	Title               string    `json:"title"`
	Description         string    `json:"description"`
	CreatedAt           time.Time `json:"created_at"`
	StatusName          string    `json:"status_name"`
	StatusColor         string    `json:"status_color"`
	WorkspaceKey        string    `json:"workspace_key"`
	WorkspaceItemNumber int       `json:"workspace_item_number"`
	PortalName          string    `json:"portal_name"`
	PortalSlug          string    `json:"portal_slug"`
	SubmitterName       *string   `json:"submitter_name,omitempty"`
	SubmitterEmail      *string   `json:"submitter_email,omitempty"`
}

// HubInboxStatusFacet is a distinct status appearing in the inbox, used to
// build the status filter dropdown dynamically so renamed or custom
// statuses stay in sync without any frontend translation.
type HubInboxStatusFacet struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// HubInboxResponse is the API response for the hub inbox
type HubInboxResponse struct {
	Items        []HubInboxItem        `json:"items"`
	Total        int                   `json:"total"`
	Page         int                   `json:"page"`
	PerPage      int                   `json:"per_page"`
	TotalPages   int                   `json:"total_pages"`
	StatusFacets []HubInboxStatusFacet `json:"status_facets"`
}
