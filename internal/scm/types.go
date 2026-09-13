// Package scm provides SCM (Source Control Management) provider integration
// for GitHub, GitLab, and Gitea/Forgejo.
package scm

import (
	"context"
	"time"

	"windshift/internal/models"
)

// Provider defines the interface that all SCM providers must implement
type Provider interface {
	// GetType returns the provider type (github, gitlab, gitea)
	GetType() models.SCMProviderType

	// TestConnection tests if the provider connection is working
	TestConnection(ctx context.Context) error

	// ListRepositories lists all accessible repositories
	ListRepositories(ctx context.Context, opts ListRepositoriesOptions) ([]Repository, error)

	// GetRepository gets details about a specific repository
	GetRepository(ctx context.Context, owner, repo string) (*Repository, error)

	// ListPullRequests lists pull requests for a repository
	ListPullRequests(ctx context.Context, owner, repo string, opts ListPROptions) ([]PullRequest, error)

	// GetPullRequest gets details about a specific pull request
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error)

	// ListPullRequestCommits lists commits contained in a pull request.
	// Pagination is handled internally; capped to avoid unbounded fetches.
	ListPullRequestCommits(ctx context.Context, owner, repo string, number int) ([]Commit, error)

	// CreateBranch creates a new branch
	CreateBranch(ctx context.Context, owner, repo, branchName, baseBranch string) error

	// CreatePullRequest creates a new pull request
	CreatePullRequest(ctx context.Context, owner, repo string, opts CreatePROptions) (*PullRequest, error)

	// GetCommit gets details about a specific commit
	GetCommit(ctx context.Context, owner, repo, sha string) (*Commit, error)

	// ListBranches lists branches for a repository
	ListBranches(ctx context.Context, owner, repo string) ([]Branch, error)

	// RegisterWebhook registers a webhook for repository events
	RegisterWebhook(ctx context.Context, owner, repo string, opts WebhookOptions) (*WebhookRegistration, error)

	// DeleteWebhook removes a registered webhook
	DeleteWebhook(ctx context.Context, owner, repo, webhookID string) error
}

// OAuthProvider extends Provider for providers that support OAuth authentication
type OAuthProvider interface {
	Provider

	// GetOAuthURL returns the URL to start the OAuth flow
	GetOAuthURL(state, redirectURI string) string

	// ExchangeCode exchanges an OAuth code for tokens
	ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthTokens, error)

	// RefreshToken refreshes an expired access token
	RefreshToken(ctx context.Context, refreshToken string) (*OAuthTokens, error)

	// GetCurrentUser returns the authenticated user's info from the SCM provider
	// This is used to store the SCM username/avatar when a user connects their account
	GetCurrentUser(ctx context.Context) (*User, error)
}

// TokenRevoker is implemented by providers that support remote revocation
// of an OAuth access token. Disconnect handlers call this best-effort: a
// failure here is logged but does not block the local token deletion,
// since the user has already asked to disconnect.
type TokenRevoker interface {
	RevokeToken(ctx context.Context, accessToken string) error
}

// ListRepositoriesOptions contains options for listing repositories
type ListRepositoriesOptions struct {
	Page         int
	PerPage      int
	Organization string // Filter by organization (if supported)
	Visibility   string // public, private, all
	Sort         string // created, updated, pushed, full_name
}

// ListPROptions contains options for listing pull requests
type ListPROptions struct {
	State     string // open, closed, all
	Page      int
	PerPage   int
	Sort      string // created, updated, popularity, long-running
	Direction string // asc, desc
}

// ListCommitsOptions contains options for listing commits from a repository.
// Sha is usually a branch or tag name; Since is an optional lower bound on the
// commit timestamp. Pagination is provider-specific but exposed uniformly here.
type ListCommitsOptions struct {
	Sha     string
	Since   *time.Time
	Page    int
	PerPage int
}

// CreatePROptions contains options for creating a pull request
type CreatePROptions struct {
	Title      string
	Body       string
	HeadBranch string
	BaseBranch string
	Draft      bool
}

// WebhookOptions contains options for webhook registration
type WebhookOptions struct {
	URL         string
	Secret      string
	Events      []string // Events to subscribe to (e.g., "push", "pull_request")
	ContentType string   // application/json or application/x-www-form-urlencoded
}

// Repository represents a repository from an SCM provider
type Repository struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"` // owner/repo
	Description   string    `json:"description,omitempty"`
	URL           string    `json:"url"`
	CloneURL      string    `json:"clone_url"`
	SSHURL        string    `json:"ssh_url,omitempty"`
	DefaultBranch string    `json:"default_branch"`
	IsPrivate     bool      `json:"is_private"`
	IsArchived    bool      `json:"is_archived"`
	Owner         string    `json:"owner"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PullRequest represents a pull request from an SCM provider
type PullRequest struct {
	ID         int    `json:"id"`
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Body       string `json:"body,omitempty"`
	State      string `json:"state"` // open, closed, merged
	URL        string `json:"url"`
	HeadBranch string `json:"head_branch"`
	// HeadRepo is the full owner/repo containing HeadBranch. It differs from
	// the base repository for fork PRs and must be known before granting push.
	HeadRepo   string     `json:"head_repo,omitempty"`
	HeadSHA    string     `json:"head_sha"`
	BaseBranch string     `json:"base_branch"`
	IsMerged   bool       `json:"is_merged"`
	IsDraft    bool       `json:"is_draft"`
	Author     User       `json:"author"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	MergedAt   *time.Time `json:"merged_at,omitempty"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
}

// Commit represents a commit from an SCM provider
type Commit struct {
	SHA       string    `json:"sha"`
	Message   string    `json:"message"`
	URL       string    `json:"url"`
	Author    User      `json:"author"`
	Committer User      `json:"committer"`
	CreatedAt time.Time `json:"created_at"`
}

// Branch represents a branch from an SCM provider
type Branch struct {
	Name      string `json:"name"`
	SHA       string `json:"sha"`
	IsDefault bool   `json:"is_default"`
	Protected bool   `json:"protected"`
}

// Tag represents a git tag (lightweight or annotated) from an SCM provider.
// CreatedAt is the tagger date when available, else the underlying commit's
// committer date — providers fall back rather than leaving this zero so
// `since` filtering works consistently.
type Tag struct {
	Name      string    `json:"name"`
	SHA       string    `json:"sha"` // target commit SHA
	URL       string    `json:"url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// User represents a user from an SCM provider
type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// OAuthTokens represents OAuth tokens returned after authentication
type OAuthTokens struct {
	AccessToken  string     `json:"access_token"`
	TokenType    string     `json:"token_type"`
	RefreshToken string     `json:"refresh_token,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Scope        string     `json:"scope,omitempty"`
}

// WebhookRegistration represents a registered webhook
type WebhookRegistration struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// WebhookPayload represents a parsed webhook payload
type WebhookPayload struct {
	EventType  string         `json:"event_type"`
	Action     string         `json:"action,omitempty"`
	Repository Repository     `json:"repository"`
	Sender     User           `json:"sender"`
	Raw        map[string]any `json:"raw,omitempty"`
	// Specific payloads
	PullRequest *PullRequest `json:"pull_request,omitempty"`
	Commit      *Commit      `json:"commit,omitempty"`
	Branch      *Branch      `json:"branch,omitempty"`
}

// ProviderConfig holds configuration for creating a provider instance
type ProviderConfig struct {
	ProviderType models.SCMProviderType
	AuthMethod   models.SCMAuthMethod
	BaseURL      string // For self-hosted instances

	// OAuth credentials
	OAuthClientID     string
	OAuthClientSecret string
	OAuthAccessToken  string
	OAuthRefreshToken string

	// Personal Access Token
	PersonalAccessToken string

	// GitHub App credentials
	GitHubAppID             string
	GitHubAppPrivateKey     string
	GitHubAppInstallationID string
}

// GitHubAppInstallation represents a GitHub App installation
type GitHubAppInstallation struct {
	ID               int64  `json:"id"`
	AccountLogin     string `json:"account_login"`
	AccountType      string `json:"account_type"` // "Organization" or "User"
	AccountID        int64  `json:"account_id"`
	AccountAvatarURL string `json:"account_avatar_url,omitempty"`
}

// Release represents a release from an SCM provider
type Release struct {
	ID           string                   `json:"id"`
	TagName      string                   `json:"tag_name"`
	TagURL       string                   `json:"tag_url,omitempty"`
	Name         string                   `json:"name"`
	Body         string                   `json:"body"`
	URL          string                   `json:"url"`
	Status       string                   `json:"status,omitempty"`
	Assets       []models.SCMReleaseAsset `json:"assets,omitempty"`
	IsDraft      bool                     `json:"is_draft"`
	IsPrerelease bool                     `json:"is_prerelease"`
	CreatedAt    time.Time                `json:"created_at"`
	PublishedAt  *time.Time               `json:"published_at,omitempty"`
	ReleasedAt   *time.Time               `json:"released_at,omitempty"`
}

// ReleaseCandidate is an existing tag that can be attached to a milestone.
// Release is populated when the provider has a first-class release for it.
type ReleaseCandidate struct {
	TagName string   `json:"tag_name"`
	TagURL  string   `json:"tag_url,omitempty"`
	SHA     string   `json:"sha,omitempty"`
	Status  string   `json:"status"`
	Release *Release `json:"release,omitempty"`
}

// CreateReleaseOptions contains options for creating a release
type CreateReleaseOptions struct {
	TagName         string `json:"tag_name"`
	TargetCommitish string `json:"target_commitish,omitempty"` // branch or commit SHA
	Name            string `json:"name"`
	Body            string `json:"body"`
	IsDraft         bool   `json:"is_draft"`
	IsPrerelease    bool   `json:"is_prerelease"`
}

// ReleaseProvider is an optional interface for providers that support releases.
type ReleaseProvider interface {
	Provider
	CreateRelease(ctx context.Context, owner, repo string, opts CreateReleaseOptions) (*Release, error)
	ListReleases(ctx context.Context, owner, repo string) ([]Release, error)
}

// CommitProvider is an optional interface for providers that can list recent
// commits on a branch. SyncService feature-detects this with a type assertion
// and skips commit-message link discovery when unavailable.
type CommitProvider interface {
	Provider
	ListCommits(ctx context.Context, owner, repo string, opts ListCommitsOptions) ([]Commit, error)
}

// RefProvider is an optional interface for providers that expose git refs
// (tags + range-compare) needed by the milestone-from-tag automation.
// SyncService feature-detects this with a type assertion and skips the
// tag/release-branch sync paths if the provider does not implement it.
type RefProvider interface {
	Provider

	// ListTags returns tags whose target commit was created at or after
	// `since`. Implementations may return all tags and let the caller
	// filter when the provider has no native cutoff API; the contract is
	// that no tag *newer than* `since` is ever dropped.
	ListTags(ctx context.Context, owner, repo string, since time.Time) ([]Tag, error)

	// CompareCommits returns the commits reachable from `head` but not
	// from `base`, in chronological order (oldest first). Used to list
	// "what shipped" between two tags.
	CompareCommits(ctx context.Context, owner, repo, base, head string) ([]Commit, error)
}

// GitHubAppProvider extends Provider for GitHub App specific functionality
type GitHubAppProvider interface {
	Provider

	// ListAppInstallations lists all installations for the GitHub App
	ListAppInstallations(ctx context.Context) ([]GitHubAppInstallation, error)

	// GetInstallationAccessToken gets an access token for a specific installation
	GetInstallationAccessToken(ctx context.Context, installationID int64) (string, *time.Time, error)
}

// NewProvider creates a new SCM provider based on the configuration
func NewProvider(cfg ProviderConfig) (Provider, error) {
	switch cfg.ProviderType {
	case models.SCMProviderTypeGitHub:
		return NewGitHubProvider(cfg)
	case models.SCMProviderTypeGitLab:
		return NewGitLabProvider(cfg)
	case models.SCMProviderTypeGitea:
		return NewGiteaProvider(cfg)
	default:
		return nil, ErrUnsupportedProvider
	}
}

// IssueComment represents a comment on a GitHub issue
type IssueComment struct {
	ID                int64     `json:"id"`
	Kind              string    `json:"kind,omitempty"` // issue_comment | review | review_comment
	Body              string    `json:"body"`
	User              User      `json:"user"`
	AuthorAssociation string    `json:"author_association,omitempty"`
	Path              string    `json:"path,omitempty"`
	Line              int       `json:"line,omitempty"`
	Side              string    `json:"side,omitempty"`
	ThreadID          string    `json:"thread_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// IssueCommentProvider is the narrow slice of issue operations that the
// "@agent" PR-comment trigger (WI-426) needs: read a PR's comments and reply on
// it (a PR is an issue on both GitHub and Gitea). It is split out from the full
// IssueProvider so a provider only needs to support comments to drive the
// trigger — Gitea implements this without the issue↔item sync surface.
type IssueCommentProvider interface {
	Provider

	// CreateIssueComment creates a comment on an issue/PR and returns its id
	CreateIssueComment(ctx context.Context, owner, repo string, number int, body string) (int64, error)

	// ListIssueComments lists all comments on an issue/PR
	ListIssueComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error)

	// UpdateIssueComment updates an existing comment on an issue/PR
	UpdateIssueComment(ctx context.Context, owner, repo string, number int, commentID int64, body string) error
}

// PullRequestReviewProvider exposes the review surfaces that are distinct
// from issue comments on GitHub/Gitea: submitted review bodies, inline
// comments, and thread replies. Implementations return normalized events.
type PullRequestReviewProvider interface {
	Provider
	ListPullRequestReviewEvents(ctx context.Context, owner, repo string, number int) ([]IssueComment, error)
}

// RepositoryPermissionProvider answers whether an SCM identity may request a
// coding-agent push. The provider token performs the repository-level check;
// callers fail closed when it cannot be established.
type RepositoryPermissionProvider interface {
	Provider
	CanUserWriteRepository(ctx context.Context, owner, repo, username string) (bool, error)
}

// IssueProvider extends Provider for providers that support issue operations
type IssueProvider interface {
	IssueCommentProvider

	// ListIssues lists issues for a repository (excludes pull requests)
	ListIssues(ctx context.Context, owner, repo string, opts ListIssueOptions) ([]Issue, error)

	// GetIssue gets details about a specific issue
	GetIssue(ctx context.Context, owner, repo string, number int) (*Issue, error)

	// UpdateIssue updates an issue (state, title, body, labels, assignees, milestone)
	UpdateIssue(ctx context.Context, owner, repo string, number int, opts UpdateIssueOptions) (*Issue, error)

	// ListRepoLabels lists all labels for a repository
	ListRepoLabels(ctx context.Context, owner, repo string) ([]IssueLabel, error)

	// ListRepoMilestones lists all milestones for a repository
	ListRepoMilestones(ctx context.Context, owner, repo string) ([]IssueMilestone, error)
}

// PaginatedIssueProvider is an optional extension for providers that can report
// whether the remote API has another raw result page. This matters for APIs
// such as GitHub's issue listing, which also returns pull requests: after those
// are filtered out, the number of issues alone cannot determine whether the
// remote page was full.
type PaginatedIssueProvider interface {
	ListIssuesPage(ctx context.Context, owner, repo string, opts ListIssueOptions) (issues []Issue, hasNext bool, err error)
}

// ListIssueOptions contains options for listing issues
type ListIssueOptions struct {
	State   string     // open, closed, all
	Labels  []string   // Filter by labels
	Since   *time.Time // Only issues updated at or after this time
	Page    int
	PerPage int
}

// UpdateIssueOptions contains options for updating an issue
type UpdateIssueOptions struct {
	State     *string // open or closed
	Title     *string
	Body      *string
	Labels    []string // Set labels (replaces all)
	Assignees []string // Set assignees (replaces all)
	Milestone *int     // Set milestone number (0 to clear)
}

// Issue represents a GitHub issue
type Issue struct {
	ID        int64           `json:"id"`
	Number    int             `json:"number"`
	Title     string          `json:"title"`
	Body      string          `json:"body,omitempty"`
	State     string          `json:"state"` // open, closed
	URL       string          `json:"url"`
	Labels    []IssueLabel    `json:"labels,omitempty"`
	Assignees []User          `json:"assignees,omitempty"`
	Milestone *IssueMilestone `json:"milestone,omitempty"`
	Author    User            `json:"author"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	ClosedAt  *time.Time      `json:"closed_at,omitempty"`
}

// IssueLabel represents a label on a GitHub issue
type IssueLabel struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// IssueMilestone represents a milestone on a GitHub issue
type IssueMilestone struct {
	ID     int64  `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"` // open, closed
}

// Default API URLs for each provider
const (
	GitHubAPIURL = "https://api.github.com"
)
