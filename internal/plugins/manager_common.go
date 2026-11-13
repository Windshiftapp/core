package plugins

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
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

// validPluginName matches only safe plugin names: alphanumeric, hyphens, underscores.
var validPluginName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// validatePluginName rejects names that could cause path traversal or injection.
func validatePluginName(name string) error {
	if name == "" {
		return fmt.Errorf("plugin name must not be empty")
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid plugin name: must not contain path separators or '..'")
	}
	if !validPluginName.MatchString(name) {
		return fmt.Errorf("invalid plugin name: must contain only alphanumeric characters, hyphens, and underscores")
	}
	return nil
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
	// MaxHTTPFetchResponseBytes bounds the response body http_fetch will read
	// from a plugin-controlled URL. The WASM memory limit does not bound the
	// Go-side read, so this is the ceiling that protects the process from a
	// hostile or buggy upstream. Zero falls back to the default.
	MaxHTTPFetchResponseBytes int64
	// LicenseVerifier, when set, enables license enforcement: a plugin only
	// contributes capabilities when its stored license verifies. Nil keeps the
	// legacy behavior where every loaded plugin's capabilities count, so it is
	// only correct when enforcement is intentionally unconfigured.
	LicenseVerifier LicenseVerifier
	// AllowPublicRoutes permits plugin routes that declare auth "none". Off by
	// default so a public route requires a deliberate operator opt-in.
	AllowPublicRoutes bool
}

// LicenseVerifier validates a plugin's license token. It returns nil when the
// license grants the named plugin its capabilities on this installation.
type LicenseVerifier func(pluginName string, license []byte) error

// Option configures the ManagerOptions.
type Option func(*ManagerOptions)

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

// WithLicenseVerifier enables license enforcement. A nil verifier (the zero
// case) keeps the legacy behavior where every loaded plugin's capabilities
// count regardless of licensing; a configured but unusable key must fail
// startup instead of falling back to that behavior.
func WithLicenseVerifier(verifier LicenseVerifier) Option {
	return func(o *ManagerOptions) {
		o.LicenseVerifier = verifier
	}
}

// WithAllowPublicRoutes permits plugin routes that declare auth "none".
func WithAllowPublicRoutes(allow bool) Option {
	return func(o *ManagerOptions) {
		o.AllowPublicRoutes = allow
	}
}

// WithMaxHTTPFetchResponseBytes bounds the http_fetch response body. Zero or a
// negative value keeps the default.
func WithMaxHTTPFetchResponseBytes(limit int64) Option {
	return func(o *ManagerOptions) {
		o.MaxHTTPFetchResponseBytes = limit
	}
}
