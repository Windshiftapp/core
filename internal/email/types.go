package email

import (
	"context"
	"time"

	"windshift/internal/models"
)

// ParsedEmail represents a parsed email message from IMAP
type ParsedEmail struct {
	UID         uint32
	MessageID   string
	InReplyTo   string
	References  []string
	From        EmailAddress
	To          []EmailAddress
	Subject     string
	Date        time.Time
	PlainBody   string
	HTMLBody    string
	Attachments []Attachment
	RawHeaders  map[string][]string
}

// EmailAddress represents an email address with optional display name
type EmailAddress struct {
	Name    string
	Address string
}

// Attachment represents an email attachment
type Attachment struct {
	Filename    string
	ContentType string
	Size        int64
	Data        []byte
}

// OAuthTokens represents OAuth tokens returned from token exchange
type OAuthTokens struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresAt    *time.Time
	Scope        string
}

// ProcessingResult represents the outcome of processing an email
type ProcessingResult struct {
	Action       ProcessingAction
	ItemID       *int
	CommentID    *int
	CustomerID   *int
	ErrorMessage string
}

// ProcessingAction represents what happened when processing an email
type ProcessingAction string

const (
	ActionItemCreated   ProcessingAction = "item_created"
	ActionCommentAdded  ProcessingAction = "comment_added"
	ActionSkipped       ProcessingAction = "skipped"
	ActionError         ProcessingAction = "error"
	ActionAlreadyExists ProcessingAction = "already_exists"
)

// Provider defines the interface for email providers (Microsoft, Google, Generic)
type Provider interface {
	// GetType returns the provider type identifier
	GetType() string

	// GetIMAPServer returns the IMAP server host and port for this provider
	GetIMAPServer(config *models.ChannelConfig) (host string, port int)

	// Connect establishes an IMAP connection using the provider's auth method
	Connect(ctx context.Context, config *models.ChannelConfig) (*Client, error)

	// TestConnection tests if the IMAP connection can be established
	TestConnection(ctx context.Context, config *models.ChannelConfig) error
}

// OAuthProvider extends Provider with OAuth-specific functionality
type OAuthProvider interface {
	Provider

	// GetOAuthURL returns the authorization URL for OAuth flow
	GetOAuthURL(state, redirectURI string) string

	// ExchangeCode exchanges an authorization code for tokens
	ExchangeCode(ctx context.Context, code, redirectURI string) (*OAuthTokens, error)

	// RefreshToken refreshes an expired access token
	RefreshToken(ctx context.Context, refreshToken string) (*OAuthTokens, error)

	// GetUserEmail retrieves the email address of the authenticated user
	GetUserEmail(ctx context.Context, accessToken string) (string, error)
}

// IMAPAuth represents authentication method for IMAP
type IMAPAuth interface {
	// Authenticate performs authentication on the IMAP client
	Authenticate(c *Client) error
}

// BasicAuth implements IMAP basic authentication
type BasicAuth struct {
	Username string
	Password string
}

// XOAuth2Auth implements IMAP XOAUTH2 SASL authentication (for OAuth providers)
type XOAuth2Auth struct {
	Email       string
	AccessToken string
}

// ProviderConfig holds provider-specific configuration
type ProviderConfig struct {
	// OAuth settings
	ClientID     string
	ClientSecret string
	TenantID     string // Microsoft-specific
	Scopes       []string

	// IMAP settings (for generic provider)
	IMAPHost       string
	IMAPPort       int
	IMAPEncryption string // "ssl", "tls", "none"
}

// FetchOptions configures how messages are fetched from IMAP
type FetchOptions struct {
	Mailbox   string
	SinceUID  uint32
	BatchSize int
}

// DefaultFetchOptions returns default fetch options
func DefaultFetchOptions() FetchOptions {
	return FetchOptions{
		Mailbox:   "INBOX",
		SinceUID:  0,
		BatchSize: 50,
	}
}
