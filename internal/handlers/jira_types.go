package handlers

import (
	"windshift/internal/jira"
	"windshift/internal/jiraimport"
)

// ================================================================
// Jira Import Request/Response Types
// ================================================================

// JiraConnectRequest represents a request to connect to Jira
type JiraConnectRequest struct {
	InstanceURL    string `json:"instance_url"`
	Email          string `json:"email"`           // Required for Cloud; unused for Data Center PATs
	APIToken       string `json:"api_token"`       // Cloud API token or Data Center PAT
	DeploymentType string `json:"deployment_type"` // "cloud" or "datacenter" (default: "cloud")
}

// JiraConnectResponse represents a successful connection response
type JiraConnectResponse struct {
	ConnectionID string                 `json:"connection_id"`
	InstanceInfo *jira.JiraInstanceInfo `json:"instance_info"`
}

// JiraConnectionUpdateRequest replaces the credentials of an existing
// connection. The instance URL and deployment type identify the tenant and
// cannot change through this endpoint.
type JiraConnectionUpdateRequest struct {
	Email    string `json:"email"`
	APIToken string `json:"api_token"`
}

// JiraProjectInfo contains information about a Jira project for display.
// IssueCount is a pointer so the JSON omits it when the caller hasn't asked
// for counts — distinguishes "not loaded yet" from "zero issues" on the
// frontend, which uses two separate calls to keep the project list responsive.
type JiraProjectInfo struct {
	Key           string `json:"key"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ProjectType   string `json:"project_type"`
	IssueCount    *int   `json:"issue_count,omitempty"`
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
	Projects                  []JiraProjectAnalysis                  `json:"projects"`
	IssueTypes                []JiraIssueTypeInfo                    `json:"issue_types"`
	Statuses                  []JiraStatusInfo                       `json:"statuses"`
	CustomFields              []jira.FieldMappingSuggestion          `json:"custom_fields"`
	Users                     []JiraUserSummary                      `json:"users"`
	Versions                  []JiraVersionInfo                      `json:"versions"`
	AssetSchemas              []JiraAssetSchemaInfo                  `json:"asset_schemas,omitempty"`
	ServiceManagementProjects []JiraServiceManagementProjectAnalysis `json:"service_management_projects,omitempty"`
	MappingTargets            jiraimport.MappingTargets              `json:"mapping_targets"`
	Xray                      JiraXrayAnalysis                       `json:"xray"`
	TotalIssues               int                                    `json:"total_issues"`
	TotalAssets               int                                    `json:"total_assets"`
	OpenIssuesOnly            bool                                   `json:"open_issues_only"`
}

// JiraXrayAnalysis reports only positively identified Xray Tests. A detection
// status of unavailable is distinct from not_detected so callers never fall
// back to issue-type display names after an upstream authorization or app
// failure.
type JiraXrayAnalysis struct {
	DetectionStatus    string                    `json:"detection_status"` // detected, not_detected, unavailable
	RequiresCredential bool                      `json:"requires_credential"`
	TotalTests         int                       `json:"total_tests"`
	TestIssueTypeIDs   []string                  `json:"test_issue_type_ids,omitempty"`
	Projects           []JiraXrayProjectAnalysis `json:"projects,omitempty"`
	WarningCode        string                    `json:"warning_code,omitempty"`
}

// JiraXrayProjectAnalysis contains the number of Xray Tests discovered in one
// selected Jira project.
type JiraXrayProjectAnalysis struct {
	ProjectKey string `json:"project_key"`
	TestCount  int    `json:"test_count"`
}

// JiraServiceManagementProjectAnalysis describes the portal-specific entities
// discovered for one selected service project.
type JiraServiceManagementProjectAnalysis struct {
	ProjectKey          string                         `json:"project_key"`
	ServiceDeskID       string                         `json:"service_desk_id"`
	RequestTypeCount    int                            `json:"request_type_count"`
	OrganizationCount   int                            `json:"organization_count"`
	OrganizationMembers int                            `json:"organization_member_count"`
	Organizations       []JiraCustomerOrganizationInfo `json:"organizations"`
}

// JiraCustomerOrganizationInfo is deliberately summary-only; customer
// account IDs stay server-side and are fetched again only when the operator
// confirms organization import.
type JiraCustomerOrganizationInfo struct {
	JiraID        string `json:"jira_id"`
	Name          string `json:"name"`
	CustomerCount int    `json:"customer_count"`
}

// JiraReadinessRequest is the body for POST /api/admin/jira-import/readiness.
type JiraReadinessRequest struct {
	ConnectionID   string   `json:"connection_id"`
	ProjectKeys    []string `json:"project_keys"`
	OpenIssuesOnly bool     `json:"open_issues_only"`
	SampleSize     int      `json:"sample_size,omitempty"` // issues sampled per project; default 200, capped at 500
}

// JiraReadinessReport is the full migration-readiness assessment returned by
// the analyser. Scores and findings are computed from a *sample* of each
// project's issues — Extrapolated is true whenever any project had more issues
// than were sampled, so the UI never presents partial coverage as complete.
type JiraReadinessReport struct {
	Projects        []JiraProjectReadiness `json:"projects"`
	OverallScore    int                    `json:"overall_score"`
	FindingsBySev   map[jira.Severity]int  `json:"findings_by_severity"`
	TotalIssues     int                    `json:"total_issues"`
	SampledIssues   int                    `json:"sampled_issues"`
	Extrapolated    bool                   `json:"extrapolated"`
	AttachmentBytes int64                  `json:"attachment_bytes_sampled"`
	OpenIssuesOnly  bool                   `json:"open_issues_only"`
}

// JiraProjectReadiness is the per-project slice of the readiness assessment.
type JiraProjectReadiness struct {
	Key           string         `json:"key"`
	Name          string         `json:"name"`
	TotalIssues   int            `json:"total_issues"`
	SampledIssues int            `json:"sampled_issues"`
	Score         int            `json:"score"`
	Findings      []jira.Finding `json:"findings"`
}

// JiraProjectAnalysis contains analysis for a single project
type JiraProjectAnalysis struct {
	Key                   string   `json:"key"`
	Name                  string   `json:"name"`
	IssueCount            int      `json:"issue_count"`
	IssueTypes            []string `json:"issue_types"`
	HasVersions           bool     `json:"has_versions"`
	VersionCount          int      `json:"version_count"`
	HasSprints            bool     `json:"has_sprints"`
	IsTeamManaged         bool     `json:"is_team_managed"`
	WorkspaceKeyCollision bool     `json:"workspace_key_collision"`
	SuggestedWorkspaceKey string   `json:"suggested_workspace_key"`
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
	Key         string `json:"key"`
	Name        string `json:"name"`
	SetName     string `json:"set_name"`
	Description string `json:"description"`
	ObjectCount int    `json:"object_count"`
	TypeCount   int    `json:"type_count"`
}

// JiraUserSummary contains summary info about a Jira user for import
type JiraUserSummary struct {
	AccountID     string `json:"account_id"`
	AccountType   string `json:"account_type,omitempty"`
	Email         string `json:"email"`
	DisplayName   string `json:"display_name"`
	AvatarURL     string `json:"avatar_url"`
	MatchedUserID *int   `json:"matched_user_id,omitempty"` // Existing Windshift user ID if matched
}

// StartImportRequest is the request body for POST /api/admin/jira-import/start
type StartImportRequest struct {
	ConnectionID   string            `json:"connection_id"`
	ProjectKeys    []string          `json:"project_keys"`
	OpenIssuesOnly bool              `json:"open_issues_only"`
	Mappings       ImportMappings    `json:"mappings"`
	Xray           XrayImportOptions `json:"xray"`
	ForceReimport  bool              `json:"force_reimport,omitempty"`
}

// XrayImportOptions contains the operator's conditional Xray choice. Cloud
// credentials are kept only in the in-memory request passed to the background
// import and are deliberately excluded from jira_import_jobs.config_json.
type XrayImportOptions struct {
	ImportTests      bool     `json:"import_tests"`
	Region           string   `json:"region,omitempty"`
	ClientID         string   `json:"client_id,omitempty"`
	ClientSecret     string   `json:"client_secret,omitempty"`
	TestIssueTypeIDs []string `json:"test_issue_type_ids,omitempty"`
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
	Workspaces        []WorkspaceMapping       `json:"workspaces"`
	IssueTypes        []IssueTypeMapping       `json:"issueTypes"`
	Statuses          []StatusMapping          `json:"statuses"`
	CustomFields      []CustomFieldMapping     `json:"customFields"`
	Versions          []VersionMapping         `json:"versions"`
	ServiceManagement ServiceManagementMapping `json:"serviceManagement"`
}

// ServiceManagementMapping contains opt-in choices for JSM-only entities.
// Portals, request types, and portal customers are intrinsic to a service
// project import; organizations require explicit confirmation.
type ServiceManagementMapping struct {
	ImportOrganizations bool `json:"importOrganizations"`
}

// WorkspaceMapping maps a Jira project to a Windshift workspace
type WorkspaceMapping struct {
	JiraKey              string `json:"jiraKey"`
	JiraName             string `json:"jiraName"`
	IssueCount           int    `json:"issueCount"`
	WindshiftID          *int   `json:"windshiftId,omitempty"`
	CreateNew            bool   `json:"createNew"`
	NewWorkspaceName     string `json:"newWorkspaceName,omitempty"`
	NewWorkspaceKey      string `json:"newWorkspaceKey,omitempty"`
	KeyAliasAcknowledged bool   `json:"keyAliasAcknowledged"`
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
	PreserveRaw   bool   `json:"preserveRaw,omitempty"`
	// AssetSchemaID controls Jira Assets field mapping. "auto" infers the
	// single asset set from all populated issue values, "text" preserves
	// display values without a native relationship, and a Jira object schema
	// ID explicitly selects that imported Windshift asset set.
	AssetSchemaID string `json:"assetSchemaId,omitempty"`
}

// ImportProgress tracks the progress of an import job.
type ImportProgress = jiraimport.Progress

// StartImportResponse is returned when starting an import
type StartImportResponse struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
}
