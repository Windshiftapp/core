package data

import (
	"fmt"
	"time"
)

// UserInfo carries the authenticated identity plumbed through from SSH.
type UserInfo struct {
	UserID         int
	CredentialID   string
	CredentialName string
	RemoteAddr     string
	Email          string
	Username       string
	FirstName      string
	LastName       string
	Timezone       string
}

// Prefs is the per-user TUI preferences document persisted server-side
// (`data.tui` from v2 /users/me/preferences). Pointer fields distinguish unset.
type Prefs struct {
	Theme           string   `json:"theme,omitempty"`
	SplitRatio      *float64 `json:"split_ratio,omitempty"`
	LastWorkspaceID *int     `json:"last_workspace_id,omitempty"`
}

// These DTOs contain the subset of the canonical v2 contract used by the TUI.

type paginationDocument struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

type dataDocument[T any] struct {
	Data T `json:"data"`
}

type workspacePageDocument struct {
	Data       []workspaceDTO     `json:"data"`
	Pagination paginationDocument `json:"pagination"`
}

type itemPageDocument struct {
	Data       []itemDTO          `json:"data"`
	Pagination paginationDocument `json:"pagination"`
}

type userSummaryDTO struct {
	ID        int    `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	FullName  string `json:"full_name"`
}

type statusSummaryDTO struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	CategoryID    int    `json:"category_id"`
	CategoryName  string `json:"category_name,omitempty"`
	CategoryColor string `json:"category_color,omitempty"`
}

type statusDTO struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Category struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Color string `json:"color"`
	} `json:"category"`
}

type priorityDTO struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

type workspaceDTO struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Key           string `json:"key"`
	Description   string `json:"description"`
	Active        bool   `json:"active"`
	TimeProjectID *int   `json:"time_project_id,omitempty"`
}

type itemDTO struct {
	ID                  int               `json:"id"`
	WorkspaceID         int               `json:"workspace_id"`
	WorkspaceKey        string            `json:"workspace_key"`
	Key                 string            `json:"key"`
	WorkspaceItemNumber int               `json:"workspace_item_number"`
	Title               string            `json:"title"`
	Description         string            `json:"description"`
	ParentID            *int              `json:"parent_id,omitempty"`
	Status              *statusSummaryDTO `json:"status,omitempty"`
	Priority            *priorityDTO      `json:"priority,omitempty"`
	Assignee            *userSummaryDTO   `json:"assignee,omitempty"`
	Creator             *userSummaryDTO   `json:"creator,omitempty"`
	Transitions         []transitionDTO   `json:"transitions,omitempty"`
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
}

type transitionDTO struct {
	ToStatusID int               `json:"to_status_id"`
	ToStatus   *statusSummaryDTO `json:"to_status,omitempty"`
}

type currentUserDTO struct {
	Timezone string `json:"timezone"`
}

type commentDTO struct {
	ID        int             `json:"id"`
	ItemID    int             `json:"item_id"`
	Content   string          `json:"content"`
	Author    *userSummaryDTO `json:"author,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type assignableUserDTO struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	FullName  string `json:"full_name"`
	IsActive  bool   `json:"is_active"`
	IsAgent   bool   `json:"is_agent"`
	AvatarURL string `json:"avatar_url"`
}

type agentRunDTO struct {
	ID        int        `json:"id"`
	Status    string     `json:"status"`
	JobKind   string     `json:"job_kind"`
	QueuedAt  time.Time  `json:"queued_at"`
	StartedAt *time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	Error     string     `json:"error"`
}

// TUI domain types are kept independent from their wire DTOs.

// User is an assignable user for the assignee picker.
type User struct {
	ID       int
	Username string
	FullName string
	IsAgent  bool
}

// AgentRun is one coding-agent execution against a work item.
type AgentRun struct {
	ID        int
	Status    string // queued|running|succeeded|failed|canceled|killed
	JobKind   string
	QueuedAt  string
	StartedAt string
	EndedAt   string
	Error     string
}

// Workspace represents a workspace from the Windshift API.
type Workspace struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Key           string `json:"key"`
	Description   string `json:"description"`
	Active        bool   `json:"active"`
	TimeProjectID *int   `json:"time_project_id"`
}

// Status represents a workflow status
type Status struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	CategoryID    int    `json:"category_id"`
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color"`
}

// Priority represents a priority level
type Priority struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	Color     string `json:"color"`
	SortOrder int    `json:"sort_order"`
}

type WorkItem struct {
	ID                  int            `json:"id"`
	WorkspaceID         int            `json:"workspace_id"`
	Key                 string         `json:"key"`
	WorkspaceItemNumber int            `json:"workspace_item_number"`
	ItemTypeID          *int           `json:"item_type_id"`
	Title               string         `json:"title"`
	Description         string         `json:"description"`
	Status              string         `json:"status"`                // Legacy text field
	Priority            string         `json:"priority"`              // Legacy text field
	StatusID            *int           `json:"status_id,omitempty"`   // ID-based status
	PriorityID          *int           `json:"priority_id,omitempty"` // ID-based priority
	MilestoneID         *int           `json:"milestone_id"`
	TimeProjectID       *int           `json:"time_project_id"`
	AssigneeID          *int           `json:"assignee_id"`
	CreatorID           *int           `json:"creator_id"`
	CustomFieldValues   map[string]any `json:"custom_field_values"`
	ParentID            *int           `json:"parent_id"`
	Path                string         `json:"path"`
	Rank                *string        `json:"rank"`
	CreatedAt           string         `json:"created_at"`
	UpdatedAt           string         `json:"updated_at"`
	// Joined fields for display
	WorkspaceName   string `json:"workspace_name"`
	WorkspaceKey    string `json:"workspace_key"`
	ItemTypeName    string `json:"item_type_name"`
	ParentTitle     string `json:"parent_title"`
	MilestoneName   string `json:"milestone_name"`
	TimeProjectName string `json:"time_project_name"`
	AssigneeName    string `json:"assignee_name"`
	AssigneeEmail   string `json:"assignee_email"`
	CreatorName     string `json:"creator_name"`
	CreatorEmail    string `json:"creator_email"`
	// ID-based status/priority display fields
	StatusName          string   `json:"status_name,omitempty"`
	StatusCategoryColor string   `json:"category_color,omitempty"`
	PriorityName        string   `json:"priority_name,omitempty"`
	PriorityIcon        string   `json:"priority_icon,omitempty"`
	PriorityColor       string   `json:"priority_color,omitempty"`
	Transitions         []Status `json:"transitions,omitempty"`
}

// DisplayKey returns the canonical API key, falling back only for legacy
// fixtures that do not provide one.
func (wi *WorkItem) DisplayKey(workspaceKey string) string {
	if wi.Key != "" {
		return wi.Key
	}
	key := wi.WorkspaceKey
	if key == "" {
		key = workspaceKey
	}
	number := wi.WorkspaceItemNumber
	if number == 0 {
		number = wi.ID
	}
	return fmt.Sprintf("%s-%d", key, number)
}

// GetLevel calculates hierarchy level from path. Items without a path stay at
// the root level.
func (wi *WorkItem) GetLevel() int {
	if wi.Path == "" {
		return 0
	}
	// Path format is like "/1/5/12/" - count slashes minus 1
	level := 0
	for _, char := range wi.Path {
		if char == '/' {
			level++
		}
	}
	// Subtract 1 because path starts and ends with /
	return level - 1
}

type Comment struct {
	ID          int     `json:"id"`
	ItemID      int     `json:"item_id"`
	AuthorID    int     `json:"author_id"`
	Content     string  `json:"content"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
	AuthorName  *string `json:"author_name"`
	AuthorEmail *string `json:"author_email"`
}

type TimeProject struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	CustomerName *string `json:"customer_name"`
	Status       string  `json:"status"`
}

// Wire-to-domain converters sanitize server-provided text for terminal output.

func workspaceFromDTO(w workspaceDTO) Workspace {
	return Workspace{
		ID:            w.ID,
		Name:          SanitizeLine(w.Name),
		Key:           SanitizeLine(w.Key),
		Description:   SanitizeText(w.Description),
		Active:        w.Active,
		TimeProjectID: w.TimeProjectID,
	}
}

func workItemFromDTO(it itemDTO) WorkItem {
	wi := WorkItem{
		ID:                  it.ID,
		WorkspaceID:         it.WorkspaceID,
		WorkspaceKey:        SanitizeLine(it.WorkspaceKey),
		Key:                 SanitizeLine(it.Key),
		WorkspaceItemNumber: it.WorkspaceItemNumber,
		Title:               SanitizeLine(it.Title),
		Description:         SanitizeText(it.Description),
		ParentID:            it.ParentID,
		CreatedAt:           it.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           it.UpdatedAt.Format(time.RFC3339),
	}
	if it.Status != nil {
		id := it.Status.ID
		wi.StatusID = &id
		wi.StatusName = SanitizeLine(it.Status.Name)
		wi.StatusCategoryColor = SanitizeLine(it.Status.CategoryColor)
		wi.Status = wi.StatusName
	}
	if it.Priority != nil {
		id := it.Priority.ID
		wi.PriorityID = &id
		wi.PriorityName = SanitizeLine(it.Priority.Name)
		wi.PriorityIcon = SanitizeLine(it.Priority.Icon)
		wi.PriorityColor = SanitizeLine(it.Priority.Color)
		wi.Priority = wi.PriorityName
	}
	if it.Assignee != nil {
		id := it.Assignee.ID
		wi.AssigneeID = &id
		wi.AssigneeName = SanitizeLine(it.Assignee.FullName)
		wi.AssigneeEmail = SanitizeLine(it.Assignee.Email)
	}
	if it.Creator != nil {
		id := it.Creator.ID
		wi.CreatorID = &id
		wi.CreatorName = SanitizeLine(it.Creator.FullName)
		wi.CreatorEmail = SanitizeLine(it.Creator.Email)
	}
	for _, transition := range it.Transitions {
		if transition.ToStatus == nil {
			continue
		}
		wi.Transitions = append(wi.Transitions, Status{
			ID:            transition.ToStatusID,
			Name:          SanitizeLine(transition.ToStatus.Name),
			CategoryID:    transition.ToStatus.CategoryID,
			CategoryName:  SanitizeLine(transition.ToStatus.CategoryName),
			CategoryColor: SanitizeLine(transition.ToStatus.CategoryColor),
		})
	}
	return wi
}

func commentFromDTO(c commentDTO) Comment {
	out := Comment{
		ID:        c.ID,
		ItemID:    c.ItemID,
		Content:   SanitizeText(c.Content),
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
	}
	if c.Author != nil {
		out.AuthorID = c.Author.ID
		name := SanitizeLine(c.Author.FullName)
		email := SanitizeLine(c.Author.Email)
		out.AuthorName = &name
		out.AuthorEmail = &email
	}
	return out
}
