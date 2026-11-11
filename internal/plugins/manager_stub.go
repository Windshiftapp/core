//go:build noplugins

package plugins

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	securejoin "github.com/cyphar/filepath-securejoin"

	"windshift/internal/database"
	"windshift/internal/logger"
)

// LoadedPlugin represents a loaded plugin instance.
// When built without the `plugins` build tag, the plugin system is disabled and no plugins are loaded.
type LoadedPlugin struct {
	Manifest   PluginManifest
	Metadata   PluginMetadata
	Routes     []Route
	Extensions []Extension
	Path       string
	Enabled    bool
	// Licensed mirrors the plugins-build field. The noplugins stub never
	// loads plugins, so it stays false; it exists so callers that read
	// LoadedPlugin compile in both build variants.
	Licensed bool
	// Permissions mirrors the plugins-build field. The stub never loads
	// plugins, so it stays empty.
	Permissions []string
	// Assets mirrors the plugins-build field. The stub never loads plugins, so
	// it stays empty.
	Assets []Asset
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

func (m *Manager) UploadPluginWithLicense(_ string, _, _ []byte) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) UploadPluginLegacy(_ string, _ []byte, _ []byte) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) DeletePlugin(_ string) error {
	return errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) Close() error { return nil }

func (m *Manager) GetAssetAuth(_, _ string) (RouteAuth, error) {
	return "", errors.New("plugins disabled (built without -tags=plugins)")
}

func (m *Manager) GetAsset(pluginName, assetPath string) ([]byte, string, error) {
	if err := validatePluginName(pluginName); err != nil {
		return nil, "", err
	}

	assetsDir := filepath.Join(m.pluginDir, pluginName, "assets")
	fullPath, err := securejoin.SecureJoin(assetsDir, assetPath)
	if err != nil {
		return nil, "", fmt.Errorf("invalid asset path: %w", err)
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, "", err
	}

	return data, mimeTypeForExt(assetPath), nil
}

// DueSchedules returns no due schedules in the noplugins build — there are
// no loaded plugins to drive periodic invocations.
func (m *Manager) DueSchedules(_ time.Time) []DueSchedule { return nil }

func (m *Manager) HasCapability(_ string) bool { return false }

func (m *Manager) GetCapabilities() []string { return nil }

func (m *Manager) GetExtensions() map[string][]Extension {
	return make(map[string][]Extension)
}
