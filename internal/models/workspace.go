package models

import "time"

// Workspace represents a project workspace
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

// WorkspaceScreen represents a screen assignment for a workspace
type WorkspaceScreen struct {
	ID          int    `json:"id"`
	WorkspaceID int    `json:"workspace_id"`
	ScreenID    int    `json:"screen_id"`
	Context     string `json:"context"` // create, edit, view
	// Joined fields for API responses
	ScreenName    string `json:"screen_name,omitempty"`
	WorkspaceName string `json:"workspace_name,omitempty"`
}

// WorkspaceConfigurationSet represents the relationship between workspace and configuration set
type WorkspaceConfigurationSet struct {
	ID                 int       `json:"id"`
	WorkspaceID        int       `json:"workspace_id"`
	ConfigurationSetID int       `json:"configuration_set_id"`
	CreatedAt          time.Time `json:"created_at"`
	// Joined fields for API responses
	WorkspaceName        string `json:"workspace_name,omitempty"`
	ConfigurationSetName string `json:"configuration_set_name,omitempty"`
}

// Project represents a time tracking project
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

// MilestoneCategory represents a category for organizing milestones
type MilestoneCategory struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // Hex color code (e.g., "#3b82f6")
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Milestone represents a project milestone
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

// IterationType represents a type of iteration (sprint, week, etc.)
type IterationType struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // Hex color code (e.g., "#3b82f6")
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Iteration represents a time-boxed iteration (sprint)
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
