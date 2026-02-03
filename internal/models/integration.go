package models

import "time"

// SCM Provider Models

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
	OAuthAccessTokenEncrypted    string          `json:"-"`
	OAuthRefreshTokenEncrypted   string          `json:"-"`
	OAuthTokenExpiresAt          *time.Time      `json:"oauth_token_expires_at,omitempty"`
	Scopes                       string          `json:"scopes"`
	WorkspaceRestrictionMode     string          `json:"workspace_restriction_mode"` // 'unrestricted' or 'restricted'
	CreatedAt                    time.Time       `json:"created_at"`
	UpdatedAt                    time.Time       `json:"updated_at"`
	// Computed fields for API responses
	HasOAuthClientSecret   bool `json:"has_oauth_client_secret,omitempty"`
	HasPAT                 bool `json:"has_pat,omitempty"`
	HasGitHubAppPrivateKey bool `json:"has_github_app_private_key,omitempty"`
	HasOAuthToken          bool `json:"has_oauth_token,omitempty"`
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
	ID          int       `json:"id"`
	ProviderID  int       `json:"provider_id"`
	WorkspaceID int       `json:"workspace_id"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   *int      `json:"created_by,omitempty"`
	// Joined fields for API responses
	WorkspaceName string `json:"workspace_name,omitempty"`
	WorkspaceKey  string `json:"workspace_key,omitempty"`
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
	ID                   int       `json:"id"`
	WorkspaceID          int       `json:"workspace_id"`
	SCMProviderID        int       `json:"scm_provider_id"`
	Enabled              bool      `json:"enabled"`
	DefaultBranchPattern string    `json:"default_branch_pattern,omitempty"`
	ItemKeyPattern       string    `json:"item_key_pattern,omitempty"`
	CreatedBy            *int      `json:"created_by,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	// Workspace-level credentials (encrypted)
	OAuthAccessTokenEncrypted    string     `json:"-"`
	OAuthRefreshTokenEncrypted   string     `json:"-"`
	OAuthTokenExpiresAt          *time.Time `json:"oauth_token_expires_at,omitempty"`
	PersonalAccessTokenEncrypted string     `json:"-"`
	// Computed fields for API responses
	HasOAuthToken bool `json:"has_oauth_token,omitempty"`
	HasPAT        bool `json:"has_pat,omitempty"`
	// Joined fields
	ProviderName string          `json:"provider_name,omitempty"`
	ProviderType SCMProviderType `json:"provider_type,omitempty"`
	ProviderSlug string          `json:"provider_slug,omitempty"`
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
	ID                       int        `json:"id"`
	WorkspaceSCMConnectionID int        `json:"workspace_scm_connection_id"`
	RepositoryExternalID     string     `json:"repository_external_id"`
	RepositoryName           string     `json:"repository_name"`
	RepositoryURL            string     `json:"repository_url"`
	DefaultBranch            string     `json:"default_branch"`
	IsActive                 bool       `json:"is_active"`
	LastSyncedAt             *time.Time `json:"last_synced_at,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	// Joined fields
	WorkspaceID  int             `json:"workspace_id,omitempty"`
	ProviderType SCMProviderType `json:"provider_type,omitempty"`
	ProviderSlug string          `json:"provider_slug,omitempty"`
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
	ID                    int          `json:"id"`
	ItemID                int          `json:"item_id"`
	WorkspaceRepositoryID int          `json:"workspace_repository_id"`
	LinkType              SCMLinkType  `json:"link_type"`
	ExternalID            string       `json:"external_id"`
	ExternalURL           string       `json:"external_url,omitempty"`
	Title                 string       `json:"title,omitempty"`
	State                 SCMLinkState `json:"state,omitempty"`
	AuthorExternalID      string       `json:"author_external_id,omitempty"`
	AuthorName            string       `json:"author_name,omitempty"`
	DetectionSource       string       `json:"detection_source,omitempty"`
	CreatedAt             time.Time    `json:"created_at"`
	UpdatedAt             time.Time    `json:"updated_at"`
	// Joined fields
	RepositoryName string          `json:"repository_name,omitempty"`
	RepositoryURL  string          `json:"repository_url,omitempty"`
	ProviderType   SCMProviderType `json:"provider_type,omitempty"`
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
	WebhookSecretEncrypted string     `json:"-"` // Never expose
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

// Time Tracking Models

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
	ID          int                    `json:"id"`
	CustomerID  *int                   `json:"customer_id,omitempty"` // Now optional
	CategoryID  *int                   `json:"category_id,omitempty"` // Link to project category
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Status      string                 `json:"status"` // Active, On Hold, Completed, Archived
	Color       string                 `json:"color,omitempty"`
	HourlyRate  float64                `json:"hourly_rate"`
	Settings    map[string]interface{} `json:"settings,omitempty"` // Flexible JSON attributes (e.g., max_hours)
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	// Joined fields for API responses
	CustomerName  string   `json:"customer_name,omitempty"`
	CategoryName  string   `json:"category_name,omitempty"`
	CategoryColor string   `json:"category_color,omitempty"`
	TotalHours    *float64 `json:"total_hours,omitempty"` // Computed from worklogs
}

// Worklog represents a time tracking entry
type Worklog struct {
	ID           int    `json:"id"`
	ProjectID    int    `json:"project_id"`
	CustomerID   int    `json:"customer_id"`
	UserID       *int   `json:"user_id,omitempty"`  // User who created the worklog
	ItemID       *int   `json:"item_id,omitempty"`  // Optional link to work item
	Description  string `json:"description"`
	Date         int64  `json:"date"`       // Unix timestamp
	StartTime    int64  `json:"start_time"` // Unix timestamp
	EndTime      int64  `json:"end_time"`   // Unix timestamp
	DurationMins int    `json:"duration_minutes"`
	CreatedAt    int64  `json:"created_at"` // Unix timestamp
	UpdatedAt    int64  `json:"updated_at"` // Unix timestamp
	// Joined fields for API responses
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
	ID           int    `json:"id"`
	WorkspaceID  int    `json:"workspace_id"`
	ItemID       *int   `json:"item_id,omitempty"` // Optional link to work item
	ProjectID    int    `json:"project_id"`
	Description  string `json:"description"`
	StartTimeUTC int64  `json:"start_time_utc"` // Unix timestamp in UTC
	CreatedAt    int64  `json:"created_at"`     // Unix timestamp
	// Joined fields for API responses - nullable since they come from LEFT JOINs
	ProjectName   *string `json:"project_name,omitempty"`
	CustomerName  *string `json:"customer_name,omitempty"`
	ItemTitle     *string `json:"item_title,omitempty"`
	WorkspaceName *string `json:"workspace_name,omitempty"`
	WorkspaceKey  *string `json:"workspace_key,omitempty"`
}

// Review System Models

// Review represents a daily or weekly personal review entry
type Review struct {
	ID         int       `json:"id"`
	UserID     int       `json:"user_id"`
	ReviewDate string    `json:"review_date"` // YYYY-MM-DD format
	ReviewType string    `json:"review_type"` // 'daily' or 'weekly'
	ReviewData string    `json:"review_data"` // JSON data - unstructured storage
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Joined fields for API responses
	UserName  string `json:"user_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
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

// Test Management Models

// TestFolder represents a folder for organizing test cases
type TestFolder struct {
	ID          int       `json:"id"`
	WorkspaceID int       `json:"workspace_id"`
	ParentID    *int      `json:"parent_id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Computed fields for API responses
	TestCaseCount int `json:"test_case_count,omitempty"`
}

// TestCase represents a test case
type TestCase struct {
	ID                int       `json:"id"`
	WorkspaceID       int       `json:"workspace_id"`
	FolderID          *int      `json:"folder_id,omitempty"`
	Title             string    `json:"title"`
	Name              string    `json:"name"`
	Priority          string    `json:"priority"`           // low, medium, high, critical
	Status            string    `json:"status"`             // active, inactive, draft
	EstimatedDuration int       `json:"estimated_duration"` // in seconds
	Preconditions     string    `json:"preconditions"`
	SortOrder         int       `json:"sort_order"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	// Computed fields for API responses
	FolderName string      `json:"folder_name,omitempty"`
	TestSteps  []TestStep  `json:"test_steps,omitempty"`
	Labels     []TestLabel `json:"labels,omitempty"`
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

	// Test case count
	TestCaseCount int `json:"test_case_count,omitempty"`

	// Test run statistics
	TotalRuns      int        `json:"total_runs,omitempty"`
	SuccessfulRuns int        `json:"successful_runs,omitempty"`
	FailedRuns     int        `json:"failed_runs,omitempty"`
	LastRunStatus  string     `json:"last_run_status,omitempty"`
	LastRunDate    *time.Time `json:"last_run_date,omitempty"`
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
	// Joined fields for API responses
	SetName string `json:"set_name,omitempty"`
}

// TestRun represents an execution of a test set
type TestRun struct {
	ID          int        `json:"id"`
	WorkspaceID int        `json:"workspace_id"`
	TemplateID  int        `json:"template_id,omitempty"` // Optional reference to template
	SetID       int        `json:"set_id"`
	Name        string     `json:"name"`
	AssigneeID  *int       `json:"assignee_id,omitempty"` // User assigned to execute this run
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at"`
	CreatedAt   time.Time  `json:"created_at"`
	// Computed fields from JOIN with users table
	AssigneeName   string `json:"assignee_name,omitempty"`
	AssigneeEmail  string `json:"assignee_email,omitempty"`
	AssigneeAvatar string `json:"assignee_avatar,omitempty"`
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

// Test Coverage Models

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

// Recurring Tasks Models

// RecurrenceRule represents a recurring task pattern for generating instances
type RecurrenceRule struct {
	ID             int        `json:"id"`
	TemplateItemID int        `json:"template_item_id"`
	WorkspaceID    int        `json:"workspace_id"`

	// iCalendar RRULE configuration (RFC 5545)
	RRule    string     `json:"rrule"`    // e.g., "FREQ=WEEKLY;BYDAY=MO,WE,FR"
	DtStart  time.Time  `json:"dtstart"`  // Recurrence start date
	DtEnd    *time.Time `json:"dtend,omitempty"`
	Timezone string     `json:"timezone"` // IANA timezone

	// Generation settings
	LeadTimeDays        int        `json:"lead_time_days"`
	LastGeneratedUntil  *time.Time `json:"last_generated_until,omitempty"`
	NextGenerationCheck *time.Time `json:"next_generation_check,omitempty"`

	// Instance configuration (what to copy from template)
	CopyAssignee     bool `json:"copy_assignee"`
	CopyPriority     bool `json:"copy_priority"`
	CopyCustomFields bool `json:"copy_custom_fields"`
	CopyDescription  bool `json:"copy_description"`
	StatusOnCreate   *int `json:"status_on_create,omitempty"`

	// Lifecycle
	IsActive  bool       `json:"is_active"`
	CreatedBy *int       `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Joined fields for API responses
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

	// Joined fields for API responses
	ItemTitle  string `json:"item_title,omitempty"`
	ItemStatus string `json:"item_status,omitempty"`
}

// CreateRecurrenceRequest is used for API input
type CreateRecurrenceRequest struct {
	TemplateItemID   int     `json:"template_item_id"`
	RRule            string  `json:"rrule"`
	DtStart          string  `json:"dtstart"`          // ISO 8601 format
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

// Actions Automation Models

// ActionTriggerType defines the type of event that triggers an action
type ActionTriggerType string

const (
	ActionTriggerStatusTransition ActionTriggerType = "status_transition"
	ActionTriggerItemCreated      ActionTriggerType = "item_created"
	ActionTriggerItemUpdated      ActionTriggerType = "item_updated"
	ActionTriggerItemLinked       ActionTriggerType = "item_linked"
	ActionTriggerManual           ActionTriggerType = "manual"
)

// ActionNodeType defines the type of action node
type ActionNodeType string

const (
	ActionNodeTrigger     ActionNodeType = "trigger"
	ActionNodeSetField    ActionNodeType = "set_field"
	ActionNodeSetStatus   ActionNodeType = "set_status"
	ActionNodeAddComment  ActionNodeType = "add_comment"
	ActionNodeNotifyUser  ActionNodeType = "notify_user"
	ActionNodeCondition   ActionNodeType = "condition"
	ActionNodeUpdateAsset ActionNodeType = "update_asset"
	ActionNodeCreateAsset ActionNodeType = "create_asset"
)

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
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	// Joined fields for API responses
	CreatorName string       `json:"creator_name,omitempty"`
	Nodes       []ActionNode `json:"nodes,omitempty"`
	Edges       []ActionEdge `json:"edges,omitempty"`
}

// ActionTriggerConfig represents trigger-specific configuration
type ActionTriggerConfig struct {
	// For status_transition
	FromStatusID *int `json:"from_status_id,omitempty"` // null means any status
	ToStatusID   *int `json:"to_status_id,omitempty"`   // null means any status
	// For item_created and item_updated
	ItemTypeID *int `json:"item_type_id,omitempty"` // Filter by item type (optional)
	// For item_updated
	FieldName string `json:"field_name,omitempty"` // Which field changed
	// For item_linked
	LinkTypeID *int `json:"link_type_id,omitempty"` // Filter by link type (optional)
	// Cascade control - applies to all trigger types
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

// ActionExecutionLog represents the audit trail for action executions
type ActionExecutionLog struct {
	ID             int                   `json:"id"`
	ActionID       int                   `json:"action_id"`
	ItemID         *int                  `json:"item_id,omitempty"`
	TriggerEvent   string                `json:"trigger_event"`
	Status         ActionExecutionStatus `json:"status"`
	StartedAt      time.Time             `json:"started_at"`
	CompletedAt    *time.Time            `json:"completed_at,omitempty"`
	ErrorMessage   string                `json:"error_message,omitempty"`
	ExecutionTrace string                `json:"execution_trace,omitempty"` // JSON step log
	// Joined fields for API responses
	ActionName string `json:"action_name,omitempty"`
	ItemTitle  string `json:"item_title,omitempty"`
}

// ActionEvent represents an event that can trigger actions
type ActionEvent struct {
	EventType   ActionTriggerType      `json:"event_type"`
	WorkspaceID int                    `json:"workspace_id"`
	ItemID      int                    `json:"item_id"`
	ActorUserID int                    `json:"actor_user_id"`
	OldValues   map[string]interface{} `json:"old_values,omitempty"` // Previous field values
	NewValues   map[string]interface{} `json:"new_values,omitempty"` // New field values
	// Cascade control fields for loop prevention
	TriggeredByAction bool   `json:"triggered_by_action,omitempty"` // True if this event was emitted by an action
	ExecutionChainID  string `json:"execution_chain_id,omitempty"`  // UUID to look up cached chain state for cycle detection
	CascadeDepth      int    `json:"cascade_depth,omitempty"`       // Depth level of this event (0 = user-triggered)
}

// ExecutionContext holds context during action execution
type ExecutionContext struct {
	Action      *Action                `json:"action"`
	Event       *ActionEvent           `json:"event"`
	Item        *Item                  `json:"item,omitempty"`
	Actor       *User                  `json:"actor,omitempty"`
	Variables   map[string]interface{} `json:"variables,omitempty"` // Dynamic variables during execution
	StepResults []StepResult           `json:"step_results,omitempty"`
	// ChainID is set when this action is part of a cascade chain (for emitting chained events)
	ChainID string `json:"-"` // Not serialized - internal use only
}

// StepResult holds the result of executing a single node
type StepResult struct {
	NodeID       int                    `json:"node_id"`
	NodeType     ActionNodeType         `json:"node_type"`
	Status       ActionExecutionStatus  `json:"status"`
	StartedAt    time.Time              `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Output       map[string]interface{} `json:"output,omitempty"`
}

// Node configuration types

// SetFieldNodeConfig configures a set_field node
type SetFieldNodeConfig struct {
	FieldName string `json:"field_name"`
	Value     string `json:"value"` // Can contain {{variable}} templates
}

// SetStatusNodeConfig configures a set_status node
type SetStatusNodeConfig struct {
	StatusID int `json:"status_id"`
}

// AddCommentNodeConfig configures an add_comment node
type AddCommentNodeConfig struct {
	Content   string `json:"content"` // Can contain {{variable}} templates
	IsPrivate bool   `json:"is_private"`
}

// NotifyUserNodeConfig configures a notify_user node
type NotifyUserNodeConfig struct {
	Recipients  []string `json:"recipients"`      // "assignee", "creator", or specific user IDs
	Message     string   `json:"message"`         // Can contain {{variable}} templates
	Title       string   `json:"title,omitempty"`
	IncludeLink bool     `json:"include_link"` // Include link to item
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

// API Request/Response types

// CreateActionRequest represents the API request to create an action
type CreateActionRequest struct {
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	TriggerType   ActionTriggerType `json:"trigger_type"`
	TriggerConfig string            `json:"trigger_config,omitempty"`
	Nodes         []ActionNode      `json:"nodes,omitempty"`
	Edges         []ActionEdge      `json:"edges,omitempty"`
}

// UpdateActionRequest represents the API request to update an action
type UpdateActionRequest struct {
	Name          *string            `json:"name,omitempty"`
	Description   *string            `json:"description,omitempty"`
	TriggerType   *ActionTriggerType `json:"trigger_type,omitempty"`
	TriggerConfig *string            `json:"trigger_config,omitempty"`
	IsEnabled     *bool              `json:"is_enabled,omitempty"`
	Nodes         []ActionNode       `json:"nodes,omitempty"`
	Edges         []ActionEdge       `json:"edges,omitempty"`
}
