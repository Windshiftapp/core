package handlers

import (
	"time"

	"windshift/internal/jira"
)

// ================================================================
// Jira Import Request/Response Types
// ================================================================

// JiraConnectRequest represents a request to connect to Jira
type JiraConnectRequest struct {
	InstanceURL    string `json:"instance_url"`
	Email          string `json:"email"`           // Email (Cloud) or username (Data Center)
	APIToken       string `json:"api_token"`       // API token (Cloud) or password/token (Data Center)
	DeploymentType string `json:"deployment_type"` // "cloud" or "datacenter" (default: "cloud")
}

// JiraConnectResponse represents a successful connection response
type JiraConnectResponse struct {
	ConnectionID string                 `json:"connection_id"`
	InstanceInfo *jira.JiraInstanceInfo `json:"instance_info"`
}

// JiraProjectInfo contains information about a Jira project for display
type JiraProjectInfo struct {
	Key           string `json:"key"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ProjectType   string `json:"project_type"`
	IssueCount    int    `json:"issue_count"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	IsTeamManaged bool   `json:"is_team_managed"` // True for next-gen/team-managed projects
}

// JiraAnalyzeRequest contains the projects to analyze
type JiraAnalyzeRequest struct {
	ConnectionID   string   `json:"connection_id"`
	ProjectKeys    []string `json:"project_keys"`
	OpenIssuesOnly bool     `json:"open_issues_only"`
}

// JiraVersionInfo contains version/release information from Jira
type JiraVersionInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Archived    bool   `json:"archived"`
	Released    bool   `json:"released"`
	ReleaseDate string `json:"release_date,omitempty"`
	ProjectKey  string `json:"project_key"`
}

// JiraAnalysisResult contains the full analysis of selected projects
type JiraAnalysisResult struct {
	Projects       []JiraProjectAnalysis         `json:"projects"`
	IssueTypes     []JiraIssueTypeInfo           `json:"issue_types"`
	Statuses       []JiraStatusInfo              `json:"statuses"`
	CustomFields   []jira.FieldMappingSuggestion `json:"custom_fields"`
	Users          []JiraUserSummary             `json:"users"`
	Versions       []JiraVersionInfo             `json:"versions"`
	AssetSchemas   []JiraAssetSchemaInfo         `json:"asset_schemas,omitempty"`
	TotalIssues    int                           `json:"total_issues"`
	TotalAssets    int                           `json:"total_assets"`
	OpenIssuesOnly bool                          `json:"open_issues_only"`
}

// JiraProjectAnalysis contains analysis for a single project
type JiraProjectAnalysis struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	IssueCount   int      `json:"issue_count"`
	IssueTypes   []string `json:"issue_types"`
	HasVersions  bool     `json:"has_versions"`
	VersionCount int      `json:"version_count"`
	HasSprints   bool     `json:"has_sprints"`
}

// JiraIssueTypeInfo contains issue type information
type JiraIssueTypeInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Subtask        bool   `json:"subtask"`
	HierarchyLevel int    `json:"hierarchy_level"`
	UsageCount     int    `json:"usage_count"`
}

// JiraStatusInfo contains status information
type JiraStatusInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CategoryID   int    `json:"category_id"`
	CategoryName string `json:"category_name"`
	CategoryKey  string `json:"category_key"`
	Color        string `json:"color"`
}

// JiraAssetSchemaInfo contains asset schema information
type JiraAssetSchemaInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ObjectCount int    `json:"object_count"`
	TypeCount   int    `json:"type_count"`
}

// JiraUserSummary contains summary info about a Jira user for import
type JiraUserSummary struct {
	AccountID     string `json:"account_id"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	AvatarURL     string `json:"avatar_url"`
	MatchedUserID *int   `json:"matched_user_id,omitempty"` // Existing Windshift user ID if matched
}

// ImportJobStatus represents the status of an import job
type ImportJobStatus struct {
	JobID        string                 `json:"job_id"`
	Status       string                 `json:"status"`
	Phase        string                 `json:"phase,omitempty"`
	Progress     map[string]interface{} `json:"progress,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
}

// StartImportRequest is the request body for POST /api/jira-import/start
type StartImportRequest struct {
	ConnectionID   string         `json:"connection_id"`
	ProjectKeys    []string       `json:"project_keys"`
	OpenIssuesOnly bool           `json:"open_issues_only"`
	Mappings       ImportMappings `json:"mappings"`
}

// VersionMapping maps a Jira version to a Windshift milestone
type VersionMapping struct {
	JiraID      string `json:"jiraId"`
	JiraName    string `json:"jiraName"`
	ProjectKey  string `json:"projectKey"`
	Released    bool   `json:"released"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	CreateNew   bool   `json:"createNew"`
}

// ImportMappings contains all the mapping configurations
type ImportMappings struct {
	Workspaces   []WorkspaceMapping   `json:"workspaces"`
	IssueTypes   []IssueTypeMapping   `json:"issueTypes"`
	Statuses     []StatusMapping      `json:"statuses"`
	CustomFields []CustomFieldMapping `json:"customFields"`
	Versions     []VersionMapping     `json:"versions"`
}

// WorkspaceMapping maps a Jira project to a Windshift workspace
type WorkspaceMapping struct {
	JiraKey          string `json:"jiraKey"`
	JiraName         string `json:"jiraName"`
	IssueCount       int    `json:"issueCount"`
	WindshiftID      *int   `json:"windshiftId,omitempty"`
	CreateNew        bool   `json:"createNew"`
	NewWorkspaceName string `json:"newWorkspaceName,omitempty"`
	NewWorkspaceKey  string `json:"newWorkspaceKey,omitempty"`
}

// IssueTypeMapping maps a Jira issue type to a Windshift item type
type IssueTypeMapping struct {
	JiraIDs        []string `json:"jiraIds"`
	JiraName       string   `json:"jiraName"`
	IsSubtask      bool     `json:"isSubtask"`
	HierarchyLevel int      `json:"hierarchyLevel"`
	WindshiftID    *int     `json:"windshiftId,omitempty"`
	CreateNew      bool     `json:"createNew"`
}

// StatusMapping maps a Jira status to a Windshift status
type StatusMapping struct {
	JiraIDs      []string `json:"jiraIds"`
	JiraName     string   `json:"jiraName"`
	CategoryKey  string   `json:"categoryKey"`
	CategoryName string   `json:"categoryName"`
	Color        string   `json:"color"`
	WindshiftID  *int     `json:"windshiftId,omitempty"`
	CreateNew    bool     `json:"createNew"`
}

// CustomFieldMapping maps a Jira custom field to a Windshift custom field
type CustomFieldMapping struct {
	JiraID        string `json:"jiraId"`
	JiraName      string `json:"jiraName"`
	JiraType      string `json:"jiraType"`
	WindshiftType string `json:"windshiftType"`
	CanMap        bool   `json:"canMap"`
	Notes         string `json:"notes,omitempty"`
	Action        string `json:"action"` // 'create', 'map', 'skip'
	WindshiftID   *int   `json:"windshiftId,omitempty"`
}

// ImportProgress tracks the progress of an import job
type ImportProgress struct {
	Phase               string `json:"phase"`
	CurrentProject      string `json:"current_project,omitempty"`
	TotalProjects       int    `json:"total_projects"`
	CompletedProjects   int    `json:"completed_projects"`
	TotalIssues         int    `json:"total_issues"`
	ImportedIssues      int    `json:"imported_issues"`
	FailedIssues        int    `json:"failed_issues"`
	TotalAttachments    int    `json:"total_attachments"`
	ImportedAttachments int    `json:"imported_attachments"`
	TotalComments       int    `json:"total_comments"`
	ImportedComments    int    `json:"imported_comments"`
}

// StartImportResponse is returned when starting an import
type StartImportResponse struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
}

// ConnectionInfo represents a saved connection for the UI
type ConnectionInfo struct {
	ID             string     `json:"id"`
	InstanceURL    string     `json:"instance_url"`
	Email          string     `json:"email"`
	InstanceName   string     `json:"instance_name"`
	DeploymentType string     `json:"deployment_type"` // "cloud" or "datacenter"
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}

// ImportJobInfo represents an import job for the UI
type ImportJobInfo struct {
	ID           string                 `json:"id"`
	ConnectionID string                 `json:"connection_id"`
	InstanceURL  string                 `json:"instance_url,omitempty"`
	InstanceName string                 `json:"instance_name,omitempty"`
	Status       string                 `json:"status"`
	Phase        string                 `json:"phase,omitempty"`
	Scope        string                 `json:"scope"`
	Progress     map[string]interface{} `json:"progress,omitempty"`
	Result       map[string]interface{} `json:"result,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
}
