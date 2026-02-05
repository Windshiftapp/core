//go:build noplugins

package plugins

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
)

// SMTPSender defines the minimal interface needed by plugins to send mail.
type SMTPSender interface {
	Send(ctx context.Context, req SMTPSendRequest) error
}

// SCMService defines the interface needed by plugins to interact with SCM providers.
type SCMService interface {
	CreateBranchForRepository(ctx context.Context, workspaceRepoID int, branchName, baseBranch string, userID ...int) (string, error)
	CreateItemSCMLink(ctx context.Context, itemID, workspaceRepoID int, linkType, externalID, externalURL, title string) (int, error)
}

// ManagerOptions controls runtime behaviour of the plugin manager.
type ManagerOptions struct {
	PluginTimeout time.Duration
	MemoryLimit   uint64
	HTTPClient    *http.Client
	SMTPSender    SMTPSender
	SCMService    SCMService
	Logger        *slog.Logger
	Database      database.Database
}

// Option configures the ManagerOptions.
type Option func(*ManagerOptions)

func WithTimeout(d time.Duration) Option {
	return func(o *ManagerOptions) {
		o.PluginTimeout = d
	}
}

func WithMemoryLimit(bytes uint64) Option {
	return func(o *ManagerOptions) {
		o.MemoryLimit = bytes
	}
}

func WithHTTPClient(c *http.Client) Option {
	return func(o *ManagerOptions) {
		o.HTTPClient = c
	}
}

func WithSMTPSender(s SMTPSender) Option {
	return func(o *ManagerOptions) {
		o.SMTPSender = s
	}
}

func WithLogger(l *slog.Logger) Option {
	return func(o *ManagerOptions) {
		o.Logger = l
	}
}

func WithDatabase(db database.Database) Option {
	return func(o *ManagerOptions) {
		o.Database = db
	}
}

func WithSCMService(s SCMService) Option {
	return func(o *ManagerOptions) {
		o.SCMService = s
	}
}

// LoadedPlugin represents a loaded plugin instance.
// When built without the `plugins` build tag, the plugin system is disabled and no plugins are loaded.
type LoadedPlugin struct {
	Manifest   PluginManifest
	Metadata   PluginMetadata
	Routes     []Route
	Extensions []Extension
	Path       string
	Enabled    bool
}

// Manager handles plugin loading and lifecycle.
// When built without the `plugins` build tag, all operations return "plugins disabled" (or empty data).
type Manager struct {
	pluginDir string
	db        database.Database
	logger    *slog.Logger
}

func NewManager(pluginDir string, opts ...Option) *Manager {
	options := ManagerOptions{
		Logger: logger.Get(),
	}
	for _, opt := range opts {
		opt(&options)
	}

	return &Manager{
		pluginDir: pluginDir,
		db:        options.Database,
		logger:    options.Logger,
	}
}

func (m *Manager) SetDatabase(db database.Database) {
	m.db = db
}

func (m *Manager) SetSCMService(_ SCMService) {}

func (m *Manager) LoadPlugins() error { return nil }

func (m *Manager) LoadPlugin(_ string) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) UnloadPlugin(_ string) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) GetPlugin(_ string) (*LoadedPlugin, bool) { return nil, false }

func (m *Manager) ListPlugins() []*LoadedPlugin { return nil }

func (m *Manager) EnablePlugin(_ string) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) DisablePlugin(_ string) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) ReloadPlugin(_ string) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) HandleRequest(_ string, _ *HTTPRequest) (*HTTPResponse, error) {
	return nil, errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) CallPluginFunction(_, _ string, _ any) ([]byte, error) {
	return nil, errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) UploadPlugin(_ string, _ []byte) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) UploadPluginLegacy(_ string, _ []byte, _ []byte) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) DeletePlugin(_ string) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) Close() error { return nil }

func (m *Manager) GetAsset(pluginName, assetPath string) ([]byte, string, error) {
	cleanPath := filepath.Clean(assetPath)
	if strings.Contains(cleanPath, "..") {
		return nil, "", errors.New("invalid asset path")
	}

	fullPath := filepath.Join(m.pluginDir, pluginName, "assets", cleanPath)
	assetsDir := filepath.Join(m.pluginDir, pluginName, "assets")
	if !strings.HasPrefix(fullPath, assetsDir) {
		return nil, "", errors.New("asset path outside assets directory")
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, "", err
	}

	ext := filepath.Ext(assetPath)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		switch ext {
		case ".js":
			mimeType = "application/javascript"
		case ".css":
			mimeType = "text/css"
		case ".json":
			mimeType = "application/json"
		case ".html":
			mimeType = "text/html"
		default:
			mimeType = "application/octet-stream"
		}
	}

	return data, mimeType, nil
}

func (m *Manager) GetExtensions() map[string][]Extension {
	return make(map[string][]Extension)
}

// ReadPluginFile reads a file from a plugin directory.
func ReadPluginFile(pluginDir, pluginName, filename string) (io.ReadCloser, error) {
	filePath := filepath.Join(pluginDir, pluginName, filename)

	if !strings.HasPrefix(filePath, filepath.Join(pluginDir, pluginName)) {
		return nil, errors.New("invalid file path")
	}

	return os.Open(filePath)
}
