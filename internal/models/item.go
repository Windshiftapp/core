package models

import "time"

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

// ItemLink represents a link between two items or test cases
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
	AssetSetID        *int   `json:"asset_set_id,omitempty"`
	AssetSetName      string `json:"asset_set_name,omitempty"`
	AssetTypeName     string `json:"asset_type_name,omitempty"`
	AssetCategoryName string `json:"asset_category_name,omitempty"`
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

// PaginatedItemsResponse represents a paginated list of items
type PaginatedItemsResponse struct {
	Items      []Item         `json:"items"`
	Pagination PaginationMeta `json:"pagination"`
}

// PaginatedAttachmentsResponse represents a paginated list of attachments
type PaginatedAttachmentsResponse struct {
	Attachments []Attachment   `json:"attachments"`
	Pagination  PaginationMeta `json:"pagination"`
}
