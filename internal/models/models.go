package models

import (
	"time"
)

// APIWarning represents a non-fatal warning in an API response
type APIWarning struct {
	Code    string `json:"code"`              // Machine-readable code, e.g., "cache_invalidation_failed"
	Message string `json:"message"`           // Human-readable message
	Context string `json:"context,omitempty"` // Additional context, e.g., "user_id:123"
}

// CollectionCategory represents a category for organizing global collections
type CollectionCategory struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // Hex color code (e.g., "#3b82f6")
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Collection represents a saved QL query with metadata
type Collection struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	QLQuery     string `json:"ql_query"`
	IsPublic    bool   `json:"is_public"`
	WorkspaceID *int   `json:"workspace_id"`
	CategoryID  *int   `json:"category_id,omitempty"`
	CreatedBy   *int   `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	// Joined fields for API responses
	CreatorName   string `json:"creator_name,omitempty"`
	CreatorEmail  string `json:"creator_email,omitempty"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
}

// CalendarScheduleEntry represents a user's calendar scheduling for an item
type CalendarScheduleEntry struct {
	UserID          int    `json:"user_id"`
	WorkspaceID     int    `json:"workspace_id"`               // User's personal workspace ID
	ScheduledDate   string `json:"scheduled_date"`             // YYYY-MM-DD format
	ScheduledTime   string `json:"scheduled_time,omitempty"`   // HH:MM format, optional
	DurationMinutes int    `json:"duration_minutes,omitempty"` // Duration in minutes, optional
	Notes           string `json:"notes,omitempty"`
	CreatedAt       string `json:"created_at"`
}

type Project struct {
	ID          int       `json:"id"`
	WorkspaceID *int      `json:"workspace_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	WorkspaceName string `json:"workspace_name,omitempty"`
	// Milestone category associations
	MilestoneCategories []int `json:"milestone_categories,omitempty"`
}

// Item represents a work item (formerly Issue) with hierarchical support
type Item struct {
	ID                  int        `json:"id"`
	WorkspaceID         int        `json:"workspace_id"`
	WorkspaceItemNumber int        `json:"workspace_item_number"`  // Workspace-specific sequential number for display keys
	ItemTypeID          *int       `json:"item_type_id,omitempty"` // Optional, defaults to workspace's default item type
	Title               string     `json:"title"`
	Description         string     `json:"description"`
	StatusID            *int       `json:"status_id,omitempty"`    // Foreign key to statuses table
	PriorityID          *int       `json:"priority_id,omitempty"`  // Foreign key to priorities table
	DueDate             *time.Time `json:"due_date,omitempty"`     // Due date for item completion
	IsTask              bool       `json:"is_task"`                // Flag to mark this item as a task (checklist item)
	MilestoneID         *int       `json:"milestone_id,omitempty"` // Optional milestone assignment
	IterationID         *int       `json:"iteration_id,omitempty"` // Optional iteration assignment
	// Project assignment
	ProjectID      *int `json:"project_id,omitempty"` // General project assignment
	InheritProject bool `json:"inherit_project"`      // If true, inherit project from parent
	// Time tracking integration
	TimeProjectID *int `json:"time_project_id,omitempty"` // Override project for time logging on this item
	// User assignment fields
	AssigneeID              *int `json:"assignee_id,omitempty"`                // User assigned to this item
	CreatorID               *int `json:"creator_id,omitempty"`                 // Internal user who created this item
	ReporterID              *int `json:"reporter_id,omitempty"`                // User who reported this item (from Jira import)
	CreatorPortalCustomerID *int `json:"creator_portal_customer_id,omitempty"` // Portal customer who created this item (for portal submissions)
	// Portal submission tracking fields (immutable once set)
	ChannelID         *int                    `json:"channel_id,omitempty"`      // Portal/channel this request was submitted through
	RequestTypeID     *int                    `json:"request_type_id,omitempty"` // Request type used for submission
	CustomFieldValues map[string]interface{}  `json:"custom_field_values,omitempty"`
	VirtualFieldData  map[string]interface{}  `json:"virtual_field_data,omitempty"`
	CalendarData      []CalendarScheduleEntry `json:"calendar_data,omitempty"`
	// Hierarchy fields
	ParentID *int `json:"parent_id"` // Foreign key to parent item
	// Personal task relationship (for linking personal workspace tasks to work items)
	RelatedWorkItemID *int `json:"related_work_item_id,omitempty"` // Link to work item (for personal tasks)
	// Manual sorting field
	FracIndex   *string    `json:"frac_index,omitempty"` // Fractional index string for manual ordering
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// Joined fields for API responses
	WorkspaceName   string `json:"workspace_name,omitempty"`
	WorkspaceKey    string `json:"workspace_key,omitempty"`
	ItemTypeName    string `json:"item_type_name,omitempty"`
	PriorityName    string `json:"priority_name,omitempty"`
	PriorityIcon    string `json:"priority_icon,omitempty"`
	PriorityColor   string `json:"priority_color,omitempty"`
	ParentTitle     string `json:"parent_title,omitempty"`
	StatusName      string `json:"status_name,omitempty"` // Name from statuses table (joined field)
	MilestoneName   string `json:"milestone_name,omitempty"`
	IterationName   string `json:"iteration_name,omitempty"`
	ProjectName     string `json:"project_name,omitempty"`      // Name of assigned project
	TimeProjectName string `json:"time_project_name,omitempty"` // Name of time project
	// Effective project (computed from inheritance)
	EffectiveProjectID     *int   `json:"effective_project_id,omitempty"`     // Computed effective project
	EffectiveProjectName   string `json:"effective_project_name,omitempty"`   // Computed effective project name
	ProjectInheritanceMode string `json:"project_inheritance_mode,omitempty"` // "none", "inherit", "direct"
	// User information for API responses
	AssigneeName               string `json:"assignee_name,omitempty"`                 // Full name of assigned user
	AssigneeEmail              string `json:"assignee_email,omitempty"`                // Email of assigned user
	AssigneeAvatar             string `json:"assignee_avatar,omitempty"`               // Avatar URL of assigned user
	CreatorName                string `json:"creator_name,omitempty"`                  // Full name of creator (internal user or portal customer)
	CreatorEmail               string `json:"creator_email,omitempty"`                 // Email of creator (internal user or portal customer)
	ReporterName               string `json:"reporter_name,omitempty"`                 // Full name of reporter
	ReporterEmail              string `json:"reporter_email,omitempty"`                // Email of reporter
	ReporterAvatar             string `json:"reporter_avatar,omitempty"`               // Avatar URL of reporter
	CreatorPortalCustomerName  string `json:"creator_portal_customer_name,omitempty"`  // Name of portal customer creator
	CreatorPortalCustomerEmail string `json:"creator_portal_customer_email,omitempty"` // Email of portal customer creator
	// Portal submission tracking joined fields
	ChannelName     string `json:"channel_name,omitempty"`      // Name of the portal/channel
	RequestTypeName string `json:"request_type_name,omitempty"` // Name of the request type
	// Related work item joined fields (for personal tasks)
	RelatedWorkItemTitle        string `json:"related_work_item_title,omitempty"`
	RelatedWorkItemWorkspaceKey string `json:"related_work_item_workspace_key,omitempty"`
	RelatedWorkItemWorkspaceID  int    `json:"related_work_item_workspace_id,omitempty"`
	RelatedWorkItemNumber       int    `json:"related_work_item_number,omitempty"`
	Children                    []Item `json:"children,omitempty"` // For tree representations
}

// ItemHistory represents a single change to an item field
type ItemHistory struct {
	ID        int       `json:"id"`
	ItemID    int       `json:"item_id"`
	UserID    int       `json:"user_id"`
	ChangedAt time.Time `json:"changed_at"`
	FieldName string    `json:"field_name"`
	OldValue  *string   `json:"old_value"`
	NewValue  *string   `json:"new_value"`
	// Joined fields for API responses
	UserName  string `json:"user_name,omitempty"`  // Full name of user who made the change
	UserEmail string `json:"user_email,omitempty"` // Email of user who made the change
	// Resolved values for display (when value is an ID)
	ResolvedOldValue *string `json:"resolved_old_value,omitempty"` // Human-readable version of old_value
	ResolvedNewValue *string `json:"resolved_new_value,omitempty"` // Human-readable version of new_value
}

// User represents a system user
type User struct {
	ID                    int       `json:"id"`
	Email                 string    `json:"email"`
	Username              string    `json:"username"`
	FirstName             string    `json:"first_name"`
	LastName              string    `json:"last_name"`
	IsActive              bool      `json:"is_active"`
	AvatarURL             string    `json:"avatar_url,omitempty"`
	PasswordHash          string    `json:"-"` // Never send password hash to client
	RequiresPasswordReset bool      `json:"requires_password_reset"`
	Timezone              string    `json:"timezone"` // User's timezone (IANA timezone, e.g., "America/New_York")
	Language              string    `json:"language"` // User's preferred language (ISO 639-1 code, e.g., "en", "de")
	// Email verification fields (for SSO users when IdP doesn't provide email_verified)
	EmailVerified            bool       `json:"email_verified"`
	EmailVerificationToken   string     `json:"-"` // Never send token to client
	EmailVerificationExpires *time.Time `json:"-"` // Never send expiry to client
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	// Virtual fields for display
	FullName      string `json:"full_name,omitempty"`
	IsSystemAdmin bool   `json:"is_system_admin"` // Populated from permissions, cached at login
	// SCIM fields
	SCIMExternalID string `json:"scim_external_id,omitempty"` // External ID from identity provider
	SCIMManaged    bool   `json:"scim_managed"`               // If true, user is managed via SCIM and cannot be edited locally
}

// UserCredential represents a user's authentication credential (FIDO key, TOTP secret, etc.)
type UserCredential struct {
	ID             string     `json:"id"` // Changed to string to support both int (legacy) and string (WebAuthn) IDs
	UserID         int        `json:"user_id"`
	CredentialType string     `json:"credential_type"` // 'fido', 'totp'
	CredentialName string     `json:"credential_name"` // User-friendly name
	CredentialData string     `json:"-"`               // JSON data, never send to client
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}

// UserSession represents an active user session
type UserSession struct {
	ID           int       `json:"id"`
	UserID       int       `json:"user_id"`
	SessionToken string    `json:"session_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	IPAddress    string    `json:"ip_address,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

type Comment struct {
	ID               int       `json:"id"`
	ItemID           int       `json:"item_id"`                      // Changed from IssueID to ItemID
	AuthorID         *int      `json:"author_id,omitempty"`          // User ID (nullable for portal customer comments)
	PortalCustomerID *int      `json:"portal_customer_id,omitempty"` // Portal customer ID (for email-based comments)
	Content          string    `json:"content"`                      // TipTap JSON content
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Joined fields for API responses
	AuthorName   string `json:"author_name,omitempty"`
	AuthorEmail  string `json:"author_email,omitempty"`
	AuthorAvatar string `json:"author_avatar,omitempty"`
}

// Mention represents an @mention in a comment or item description
type Mention struct {
	ID                       int       `json:"id"`
	SourceType               string    `json:"source_type"` // "comment" or "item_description"
	SourceID                 int       `json:"source_id"`   // comment.id or item.id
	MentionedUserID          int       `json:"mentioned_user_id"`
	ItemID                   int       `json:"item_id"`
	WorkspaceID              int       `json:"workspace_id"`
	CreatedBy                int       `json:"created_by"`
	MentionedUserDisplayName string    `json:"mentioned_user_display_name"` // Snapshot at mention time
	NotificationSent         bool      `json:"notification_sent"`
	CreatedAt                time.Time `json:"created_at"`
}

type CustomFieldDefinition struct {
	ID                             int       `json:"id"`
	Name                           string    `json:"name"`
	FieldType                      string    `json:"field_type"`
	Description                    string    `json:"description,omitempty"`
	Required                       bool      `json:"required"`
	Options                        string    `json:"options,omitempty"` // JSON string for select options
	DisplayOrder                   int       `json:"display_order"`
	SystemDefault                  bool      `json:"system_default"` // Cannot be deleted by users
	AppliesToPortalCustomers       bool      `json:"applies_to_portal_customers"`
	AppliesToCustomerOrganisations bool      `json:"applies_to_customer_organisations"`
	CreatedAt                      time.Time `json:"created_at"`
	UpdatedAt                      time.Time `json:"updated_at"`
}

type ProjectFieldRequirement struct {
	ID            int  `json:"id"`
	ProjectID     int  `json:"project_id"`
	CustomFieldID int  `json:"custom_field_id"`
	IsRequired    bool `json:"is_required"`
	// Joined fields for API responses
	FieldName   string `json:"field_name,omitempty"`
	FieldType   string `json:"field_type,omitempty"`
	ProjectName string `json:"project_name,omitempty"`
}

type Workspace struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"` // Workspace key for issue prefixes (e.g., "TEST", "PROJ")
	Description string `json:"description"`
	Active      bool   `json:"active"`
	// Personal workspace fields
	IsPersonal bool `json:"is_personal"`        // Flag to indicate personal workspace
	OwnerID    *int `json:"owner_id,omitempty"` // User ID for personal workspaces
	// Time tracking integration
	TimeProjectID *int `json:"time_project_id,omitempty"` // Default project for time logging in this workspace
	// Visual identity fields
	Icon           string    `json:"icon"`                      // Lucide icon name for workspace
	Color          string    `json:"color"`                     // Hex color code for workspace
	AvatarURL      *string   `json:"avatar_url,omitempty"`      // Custom avatar image URL
	HomepageLayout *string   `json:"homepage_layout,omitempty"` // JSON object with sections and widgets
	DefaultView    string    `json:"default_view,omitempty"`    // Default view when entering workspace (board, backlog, list, tree, map)
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	// Joined fields for API responses
	ProjectCount          int    `json:"project_count,omitempty"`
	TimeProjectName       string `json:"time_project_name,omitempty"`
	OwnerName             string `json:"owner_name,omitempty"` // Name of workspace owner for API responses
	ConfigurationSetID    *int64 `json:"configuration_set_id,omitempty"`
	TimeProjectCategories []int  `json:"time_project_categories,omitempty"` // Restricted time project categories for this workspace
}

// WorkspaceHomepageSection represents a section on the workspace homepage
type WorkspaceHomepageSection struct {
	ID           string   `json:"id"`            // UUID for client-side tracking
	Title        string   `json:"title"`         // Section title (e.g., "Overview", "Charts")
	Subtitle     string   `json:"subtitle"`      // Section subtitle (optional)
	DisplayOrder int      `json:"display_order"` // Order of section on page
	WidgetIDs    []string `json:"widget_ids"`    // Ordered list of widget IDs in this section
}

// WorkspaceWidget represents a single widget in the workspace homepage
type WorkspaceWidget struct {
	ID        string                 `json:"id"`               // UUID for client-side tracking
	Type      string                 `json:"type"`             // Widget type: "stats", "completion-chart", "created-chart", "milestone-progress", etc.
	SectionID string                 `json:"section_id"`       // Which section this widget belongs to
	Position  int                    `json:"position"`         // Display order within the section
	Width     int                    `json:"width"`            // Column span: 1, 2, or 3 (for grid-based sections)
	Config    map[string]interface{} `json:"config,omitempty"` // Widget-specific configuration
}

// WorkspaceHomepageLayout represents the complete homepage layout structure
type WorkspaceHomepageLayout struct {
	Sections        []WorkspaceHomepageSection `json:"sections"`        // Sections on the homepage
	Widgets         []WorkspaceWidget          `json:"widgets"`         // All widgets across all sections
	Gradient        int                        `json:"gradient"`        // Selected gradient index (-1 = none, 0-17 = gradient index)
	ApplyToAllViews bool                       `json:"applyToAllViews"` // If true, apply gradient to all workspace views
}

type Screen struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	Fields       []ScreenField `json:"fields,omitempty"`
	SystemFields []string      `json:"system_fields,omitempty"` // List of system field names to show
}

type ScreenField struct {
	ID              int    `json:"id"`
	ScreenID        int    `json:"screen_id"`
	FieldType       string `json:"field_type"` // 'default' or 'custom'
	FieldIdentifier string `json:"field_identifier"`
	DisplayOrder    int    `json:"display_order"`
	IsRequired      bool   `json:"is_required"`
	FieldWidth      string `json:"field_width"`
	// Joined/computed fields for API responses
	FieldName   string                 `json:"field_name,omitempty"`
	FieldLabel  string                 `json:"field_label,omitempty"`
	FieldConfig map[string]interface{} `json:"field_config,omitempty"`
}

type ConfigurationSet struct {
	ID                      int       `json:"id"`
	WorkspaceID             int       `json:"workspace_id"` // Keep for backward compatibility
	Name                    string    `json:"name"`
	Description             string    `json:"description"`
	IsDefault               bool      `json:"is_default"`
	DifferentiateByItemType bool      `json:"differentiate_by_item_type"`
	WorkflowID              *int      `json:"workflow_id,omitempty"`
	NotificationSettingID   *int      `json:"notification_setting_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	// Joined fields for API responses
	WorkspaceName           string `json:"workspace_name,omitempty"`
	WorkflowName            string `json:"workflow_name,omitempty"`
	NotificationSettingName string `json:"notification_setting_name,omitempty"`
	// Many-to-many workspace relationships
	WorkspaceIDs []int    `json:"workspace_ids,omitempty"`
	Workspaces   []string `json:"workspaces,omitempty"` // Workspace names for display
	// Item types associated with this configuration set
	ItemTypes         []string          `json:"item_types,omitempty"`          // Item type names for display (deprecated, use ItemTypesDetailed)
	ItemTypesDetailed []ItemTypeDisplay `json:"item_types_detailed,omitempty"` // Full item type data with icons and colors (deprecated, use ItemTypeConfigs)
	ItemTypeConfigs   []ItemTypeConfig  `json:"item_type_configs,omitempty"`   // Item type configurations with optional workflow and screen overrides
	// Priorities associated with this configuration set
	PriorityIDs        []int             `json:"priority_ids,omitempty"`        // IDs of associated priorities
	Priorities         []string          `json:"priorities,omitempty"`          // Priority names for display (deprecated, use PrioritiesDetailed)
	PrioritiesDetailed []PriorityDisplay `json:"priorities_detailed,omitempty"` // Full priority data with icons and colors
	// Screen assignments for different contexts
	CreateScreenID   *int   `json:"create_screen_id,omitempty"`
	EditScreenID     *int   `json:"edit_screen_id,omitempty"`
	ViewScreenID     *int   `json:"view_screen_id,omitempty"`
	CreateScreenName string `json:"create_screen_name,omitempty"`
	EditScreenName   string `json:"edit_screen_name,omitempty"`
	ViewScreenName   string `json:"view_screen_name,omitempty"`
	// Default item type for new items (when user has no localStorage preference)
	DefaultItemTypeID   *int   `json:"default_item_type_id,omitempty"`
	DefaultItemTypeName string `json:"default_item_type_name,omitempty"`
}

type WorkspaceConfigurationSet struct {
	ID                 int       `json:"id"`
	WorkspaceID        int       `json:"workspace_id"`
	ConfigurationSetID int       `json:"configuration_set_id"`
	CreatedAt          time.Time `json:"created_at"`
	// Joined fields for API responses
	WorkspaceName        string `json:"workspace_name,omitempty"`
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`
}

type ConfigurationSetScreen struct {
	ID                 int       `json:"id"`
	ConfigurationSetID int       `json:"configuration_set_id"`
	ScreenID           int       `json:"screen_id"`
	Context            string    `json:"context"` // create, edit, view
	CreatedAt          time.Time `json:"created_at"`
	// Joined fields for API responses
	ScreenName           string `json:"screen_name,omitempty"`
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`
}

type ItemType struct {
	ID                 int       `json:"id"`
	ConfigurationSetID int       `json:"configuration_set_id,omitempty"` // Deprecated: kept for backward compatibility
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	IsDefault          bool      `json:"is_default"`
	Icon               string    `json:"icon"`            // Lucide icon name
	Color              string    `json:"color"`           // Hex color for background
	HierarchyLevel     int       `json:"hierarchy_level"` // 0=top level, 1=level 1, etc.
	SortOrder          int       `json:"sort_order"`      // For ordering within same level
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// Many-to-many configuration set relationships
	ConfigurationSetIDs   []int    `json:"configuration_set_ids,omitempty"`   // IDs of associated configuration sets
	ConfigurationSetNames []string `json:"configuration_set_names,omitempty"` // Names for display
	// Deprecated joined fields (kept for backward compatibility)
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`
	WorkspaceName        string `json:"workspace_name,omitempty"`
}

// ItemTypeDisplay holds minimal item type data for displaying in configuration sets
type ItemTypeDisplay struct {
	Name           string `json:"name"`
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	HierarchyLevel int    `json:"hierarchy_level"`
}

// ItemTypeConfig represents item type configuration with optional workflow and screen overrides
type ItemTypeConfig struct {
	ItemTypeID     int    `json:"item_type_id"`
	ItemTypeName   string `json:"item_type_name"`
	ItemTypeIcon   string `json:"item_type_icon"`
	ItemTypeColor  string `json:"item_type_color"`
	HierarchyLevel int    `json:"hierarchy_level"`
	// Override workflow (NULL = use configuration set default)
	WorkflowID   *int   `json:"workflow_id,omitempty"`
	WorkflowName string `json:"workflow_name,omitempty"` // "Default" or workflow name
	// Override screens (NULL = use configuration set defaults)
	CreateScreenID   *int   `json:"create_screen_id,omitempty"`
	CreateScreenName string `json:"create_screen_name,omitempty"`
	EditScreenID     *int   `json:"edit_screen_id,omitempty"`
	EditScreenName   string `json:"edit_screen_name,omitempty"`
	ViewScreenID     *int   `json:"view_screen_id,omitempty"`
	ViewScreenName   string `json:"view_screen_name,omitempty"`
}

type Priority struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	Icon        string    `json:"icon"`       // Lucide icon name
	Color       string    `json:"color"`      // Hex color for background
	SortOrder   int       `json:"sort_order"` // For ordering priorities
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Many-to-many configuration set relationships
	ConfigurationSetIDs   []int    `json:"configuration_set_ids,omitempty"`   // IDs of associated configuration sets
	ConfigurationSetNames []string `json:"configuration_set_names,omitempty"` // Names for display
}

// PriorityDisplay holds minimal priority data for displaying in configuration sets
type PriorityDisplay struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

type HierarchyLevel struct {
	ID          int       `json:"id"`
	Level       int       `json:"level"` // 0, 1, 2, 3...
	Name        string    `json:"name"`  // e.g., "Initiative", "Epic", "Task", "Sub-task"
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type WorkspaceScreen struct {
	ID          int    `json:"id"`
	WorkspaceID int    `json:"workspace_id"`
	ScreenID    int    `json:"screen_id"`
	Context     string `json:"context"` // create, edit, view
	// Joined fields for API responses
	ScreenName    string `json:"screen_name,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

// Workflow System Models

type StatusCategory struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // Hex color code (e.g., "#3b82f6")
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	IsCompleted bool      `json:"is_completed"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Status struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CategoryID  int       `json:"category_id"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	IsCompleted   bool   `json:"is_completed,omitempty"`
}

type Workflow struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	Transitions []WorkflowTransition `json:"transitions,omitempty"`
}

type WorkflowTransition struct {
	ID           int       `json:"id"`
	WorkflowID   int       `json:"workflow_id"`
	FromStatusID *int      `json:"from_status_id"` // NULL means it's an initial status
	ToStatusID   int       `json:"to_status_id"`
	DisplayOrder int       `json:"display_order"`
	SourceHandle string    `json:"source_handle,omitempty"` // Connection point on source status (top, right, bottom, left)
	TargetHandle string    `json:"target_handle,omitempty"` // Connection point on target status (top, right, bottom, left)
	CreatedAt    time.Time `json:"created_at"`
	// Joined fields for API responses
	FromStatusName string `json:"from_status_name,omitempty"`
	ToStatusName   string `json:"to_status_name,omitempty"`
	WorkflowName   string `json:"workflow_name,omitempty"`
}

type UserAppToken struct {
	ID          int        `json:"id"`
	UserID      int        `json:"user_id"`
	TokenName   string     `json:"token_name"`
	TokenHash   string     `json:"-"` // Never send hash to client
	TokenPrefix string     `json:"token_prefix"`
	Scopes      string     `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at"`
	IsActive    bool       `json:"is_active"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Milestone System Models

type MilestoneCategory struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // Hex color code (e.g., "#3b82f6")
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Milestone struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	TargetDate  *string   `json:"target_date,omitempty"` // Date in YYYY-MM-DD format, nullable
	Status      string    `json:"status"`                // planning, in-progress, completed, cancelled
	CategoryID  *int      `json:"category_id,omitempty"`
	IsGlobal    bool      `json:"is_global"`              // false=local to workspace, true=global (shared)
	WorkspaceID *int      `json:"workspace_id,omitempty"` // NULL if global, set if local to workspace
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

// Iteration System Models

type IterationType struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // Hex color code (e.g., "#3b82f6")
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Iteration struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	StartDate   string    `json:"start_date"` // YYYY-MM-DD format
	EndDate     string    `json:"end_date"`   // YYYY-MM-DD format
	Status      string    `json:"status"`     // planned, active, completed, cancelled
	TypeID      *int      `json:"type_id,omitempty"`
	IsGlobal    bool      `json:"is_global"`              // false=local to workspace, true=global (shared)
	WorkspaceID *int      `json:"workspace_id,omitempty"` // NULL if global, set if local to workspace
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	TypeName      string `json:"type_name,omitempty"`
	TypeColor     string `json:"type_color,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

// Attachment System Models

type Attachment struct {
	ID               int       `json:"id"`
	ItemID           *int      `json:"item_id,omitempty"`
	Filename         string    `json:"filename"`          // Stored filename (UUID-based)
	OriginalFilename string    `json:"original_filename"` // Original user filename
	FilePath         string    `json:"-"`                 // Full file path, not sent to client
	MimeType         string    `json:"mime_type"`
	FileSize         int64     `json:"file_size"`
	UploadedBy       *int      `json:"uploaded_by,omitempty"`
	HasThumbnail     bool      `json:"has_thumbnail"` // Whether thumbnail was generated
	ThumbnailPath    string    `json:"-"`             // Thumbnail file path, not sent to client
	CreatedAt        time.Time `json:"created_at"`
	// Joined fields for API responses
	UploaderName  string `json:"uploader_name,omitempty"`
	UploaderEmail string `json:"uploader_email,omitempty"`
}

type AttachmentSettings struct {
	ID               int       `json:"id"`
	MaxFileSize      int64     `json:"max_file_size"`      // Maximum file size in bytes
	AllowedMimeTypes string    `json:"allowed_mime_types"` // JSON array of allowed MIME types
	AttachmentPath   string    `json:"attachment_path"`    // Base path for storing attachments
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Diagram System Models

type ItemDiagram struct {
	ID          int       `json:"id"`
	ItemID      int       `json:"item_id"`
	Name        string    `json:"name"`
	DiagramData string    `json:"diagram_data"` // JSON with elements, appState, files
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	UpdatedBy   *int      `json:"updated_by,omitempty"`
	// Joined fields for API responses
	CreatorName    string `json:"creator_name,omitempty"`
	CreatorEmail   string `json:"creator_email,omitempty"`
	UpdatedByName  string `json:"updated_by_name,omitempty"`
	UpdatedByEmail string `json:"updated_by_email,omitempty"`
}

// Request/Response DTOs for attachments

type AttachmentUploadResponse struct {
	Success    bool       `json:"success"`
	Message    string     `json:"message"`
	Attachment Attachment `json:"attachment,omitempty"`
}

type AttachmentSettingsRequest struct {
	MaxFileSize      int64    `json:"max_file_size"`
	AllowedMimeTypes []string `json:"allowed_mime_types"`
	Enabled          bool     `json:"enabled"`
}

// Pagination Models

type PaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type PaginatedAttachmentsResponse struct {
	Attachments []Attachment   `json:"attachments"`
	Pagination  PaginationMeta `json:"pagination"`
}

type PaginatedItemsResponse struct {
	Items      []Item         `json:"items"`
	Pagination PaginationMeta `json:"pagination"`
}

type PaginatedConfigurationSetsResponse struct {
	ConfigurationSets []ConfigurationSet `json:"configuration_sets"`
	Pagination        PaginationMeta     `json:"pagination"`
}

// Time tracking models
// CustomerOrganisation represents a B2B entity for time tracking
type CustomerOrganisation struct {
	ID                int                    `json:"id"`
	Name              string                 `json:"name"`
	Email             string                 `json:"email"`
	Description       string                 `json:"description"`
	Active            bool                   `json:"active"`
	AvatarURL         string                 `json:"avatar_url,omitempty"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// ContactRole represents a role that can be assigned to portal customers
type ContactRole struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
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

// PortalCustomer represents an individual portal user
type PortalCustomer struct {
	ID                     int                    `json:"id"`
	Name                   string                 `json:"name"`
	Email                  string                 `json:"email"`
	Phone                  string                 `json:"phone,omitempty"`
	UserID                 *int                   `json:"user_id,omitempty"`                  // Links to internal user if applicable
	CustomerOrganisationID *int                   `json:"customer_organisation_id,omitempty"` // Links to B2B organisation
	IsPrimary              bool                   `json:"is_primary"`                         // Primary contact for the organization
	CustomFieldValues      map[string]interface{} `json:"custom_field_values,omitempty"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
	// Joined fields for API responses
	UserName                 string        `json:"user_name,omitempty"`
	UserEmail                string        `json:"user_email,omitempty"`
	CustomerOrganisationName string        `json:"customer_organisation_name,omitempty"`
	Roles                    []ContactRole `json:"roles,omitempty"` // Contact roles assigned to this customer
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

type TimeProjectCategory struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Color        string    `json:"color,omitempty"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TimeProject struct {
	ID          int       `json:"id"`
	CustomerID  *int      `json:"customer_id,omitempty"` // Now optional
	CategoryID  *int      `json:"category_id,omitempty"` // Link to project category
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // Active, On Hold, Completed, Archived
	Color       string    `json:"color,omitempty"`
	HourlyRate  float64   `json:"hourly_rate"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	CustomerName  string `json:"customer_name,omitempty"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
}

type Worklog struct {
	ID           int    `json:"id"`
	ProjectID    int    `json:"project_id"`
	CustomerID   int    `json:"customer_id"`
	ItemID       *int   `json:"item_id,omitempty"` // Optional link to work item
	Description  string `json:"description"`
	Date         int64  `json:"date"`       // Unix timestamp
	StartTime    int64  `json:"start_time"` // Unix timestamp
	EndTime      int64  `json:"end_time"`   // Unix timestamp
	DurationMins int    `json:"duration_minutes"`
	CreatedAt    int64  `json:"created_at"` // Unix timestamp
	UpdatedAt    int64  `json:"updated_at"` // Unix timestamp
	// Joined fields for API responses
	CustomerName        string `json:"customer_name,omitempty"`
	ProjectName         string `json:"project_name,omitempty"`
	ItemTitle           string `json:"item_title,omitempty"`            // Title of linked work item
	WorkspaceID         *int   `json:"workspace_id,omitempty"`          // Workspace ID of linked item
	WorkspaceKey        string `json:"workspace_key,omitempty"`         // Workspace key for navigation (e.g., "TEST")
	WorkspaceItemNumber int    `json:"workspace_item_number,omitempty"` // Item number for display key (e.g., "TEST-123")
}

// Active Timer Model
type ActiveTimer struct {
	ID           int    `json:"id"`
	WorkspaceID  int    `json:"workspace_id"`
	ItemID       *int   `json:"item_id,omitempty"` // Optional link to work item
	ProjectID    int    `json:"project_id"`
	Description  string `json:"description"`
	StartTimeUTC int64  `json:"start_time_utc"` // Unix timestamp in UTC
	CreatedAt    int64  `json:"created_at"`     // Unix timestamp
	// Joined fields for API responses - nullable since they come from LEFT JOINs
	ProjectName   *string `json:"project_name,omitempty"`
	CustomerName  *string `json:"customer_name,omitempty"`
	ItemTitle     *string `json:"item_title,omitempty"`
	WorkspaceName *string `json:"workspace_name,omitempty"`
	WorkspaceKey  *string `json:"workspace_key,omitempty"`
}

// Test Management Models

type TestFolder struct {
	ID          int       `json:"id"`
	WorkspaceID int       `json:"workspace_id"`
	ParentID    *int      `json:"parent_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Computed fields for API responses
	TestCaseCount int `json:"test_case_count,omitempty"`
}

type TestCase struct {
	ID                int       `json:"id"`
	WorkspaceID       int       `json:"workspace_id"`
	FolderID          *int      `json:"folder_id,omitempty"`
	Title             string    `json:"title"`
	Name              string    `json:"name"`
	Priority          string    `json:"priority"`           // low, medium, high, critical
	Status            string    `json:"status"`             // active, inactive, draft
	EstimatedDuration int       `json:"estimated_duration"` // in seconds
	Preconditions     string    `json:"preconditions"`
	SortOrder         int       `json:"sort_order"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	// Computed fields for API responses
	FolderName string      `json:"folder_name,omitempty"`
	TestSteps  []TestStep  `json:"test_steps,omitempty"`
	Labels     []TestLabel `json:"labels,omitempty"`
}

type TestSet struct {
	ID            int       `json:"id"`
	WorkspaceID   int       `json:"workspace_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	MilestoneID   *int      `json:"milestone_id"`
	MilestoneName string    `json:"milestone_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Test case count
	TestCaseCount int `json:"test_case_count,omitempty"`

	// Test run statistics
	TotalRuns      int        `json:"total_runs,omitempty"`
	SuccessfulRuns int        `json:"successful_runs,omitempty"`
	FailedRuns     int        `json:"failed_runs,omitempty"`
	LastRunStatus  string     `json:"last_run_status,omitempty"`
	LastRunDate    *time.Time `json:"last_run_date,omitempty"`
}

type SetTestCase struct {
	ID         int `json:"id"`
	SetID      int `json:"set_id"`
	TestCaseID int `json:"test_case_id"`
}

type TestRunTemplate struct {
	ID          int       `json:"id"`
	WorkspaceID int       `json:"workspace_id"`
	SetID       int       `json:"set_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	SetName string `json:"set_name,omitempty"`
}

type TestRun struct {
	ID          int        `json:"id"`
	WorkspaceID int        `json:"workspace_id"`
	TemplateID  int        `json:"template_id,omitempty"` // Optional reference to template
	SetID       int        `json:"set_id"`
	Name        string     `json:"name"`
	AssigneeID  *int       `json:"assignee_id,omitempty"` // User assigned to execute this run
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at"`
	CreatedAt   time.Time  `json:"created_at"`
	// Computed fields from JOIN with users table
	AssigneeName   string `json:"assignee_name,omitempty"`
	AssigneeEmail  string `json:"assignee_email,omitempty"`
	AssigneeAvatar string `json:"assignee_avatar,omitempty"`
}

type TestResult struct {
	ID           int        `json:"id"`
	RunID        int        `json:"run_id"`
	TestCaseID   int        `json:"test_case_id"`
	Status       string     `json:"status"` // "passed", "failed", "blocked", "skipped", "not_run"
	ActualResult string     `json:"actual_result"`
	Notes        string     `json:"notes"`
	ExecutedAt   *time.Time `json:"executed_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Test Step Models for individual step-by-step execution
type TestStep struct {
	ID         int       `json:"id"`
	TestCaseID int       `json:"test_case_id"`
	StepNumber int       `json:"step_number"`
	Action     string    `json:"action"`
	Data       string    `json:"data"`
	Expected   string    `json:"expected"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Test Step Results for execution tracking
type TestStepResult struct {
	ID           int        `json:"id"`
	TestResultID int        `json:"test_result_id"`
	TestStepID   int        `json:"test_step_id"`
	Status       string     `json:"status"` // "passed", "failed", "blocked", "skipped"
	ActualResult string     `json:"actual_result"`
	Notes        string     `json:"notes"`
	ItemID       *int       `json:"item_id,omitempty"` // Link to work item (e.g., Bug)
	ExecutedAt   *time.Time `json:"executed_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// Test Result Item junction table for linking test results to work items
type TestResultItem struct {
	ID           int       `json:"id"`
	TestResultID int       `json:"test_result_id"`
	ItemID       int       `json:"item_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// Test Label Models for organizing and categorizing test cases
type TestLabel struct {
	ID          int       `json:"id"`
	WorkspaceID int       `json:"workspace_id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TestCaseLabel struct {
	ID         int       `json:"id"`
	TestCaseID int       `json:"test_case_id"`
	LabelID    int       `json:"label_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// Personal Label Models for organizing personal tasks
type PersonalLabel struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	UserID    *int      `json:"user_id,omitempty"` // NULL for global labels, user_id for user-specific labels
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Link Management Models

type LinkType struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	ForwardLabel string    `json:"forward_label"`
	ReverseLabel string    `json:"reverse_label"`
	Color        string    `json:"color"`
	IsSystem     bool      `json:"is_system"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ItemLink struct {
	ID         int       `json:"id"`
	LinkTypeID int       `json:"link_type_id"`
	SourceType string    `json:"source_type"` // "item" or "test_case"
	SourceID   int       `json:"source_id"`
	TargetType string    `json:"target_type"` // "item" or "test_case"
	TargetID   int       `json:"target_id"`
	CreatedBy  *int      `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	// Joined fields for API responses
	LinkTypeName         string `json:"link_type_name,omitempty"`
	LinkTypeColor        string `json:"link_type_color,omitempty"`
	LinkTypeForwardLabel string `json:"link_type_forward_label,omitempty"`
	LinkTypeReverseLabel string `json:"link_type_reverse_label,omitempty"`
	SourceTitle          string `json:"source_title,omitempty"`
	TargetTitle          string `json:"target_title,omitempty"`
	CreatedByName        string `json:"created_by_name,omitempty"`
	// Source item details
	SourceStatusID      *int   `json:"source_status_id,omitempty"`
	SourceStatusName    string `json:"source_status_name,omitempty"`
	SourceItemTypeID    *int   `json:"source_item_type_id,omitempty"`
	SourceItemTypeName  string `json:"source_item_type_name,omitempty"`
	SourceItemTypeIcon  string `json:"source_item_type_icon,omitempty"`
	SourceItemTypeColor string `json:"source_item_type_color,omitempty"`
	SourceWorkspaceKey  string `json:"source_workspace_key,omitempty"`
	// Target item details
	TargetStatusID      *int   `json:"target_status_id,omitempty"`
	TargetStatusName    string `json:"target_status_name,omitempty"`
	TargetItemTypeID    *int   `json:"target_item_type_id,omitempty"`
	TargetItemTypeName  string `json:"target_item_type_name,omitempty"`
	TargetItemTypeIcon  string `json:"target_item_type_icon,omitempty"`
	TargetItemTypeColor string `json:"target_item_type_color,omitempty"`
	TargetWorkspaceKey  string `json:"target_workspace_key,omitempty"`
}

// LinkableItem represents an item that can be linked (work item, test case, or asset)
type LinkableItem struct {
	ID          int    `json:"id"`
	Type        string `json:"type"` // "item", "test_case", or "asset"
	Title       string `json:"title"`
	Description string `json:"description"`
	// For work items
	WorkspaceID   *int   `json:"workspace_id,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
	Status        string `json:"status,omitempty"`
	Priority      string `json:"priority,omitempty"`
	// For assets
	AssetSetID       *int   `json:"asset_set_id,omitempty"`
	AssetSetName     string `json:"asset_set_name,omitempty"`
	AssetTypeName    string `json:"asset_type_name,omitempty"`
	AssetCategoryName string `json:"asset_category_name,omitempty"`
}

// SystemSetting represents a system configuration setting
type SystemSetting struct {
	ID          int       `json:"id"`
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	ValueType   string    `json:"value_type"` // string, boolean, integer, json
	Description string    `json:"description"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SetupStatus represents the system setup status
type SetupStatus struct {
	SetupCompleted        bool `json:"setup_completed"`
	AdminUserCreated      bool `json:"admin_user_created"`
	TimeTrackingEnabled   bool `json:"time_tracking_enabled"`
	TestManagementEnabled bool `json:"test_management_enabled"`
}

// SetupRequest represents the initial setup configuration
type SetupRequest struct {
	AdminUser      SetupUser      `json:"admin_user"`
	ModuleSettings ModuleSettings `json:"module_settings"`
}

// SetupUser represents a user for initial setup (includes password)
type SetupUser struct {
	Email        string `json:"email"`
	Username     string `json:"username"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	PasswordHash string `json:"password_hash"` // For setup only, will be hashed server-side
}

// ModuleSettings represents module visibility settings
type ModuleSettings struct {
	TimeTrackingEnabled   bool `json:"time_tracking_enabled"`
	TestManagementEnabled bool `json:"test_management_enabled"`
}

// API Token Models for bearer token authentication
type ApiToken struct {
	ID          int        `json:"id"`
	UserID      int        `json:"user_id"`
	Name        string     `json:"name"`         // Human-readable name for the token
	Token       string     `json:"-"`            // The actual token (hashed in DB, not sent to client)
	TokenPrefix string     `json:"token_prefix"` // First few characters for identification
	Permissions string     `json:"permissions"`  // JSON array of permissions
	IsTemporary bool       `json:"is_temporary"` // True for SSH session tokens
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// Joined fields
	UserEmail string `json:"user_email,omitempty"`
	UserName  string `json:"user_name,omitempty"`
}

type ApiTokenCreate struct {
	Name        string     `json:"name"`
	UserID      *int       `json:"user_id,omitempty"` // Optional: admins can create tokens for other users
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type ApiTokenResponse struct {
	Token    string   `json:"token"`     // Only returned on creation
	ApiToken ApiToken `json:"api_token"` // Token metadata
}

// Notification represents a system notification for a user
type Notification struct {
	ID        int        `json:"id"`
	UserID    int        `json:"user_id"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	Type      string     `json:"type"` // info, warning, error, success, assignment, comment, status_change, reminder, milestone
	Timestamp time.Time  `json:"timestamp"`
	Read      bool       `json:"read"`
	SentAt    *time.Time `json:"sent_at,omitempty"`    // When notification was sent via email (NULL if not sent)
	Avatar    string     `json:"avatar,omitempty"`     // Initials or avatar identifier
	ActionURL string     `json:"action_url,omitempty"` // URL to navigate to when clicked
	Metadata  string     `json:"metadata,omitempty"`   // JSON for additional data
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// NotificationCache represents a cached notification for BigCache storage
type NotificationCache struct {
	Notifications []Notification `json:"notifications"`
	LastSynced    time.Time      `json:"last_synced"`
	IsDirty       bool           `json:"is_dirty"` // Indicates if cache needs DB sync
}

// NotificationTemplate represents a customizable notification email template
type NotificationTemplate struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	TemplateType string    `json:"template_type"` // 'header', 'footer', 'notification_type'
	Subject      string    `json:"subject,omitempty"`
	Content      string    `json:"content"`
	Description  string    `json:"description,omitempty"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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

// Channel represents an integration channel (inbound/outbound)
type Channel struct {
	ID              int        `json:"id"`
	Name            string     `json:"name"`
	Type            string     `json:"type"`      // smtp, webhook, imap, portal, widget
	Direction       string     `json:"direction"` // inbound, outbound
	Description     string     `json:"description"`
	Status          string     `json:"status"`     // enabled, disabled
	IsDefault       bool       `json:"is_default"` // Default channel for its type
	Config          string     `json:"config"`     // JSON configuration data
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
}

// ChannelConfig represents configuration for different channel types
type ChannelConfig struct {
	// SMTP Configuration
	SMTPHost       string `json:"smtp_host,omitempty"`
	SMTPPort       int    `json:"smtp_port,omitempty"`
	SMTPUsername   string `json:"smtp_username,omitempty"`
	SMTPPassword   string `json:"smtp_password,omitempty"`
	SMTPFromEmail  string `json:"smtp_from_email,omitempty"`
	SMTPFromName   string `json:"smtp_from_name,omitempty"`
	SMTPEncryption string `json:"smtp_encryption,omitempty"` // tls, ssl, none

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
	EmailProviderID         *int       `json:"email_provider_id,omitempty"`          // Link to email_providers table (legacy)
	EmailAuthMethod         string     `json:"email_auth_method,omitempty"`          // 'oauth' or 'basic'

	// Inline OAuth App Credentials (per-channel)
	EmailOAuthProviderType  string     `json:"email_oauth_provider_type,omitempty"`  // 'microsoft' or 'google'
	EmailOAuthClientID      string     `json:"email_oauth_client_id,omitempty"`      // OAuth app client ID
	EmailOAuthClientSecret  string     `json:"email_oauth_client_secret,omitempty"`  // Encrypted client secret
	EmailOAuthTenantID      string     `json:"email_oauth_tenant_id,omitempty"`      // Microsoft tenant ID (or 'common')

	// OAuth Tokens (populated after successful OAuth flow)
	EmailOAuthAccessToken   string     `json:"email_oauth_access_token,omitempty"`   // Encrypted OAuth access token
	EmailOAuthRefreshToken  string     `json:"email_oauth_refresh_token,omitempty"`  // Encrypted OAuth refresh token
	EmailOAuthExpiresAt     *time.Time `json:"email_oauth_expires_at,omitempty"`     // Token expiration time
	EmailOAuthEmail         string     `json:"email_oauth_email,omitempty"`          // Connected email address
	EmailWorkspaceID        int        `json:"email_workspace_id,omitempty"`         // Target workspace for items
	EmailItemTypeID         *int       `json:"email_item_type_id,omitempty"`         // Item type to create
	EmailDefaultPriorityID  *int       `json:"email_default_priority_id,omitempty"`  // Default priority for items
	EmailPollInterval       int        `json:"email_poll_interval,omitempty"`        // Poll interval in minutes (default 5)
	EmailMailbox            string     `json:"email_mailbox,omitempty"`              // IMAP mailbox (default "INBOX")
	EmailMarkAsRead         bool       `json:"email_mark_as_read,omitempty"`         // Mark processed emails as read
	EmailDeleteAfterProcess bool       `json:"email_delete_after_process,omitempty"` // Delete emails after processing
	EmailConnectedPortalID  *int       `json:"email_connected_portal_id,omitempty"`  // Portal for "My Requests" visibility

	// Portal Configuration
	PortalSlug         string `json:"portal_slug,omitempty"`        // URL-friendly identifier (e.g., "support-portal")
	PortalWorkspaceIDs []int  `json:"portal_workspace_ids"`         // Target workspaces for submissions
	PortalEnabled      bool   `json:"portal_enabled,omitempty"`     // Enable/disable portal
	PortalTitle        string `json:"portal_title,omitempty"`       // Display title for portal
	PortalDescription  string `json:"portal_description,omitempty"` // Description shown on portal page

	// Portal Customization
	PortalGradient          int    `json:"portal_gradient,omitempty"`           // Selected gradient index (0-17)
	PortalTheme             string `json:"portal_theme,omitempty"`              // Theme mode: "light" or "dark"
	PortalSearchPlaceholder string `json:"portal_search_placeholder,omitempty"` // Custom search placeholder text
	PortalSearchHint        string `json:"portal_search_hint,omitempty"`        // Custom search hint text
	PortalFooterColumns     []struct {
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
	IMAPEncryption string `json:"imap_encryption,omitempty"` // ssl, tls, none
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
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	ErrorCount    int        `json:"error_count"`
	LastError     string     `json:"last_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// EmailMessageTracking records processed emails for deduplication and reply threading
type EmailMessageTracking struct {
	ID          int       `json:"id"`
	ChannelID   int       `json:"channel_id"`
	MessageID   string    `json:"message_id"`   // RFC 5322 Message-ID header
	InReplyTo   string    `json:"in_reply_to"`  // For reply threading
	FromEmail   string    `json:"from_email"`
	FromName    string    `json:"from_name,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	ItemID      *int      `json:"item_id,omitempty"`    // Created item (nil if comment)
	CommentID   *int      `json:"comment_id,omitempty"` // Created comment (nil if new item)
	ProcessedAt time.Time `json:"processed_at"`
}

// RequestType represents a portal request type that maps to an item type
type RequestType struct {
	ID           int       `json:"id"`
	ChannelID    int       `json:"channel_id"` // Scope request type to specific portal/channel
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	ItemTypeID   int       `json:"item_type_id"`  // n:1 relationship - which item type submissions create
	Icon         string    `json:"icon"`          // Lucide icon name for visual representation
	Color        string    `json:"color"`         // Hex color for visual representation
	DisplayOrder int       `json:"display_order"` // Ordering within channel
	IsActive     bool      `json:"is_active"`     // Enable/disable this request type
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Joined fields for API responses
	ChannelName  string `json:"channel_name,omitempty"`
	ItemTypeName string `json:"item_type_name,omitempty"`
}

// RequestTypeField represents a field configuration for a request type
type RequestTypeField struct {
	ID              int       `json:"id"`
	RequestTypeID   int       `json:"request_type_id"`
	FieldIdentifier string    `json:"field_identifier"` // Field identifier (e.g., "title", "description", custom field ID, or virtual field ID)
	FieldType       string    `json:"field_type"`       // 'default', 'custom', or 'virtual'
	DisplayOrder    int       `json:"display_order"`    // Order in form
	IsRequired      bool      `json:"is_required"`      // Whether field is required
	// Display customization for portal
	DisplayName *string `json:"display_name,omitempty"` // Override label shown in portal
	Description *string `json:"description,omitempty"`  // Help text shown below field
	// Multi-step form support
	StepNumber int `json:"step_number"` // Which step this field appears on (default 1)
	// Virtual field support (only for field_type = 'virtual')
	VirtualFieldType    *string `json:"virtual_field_type,omitempty"`    // 'text', 'textarea', 'select', 'checkbox'
	VirtualFieldOptions *string `json:"virtual_field_options,omitempty"` // JSON array for select options
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	// Joined/computed fields for API responses
	FieldName  string `json:"field_name,omitempty"`
	FieldLabel string `json:"field_label,omitempty"` // Uses display_name if set, otherwise field_name
}

// UserPermissionCache represents a user's complete cached permission set
type UserPermissionCache struct {
	UserID               int                       `json:"user_id"`
	IsSystemAdmin        bool                      `json:"is_system_admin"`
	GlobalPermissions    map[string]bool           `json:"global_permissions"`    // permission_key -> has_permission
	WorkspacePermissions map[int]map[string]bool   `json:"workspace_permissions"` // workspace_id -> permission_key -> has_permission
	WorkspaceEveryone    map[int]map[string]bool   `json:"workspace_everyone"`    // workspace_id -> permission_key -> has_permission (applies to all users)
	GroupMemberships     []int                     `json:"group_memberships"`     // group_ids
	RoleAssignments      map[int][]int             `json:"role_assignments"`      // workspace_id -> role_ids
	DirectPermissions    map[int][]string          `json:"direct_permissions"`    // workspace_id -> permission_keys (direct assignments)
	PermissionSources    map[int]map[string]string `json:"permission_sources"`    // workspace_id -> permission_key -> source (role/direct/group)
	ItemWorkspaceMap     map[int]int               `json:"item_workspace_map"`    // item_id -> workspace_id (lazy-loaded on demand)
	CachedAt             time.Time                 `json:"cached_at"`
	ExpiresAt            time.Time                 `json:"expires_at"`
}

// CacheStats represents permission cache performance metrics
type CacheStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	Errors      int64   `json:"errors"`
	HitRatio    float64 `json:"hit_ratio"`
	AvgLoadTime int64   `json:"avg_load_time_ms"`
	TotalUsers  int64   `json:"total_cached_users"`
}

// NotificationSetting represents a notification configuration that can be assigned to configuration sets
type NotificationSetting struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`        // e.g., "Development Team Notifications"
	Description string    `json:"description"` // e.g., "Standard notifications for development workspaces"
	IsActive    bool      `json:"is_active"`
	CreatedBy   int       `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	CustomRecipients      string    `json:"custom_recipients"` // JSON array of user IDs or email addresses
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
	}
}

// Workflow Migration Models

type StatusMigrationInfo struct {
	CurrentStatus       string `json:"current_status"`
	CurrentStatusID     *int   `json:"current_status_id"`
	ItemTypeID          *int   `json:"item_type_id,omitempty"`
	ItemTypeName        string `json:"item_type_name,omitempty"`
	RequiresMigration   bool   `json:"requires_migration"`
	SuggestedStatusID   *int   `json:"suggested_status_id"`
	SuggestedStatusName string `json:"suggested_status_name"`
	ItemCount           int    `json:"item_count"`
}

type WorkflowMigrationAnalysis struct {
	OldWorkflowID      *int                  `json:"old_workflow_id"`
	OldWorkflowName    string                `json:"old_workflow_name"`
	NewWorkflowID      *int                  `json:"new_workflow_id"`
	NewWorkflowName    string                `json:"new_workflow_name"`
	AffectedWorkspaces []int                 `json:"affected_workspaces"`
	StatusMigrations   []StatusMigrationInfo `json:"status_migrations"`
	RequiresMigration  bool                  `json:"requires_migration"`
	TotalAffectedItems int                   `json:"total_affected_items"`
}

type StatusMigrationMapping struct {
	FromStatus   string `json:"from_status"`
	FromStatusID int    `json:"from_status_id"`
	ToStatusID   int    `json:"to_status_id"`
	ItemTypeID   *int   `json:"item_type_id,omitempty"`
	ItemCount    int    `json:"item_count"`
}

type WorkflowMigrationRequest struct {
	ConfigurationSetID int                      `json:"configuration_set_id"`
	WorkspaceIDs       []int                    `json:"workspace_ids"`
	StatusMappings     []StatusMigrationMapping `json:"status_mappings"`
}

// Comprehensive Configuration Set Migration Models

// ItemTypeMigrationInfo describes an item type that needs migration
type ItemTypeMigrationInfo struct {
	CurrentItemTypeID     *int             `json:"current_item_type_id"`
	CurrentItemTypeName   string           `json:"current_item_type_name"`
	ItemCount             int              `json:"item_count"`
	RequiresMigration     bool             `json:"requires_migration"`
	SuggestedItemTypeID   *int             `json:"suggested_item_type_id,omitempty"`
	SuggestedItemTypeName string           `json:"suggested_item_type_name,omitempty"`
	AvailableTargets      []ItemTypeTarget `json:"available_targets,omitempty"`
}

// ItemTypeTarget represents an available target item type for migration
type ItemTypeTarget struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Icon           string `json:"icon"`
	Color          string `json:"color"`
	HierarchyLevel int    `json:"hierarchy_level"`
}

// CustomFieldMigrationInfo describes a custom field migration need
type CustomFieldMigrationInfo struct {
	FieldID         int    `json:"field_id"`
	FieldName       string `json:"field_name"`
	FieldType       string `json:"field_type"`
	ItemCount       int    `json:"item_count"`       // items with non-null value for this field
	Action          string `json:"action"`           // keep, orphan, add_default
	RequiresDefault bool   `json:"requires_default"` // new required field needs default value
}

// PriorityMigrationInfo describes a priority that needs migration
type PriorityMigrationInfo struct {
	CurrentPriorityID     *int   `json:"current_priority_id"`
	CurrentPriorityName   string `json:"current_priority_name"`
	ItemCount             int    `json:"item_count"`
	RequiresMigration     bool   `json:"requires_migration"`
	SuggestedPriorityID   *int   `json:"suggested_priority_id,omitempty"`
	SuggestedPriorityName string `json:"suggested_priority_name,omitempty"`
}

// PriorityTarget represents an available target priority for migration
type PriorityTarget struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

// ComprehensiveMigrationAnalysis is the full analysis response for config set migration
type ComprehensiveMigrationAnalysis struct {
	// Existing status migration fields (backward compatible)
	StatusMigrations []StatusMigrationInfo `json:"status_migrations"`
	NewWorkflowID    *int                  `json:"new_workflow_id"`
	NewWorkflowName  string                `json:"new_workflow_name"`

	// New dimensions
	ItemTypeMigrations    []ItemTypeMigrationInfo    `json:"item_type_migrations"`
	CustomFieldMigrations []CustomFieldMigrationInfo `json:"custom_field_migrations"`
	PriorityMigrations    []PriorityMigrationInfo    `json:"priority_migrations"`

	// Available targets for UI dropdowns
	AvailableItemTypes  []ItemTypeTarget `json:"available_item_types"`
	AvailablePriorities []PriorityTarget `json:"available_priorities"`

	// Context
	OldConfigSetID     int    `json:"old_config_set_id"`
	OldConfigSetName   string `json:"old_config_set_name"`
	NewConfigSetID     int    `json:"new_config_set_id"`
	NewConfigSetName   string `json:"new_config_set_name"`
	AffectedWorkspaces []int  `json:"affected_workspaces"`
	TotalAffectedItems int    `json:"total_affected_items"`

	// Flags
	RequiresMigration         bool `json:"requires_migration"`
	RequiresItemTypeMigration bool `json:"requires_item_type_migration"`
	RequiresFieldMigration    bool `json:"requires_field_migration"`
	RequiresStatusMigration   bool `json:"requires_status_migration"`
	RequiresPriorityMigration bool `json:"requires_priority_migration"`
}

// ItemTypeMigrationMapping maps old item type to new
type ItemTypeMigrationMapping struct {
	FromItemTypeID *int `json:"from_item_type_id"` // nil = items with no type
	ToItemTypeID   int  `json:"to_item_type_id"`
}

// CustomFieldMigrationMapping specifies how to handle a custom field
type CustomFieldMigrationMapping struct {
	FieldID      int         `json:"field_id"`
	Action       string      `json:"action"`                  // keep, orphan, add_default
	DefaultValue interface{} `json:"default_value,omitempty"` // for new required fields
}

// PriorityMigrationMapping maps old priority to new
type PriorityMigrationMapping struct {
	FromPriorityID *int `json:"from_priority_id"` // nil = items with no priority
	ToPriorityID   int  `json:"to_priority_id"`
}

// ComprehensiveMigrationRequest is the full migration execution request
type ComprehensiveMigrationRequest struct {
	OldConfigurationSetID int   `json:"old_configuration_set_id"`
	NewConfigurationSetID int   `json:"new_configuration_set_id"`
	WorkspaceIDs          []int `json:"workspace_ids"`

	StatusMappings      []StatusMigrationMapping      `json:"status_mappings"`
	ItemTypeMappings    []ItemTypeMigrationMapping    `json:"item_type_mappings"`
	CustomFieldMappings []CustomFieldMigrationMapping `json:"custom_field_mappings"`
	PriorityMappings    []PriorityMigrationMapping    `json:"priority_mappings"`
}

// Review System Models

// Review represents a daily or weekly personal review entry
type Review struct {
	ID         int       `json:"id"`
	UserID     int       `json:"user_id"`
	ReviewDate string    `json:"review_date"` // YYYY-MM-DD format
	ReviewType string    `json:"review_type"` // 'daily' or 'weekly'
	ReviewData string    `json:"review_data"` // JSON data - unstructured storage
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Joined fields for API responses
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
}

// ReviewCreateRequest represents the payload for creating a new review
type ReviewCreateRequest struct {
	ReviewDate string `json:"review_date"` // YYYY-MM-DD format
	ReviewType string `json:"review_type"` // 'daily' or 'weekly'
	ReviewData string `json:"review_data"` // JSON data
}

// ReviewUpdateRequest represents the payload for updating a review
type ReviewUpdateRequest struct {
	ReviewData string `json:"review_data"` // JSON data
}

// CompletedItemsRequest represents the query parameters for getting completed items
type CompletedItemsRequest struct {
	StartDate string `json:"start_date"` // YYYY-MM-DD format
	EndDate   string `json:"end_date"`   // YYYY-MM-DD format
	UserID    int    `json:"user_id"`    // Filter by assignee
}

// Group Management Models

// TeamGroup represents a user group for access control and organization
type TeamGroup struct {
	ID          int    `json:"id"`
	Name        string `json:"name"` // Group name (e.g., "Developers", "Managers")
	Description string `json:"description"`
	// LDAP sync fields
	LDAPDistinguishedName string     `json:"ldap_distinguished_name,omitempty"` // Full LDAP DN for sync
	LDAPCommonName        string     `json:"ldap_common_name,omitempty"`        // CN from LDAP
	LDAPSyncEnabled       bool       `json:"ldap_sync_enabled"`                 // Whether this group syncs from LDAP
	LDAPLastSyncAt        *time.Time `json:"ldap_last_sync_at,omitempty"`       // Last successful LDAP sync
	// Group metadata
	IsSystemGroup bool      `json:"is_system_group"`      // Whether this is a system-defined group
	IsActive      bool      `json:"is_active"`            // Whether the group is active
	CreatedBy     *int      `json:"created_by,omitempty"` // User who created the group
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	// Joined fields for API responses
	CreatedByName string            `json:"created_by_name,omitempty"`
	MemberCount   int               `json:"member_count,omitempty"` // Number of members in this group
	Members       []TeamGroupMember `json:"members,omitempty"`      // Group members (for detailed views)
	// SCIM fields
	SCIMExternalID string `json:"scim_external_id,omitempty"` // External ID from identity provider
	SCIMManaged    bool   `json:"scim_managed"`               // If true, group is managed via SCIM and cannot be edited locally
}

// TeamGroupMember represents a user's membership in a group
type TeamGroupMember struct {
	ID      int `json:"id"`
	GroupID int `json:"group_id"`
	UserID  int `json:"user_id"`
	// LDAP sync fields
	LDAPSyncEnabled bool       `json:"ldap_sync_enabled"`           // Whether this membership is managed by LDAP
	LDAPLastSyncAt  *time.Time `json:"ldap_last_sync_at,omitempty"` // Last LDAP sync for this membership
	// Membership metadata
	AddedBy     *int      `json:"added_by,omitempty"` // User who added this member (NULL for LDAP)
	AddedAt     time.Time `json:"added_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	SCIMManaged bool      `json:"scim_managed"` // Whether this membership is managed via SCIM
	// Joined fields for API responses
	UserEmail    string `json:"user_email,omitempty"`
	UserName     string `json:"user_name,omitempty"` // Full name (first + last)
	UserUsername string `json:"user_username,omitempty"`
	GroupName    string `json:"group_name,omitempty"`
	AddedByName  string `json:"added_by_name,omitempty"`
}

// TeamGroupCreateRequest represents the payload for creating a new group
type TeamGroupCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// TeamGroupUpdateRequest represents the payload for updating a group
type TeamGroupUpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
}

// TeamGroupMemberRequest represents the payload for adding/removing group members
type TeamGroupMemberRequest struct {
	UserIDs []int `json:"user_ids"`
}

// TeamGroupMembershipResponse represents a user's group memberships
type TeamGroupMembershipResponse struct {
	UserID int         `json:"user_id"`
	Groups []TeamGroup `json:"groups"`
}

// Theme Models

// Theme represents the application's visual theme settings
type Theme struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	IsActive    bool   `json:"is_active"`
	// Navigation bar theme properties for light mode
	NavBackgroundColorLight string `json:"nav_background_color_light"` // CSS color value (hex, rgb, etc.)
	NavTextColorLight       string `json:"nav_text_color_light"`       // CSS color value (hex, rgb, etc.)
	// Navigation bar theme properties for dark mode
	NavBackgroundColorDark string    `json:"nav_background_color_dark"` // CSS color value (hex, rgb, etc.)
	NavTextColorDark       string    `json:"nav_text_color_dark"`       // CSS color value (hex, rgb, etc.)
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// ThemeCreateRequest represents the payload for creating a new theme
type ThemeCreateRequest struct {
	Name                    string `json:"name"`
	Description             string `json:"description"`
	NavBackgroundColorLight string `json:"nav_background_color_light"`
	NavTextColorLight       string `json:"nav_text_color_light"`
	NavBackgroundColorDark  string `json:"nav_background_color_dark"`
	NavTextColorDark        string `json:"nav_text_color_dark"`
}

// ThemeUpdateRequest represents the payload for updating a theme
type ThemeUpdateRequest struct {
	Name                    string `json:"name"`
	Description             string `json:"description"`
	NavBackgroundColorLight string `json:"nav_background_color_light"`
	NavTextColorLight       string `json:"nav_text_color_light"`
	NavBackgroundColorDark  string `json:"nav_background_color_dark"`
	NavTextColorDark        string `json:"nav_text_color_dark"`
	IsActive                bool   `json:"is_active"`
}

// User Preferences Models

// UserPreferences represents user-specific preferences stored as JSON
type UserPreferences struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	Preferences string    `json:"preferences"` // JSON string for database storage
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserPreferencesData represents the parsed preferences JSON structure
type UserPreferencesData struct {
	ColorMode string `json:"color_mode,omitempty"` // "light", "dark", or "system"
	ThemeID   *int   `json:"theme_id,omitempty"`
}

// UserPreferencesRequest represents the API request for updating preferences
type UserPreferencesRequest struct {
	ColorMode string `json:"color_mode,omitempty"`
	ThemeID   *int   `json:"theme_id,omitempty"`
}

// UserPreferencesResponse represents the API response with resolved data
type UserPreferencesResponse struct {
	ColorMode string  `json:"color_mode"`
	ThemeID   *int    `json:"theme_id,omitempty"`
	Theme     *Theme  `json:"theme,omitempty"` // Resolved theme if theme_id is set
}

// Board Configuration Models

// BoardConfiguration represents a board layout configuration for a collection
type BoardConfiguration struct {
	ID               int       `json:"id"`
	CollectionID     *int      `json:"collection_id,omitempty"`
	WorkspaceID      *int      `json:"workspace_id,omitempty"`
	BacklogStatusIDs []int     `json:"backlog_status_ids,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Joined fields
	Columns []BoardColumn `json:"columns,omitempty"`
}

// BoardColumn represents a column in a board configuration
type BoardColumn struct {
	ID                   int       `json:"id"`
	BoardConfigurationID int       `json:"board_configuration_id"`
	Name                 string    `json:"name"`
	DisplayOrder         int       `json:"display_order"`
	WIPLimit             *int      `json:"wip_limit"`
	Color                string    `json:"color"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	// Joined fields
	StatusIDs []int `json:"status_ids,omitempty"`
}

// BoardColumnStatus represents the mapping between a board column and a status
type BoardColumnStatus struct {
	ID            int       `json:"id"`
	BoardColumnID int       `json:"board_column_id"`
	StatusID      int       `json:"status_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// BoardConfigurationRequest represents the payload for creating/updating a board configuration
type BoardConfigurationRequest struct {
	Columns          []BoardColumnRequest `json:"columns"`
	BacklogStatusIDs []int                `json:"backlog_status_ids,omitempty"`
}

// BoardColumnRequest represents the payload for a column in the board configuration
type BoardColumnRequest struct {
	ID           *int   `json:"id,omitempty"` // Null for new columns
	Name         string `json:"name"`
	DisplayOrder int    `json:"display_order"`
	WIPLimit     *int   `json:"wip_limit"`
	Color        string `json:"color"`
	StatusIDs    []int  `json:"status_ids"`
}

// SCM Provider Models

// SCMProviderType represents the type of SCM provider
type SCMProviderType string

const (
	SCMProviderTypeGitHub SCMProviderType = "github"
	SCMProviderTypeGitea  SCMProviderType = "gitea"
)

// SCMAuthMethod represents the authentication method for an SCM provider
type SCMAuthMethod string

const (
	SCMAuthMethodOAuth     SCMAuthMethod = "oauth"
	SCMAuthMethodPAT       SCMAuthMethod = "pat"
	SCMAuthMethodGitHubApp SCMAuthMethod = "github_app"
)

// SCMProvider represents a configured SCM provider (GitHub, GitLab, etc.)
type SCMProvider struct {
	ID                           int              `json:"id"`
	Slug                         string           `json:"slug"`
	Name                         string           `json:"name"`
	ProviderType                 SCMProviderType  `json:"provider_type"`
	AuthMethod                   SCMAuthMethod    `json:"auth_method"`
	Enabled                      bool             `json:"enabled"`
	IsDefault                    bool             `json:"is_default"`
	BaseURL                      string           `json:"base_url,omitempty"`
	OAuthClientID                string           `json:"oauth_client_id,omitempty"`
	OAuthClientSecretEncrypted   string           `json:"-"` // Never expose encrypted secrets
	PersonalAccessTokenEncrypted string           `json:"-"`
	GitHubAppID                  string           `json:"github_app_id,omitempty"`
	GitHubAppPrivateKeyEncrypted string           `json:"-"`
	GitHubAppInstallationID      string           `json:"github_app_installation_id,omitempty"`
	GitHubOrgID                  *int64           `json:"github_org_id,omitempty"` // Stable org ID for GitHub App discovery
	OAuthAccessTokenEncrypted    string           `json:"-"`
	OAuthRefreshTokenEncrypted   string           `json:"-"`
	OAuthTokenExpiresAt          *time.Time       `json:"oauth_token_expires_at,omitempty"`
	Scopes                       string           `json:"scopes"`
	WorkspaceRestrictionMode     string           `json:"workspace_restriction_mode"` // 'unrestricted' or 'restricted'
	CreatedAt                    time.Time        `json:"created_at"`
	UpdatedAt                    time.Time        `json:"updated_at"`
	// Computed fields for API responses
	HasOAuthClientSecret   bool `json:"has_oauth_client_secret,omitempty"`
	HasPAT                 bool `json:"has_pat,omitempty"`
	HasGitHubAppPrivateKey bool `json:"has_github_app_private_key,omitempty"`
	HasOAuthToken          bool `json:"has_oauth_token,omitempty"`
}

// SCMProviderRequest represents the API request for creating/updating an SCM provider
type SCMProviderRequest struct {
	Slug                    string          `json:"slug"`
	Name                    string          `json:"name"`
	ProviderType            SCMProviderType `json:"provider_type"`
	AuthMethod              SCMAuthMethod   `json:"auth_method"`
	Enabled                 bool            `json:"enabled"`
	IsDefault               bool            `json:"is_default"`
	BaseURL                 string          `json:"base_url,omitempty"`
	OAuthClientID           string          `json:"oauth_client_id,omitempty"`
	OAuthClientSecret       string          `json:"oauth_client_secret,omitempty"` // Plaintext, will be encrypted
	PersonalAccessToken     string          `json:"personal_access_token,omitempty"`
	GitHubAppID              string          `json:"github_app_id,omitempty"`
	GitHubAppPrivateKey      string          `json:"github_app_private_key,omitempty"`
	GitHubAppInstallationID  string          `json:"github_app_installation_id,omitempty"`
	GitHubOrgID              *int64          `json:"github_org_id,omitempty"` // Stable org ID for GitHub App discovery
	Scopes                   string          `json:"scopes,omitempty"`
	WorkspaceRestrictionMode string          `json:"workspace_restriction_mode,omitempty"` // 'unrestricted' or 'restricted'
}

// SCMProviderWorkspaceAllowlist represents a workspace allowed to use an SCM provider
type SCMProviderWorkspaceAllowlist struct {
	ID          int       `json:"id"`
	ProviderID  int       `json:"provider_id"`
	WorkspaceID int       `json:"workspace_id"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	// Joined fields for API responses
	WorkspaceName string `json:"workspace_name,omitempty"`
	WorkspaceKey  string `json:"workspace_key,omitempty"`
}

// SCMOAuthState represents a temporary OAuth state token
type SCMOAuthState struct {
	ID          int       `json:"id"`
	ProviderID  int       `json:"provider_id"`
	State       string    `json:"state"`
	RedirectURI string    `json:"redirect_uri"`
	UserID      int       `json:"user_id"`
	WorkspaceID *int      `json:"workspace_id,omitempty"` // If set, store credentials at workspace level
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// WorkspaceSCMConnection represents a connection between a workspace and an SCM provider
type WorkspaceSCMConnection struct {
	ID                   int       `json:"id"`
	WorkspaceID          int       `json:"workspace_id"`
	SCMProviderID        int       `json:"scm_provider_id"`
	Enabled              bool      `json:"enabled"`
	DefaultBranchPattern string    `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       string    `json:"item_key_pattern,omitempty"`
	CreatedBy            *int      `json:"created_by,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	// Workspace-level credentials (encrypted)
	OAuthAccessTokenEncrypted    string     `json:"-"`
	OAuthRefreshTokenEncrypted   string     `json:"-"`
	OAuthTokenExpiresAt          *time.Time `json:"oauth_token_expires_at,omitempty"`
	PersonalAccessTokenEncrypted string     `json:"-"`
	// Computed fields for API responses
	HasOAuthToken bool `json:"has_oauth_token,omitempty"`
	HasPAT        bool `json:"has_pat,omitempty"`
	// Joined fields
	ProviderName string          `json:"provider_name,omitempty"`
	ProviderType SCMProviderType `json:"provider_type,omitempty"`
	ProviderSlug string          `json:"provider_slug,omitempty"`
}

// WorkspaceSCMConnectionRequest represents the API request for workspace SCM connections
type WorkspaceSCMConnectionRequest struct {
	SCMProviderID        int    `json:"scm_provider_id"`
	Enabled              bool   `json:"enabled"`
	DefaultBranchPattern string `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       string `json:"item_key_pattern,omitempty"`
}

// WorkspaceRepository represents a repository linked to a workspace
type WorkspaceRepository struct {
	ID                       int       `json:"id"`
	WorkspaceSCMConnectionID int       `json:"workspace_scm_connection_id"`
	RepositoryExternalID     string    `json:"repository_external_id"`
	RepositoryName           string    `json:"repository_name"`
	RepositoryURL            string    `json:"repository_url"`
	DefaultBranch            string    `json:"default_branch"`
	IsActive                 bool      `json:"is_active"`
	LastSyncedAt             *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
	// Joined fields
	WorkspaceID  int             `json:"workspace_id,omitempty"`
	ProviderType SCMProviderType `json:"provider_type,omitempty"`
	ProviderSlug string          `json:"provider_slug,omitempty"`
}

// WorkspaceRepositoryRequest represents the API request for linking a repository
type WorkspaceRepositoryRequest struct {
	RepositoryExternalID string `json:"repository_external_id"`
	RepositoryName       string `json:"repository_name"`
	RepositoryURL        string `json:"repository_url"`
	DefaultBranch        string `json:"default_branch,omitempty"`
}

// SCMLinkType represents the type of SCM link
type SCMLinkType string

const (
	SCMLinkTypePullRequest SCMLinkType = "pull_request"
	SCMLinkTypeCommit      SCMLinkType = "commit"
	SCMLinkTypeBranch      SCMLinkType = "branch"
)

// SCMLinkState represents the state of a PR
type SCMLinkState string

const (
	SCMLinkStateOpen   SCMLinkState = "open"
	SCMLinkStateClosed SCMLinkState = "closed"
	SCMLinkStateMerged SCMLinkState = "merged"
)

// ItemSCMLink represents a link between an item and an SCM resource (PR, commit, branch)
type ItemSCMLink struct {
	ID                    int          `json:"id"`
	ItemID                int          `json:"item_id"`
	WorkspaceRepositoryID int          `json:"workspace_repository_id"`
	LinkType              SCMLinkType  `json:"link_type"`
	ExternalID            string       `json:"external_id"`
	ExternalURL           string       `json:"external_url,omitempty"`
	Title                 string       `json:"title,omitempty"`
	State                 SCMLinkState `json:"state,omitempty"`
	AuthorExternalID      string       `json:"author_external_id,omitempty"`
	AuthorName            string       `json:"author_name,omitempty"`
	DetectionSource       string       `json:"detection_source,omitempty"`
	CreatedAt             time.Time    `json:"created_at"`
	UpdatedAt             time.Time    `json:"updated_at"`
	// Joined fields
	RepositoryName string          `json:"repository_name,omitempty"`
	RepositoryURL  string          `json:"repository_url,omitempty"`
	ProviderType   SCMProviderType `json:"provider_type,omitempty"`
}

// ItemSCMLinkRequest represents the API request for creating an SCM link
type ItemSCMLinkRequest struct {
	WorkspaceRepositoryID int         `json:"workspace_repository_id"`
	LinkType              SCMLinkType `json:"link_type"`
	ExternalID            string      `json:"external_id"`
	ExternalURL           string      `json:"external_url,omitempty"`
	Title                 string      `json:"title,omitempty"`
	State                 string      `json:"state,omitempty"`
	AuthorName            string      `json:"author_name,omitempty"`
}

// SCMWebhook represents a registered webhook for a repository
type SCMWebhook struct {
	ID                    int       `json:"id"`
	WorkspaceRepositoryID int       `json:"workspace_repository_id"`
	WebhookExternalID     string    `json:"webhook_external_id,omitempty"`
	WebhookSecretEncrypted string   `json:"-"` // Never expose
	Events                string    `json:"events"` // JSON array
	IsActive              bool      `json:"is_active"`
	LastDeliveryAt        *time.Time `json:"last_delivery_at,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// SCMWebhookDelivery represents a webhook delivery record
type SCMWebhookDelivery struct {
	ID               int       `json:"id"`
	SCMWebhookID     int       `json:"scm_webhook_id"`
	DeliveryID       string    `json:"delivery_id,omitempty"`
	EventType        string    `json:"event_type"`
	PayloadSummary   string    `json:"payload_summary,omitempty"`
	Status           string    `json:"status"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	ProcessingTimeMs int       `json:"processing_time_ms,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// ============================================================================
// Asset Management Models
// ============================================================================

// AssetManagementSet represents a system-wide asset management container
type AssetManagementSet struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	CreatorName    string `json:"creator_name,omitempty"`
	AssetTypeCount int    `json:"asset_type_count,omitempty"`
	AssetCount     int    `json:"asset_count,omitempty"`
	// User's permission on this set (populated per-request)
	UserPermission string `json:"user_permission,omitempty"` // view, edit, admin, or empty
}

// AssetManagementSetPermission represents user-level permission for an asset set
type AssetManagementSetPermission struct {
	ID              int       `json:"id"`
	SetID           int       `json:"set_id"`
	UserID          int       `json:"user_id"`
	PermissionLevel string    `json:"permission_level"` // view, edit, admin
	GrantedBy       *int      `json:"granted_by,omitempty"`
	GrantedAt       time.Time `json:"granted_at"`
	// Joined fields
	UserName      string `json:"user_name,omitempty"`
	UserEmail     string `json:"user_email,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}

// AssetManagementSetGroupPermission represents group-level permission for an asset set
type AssetManagementSetGroupPermission struct {
	ID              int       `json:"id"`
	SetID           int       `json:"set_id"`
	GroupID         int       `json:"group_id"`
	PermissionLevel string    `json:"permission_level"` // view, edit, admin
	GrantedBy       *int      `json:"granted_by,omitempty"`
	GrantedAt       time.Time `json:"granted_at"`
	// Joined fields
	GroupName     string `json:"group_name,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}

// AssetType defines the structure/attributes of assets
type AssetType struct {
	ID           int       `json:"id"`
	SetID        int       `json:"set_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Icon         string    `json:"icon"`
	Color        string    `json:"color"`
	DisplayOrder int       `json:"display_order"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Joined fields
	SetName    string           `json:"set_name,omitempty"`
	AssetCount int              `json:"asset_count,omitempty"`
	Fields     []AssetTypeField `json:"fields,omitempty"`
}

// AssetTypeField represents a custom field assignment to an asset type
type AssetTypeField struct {
	ID            int       `json:"id"`
	AssetTypeID   int       `json:"asset_type_id"`
	CustomFieldID int       `json:"custom_field_id"`
	IsRequired    bool      `json:"is_required"`
	DisplayOrder  int       `json:"display_order"`
	CreatedAt     time.Time `json:"created_at"`
	// Joined fields from custom_field_definitions
	FieldName        string `json:"field_name,omitempty"`
	FieldType        string `json:"field_type,omitempty"`
	FieldDescription string `json:"field_description,omitempty"`
	Options          string `json:"options,omitempty"`
}

// AssetCategory represents a hierarchical organizational unit for assets
type AssetCategory struct {
	ID               int       `json:"id"`
	SetID            int       `json:"set_id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	ParentID         *int      `json:"parent_id,omitempty"`
	Path             string    `json:"path,omitempty"`
	HasChildren      bool      `json:"has_children"`
	ChildrenCount    int       `json:"children_count"`
	DescendantsCount int       `json:"descendants_count"`
	FracIndex        *string   `json:"frac_index,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Joined/computed fields
	SetName    string          `json:"set_name,omitempty"`
	ParentName string          `json:"parent_name,omitempty"`
	AssetCount int             `json:"asset_count,omitempty"`
	Children   []AssetCategory `json:"children,omitempty"`
}

// AssetStatus represents a configurable status for assets within a set
type AssetStatus struct {
	ID           int       `json:"id"`
	SetID        int       `json:"set_id"`
	Name         string    `json:"name"`
	Color        string    `json:"color"`
	Description  string    `json:"description"`
	IsDefault    bool      `json:"is_default"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Asset represents an individual asset instance
type Asset struct {
	ID                int                    `json:"id"`
	SetID             int                    `json:"set_id"`
	AssetTypeID       int                    `json:"asset_type_id"`
	CategoryID        *int                   `json:"category_id,omitempty"`
	StatusID          *int                   `json:"status_id,omitempty"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	AssetTag          string                 `json:"asset_tag,omitempty"`
	CustomFieldValues map[string]interface{} `json:"custom_field_values,omitempty"`
	FracIndex         *string                `json:"frac_index,omitempty"`
	CreatedBy         *int                   `json:"created_by,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	// Joined fields
	SetName        string `json:"set_name,omitempty"`
	AssetTypeName  string `json:"asset_type_name,omitempty"`
	AssetTypeIcon  string `json:"asset_type_icon,omitempty"`
	AssetTypeColor string `json:"asset_type_color,omitempty"`
	CategoryName   string `json:"category_name,omitempty"`
	CategoryPath   string `json:"category_path,omitempty"`
	StatusName     string `json:"status_name,omitempty"`
	StatusColor    string `json:"status_color,omitempty"`
	CreatorName    string `json:"creator_name,omitempty"`
	CreatorEmail   string `json:"creator_email,omitempty"`
	// Linked items count
	LinkedItemCount int `json:"linked_item_count,omitempty"`
}

// UserAssetSetPreference stores user's primary asset set preference
type UserAssetSetPreference struct {
	ID           int       `json:"id"`
	UserID       int       `json:"user_id"`
	PrimarySetID *int      `json:"primary_set_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Joined fields
	PrimarySetName string `json:"primary_set_name,omitempty"`
}

// AssetRole represents a role for asset management (Viewer, Editor, Administrator)
type AssetRole struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	IsSystem     bool      `json:"is_system"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Joined fields
	Permissions []AssetPermission `json:"permissions,omitempty"`
}

// AssetPermission represents a specific asset permission (asset.view, asset.edit, etc.)
type AssetPermission struct {
	ID             int       `json:"id"`
	PermissionKey  string    `json:"permission_key"`
	PermissionName string    `json:"permission_name"`
	Description    string    `json:"description"`
	CreatedAt      time.Time `json:"created_at"`
}

// UserAssetSetRole represents a user's role assignment for an asset set
type UserAssetSetRole struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	SetID     int       `json:"set_id"`
	RoleID    int       `json:"role_id"`
	GrantedBy *int      `json:"granted_by,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
	// Joined fields
	UserName      string `json:"user_name,omitempty"`
	UserEmail     string `json:"user_email,omitempty"`
	RoleName      string `json:"role_name,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}

// GroupAssetSetRole represents a group's role assignment for an asset set
type GroupAssetSetRole struct {
	ID        int       `json:"id"`
	GroupID   int       `json:"group_id"`
	SetID     int       `json:"set_id"`
	RoleID    int       `json:"role_id"`
	GrantedBy *int      `json:"granted_by,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
	// Joined fields
	GroupName     string `json:"group_name,omitempty"`
	RoleName      string `json:"role_name,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}

// AssetSetEveryoneRole represents the default role for all authenticated users on a set
type AssetSetEveryoneRole struct {
	SetID     int       `json:"set_id"`
	RoleID    *int      `json:"role_id,omitempty"`
	GrantedBy *int      `json:"granted_by,omitempty"`
	GrantedAt time.Time `json:"granted_at"`
	// Joined fields
	RoleName      string `json:"role_name,omitempty"`
	GrantedByName string `json:"granted_by_name,omitempty"`
}

// Recurring Tasks Models

// RecurrenceRule represents a recurring task pattern for generating instances
type RecurrenceRule struct {
	ID             int        `json:"id"`
	TemplateItemID int        `json:"template_item_id"`
	WorkspaceID    int        `json:"workspace_id"`

	// iCalendar RRULE configuration (RFC 5545)
	RRule    string     `json:"rrule"`    // e.g., "FREQ=WEEKLY;BYDAY=MO,WE,FR"
	DtStart  time.Time  `json:"dtstart"`  // Recurrence start date
	DtEnd    *time.Time `json:"dtend,omitempty"`
	Timezone string     `json:"timezone"` // IANA timezone

	// Generation settings
	LeadTimeDays        int        `json:"lead_time_days"`
	LastGeneratedUntil  *time.Time `json:"last_generated_until,omitempty"`
	NextGenerationCheck *time.Time `json:"next_generation_check,omitempty"`

	// Instance configuration (what to copy from template)
	CopyAssignee     bool `json:"copy_assignee"`
	CopyPriority     bool `json:"copy_priority"`
	CopyCustomFields bool `json:"copy_custom_fields"`
	CopyDescription  bool `json:"copy_description"`
	StatusOnCreate   *int `json:"status_on_create,omitempty"`

	// Lifecycle
	IsActive  bool       `json:"is_active"`
	CreatedBy *int       `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Joined fields for API responses
	TemplateTitle  string     `json:"template_title,omitempty"`
	WorkspaceName  string     `json:"workspace_name,omitempty"`
	WorkspaceKey   string     `json:"workspace_key,omitempty"`
	CreatorName    string     `json:"creator_name,omitempty"`
	InstanceCount  int        `json:"instance_count,omitempty"`
	NextOccurrence *time.Time `json:"next_occurrence,omitempty"`
}

// RecurrenceInstance represents a generated instance from a recurring rule
type RecurrenceInstance struct {
	ID               int       `json:"id"`
	RecurrenceRuleID int       `json:"recurrence_rule_id"`
	InstanceItemID   int       `json:"instance_item_id"`
	ScheduledDate    time.Time `json:"scheduled_date"`
	SequenceNumber   int       `json:"sequence_number"`
	CreatedAt        time.Time `json:"created_at"`

	// Joined fields for API responses
	ItemTitle  string `json:"item_title,omitempty"`
	ItemStatus string `json:"item_status,omitempty"`
}

// CreateRecurrenceRequest is used for API input
type CreateRecurrenceRequest struct {
	TemplateItemID   int     `json:"template_item_id"`
	RRule            string  `json:"rrule"`
	DtStart          string  `json:"dtstart"`          // ISO 8601 format
	DtEnd            *string `json:"dtend,omitempty"`
	Timezone         string  `json:"timezone,omitempty"`
	LeadTimeDays     *int    `json:"lead_time_days,omitempty"`
	CopyAssignee     *bool   `json:"copy_assignee,omitempty"`
	CopyPriority     *bool   `json:"copy_priority,omitempty"`
	CopyCustomFields *bool   `json:"copy_custom_fields,omitempty"`
	CopyDescription  *bool   `json:"copy_description,omitempty"`
	StatusOnCreate   *int    `json:"status_on_create,omitempty"`
}

// UpdateRecurrenceRequest is used for API input when updating a recurrence rule
type UpdateRecurrenceRequest struct {
	RRule            *string `json:"rrule,omitempty"`
	DtStart          *string `json:"dtstart,omitempty"`
	DtEnd            *string `json:"dtend,omitempty"`
	Timezone         *string `json:"timezone,omitempty"`
	LeadTimeDays     *int    `json:"lead_time_days,omitempty"`
	CopyAssignee     *bool   `json:"copy_assignee,omitempty"`
	CopyPriority     *bool   `json:"copy_priority,omitempty"`
	CopyCustomFields *bool   `json:"copy_custom_fields,omitempty"`
	CopyDescription  *bool   `json:"copy_description,omitempty"`
	StatusOnCreate   *int    `json:"status_on_create,omitempty"`
	IsActive         *bool   `json:"is_active,omitempty"`
}

// RRulePreviewRequest is used for previewing RRULE occurrences
type RRulePreviewRequest struct {
	RRule   string `json:"rrule"`
	DtStart string `json:"dtstart"`
	Count   int    `json:"count,omitempty"` // Number of occurrences to preview (default 10)
}
