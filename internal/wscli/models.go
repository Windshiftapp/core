package wscli

import (
	"encoding/json"
	"time"
)

// ============================================
// Pagination
// ============================================

type PaginatedResponse[T any] struct {
	Data       []T            `json:"data"`
	Pagination PaginationMeta `json:"pagination,omitempty"`
	Total      int            `json:"total,omitempty"` // Some endpoints use total instead of pagination
}

type DataDocument[T any] struct {
	Data T `json:"data"`
}

type PaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
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
	ID        int    `json:"id"`
	FullName  string `json:"full_name,omitempty"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
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
	ID                  int            `json:"id"`
	WorkspaceID         int            `json:"workspace_id"`
	WorkspaceKey        string         `json:"workspace_key"`
	Key                 string         `json:"key"`
	WorkspaceItemNumber int            `json:"workspace_item_number"`
	Title               string         `json:"title"`
	Description         string         `json:"description,omitempty"`
	IsTask              bool           `json:"is_task"`
	DueDate             *time.Time     `json:"due_date,omitempty"`
	StartDate           *time.Time     `json:"start_date,omitempty"`
	EndDate             *time.Time     `json:"end_date,omitempty"`
	CustomFields        map[string]any `json:"custom_fields,omitempty"`

	// Hierarchy. ParentKey/ParentTitle are populated by the server on
	// permission-checked single-item reads (omitted when the caller may not
	// view the parent); render ParentKey rather than deriving one from ParentID.
	ParentID    *int   `json:"parent_id,omitempty"`
	ParentKey   string `json:"parent_key,omitempty"`
	ParentTitle string `json:"parent_title,omitempty"`

	// Canonical v2 relationship fields use IDs and display names directly.
	StatusID      *int               `json:"status_id,omitempty"`
	StatusName    string             `json:"status_name,omitempty"`
	StatusColor   string             `json:"status_color,omitempty"`
	PriorityID    *int               `json:"priority_id,omitempty"`
	PriorityName  string             `json:"priority_name,omitempty"`
	PriorityIcon  string             `json:"priority_icon,omitempty"`
	PriorityColor string             `json:"priority_color,omitempty"`
	ItemTypeID    *int               `json:"item_type_id,omitempty"`
	ItemTypeName  string             `json:"item_type_name,omitempty"`
	AssigneeID    *int               `json:"assignee_id,omitempty"`
	AssigneeName  string             `json:"assignee_name,omitempty"`
	CreatorID     *int               `json:"creator_id,omitempty"`
	CreatorName   string             `json:"creator_name,omitempty"`
	WorkspaceName string             `json:"workspace_name,omitempty"`
	IterationID   *int               `json:"iteration_id,omitempty"`
	IterationName string             `json:"iteration_name,omitempty"`
	ProjectID     *int               `json:"project_id,omitempty"`
	ProjectName   string             `json:"project_name,omitempty"`
	Milestones    []MilestoneSummary `json:"milestones,omitempty"`

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

	// EnforcedTemplate is set on the create response when the item's type
	// enforces a mandatory work item template (WI-438), so the CLI can echo the
	// expected structure even when a description was supplied.
	EnforcedTemplate *EnforcedTemplate `json:"enforced_template,omitempty"`
}

// EnforcedTemplate mirrors dto.EnforcedTemplateSummary on the create response.
type EnforcedTemplate struct {
	TemplateID int    `json:"template_id"`
	Name       string `json:"name"`
	Applied    bool   `json:"applied"`
}

type ItemCreateRequest struct {
	WorkspaceID  int            `json:"workspace_id"`
	Title        string         `json:"title"`
	Description  string         `json:"description,omitempty"`
	StatusID     *int           `json:"status_id,omitempty"`
	PriorityID   *int           `json:"priority_id,omitempty"`
	ItemTypeID   *int           `json:"item_type_id,omitempty"`
	AssigneeID   *int           `json:"assignee_id,omitempty"`
	ParentID     *int           `json:"parent_id,omitempty"`
	MilestoneIDs []int          `json:"milestone_ids,omitempty"`
	IterationID  *int           `json:"iteration_id,omitempty"`
	ProjectID    *int           `json:"project_id,omitempty"`
	DueDate      *time.Time     `json:"due_date,omitempty"`
	StartDate    *time.Time     `json:"start_date,omitempty"`
	EndDate      *time.Time     `json:"end_date,omitempty"`
	IsTask       bool           `json:"is_task,omitempty"`
	CustomFields map[string]any `json:"custom_fields,omitempty"`
}

// ItemUpdateRequest is the merge patch for /rest/api/v2/items/{id}. It does not
// carry status_id — status changes go through TransitionRequest on a
// dedicated endpoint so workflow and condition rules are enforced.
type ItemUpdateRequest struct {
	Title        *string        `json:"title,omitempty"`
	Description  *string        `json:"description,omitempty"`
	PriorityID   *int           `json:"priority_id,omitempty"`
	ItemTypeID   *int           `json:"item_type_id,omitempty"`
	AssigneeID   *int           `json:"assignee_id,omitempty"`
	ParentID     *int           `json:"parent_id,omitempty"`
	MilestoneIDs *[]int         `json:"milestone_ids,omitempty"`
	IterationID  *int           `json:"iteration_id,omitempty"`
	ProjectID    *int           `json:"project_id,omitempty"`
	DueDate      *time.Time     `json:"due_date,omitempty"`
	StartDate    *time.Time     `json:"start_date,omitempty"`
	EndDate      *time.Time     `json:"end_date,omitempty"`
	IsTask       *bool          `json:"is_task,omitempty"`
	CustomFields map[string]any `json:"custom_fields,omitempty"`
}

// TransitionRequest is the body for POST /rest/api/v2/items/{id}/transition.
type TransitionRequest struct {
	ToStatusID int `json:"to_status_id"`
}

// ItemTypeChangeRequest is the body for POST /rest/api/v2/items/{id}/change-type.
type ItemTypeChangeRequest struct {
	TargetItemTypeID int  `json:"target_item_type_id"`
	TargetStatusID   *int `json:"target_status_id,omitempty"`
}

// TransitionResult is the response from a transition call.
type TransitionResult struct {
	Item        *Item `json:"item"`
	OldStatusID *int  `json:"old_status_id,omitempty"`
	NewStatusID *int  `json:"new_status_id,omitempty"`
	NoOp        bool  `json:"no_op"`
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

// StatusListResult makes the discovery scope explicit. Workspace-scoped
// status output is safe to use for item transitions in that workspace;
// system scope is only the global status catalog.
type StatusListResult struct {
	Scope     string               `json:"scope"`
	Workspace *StatusListWorkspace `json:"workspace,omitempty"`
	Statuses  []Status             `json:"statuses"`
}

type StatusListWorkspace struct {
	ID   int    `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
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
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
	SortOrder   int    `json:"sort_order"`
	IsDefault   bool   `json:"is_default"`
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

type Milestone struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description,omitempty"`
	TargetDate    *string `json:"target_date,omitempty"`
	Status        string  `json:"status"`
	CategoryID    *int    `json:"category_id,omitempty"`
	CategoryName  string  `json:"category_name,omitempty"`
	CategoryColor string  `json:"category_color,omitempty"`
	IsGlobal      bool    `json:"is_global"`
	WorkspaceID   *int    `json:"workspace_id,omitempty"`
	WorkspaceName string  `json:"workspace_name,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

type MilestoneCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	TargetDate  string `json:"target_date,omitempty"`
	Status      string `json:"status,omitempty"`
}

type MilestoneUpdateRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	TargetDate  *string `json:"target_date,omitempty"`
	Status      *string `json:"status,omitempty"`
}

// MilestoneProgress mirrors services.MilestoneProgressReport returned by
// GET /api/milestones/{id}/progress.
type MilestoneProgress struct {
	MilestoneID     int                                `json:"milestone_id"`
	MilestoneName   string                             `json:"milestone_name"`
	Description     string                             `json:"description,omitempty"`
	TargetDate      *string                            `json:"target_date,omitempty"`
	Status          string                             `json:"status"`
	CategoryColor   string                             `json:"category_color,omitempty"`
	TotalItems      int                                `json:"total_items"`
	CompletedItems  int                                `json:"completed_items"`
	PercentComplete float64                            `json:"percent_complete"`
	StatusBreakdown []MilestoneStatusBreakdown         `json:"status_breakdown"`
	ItemsByCategory map[string][]MilestoneProgressItem `json:"items_by_category"`
}

type MilestoneStatusBreakdown struct {
	CategoryName       string `json:"category_name"`
	CategoryBuiltinKey string `json:"category_builtin_key,omitempty"`
	CategoryColor      string `json:"category_color,omitempty"`
	ItemCount          int    `json:"item_count"`
	IsCompleted        bool   `json:"is_completed"`
}

type MilestoneProgressItem struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	WorkspaceID    int    `json:"workspace_id"`
	WorkspaceKey   string `json:"workspace_key"`
	ItemNumber     int    `json:"item_number"`
	StatusName     string `json:"status_name,omitempty"`
	StatusColor    string `json:"status_color,omitempty"`
	PriorityName   string `json:"priority_name,omitempty"`
	PriorityColor  string `json:"priority_color,omitempty"`
	AssigneeName   string `json:"assignee_name,omitempty"`
	AssigneeAvatar string `json:"assignee_avatar,omitempty"`
}

type IterationSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`
}

// Iteration is the canonical v2 iteration payload returned by
// /rest/api/v2/iterations and /rest/api/v2/workspaces/{id}/iterations.
type Iteration struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Status      string `json:"status"`
	IsGlobal    bool   `json:"is_global"`
	WorkspaceID *int   `json:"workspace_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type ProjectSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`
}

// ============================================
// Custom Fields
// ============================================

// CustomField is the canonical v2 custom-field payload returned by
// /rest/api/v2/custom-fields. Options is a JSON string for select /
// multiselect fields.
type CustomField struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	FieldType    string `json:"field_type"`
	Description  string `json:"description,omitempty"`
	Options      any    `json:"options"`
	Required     bool   `json:"required"`
	DisplayOrder int    `json:"display_order"`
}

// ============================================
// Item Labels
// ============================================

// Label is a global work-item label, separate from page labels.
type Label struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ItemLabelSetRequest is the body for PUT /rest/api/v2/items/{id}/labels.
type ItemLabelSetRequest struct {
	LabelIDs []int `json:"label_ids"`
}

// ItemLabelAddRequest is the body for POST /rest/api/v2/items/{id}/labels.
type ItemLabelAddRequest struct {
	LabelID int `json:"label_id"`
}

// ItemTemplate mirrors models.ItemTemplate on the wire (WI-438): a workspace
// reusable body that pre-fills a new item's description.
type ItemTemplate struct {
	ID              int    `json:"id"`
	WorkspaceID     int    `json:"workspace_id"`
	Name            string `json:"name"`
	DescriptionBody string `json:"description_body"`
	Mode            string `json:"mode"`
	IsActive        bool   `json:"is_active"`
	ItemTypeIDs     []int  `json:"item_type_ids"`
}

// ItemTemplateListResponse is the envelope from GET
// /rest/api/v2/workspaces/{id}/templates. MandatoryTemplateID is set when the
// list was filtered by item_type_id and that type enforces a mandatory template.
type ItemTemplateListResponse struct {
	Items               []ItemTemplate `json:"data"`
	MandatoryTemplateID *int           `json:"mandatory_template_id,omitempty"`
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
// Diagrams
// ============================================

// Diagram is the wire shape returned by /api/items/{itemId}/diagrams and
// /api/diagrams/{id}. The DiagramData field is opaque text — either an
// Excalidraw scene JSON or a {type:"mermaid",source:...} seed wrapper.
type Diagram struct {
	ID             int       `json:"id"`
	ItemID         int       `json:"item_id"`
	Name           string    `json:"name"`
	DiagramData    string    `json:"diagram_data"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CreatedBy      *int      `json:"created_by,omitempty"`
	UpdatedBy      *int      `json:"updated_by,omitempty"`
	CreatorName    string    `json:"creator_name,omitempty"`
	CreatorEmail   string    `json:"creator_email,omitempty"`
	UpdatedByName  string    `json:"updated_by_name,omitempty"`
	UpdatedByEmail string    `json:"updated_by_email,omitempty"`
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
	PlanID         int        `json:"plan_id"`
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
	PlanID     int    `json:"plan_id"`
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

// ============================================
// Pages (workspace knowledge / wiki)
// ============================================

// Page is the canonical v2 page payload.
type Page struct {
	ID                 int             `json:"id"`
	WorkspaceID        int             `json:"workspace_id"`
	ParentID           *int            `json:"parent_id,omitempty"`
	Title              string          `json:"title"`
	Slug               string          `json:"slug"`
	Metadata           json.RawMessage `json:"metadata"`
	Content            string          `json:"content,omitempty"`
	Excerpt            string          `json:"excerpt,omitempty"`
	ContentHash        string          `json:"content_hash,omitempty"`
	Path               string          `json:"path"`
	Depth              int             `json:"depth"`
	IsHome             bool            `json:"is_home"`
	InheritPermissions bool            `json:"inherit_permissions"`
	CreatedBy          int             `json:"created_by"`
	UpdatedBy          *int            `json:"updated_by,omitempty"`
	ArchivedBy         *int            `json:"archived_by,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
	ArchivedAt         *time.Time      `json:"archived_at,omitempty"`

	// Labels carries any page labels attached to this page (always non-nil
	// in JSON). Lets `ws page list --label foo` filter client-side without
	// a per-page round-trip.
	Labels []PageLabel `json:"labels,omitempty"`
	Links  *PageLinks  `json:"_links,omitempty"`
}

type PageLinks struct {
	Self        string `json:"self"`
	Workspace   string `json:"workspace"`
	History     string `json:"history"`
	Permissions string `json:"permissions"`
}

// PageListResponse is the wire shape of GET /workspaces/:id/pages.
type PageListResponse struct {
	Items []Page `json:"items"`
}

// PageRevision mirrors dto.PageRevisionResponse.
type PageRevision struct {
	ID             int       `json:"id"`
	PageID         int       `json:"page_id"`
	RevisionNumber int       `json:"revision_number"`
	Title          string    `json:"title"`
	Slug           string    `json:"slug"`
	Content        string    `json:"content,omitempty"`
	Excerpt        string    `json:"excerpt,omitempty"`
	ParentID       *int      `json:"parent_id,omitempty"`
	Path           string    `json:"path"`
	Depth          int       `json:"depth"`
	ChangeSummary  string    `json:"change_summary,omitempty"`
	ChangeType     string    `json:"change_type"`
	CreatedBy      int       `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}

// PageHistoryResponse wraps a list of revisions newest-first.
type PageHistoryResponse struct {
	Items []PageRevision `json:"items"`
}

type PagePermissions struct {
	PageID             int              `json:"page_id"`
	InheritPermissions bool             `json:"inherit_permissions"`
	EffectiveLevel     string           `json:"effective_level,omitempty"`
	ACL                []PagePermission `json:"acl"`
}

type PagePermission struct {
	ID              int        `json:"id"`
	PageID          int        `json:"page_id"`
	PrincipalType   string     `json:"principal_type"`
	PrincipalID     int        `json:"principal_id"`
	PermissionLevel string     `json:"permission_level"`
	GrantedBy       *int       `json:"granted_by,omitempty"`
	GrantedAt       *time.Time `json:"granted_at,omitempty"`
}

type PageGrantPermissionRequest struct {
	PrincipalType   string `json:"principal_type"`
	PrincipalID     int    `json:"principal_id"`
	PermissionLevel string `json:"permission_level"`
}

type PageSetInheritanceRequest struct {
	InheritPermissions bool `json:"inherit_permissions"`
}

// PageCreateRequest is the body for POST /workspaces/:id/pages.
type PageCreateRequest struct {
	Title    string `json:"title"`
	Content  string `json:"content,omitempty"`
	ParentID *int   `json:"parent_id,omitempty"`
	IsHome   bool   `json:"is_home,omitempty"`
}

// PageUpdateRequest is the body for PUT /workspaces/:id/pages/:pageId.
// All fields are pointers so callers can leave them unchanged. There is
// no inherit_permissions field — that toggle is an admin-only operation
// with its own endpoint and would not be accepted by the server here.
type PageUpdateRequest struct {
	Title               *string `json:"title,omitempty"`
	Content             *string `json:"content,omitempty"`
	ExpectedContentHash *string `json:"expected_content_hash,omitempty"`
}

// PageDiagram is an immutable attachment-backed Excalidraw or Mermaid diagram
// embedded in a Page's Markdown.
type PageDiagram struct {
	PageID         int             `json:"page_id"`
	AttachmentID   int             `json:"attachment_id"`
	Name           string          `json:"name"`
	Kind           string          `json:"kind"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	ContentHash    string          `json:"content_hash,omitempty"`
	RevisionNumber int             `json:"revision_number,omitempty"`
	CreatedAt      time.Time       `json:"created_at,omitempty"`
}

type PageDiagramCreateRequest struct {
	Name                string          `json:"name"`
	Mermaid             string          `json:"mermaid,omitempty"`
	Excalidraw          json.RawMessage `json:"excalidraw,omitempty"`
	Placement           string          `json:"placement"`
	ExpectedContentHash *string         `json:"expected_content_hash,omitempty"`
}

type PageDiagramUpdateRequest struct {
	Name                string          `json:"name,omitempty"`
	Mermaid             string          `json:"mermaid,omitempty"`
	Excalidraw          json.RawMessage `json:"excalidraw,omitempty"`
	ExpectedContentHash *string         `json:"expected_content_hash,omitempty"`
}

// PageMoveRequest is the body for POST /workspaces/:id/pages/:pageId/move.
// PrevSiblingID / NextSiblingID let callers place the page at a specific
// position among its siblings; both omitted means server-default placement.
type PageMoveRequest struct {
	ParentID               *int `json:"parent_id"`
	DestinationWorkspaceID *int `json:"destination_workspace_id,omitempty"`
	PrevSiblingID          *int `json:"prev_sibling_id,omitempty"`
	NextSiblingID          *int `json:"next_sibling_id,omitempty"`
}

// PageLabel is a workspace-scoped label that attaches to pages only.
// Separate from work-item Label (no shared rows or endpoints).
type PageLabel struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	WorkspaceID int       `json:"workspace_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PageLabelListResponse wraps a list of page labels.
type PageLabelListResponse struct {
	Items []PageLabel `json:"items"`
}

// PageLabelCreateRequest is the body for POST /workspaces/:id/page-labels.
type PageLabelCreateRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// PageLabelUpdateRequest is the body for PUT /workspaces/:id/page-labels/:labelId.
// Pointers so callers can partial-update.
type PageLabelUpdateRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// PageLabelSetRequest is the body for PUT /workspaces/:id/pages/:pageId/labels.
type PageLabelSetRequest struct {
	LabelIDs []int `json:"label_ids"`
}

// PageLabelAddRequest is the body for POST /workspaces/:id/pages/:pageId/labels.
type PageLabelAddRequest struct {
	LabelID int `json:"label_id"`
}

// ============================================
// Links (item ↔ item / item ↔ page / item ↔ test_case)
// ============================================

// LinkType is the wire shape of GET /link-types entries. AllowedEntityTypes
// being nil means "any combination"; a populated list (e.g. ["item","page"]
// for the system "Page" type) gates which source/target entity-type pairs
// are valid.
type LinkType struct {
	ID                 int      `json:"id"`
	BuiltinKey         string   `json:"builtin_key,omitempty"`
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	ForwardLabel       string   `json:"forward_label"`
	ReverseLabel       string   `json:"reverse_label"`
	Color              string   `json:"color,omitempty"`
	IsSystem           bool     `json:"is_system"`
	Active             bool     `json:"active"`
	AllowedEntityTypes []string `json:"allowed_entity_types"`
}

// ItemLink mirrors the server-side ItemLink with the joined display fields
// the link handlers return (link_type_name, source/target titles, status
// names, workspace keys). Only fields the CLI actually reads or surfaces
// are declared — `omitempty` keeps the request shape minimal.
type ItemLink struct {
	ID         int    `json:"id"`
	LinkTypeID int    `json:"link_type_id"`
	SourceType string `json:"source_type"`
	SourceID   int    `json:"source_id"`
	TargetType string `json:"target_type"`
	TargetID   int    `json:"target_id"`

	LinkTypeName         string `json:"link_type_name,omitempty"`
	LinkTypeForwardLabel string `json:"link_type_forward_label,omitempty"`
	LinkTypeReverseLabel string `json:"link_type_reverse_label,omitempty"`

	SourceTitle        string `json:"source_title,omitempty"`
	SourceWorkspaceKey string `json:"source_workspace_key,omitempty"`
	SourceStatusName   string `json:"source_status_name,omitempty"`

	TargetTitle        string `json:"target_title,omitempty"`
	TargetWorkspaceKey string `json:"target_workspace_key,omitempty"`
	TargetStatusName   string `json:"target_status_name,omitempty"`
}

// LinkCreateRequest is the body for POST /links.
type LinkCreateRequest struct {
	LinkTypeID int    `json:"link_type_id"`
	SourceType string `json:"source_type"`
	SourceID   int    `json:"source_id"`
	TargetType string `json:"target_type"`
	TargetID   int    `json:"target_id"`
}

// LinkListResponse is the wire shape of GET /items/{id}/links (and the
// matching page/test_case variants): two parallel arrays split by
// direction relative to the queried entity.
type LinkListResponse struct {
	Outgoing []ItemLink `json:"outgoing"`
	Incoming []ItemLink `json:"incoming"`
}

// Asset is the CLI-side mirror of dto.AssetResponse.
type Asset struct {
	ID                int                   `json:"id"`
	SetID             int                   `json:"set_id"`
	Title             string                `json:"title"`
	Description       string                `json:"description,omitempty"`
	AssetTag          string                `json:"asset_tag,omitempty"`
	AssetTypeID       int                   `json:"asset_type_id"`
	CategoryID        *int                  `json:"category_id,omitempty"`
	StatusID          *int                  `json:"status_id,omitempty"`
	CreatedBy         *int                  `json:"created_by,omitempty"`
	CreatedAt         string                `json:"created_at"`
	UpdatedAt         string                `json:"updated_at"`
	CustomFieldValues map[string]any        `json:"custom_field_values,omitempty"`
	Set               *AssetSetSummary      `json:"set,omitempty"`
	AssetType         *AssetTypeSummary     `json:"asset_type,omitempty"`
	Category          *AssetCategorySummary `json:"category,omitempty"`
	Status            *AssetStatusSummary   `json:"status,omitempty"`
	Creator           *UserSummary          `json:"creator,omitempty"`
	LinkedItemCount   int                   `json:"linked_item_count,omitempty"`
	Warnings          []string              `json:"warnings"`
}

// AssetCreateRequest is the JSON body for POST /asset-sets/{setId}/assets.
type AssetCreateRequest struct {
	Title             string         `json:"title"`
	Description       string         `json:"description,omitempty"`
	AssetTag          string         `json:"asset_tag,omitempty"`
	AssetTypeID       int            `json:"asset_type_id"`
	CategoryID        *int           `json:"category_id,omitempty"`
	StatusID          *int           `json:"status_id,omitempty"`
	CustomFieldValues map[string]any `json:"custom_field_values,omitempty"`
}

// AssetUpdateRequest is the JSON body for PUT /assets/{id}. Pointers so
// "not set" is distinguishable from "set to zero value".
type AssetUpdateRequest struct {
	Title             *string         `json:"title,omitempty"`
	Description       *string         `json:"description,omitempty"`
	AssetTag          *string         `json:"asset_tag,omitempty"`
	AssetTypeID       *int            `json:"asset_type_id,omitempty"`
	CategoryID        *int            `json:"category_id,omitempty"`
	StatusID          *int            `json:"status_id,omitempty"`
	CustomFieldValues *map[string]any `json:"custom_field_values,omitempty"`
}

// AssetSet mirrors dto.AssetSetResponse.
type AssetSet struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	IsDefault      bool   `json:"is_default"`
	AssetTypeCount int    `json:"asset_type_count,omitempty"`
	AssetCount     int    `json:"asset_count,omitempty"`
	UserPermission string `json:"user_permission,omitempty"`
	CreatedBy      *int   `json:"created_by,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// AssetSetSummary is the inline shape used inside Asset.
type AssetSetSummary struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// AssetType mirrors dto.AssetTypeResponse.
type AssetType struct {
	ID           int              `json:"id"`
	SetID        int              `json:"set_id"`
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	Icon         string           `json:"icon,omitempty"`
	Color        string           `json:"color,omitempty"`
	DisplayOrder int              `json:"display_order"`
	IsActive     bool             `json:"is_active"`
	AssetCount   int              `json:"asset_count,omitempty"`
	Fields       []AssetTypeField `json:"fields"`
	CreatedAt    string           `json:"created_at"`
	UpdatedAt    string           `json:"updated_at"`
}

// AssetTypeField mirrors dto.AssetTypeFieldResponse.
type AssetTypeField struct {
	ID               int    `json:"id"`
	CustomFieldID    int    `json:"custom_field_id"`
	FieldName        string `json:"field_name"`
	FieldType        string `json:"field_type"`
	FieldDescription string `json:"field_description,omitempty"`
	Options          string `json:"options,omitempty"`
	IsRequired       bool   `json:"is_required"`
	DisplayOrder     int    `json:"display_order"`
}

// AssetTypeSummary is the inline shape used inside Asset.
type AssetTypeSummary struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

// AssetCategory mirrors dto.AssetCategoryResponse.
type AssetCategory struct {
	ID               int    `json:"id"`
	SetID            int    `json:"set_id"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	ParentID         *int   `json:"parent_id,omitempty"`
	Path             string `json:"path,omitempty"`
	HasChildren      bool   `json:"has_children"`
	ChildrenCount    int    `json:"children_count"`
	DescendantsCount int    `json:"descendants_count"`
	AssetCount       int    `json:"asset_count,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// AssetCategorySummary is the inline shape used inside Asset.
type AssetCategorySummary struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// AssetStatus mirrors dto.AssetStatusResponse.
type AssetStatus struct {
	ID           int    `json:"id"`
	SetID        int    `json:"set_id"`
	Name         string `json:"name"`
	Color        string `json:"color,omitempty"`
	Description  string `json:"description,omitempty"`
	IsDefault    bool   `json:"is_default"`
	DisplayOrder int    `json:"display_order"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// AssetStatusSummary is the inline shape used inside Asset.
type AssetStatusSummary struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// TimeProject is the canonical v2 time-project payload.
type TimeProject struct {
	ID            int            `json:"id"`
	CustomerID    *int           `json:"customer_id,omitempty"`
	CategoryID    *int           `json:"category_id,omitempty"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Status        string         `json:"status"`
	Color         string         `json:"color,omitempty"`
	HourlyRate    float64        `json:"hourly_rate"`
	Settings      map[string]any `json:"settings,omitempty"`
	CustomerName  string         `json:"customer_name,omitempty"`
	CategoryName  string         `json:"category_name,omitempty"`
	CategoryColor string         `json:"category_color,omitempty"`
	TotalHours    *float64       `json:"total_hours,omitempty"`
}

// TimeWorklog is the canonical v2 worklog payload.
type TimeWorklog struct {
	ID                  int      `json:"id"`
	ProjectID           int      `json:"project_id"`
	CustomerID          int      `json:"customer_id"`
	ItemID              *int     `json:"item_id,omitempty"`
	Description         string   `json:"description"`
	Date                int64    `json:"date"`
	StartTime           int64    `json:"start_time"`
	EndTime             int64    `json:"end_time"`
	DurationMinutes     int      `json:"duration_minutes"`
	CreatedAt           int64    `json:"created_at"`
	UpdatedAt           int64    `json:"updated_at"`
	CustomerName        string   `json:"customer_name,omitempty"`
	ProjectName         string   `json:"project_name,omitempty"`
	ItemTitle           string   `json:"item_title,omitempty"`
	WorkspaceID         *int     `json:"workspace_id,omitempty"`
	WorkspaceKey        string   `json:"workspace_key,omitempty"`
	WorkspaceItemNumber int      `json:"workspace_item_number,omitempty"`
	ProjectMaxHours     *float64 `json:"project_max_hours,omitempty"`
	ProjectTotalHours   *float64 `json:"project_total_hours,omitempty"`
}

// TimeWorklogCreateRequest is the canonical v2 create-worklog request body.
type TimeWorklogCreateRequest struct {
	ProjectID       int    `json:"project_id"`
	Description     string `json:"description"`
	Date            string `json:"date"`
	Duration        string `json:"duration,omitempty"`
	DurationMinutes int    `json:"duration_minutes,omitempty"`
	StartTime       string `json:"start_time,omitempty"`
	EndTime         string `json:"end_time,omitempty"`
	ItemID          *int   `json:"item_id,omitempty"`
	ItemKey         string `json:"item_key,omitempty"`
}

// TimerStartRequest is the v2 start-timer request body.
type TimerStartRequest struct {
	WorkspaceID int    `json:"workspace_id"`
	ProjectID   int    `json:"project_id"`
	ItemID      *int   `json:"item_id,omitempty"`
	Description string `json:"description"`
}

type AssetCSVUpload struct {
	UploadID  string   `json:"upload_id"`
	Headers   []string `json:"headers"`
	TotalRows int      `json:"total_rows"`
	Delimiter string   `json:"delimiter"`
}

type AssetImportProgress struct {
	Phase         string   `json:"phase"`
	TotalRows     int      `json:"total_rows"`
	ImportedCount int      `json:"imported_count"`
	FailedCount   int      `json:"failed_count"`
	Errors        []string `json:"errors,omitempty"`
}

// AssetImportJob is the v2 asynchronous import job.
type AssetImportJob struct {
	JobID        string               `json:"job_id"`
	Status       string               `json:"status"`
	Phase        string               `json:"phase,omitempty"`
	Progress     *AssetImportProgress `json:"progress,omitempty"`
	ErrorMessage string               `json:"error_message,omitempty"`
	CreatedAt    *time.Time           `json:"created_at,omitempty"`
	StartedAt    *time.Time           `json:"started_at,omitempty"`
	CompletedAt  *time.Time           `json:"completed_at,omitempty"`
}
