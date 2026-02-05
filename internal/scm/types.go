// Package scm provides SCM (Source Control Management) provider integration
// for GitHub and Gitea/Forgejo.
package scm

import (
	"context"
	"time"

	"windshift/internal/models"
)

// Provider defines the interface that all SCM providers must implement
type Provider interface {
	// GetType returns the provider type (github, gitea)
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
	State   string // open, closed, all
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
	ID         int        `json:"id"`
	Number     int        `json:"number"`
	Title      string     `json:"title"`
	Body       string     `json:"body,omitempty"`
	State      string     `json:"state"` // open, closed, merged
	URL        string     `json:"url"`
	HeadBranch string     `json:"head_branch"`
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
	EventType  string                 `json:"event_type"`
	Action     string                 `json:"action,omitempty"`
	Repository Repository             `json:"repository"`
	Sender     User                   `json:"sender"`
	Raw        map[string]interface{} `json:"raw,omitempty"`
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
	case models.SCMProviderTypeGitea:
		return NewGiteaProvider(cfg)
	default:
		return nil, ErrUnsupportedProvider
	}
}

// Default API URLs for each provider
const (
	GitHubAPIURL = "https://api.github.com"
)
