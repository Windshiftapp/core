package models

import (
	"encoding/json"
	"strings"
	"time"
)

// Workspace is a container for Items and Pages plus their configuration.
// "Project" in Windshift refers exclusively to time-tracking projects
// (table time_projects); a Workspace is not a project.
type Workspace struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Key         string `json:"key"` // Workspace key for item prefixes (e.g., "WI", "TEST")
	Description string `json:"description"`
	Active      bool   `json:"active"`
	// Personal workspace fields
	IsPersonal bool `json:"is_personal"`        // Flag to indicate personal workspace
	OwnerID    *int `json:"owner_id,omitempty"` // User ID for personal workspaces
	// Template flag: workspace can be selected as a source when creating workspaces
	IsTemplate bool `json:"is_template"`
	// Time tracking integration
	TimeProjectID *int `json:"time_project_id,omitempty"` // Default time-tracking project for worklogs on items in this workspace
	// Visual identity fields
	Icon                    string    `json:"icon"`                      // Lucide icon name for workspace
	Color                   string    `json:"color"`                     // Hex color code for workspace
	AvatarURL               *string   `json:"avatar_url,omitempty"`      // Custom avatar image URL
	HomepageLayout          *string   `json:"homepage_layout,omitempty"` // JSON object with sections and widgets
	DefaultView             string    `json:"default_view,omitempty"`    // Default view when entering workspace (board, backlog, list, tree, map)
	InternalCommentsEnabled bool      `json:"internal_comments_enabled"` // Allow internal comments on all items
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
	// Joined fields for API responses
	TimeProjectName       string `json:"time_project_name,omitempty"`
	OwnerName             string `json:"owner_name,omitempty"` // Name of workspace owner for API responses
	ConfigurationSetID    *int64 `json:"configuration_set_id,omitempty"`
	TimeProjectCategories []int  `json:"time_project_categories,omitempty"` // Restricted time project categories for this workspace
}

// WorkspaceTemplateSummary is the picker-facing summary of a workspace marked
// as a template: identity fields plus counts of what a clone would receive.
type WorkspaceTemplateSummary struct {
	ID                   int    `json:"id"`
	Name                 string `json:"name"`
	Description          string `json:"description"`
	Icon                 string `json:"icon"`
	Color                string `json:"color"`
	ConfigurationSetName string `json:"configuration_set_name"`
	TemplateCount        int    `json:"template_count"`
	ItemCount            int    `json:"item_count"`
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
	ID        string         `json:"id"`               // UUID for client-side tracking
	Type      string         `json:"type"`             // Widget type: "stats", "completion-chart", "created-chart", "milestone-progress", etc.
	SectionID string         `json:"section_id"`       // Which section this widget belongs to
	Position  int            `json:"position"`         // Display order within the section
	Width     int            `json:"width"`            // Column span: 1, 2, or 3 (for grid-based sections)
	Config    map[string]any `json:"config,omitempty"` // Widget-specific configuration
}

// WorkspaceHomepageLayout represents the complete homepage layout structure
type WorkspaceHomepageLayout struct {
	Sections           []WorkspaceHomepageSection `json:"sections"`                     // Sections on the homepage
	Widgets            []WorkspaceWidget          `json:"widgets"`                      // All widgets across all sections
	Gradient           int                        `json:"gradient"`                     // Selected gradient index (0 = none, 1-17 = gradient index)
	ApplyToAllViews    bool                       `json:"applyToAllViews"`              // If true, apply gradient to all workspace views
	BackgroundImageURL string                     `json:"backgroundImageUrl,omitempty"` // Custom background image URL (takes priority over gradient)
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

// MilestoneRelease represents a release record for a milestone
type MilestoneRelease struct {
	ID                    int               `json:"id"`
	MilestoneID           int               `json:"milestone_id"`
	WorkspaceRepositoryID *int              `json:"workspace_repository_id,omitempty"`
	TagName               string            `json:"tag_name"`
	TagURL                *string           `json:"tag_url,omitempty"`
	ReleaseStatus         string            `json:"release_status,omitempty"`
	ReleasedAt            *string           `json:"released_at,omitempty"`
	Assets                []SCMReleaseAsset `json:"assets,omitempty"`
	LastSyncedAt          *string           `json:"last_synced_at,omitempty"`
	Name                  string            `json:"name,omitempty"`
	Body                  string            `json:"body,omitempty"`
	IsDraft               bool              `json:"is_draft"`
	IsPrerelease          bool              `json:"is_prerelease"`
	TargetCommitish       string            `json:"target_commitish,omitempty"`
	SCMConnectionID       *int              `json:"scm_connection_id,omitempty"` // Visible only to users with connection workspace access
	SCMRepository         *string           `json:"scm_repository,omitempty"`    // "owner/repo"; same visibility as SCMConnectionID
	SCMReleaseID          *string           `json:"scm_release_id,omitempty"`
	SCMReleaseURL         *string           `json:"scm_release_url,omitempty"`
	CreatedBy             *int              `json:"created_by,omitempty"`
	CreatedAt             string            `json:"created_at"`
}

// Milestone represents a project milestone
type Milestone struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	TargetDate  *string `json:"target_date,omitempty"` // Date in YYYY-MM-DD format, nullable
	//nolint:misspell // British spelling used in database
	Status      string    `json:"status"` // planning, in-progress, completed, cancelled
	CategoryID  *int      `json:"category_id,omitempty"`
	IsGlobal    bool      `json:"is_global"`              // false=local to workspace, true=global (shared)
	WorkspaceID *int      `json:"workspace_id,omitempty"` // NULL if global, set if local to workspace
	ExternalKey *string   `json:"external_key,omitempty"` // Stable upsert key set by automation (e.g. tag short-name); unique per workspace when set
	Position    int       `json:"position"`               // Manual sort order (drag-and-drop), scoped per (is_global, workspace_id, category_id)
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Joined fields for API responses
	CategoryName  string             `json:"category_name,omitempty"`
	CategoryColor string             `json:"category_color,omitempty"`
	WorkspaceName string             `json:"workspace_name,omitempty"`
	LatestRelease *MilestoneRelease  `json:"latest_release,omitempty"`
	Releases      []MilestoneRelease `json:"releases,omitempty"`
}

// IterationType categorizes an Iteration (e.g. "Sprint", "Week", "Cycle", "Release").
type IterationType struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"` // Hex color code (e.g., "#3b82f6")
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Iteration represents a time-boxed period (start + end date) for organizing work, typed by an IterationType.
type Iteration struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	StartDate   string `json:"start_date"` // YYYY-MM-DD format
	EndDate     string `json:"end_date"`   // YYYY-MM-DD format
	//nolint:misspell // British spelling used in database
	Status      string    `json:"status"` // planned, active, completed, cancelled
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

// NullableIntPatch distinguishes an omitted JSON field from an explicit null.
// This is required for nullable foreign keys such as iteration.type_id.
type NullableIntPatch struct {
	Present bool
	Value   *int
}

func (p *NullableIntPatch) UnmarshalJSON(data []byte) error {
	p.Present = true
	if string(data) == "null" {
		p.Value = nil
		return nil
	}
	var value int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	p.Value = &value
	return nil
}

// NullableStringPatch is the string counterpart of NullableIntPatch, for
// nullable columns such as milestone.target_date. An empty string reads as a
// clear too, since a blank date is never a meaningful stored value.
type NullableStringPatch struct {
	Present bool
	Value   *string
}

func (p *NullableStringPatch) UnmarshalJSON(data []byte) error {
	p.Present = true
	if string(data) == "null" {
		p.Value = nil
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		p.Value = nil
		return nil
	}
	p.Value = &value
	return nil
}

// MilestonePatch is the presence-aware update contract for milestones. Empty
// description is a valid clear, null (or blank) target_date and null
// category_id are valid clears, and omitted fields preserve their stored
// values.
type MilestonePatch struct {
	Name        *string             `json:"name,omitempty"`
	Description *string             `json:"description,omitempty"`
	TargetDate  NullableStringPatch `json:"target_date,omitempty" swaggertype:"string" extensions:"x-nullable"`
	Status      *string             `json:"status,omitempty"`
	CategoryID  NullableIntPatch    `json:"category_id,omitempty" swaggertype:"integer" extensions:"x-nullable"`
}

func (p MilestonePatch) Apply(existing Milestone) Milestone {
	if p.Name != nil {
		existing.Name = *p.Name
	}
	if p.Description != nil {
		existing.Description = *p.Description
	}
	if p.TargetDate.Present {
		existing.TargetDate = p.TargetDate.Value
	}
	if p.Status != nil {
		existing.Status = *p.Status
	}
	if p.CategoryID.Present {
		existing.CategoryID = p.CategoryID.Value
	}
	return existing
}

// IterationPatch is the presence-aware update contract shared by the legacy
// and v1 adapters. Empty description is a valid clear, null type_id is a valid
// clear, and omitted fields preserve their stored values.
type IterationPatch struct {
	Name        *string          `json:"name,omitempty"`
	Description *string          `json:"description,omitempty"`
	StartDate   *string          `json:"start_date,omitempty"`
	EndDate     *string          `json:"end_date,omitempty"`
	Status      *string          `json:"status,omitempty"`
	TypeID      NullableIntPatch `json:"type_id,omitempty" swaggertype:"integer" extensions:"x-nullable"`
}

func (p IterationPatch) Apply(existing Iteration) Iteration {
	if p.Name != nil {
		existing.Name = *p.Name
	}
	if p.Description != nil {
		existing.Description = *p.Description
	}
	if p.StartDate != nil {
		existing.StartDate = *p.StartDate
	}
	if p.EndDate != nil {
		existing.EndDate = *p.EndDate
	}
	if p.Status != nil {
		existing.Status = *p.Status
	}
	if p.TypeID.Present {
		existing.TypeID = p.TypeID.Value
	}
	return existing
}

// BoardConfiguration represents a board layout configuration for a collection
type BoardConfiguration struct {
	ID                         int            `json:"id"`
	CollectionID               *int           `json:"collection_id,omitempty"`
	WorkspaceID                *int           `json:"workspace_id,omitempty"`
	BacklogStatusIDs           []int          `json:"backlog_status_ids,omitempty"`
	ListColumns                []ListColumn   `json:"list_columns,omitempty"`
	CardFields                 []ListColumn   `json:"card_fields,omitempty"`
	RoadmapConfig              *RoadmapConfig `json:"roadmap_config,omitempty"`
	ShowRightmostColumnLast50  bool           `json:"show_rightmost_column_last_50"`
	CompletedItemRetentionDays *int           `json:"completed_item_retention_days,omitempty"`
	CreatedAt                  time.Time      `json:"created_at"`
	UpdatedAt                  time.Time      `json:"updated_at"`
	// Joined fields
	Columns []BoardColumn `json:"columns,omitempty"`
}

// RoadmapConfig represents the configuration for a roadmap view
type RoadmapConfig struct {
	StartFieldID         string `json:"start_field_id"`
	EndFieldID           string `json:"end_field_id"`
	DependencyLinkTypeID *int   `json:"dependency_link_type_id,omitempty"`
}

// ListColumn represents a column configuration for list view
type ListColumn struct {
	FieldIdentifier string `json:"field_identifier"` // 'key', 'title', 'status', 'custom_field_123'
	FieldType       string `json:"field_type"`       // 'system' or 'custom'
	DisplayOrder    int    `json:"display_order"`
	Width           int    `json:"width"` // grid column span (1-4)
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
	Columns                    []BoardColumnRequest `json:"columns"`
	BacklogStatusIDs           []int                `json:"backlog_status_ids,omitempty"`
	ListColumns                []ListColumn         `json:"list_columns,omitempty"`
	CardFields                 []ListColumn         `json:"card_fields,omitempty"`
	RoadmapConfig              *RoadmapConfig       `json:"roadmap_config,omitempty"`
	ShowRightmostColumnLast50  bool                 `json:"show_rightmost_column_last_50"`
	CompletedItemRetentionDays *int                 `json:"completed_item_retention_days"`
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
