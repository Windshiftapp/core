package models

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// SCMProviderType represents the type of SCM provider
type SCMProviderType string

const (
	SCMProviderTypeGitHub SCMProviderType = "github"
	SCMProviderTypeGitea  SCMProviderType = "gitea"
)

// SCMAuthMethod represents the authentication method for an SCM provider
type SCMAuthMethod string

const (
	SCMAuthMethodOAuth     SCMAuthMethod = "oauth"
	SCMAuthMethodPAT       SCMAuthMethod = "pat"
	SCMAuthMethodGitHubApp SCMAuthMethod = "github_app"
)

// SCMProvider represents a configured SCM provider (GitHub, GitLab, etc.)
type SCMProvider struct {
	ID                           int             `json:"id"`
	Slug                         string          `json:"slug"`
	Name                         string          `json:"name"`
	ProviderType                 SCMProviderType `json:"provider_type"`
	AuthMethod                   SCMAuthMethod   `json:"auth_method"`
	Enabled                      bool            `json:"enabled"`
	IsDefault                    bool            `json:"is_default"`
	BaseURL                      string          `json:"base_url,omitempty"`
	OAuthClientID                string          `json:"oauth_client_id,omitempty"`
	OAuthClientSecretEncrypted   string          `json:"-"` // Never expose encrypted secrets
	PersonalAccessTokenEncrypted string          `json:"-"`
	GitHubAppID                  string          `json:"github_app_id,omitempty"`
	GitHubAppPrivateKeyEncrypted string          `json:"-"`
	GitHubAppInstallationID      string          `json:"github_app_installation_id,omitempty"`
	GitHubOrgID                  *int64          `json:"github_org_id,omitempty"` // Stable org ID for GitHub App discovery
	Scopes                       string          `json:"scopes"`
	WorkspaceRestrictionMode     string          `json:"workspace_restriction_mode"` // 'unrestricted' or 'restricted'
	CreatedAt                    time.Time       `json:"created_at"`
	UpdatedAt                    time.Time       `json:"updated_at"`
	HasOAuthClientSecret         bool            `json:"has_oauth_client_secret,omitempty"`
	HasPAT                       bool            `json:"has_pat,omitempty"`
	HasGitHubAppPrivateKey       bool            `json:"has_github_app_private_key,omitempty"`
}

// SCMProviderRequest represents the API request for creating/updating an SCM provider
type SCMProviderRequest struct {
	Slug                     string          `json:"slug"`
	Name                     string          `json:"name"`
	ProviderType             SCMProviderType `json:"provider_type"`
	AuthMethod               SCMAuthMethod   `json:"auth_method"`
	Enabled                  bool            `json:"enabled"`
	IsDefault                bool            `json:"is_default"`
	BaseURL                  string          `json:"base_url,omitempty"`
	OAuthClientID            string          `json:"oauth_client_id,omitempty"`
	OAuthClientSecret        string          `json:"oauth_client_secret,omitempty"` // Plaintext, will be encrypted
	PersonalAccessToken      string          `json:"personal_access_token,omitempty"`
	GitHubAppID              string          `json:"github_app_id,omitempty"`
	GitHubAppPrivateKey      string          `json:"github_app_private_key,omitempty"`
	GitHubAppInstallationID  string          `json:"github_app_installation_id,omitempty"`
	GitHubOrgID              *int64          `json:"github_org_id,omitempty"` // Stable org ID for GitHub App discovery
	Scopes                   string          `json:"scopes,omitempty"`
	WorkspaceRestrictionMode string          `json:"workspace_restriction_mode,omitempty"` // 'unrestricted' or 'restricted'
}

// SCMProviderWorkspaceAllowlist represents a workspace allowed to use an SCM provider
type SCMProviderWorkspaceAllowlist struct {
	ID            int       `json:"id"`
	ProviderID    int       `json:"provider_id"`
	WorkspaceID   int       `json:"workspace_id"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     *int      `json:"created_by,omitempty"`
	WorkspaceName string    `json:"workspace_name,omitempty"`
	WorkspaceKey  string    `json:"workspace_key,omitempty"`
}

// SCMOAuthState represents a temporary OAuth state token
type SCMOAuthState struct {
	ID          int       `json:"id"`
	ProviderID  int       `json:"provider_id"`
	State       string    `json:"state"`
	RedirectURI string    `json:"redirect_uri"`
	UserID      int       `json:"user_id"`
	WorkspaceID *int      `json:"workspace_id,omitempty"` // If set, store credentials at workspace level
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// WorkspaceSCMConnection represents a connection between a workspace and an SCM provider
type WorkspaceSCMConnection struct {
	ID                           int             `json:"id"`
	WorkspaceID                  int             `json:"workspace_id"`
	SCMProviderID                int             `json:"scm_provider_id"`
	Enabled                      bool            `json:"enabled"`
	SmartCommitsEnabled          bool            `json:"smart_commits_enabled"`
	DefaultBranchPattern         string          `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern               string          `json:"item_key_pattern,omitempty"`
	CreatedBy                    *int            `json:"created_by,omitempty"`
	CreatedAt                    time.Time       `json:"created_at"`
	UpdatedAt                    time.Time       `json:"updated_at"`
	OAuthAccessTokenEncrypted    string          `json:"-"`
	OAuthRefreshTokenEncrypted   string          `json:"-"`
	OAuthTokenExpiresAt          *time.Time      `json:"oauth_token_expires_at,omitempty"`
	PersonalAccessTokenEncrypted string          `json:"-"`
	HasOAuthToken                bool            `json:"has_oauth_token,omitempty"`
	HasPAT                       bool            `json:"has_pat,omitempty"`
	ProviderName                 string          `json:"provider_name,omitempty"`
	ProviderType                 SCMProviderType `json:"provider_type,omitempty"`
	ProviderSlug                 string          `json:"provider_slug,omitempty"`
}

// WorkspaceSCMConnectionRequest represents the API request for workspace SCM connections
type WorkspaceSCMConnectionRequest struct {
	SCMProviderID        int    `json:"scm_provider_id"`
	Enabled              bool   `json:"enabled"`
	DefaultBranchPattern string `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       string `json:"item_key_pattern,omitempty"`
}

// WorkspaceRepository represents a repository linked to a workspace
type WorkspaceRepository struct {
	ID                       int             `json:"id"`
	WorkspaceSCMConnectionID int             `json:"workspace_scm_connection_id"`
	RepositoryExternalID     string          `json:"repository_external_id"`
	RepositoryName           string          `json:"repository_name"`
	RepositoryURL            string          `json:"repository_url"`
	DefaultBranch            string          `json:"default_branch"`
	IsActive                 bool            `json:"is_active"`
	LastSyncedAt             *time.Time      `json:"last_synced_at,omitempty"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
	WorkspaceID              int             `json:"workspace_id,omitempty"`
	ProviderType             SCMProviderType `json:"provider_type,omitempty"`
	ProviderSlug             string          `json:"provider_slug,omitempty"`
}

// WorkspaceRepositoryRequest represents the API request for linking a repository
type WorkspaceRepositoryRequest struct {
	RepositoryExternalID string `json:"repository_external_id"`
	RepositoryName       string `json:"repository_name"`
	RepositoryURL        string `json:"repository_url"`
	DefaultBranch        string `json:"default_branch,omitempty"`
}

// SCMLinkType represents the type of SCM link
type SCMLinkType string

const (
	SCMLinkTypePullRequest SCMLinkType = "pull_request"
	SCMLinkTypeCommit      SCMLinkType = "commit"
	SCMLinkTypeBranch      SCMLinkType = "branch"
)

// SCMLinkState represents the state of a PR
type SCMLinkState string

const (
	SCMLinkStateOpen   SCMLinkState = "open"
	SCMLinkStateClosed SCMLinkState = "closed"
	SCMLinkStateMerged SCMLinkState = "merged"
)

// ItemSCMLink represents a link between an item and an SCM resource (PR, commit, branch)
type ItemSCMLink struct {
	ID                    int             `json:"id"`
	ItemID                int             `json:"item_id"`
	WorkspaceRepositoryID int             `json:"workspace_repository_id"`
	LinkType              SCMLinkType     `json:"link_type"`
	ExternalID            string          `json:"external_id"`
	ExternalURL           string          `json:"external_url,omitempty"`
	Title                 string          `json:"title,omitempty"`
	State                 SCMLinkState    `json:"state,omitempty"`
	AuthorExternalID      string          `json:"author_external_id,omitempty"`
	AuthorName            string          `json:"author_name,omitempty"`
	DetectionSource       string          `json:"detection_source,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
	RepositoryName        string          `json:"repository_name,omitempty"`
	RepositoryURL         string          `json:"repository_url,omitempty"`
	ProviderType          SCMProviderType `json:"provider_type,omitempty"`
}

// ItemSCMLinkRequest represents the API request for creating an SCM link
type ItemSCMLinkRequest struct {
	WorkspaceRepositoryID int         `json:"workspace_repository_id"`
	LinkType              SCMLinkType `json:"link_type"`
	ExternalID            string      `json:"external_id"`
	ExternalURL           string      `json:"external_url,omitempty"`
	Title                 string      `json:"title,omitempty"`
	State                 string      `json:"state,omitempty"`
	AuthorName            string      `json:"author_name,omitempty"`
}

// SCMWebhook represents a registered webhook for a repository
type SCMWebhook struct {
	ID                     int        `json:"id"`
	WorkspaceRepositoryID  int        `json:"workspace_repository_id"`
	WebhookExternalID      string     `json:"webhook_external_id,omitempty"`
	WebhookSecretEncrypted string     `json:"-"`      // Never expose
	Events                 string     `json:"events"` // JSON array
	IsActive               bool       `json:"is_active"`
	LastDeliveryAt         *time.Time `json:"last_delivery_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// SCMWebhookDelivery represents a webhook delivery record
type SCMWebhookDelivery struct {
	ID               int       `json:"id"`
	SCMWebhookID     int       `json:"scm_webhook_id"`
	DeliveryID       string    `json:"delivery_id,omitempty"`
	EventType        string    `json:"event_type"`
	PayloadSummary   string    `json:"payload_summary,omitempty"`
	Status           string    `json:"status"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	ProcessingTimeMs int       `json:"processing_time_ms,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// IssueSyncLabelMode represents the label sync mode
type IssueSyncLabelMode string

const (
	IssueSyncLabelMirror IssueSyncLabelMode = "mirror"
	IssueSyncLabelMapped IssueSyncLabelMode = "mapped"
	IssueSyncLabelNone   IssueSyncLabelMode = "none"
)

// IssueSyncConfig represents a per-workspace-repository issue sync configuration
type IssueSyncConfig struct {
	ID                    int                `json:"id"`
	WorkspaceRepositoryID int                `json:"workspace_repository_id"`
	SyncEnabled           bool               `json:"sync_enabled"`
	StatusMapping         string             `json:"status_mapping"`
	ReverseStatusMapping  string             `json:"reverse_status_mapping"`
	LabelSyncMode         IssueSyncLabelMode `json:"label_sync_mode"`
	LabelMappings         string             `json:"label_mappings"`
	FilterLabels          string             `json:"filter_labels"`
	AssigneeMappings      string             `json:"assignee_mappings"`
	MilestoneMappings     string             `json:"milestone_mappings"`
	DefaultItemTypeID     *int               `json:"default_item_type_id,omitempty"`
	DefaultPriorityID     *int               `json:"default_priority_id,omitempty"`
	SyncComments          bool               `json:"sync_comments"`
	LastFullSyncAt        *time.Time         `json:"last_full_sync_at,omitempty"`
	LastSyncError         string             `json:"last_sync_error,omitempty"`
	CreatedBy             *int               `json:"created_by,omitempty"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
	RepositoryName        string             `json:"repository_name,omitempty"`
	WorkspaceID           int                `json:"workspace_id,omitempty"`
	SyncedItemCount       int                `json:"synced_item_count,omitempty"`
}

// IssueSyncConfigRequest represents the API request for creating/updating an issue sync config
type IssueSyncConfigRequest struct {
	WorkspaceRepositoryID int                `json:"workspace_repository_id"`
	SyncEnabled           bool               `json:"sync_enabled"`
	StatusMapping         string             `json:"status_mapping"`
	ReverseStatusMapping  string             `json:"reverse_status_mapping"`
	LabelSyncMode         IssueSyncLabelMode `json:"label_sync_mode"`
	LabelMappings         string             `json:"label_mappings"`
	FilterLabels          string             `json:"filter_labels"`
	AssigneeMappings      string             `json:"assignee_mappings"`
	MilestoneMappings     string             `json:"milestone_mappings"`
	DefaultItemTypeID     *int               `json:"default_item_type_id,omitempty"`
	DefaultPriorityID     *int               `json:"default_priority_id,omitempty"`
	SyncComments          bool               `json:"sync_comments"`
}

// IssueSyncItem represents a mapping between a GitHub Issue and a Windshift Item
type IssueSyncItem struct {
	ID                  int        `json:"id"`
	IssueSyncConfigID   int        `json:"issue_sync_config_id"`
	ItemID              int        `json:"item_id"`
	GitHubIssueNumber   int        `json:"github_issue_number"`
	GitHubIssueID       int64      `json:"github_issue_id"`
	GitHubIssueURL      string     `json:"github_issue_url"`
	LastSyncedAt        *time.Time `json:"last_synced_at,omitempty"`
	LastGitHubUpdatedAt *time.Time `json:"last_github_updated_at,omitempty"`
	SyncLock            bool       `json:"sync_lock"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ItemTitle           string     `json:"item_title,omitempty"`
	WorkspaceItemNumber int        `json:"workspace_item_number,omitempty"`
	WorkspaceKey        string     `json:"workspace_key,omitempty"`
}

// LabelMapping represents a mapping between a GitHub label and a Windshift label
type LabelMapping struct {
	GitHubLabel      string `json:"github_label"`
	WindshiftLabelID int    `json:"windshift_label_id"`
}

// TimeProjectCategory represents a category for time projects
type TimeProjectCategory struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Color        string    `json:"color,omitempty"`
	DisplayOrder int       `json:"display_order"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TimeProject represents a time tracking project
type TimeProject struct {
	ID            int            `json:"id"`
	CustomerID    *int           `json:"customer_id,omitempty"` // Now optional
	CategoryID    *int           `json:"category_id,omitempty"` // Link to project category
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Status        string         `json:"status"` // Active, On Hold, Completed, Archived
	Color         string         `json:"color,omitempty"`
	HourlyRate    float64        `json:"hourly_rate"`
	Settings      map[string]any `json:"settings,omitempty"` // Flexible JSON attributes (e.g., max_hours)
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	CustomerName  string         `json:"customer_name,omitempty"`
	CategoryName  string         `json:"category_name,omitempty"`
	CategoryColor string         `json:"category_color,omitempty"`
	TotalHours    *float64       `json:"total_hours,omitempty"` // Computed from worklogs
	IsManager     bool           `json:"is_manager,omitempty"`  // Whether current user is a manager of this project
}

// Worklog represents a time tracking entry
type Worklog struct {
	ID                  int      `json:"id"`
	ProjectID           int      `json:"project_id"`
	CustomerID          int      `json:"customer_id"`
	UserID              *int     `json:"user_id,omitempty"` // User who created the worklog
	ItemID              *int     `json:"item_id,omitempty"` // Optional link to work item
	Description         string   `json:"description"`
	Date                int64    `json:"date"`       // Unix timestamp
	StartTime           int64    `json:"start_time"` // Unix timestamp
	EndTime             int64    `json:"end_time"`   // Unix timestamp
	DurationMins        int      `json:"duration_minutes"`
	CreatedAt           int64    `json:"created_at"` // Unix timestamp
	UpdatedAt           int64    `json:"updated_at"` // Unix timestamp
	CustomerName        string   `json:"customer_name,omitempty"`
	ProjectName         string   `json:"project_name,omitempty"`
	UserName            string   `json:"user_name,omitempty"`             // Name of user who created the worklog
	ItemTitle           string   `json:"item_title,omitempty"`            // Title of linked work item
	WorkspaceID         *int     `json:"workspace_id,omitempty"`          // Workspace ID of linked item
	WorkspaceKey        string   `json:"workspace_key,omitempty"`         // Workspace key for navigation (e.g., "TEST")
	WorkspaceItemNumber int      `json:"workspace_item_number,omitempty"` // Item number for display key (e.g., "TEST-123")
	ProjectMaxHours     *float64 `json:"project_max_hours,omitempty"`     // Project budget limit for indicator
	ProjectTotalHours   *float64 `json:"project_total_hours,omitempty"`   // Project total hours for indicator
}

// ActiveTimer represents a running timer
type ActiveTimer struct {
	ID                  int     `json:"id"`
	WorkspaceID         int     `json:"workspace_id"`
	ItemID              *int    `json:"item_id,omitempty"` // Optional link to work item
	ProjectID           int     `json:"project_id"`
	UserID              int     `json:"user_id"`
	Description         string  `json:"description"`
	StartTimeUTC        int64   `json:"start_time_utc"` // Unix timestamp in UTC
	CreatedAt           int64   `json:"created_at"`     // Unix timestamp
	ProjectName         *string `json:"project_name,omitempty"`
	CustomerName        *string `json:"customer_name,omitempty"`
	ItemTitle           *string `json:"item_title,omitempty"`
	WorkspaceName       *string `json:"workspace_name,omitempty"`
	WorkspaceKey        *string `json:"workspace_key,omitempty"`
	WorkspaceItemNumber *int    `json:"workspace_item_number,omitempty"`
}

// Review represents a daily or weekly personal review entry
type Review struct {
	ID         int       `json:"id"`
	UserID     int       `json:"user_id"`
	ReviewDate string    `json:"review_date"` // YYYY-MM-DD format
	ReviewType string    `json:"review_type"` // 'daily' or 'weekly'
	ReviewData string    `json:"review_data"` // JSON data - unstructured storage
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	UserName   string    `json:"user_name,omitempty"`
	UserEmail  string    `json:"user_email,omitempty"`
}

// ReviewCreateRequest represents the payload for creating a new review
type ReviewCreateRequest struct {
	ReviewDate string `json:"review_date"` // YYYY-MM-DD format
	ReviewType string `json:"review_type"` // 'daily' or 'weekly'
	ReviewData string `json:"review_data"` // JSON data
}

// ReviewUpdateRequest represents the payload for updating a review
type ReviewUpdateRequest struct {
	ReviewData string `json:"review_data"` // JSON data
}

// CompletedItemsRequest represents the query parameters for getting completed items
type CompletedItemsRequest struct {
	StartDate string `json:"start_date"` // YYYY-MM-DD format
	EndDate   string `json:"end_date"`   // YYYY-MM-DD format
	UserID    int    `json:"user_id"`    // Filter by assignee
}

// TestFolder represents a folder for organizing test cases
type TestFolder struct {
	ID            int       `json:"id"`
	WorkspaceID   int       `json:"workspace_id"`
	ParentID      *int      `json:"parent_id,omitempty"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	SortOrder     int       `json:"sort_order"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	TestCaseCount int       `json:"test_case_count,omitempty"`
}

// TestCase represents a test case
type TestCase struct {
	ID                int         `json:"id"`
	WorkspaceID       int         `json:"workspace_id"`
	FolderID          *int        `json:"folder_id,omitempty"`
	Title             string      `json:"title"`
	Name              string      `json:"name"`
	Priority          string      `json:"priority"`           // low, medium, high, critical
	Status            string      `json:"status"`             // active, inactive, draft
	EstimatedDuration int         `json:"estimated_duration"` // in seconds
	Preconditions     string      `json:"preconditions"`
	SortOrder         int         `json:"sort_order"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
	FolderName        string      `json:"folder_name,omitempty"`
	TestSteps         []TestStep  `json:"test_steps,omitempty"`
	Labels            []TestLabel `json:"labels,omitempty"`
}

// TestSet represents a collection of test cases
type TestSet struct {
	ID            int       `json:"id"`
	WorkspaceID   int       `json:"workspace_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	MilestoneID   *int      `json:"milestone_id"`
	MilestoneName string    `json:"milestone_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	TestCaseCount int `json:"test_case_count,omitempty"`

	TotalRuns      int        `json:"total_runs,omitempty"`
	SuccessfulRuns int        `json:"successful_runs,omitempty"`
	FailedRuns     int        `json:"failed_runs,omitempty"`
	LastRunStatus  string     `json:"last_run_status,omitempty"`
	LastRunDate    *time.Time `json:"last_run_date,omitempty"`
}

// MilestoneTestStats aggregates test-plan activity for a milestone.
type MilestoneTestStats struct {
	TotalTestPlans     int `json:"total_test_plans"`
	TotalTestRuns      int `json:"total_test_runs"`
	SuccessfulTestRuns int `json:"successful_test_runs"`
	FailedTestRuns     int `json:"failed_test_runs"`
	InProgressTestRuns int `json:"in_progress_test_runs"`
	TotalTestCases     int `json:"total_test_cases"`
}

// SetTestCase represents the relationship between a test set and a test case
type SetTestCase struct {
	ID         int `json:"id"`
	SetID      int `json:"set_id"`
	TestCaseID int `json:"test_case_id"`
}

// TestRunTemplate represents a template for test runs
type TestRunTemplate struct {
	ID          int       `json:"id"`
	WorkspaceID int       `json:"workspace_id"`
	SetID       int       `json:"set_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	SetName     string    `json:"set_name,omitempty"`
}

// TestRun represents an execution of a test set
type TestRun struct {
	ID             int        `json:"id"`
	WorkspaceID    int        `json:"workspace_id"`
	TemplateID     int        `json:"template_id,omitempty"` // Optional reference to template
	SetID          int        `json:"set_id"`
	Name           string     `json:"name"`
	AssigneeID     *int       `json:"assignee_id,omitempty"` // User assigned to execute this run
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at"`
	CreatedAt      time.Time  `json:"created_at"`
	AssigneeName   string     `json:"assignee_name,omitempty"`
	AssigneeEmail  string     `json:"assignee_email,omitempty"`
	AssigneeAvatar string     `json:"assignee_avatar,omitempty"`
}

// TestResult represents the result of a test case execution
type TestResult struct {
	ID           int        `json:"id"`
	RunID        int        `json:"run_id"`
	TestCaseID   int        `json:"test_case_id"`
	Status       string     `json:"status"` // "passed", "failed", "blocked", "skipped", "not_run"
	ActualResult string     `json:"actual_result"`
	Notes        string     `json:"notes"`
	ExecutedAt   *time.Time `json:"executed_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TestStep represents a single step in a test case
type TestStep struct {
	ID         int       `json:"id"`
	TestCaseID int       `json:"test_case_id"`
	StepNumber int       `json:"step_number"`
	Action     string    `json:"action"`
	Data       string    `json:"data"`
	Expected   string    `json:"expected"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TestStepResult represents the result of a single test step execution
type TestStepResult struct {
	ID           int        `json:"id"`
	TestResultID int        `json:"test_result_id"`
	TestStepID   int        `json:"test_step_id"`
	Status       string     `json:"status"` // "passed", "failed", "blocked", "skipped"
	ActualResult string     `json:"actual_result"`
	Notes        string     `json:"notes"`
	ItemID       *int       `json:"item_id,omitempty"` // Link to work item (e.g., Bug)
	ExecutedAt   *time.Time `json:"executed_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TestResultItem represents the junction table for linking test results to work items
type TestResultItem struct {
	ID           int       `json:"id"`
	TestResultID int       `json:"test_result_id"`
	ItemID       int       `json:"item_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// TestLabel represents a label for organizing test cases
type TestLabel struct {
	ID          int       `json:"id"`
	WorkspaceID int       `json:"workspace_id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TestCaseLabel represents the relationship between a test case and a label
type TestCaseLabel struct {
	ID         int       `json:"id"`
	TestCaseID int       `json:"test_case_id"`
	LabelID    int       `json:"label_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// TestCoverageConfiguration represents the requirement type configuration for a collection or workspace
type TestCoverageConfiguration struct {
	ID                     int       `json:"id"`
	WorkspaceID            *int      `json:"workspace_id,omitempty"`
	CollectionID           *int      `json:"collection_id,omitempty"`
	RequirementItemTypeIDs []int     `json:"requirement_item_type_ids"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// TestCoverageConfigRequest represents the payload for creating/updating test coverage config
type TestCoverageConfigRequest struct {
	RequirementItemTypeIDs []int `json:"requirement_item_type_ids"`
}

// TestCoverageSummary represents the coverage statistics for pie chart
type TestCoverageSummary struct {
	Total        int     `json:"total"`
	Covered      int     `json:"covered"`
	NotCovered   int     `json:"not_covered"`
	CoverageRate float64 `json:"coverage_rate"`
}

// RequirementCoverageItem represents a single requirement with its coverage status
type RequirementCoverageItem struct {
	ItemID           int    `json:"item_id"`
	WorkspaceKey     string `json:"workspace_key"`
	WorkspaceItemNum int    `json:"workspace_item_number"`
	Title            string `json:"title"`
	ItemTypeID       int    `json:"item_type_id"`
	ItemTypeName     string `json:"item_type_name"`
	ItemTypeIcon     string `json:"item_type_icon"`
	ItemTypeColor    string `json:"item_type_color"`
	StatusID         *int   `json:"status_id,omitempty"`
	StatusName       string `json:"status_name,omitempty"`
	IsCovered        bool   `json:"is_covered"`
	LinkedTestCount  int    `json:"linked_test_count"`
}

// TestCoverageListResponse represents the paginated response for requirements list
type TestCoverageListResponse struct {
	Items      []RequirementCoverageItem `json:"items"`
	Pagination PaginationMeta            `json:"pagination"`
	Summary    TestCoverageSummary       `json:"summary"`
}

// RecurrenceRule represents a recurring task pattern for generating instances
type RecurrenceRule struct {
	ID             int `json:"id"`
	TemplateItemID int `json:"template_item_id"`
	WorkspaceID    int `json:"workspace_id"`

	RRule    string     `json:"rrule"`   // e.g., "FREQ=WEEKLY;BYDAY=MO,WE,FR"
	DtStart  time.Time  `json:"dtstart"` // Recurrence start date
	DtEnd    *time.Time `json:"dtend,omitempty"`
	Timezone string     `json:"timezone"` // IANA timezone

	LeadTimeDays        int        `json:"lead_time_days"`
	LastGeneratedUntil  *time.Time `json:"last_generated_until,omitempty"`
	NextGenerationCheck *time.Time `json:"next_generation_check,omitempty"`

	CopyAssignee     bool `json:"copy_assignee"`
	CopyPriority     bool `json:"copy_priority"`
	CopyCustomFields bool `json:"copy_custom_fields"`
	CopyDescription  bool `json:"copy_description"`
	StatusOnCreate   *int `json:"status_on_create,omitempty"`

	IsActive  bool      `json:"is_active"`
	CreatedBy *int      `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	TemplateTitle  string     `json:"template_title,omitempty"`
	WorkspaceName  string     `json:"workspace_name,omitempty"`
	WorkspaceKey   string     `json:"workspace_key,omitempty"`
	CreatorName    string     `json:"creator_name,omitempty"`
	InstanceCount  int        `json:"instance_count,omitempty"`
	NextOccurrence *time.Time `json:"next_occurrence,omitempty"`
}

// RecurrenceInstance represents a generated instance from a recurring rule
type RecurrenceInstance struct {
	ID               int       `json:"id"`
	RecurrenceRuleID int       `json:"recurrence_rule_id"`
	InstanceItemID   int       `json:"instance_item_id"`
	ScheduledDate    time.Time `json:"scheduled_date"`
	SequenceNumber   int       `json:"sequence_number"`
	CreatedAt        time.Time `json:"created_at"`

	ItemTitle  string `json:"item_title,omitempty"`
	ItemStatus string `json:"item_status,omitempty"`
}

// CreateRecurrenceRequest is used for API input
type CreateRecurrenceRequest struct {
	TemplateItemID   int     `json:"template_item_id"`
	RRule            string  `json:"rrule"`
	DtStart          string  `json:"dtstart"` // ISO 8601 format
	DtEnd            *string `json:"dtend,omitempty"`
	Timezone         string  `json:"timezone,omitempty"`
	LeadTimeDays     *int    `json:"lead_time_days,omitempty"`
	CopyAssignee     *bool   `json:"copy_assignee,omitempty"`
	CopyPriority     *bool   `json:"copy_priority,omitempty"`
	CopyCustomFields *bool   `json:"copy_custom_fields,omitempty"`
	CopyDescription  *bool   `json:"copy_description,omitempty"`
	StatusOnCreate   *int    `json:"status_on_create,omitempty"`
}

// UpdateRecurrenceRequest is used for API input when updating a recurrence rule
type UpdateRecurrenceRequest struct {
	RRule            *string `json:"rrule,omitempty"`
	DtStart          *string `json:"dtstart,omitempty"`
	DtEnd            *string `json:"dtend,omitempty"`
	Timezone         *string `json:"timezone,omitempty"`
	LeadTimeDays     *int    `json:"lead_time_days,omitempty"`
	CopyAssignee     *bool   `json:"copy_assignee,omitempty"`
	CopyPriority     *bool   `json:"copy_priority,omitempty"`
	CopyCustomFields *bool   `json:"copy_custom_fields,omitempty"`
	CopyDescription  *bool   `json:"copy_description,omitempty"`
	StatusOnCreate   *int    `json:"status_on_create,omitempty"`
	IsActive         *bool   `json:"is_active,omitempty"`
}

// RRulePreviewRequest is used for previewing RRULE occurrences
type RRulePreviewRequest struct {
	RRule   string `json:"rrule"`
	DtStart string `json:"dtstart"`
	Count   int    `json:"count,omitempty"` // Number of occurrences to preview (default 10)
}

// ActionTriggerType defines the type of event that triggers an action
type ActionTriggerType string

const (
	ActionTriggerStatusTransition ActionTriggerType = "status_transition"
	ActionTriggerItemCreated      ActionTriggerType = "item_created"
	ActionTriggerItemUpdated      ActionTriggerType = "item_updated"
	ActionTriggerItemLinked       ActionTriggerType = "item_linked"
	ActionTriggerManual           ActionTriggerType = "manual"

	// SCM-driven triggers emitted by the repo-sync loop when a new git ref
	// matching the per-repository pattern is observed. Payload (in
	// ActionEvent.NewValues) carries: ref.name, ref.short, ref.sha,
	// ref.type, repo.owner, repo.name, repo.full_name, repo.workspace_repository_id,
	// and (tags only) ref.prev_name for "what shipped" range queries.
	ActionTriggerSCMTagCreated           ActionTriggerType = "scm_tag_created"
	ActionTriggerSCMReleaseBranchCreated ActionTriggerType = "scm_release_branch_created"

	// SCM pull-request lifecycle triggers emitted by the repo sync loop when
	// a synced repository discovers a new pull request linked to an item
	// (scm_pr_linked) or when a linked pull request transitions to merged
	// (scm_pr_merged).
	ActionTriggerSCMPRLinked ActionTriggerType = "scm_pr_linked"
	ActionTriggerSCMPRMerged ActionTriggerType = "scm_pr_merged"
)

// ActionNodeType defines the type of action node
type ActionNodeType string

const (
	ActionNodeTrigger          ActionNodeType = "trigger"
	ActionNodeSetField         ActionNodeType = "set_field"
	ActionNodeSetStatus        ActionNodeType = "set_status"
	ActionNodeAddComment       ActionNodeType = "add_comment"
	ActionNodeNotifyUser       ActionNodeType = "notify_user"
	ActionNodeCondition        ActionNodeType = "condition"
	ActionNodeUpdateAsset      ActionNodeType = "update_asset"
	ActionNodeCreateAsset      ActionNodeType = "create_asset"
	ActionNodeRoundRobinAssign ActionNodeType = "round_robin_assign"
	ActionNodeAIExtract        ActionNodeType = "ai_extract"
	ActionNodeAIAgent          ActionNodeType = "ai_agent"
	ActionNodeContainerRun     ActionNodeType = "container_run"
	ActionNodeHTTPRequest      ActionNodeType = "http_request"
	// ActionNodeTransitionItem transitions whatever item is currently in the
	// execution context (set by an iterator like related_items, or falling back
	// to the trigger item). Unlike set_status, the target status can be picked
	// dynamically per item: an explicit ID, by category name, or by mirroring
	// the trigger event's terminal category — so a single configured node works
	// across descendants whose workflows have different status IDs.
	ActionNodeTransitionItem ActionNodeType = "transition_item"
	// ActionNodeRelatedItems is an iterator: it fans out from the current item
	// (descendants, direct children, ancestors, or linked items) and re-runs
	// the downstream subgraph once per emitted item with ctx.Item swapped.
	ActionNodeRelatedItems ActionNodeType = "related_items"
	// ActionNodeCreateMilestone upserts a workspace milestone keyed by
	// external_key, optionally promoting status and attaching a release
	// row on scm_tag_created events. Registered via the node-executor
	// registry rather than the legacy switch in action_service.go.
	ActionNodeCreateMilestone ActionNodeType = "create_milestone"
)

// IsIterator reports whether this node type fans out — i.e. the engine must
// run its downstream body subgraph once per emitted item rather than once.
func (t ActionNodeType) IsIterator() bool {
	return t == ActionNodeRelatedItems
}

// ActionExecutionStatus defines the status of an action execution
type ActionExecutionStatus string

const (
	ActionStatusRunning   ActionExecutionStatus = "running"
	ActionStatusCompleted ActionExecutionStatus = "completed"
	ActionStatusFailed    ActionExecutionStatus = "failed"
	ActionStatusSkipped   ActionExecutionStatus = "skipped"
)

// Action represents a workspace-scoped automation definition
type Action struct {
	ID            int               `json:"id"`
	WorkspaceID   int               `json:"workspace_id"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	IsEnabled     bool              `json:"is_enabled"`
	TriggerType   ActionTriggerType `json:"trigger_type"`
	TriggerConfig string            `json:"trigger_config,omitempty"` // JSON with trigger-specific conditions
	CreatedBy     *int              `json:"created_by,omitempty"`
	// ActorUserID overrides the execution actor. NULL means the action runs under
	// the triggering user's permissions. Setting this field requires the global
	// action.set_actor permission because it grants cross-workspace impersonation.
	ActorUserID *int `json:"actor_user_id,omitempty"`
	// AllowedRoleIDs restricts a manual action to members of these workspace
	// roles. An empty list means every workspace editor may trigger it.
	AllowedRoleIDs []int     `json:"allowed_role_ids"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	// Joined fields for API responses
	CreatorName string       `json:"creator_name,omitempty"`
	ActorName   string       `json:"actor_name,omitempty"`
	Nodes       []ActionNode `json:"nodes,omitempty"`
	Edges       []ActionEdge `json:"edges,omitempty"`
}

// ActionTriggerConfig represents trigger-specific configuration
type ActionTriggerConfig struct {
	FromStatusID *int `json:"from_status_id,omitempty"` // null means any status
	ToStatusID   *int `json:"to_status_id,omitempty"`   // null means any status
	// ToStatusCategoryIsCompleted matches when the to-status's category has
	// is_completed=true (or =false when explicitly false). Used by templates
	// that want to fire on "any terminal transition" without enumerating
	// per-workflow status IDs. Evaluated after FromStatusID/ToStatusID; both
	// can be set together.
	ToStatusCategoryIsCompleted *bool  `json:"to_status_category_completed,omitempty"`
	ItemTypeID                  *int   `json:"item_type_id,omitempty"` // Filter by item type (optional)
	FieldName                   string `json:"field_name,omitempty"`   // Which field changed
	LinkTypeID                  *int   `json:"link_type_id,omitempty"` // Filter by link type (optional)
	// For SCM triggers: optional filter to a specific
	// workspace repository. When nil the trigger fires for any repository.
	WorkspaceRepositoryID *int `json:"workspace_repository_id,omitempty"`

	// For SCM triggers: optional filter to a specific
	// repository slug (e.g. "owner/repo"). Empty matches any repository.
	RepositoryFullName string `json:"repository_full_name,omitempty"`

	RespondToCascades bool `json:"respond_to_cascades,omitempty"` // If true, action responds to events triggered by other actions
}

// ActionNode represents a step in the action flow
type ActionNode struct {
	ID         int            `json:"id"`
	ActionID   int            `json:"action_id"`
	NodeType   ActionNodeType `json:"node_type"`
	NodeConfig string         `json:"node_config"` // JSON configuration for the node
	PositionX  float64        `json:"position_x"`
	PositionY  float64        `json:"position_y"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// FlowNodeID returns the node's ID for generic action-flow helpers.
func (n ActionNode) FlowNodeID() int { return n.ID }

// SetFlowActionID sets the node's action ID for generic action-flow helpers.
func (n *ActionNode) SetFlowActionID(id int) { n.ActionID = id }

// FlowNodeData returns the node fields for storage-level flow conversion.
func (n ActionNode) FlowNodeData() (actionID int, nodeType, config string, x, y float64) {
	return n.ActionID, string(n.NodeType), n.NodeConfig, n.PositionX, n.PositionY
}

// ActionEdge represents a connection between nodes
type ActionEdge struct {
	ID           int       `json:"id"`
	ActionID     int       `json:"action_id"`
	SourceNodeID int       `json:"source_node_id"`
	TargetNodeID int       `json:"target_node_id"`
	EdgeType     string    `json:"edge_type"` // default, true, false (for conditions)
	SourceHandle string    `json:"source_handle,omitempty"`
	TargetHandle string    `json:"target_handle,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// SetFlowActionID sets the edge's action ID for generic action-flow helpers.
func (e *ActionEdge) SetFlowActionID(id int) { e.ActionID = id }

// FlowSourceNodeID returns the edge's source node ID for generic action-flow helpers.
func (e ActionEdge) FlowSourceNodeID() int { return e.SourceNodeID }

// FlowTargetNodeID returns the edge's target node ID for generic action-flow helpers.
func (e ActionEdge) FlowTargetNodeID() int { return e.TargetNodeID }

// SetFlowSourceNodeID sets the edge's source node ID for generic action-flow helpers.
func (e *ActionEdge) SetFlowSourceNodeID(id int) { e.SourceNodeID = id }

// SetFlowTargetNodeID sets the edge's target node ID for generic action-flow helpers.
func (e *ActionEdge) SetFlowTargetNodeID(id int) { e.TargetNodeID = id }

// FlowEdgeID returns the edge's ID for storage-level flow conversion.
func (e ActionEdge) FlowEdgeID() int { return e.ID }

// FlowEdgeData returns the edge fields for storage-level flow conversion.
func (e ActionEdge) FlowEdgeData() (actionID int, edgeType, sourceHandle, targetHandle string) {
	return e.ActionID, e.EdgeType, e.SourceHandle, e.TargetHandle
}

// ActionExecutionLog represents the audit trail for action executions
type ActionExecutionLog struct {
	ID                   int                   `json:"id"`
	ActionID             int                   `json:"action_id"`
	ItemID               *int                  `json:"item_id,omitempty"`
	TriggerEvent         string                `json:"trigger_event"`
	Status               ActionExecutionStatus `json:"status"`
	TriggerUserID        *int                  `json:"trigger_user_id,omitempty"`
	EffectiveActorUserID *int                  `json:"effective_actor_user_id,omitempty"`
	StartedAt            time.Time             `json:"started_at"`
	CompletedAt          *time.Time            `json:"completed_at,omitempty"`
	ErrorMessage         string                `json:"error_message,omitempty"`
	ExecutionTrace       string                `json:"execution_trace,omitempty"` // JSON step log
	// Joined fields for API responses
	ActionName      string `json:"action_name,omitempty"`
	ItemTitle       string `json:"item_title,omitempty"`
	WorkspaceID     *int   `json:"workspace_id,omitempty"`
	WorkspaceName   string `json:"workspace_name,omitempty"`
	DurationMs      *int64 `json:"duration_ms,omitempty"`
	DurableEventKey string `json:"-"`
}

// ActionEvent represents an event that can trigger actions
type ActionEvent struct {
	EventType   ActionTriggerType `json:"event_type"`
	WorkspaceID int               `json:"workspace_id"`
	ItemID      int               `json:"item_id"`
	ActorUserID int               `json:"actor_user_id"`
	OldValues   map[string]any    `json:"old_values,omitempty"` // Previous field values
	NewValues   map[string]any    `json:"new_values,omitempty"` // New field values
	// Cascade control fields for loop prevention
	TriggeredByAction bool   `json:"triggered_by_action,omitempty"` // True if this event was emitted by an action
	ExecutionChainID  string `json:"execution_chain_id,omitempty"`  // UUID to look up cached chain state for cycle detection
	CascadeDepth      int    `json:"cascade_depth,omitempty"`       // Depth level of this event (0 = user-triggered)
	SourceApplication string `json:"source_application,omitempty"`  // "workspace", "logbook", or "asset"
}

// ExecutionContext holds context during action execution
type ExecutionContext struct {
	Context         context.Context `json:"-"`
	DurableEventKey string          `json:"-"`
	Action          *Action         `json:"action"`
	Event           *ActionEvent    `json:"event"`
	Item            *Item           `json:"item,omitempty"`
	Actor           *User           `json:"actor,omitempty"`
	// EffectiveActorID is the user whose permissions govern this execution and
	// whose identity downstream services record as the actor for side effects
	// (comments authored, item history entries, etc.). It equals the action's
	// ActorUserID override when set, otherwise the triggering user from the
	// event. All node executors MUST use this instead of Event.ActorUserID.
	EffectiveActorID int            `json:"effective_actor_id"`
	Variables        map[string]any `json:"variables,omitempty"` // Dynamic variables during execution
	StepResults      []StepResult   `json:"step_results,omitempty"`
	// ChainID is set when this action is part of a cascade chain (for emitting chained events)
	ChainID string `json:"-"` // Not serialized - internal use only
	// TotalSteps counts every node execution within this action invocation,
	// including iterator body nodes summed across iterations. Bounded by the
	// engine's per-flow step budget so a misconfigured nested iterator can't
	// fan out into millions of executions before the cascade-depth guard fires.
	TotalSteps int `json:"-"`
}

// StepResult holds the result of executing a single node
type StepResult struct {
	NodeID       int                   `json:"node_id"`
	NodeType     ActionNodeType        `json:"node_type"`
	Status       ActionExecutionStatus `json:"status"`
	StartedAt    time.Time             `json:"started_at"`
	CompletedAt  *time.Time            `json:"completed_at,omitempty"`
	ErrorMessage string                `json:"error_message,omitempty"`
	Output       map[string]any        `json:"output,omitempty"`
	// Iterations is populated when the node is an iterator (related_items).
	// Each entry holds the per-item subgraph step results so the trace can
	// show which downstream nodes ran for which item.
	Iterations []IterationResult `json:"iterations,omitempty"`
}

// IterationResult is one pass through an iterator's body subgraph.
type IterationResult struct {
	ItemID      int          `json:"item_id"`
	WorkspaceID int          `json:"workspace_id,omitempty"`
	Steps       []StepResult `json:"steps,omitempty"`
}

// Node configuration types

// SetFieldNodeConfig configures a set_field node.
//
// A node targets either a real items-table column or a custom field:
//   - Target == "" or "column": update a supported item field through its
//     domain service. Status changes are routed through workflow transitions;
//     item type and ordering changes require their dedicated workflows.
//   - Target == "custom_field": update a single key inside items.custom_field_values.
//     CustomFieldID must reference a row in custom_field_definitions.
//
// The Target field is optional so pre-existing configs (FieldName only) keep
// working without migration.
type SetFieldNodeConfig struct {
	Target        string `json:"target,omitempty"`
	FieldName     string `json:"field_name"`
	CustomFieldID int    `json:"custom_field_id,omitempty"`
	Value         string `json:"value"` // Can contain {{variable}} templates
	// DisplayName fields are UI-only hints stored with editor-authored nodes.
	// Executors ignore them, but the catalog schema must accept them so saving
	// an edited visual flow does not fail validation.
	FieldDisplayName string `json:"field_display_name,omitempty"`
	FieldType        string `json:"field_type,omitempty"`
	ValueDisplayName string `json:"value_display_name,omitempty"`
}

// SetStatusNodeConfig configures a set_status node
type SetStatusNodeConfig struct {
	StatusID int `json:"status_id"`
}

// TransitionItemTargetMode selects how transition_item picks the destination
// status for whichever item is currently in execution context.
const (
	// TransitionTargetExplicit transitions to the literal StatusID. Best for
	// single-workflow automations where you know the exact target.
	TransitionTargetExplicit = "explicit"
	// TransitionTargetCategoryName looks up the current item's workflow and
	// picks the terminal status whose category name matches CategoryName.
	// Falls back to the first terminal status if no match.
	TransitionTargetCategoryName = "category_name"
	// TransitionTargetMatchingTerminal looks at the trigger event's new-status
	// category name and finds the terminal status in the *current* item's
	// workflow with the same category name. Falls back to first terminal.
	// Used when "close subtasks" should mirror the parent's terminal category
	// (e.g. parent → "Canceled" should close children to "Canceled" in
	// their own workflow, not just any "Done").
	TransitionTargetMatchingTerminal = "matching_terminal"
)

// RelatedItemsRelation selects which items the iterator fans out to.
const (
	RelatedItemsDescendants    = "descendants"
	RelatedItemsDirectChildren = "direct_children"
	RelatedItemsAncestors      = "ancestors"
	// RelatedItemsLinked emits items connected via the item_links table.
	// LinkTypeID + LinkDirection on the config narrow the emission.
	RelatedItemsLinked = "linked"
)

// LinkDirection selects which direction of an item-link relationship the
// linked iterator follows. "" defaults to "both".
const (
	LinkDirectionOutgoing = "outgoing"
	LinkDirectionIncoming = "incoming"
	LinkDirectionBoth     = "both"
)

// RelatedItemsNodeConfig configures a related_items iterator. The iterator's
// downstream body subgraph runs once per emitted item with ctx.Item set to
// that item. CrossWorkspace=false restricts iteration to items in the same
// workspace as the iterator's input item.
type RelatedItemsNodeConfig struct {
	Relation       string `json:"relation"`
	CrossWorkspace bool   `json:"cross_workspace"`
	// LinkTypeID filters relation=linked to a single link type (e.g. "blocks").
	// Nil means any link type.
	LinkTypeID *int `json:"link_type_id,omitempty"`
	// LinkDirection (relation=linked only): "outgoing", "incoming", or "both".
	// Empty defaults to "both".
	LinkDirection string `json:"link_direction,omitempty"`
	// MaxItems caps emission to prevent runaway iteration on pathological
	// trees. Zero means use the engine default (1000).
	MaxItems int `json:"max_items,omitempty"`
}

// TransitionItemNodeConfig configures a transition_item node.
type TransitionItemNodeConfig struct {
	Target struct {
		Mode         string `json:"mode"`
		StatusID     int    `json:"status_id,omitempty"`
		CategoryName string `json:"category_name,omitempty"`
	} `json:"target"`
	// SkipIfAlreadyMatching: when true, no transition is attempted if the
	// current item's status already equals the resolved target. Default true
	// (an explicit false in JSON disables the check). Prevents noisy no-ops
	// in execution traces when descendants are already closed. Pointer so we
	// can distinguish "omitted" (default skip) from "explicit false" (force
	// re-apply).
	SkipIfAlreadyMatching *bool `json:"skip_if_already_matching,omitempty"`
}

// AddCommentNodeConfig configures an add_comment node
type AddCommentNodeConfig struct {
	Content   string `json:"content"` // Can contain {{variable}} templates
	IsPrivate bool   `json:"is_private"`
}

// NotifyUserNodeConfig configures a notify_user node
type NotifyUserNodeConfig struct {
	Recipients []string `json:"recipients,omitempty"` // "assignee", "creator", or specific user IDs
	// RecipientType is a UI convenience field used by the visual editor. When
	// Recipients is empty, executors treat "assignee"/"creator" as a single
	// recipient token. The persisted source of truth remains Recipients.
	RecipientType string `json:"recipient_type,omitempty"`
	Message       string `json:"message"` // Can contain {{variable}} templates
	Title         string `json:"title,omitempty"`
	IncludeLink   bool   `json:"include_link"` // Include link to item
}

// ConditionNodeConfig configures a condition node
type ConditionNodeConfig struct {
	FieldName string `json:"field_name"` // Field to check
	Operator  string `json:"operator"`   // eq, ne, gt, lt, contains, etc.
	Value     string `json:"value"`      // Value to compare against
}

// UpdateAssetNodeConfig configures an update_asset node
type UpdateAssetNodeConfig struct {
	SourceFieldID string              `json:"source_field_id"` // Item's asset field containing the asset reference
	AssetTypeID   int                 `json:"asset_type_id"`   // Expected asset type
	AssetSetID    int                 `json:"asset_set_id"`    // Asset set for validation
	FieldMappings []AssetFieldMapping `json:"field_mappings"`
}

// AssetFieldMapping represents a single field mapping from item to asset
type AssetFieldMapping struct {
	SourceType    string `json:"source_type"`     // "item_field", "literal", or "variable"
	SourceValue   string `json:"source_value"`    // Field name, literal value, or template
	TargetFieldID string `json:"target_field_id"` // Asset field to update
}

// CreateAssetNodeConfig configures a create_asset node
type CreateAssetNodeConfig struct {
	AssetSetID    int                 `json:"asset_set_id"`   // Target asset set
	AssetTypeID   int                 `json:"asset_type_id"`  // Asset type to create
	Title         string              `json:"title"`          // Title template (supports {{variables}})
	Description   string              `json:"description"`    // Description template (optional)
	AssetTag      string              `json:"asset_tag"`      // Asset tag template (optional)
	CategoryID    *int                `json:"category_id"`    // Optional category
	StatusID      *int                `json:"status_id"`      // Optional status (defaults to set default)
	FieldMappings []AssetFieldMapping `json:"field_mappings"` // Field mappings for custom fields
}

// AIExtractNodeConfig configures an ai_extract node — sandboxed LLM analysis
// that processes untrusted input with no tools and structured output only.
type AIExtractNodeConfig struct {
	Prompt       string `json:"prompt"`        // System prompt for extraction
	InputField   string `json:"input_field"`   // Execution context variable holding untrusted input
	OutputSchema string `json:"output_schema"` // JSON Schema defining the output struct
	OutputField  string `json:"output_field"`  // Variable name to store the extracted struct
	CapabilityID int    `json:"capability_id"` // References an llm_connection capability
}

// MaxAIAgentSteps is the hard upper bound on AIAgentNodeConfig.MaxSteps.
// The UI caps the input at this value; the catalog validator and the action
// executor both enforce it server-side so REST / MCP / aitool create paths
// can't bypass it and trigger runaway LLM loops.
const MaxAIAgentSteps = 50

// AIAgentNodeConfig configures an ai_agent node — agentic execution with
// a purpose-built tool set. Never receives raw untrusted input.
type AIAgentNodeConfig struct {
	Prompt       string   `json:"prompt"`        // System prompt (can reference {{variables}})
	InputFields  []string `json:"input_fields"`  // Which context variables to include in the user message
	Tools        []string `json:"tools"`         // Capability IDs to enable as tools
	MaxSteps     int      `json:"max_steps"`     // Max agent loop iterations (server-capped to MaxAIAgentSteps)
	OutputField  string   `json:"output_field"`  // Variable name to store the agent's result
	CapabilityID int      `json:"capability_id"` // References an llm_connection capability
}

// ContainerRunNodeConfig configures a container_run node — Docker container lifecycle.
type ContainerRunNodeConfig struct {
	CapabilityID int    `json:"capability_id"` // References a docker_environment capability
	OutputField  string `json:"output_field"`  // Variable storing container info (ID, port, etc.)
	TimeoutSecs  int    `json:"timeout_secs"`  // Max lifetime before auto-teardown
	// PoolCapabilityID, when set, dispatches the container to a remote runner
	// pool (WI-146): an action_container agent_run is enqueued for the pool
	// instead of running locally. 0 = run locally via ContainerService.
	PoolCapabilityID int `json:"pool_capability_id,omitempty"`
}

// HTTPRequestNodeConfig configures an http_request node — scoped HTTP client.
type HTTPRequestNodeConfig struct {
	Method       string            `json:"method"`                  // HTTP method
	URLTemplate  string            `json:"url_template"`            // Supports {{variables}}
	Headers      map[string]string `json:"headers,omitempty"`       // Request headers
	Body         string            `json:"body,omitempty"`          // Body template
	OutputField  string            `json:"output_field"`            // Variable name to store the response
	CapabilityID int               `json:"capability_id,omitempty"` // References an http_client capability for URL validation
}

// CapabilityType defines the type of admin-provisioned capability.
type CapabilityType string

const (
	CapabilityDockerEnvironment CapabilityType = "docker_environment"
	CapabilityHTTPClient        CapabilityType = "http_client"
	CapabilityLLMConnection     CapabilityType = "llm_connection"
	// CapabilityRunnerPool is an admin-provisioned pool of runners that
	// jobs are dispatched to (Initiative WI-141). It reuses the same
	// scoping as every other capability — applies_to_all_workspaces plus
	// the action_capability_workspaces join table — so "which workspaces
	// may dispatch to this pool" is the same resolveCapability gate as
	// "which workspaces may use this docker image".
	CapabilityRunnerPool CapabilityType = "runner_pool"
)

// ActionCapability represents an admin-provisioned resource that action nodes can reference.
type ActionCapability struct {
	ID             int            `json:"id"`
	Name           string         `json:"name"`
	CapabilityType CapabilityType `json:"capability_type"`
	Config         string         `json:"config"` // JSON, type-specific configuration
	IsEnabled      bool           `json:"is_enabled"`
	// AppliesToAllWorkspaces gates whether every workspace's actions can
	// reference this capability. When false, only workspaces listed in
	// WorkspaceIDs (the action_capability_workspaces join table) may use it.
	AppliesToAllWorkspaces bool `json:"applies_to_all_workspaces"`
	// WorkspaceIDs is populated by the read path from the join table. Only
	// meaningful when AppliesToAllWorkspaces is false. Always nil-or-empty
	// when AppliesToAllWorkspaces is true (the per-workspace allowlist is
	// irrelevant in that case).
	WorkspaceIDs []int     `json:"workspace_ids,omitempty"`
	CreatedBy    *int      `json:"created_by,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DockerEnvironmentConfig is the config for a docker_environment capability.
type DockerEnvironmentConfig struct {
	Image          string             `json:"image"`
	ResourceLimits ResourceLimits     `json:"resource_limits"`
	NetworkMode    string             `json:"network_mode"`
	EnvVars        map[string]string  `json:"env_vars,omitempty"`
	HealthCheck    *HealthCheckConfig `json:"health_check,omitempty"`
}

// RunnerPoolConfig is the config blob for a runner_pool capability. Pool
// membership/scoping rides the shared ActionCapability fields
// (applies_to_all_workspaces + WorkspaceIDs); this struct holds only the
// pool's own behavior. Registration tokens and runner instances live in
// their own tables (runner_registration_tokens, runner_instances), not here.
type RunnerPoolConfig struct {
	// MaxConcurrentRuns caps how many runs this pool executes at once
	// (0 = unlimited). It is the per-pool quota referenced by the
	// scheduler in WI-161.
	MaxConcurrentRuns int `json:"max_concurrent_runs,omitempty"`
	// Ephemeral, when true, opts the pool into the truly-ephemeral-agent
	// mode (re-register + exit per job) instead of the default
	// persistent-agent / ephemeral-container model (decision #5). Default
	// false; honored by the agent binary in a later phase.
	Ephemeral bool `json:"ephemeral,omitempty"`
}

// ResourceLimits defines resource constraints for a Docker container.
type ResourceLimits struct {
	Memory string `json:"memory"` // e.g., "512m"
	CPUs   string `json:"cpus"`   // e.g., "1"
}

// HealthCheckConfig defines a container health check.
type HealthCheckConfig struct {
	Endpoint    string `json:"endpoint"`
	IntervalSec int    `json:"interval_secs"`
	TimeoutSec  int    `json:"timeout_secs"`
}

// HTTPClientConfig is the config for an http_client capability.
//
// DefaultHeaders must contain only non-sensitive literals (Accept,
// User-Agent, etc.). Sensitive auth material (API tokens, API keys, basic
// auth) is referenced by ID via Auth / SecretHeaderRefs and resolved at
// execution time from the action_credentials store. The capability
// validator rejects any DefaultHeaders key whose name matches the
// sensitive list (see IsSensitiveHeaderName).
type HTTPClientConfig struct {
	AllowedURLPatterns []string          `json:"allowed_url_patterns"`
	DefaultHeaders     map[string]string `json:"default_headers,omitempty"`
	TimeoutSecs        int               `json:"timeout_secs"`
	// Auth is the primary auth header (e.g. Authorization: Bearer <token>).
	// Single header per capability; multi-secret APIs use SecretHeaderRefs.
	Auth *HTTPAuthRef `json:"auth,omitempty"`
	// SecretHeaderRefs maps additional secret header names to credential IDs
	// (e.g. {"X-API-Key": 12, "X-Signature": 14}). Used by APIs that require
	// multiple secret headers per request.
	SecretHeaderRefs map[string]int `json:"secret_header_refs,omitempty"`
}

// HTTPAuthRef captures the primary auth header for an http_client capability.
// The credential is resolved server-side at execution time; the credential ID
// is the only secret-adjacent value that ever flows over the wire.
type HTTPAuthRef struct {
	CredentialID int    `json:"credential_id"`
	Placement    string `json:"placement,omitempty"` // "header" (only value supported in v1)
	HeaderName   string `json:"header_name"`         // e.g. "Authorization"
	Scheme       string `json:"scheme,omitempty"`    // e.g. "Bearer"
}

// IsValidHTTPHeaderName reports whether name is a non-empty RFC 9110 token.
// Header maps are persisted before net/http sees them, so rejecting malformed
// names here prevents CR/LF and whitespace tricks from becoming latent runtime
// failures (or confusing case-insensitive collision checks).
func IsValidHTTPHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isHTTPTokenChar(name[i]) {
			return false
		}
	}
	return true
}

func isHTTPTokenChar(c byte) bool {
	if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}

// IsValidHTTPAuthScheme validates the auth-scheme grammar (RFC 9110 §11.1).
// In particular it excludes spaces, so a scheme field cannot smuggle an
// inline credential alongside the encrypted credential reference.
func IsValidHTTPAuthScheme(scheme string) bool {
	if scheme == "" {
		return true
	}
	for i := 0; i < len(scheme); i++ {
		c := scheme[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(i > 0 && ((c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~')) {
			continue
		}
		return false
	}
	return true
}

// NormalizeActionHTTPMethod canonicalizes the intentionally small method set
// supported by action HTTP nodes and agent HTTP tools.
func NormalizeActionHTTPMethod(method string) (string, bool) {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH":
		return method, true
	default:
		return method, false
	}
}

// IsSensitiveHeaderName reports whether the given HTTP header name is
// considered sensitive and must therefore not appear as a literal in
// DefaultHeaders or in an HTTPRequestNodeConfig.Headers map. Matching is
// case-insensitive. The list errs on the side of caution — a benign header
// that happens to match (e.g. "token-style-something") is just blocked, and
// the user can rename it.
func IsSensitiveHeaderName(name string) bool {
	lk := strings.ToLower(strings.TrimSpace(name))
	if lk == "" {
		return false
	}
	switch lk {
	case
		"authorization",
		"proxy-authorization",
		"cookie",
		"set-cookie",
		"x-api-key",
		"api-key",
		"x-auth-token",
		"x-access-token",
		"x-secret-key",
		"x-signature",
		"x-amz-security-token",
		"x-github-token":
		return true
	}
	// Generic patterns: anything ending in -token/-key/-secret/-password is
	// almost certainly secret material. Standard headers like Content-Type,
	// Accept, User-Agent, X-Request-Id don't trip these suffixes.
	return strings.HasSuffix(lk, "-token") ||
		strings.HasSuffix(lk, "-key") ||
		strings.HasSuffix(lk, "-secret") ||
		strings.HasSuffix(lk, "-password") ||
		strings.HasSuffix(lk, "-signature")
}

// LLMConnectionCapabilityConfig is the config for an llm_connection capability.
type LLMConnectionCapabilityConfig struct {
	ConnectionID int `json:"connection_id"`
}

// ContainerInfo holds runtime info about a started container.
type ContainerInfo struct {
	ContainerID string `json:"container_id"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
}

// CreateCapabilityRequest represents the API request to create a capability.
type CreateCapabilityRequest struct {
	Name                   string         `json:"name"`
	CapabilityType         CapabilityType `json:"capability_type"`
	Config                 string         `json:"config"`
	IsEnabled              *bool          `json:"is_enabled,omitempty"`
	AppliesToAllWorkspaces *bool          `json:"applies_to_all_workspaces,omitempty"`
	WorkspaceIDs           []int          `json:"workspace_ids,omitempty"`
}

// UpdateCapabilityRequest represents the API request to update a capability.
type UpdateCapabilityRequest struct {
	Name                   *string `json:"name,omitempty"`
	Config                 *string `json:"config,omitempty"`
	IsEnabled              *bool   `json:"is_enabled,omitempty"`
	AppliesToAllWorkspaces *bool   `json:"applies_to_all_workspaces,omitempty"`
	WorkspaceIDs           *[]int  `json:"workspace_ids,omitempty"`
}

// API Request/Response types

// CreateActionRequest represents the API request to create an action
type CreateActionRequest struct {
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	TriggerType    ActionTriggerType `json:"trigger_type"`
	TriggerConfig  string            `json:"trigger_config,omitempty"`
	ActorUserID    *int              `json:"actor_user_id,omitempty"` // requires action.set_actor global permission
	AllowedRoleIDs []int             `json:"allowed_role_ids,omitempty"`
	Nodes          []ActionNode      `json:"nodes,omitempty"`
	Edges          []ActionEdge      `json:"edges,omitempty"`
}

// ActionActorUpdate carries an optional actor_user_id patch for an action.
// Present=true means the field is being changed (including to null).
type ActionActorUpdate struct {
	Present bool
	Value   *int
}

// UnmarshalJSON treats `"actor_user_id": null` as an explicit clear and an
// omitted key as "no change", which lets callers without action.set_actor
// permission still update other fields.
func (a *ActionActorUpdate) UnmarshalJSON(data []byte) error {
	a.Present = true
	if string(data) == "null" {
		a.Value = nil
		return nil
	}
	var v int
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	a.Value = &v
	return nil
}

// UpdateActionRequest represents the API request to update an action
type UpdateActionRequest struct {
	Name           *string            `json:"name,omitempty"`
	Description    *string            `json:"description,omitempty"`
	TriggerType    *ActionTriggerType `json:"trigger_type,omitempty"`
	TriggerConfig  *string            `json:"trigger_config,omitempty"`
	IsEnabled      *bool              `json:"is_enabled,omitempty"`
	ActorUserID    ActionActorUpdate  `json:"actor_user_id,omitempty"` // requires action.set_actor global permission when present
	AllowedRoleIDs []int              `json:"allowed_role_ids,omitempty"`
	Nodes          []ActionNode       `json:"nodes,omitempty"`
	Edges          []ActionEdge       `json:"edges,omitempty"`
}
