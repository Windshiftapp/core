//go:build !noplugins

package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/services"
	"windshift/internal/utils"

	securejoin "github.com/cyphar/filepath-securejoin"
	extism "github.com/extism/go-sdk"
)

// licenseTokenFileName stores the plugin's license token alongside the
// manifest and entry point so verification survives restarts.
const licenseTokenFileName = "license.token"

// defaultMaxHTTPFetchResponseBytes bounds the http_fetch response body. The
// WASM memory limit bounds what a plugin can hold, not what the host reads from
// an upstream, so this is the process-level ceiling.
const defaultMaxHTTPFetchResponseBytes = 32 << 20 // 32 MiB

// LoadedPlugin represents a loaded plugin instance backed by a compiled Extism module.
type LoadedPlugin struct {
	Manifest   PluginManifest
	Metadata   PluginMetadata
	Routes     []Route
	Extensions []Extension
	Path       string
	Enabled    bool
	// Licensed reports whether the plugin's license verified. It only gates
	// capabilities and host-function permissions when a LicenseVerifier is
	// configured.
	Licensed bool
	// Permissions is the effective host-function permission set after manifest
	// validation and license gating. Only these permissions can reach host
	// functions; an empty set leaves only log available. Manifest.Permissions
	// holds what the plugin requested.
	Permissions []string
	// Assets is the normalized per-prefix auth policy for served assets. An
	// asset that matches no entry requires an authenticated session.
	Assets   []Asset
	compiled *extism.CompiledPlugin
}

// Manager handles plugin loading and lifecycle.
type Manager struct {
	mu              sync.RWMutex
	plugins         map[string]*LoadedPlugin
	pluginDirs      []string
	httpClient      *http.Client
	smtpSender      SMTPSender
	scmService      SCMService
	commentService  *services.CommentService
	logger          *slog.Logger
	pluginTimeout   time.Duration
	memoryLimit     uint64
	maxFetchBytes   int64
	licenseVerifier LicenseVerifier
	// allowPublicRoutes permits plugin routes that declare auth "none".
	allowPublicRoutes bool
	hostFuncs         []extism.HostFunction
	db                database.Database

	// Plugin-declared periodic invocations, keyed by plugin name. Guarded by
	// its own mutex so DueSchedules from the scheduler tick doesn't contend
	// with plugin lookups or LoadPlugin. See internal/plugins/schedules.go.
	schedulesMu sync.Mutex
	schedules   map[string][]*scheduledPlugin
}

// pluginNameKey is the unexported context key under which the executing
// plugin's name is carried to host functions.
type pluginNameKey struct{}

// withPluginName returns a copy of ctx carrying the plugin name so host
// functions can retrieve it per-invocation instead of through shared state.
func withPluginName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, pluginNameKey{}, name)
}

// pluginNameFromContext returns the plugin name stored in ctx, or "" if absent.
func pluginNameFromContext(ctx context.Context) string {
	name, _ := ctx.Value(pluginNameKey{}).(string)
	return name
}

// pluginPermissionsKey is the unexported context key under which the executing
// plugin's effective host-function permissions are carried to host functions.
type pluginPermissionsKey struct{}

// withPluginPermissions returns a copy of ctx carrying the plugin's effective
// host-function permissions so host functions can enforce them per-invocation.
func withPluginPermissions(ctx context.Context, permissions []string) context.Context {
	return context.WithValue(ctx, pluginPermissionsKey{}, permissions)
}

// pluginPermissionsFromContext returns the effective permissions stored in ctx,
// or nil when absent. Absent means deny, not allow.
func pluginPermissionsFromContext(ctx context.Context) []string {
	permissions, _ := ctx.Value(pluginPermissionsKey{}).([]string)
	return permissions
}

// requirePermission reports whether the plugin executing in ctx declared the
// permission. It logs every denial; callers return their own typed error
// response. A missing plugin context denies, so metadata population and any
// future direct invocation cannot reach a gated host function.
func (m *Manager) requirePermission(ctx context.Context, permission string) bool {
	for _, granted := range pluginPermissionsFromContext(ctx) {
		if granted == permission {
			return true
		}
	}
	m.logger.Warn("plugin host function denied", "plugin", pluginNameFromContext(ctx), "permission", permission)
	return false
}

// NewManager creates a new plugin manager configured for Extism-backed plugins.
func NewManager(pluginDir string, opts ...Option) *Manager {
	options := ManagerOptions{
		PluginTimeout: 5 * time.Second,
		MemoryLimit:   64 * 1024 * 1024, // 64MiB default ceiling
		// Plugin http_fetch dials a plugin-controlled URL. Route it through the
		// SSRF-safe dialer (blocks loopback/RFC1918/link-local/CGNAT/metadata,
		// re-checked on every redirect hop) so a plugin cannot reach
		// 169.254.169.254 or internal services. Redirect-following is preserved.
		HTTPClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: utils.ConfigureHTTPTransport(&http.Transport{DialContext: utils.SafeNetDialer(10 * time.Second).DialContext}),
		},
		Logger: logger.Get(),
	}

	for _, opt := range opts {
		opt(&options)
	}

	if options.MaxHTTPFetchResponseBytes <= 0 {
		options.MaxHTTPFetchResponseBytes = defaultMaxHTTPFetchResponseBytes
	}

	// Build list of plugin directories: primary dir + any additional dirs
	pluginDirs := make([]string, 0, 1+len(options.AdditionalPluginDirs))
	pluginDirs = append(pluginDirs, pluginDir)
	pluginDirs = append(pluginDirs, options.AdditionalPluginDirs...)

	m := &Manager{
		plugins:           make(map[string]*LoadedPlugin),
		schedules:         make(map[string][]*scheduledPlugin),
		pluginDirs:        pluginDirs,
		httpClient:        options.HTTPClient,
		smtpSender:        options.SMTPSender,
		scmService:        options.SCMService,
		commentService:    options.CommentService,
		logger:            options.Logger,
		pluginTimeout:     options.PluginTimeout,
		memoryLimit:       options.MemoryLimit,
		maxFetchBytes:     options.MaxHTTPFetchResponseBytes,
		licenseVerifier:   options.LicenseVerifier,
		allowPublicRoutes: options.AllowPublicRoutes,
		db:                options.Database,
	}
	m.hostFuncs = m.buildHostFunctions()
	// A host function without a permission mapping is a programming error: it
	// would be callable without any declared permission. Fail the construction
	// (and therefore startup) rather than exposing it.
	if err := validateHostFunctionCatalog(m.hostFuncs); err != nil {
		panic(fmt.Sprintf("plugins: invalid host function catalog: %v", err))
	}
	return m
}

// SetDatabase sets the database for plugin host functions.
// This allows setting the database after manager creation (for circular dependency resolution).
func (m *Manager) SetDatabase(db database.Database) {
	m.db = db
}

// SetSCMService sets the SCM service for plugin host functions.
// This allows setting the service after manager creation (for circular dependency resolution).
func (m *Manager) SetSCMService(s SCMService) {
	m.scmService = s
}

// LoadPlugins loads all plugins from configured plugin directories.
func (m *Manager) LoadPlugins() error {
	for _, pluginDir := range m.pluginDirs {
		if err := m.loadPluginsFromDir(pluginDir); err != nil {
			m.logger.Warn("failed to load plugins from directory", "dir", pluginDir, "error", err)
		}
	}
	return nil
}

// loadPluginsFromDir loads all plugins from a single directory.
func (m *Manager) loadPluginsFromDir(pluginDir string) error {
	// Only create the primary plugins directory, not additional ones
	if pluginDir == m.pluginDirs[0] {
		if err := os.MkdirAll(pluginDir, 0o750); err != nil {
			return fmt.Errorf("failed to create plugins directory: %w", err)
		}
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Additional directories may not exist, that's okay
			m.logger.Debug("plugin directory does not exist", "dir", pluginDir)
			return nil
		}
		return fmt.Errorf("failed to read plugins directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Staging and backup directories from an interrupted upload start with
		// a dot; they are not plugins.
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		pluginPath := filepath.Join(pluginDir, entry.Name())
		if err := m.LoadPlugin(pluginPath); err != nil {
			m.logger.Warn("failed to load plugin", "path", pluginPath, "error", err)
		}
	}

	return nil
}

// LoadPlugin loads a single plugin from a directory and compiles its WASM.
func (m *Manager) LoadPlugin(pluginPath string) error {
	manifestPath := filepath.Join(pluginPath, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath) //nolint:gosec // G304 — pluginPath from os.ReadDir enumeration, hardcoded filename
	if err != nil {
		return fmt.Errorf("failed to read manifest.json: %w", err)
	}

	var manifest PluginManifest
	if err = json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest.json: %w", err)
	}

	if manifest.EntryPoint == "" {
		manifest.EntryPoint = "plugin.wasm"
	}

	// Validate schedules up-front so a malformed manifest fails load before we
	// touch the WASM compiler or any shared state. Registration happens after
	// the plugin is added to m.plugins below.
	if _, err = parsePluginSchedules(manifest.Schedules); err != nil {
		return fmt.Errorf("invalid plugin schedules: %w", err)
	}

	// Permissions are validated before compilation so an unknown name fails the
	// load with the offending value instead of leaving a host function denied.
	normalizedPermissions, err := normalizePermissions(manifest.Permissions)
	if err != nil {
		return fmt.Errorf("invalid plugin permissions: %w", err)
	}
	manifest.Permissions = normalizedPermissions

	// Validate editor-provider extensions before compilation so a malformed
	// manifest fails the load with the offending value.
	if err = normalizeEditorExtensions(manifest.Extensions); err != nil {
		return fmt.Errorf("invalid plugin extensions: %w", err)
	}

	// Asset auth is manifest-only and validated before compilation so a
	// malformed or ambiguous policy fails the load without touching the WASM
	// compiler or any shared state.
	normalizedAssets, err := normalizeAssets(manifest.Assets, m.allowPublicRoutes)
	if err != nil {
		return fmt.Errorf("invalid plugin assets: %w", err)
	}

	wasmPath := filepath.Join(pluginPath, manifest.EntryPoint)
	if _, err = os.Stat(wasmPath); err != nil {
		return fmt.Errorf("failed to read WASM file: %w", err)
	}

	extismManifest := m.buildExtismManifest(wasmPath)

	ctx := context.Background()
	compiled, err := extism.NewCompiledPlugin(ctx, extismManifest, m.pluginConfig(), m.hostFuncs)
	if err != nil {
		return fmt.Errorf("failed to compile plugin: %w", err)
	}

	// Close the compiled module if a later validation step fails, so a rejected
	// load does not leak a WASM runtime.
	loaded := false
	defer func() {
		if !loaded {
			_ = compiled.Close(context.Background())
		}
	}()

	licensed := m.verifyStoredLicense(pluginPath, manifest.Name)
	// Capability AND license: an unlicensed plugin gets no host-function
	// permissions even when its manifest declares them.
	var effectivePermissions []string
	if licensed {
		effectivePermissions = manifest.Permissions
	}

	plugin := &LoadedPlugin{
		Manifest: manifest,
		Metadata: PluginMetadata{
			Name:        manifest.Name,
			Version:     manifest.Version,
			Description: manifest.Description,
			Author:      manifest.Author,
		},
		Routes:      manifest.Routes,
		Path:        pluginPath,
		Enabled:     true,
		Licensed:    licensed,
		Permissions: effectivePermissions,
		Assets:      normalizedAssets,
		compiled:    compiled,
	}

	if err := m.populateMetadata(ctx, plugin); err != nil {
		m.logger.Warn("failed to fetch plugin metadata", "name", manifest.Name, "error", err)
		// Fallback: use manifest extensions even if metadata fetch fails
		plugin.Extensions = attachPluginName(manifest.Name, manifest.Extensions, nil)
	}

	// get_metadata can add editor providers the manifest did not declare, so
	// validate the resolved set as well.
	if err := normalizeEditorExtensions(plugin.Extensions); err != nil {
		return fmt.Errorf("invalid plugin extensions: %w", err)
	}

	// Route auth defaults to session and is validated here so a malformed or
	// ambiguous manifest fails the load before the plugin is registered.
	normalizedRoutes, err := normalizePluginRoutes(plugin.Routes, m.allowPublicRoutes)
	if err != nil {
		return fmt.Errorf("invalid plugin routes: %w", err)
	}
	plugin.Routes = normalizedRoutes

	for _, route := range plugin.Routes {
		if route.Auth == RouteAuthNone {
			m.logger.Warn("plugin route is public", "plugin", manifest.Name, "method", route.Method, "path", route.Path)
		}
	}

	for _, asset := range plugin.Assets {
		if asset.Auth == RouteAuthNone {
			m.logger.Warn("plugin asset is public", "plugin", manifest.Name, "path", asset.Path)
		}
	}

	// Loud load-time notice of the compatibility break: a plugin that predates
	// host-function permissions still relies on functions it never declared, and
	// those calls are now denied.
	if denied := deniedHostFunctions(plugin.Permissions); len(denied) > 0 {
		m.logger.Warn("plugin host functions are denied because their permission is not granted",
			"plugin", manifest.Name,
			"declared_permissions", manifest.Permissions,
			"denied_host_functions", denied)
	}

	// Close and unregister any previous runtime for this name so its compiled
	// module and schedules do not survive a replacement. All fallible
	// validation is done above, so nothing can fail between here and
	// registration.
	m.mu.RLock()
	_, replacing := m.plugins[manifest.Name]
	m.mu.RUnlock()
	if replacing {
		if err := m.UnloadPlugin(manifest.Name); err != nil {
			m.logger.Warn("failed to unload plugin being replaced", "name", manifest.Name, "error", err)
		}
	}

	m.mu.Lock()
	m.plugins[manifest.Name] = plugin
	m.mu.Unlock()

	// Schedules were pre-validated above, so this cannot fail with
	// ErrInvalidSchedule. Called outside m.mu to keep lock ordering simple
	// (registerSchedules takes only schedulesMu).
	if err := m.registerSchedules(manifest.Name, manifest.Schedules); err != nil {
		m.logger.Warn("failed to register plugin schedules", "name", manifest.Name, "error", err)
	}

	loaded = true
	m.logger.Info("loaded plugin", "name", manifest.Name, "version", manifest.Version, "routes", len(plugin.Routes))
	return nil
}

// populateMetadata instantiates a temporary instance to gather routes and extensions.
func (m *Manager) populateMetadata(ctx context.Context, plugin *LoadedPlugin) error {
	instance, err := plugin.compiled.Instance(ctx, extism.PluginInstanceConfig{})
	if err != nil {
		return err
	}
	defer func() { _ = instance.Close(ctx) }()

	metadata, err := m.callFunction(ctx, instance, "get_metadata", nil)
	if err == nil && len(metadata) > 0 {
		var meta PluginMetadata
		if jsonErr := json.Unmarshal(metadata, &meta); jsonErr == nil {
			plugin.Metadata = mergeMetadata(plugin.Metadata, meta)
		}
	}

	routes := plugin.Manifest.Routes

	routePayload, err := m.callFunction(ctx, instance, "get_routes", nil)
	if err == nil && len(routePayload) > 0 {
		if parsed := parseRoutes(routePayload); len(parsed) > 0 {
			routes = parsed
		}
	} else if len(plugin.Metadata.Routes) > 0 {
		routes = plugin.Metadata.Routes
	}

	plugin.Routes = routes
	plugin.Extensions = attachPluginName(plugin.Manifest.Name, plugin.Manifest.Extensions, plugin.Metadata.Extensions)
	return nil
}

// buildExtismManifest constructs the Extism manifest for a plugin.
func (m *Manager) buildExtismManifest(wasmPath string) extism.Manifest {
	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmFile{Path: wasmPath},
		},
		Timeout: uint64(m.pluginTimeout.Milliseconds()), //nolint:gosec // G115: value is bounded by domain constraints
	}

	if m.memoryLimit > 0 {
		const wasmPageSize = 64 * 1024
		pages := m.memoryLimit / wasmPageSize
		if pages == 0 {
			pages = 1
		}
		manifest.Memory = &extism.ManifestMemory{
			MaxPages: uint32(pages), //nolint:gosec // G115: value is bounded by domain constraints
		}
	}

	return manifest
}

// UnloadPlugin unloads a plugin by name.
func (m *Manager) UnloadPlugin(name string) error {
	// Stop any periodic invocations before tearing down the plugin runtime.
	// unregisterSchedules takes only schedulesMu, so doing this before
	// acquiring m.mu keeps lock ordering simple.
	m.unregisterSchedules(name)

	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	if p.compiled != nil {
		if err := p.compiled.Close(context.Background()); err != nil {
			m.logger.Warn("error closing plugin runtime", "name", name, "error", err)
		}
	}

	delete(m.plugins, name)
	m.logger.Info("unloaded plugin", "name", name)
	return nil
}

// GetPlugin returns a loaded plugin by name.
func (m *Manager) GetPlugin(name string) (*LoadedPlugin, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, exists := m.plugins[name]
	return p, exists
}

// ListPlugins returns all loaded plugins.
func (m *Manager) ListPlugins() []*LoadedPlugin {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugins := make([]*LoadedPlugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// EnablePlugin enables a plugin.
func (m *Manager) EnablePlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	p.Enabled = true
	return nil
}

// DisablePlugin disables a plugin.
func (m *Manager) DisablePlugin(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	p.Enabled = false
	return nil
}

// ReloadPlugin reloads a plugin.
func (m *Manager) ReloadPlugin(name string) error {
	m.mu.RLock()
	p, exists := m.plugins[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("plugin not found: %s", name)
	}

	pluginPath := p.Path

	if err := m.UnloadPlugin(name); err != nil {
		return err
	}

	return m.LoadPlugin(pluginPath)
}

// invokeEnabledPlugin looks up an enabled plugin, instantiates it under a
// timeout-bound context, and hands the resulting instance to fn. It wraps the
// "lookup + enabled check + instance" scaffold shared by HandleRequest,
// CallPluginFunction, and any other exported dispatcher.
func (m *Manager) invokeEnabledPlugin(
	pluginName string,
	fn func(ctx context.Context, instance *extism.Plugin) ([]byte, error),
) ([]byte, error) {
	m.mu.RLock()
	p, exists := m.plugins[pluginName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", pluginName)
	}
	if !p.Enabled {
		return nil, fmt.Errorf("plugin is disabled: %s", pluginName)
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.pluginTimeout)
	defer cancel()
	ctx = withPluginName(ctx, pluginName)
	ctx = withPluginPermissions(ctx, p.Permissions)

	instance, err := p.compiled.Instance(ctx, extism.PluginInstanceConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate plugin: %w", err)
	}
	defer func() { _ = instance.Close(ctx) }()

	return fn(ctx, instance)
}

// HandleRequest forwards an HTTP request to a plugin's handle_request export.
func (m *Manager) HandleRequest(pluginName string, req *HTTPRequest) (*HTTPResponse, error) {
	respBytes, err := m.invokeEnabledPlugin(pluginName, func(ctx context.Context, instance *extism.Plugin) ([]byte, error) {
		b, cfErr := m.callFunction(ctx, instance, "handle_request", req)
		if cfErr != nil {
			return nil, fmt.Errorf("failed to call plugin handler: %w", cfErr)
		}
		return b, nil
	})
	if err != nil {
		return nil, err
	}

	var response HTTPResponse
	if err := json.Unmarshal(respBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to parse plugin response: %w", err)
	}
	return &response, nil
}

// CallPluginFunction calls a specific export on a plugin (e.g. webhook handler).
// The plugin name is threaded through ctx so host functions can namespace KV access.
func (m *Manager) CallPluginFunction(pluginName, funcName string, payload any) ([]byte, error) {
	return m.invokeEnabledPlugin(pluginName, func(ctx context.Context, instance *extism.Plugin) ([]byte, error) {
		return m.callFunction(ctx, instance, funcName, payload)
	})
}

// validateManifest parses the manifest and resolves the plugin name. The
// requested name, when non-empty, must match the manifest name so the
// installed directory, registry key, and runtime identity cannot diverge.
func (m *Manager) validateManifest(name string, manifestData []byte) (PluginManifest, string, error) {
	var manifest PluginManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return manifest, "", fmt.Errorf("invalid manifest.json: %w", err)
	}
	if manifest.Name == "" {
		return manifest, "", fmt.Errorf("invalid manifest.json: name is required")
	}
	if err := validatePluginName(manifest.Name); err != nil {
		return manifest, "", err
	}
	if name != "" && name != manifest.Name {
		return manifest, "", fmt.Errorf("plugin name %q does not match manifest name %q", name, manifest.Name)
	}
	return manifest, manifest.Name, nil
}

// UploadPlugin handles plugin upload from a zip file.
func (m *Manager) UploadPlugin(name string, zipData []byte) error {
	return m.UploadPluginWithLicense(name, zipData, nil)
}

// UploadPluginWithLicense handles plugin upload with an optional license
// token. When a verifier is configured the license is verified immediately so
// the admin gets feedback; it is stored with the plugin either way and
// re-checked on every load.
func (m *Manager) UploadPluginWithLicense(name string, zipData, license []byte) error {
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("invalid zip file: %w", err)
	}

	manifestEntry, root, err := canonicalManifestEntry(zipReader.File)
	if err != nil {
		return err
	}

	manifestData, err := readZipEntry(manifestEntry)
	if err != nil {
		return fmt.Errorf("failed to read manifest from zip: %w", err)
	}

	manifest, pluginName, err := m.validateManifest(name, manifestData)
	if err != nil {
		return err
	}

	// Immediate feedback at upload time; the token is stored with the plugin
	// and re-verified on every load.
	if len(license) > 0 && m.licenseVerifier != nil {
		if err := m.licenseVerifier(manifest.Name, license); err != nil {
			return fmt.Errorf("license rejected: %w", err)
		}
	}

	// Extract into a staging directory first so a malformed archive or a failed
	// load cannot leave a partial installation behind.
	stagingPath, err := m.newStagingDir(pluginName)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stagingPath) }() // no-op once promoted

	if err := extractPluginArchive(zipReader.File, root, manifestEntry, manifestData, stagingPath); err != nil {
		return err
	}
	if len(license) > 0 {
		if err := os.WriteFile(filepath.Join(stagingPath, licenseTokenFileName), license, 0o600); err != nil {
			return fmt.Errorf("failed to store license: %w", err)
		}
	}
	if err := ensureEntryPoint(stagingPath, manifest); err != nil {
		return err
	}

	return m.promoteStagedPlugin(pluginName, stagingPath)
}

// UploadPluginLegacy handles plugin upload with separate WASM and manifest (backwards compatibility).
func (m *Manager) UploadPluginLegacy(name string, wasmData, manifestData []byte) error {
	manifest, pluginName, err := m.validateManifest(name, manifestData)
	if err != nil {
		return err
	}

	stagingPath, err := m.newStagingDir(pluginName)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stagingPath) }() // no-op once promoted

	if err := os.MkdirAll(filepath.Join(stagingPath, "assets"), 0o750); err != nil {
		return fmt.Errorf("failed to create assets directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stagingPath, "manifest.json"), manifestData, 0o640); err != nil { //nolint:gosec // G306: plugin files need owner rw, group r
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	entryPoint := manifest.EntryPoint
	if entryPoint == "" {
		entryPoint = "plugin.wasm"
	}
	wasmPath, err := securejoin.SecureJoin(stagingPath, entryPoint)
	if err != nil {
		return fmt.Errorf("invalid entry point path in manifest: %w", err)
	}
	if err := os.WriteFile(wasmPath, wasmData, 0o640); err != nil { //nolint:gosec // G306: plugin files need owner rw, group r
		return fmt.Errorf("failed to write WASM file: %w", err)
	}

	return m.promoteStagedPlugin(pluginName, stagingPath)
}

// newStagingDir creates a staging directory next to the live plugin directory
// so promotion is a same-filesystem rename. The leading dot keeps an
// interrupted upload from being picked up as a plugin on the next start.
func (m *Manager) newStagingDir(pluginName string) (string, error) {
	if err := os.MkdirAll(m.pluginDirs[0], 0o750); err != nil {
		return "", fmt.Errorf("failed to create plugin directory: %w", err)
	}
	stagingPath, err := os.MkdirTemp(m.pluginDirs[0], "."+pluginName+".staging-")
	if err != nil {
		return "", fmt.Errorf("failed to create staging directory: %w", err)
	}
	return stagingPath, nil
}

// promoteStagedPlugin moves a fully staged plugin into the live directory and
// loads it. The previous installation is moved aside first and restored if the
// promotion or load fails, so a failed upgrade never leaves a partial plugin.
func (m *Manager) promoteStagedPlugin(pluginName, stagingPath string) error {
	pluginPath := filepath.Join(m.pluginDirs[0], pluginName)

	backupPath := ""
	if _, err := os.Stat(pluginPath); err == nil {
		// The backup shares the leading dot so a crash mid-upgrade cannot leave
		// it to be loaded as a plugin on the next start.
		backupPath = filepath.Join(m.pluginDirs[0], "."+pluginName+".replaced")
		_ = os.RemoveAll(backupPath)
		if err := os.Rename(pluginPath, backupPath); err != nil {
			return fmt.Errorf("failed to move existing plugin aside: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to inspect plugin directory: %w", err)
	}

	if err := os.Rename(stagingPath, pluginPath); err != nil {
		if backupPath != "" {
			_ = os.Rename(backupPath, pluginPath)
		}
		return fmt.Errorf("failed to activate plugin: %w", err)
	}

	if err := m.LoadPlugin(pluginPath); err != nil {
		_ = os.RemoveAll(pluginPath)
		if backupPath != "" {
			_ = os.Rename(backupPath, pluginPath)
		}
		return err
	}

	if backupPath != "" {
		_ = os.RemoveAll(backupPath)
	}
	return nil
}

// ensureEntryPoint verifies the staged plugin contains the WASM module the
// manifest names, so a broken archive fails before promotion.
func ensureEntryPoint(stagingPath string, manifest PluginManifest) error {
	entryPoint := manifest.EntryPoint
	if entryPoint == "" {
		entryPoint = "plugin.wasm"
	}
	if _, err := os.Stat(filepath.Join(stagingPath, entryPoint)); err != nil {
		return fmt.Errorf("entry point %q not found in plugin archive: %w", entryPoint, err)
	}
	return nil
}

// canonicalManifestEntry locates exactly one manifest.json in the archive and
// returns the archive-root prefix every entry must share. A second
// manifest.json anywhere in the archive is rejected: extraction must not be
// able to overwrite the manifest that selected the destination.
func canonicalManifestEntry(files []*zip.File) (*zip.File, string, error) {
	var matches []*zip.File
	for _, file := range files {
		if file.FileInfo().IsDir() {
			continue
		}
		if path.Base(path.Clean(file.Name)) == "manifest.json" {
			matches = append(matches, file)
		}
	}
	switch len(matches) {
	case 0:
		return nil, "", fmt.Errorf("manifest.json not found in zip file")
	case 1:
	default:
		return nil, "", fmt.Errorf("zip file contains %d manifest.json entries; exactly one is required", len(matches))
	}

	entry := matches[0]
	root := path.Dir(path.Clean(entry.Name))
	if root == "." {
		root = ""
	}
	return entry, root, nil
}

// extractPluginArchive writes a validated archive into dest. Entries under
// assets/ keep their relative path so per-prefix asset policies can address
// them; other entries keep the legacy flattened layout. The manifest entry is
// written from the validated bytes rather than re-extracted, so the bytes on
// disk are exactly what was validated.
func extractPluginArchive(files []*zip.File, root string, manifestEntry *zip.File, manifestData []byte, dest string) error {
	assetsPath := filepath.Join(dest, "assets")
	if err := os.MkdirAll(assetsPath, 0o750); err != nil {
		return fmt.Errorf("failed to create assets directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dest, "manifest.json"), manifestData, 0o640); err != nil { //nolint:gosec // G306: plugin files need owner rw, group r
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	for _, file := range files {
		if file == manifestEntry || file.FileInfo().IsDir() {
			continue
		}
		// Skip symlinks to prevent symlink-based path traversal
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			continue
		}
		// Directory entries that did not set the directory bit.
		if strings.HasSuffix(file.Name, "/") {
			continue
		}

		rel, err := archiveRelativePath(file.Name, root)
		if err != nil {
			return err
		}

		var destPath string
		switch {
		case rel == "assets":
			continue
		case strings.HasPrefix(rel, "assets/"):
			// Preserve the directory structure under assets/ so a per-prefix
			// policy can address subdirectories.
			destPath, err = securejoin.SecureJoin(assetsPath, strings.TrimPrefix(rel, "assets/"))
		case strings.HasSuffix(path.Base(rel), ".js") || strings.HasSuffix(path.Base(rel), ".css"):
			// Legacy layout: root-level scripts and styles are served as assets.
			destPath, err = securejoin.SecureJoin(assetsPath, path.Base(rel))
		default:
			destPath, err = securejoin.SecureJoin(dest, rel)
		}
		if err != nil {
			return fmt.Errorf("invalid path in zip file: %s: %w", file.Name, err)
		}

		data, err := readZipEntry(file)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", rel, err)
		}
		if err := os.WriteFile(destPath, data, 0o640); err != nil { //nolint:gosec // G306: plugin files need owner rw, group r
			return fmt.Errorf("failed to write file %s: %w", rel, err)
		}
	}
	return nil
}

// archiveRelativePath resolves an archive entry name against the archive root
// and rejects absolute paths, entries outside the root, and traversal.
func archiveRelativePath(name, root string) (string, error) {
	clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	if clean == "." || clean == "/" {
		return "", fmt.Errorf("invalid zip entry %q", name)
	}
	if path.IsAbs(clean) {
		return "", fmt.Errorf("invalid absolute path in zip file: %s", name)
	}
	if root != "" {
		if clean == root {
			return "", fmt.Errorf("invalid zip entry %q: expected a file", name)
		}
		if !strings.HasPrefix(clean, root+"/") {
			return "", fmt.Errorf("zip entry %q is outside the plugin root %q", name, root)
		}
		clean = strings.TrimPrefix(clean, root+"/")
	}
	for _, segment := range strings.Split(clean, "/") {
		if segment == ".." {
			return "", fmt.Errorf("invalid path in zip file: %s", name)
		}
	}
	return clean, nil
}

// readZipEntry reads one archive entry in full.
func readZipEntry(file *zip.File) ([]byte, error) {
	rc, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s in zip: %w", file.Name, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s from zip: %w", file.Name, err)
	}
	return data, nil
}

// DeletePlugin removes a plugin from the filesystem.
func (m *Manager) DeletePlugin(name string) error {
	m.mu.RLock()
	plugin, exists := m.plugins[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// Store the path before unloading (unload removes from map)
	pluginPath := plugin.Path

	if err := m.UnloadPlugin(name); err != nil {
		return err
	}

	return os.RemoveAll(pluginPath)
}

// Close cleans up the plugin manager.
func (m *Manager) Close() error {
	for name := range m.plugins {
		_ = m.UnloadPlugin(name)
	}
	return nil
}

// GetAssetAuth returns the auth policy for a plugin asset. It errors when the
// plugin is unknown or disabled so callers can fail closed before reading the
// asset. An asset that matches no declared policy requires a session.
func (m *Manager) GetAssetAuth(pluginName, assetPath string) (RouteAuth, error) {
	m.mu.RLock()
	p, exists := m.plugins[pluginName]
	m.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("plugin not found: %s", pluginName)
	}
	if !p.Enabled {
		return "", fmt.Errorf("plugin is disabled: %s", pluginName)
	}

	return assetAuth(p.Assets, assetPath), nil
}

// GetAsset serves a static asset from a plugin's assets directory.
func (m *Manager) GetAsset(pluginName, assetPath string) (data []byte, contentType string, err error) {
	m.mu.RLock()
	p, exists := m.plugins[pluginName]
	m.mu.RUnlock()

	if !exists {
		return nil, "", fmt.Errorf("plugin not found: %s", pluginName)
	}

	if !p.Enabled {
		return nil, "", fmt.Errorf("plugin is disabled: %s", pluginName)
	}

	assetsDir := filepath.Join(p.Path, "assets")
	fullPath, err := securejoin.SecureJoin(assetsDir, assetPath)
	if err != nil {
		return nil, "", fmt.Errorf("invalid asset path: %w", err)
	}

	data, err = os.ReadFile(fullPath) //nolint:gosec // G304 — fullPath sanitized via securejoin.SecureJoin
	if err != nil {
		return nil, "", fmt.Errorf("failed to read asset: %w", err)
	}

	return data, mimeTypeForExt(assetPath), nil
}

// verifyStoredLicense reports whether the plugin's stored license (if any)
// satisfies the configured verifier. enforcement := verifier != nil.
func (m *Manager) verifyStoredLicense(pluginPath, pluginName string) bool {
	if m.licenseVerifier == nil {
		return true // enforcement off: every plugin counts
	}
	licenseBytes, err := os.ReadFile(filepath.Join(pluginPath, licenseTokenFileName)) //nolint:gosec // G304 — pluginPath from securejoin, fixed name
	if err != nil || len(licenseBytes) == 0 {
		m.logger.Warn("plugin has no license; capabilities and host-function permissions are disabled for it", "name", pluginName)
		return false
	}
	if err := m.licenseVerifier(pluginName, licenseBytes); err != nil {
		m.logger.Warn("plugin license verification failed; capabilities and host-function permissions are disabled for it", "name", pluginName, "error", err)
		return false
	}
	return true
}

// HasCapability checks if any enabled plugin provides the given capability.
// With license enforcement on, only plugins whose license verified count.
func (m *Manager) HasCapability(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, p := range m.plugins {
		if !p.Enabled {
			continue
		}
		if m.licenseVerifier != nil && !p.Licensed {
			continue
		}
		for _, cap := range p.Manifest.Capabilities {
			if cap == name {
				return true
			}
		}
	}
	return false
}

// GetCapabilities returns a deduplicated list of capabilities from all enabled plugins.
func (m *Manager) GetCapabilities() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	seen := make(map[string]struct{})
	var caps []string

	for _, p := range m.plugins {
		if !p.Enabled {
			continue
		}
		if m.licenseVerifier != nil && !p.Licensed {
			continue
		}
		for _, cap := range p.Manifest.Capabilities {
			if _, ok := seen[cap]; !ok {
				seen[cap] = struct{}{}
				caps = append(caps, cap)
			}
		}
	}
	return caps
}

// GetExtensions returns all extensions from enabled plugins.
func (m *Manager) GetExtensions() map[string][]Extension {
	m.mu.RLock()
	defer m.mu.RUnlock()

	extensionsByPoint := make(map[string][]Extension)

	for _, p := range m.plugins {
		if !p.Enabled {
			continue
		}
		if m.licenseVerifier != nil && !p.Licensed {
			continue
		}

		for _, ext := range p.Extensions {
			extensionsByPoint[ext.Point] = append(extensionsByPoint[ext.Point], ext)
		}
	}

	return extensionsByPoint
}

func (m *Manager) callFunction(ctx context.Context, instance *extism.Plugin, funcName string, payload any) ([]byte, error) {
	var input []byte
	if payload != nil {
		var err error
		input, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}

	rc, output, err := instance.CallWithContext(ctx, funcName, input)
	if err != nil {
		return nil, err
	}

	if rc != 0 {
		return nil, fmt.Errorf("plugin returned non-zero status: %d", rc)
	}

	return output, nil
}

func (m *Manager) pluginConfig() extism.PluginConfig {
	return extism.PluginConfig{
		// Enable WASI so plugins can use standard WASM runtimes, but do not add
		// any host filesystem preopens/allowed paths to the Extism manifest. File
		// access should go through explicit host functions (KV/HTTP/etc.) rather
		// than ambient access to the Windshift process filesystem.
		EnableWasi: true,
	}
}

// normalizePluginRoutes defaults an unset route auth to session and rejects an
// unknown value. It also rejects two routes that can match the same request
// while declaring different auth, so a request's required credential is
// unambiguous. Called after metadata resolution so manifest, get_routes, and
// get_metadata routes are all covered.
func normalizePluginRoutes(routes []Route, allowPublic bool) ([]Route, error) {
	normalized := make([]Route, len(routes))
	for i, route := range routes {
		switch route.Auth {
		case "":
			route.Auth = RouteAuthSession
		case RouteAuthNone:
			if !allowPublic {
				return nil, fmt.Errorf("route %s %s declares auth %q but public plugin routes are disabled (set PLUGIN_PUBLIC_ROUTES=true to allow)", route.Method, route.Path, RouteAuthNone)
			}
		case RouteAuthSession, RouteAuthAdmin:
		default:
			return nil, fmt.Errorf("route %s %s declares unknown auth %q", route.Method, route.Path, route.Auth)
		}
		normalized[i] = route
	}
	for i := range normalized {
		for j := i + 1; j < len(normalized); j++ {
			if normalized[i].Auth == normalized[j].Auth {
				continue
			}
			if routesOverlap(normalized[i], normalized[j]) {
				return nil, fmt.Errorf(
					"routes %s %s (%s) and %s %s (%s) can match the same request with different auth",
					normalized[i].Method, normalized[i].Path, normalized[i].Auth,
					normalized[j].Method, normalized[j].Path, normalized[j].Auth)
			}
		}
	}
	return normalized, nil
}

// normalizeEditorExtensions validates editor-provider extensions and defaults
// an unset mode to toggle. Non-editor extension points pass through unchanged.
// Called for the manifest extensions before compilation and again for the
// resolved set (manifest plus get_metadata) afterwards, so a provider declared
// either way is validated.
func normalizeEditorExtensions(extensions []Extension) error {
	for i := range extensions {
		ext := &extensions[i]
		if ext.Point != ExtensionPointEditorMarkdown {
			continue
		}
		if strings.TrimSpace(ext.Component) == "" {
			return fmt.Errorf("editor.markdown extension %q: component is required", ext.ID)
		}
		switch ext.Mode {
		case "":
			ext.Mode = EditorModeToggle
		case EditorModeToggle, EditorModeReplace:
		default:
			return fmt.Errorf("editor.markdown extension %q: mode must be %q or %q", ext.ID, EditorModeToggle, EditorModeReplace)
		}
		for _, entityType := range ext.EntityTypes {
			if _, ok := editorEntityTypes[entityType]; !ok {
				return fmt.Errorf("editor.markdown extension %q: unknown entity type %q", ext.ID, entityType)
			}
		}
	}
	return nil
}

// editorEntityTypes is the closed set of surfaces an editor provider may edit.
var editorEntityTypes = map[string]struct{}{
	"item":    {},
	"page":    {},
	"comment": {},
}

// routesOverlap reports whether two routes can match the same request. Methods
// overlap when either is empty (any) or they are equal.
func routesOverlap(a, b Route) bool {
	if a.Method != "" && b.Method != "" && a.Method != b.Method {
		return false
	}
	return routePatternsOverlap(a.Path, b.Path)
}

// routePatternsOverlap lives in assets.go so both build variants can use it.

func parseRoutes(data []byte) []Route {
	var wrapper struct {
		Routes []Route `json:"routes"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.Routes) > 0 {
		return wrapper.Routes
	}

	var routes []Route
	if err := json.Unmarshal(data, &routes); err == nil {
		return routes
	}

	return nil
}

func attachPluginName(pluginName string, fromManifest, fromMetadata []Extension) []Extension {
	extensions := make([]Extension, 0, len(fromManifest)+len(fromMetadata))
	for _, ext := range append(fromManifest, fromMetadata...) {
		ext.PluginName = pluginName
		extensions = append(extensions, ext)
	}
	return extensions
}

func mergeMetadata(base, meta PluginMetadata) PluginMetadata {
	if meta.Name != "" {
		base.Name = meta.Name
	}
	if meta.Version != "" {
		base.Version = meta.Version
	}
	if meta.Description != "" {
		base.Description = meta.Description
	}
	if meta.Author != "" {
		base.Author = meta.Author
	}
	if len(meta.Capabilities) > 0 {
		base.Capabilities = meta.Capabilities
	}
	if len(meta.Routes) > 0 {
		base.Routes = meta.Routes
	}
	if len(meta.Extensions) > 0 {
		base.Extensions = meta.Extensions
	}
	return base
}
