package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"windshift/internal/database"
	"windshift/internal/plugins"
	"windshift/internal/restapi"
)

// PluginHandler handles plugin-related HTTP requests
type PluginHandler struct {
	db              database.Database
	manager         *plugins.Manager
	pluginsDisabled bool
}

// NewPluginHandler creates a new plugin handler
func NewPluginHandler(db database.Database, manager *plugins.Manager, disabled bool) *PluginHandler {
	return &PluginHandler{
		db:              db,
		manager:         manager,
		pluginsDisabled: disabled,
	}
}

// PluginInfo represents plugin information for API responses
type PluginInfo struct {
	ID          int                 `json:"id"`
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Description string              `json:"description"`
	Author      string              `json:"author"`
	Enabled     bool                `json:"enabled"`
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

	// Get plugins from database
	rows, err := h.db.Query(`
		SELECT id, name, version, description, author, enabled, routes, extensions, installed_at
		FROM plugin_registry
		ORDER BY name
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var pluginList []PluginInfo
	for rows.Next() {
		var p PluginInfo
		var routesJSON sql.NullString
		var extensionsJSON sql.NullString

		err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Description, &p.Author, &p.Enabled, &routesJSON, &extensionsJSON, &p.InstalledAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Parse routes JSON
		if routesJSON.Valid && routesJSON.String != "" {
			var routes []map[string]string
			if err := json.Unmarshal([]byte(routesJSON.String), &routes); err == nil {
				p.Routes = routes
			}
		}

		// Parse extensions JSON
		if extensionsJSON.Valid && extensionsJSON.String != "" {
			var extensions []plugins.Extension
			if err := json.Unmarshal([]byte(extensionsJSON.String), &extensions); err == nil {
				p.Extensions = extensions
			}
		}

		pluginList = append(pluginList, p)
	}

	// Check for loaded plugins not in database (skip if manager is nil)
	if h.manager != nil {
		for _, loadedPlugin := range h.manager.ListPlugins() {
			found := false
			for _, dbPlugin := range pluginList {
				if dbPlugin.Name == loadedPlugin.Manifest.Name {
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
					Routes:      routes,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pluginList)
}

// UploadPlugin handles plugin upload
func (h *PluginHandler) UploadPlugin(w http.ResponseWriter, r *http.Request) {
	if h.pluginsDisabled {
		respondError(w, r, restapi.ErrPluginsDisabled)
		return
	}

	// Parse multipart form (32MB max)
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

	// Check if it's a zip file or direct wasm
	switch {
	case strings.HasSuffix(header.Filename, ".zip"):
		// Handle zip file - new unified approach
		err = h.manager.UploadPlugin("", fileData)
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
		respondInternalError(w, r, err)
		return
	}

	// Update database registry
	h.syncPluginToDatabase()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Plugin uploaded successfully"})
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

	w.Header().Set("Content-Type", mimeType)
	// Enable CORS for plugin assets
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = w.Write(data)
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
	_, err = h.db.ExecWrite("UPDATE plugin_registry SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?", req.Enabled, pluginName)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "success", "enabled": req.Enabled})
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
	_, err := h.db.ExecWrite("DELETE FROM plugin_registry WHERE name = ?", pluginName)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Plugin deleted successfully"})
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Plugin reloaded successfully"})
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
		routesJSON, _ := json.Marshal(routes)

		// Convert extensions to JSON
		extensionsJSON, _ := json.Marshal(p.Manifest.Extensions)

		// Upsert plugin record
		_, err := h.db.ExecWrite(`
			INSERT INTO plugin_registry (name, version, description, author, path, routes, extensions, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET
				version = excluded.version,
				description = excluded.description,
				author = excluded.author,
				path = excluded.path,
				routes = excluded.routes,
				extensions = excluded.extensions,
				enabled = excluded.enabled,
				updated_at = CURRENT_TIMESTAMP
		`, p.Manifest.Name, p.Manifest.Version, p.Manifest.Description,
			p.Manifest.Author, p.Path, string(routesJSON), string(extensionsJSON), p.Enabled)

		if err != nil {
			// Log error but continue
			slog.Error("failed to sync plugin to database", slog.String("plugin", p.Manifest.Name), slog.Any("error", err))
		}
	}
}
