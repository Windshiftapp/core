package main

import "time"

// ============================================
// Pagination
// ============================================

type PaginatedResponse[T any] struct {
	Data       []T            `json:"data"`
	Pagination PaginationMeta `json:"pagination,omitempty"`
	Total      int            `json:"total,omitempty"` // Some endpoints use total instead of pagination
}

type PaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// ============================================
// Users
// ============================================

type User struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	FullName  string `json:"full_name"`
	IsActive  bool   `json:"is_active"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	Language  string `json:"language,omitempty"`
	CreatedAt string `json:"created_at"`
}

type UserSummary struct {
	ID     int    `json:"id"`
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	Avatar string `json:"avatar,omitempty"`
}

// ============================================
// Workspaces
// ============================================

type Workspace struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	IsPersonal  bool   `json:"is_personal"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ============================================
// Items
// ============================================

type Item struct {
	ID                  int                    `json:"id"`
	WorkspaceID         int                    `json:"workspace_id"`
	WorkspaceKey        string                 `json:"workspace_key"`
	Key                 string                 `json:"key"`
	WorkspaceItemNumber int                    `json:"workspace_item_number"`
	Title               string                 `json:"title"`
	Description         string                 `json:"description,omitempty"`
	IsTask              bool                   `json:"is_task"`
	DueDate             *time.Time             `json:"due_date,omitempty"`
	CustomFields        map[string]interface{} `json:"custom_fields,omitempty"`

	// Hierarchy
	ParentID *int `json:"parent_id,omitempty"`

	// Related entities
	Status    *StatusSummary    `json:"status,omitempty"`
	Priority  *PrioritySummary  `json:"priority,omitempty"`
	ItemType  *ItemTypeSummary  `json:"item_type,omitempty"`
	Assignee  *UserSummary      `json:"assignee,omitempty"`
	Creator   *UserSummary      `json:"creator,omitempty"`
	Workspace *WorkspaceSummary `json:"workspace,omitempty"`
	Milestone *MilestoneSummary `json:"milestone,omitempty"`
	Iteration *IterationSummary `json:"iteration,omitempty"`
	Project   *ProjectSummary   `json:"project,omitempty"`

	// Expanded collections
	Comments    []Comment    `json:"comments,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	History     []History    `json:"history,omitempty"`
	Children    []Item       `json:"children,omitempty"`
	Transitions []Transition `json:"transitions,omitempty"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type ItemCreateRequest struct {
	WorkspaceID  int                    `json:"workspace_id"`
	Title        string                 `json:"title"`
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

type ItemUpdateRequest struct {
	Title        *string                `json:"title,omitempty"`
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

// ============================================
// Statuses
// ============================================

type Status struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	CategoryID    int    `json:"category_id"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	IsDefault     bool   `json:"is_default"`
	IsCompleted   bool   `json:"is_completed"`
}

type StatusSummary struct {
	ID            int    `json:"id"`
	Name          string `json:"name,omitempty"`
	CategoryID    int    `json:"category_id,omitempty"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
	IsCompleted   bool   `json:"is_completed,omitempty"`
}

// ============================================
// Item Types
// ============================================

type ItemType struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
}

type ItemTypeSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`
	Icon string `json:"icon,omitempty"`
}

// ============================================
// Priorities
// ============================================

type Priority struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

type PrioritySummary struct {
	ID    int    `json:"id"`
	Name  string `json:"name,omitempty"`
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

// ============================================
// Workflows & Transitions
// ============================================

type Workflow struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsDefault   bool   `json:"is_default"`
}

type Transition struct {
	ID           int            `json:"id"`
	FromStatusID *int           `json:"from_status_id,omitempty"`
	ToStatusID   int            `json:"to_status_id"`
	FromStatus   *StatusSummary `json:"from_status,omitempty"`
	ToStatus     *StatusSummary `json:"to_status,omitempty"`
}

// ============================================
// Other Summaries
// ============================================

type WorkspaceSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`
	Key  string `json:"key,omitempty"`
}

type MilestoneSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`
}

type IterationSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`
}

type ProjectSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`
}

// ============================================
// Comments
// ============================================

type Comment struct {
	ID        int          `json:"id"`
	ItemID    int          `json:"item_id"`
	Content   string       `json:"content"`
	Author    *UserSummary `json:"author,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// ============================================
// Attachments
// ============================================

type Attachment struct {
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

// ============================================
// History
// ============================================

type History struct {
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

// ============================================
// Test Management
// ============================================

type TestCase struct {
	ID                int         `json:"id"`
	WorkspaceID       int         `json:"workspace_id"`
	FolderID          *int        `json:"folder_id,omitempty"`
	FolderName        string      `json:"folder_name,omitempty"`
	Title             string      `json:"title"`
	Preconditions     string      `json:"preconditions,omitempty"`
	Priority          string      `json:"priority,omitempty"`
	Status            string      `json:"status,omitempty"`
	EstimatedDuration int         `json:"estimated_duration,omitempty"`
	SortOrder         int         `json:"sort_order,omitempty"`
	Labels            []TestLabel `json:"labels,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

type TestStep struct {
	ID         int       `json:"id"`
	TestCaseID int       `json:"test_case_id"`
	StepNumber int       `json:"step_number"`
	Action     string    `json:"action"`
	Data       string    `json:"data,omitempty"`
	Expected   string    `json:"expected,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type TestLabel struct {
	ID          int       `json:"id"`
	WorkspaceID int       `json:"workspace_id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TestSet struct {
	ID             int        `json:"id"`
	WorkspaceID    int        `json:"workspace_id"`
	Name           string     `json:"name"`
	Description    string     `json:"description,omitempty"`
	MilestoneID    *int       `json:"milestone_id,omitempty"`
	MilestoneName  string     `json:"milestone_name,omitempty"`
	TestCaseCount  int        `json:"test_case_count,omitempty"`
	TotalRuns      int        `json:"total_runs,omitempty"`
	SuccessfulRuns int        `json:"successful_runs,omitempty"`
	FailedRuns     int        `json:"failed_runs,omitempty"`
	LastRunStatus  string     `json:"last_run_status,omitempty"`
	LastRunDate    *time.Time `json:"last_run_date,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type TestRun struct {
	ID             int        `json:"id"`
	WorkspaceID    int        `json:"workspace_id"`
	TemplateID     int        `json:"template_id,omitempty"`
	SetID          int        `json:"set_id"`
	Name           string     `json:"name"`
	AssigneeID     *int       `json:"assignee_id,omitempty"`
	AssigneeName   string     `json:"assignee_name,omitempty"`
	AssigneeEmail  string     `json:"assignee_email,omitempty"`
	AssigneeAvatar string     `json:"assignee_avatar,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type TestRunCreateRequest struct {
	SetID      int    `json:"set_id"`
	Name       string `json:"name"`
	TemplateID int    `json:"template_id,omitempty"`
	AssigneeID *int   `json:"assignee_id,omitempty"`
}

type TestResult struct {
	ID            int        `json:"id"`
	RunID         int        `json:"run_id"`
	TestCaseID    int        `json:"test_case_id"`
	TestCaseTitle string     `json:"test_case_title,omitempty"`
	Status        string     `json:"status"`
	ActualResult  string     `json:"actual_result,omitempty"`
	Notes         string     `json:"notes,omitempty"`
	ExecutedAt    *time.Time `json:"executed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type TestResultUpdateRequest struct {
	Status       string `json:"status"`
	ActualResult string `json:"actual_result,omitempty"`
	Notes        string `json:"notes,omitempty"`
}
