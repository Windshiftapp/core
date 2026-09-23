package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"windshift/internal/licensing"
	"windshift/internal/logger"
	"windshift/internal/plugins"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// PluginHandler handles plugin-related HTTP requests
type PluginHandler struct {
	manager         *plugins.Manager
	registry        *repository.PluginRegistryRepository
	auditor         *logger.Auditor
	pluginsDisabled bool
	licenseVerifier plugins.LicenseVerifier
	instanceService *services.InstanceService
}

// NewPluginHandler creates a new plugin handler
func NewPluginHandler(manager *plugins.Manager, registry *repository.PluginRegistryRepository, auditor *logger.Auditor, disabled bool) *PluginHandler {
	return &PluginHandler{
		manager:         manager,
		registry:        registry,
		auditor:         auditor,
		pluginsDisabled: disabled,
	}
}

// SetLicensing wires license enforcement and instance identity into the
// handler. Both may be nil when licensing is not configured.
func (h *PluginHandler) SetLicensing(verifier plugins.LicenseVerifier, instanceService *services.InstanceService) {
	h.licenseVerifier = verifier
	h.instanceService = instanceService
}

// PluginInfo represents plugin information for API responses
type PluginInfo struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Enabled     bool   `json:"enabled"`
	// Licensed reports whether the plugin's license verified for this install.
	// Always true when license enforcement is not configured.
	Licensed    bool                `json:"licensed"`
	Routes      []map[string]string `json:"routes"`
	Extensions  []plugins.Extension `json:"extensions,omitempty"`
	InstalledAt string              `json:"installed_at"`
}

// ListPlugins returns all installed plugins
func (h *PluginHandler) ListPlugins(w http.ResponseWriter, r *http.Request) {
	if h.pluginsDisabled {
		respondError(w, r, restapi.ErrPluginsDisabled)
		return
	}

	entries, err := h.registry.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	pluginList := make([]PluginInfo, 0, len(entries))
	for _, entry := range entries {
		info := PluginInfo{
			ID:          entry.ID,
			Name:        entry.Name,
			Version:     entry.Version,
			Description: entry.Description,
			Author:      entry.Author,
			Enabled:     entry.Enabled,
			Routes:      entry.Routes,
			InstalledAt: entry.InstalledAt,
		}
		if entry.ExtensionsJSON != "" {
			_ = json.Unmarshal([]byte(entry.ExtensionsJSON), &info.Extensions)
		}
		pluginList = append(pluginList, info)
	}

	// Check for loaded plugins not in database (skip if manager is nil)
	if h.manager != nil {
		for _, loadedPlugin := range h.manager.ListPlugins() {
			found := false
			for i := range pluginList {
				dbPlugin := &pluginList[i]
				if dbPlugin.Name == loadedPlugin.Manifest.Name {
					// The loaded state is the source of truth for licensing.
					dbPlugin.Licensed = loadedPlugin.Licensed
					found = true
					break
				}
			}

			if !found {
				// Add loaded plugin that's not in database
				routes := make([]map[string]string, 0, len(loadedPlugin.Routes))
				for _, r := range loadedPlugin.Routes {
					routes = append(routes, map[string]string{
						"method":      r.Method,
						"path":        r.Path,
						"description": r.Description,
					})
				}

				pluginList = append(pluginList, PluginInfo{
					Name:        loadedPlugin.Manifest.Name,
					Version:     loadedPlugin.Manifest.Version,
					Description: loadedPlugin.Manifest.Description,
					Author:      loadedPlugin.Manifest.Author,
					Enabled:     loadedPlugin.Enabled,
					Licensed:    loadedPlugin.Licensed,
					Routes:      routes,
				})
			}
		}
	}

	respondJSONOK(w, pluginList)
}

// UploadPlugin handles plugin upload
func (h *PluginHandler) UploadPlugin(w http.ResponseWriter, r *http.Request) {
	if h.pluginsDisabled {
		respondError(w, r, restapi.ErrPluginsDisabled)
		return
	}

	// Limit request body size at the HTTP level before parsing
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	// Parse multipart form (32MB max)
	// #nosec G120 -- the body is already capped by MaxBytesReader above; the int arg is the in-memory threshold, not the upper bound
	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		respondBadRequest(w, r, "Failed to parse form")
		return
	}

	// Get the uploaded file
	file, header, err := r.FormFile("plugin")
	if err != nil {
		respondBadRequest(w, r, "Missing plugin file")
		return
	}
	defer func() { _ = file.Close() }()

	// Read file content
	fileData, err := io.ReadAll(file)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Optional license token. When licensing is configured an invalid license
	// is rejected here so the admin gets immediate feedback.
	var license []byte
	if licenseFile, _, err := r.FormFile("license"); err == nil {
		license, err = io.ReadAll(licenseFile)
		_ = licenseFile.Close()
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Check if it's a zip file or direct wasm
	switch {
	case strings.HasSuffix(header.Filename, ".zip"):
		// Handle zip file - new unified approach
		err = h.manager.UploadPluginWithLicense("", fileData, license)
	case strings.HasSuffix(header.Filename, ".wasm"):
		// Handle direct WASM file - need manifest (legacy)
		manifestFile, _, formErr := r.FormFile("manifest")
		if formErr != nil {
			respondBadRequest(w, r, "Missing manifest.json for WASM upload")
			return
		}
		defer func() { _ = manifestFile.Close() }()

		manifestData, readErr := io.ReadAll(manifestFile)
		if readErr != nil {
			respondInternalError(w, r, readErr)
			return
		}

		// Extract plugin name from filename or manifest
		pluginName := strings.TrimSuffix(header.Filename, ".wasm")
		err = h.manager.UploadPluginLegacy(pluginName, fileData, manifestData)
	default:
		respondBadRequest(w, r, "Unsupported file type. Upload .wasm or .zip files")
		return
	}

	if err != nil {
		// License rejections are client errors: surface the reason so the admin
		// can fix the license instead of retrying blindly.
		if errors.Is(err, licensing.ErrLicenseMalformed) ||
			errors.Is(err, licensing.ErrLicenseBadSignature) ||
			errors.Is(err, licensing.ErrLicenseWrongPlugin) ||
			errors.Is(err, licensing.ErrLicenseWrongHost) ||
			errors.Is(err, licensing.ErrLicenseExpired) {
			respondBadRequest(w, r, err.Error())
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Update database registry
	h.syncPluginToDatabase()

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionPluginUpload, logger.ResourcePlugin, nil, header.Filename)
	}
	respondJSONOK(w, map[string]string{"status": "success", "message": "Plugin uploaded successfully"})
}

// GetExtensions returns all extensions from enabled plugins
func (h *PluginHandler) GetExtensions(w http.ResponseWriter, r *http.Request) {
	if h.pluginsDisabled {
		respondError(w, r, restapi.ErrPluginsDisabled)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if h.manager == nil {
		_ = json.NewEncoder(w).Encode(map[string][]plugins.Extension{})
		return
	}

	extensions := h.manager.GetExtensions()
	_ = json.NewEncoder(w).Encode(extensions)
}

// GetAsset serves a static asset from a plugin
func (h *PluginHandler) GetAsset(w http.ResponseWriter, r *http.Request) {
	if h.pluginsDisabled {
		respondError(w, r, restapi.ErrPluginsDisabled)
		return
	}

	if h.manager == nil {
		respondNotFound(w, r, "Plugin system")
		return
	}

	pluginName := r.PathValue("name")
	assetPath := r.PathValue("asset")

	data, mimeType, err := h.manager.GetAsset(pluginName, assetPath)
	if err != nil {
		respondNotFound(w, r, "asset")
		return
	}

	// This route is unauthenticated and the Content-Type is derived from the
	// asset's file extension, so an HTML/JS/SVG asset would otherwise render
	// inline in the app's same-origin context. Mirror the attachment download
	// hardening: always forbid MIME sniffing + framing, and for non-passive
	// (script-capable) types force a sandboxed download instead of inline
	// rendering. Plugins are admin-installed, so this is defense-in-depth.
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	if !isPassivePluginAssetType(mimeType) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		w.Header().Set("Content-Disposition", "attachment")
	}
	_, _ = w.Write(data) //nolint:gosec // G705: static plugin assets served with hardened headers
}

// isPassivePluginAssetType reports whether a plugin asset MIME type is inert
// when served inline (images, fonts, stylesheets, plain media). Anything else —
// notably text/html, SVG, and any */*script* type — is treated as
// script-capable and forced to a sandboxed download.
func isPassivePluginAssetType(mimeType string) bool {
	mt := strings.ToLower(strings.TrimSpace(mimeType))
	if i := strings.IndexByte(mt, ';'); i >= 0 {
		mt = strings.TrimSpace(mt[:i])
	}
	if strings.Contains(mt, "script") || mt == "image/svg+xml" {
		return false
	}
	switch {
	case strings.HasPrefix(mt, "image/"),
		strings.HasPrefix(mt, "font/"),
		strings.HasPrefix(mt, "audio/"),
		strings.HasPrefix(mt, "video/"),
		mt == "text/css",
		mt == "application/font-woff",
		mt == "application/font-woff2":
		return true
	}
	return false
}

// TogglePlugin enables or disables a plugin
func (h *PluginHandler) TogglePlugin(w http.ResponseWriter, r *http.Request) {
	if h.pluginsDisabled {
		respondError(w, r, restapi.ErrPluginsDisabled)
		return
	}

	pluginName := r.PathValue("name")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := newJSONDecoder(w, r).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	var err error
	if req.Enabled {
		err = h.manager.EnablePlugin(pluginName)
	} else {
		err = h.manager.DisablePlugin(pluginName)
	}

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update database
	if err := h.registry.SetEnabled(pluginName, req.Enabled); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		action := logger.ActionPluginDisable
		if req.Enabled {
			action = logger.ActionPluginEnable
		}
		h.auditor.LogWithDetails(r, currentUser, action, logger.ResourcePlugin, nil, pluginName, map[string]any{
			"enabled": req.Enabled,
		})
	}

	respondJSONOK(w, map[string]any{"status": "success", "enabled": req.Enabled})
}

// DeletePlugin removes a plugin
func (h *PluginHandler) DeletePlugin(w http.ResponseWriter, r *http.Request) {
	if h.pluginsDisabled {
		respondError(w, r, restapi.ErrPluginsDisabled)
		return
	}

	pluginName := r.PathValue("name")

	// Delete from manager and filesystem
	if err := h.manager.DeletePlugin(pluginName); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete from database
	if err := h.registry.Delete(pluginName); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionPluginDelete, logger.ResourcePlugin, nil, pluginName)
	}
	respondJSONOK(w, map[string]string{"status": "success", "message": "Plugin deleted successfully"})
}

// ReloadPlugin reloads a plugin
func (h *PluginHandler) ReloadPlugin(w http.ResponseWriter, r *http.Request) {
	if h.pluginsDisabled {
		respondError(w, r, restapi.ErrPluginsDisabled)
		return
	}

	pluginName := r.PathValue("name")

	if err := h.manager.ReloadPlugin(pluginName); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Update database with new metadata
	h.syncPluginToDatabase()

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionPluginReload, logger.ResourcePlugin, nil, pluginName)
	}

	respondJSONOK(w, map[string]string{"status": "success", "message": "Plugin reloaded successfully"})
}

// syncPluginToDatabase syncs loaded plugins with database
func (h *PluginHandler) syncPluginToDatabase() {
	if h.manager == nil {
		return
	}
	for _, p := range h.manager.ListPlugins() {
		// Convert routes to JSON
		routes := make([]map[string]string, 0, len(p.Routes))
		for _, r := range p.Routes {
			routes = append(routes, map[string]string{
				"method":      r.Method,
				"path":        r.Path,
				"description": r.Description,
			})
		}
		extensionsJSON, _ := json.Marshal(p.Manifest.Extensions)
		if err := h.registry.Upsert(repository.PluginRegistryUpsert{
			Name:           p.Manifest.Name,
			Version:        p.Manifest.Version,
			Description:    p.Manifest.Description,
			Author:         p.Manifest.Author,
			Path:           p.Path,
			Routes:         routes,
			ExtensionsJSON: string(extensionsJSON),
			Enabled:        p.Enabled,
		}); err != nil {
			// Log error but continue
			slog.Error("failed to sync plugin to database", slog.String("plugin", p.Manifest.Name), slog.Any("error", err))
		}
	}
}

// GetInstanceID returns this installation's stable identity. Admins paste it
// into the Windshift portal to issue a plugin license bound to this install.
func (h *PluginHandler) GetInstanceID(w http.ResponseWriter, r *http.Request) {
	if h.pluginsDisabled {
		respondError(w, r, restapi.ErrPluginsDisabled)
		return
	}
	if h.instanceService == nil {
		respondInternalError(w, r, errors.New("instance identity not configured"))
		return
	}
	instanceID, err := h.instanceService.GetOrCreate()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, map[string]string{"instance_id": instanceID})
}
