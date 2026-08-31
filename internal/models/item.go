package models

import "time"

// Item represents a unit of work with hierarchical support.
type Item struct {
	ID                  int        `json:"id"`
	WorkspaceID         int        `json:"workspace_id"`
	WorkspaceItemNumber int        `json:"workspace_item_number"`  // Workspace-specific sequential number for display keys
	ItemTypeID          *int       `json:"item_type_id,omitempty"` // Optional, defaults to workspace's default item type
	Title               string     `json:"title"`
	Description         string     `json:"description"`
	DescriptionHTML     string     `json:"description_html,omitempty" db:"-"`
	StatusID            *int       `json:"status_id,omitempty"`    // Foreign key to statuses table
	PriorityID          *int       `json:"priority_id,omitempty"`  // Foreign key to priorities table
	DueDate             *time.Time `json:"due_date,omitempty"`     // Due date for item completion
	StartDate           *time.Time `json:"start_date,omitempty"`   // Start date for item
	EndDate             *time.Time `json:"end_date,omitempty"`     // End date for item
	IsTask              bool       `json:"is_task"`                // Flag to mark this item as a task (checklist item)
	IterationID         *int       `json:"iteration_id,omitempty"` // Optional iteration assignment
	// Time-tracking project membership. "Project" in Windshift always refers
	// to a row in time_projects; both ProjectID and TimeProjectID below are
	// FKs to that table, serving different roles:
	//   ProjectID       - the project this item belongs to (the bucket
	//                     worklogs default to). Can be inherited from the
	//                     parent item; see InheritProject.
	//   TimeProjectID   - overrides the project used when logging time on
	//                     this specific item (independent of the bucket the
	//                     item belongs to).
	// EffectiveProjectID (below) is the resolved value after applying
	// inheritance, and is what consumers should read.
	ProjectID      *int `json:"project_id,omitempty"`
	InheritProject bool `json:"inherit_project"` // If true, ProjectID is ignored and the parent item's effective project is used
	TimeProjectID  *int `json:"time_project_id,omitempty"`
	// User assignment fields
	AssigneeID              *int `json:"assignee_id,omitempty"`                // User assigned to this item
	CreatorID               *int `json:"creator_id,omitempty"`                 // Internal user who created this item
	ReporterID              *int `json:"reporter_id,omitempty"`                // User who reported this item (from Jira import)
	CreatorPortalCustomerID *int `json:"creator_portal_customer_id,omitempty"` // Portal customer who created this item (for portal submissions)
	// Portal submission tracking fields (immutable once set)
	ChannelID         *int                    `json:"channel_id,omitempty"`      // Portal/channel this request was submitted through
	RequestTypeID     *int                    `json:"request_type_id,omitempty"` // Request type used for submission
	CustomFieldValues map[string]any          `json:"custom_field_values,omitempty"`
	VirtualFieldData  map[string]any          `json:"virtual_field_data,omitempty"`
	CalendarData      []CalendarScheduleEntry `json:"calendar_data,omitempty"`
	// Hierarchy fields
	ParentID *int `json:"parent_id"` // Foreign key to parent item
	// Personal task relationship (for linking personal workspace tasks to work items)
	RelatedWorkItemID *int `json:"related_work_item_id,omitempty"` // Link to work item (for personal tasks)
	// Estimation
	StoryPoints     *float64 `json:"story_points,omitempty"`     // Story points for velocity tracking
	EstimateMinutes *int     `json:"estimate_minutes,omitempty"` // Time estimate in minutes (compared against logged worklog time)
	// Manual sorting field
	FracIndex *string   `json:"frac_index,omitempty"` // Fractional index string for manual ordering
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Recency for the board "Bubble Mode" sort. Bumped on activity (comments,
	// edits, transitions) but not on manual frac_index reorder.
	LastActiveAt time.Time  `json:"last_active_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	// Joined fields for API responses
	WorkspaceName             string `json:"workspace_name,omitempty"`
	WorkspaceKey              string `json:"workspace_key,omitempty"`
	ItemTypeName              string `json:"item_type_name,omitempty"`
	ItemTypeBuiltinKey        string `json:"item_type_builtin_key,omitempty"`
	PriorityName              string `json:"priority_name,omitempty"`
	PriorityBuiltinKey        string `json:"priority_builtin_key,omitempty"`
	PriorityIcon              string `json:"priority_icon,omitempty"`
	PriorityColor             string `json:"priority_color,omitempty"`
	ParentTitle               string `json:"parent_title,omitempty"`
	ParentWorkspaceItemNumber *int   `json:"parent_workspace_item_number,omitempty"` // Parent's workspace-scoped number, for rendering the parent key
	StatusName                string `json:"status_name,omitempty"`                  // Name from statuses table (joined field)
	StatusBuiltinKey          string `json:"status_builtin_key,omitempty"`           // Stable key for translating built-in statuses
	StatusColor               string `json:"status_color,omitempty"`                 // Color from the status category (joined field)
	// StatusSince is when the item last entered its current status, derived from
	// item_history (falls back to created_at when there is no recorded change).
	StatusSince      *time.Time `json:"status_since,omitempty"`
	IterationName    string     `json:"iteration_name,omitempty"`
	IterationEndDate string     `json:"iteration_end_date,omitempty"`
	ProjectName      string     `json:"project_name,omitempty"`      // Name of the time-tracking project this item belongs to (joined from time_projects via ProjectID)
	TimeProjectName  string     `json:"time_project_name,omitempty"` // Name of the time-tracking project overriding worklog defaults (joined from time_projects via TimeProjectID)
	// EffectiveProjectID is the resolved project after walking InheritProject
	// up the parent chain. Consumers that need "which time-tracking project
	// applies to this item" should read this, not ProjectID directly.
	EffectiveProjectID     *int   `json:"effective_project_id,omitempty"`
	EffectiveProjectName   string `json:"effective_project_name,omitempty"`
	ProjectInheritanceMode string `json:"project_inheritance_mode,omitempty"` // "none" | "inherit" | "direct"
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
	RelatedWorkItemTitle        string          `json:"related_work_item_title,omitempty"`
	RelatedWorkItemWorkspaceKey string          `json:"related_work_item_workspace_key,omitempty"`
	RelatedWorkItemWorkspaceID  int             `json:"related_work_item_workspace_id,omitempty"`
	RelatedWorkItemNumber       int             `json:"related_work_item_number,omitempty"`
	Children                    []Item          `json:"children,omitempty"` // For tree representations
	Labels                      []Label         `json:"labels,omitempty"`   // Global item labels
	PersonalLabels              []PersonalLabel `json:"personal_labels,omitempty"`
	Milestones                  []Milestone     `json:"milestones,omitempty"`
}

// TimeRollup is the rolled-up estimate/logged total for an item plus its
// descendants. Returned by GET /items/{id}/time-rollup so the Time Tracking
// tab can show "X logged of Y estimated" across an entire subtree.
type TimeRollup struct {
	ItemCount            int   `json:"item_count"`             // Items included in the sum (root + descendants).
	TotalEstimateMinutes int64 `json:"total_estimate_minutes"` // Sum of items.estimate_minutes across the included items.
	TotalLoggedMinutes   int64 `json:"total_logged_minutes"`   // Sum of time_worklogs.duration_minutes for those items.
	Truncated            bool  `json:"truncated"`              // True when the descendant set hit the max-items cap.
}

// RoadmapHierarchyDate is the minimal hierarchy projection used by roadmap
// rollup and rolldown calculations.
type RoadmapHierarchyDate struct {
	ID          int    `json:"id"`
	WorkspaceID int    `json:"workspace_id"`
	ParentID    *int   `json:"parent_id"`
	StartDate   string `json:"start_date,omitempty"`
	EndDate     string `json:"end_date,omitempty"`
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
	IsAgent   bool   `json:"is_agent"`             // Whether the user is an AI agent
	// AgentOwnerName is permission-filtered by the item-history handler.
	AgentOwnerName string `json:"agent_owner_name,omitempty"`
	// Resolved values for display (when value is an ID)
	ResolvedOldValue *string `json:"resolved_old_value,omitempty"` // Human-readable version of old_value
	ResolvedNewValue *string `json:"resolved_new_value,omitempty"` // Human-readable version of new_value
}

// ItemStatusDuration is the accumulated time an item has spent in one status.
type ItemStatusDuration struct {
	StatusID        int       `json:"status_id"`
	StatusName      string    `json:"status_name"`
	DurationSeconds int64     `json:"duration_seconds"`
	FirstEnteredAt  time.Time `json:"first_entered_at"`
	LastEnteredAt   time.Time `json:"last_entered_at"`
	IsCurrent       bool      `json:"is_current"`
}

// ItemStatusDurations is computed from the item's status transition history.
type ItemStatusDurations struct {
	Statuses     []ItemStatusDuration `json:"statuses"`
	CalculatedAt time.Time            `json:"calculated_at"`
}

// ItemDiagram represents a diagram associated with an item
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

// Comment represents a comment on an item
type Comment struct {
	ID               int       `json:"id"`
	ItemID           int       `json:"item_id"`                      // Changed from IssueID to ItemID
	AuthorID         *int      `json:"author_id,omitempty"`          // User ID (nullable for portal customer comments)
	PortalCustomerID *int      `json:"portal_customer_id,omitempty"` // Portal customer ID (for email-based comments)
	Content          string    `json:"content"`                      // Untrusted Markdown source
	ContentHTML      string    `json:"content_html,omitempty" db:"-"`
	IsPrivate        bool      `json:"is_private"` // For private notes (action automation)
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Joined fields for API responses
	AuthorName   string `json:"author_name,omitempty"`
	AuthorEmail  string `json:"author_email,omitempty"`
	AuthorAvatar string `json:"author_avatar,omitempty"`
	// Source is the origin of the row: "human" for rows from the comments
	// table, "approval" for synthetic rows projected from approval_decisions.
	// IsAgent mirrors users.is_agent so the UI can mark agent-authored rows.
	Source  string `json:"source"`
	IsAgent bool   `json:"is_agent,omitempty"`
	// AgentOwnerName is permission-filtered by the comment-list handler.
	AgentOwnerName string `json:"agent_owner_name,omitempty"`
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

// Attachment represents a file attached to an item
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

// AttachmentSettings represents system-wide attachment configuration
type AttachmentSettings struct {
	ID               int       `json:"id"`
	MaxFileSize      int64     `json:"max_file_size"`      // Maximum file size in bytes
	AllowedMimeTypes string    `json:"allowed_mime_types"` // JSON array of allowed MIME types
	AttachmentPath   string    `json:"attachment_path"`    // Base path for storing attachments
	Enabled          bool      `json:"enabled"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AttachmentUploadResponse represents the response from an attachment upload
type AttachmentUploadResponse struct {
	Success    bool       `json:"success"`
	Message    string     `json:"message"`
	Attachment Attachment `json:"attachment,omitempty"`
}

// AttachmentSettingsRequest represents the request for updating attachment settings
type AttachmentSettingsRequest struct {
	MaxFileSize      int64    `json:"max_file_size"`
	AllowedMimeTypes []string `json:"allowed_mime_types"`
	Enabled          bool     `json:"enabled"`
}

// LinkType represents a type of link between items
type LinkType struct {
	ID                 int       `json:"id"`
	BuiltinKey         string    `json:"builtin_key,omitempty"`
	Name               string    `json:"name"`
	DisplayName        string    `json:"display_name"`
	Description        string    `json:"description"`
	DisplayDescription string    `json:"display_description,omitempty"`
	ForwardLabel       string    `json:"forward_label"`
	ReverseLabel       string    `json:"reverse_label"`
	Color              string    `json:"color"`
	IsSystem           bool      `json:"is_system"`
	Active             bool      `json:"active"`
	AllowedEntityTypes []string  `json:"allowed_entity_types"` // nil = all types allowed; e.g. ["item","test_case"]
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ItemLink represents a link between two items or test cases
type ItemLink struct {
	ID            int       `json:"id"`
	LinkTypeID    int       `json:"link_type_id"`
	SourceType    string    `json:"source_type"` // "item" or "test_case"
	SourceID      int       `json:"source_id"`
	TargetType    string    `json:"target_type"` // "item" or "test_case"
	TargetID      int       `json:"target_id"`
	CreatedBy     *int      `json:"created_by"`
	CustomFieldID *int      `json:"custom_field_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
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
	SourceStatusColor   string `json:"source_status_color,omitempty"`
	SourceItemTypeID    *int   `json:"source_item_type_id,omitempty"`
	SourceItemTypeName  string `json:"source_item_type_name,omitempty"`
	SourceItemTypeIcon  string `json:"source_item_type_icon,omitempty"`
	SourceItemTypeColor string `json:"source_item_type_color,omitempty"`
	SourceWorkspaceKey  string `json:"source_workspace_key,omitempty"`
	SourceWorkspaceID   *int   `json:"source_workspace_id,omitempty"`
	// SourceItemNumber is the workspace-scoped item number (the "74" in
	// "WI-74"). Display the user-facing key as
	// SourceWorkspaceKey + "-" + SourceItemNumber, never SourceID — that
	// field is the global DB id and looks wrong (e.g. "WI-149") in UI.
	SourceItemNumber *int `json:"source_item_number,omitempty"`
	// Target item details
	TargetStatusID      *int   `json:"target_status_id,omitempty"`
	TargetStatusName    string `json:"target_status_name,omitempty"`
	TargetStatusColor   string `json:"target_status_color,omitempty"`
	TargetItemTypeID    *int   `json:"target_item_type_id,omitempty"`
	TargetItemTypeName  string `json:"target_item_type_name,omitempty"`
	TargetItemTypeIcon  string `json:"target_item_type_icon,omitempty"`
	TargetItemTypeColor string `json:"target_item_type_color,omitempty"`
	TargetWorkspaceKey  string `json:"target_workspace_key,omitempty"`
	TargetWorkspaceID   *int   `json:"target_workspace_id,omitempty"`
	// TargetItemNumber: see SourceItemNumber.
	TargetItemNumber *int `json:"target_item_number,omitempty"`
	// Custom field link metadata
	CustomFieldName string `json:"custom_field_name,omitempty"`
}

// LinkableItem represents an item that can be linked (work item, test case, or asset)
type LinkableItem struct {
	ID          int    `json:"id"`
	Type        string `json:"type"` // "item", "test_case", or "asset"
	Title       string `json:"title"`
	Description string `json:"description"`
	// For work items
	WorkspaceID         *int   `json:"workspace_id,omitempty"`
	WorkspaceName       string `json:"workspace_name,omitempty"`
	WorkspaceKey        string `json:"workspace_key,omitempty"`
	WorkspaceItemNumber *int   `json:"workspace_item_number,omitempty"`
	Status              string `json:"status,omitempty"`
	Priority            string `json:"priority,omitempty"`
	ItemTypeID          *int   `json:"item_type_id,omitempty"`
	ItemTypeName        string `json:"item_type_name,omitempty"`
	ItemTypeIcon        string `json:"item_type_icon,omitempty"`
	ItemTypeColor       string `json:"item_type_color,omitempty"`
	// For assets
	AssetSetID        *int   `json:"asset_set_id,omitempty"`
	AssetSetName      string `json:"asset_set_name,omitempty"`
	AssetTypeName     string `json:"asset_type_name,omitempty"`
	AssetCategoryName string `json:"asset_category_name,omitempty"`
}

// LinkedAsset is a display row for an asset linked to an item. Direction is
// "outgoing" when the item is the link source and "incoming" when the asset
// is the link source.
type LinkedAsset struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	SetID        int    `json:"set_id"`
	SetName      string `json:"set_name"`
	TypeName     string `json:"type_name"`
	CategoryName string `json:"category_name"`
	LinkID       int    `json:"link_id"`
	LinkTypeName string `json:"link_type_name"`
	LinkLabel    string `json:"link_label"`
	Direction    string `json:"direction"` // "outgoing" or "incoming"
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

// Collection represents a saved QL query with metadata
type Collection struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	QLQuery     string  `json:"ql_query"`
	FilterState *string `json:"filter_state"`
	IsPublic    bool    `json:"is_public"`
	WorkspaceID *int    `json:"workspace_id"`
	CategoryID  *int    `json:"category_id,omitempty"`
	CreatedBy   *int    `json:"created_by"`
	PublicSlug  *string `json:"public_slug,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	// Joined fields for API responses
	CreatorName   string `json:"creator_name,omitempty"`
	CreatorEmail  string `json:"creator_email,omitempty"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
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

// PersonalLabel represents a label for organizing personal tasks
type PersonalLabel struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	UserID    *int      `json:"user_id,omitempty"` // NULL for global labels, user_id for user-specific labels
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RelationshipGraphNode represents a node in the asset relationship graph
type RelationshipGraphNode struct {
	ID       string         `json:"id"` // Unique node key: "{type}-{entity_id}"
	EntityID int            `json:"entity_id"`
	Type     string         `json:"type"` // "asset", "item", "test_case"
	Title    string         `json:"title"`
	IsOrigin bool           `json:"is_origin"`
	Hop      int            `json:"hop"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// RelationshipGraphEdge represents an edge in the asset relationship graph
type RelationshipGraphEdge struct {
	ID       string `json:"id"`
	Source   string `json:"source"` // Node ID of source
	Target   string `json:"target"` // Node ID of target
	Label    string `json:"label"`
	Color    string `json:"color,omitempty"`
	EdgeType string `json:"edge_type"` // "link" or "field_reference"
}

// RelationshipGraphResponse is the response for the relationship graph endpoint
type RelationshipGraphResponse struct {
	Nodes      []RelationshipGraphNode `json:"nodes"`
	Edges      []RelationshipGraphEdge `json:"edges"`
	Truncated  bool                    `json:"truncated"`
	TotalCount int                     `json:"total_count"`
}

// PaginatedItemsResponse represents a paginated list of items
type PaginatedItemsResponse struct {
	Items          []Item         `json:"items"`
	Pagination     PaginationMeta `json:"pagination"`
	SortableFields []string       `json:"sortable_fields,omitempty"`
	Watermark      int64          `json:"watermark,omitempty"`
	NextCursor     string         `json:"next_cursor,omitempty"`
}

// PaginatedAttachmentsResponse represents a paginated list of attachments
type PaginatedAttachmentsResponse struct {
	Attachments []Attachment   `json:"attachments"`
	Pagination  PaginationMeta `json:"pagination"`
}
