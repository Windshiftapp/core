package plugins

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/services"
)

// SMTPSender defines the minimal interface needed by plugins to send mail.
type SMTPSender interface {
	Send(ctx context.Context, req SMTPSendRequest) error
}

// SCMService defines the interface needed by plugins to interact with SCM providers.
type SCMService interface {
	// CreateBranchForRepository creates a branch in a workspace repository.
	// Optional userID can be passed to use user-specific OAuth credentials.
	CreateBranchForRepository(ctx context.Context, workspaceRepoID int, branchName, baseBranch string, userID ...int) (string, error)
	// CreateItemSCMLink creates a link between an item and an SCM resource.
	CreateItemSCMLink(ctx context.Context, itemID, workspaceRepoID int, linkType, externalID, externalURL, title string) (int, error)
}

// ManagerOptions controls runtime behavior of the plugin manager.
type ManagerOptions struct {
	PluginTimeout        time.Duration
	MemoryLimit          uint64
	HTTPClient           *http.Client
	SMTPSender           SMTPSender
	SCMService           SCMService
	CommentService       *services.CommentService
	Logger               *slog.Logger
	Database             database.Database
	AdditionalPluginDirs []string
}

// Option configures the ManagerOptions.
type Option func(*ManagerOptions)

// WithTimeout sets a per-call timeout when invoking plugin exports.
func WithTimeout(d time.Duration) Option {
	return func(o *ManagerOptions) { o.PluginTimeout = d }
}

// WithMemoryLimit sets a soft memory ceiling in bytes (converted to wasm pages).
func WithMemoryLimit(memoryBytes uint64) Option {
	return func(o *ManagerOptions) { o.MemoryLimit = memoryBytes }
}

// WithHTTPClient overrides the HTTP client used by the http_fetch host function.
func WithHTTPClient(c *http.Client) Option {
	return func(o *ManagerOptions) { o.HTTPClient = c }
}

// WithSMTPSender wires a concrete SMTP sender for smtp_send host calls.
func WithSMTPSender(s SMTPSender) Option {
	return func(o *ManagerOptions) { o.SMTPSender = s }
}

// WithLogger overrides the logger used by the manager and host functions.
func WithLogger(l *slog.Logger) Option {
	return func(o *ManagerOptions) { o.Logger = l }
}

// WithDatabase sets the database for plugin host functions (KV store, create_comment, etc.).
func WithDatabase(db database.Database) Option {
	return func(o *ManagerOptions) { o.Database = db }
}

// WithSCMService sets the SCM service for plugin host functions (branch creation, etc.).
func WithSCMService(s SCMService) Option {
	return func(o *ManagerOptions) { o.SCMService = s }
}

// WithCommentService sets the comment service for plugin host functions (create_comment).
func WithCommentService(cs *services.CommentService) Option {
	return func(o *ManagerOptions) { o.CommentService = cs }
}

// WithAdditionalPluginDirs adds additional directories to search for plugins.
// This allows loading plugins from multiple locations (e.g., for separate plugin repositories).
func WithAdditionalPluginDirs(dirs ...string) Option {
	return func(o *ManagerOptions) {
		o.AdditionalPluginDirs = append(o.AdditionalPluginDirs, dirs...)
	}
}
