package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	extism "github.com/extism/go-sdk"
	"windshift/internal/database"
	"windshift/internal/logger"
)

// SMTPSender defines the minimal interface needed by plugins to send mail.
type SMTPSender interface {
	Send(ctx context.Context, req SMTPSendRequest) error
}

// SCMService defines the interface needed by plugins to interact with SCM providers.
type SCMService interface {
	// CreateBranchForRepository creates a branch in a workspace repository
	// Optional userID can be passed to use user-specific OAuth credentials
	CreateBranchForRepository(ctx context.Context, workspaceRepoID int, branchName, baseBranch string, userID ...int) (string, error)
	// CreateItemSCMLink creates a link between an item and an SCM resource
	CreateItemSCMLink(ctx context.Context, itemID, workspaceRepoID int, linkType, externalID, externalURL, title string) (int, error)
}

// ManagerOptions controls runtime behaviour of the plugin manager.
type ManagerOptions struct {
	PluginTimeout        time.Duration
	MemoryLimit          uint64
	HTTPClient           *http.Client
	SMTPSender           SMTPSender
	SCMService           SCMService
	Logger               *slog.Logger
	Database             database.Database
	AdditionalPluginDirs []string
}

// Option configures the ManagerOptions.
type Option func(*ManagerOptions)

// WithTimeout sets a per-call timeout when invoking plugin exports.
func WithTimeout(d time.Duration) Option {
	return func(o *ManagerOptions) {
		o.PluginTimeout = d
	}
}

// WithMemoryLimit sets a soft memory ceiling in bytes (converted to wasm pages).
func WithMemoryLimit(bytes uint64) Option {
	return func(o *ManagerOptions) {
		o.MemoryLimit = bytes
	}
}

// WithHTTPClient overrides the HTTP client used by the http_fetch host function.
func WithHTTPClient(c *http.Client) Option {
	return func(o *ManagerOptions) {
		o.HTTPClient = c
	}
}

// WithSMTPSender wires a concrete SMTP sender for smtp_send host calls.
func WithSMTPSender(s SMTPSender) Option {
	return func(o *ManagerOptions) {
		o.SMTPSender = s
	}
}

// WithLogger overrides the logger used by the manager and host functions.
func WithLogger(l *slog.Logger) Option {
	return func(o *ManagerOptions) {
		o.Logger = l
	}
}

// WithDatabase sets the database for plugin host functions (KV store, create_comment, etc.).
func WithDatabase(db database.Database) Option {
	return func(o *ManagerOptions) {
		o.Database = db
	}
}

// WithSCMService sets the SCM service for plugin host functions (branch creation, etc.).
func WithSCMService(s SCMService) Option {
	return func(o *ManagerOptions) {
		o.SCMService = s
	}
}

// WithAdditionalPluginDirs adds additional directories to search for plugins.
// This allows loading plugins from multiple locations (e.g., for separate plugin repositories).
func WithAdditionalPluginDirs(dirs ...string) Option {
	return func(o *ManagerOptions) {
		o.AdditionalPluginDirs = append(o.AdditionalPluginDirs, dirs...)
	}
}

// LoadedPlugin represents a loaded plugin instance backed by a compiled Extism module.
type LoadedPlugin struct {
	Manifest   PluginManifest
	Metadata   PluginMetadata
	Routes     []Route
	Extensions []Extension
	Path       string
	Enabled    bool
	compiled   *extism.CompiledPlugin
}

// Manager handles plugin loading and lifecycle.
type Manager struct {
	mu            sync.RWMutex
	plugins       map[string]*LoadedPlugin
	pluginDirs    []string
	httpClient    *http.Client
	smtpSender    SMTPSender
	scmService    SCMService
	logger        *slog.Logger
	pluginTimeout time.Duration
	memoryLimit   uint64
	hostFuncs     []extism.HostFunction
	db            database.Database

	// currentPluginName tracks which plugin is currently executing (for host function context)
	currentPluginMu   sync.RWMutex
	currentPluginName string
}

// NewManager creates a new plugin manager configured for Extism-backed plugins.
func NewManager(pluginDir string, opts ...Option) *Manager {
	options := ManagerOptions{
		PluginTimeout: 5 * time.Second,
		MemoryLimit:   64 * 1024 * 1024, // 64MiB default ceiling
		HTTPClient:    &http.Client{Timeout: 10 * time.Second},
		Logger:        logger.Get(),
	}

	for _, opt := range opts {
		opt(&options)
	}

	// Build list of plugin directories: primary dir + any additional dirs
	pluginDirs := []string{pluginDir}
	pluginDirs = append(pluginDirs, options.AdditionalPluginDirs...)

	m := &Manager{
		plugins:       make(map[string]*LoadedPlugin),
		pluginDirs:    pluginDirs,
		httpClient:    options.HTTPClient,
		smtpSender:    options.SMTPSender,
		scmService:    options.SCMService,
		logger:        options.Logger,
		pluginTimeout: options.PluginTimeout,
		memoryLimit:   options.MemoryLimit,
		db:            options.Database,
	}
	m.hostFuncs = m.buildHostFunctions()
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
		if err := os.MkdirAll(pluginDir, 0o755); err != nil {
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
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest.json: %w", err)
	}

	var manifest PluginManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest.json: %w", err)
	}

	if manifest.EntryPoint == "" {
		manifest.EntryPoint = "plugin.wasm"
	}

	wasmPath := filepath.Join(pluginPath, manifest.EntryPoint)
	if _, err := os.Stat(wasmPath); err != nil {
		return fmt.Errorf("failed to read WASM file: %w", err)
	}

	extismManifest := m.buildExtismManifest(wasmPath)

	ctx := context.Background()
	compiled, err := extism.NewCompiledPlugin(ctx, extismManifest, m.pluginConfig(), m.hostFuncs)
	if err != nil {
		return fmt.Errorf("failed to compile plugin: %w", err)
	}

	plugin := &LoadedPlugin{
		Manifest: manifest,
		Metadata: PluginMetadata{
			Name:        manifest.Name,
			Version:     manifest.Version,
			Description: manifest.Description,
			Author:      manifest.Author,
		},
		Routes:   manifest.Routes,
		Path:     pluginPath,
		Enabled:  true,
		compiled: compiled,
	}

	if err := m.populateMetadata(ctx, plugin); err != nil {
		m.logger.Warn("failed to fetch plugin metadata", "name", manifest.Name, "error", err)
	}

	m.mu.Lock()
	m.plugins[manifest.Name] = plugin
	m.mu.Unlock()

	m.logger.Info("loaded plugin", "name", manifest.Name, "version", manifest.Version, "routes", len(plugin.Routes))
	return nil
}

// populateMetadata instantiates a temporary instance to gather routes and extensions.
func (m *Manager) populateMetadata(ctx context.Context, plugin *LoadedPlugin) error {
	instance, err := plugin.compiled.Instance(ctx, extism.PluginInstanceConfig{})
	if err != nil {
		return err
	}
	defer instance.Close(ctx)

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
		Timeout: uint64(m.pluginTimeout.Milliseconds()),
	}

	if m.memoryLimit > 0 {
		const wasmPageSize = 64 * 1024
		pages := m.memoryLimit / wasmPageSize
		if pages == 0 {
			pages = 1
		}
		manifest.Memory = &extism.ManifestMemory{
			MaxPages: uint32(pages),
		}
	}

	return manifest
}

// UnloadPlugin unloads a plugin by name.
func (m *Manager) UnloadPlugin(name string) error {
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

// HandleRequest forwards HTTP request to plugin.
func (m *Manager) HandleRequest(pluginName string, req *HTTPRequest) (*HTTPResponse, error) {
	m.mu.RLock()
	p, exists := m.plugins[pluginName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", pluginName)
	}

	if !p.Enabled {
		return nil, fmt.Errorf("plugin is disabled: %s", pluginName)
	}

	// Set current plugin context for host functions (KV operations, etc.)
	m.setCurrentPlugin(pluginName)
	defer m.clearCurrentPlugin()

	ctx, cancel := context.WithTimeout(context.Background(), m.pluginTimeout)
	defer cancel()

	instance, err := p.compiled.Instance(ctx, extism.PluginInstanceConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate plugin: %w", err)
	}
	defer instance.Close(ctx)

	respBytes, err := m.callFunction(ctx, instance, "handle_request", req)
	if err != nil {
		return nil, fmt.Errorf("failed to call plugin handler: %w", err)
	}

	var response HTTPResponse
	if err := json.Unmarshal(respBytes, &response); err != nil {
		return nil, fmt.Errorf("failed to parse plugin response: %w", err)
	}

	return &response, nil
}

// setCurrentPlugin sets the currently executing plugin name for host function context.
func (m *Manager) setCurrentPlugin(name string) {
	m.currentPluginMu.Lock()
	m.currentPluginName = name
	m.currentPluginMu.Unlock()
}

// clearCurrentPlugin clears the currently executing plugin name.
func (m *Manager) clearCurrentPlugin() {
	m.currentPluginMu.Lock()
	m.currentPluginName = ""
	m.currentPluginMu.Unlock()
}

// getCurrentPlugin returns the currently executing plugin name.
func (m *Manager) getCurrentPlugin() string {
	m.currentPluginMu.RLock()
	defer m.currentPluginMu.RUnlock()
	return m.currentPluginName
}

// CallPluginFunction calls a specific function on a plugin (for webhook handlers, etc.).
// This sets up the plugin context so host functions work correctly.
func (m *Manager) CallPluginFunction(pluginName, funcName string, payload any) ([]byte, error) {
	m.mu.RLock()
	p, exists := m.plugins[pluginName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", pluginName)
	}

	if !p.Enabled {
		return nil, fmt.Errorf("plugin is disabled: %s", pluginName)
	}

	// Set current plugin context for host functions
	m.setCurrentPlugin(pluginName)
	defer m.clearCurrentPlugin()

	ctx, cancel := context.WithTimeout(context.Background(), m.pluginTimeout)
	defer cancel()

	instance, err := p.compiled.Instance(ctx, extism.PluginInstanceConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate plugin: %w", err)
	}
	defer instance.Close(ctx)

	return m.callFunction(ctx, instance, funcName, payload)
}

// UploadPlugin handles plugin upload from a zip file.
func (m *Manager) UploadPlugin(name string, zipData []byte) error {
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("invalid zip file: %w", err)
	}

	var manifestData []byte
	var manifest PluginManifest
	for _, file := range zipReader.File {
		if file.Name == "manifest.json" || filepath.Base(file.Name) == "manifest.json" {
			rc, err := file.Open()
			if err != nil {
				return fmt.Errorf("failed to read manifest from zip: %w", err)
			}
			manifestData, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return fmt.Errorf("failed to read manifest data: %w", err)
			}
			break
		}
	}

	if manifestData == nil {
		return fmt.Errorf("manifest.json not found in zip file")
	}

	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("invalid manifest.json: %w", err)
	}

	if name == "" {
		name = manifest.Name
	}

	// Install plugins to the primary plugin directory
	pluginPath := filepath.Join(m.pluginDirs[0], name)
	if err := os.MkdirAll(pluginPath, 0o755); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	assetsPath := filepath.Join(pluginPath, "assets")
	if err := os.MkdirAll(assetsPath, 0o755); err != nil {
		return fmt.Errorf("failed to create assets directory: %w", err)
	}

	for _, file := range zipReader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file %s in zip: %w", file.Name, err)
		}

		fileData, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return fmt.Errorf("failed to read file %s from zip: %w", file.Name, err)
		}

		fileName := filepath.Base(file.Name)

		var destPath string
		if strings.HasSuffix(fileName, ".js") || strings.HasSuffix(fileName, ".css") ||
			strings.HasPrefix(filepath.Dir(file.Name), "assets") {
			destPath = filepath.Join(assetsPath, fileName)
		} else if fileName == "manifest.json" || strings.HasSuffix(fileName, ".wasm") {
			destPath = filepath.Join(pluginPath, fileName)
		} else {
			destPath = filepath.Join(pluginPath, fileName)
		}

		// Validate path stays within plugin directory (prevent path traversal)
		cleanDest := filepath.Clean(destPath)
		cleanBase := filepath.Clean(pluginPath) + string(os.PathSeparator)
		if !strings.HasPrefix(cleanDest, cleanBase) && cleanDest != filepath.Clean(pluginPath) {
			return fmt.Errorf("invalid path in zip file: %s", file.Name)
		}

		if err := os.WriteFile(destPath, fileData, 0o644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", fileName, err)
		}
	}

	return m.LoadPlugin(pluginPath)
}

// UploadPluginLegacy handles plugin upload with separate WASM and manifest (backwards compatibility).
func (m *Manager) UploadPluginLegacy(name string, wasmData []byte, manifestData []byte) error {
	var manifest PluginManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("invalid manifest.json: %w", err)
	}

	if name == "" {
		name = manifest.Name
	}

	// Install plugins to the primary plugin directory
	pluginPath := filepath.Join(m.pluginDirs[0], name)
	if err := os.MkdirAll(pluginPath, 0o755); err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	manifestPath := filepath.Join(pluginPath, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	wasmFileName := manifest.EntryPoint
	if wasmFileName == "" {
		wasmFileName = "plugin.wasm"
	}
	wasmPath := filepath.Join(pluginPath, wasmFileName)
	if err := os.WriteFile(wasmPath, wasmData, 0o644); err != nil {
		return fmt.Errorf("failed to write WASM file: %w", err)
	}

	return m.LoadPlugin(pluginPath)
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

// GetAsset serves a static asset from a plugin's assets directory.
func (m *Manager) GetAsset(pluginName, assetPath string) ([]byte, string, error) {
	m.mu.RLock()
	p, exists := m.plugins[pluginName]
	m.mu.RUnlock()

	if !exists {
		return nil, "", fmt.Errorf("plugin not found: %s", pluginName)
	}

	if !p.Enabled {
		return nil, "", fmt.Errorf("plugin is disabled: %s", pluginName)
	}

	cleanPath := filepath.Clean(assetPath)
	if strings.Contains(cleanPath, "..") {
		return nil, "", fmt.Errorf("invalid asset path")
	}

	fullPath := filepath.Join(p.Path, "assets", cleanPath)
	assetsDir := filepath.Join(p.Path, "assets")
	if !strings.HasPrefix(fullPath, assetsDir) {
		return nil, "", fmt.Errorf("asset path outside assets directory")
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read asset: %w", err)
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

// GetExtensions returns all extensions from enabled plugins.
func (m *Manager) GetExtensions() map[string][]Extension {
	m.mu.RLock()
	defer m.mu.RUnlock()

	extensionsByPoint := make(map[string][]Extension)

	for _, p := range m.plugins {
		if !p.Enabled {
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

func (m *Manager) buildHostFunctions() []extism.HostFunction {
	return []extism.HostFunction{
		extism.NewHostFunctionWithStack("log", m.logHostFunction, []extism.ValueType{extism.ValueTypeI64}, nil),
		extism.NewHostFunctionWithStack("smtp_send", m.smtpHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("http_fetch", m.httpFetchHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("cli_exec", m.cliExecHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("kv_get", m.kvGetHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("kv_set", m.kvSetHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("kv_delete", m.kvDeleteHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("create_comment", m.createCommentHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("scm_create_branch", m.scmCreateBranchHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
		extism.NewHostFunctionWithStack("scm_create_item_link", m.scmCreateItemLinkHostFunction, []extism.ValueType{extism.ValueTypeI64}, []extism.ValueType{extism.ValueTypeI64}),
	}
}

func (m *Manager) logHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("log host function failed to read payload", "error", err)
		return
	}

	var logReq LogRequest
	if err := json.Unmarshal(payload, &logReq); err != nil {
		m.logger.Warn("log host function failed to parse payload", "error", err)
		return
	}

	level := slog.LevelInfo
	switch strings.ToLower(logReq.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	case "info":
		level = slog.LevelInfo
	}

	m.logger.Log(ctx, level, logReq.Message)
}

func (m *Manager) smtpHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("smtp_send host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var sendReq SMTPSendRequest
	if err := json.Unmarshal(payload, &sendReq); err != nil {
		m.logger.Warn("smtp_send host function failed to parse payload", "error", err)
		stack[0] = 0
		return
	}

	result := SMTPSendResponse{Status: "ok"}
	if m.smtpSender == nil {
		result.Status = "error"
		result.Error = "smtp sender not configured"
	} else if err := m.smtpSender.Send(ctx, sendReq); err != nil {
		result.Status = "error"
		result.Error = err.Error()
	}

	m.writeHostResponse(plugin, stack, result)
}

func (m *Manager) httpFetchHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("http_fetch host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var fetchReq HTTPFetchRequest
	if err := json.Unmarshal(payload, &fetchReq); err != nil {
		m.logger.Warn("http_fetch host function failed to parse payload", "error", err)
		stack[0] = 0
		return
	}

	method := strings.ToUpper(fetchReq.Method)
	if method == "" {
		method = http.MethodGet
	}

	if fetchReq.URL == "" {
		m.writeHostResponse(plugin, stack, HTTPFetchResponse{Status: http.StatusBadRequest})
		return
	}

	client := m.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	timeout := m.pluginTimeout
	if fetchReq.TimeoutMs > 0 {
		timeout = time.Duration(fetchReq.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, fetchReq.URL, bytes.NewReader(fetchReq.Body))
	if err != nil {
		m.writeHostResponse(plugin, stack, HTTPFetchResponse{Status: http.StatusBadRequest, Body: []byte(err.Error())})
		return
	}

	for k, v := range fetchReq.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		m.writeHostResponse(plugin, stack, HTTPFetchResponse{Status: http.StatusBadGateway, Body: []byte(err.Error())})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	headers := make(map[string]string)
	for k, vals := range resp.Header {
		if len(vals) > 0 {
			headers[k] = vals[0]
		}
	}

	m.writeHostResponse(plugin, stack, HTTPFetchResponse{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    body,
	})
}

func (m *Manager) writeHostResponse(plugin *extism.CurrentPlugin, stack []uint64, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		m.logger.Warn("host response marshal failed", "error", err)
		stack[0] = 0
		return
	}

	ptr, err := plugin.WriteBytes(data)
	if err != nil {
		m.logger.Warn("host response write failed", "error", err)
		stack[0] = 0
		return
	}

	stack[0] = ptr
}

func (m *Manager) kvGetHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("kv_get host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var kvReq KVGetRequest
	if err := json.Unmarshal(payload, &kvReq); err != nil {
		m.logger.Warn("kv_get host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if kvReq.Key == "" {
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "key is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.db == nil {
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "database not configured"})
		return
	}

	var value string
	err = m.db.QueryRowContext(ctx,
		"SELECT value FROM plugin_kv_store WHERE plugin_name = ? AND key = ?",
		pluginName, kvReq.Key,
	).Scan(&value)

	if err == sql.ErrNoRows {
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "not_found"})
		return
	}
	if err != nil {
		m.logger.Warn("kv_get database error", "error", err, "plugin", pluginName, "key", kvReq.Key)
		m.writeHostResponse(plugin, stack, KVGetResponse{Status: "error", Error: "database error"})
		return
	}

	m.writeHostResponse(plugin, stack, KVGetResponse{Status: "ok", Value: value})
}

func (m *Manager) kvSetHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("kv_set host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var kvReq KVSetRequest
	if err := json.Unmarshal(payload, &kvReq); err != nil {
		m.logger.Warn("kv_set host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if kvReq.Key == "" {
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "key is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.db == nil {
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "database not configured"})
		return
	}

	now := time.Now()
	// Use upsert pattern - INSERT ... ON CONFLICT UPDATE
	var query string
	if m.db.GetDriverName() == "postgres" {
		query = `
			INSERT INTO plugin_kv_store (plugin_name, key, value, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (plugin_name, key) DO UPDATE SET value = $3, updated_at = $5
		`
	} else {
		query = `
			INSERT INTO plugin_kv_store (plugin_name, key, value, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (plugin_name, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`
	}

	_, err = m.db.ExecWriteContext(ctx, query, pluginName, kvReq.Key, kvReq.Value, now, now)
	if err != nil {
		m.logger.Warn("kv_set database error", "error", err, "plugin", pluginName, "key", kvReq.Key)
		m.writeHostResponse(plugin, stack, KVSetResponse{Status: "error", Error: "database error"})
		return
	}

	m.writeHostResponse(plugin, stack, KVSetResponse{Status: "ok"})
}

func (m *Manager) kvDeleteHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("kv_delete host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var kvReq KVDeleteRequest
	if err := json.Unmarshal(payload, &kvReq); err != nil {
		m.logger.Warn("kv_delete host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if kvReq.Key == "" {
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "key is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.db == nil {
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "database not configured"})
		return
	}

	_, err = m.db.ExecWriteContext(ctx,
		"DELETE FROM plugin_kv_store WHERE plugin_name = ? AND key = ?",
		pluginName, kvReq.Key,
	)
	if err != nil {
		m.logger.Warn("kv_delete database error", "error", err, "plugin", pluginName, "key", kvReq.Key)
		m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "error", Error: "database error"})
		return
	}

	m.writeHostResponse(plugin, stack, KVDeleteResponse{Status: "ok"})
}

func (m *Manager) createCommentHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("create_comment host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var req CreateCommentRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		m.logger.Warn("create_comment host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if req.ItemID <= 0 {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "item_id is required"})
		return
	}
	if req.AuthorID <= 0 {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "author_id is required"})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "content is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.db == nil {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "database not configured"})
		return
	}

	// Verify item exists
	var itemExists bool
	err = m.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM items WHERE id = ?)", req.ItemID).Scan(&itemExists)
	if err != nil || !itemExists {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "item not found"})
		return
	}

	// Verify author exists
	var authorExists bool
	err = m.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", req.AuthorID).Scan(&authorExists)
	if err != nil || !authorExists {
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "author not found"})
		return
	}

	// Convert plain text to TipTap JSON format
	content := convertToTipTapJSON(req.Content)

	// Insert the comment
	now := time.Now()
	var commentID int64
	err = m.db.QueryRowContext(ctx, `
		INSERT INTO comments (item_id, author_id, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, req.ItemID, req.AuthorID, content, now, now).Scan(&commentID)
	if err != nil {
		m.logger.Warn("create_comment database error", "error", err, "plugin", pluginName, "item_id", req.ItemID)
		m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "error", Error: "failed to create comment"})
		return
	}

	m.logger.Info("plugin created comment", "plugin", pluginName, "comment_id", commentID, "item_id", req.ItemID)
	m.writeHostResponse(plugin, stack, CreateCommentResponse{Status: "ok", CommentID: int(commentID)})
}

func (m *Manager) scmCreateBranchHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("scm_create_branch host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var req SCMCreateBranchRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		m.logger.Warn("scm_create_branch host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if req.WorkspaceRepositoryID <= 0 {
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "workspace_repository_id is required"})
		return
	}
	if strings.TrimSpace(req.BranchName) == "" {
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "branch_name is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.scmService == nil {
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: "SCM service not configured"})
		return
	}

	branchURL, err := m.scmService.CreateBranchForRepository(ctx, req.WorkspaceRepositoryID, req.BranchName, req.BaseBranch)
	if err != nil {
		m.logger.Warn("scm_create_branch failed", "error", err, "plugin", pluginName, "repo_id", req.WorkspaceRepositoryID)
		m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "error", Error: err.Error()})
		return
	}

	m.logger.Info("plugin created branch", "plugin", pluginName, "repo_id", req.WorkspaceRepositoryID, "branch", req.BranchName)
	m.writeHostResponse(plugin, stack, SCMCreateBranchResponse{Status: "ok", BranchURL: branchURL})
}

func (m *Manager) scmCreateItemLinkHostFunction(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
	payload, err := plugin.ReadBytes(stack[0])
	if err != nil {
		m.logger.Warn("scm_create_item_link host function failed to read payload", "error", err)
		stack[0] = 0
		return
	}

	var req SCMCreateItemLinkRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		m.logger.Warn("scm_create_item_link host function failed to parse payload", "error", err)
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "invalid request payload"})
		return
	}

	if req.ItemID <= 0 {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "item_id is required"})
		return
	}
	if req.WorkspaceRepositoryID <= 0 {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "workspace_repository_id is required"})
		return
	}
	if strings.TrimSpace(req.LinkType) == "" {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "link_type is required"})
		return
	}
	if strings.TrimSpace(req.ExternalID) == "" {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "external_id is required"})
		return
	}

	pluginName := m.getCurrentPlugin()
	if pluginName == "" {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "no plugin context"})
		return
	}

	if m.scmService == nil {
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: "SCM service not configured"})
		return
	}

	linkID, err := m.scmService.CreateItemSCMLink(ctx, req.ItemID, req.WorkspaceRepositoryID, req.LinkType, req.ExternalID, req.ExternalURL, req.Title)
	if err != nil {
		m.logger.Warn("scm_create_item_link failed", "error", err, "plugin", pluginName, "item_id", req.ItemID)
		m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "error", Error: err.Error()})
		return
	}

	m.logger.Info("plugin created item SCM link", "plugin", pluginName, "item_id", req.ItemID, "link_id", linkID)
	m.writeHostResponse(plugin, stack, SCMCreateItemLinkResponse{Status: "ok", LinkID: linkID})
}

// convertToTipTapJSON converts plain text to TipTap JSON format for rich text storage.
func convertToTipTapJSON(plainText string) string {
	// Split by newlines to create paragraphs
	lines := strings.Split(plainText, "\n")
	paragraphs := make([]map[string]interface{}, 0, len(lines))

	for _, line := range lines {
		if line == "" {
			// Empty line becomes empty paragraph
			paragraphs = append(paragraphs, map[string]interface{}{
				"type": "paragraph",
			})
		} else {
			paragraphs = append(paragraphs, map[string]interface{}{
				"type": "paragraph",
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": line,
					},
				},
			})
		}
	}

	doc := map[string]interface{}{
		"type":    "doc",
		"content": paragraphs,
	}

	jsonBytes, _ := json.Marshal(doc)
	return string(jsonBytes)
}

func (m *Manager) pluginConfig() extism.PluginConfig {
	return extism.PluginConfig{
		EnableWasi: true,
	}
}

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

func attachPluginName(pluginName string, fromManifest []Extension, fromMetadata []Extension) []Extension {
	var extensions []Extension
	for _, ext := range append(fromManifest, fromMetadata...) {
		ext.PluginName = pluginName
		extensions = append(extensions, ext)
	}
	return extensions
}

func mergeMetadata(base PluginMetadata, meta PluginMetadata) PluginMetadata {
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

// ReadPluginFile reads a file from a plugin directory.
func ReadPluginFile(pluginDir, pluginName, filename string) (io.ReadCloser, error) {
	filePath := filepath.Join(pluginDir, pluginName, filename)

	if !strings.HasPrefix(filePath, filepath.Join(pluginDir, pluginName)) {
		return nil, errors.New("invalid file path")
	}

	return os.Open(filePath)
}
