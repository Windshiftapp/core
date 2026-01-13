package dto

import (
	"time"
)

// ItemResponse is the public API representation of an Item
type ItemResponse struct {
	ID                  int                    `json:"id"`
	WorkspaceID         int                    `json:"workspace_id"`
	WorkspaceKey        string                 `json:"workspace_key"`
	Key                 string                 `json:"key"` // e.g., "PROJ-123"
	WorkspaceItemNumber int                    `json:"workspace_item_number"`
	Title               string                 `json:"title"`
	Description         string                 `json:"description,omitempty"`
	IsTask              bool                   `json:"is_task"`
	DueDate             *time.Time             `json:"due_date,omitempty"`
	CustomFields        map[string]interface{} `json:"custom_fields,omitempty"`

	// Hierarchy
	ParentID *int `json:"parent_id,omitempty"`

	// Related entities (populated based on ?expand= parameter)
	Status    *StatusSummary    `json:"status,omitempty"`
	Priority  *PrioritySummary  `json:"priority,omitempty"`
	ItemType  *ItemTypeSummary  `json:"item_type,omitempty"`
	Assignee  *UserSummary      `json:"assignee,omitempty"`
	Creator   *UserSummary      `json:"creator,omitempty"`
	Workspace *WorkspaceSummary `json:"workspace,omitempty"`
	Milestone *MilestoneSummary `json:"milestone,omitempty"`
	Iteration *IterationSummary `json:"iteration,omitempty"`
	Project   *ProjectSummary   `json:"project,omitempty"`

	// Expanded collections (populated based on ?expand= parameter)
	Comments    []CommentResponse    `json:"comments,omitempty"`
	Attachments []AttachmentResponse `json:"attachments,omitempty"`
	History     []HistoryResponse    `json:"history,omitempty"`
	Children    []ItemResponse       `json:"children,omitempty"`
	Transitions []TransitionResponse `json:"transitions,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// HATEOAS links
	Links *ItemLinks `json:"_links,omitempty"`
}

// ItemLinks provides HATEOAS-style links for an item
type ItemLinks struct {
	Self        string `json:"self"`
	Workspace   string `json:"workspace,omitempty"`
	Comments    string `json:"comments,omitempty"`
	History     string `json:"history,omitempty"`
	Attachments string `json:"attachments,omitempty"`
	Children    string `json:"children,omitempty"`
	Parent      string `json:"parent,omitempty"`
	Transitions string `json:"transitions,omitempty"`
}

// ItemCreateRequest is the request body for creating a new item
type ItemCreateRequest struct {
	WorkspaceID  int                    `json:"workspace_id" validate:"required"`
	Title        string                 `json:"title" validate:"required,max=255"`
	Description  string                 `json:"description,omitempty"`
	StatusID     *int                   `json:"status_id,omitempty"`
	PriorityID   *int                   `json:"priority_id,omitempty"`
	ItemTypeID   *int                   `json:"item_type_id,omitempty"`
	AssigneeID   *int                   `json:"assignee_id,omitempty"`
	ParentID     *int                   `json:"parent_id,omitempty"`
	MilestoneID  *int                   `json:"milestone_id,omitempty"`
	IterationID  *int                   `json:"iteration_id,omitempty"`
	ProjectID    *int                   `json:"project_id,omitempty"`
	DueDate      *time.Time             `json:"due_date,omitempty"`
	IsTask       bool                   `json:"is_task,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

// ItemUpdateRequest is the request body for updating an item
type ItemUpdateRequest struct {
	Title        *string                `json:"title,omitempty" validate:"omitempty,max=255"`
	Description  *string                `json:"description,omitempty"`
	StatusID     *int                   `json:"status_id,omitempty"`
	PriorityID   *int                   `json:"priority_id,omitempty"`
	ItemTypeID   *int                   `json:"item_type_id,omitempty"`
	AssigneeID   *int                   `json:"assignee_id,omitempty"`
	ParentID     *int                   `json:"parent_id,omitempty"`
	MilestoneID  *int                   `json:"milestone_id,omitempty"`
	IterationID  *int                   `json:"iteration_id,omitempty"`
	ProjectID    *int                   `json:"project_id,omitempty"`
	DueDate      *time.Time             `json:"due_date,omitempty"`
	IsTask       *bool                  `json:"is_task,omitempty"`
	CustomFields map[string]interface{} `json:"custom_fields,omitempty"`
}

// CommentResponse is the public API representation of a Comment
type CommentResponse struct {
	ID        int          `json:"id"`
	ItemID    int          `json:"item_id"`
	Content   string       `json:"content"`
	Author    *UserSummary `json:"author,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// CommentCreateRequest is the request body for creating a comment
type CommentCreateRequest struct {
	Content string `json:"content" validate:"required"`
}

// CommentUpdateRequest is the request body for updating a comment
type CommentUpdateRequest struct {
	Content string `json:"content" validate:"required"`
}

// HistoryResponse is the public API representation of an item history entry
type HistoryResponse struct {
	ID               int          `json:"id"`
	ItemID           int          `json:"item_id"`
	FieldName        string       `json:"field_name"`
	OldValue         *string      `json:"old_value,omitempty"`
	NewValue         *string      `json:"new_value,omitempty"`
	ResolvedOldValue *string      `json:"resolved_old_value,omitempty"`
	ResolvedNewValue *string      `json:"resolved_new_value,omitempty"`
	User             *UserSummary `json:"user,omitempty"`
	ChangedAt        time.Time    `json:"changed_at"`
}

// AttachmentResponse is the public API representation of an Attachment
type AttachmentResponse struct {
	ID               int          `json:"id"`
	ItemID           *int         `json:"item_id,omitempty"`
	Filename         string       `json:"filename"`
	OriginalFilename string       `json:"original_filename"`
	MimeType         string       `json:"mime_type"`
	FileSize         int64        `json:"file_size"`
	HasThumbnail     bool         `json:"has_thumbnail"`
	Uploader         *UserSummary `json:"uploader,omitempty"`
	CreatedAt        time.Time    `json:"created_at"`
	DownloadURL      string       `json:"download_url,omitempty"`
	ThumbnailURL     string       `json:"thumbnail_url,omitempty"`
}

// TransitionResponse represents an available workflow transition
type TransitionResponse struct {
	ID           int            `json:"id"`
	FromStatusID *int           `json:"from_status_id,omitempty"`
	ToStatusID   int            `json:"to_status_id"`
	FromStatus   *StatusSummary `json:"from_status,omitempty"`
	ToStatus     *StatusSummary `json:"to_status,omitempty"`
}
