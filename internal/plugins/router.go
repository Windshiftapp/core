package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// Router handles HTTP routing for plugins
type Router struct {
	manager *Manager
}

// NewRouter creates a new plugin router
func NewRouter(manager *Manager) *Router {
	return &Router{
		manager: manager,
	}
}

// RegisterRoutes registers plugin routes with the main ServeMux
// Uses catch-all pattern {path...} for plugin path matching
func (r *Router) RegisterRoutes(mux *http.ServeMux) {
	// Register catch-all routes for each HTTP method
	// Pattern: /api/plugins/{plugin}/{path...} captures the rest of the path
	mux.HandleFunc("GET /api/plugins/{plugin}/{path...}", r.HandlePluginRequest)
	mux.HandleFunc("POST /api/plugins/{plugin}/{path...}", r.HandlePluginRequest)
	mux.HandleFunc("PUT /api/plugins/{plugin}/{path...}", r.HandlePluginRequest)
	mux.HandleFunc("DELETE /api/plugins/{plugin}/{path...}", r.HandlePluginRequest)
	mux.HandleFunc("PATCH /api/plugins/{plugin}/{path...}", r.HandlePluginRequest)
	mux.HandleFunc("OPTIONS /api/plugins/{plugin}/{path...}", r.HandlePluginRequest)
}

// HandlePluginRequest handles incoming requests for plugins
func (r *Router) HandlePluginRequest(w http.ResponseWriter, req *http.Request) {
	pluginName := req.PathValue("plugin")

	// Extract the plugin path from the catch-all or trailing slash
	pluginPath := "/" + req.PathValue("path")
	if pluginPath == "/" {
		pluginPath = "/"
	}

	// Get the plugin
	plugin, exists := r.manager.GetPlugin(pluginName)
	if !exists {
		http.Error(w, fmt.Sprintf("Plugin not found: %s", pluginName), http.StatusNotFound)
		return
	}

	if !plugin.Enabled {
		http.Error(w, fmt.Sprintf("Plugin is disabled: %s", pluginName), http.StatusForbidden)
		return
	}

	// Check if the route is registered
	routeFound := false
	for _, route := range plugin.Routes {
		if matchRoute(route, req.Method, pluginPath) {
			routeFound = true
			break
		}
	}

	if !routeFound {
		http.Error(w, fmt.Sprintf("Route not found in plugin: %s %s", req.Method, pluginPath), http.StatusNotFound)
		return
	}

	// Read request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Convert headers
	headers := make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Convert query parameters
	query := make(map[string]string)
	for key, values := range req.URL.Query() {
		if len(values) > 0 {
			query[key] = values[0]
		}
	}

	// Create plugin request
	pluginReq := &HTTPRequest{
		Method:  req.Method,
		Path:    pluginPath,
		Headers: headers,
		Body:    string(body),
		Query:   query,
		Params:  map[string]string{"plugin": pluginName},
	}

	// Forward to plugin
	pluginResp, err := r.manager.HandleRequest(pluginName, pluginReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Plugin error: %v", err), http.StatusInternalServerError)
		return
	}

	// Write response headers
	for key, value := range pluginResp.Headers {
		w.Header().Set(key, value)
	}

	// Set default content type if not provided
	if w.Header().Get("Content-Type") == "" {
		// Try to detect JSON
		var js json.RawMessage
		if err := json.Unmarshal([]byte(pluginResp.Body), &js); err == nil {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/plain")
		}
	}

	// Write status code
	if pluginResp.StatusCode != 0 {
		w.WriteHeader(pluginResp.StatusCode)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	// Write response body
	if _, err := w.Write([]byte(pluginResp.Body)); err != nil { //nolint:gosec // G705: plugin responses from trusted/verified plugin code
		// Log error but response is already partially written
		slog.Error("failed to write plugin response", slog.Any("error", err))
	}
}

// matchRoute checks if a route matches the request
func matchRoute(route Route, method, path string) bool {
	// Check method
	if route.Method != "" && route.Method != method {
		return false
	}

	// Simple path matching (could be enhanced with pattern matching)
	// For now, exact match or prefix match with trailing slash
	if route.Path == path {
		return true
	}

	// Check if route path ends with * for wildcard matching
	if strings.HasSuffix(route.Path, "*") {
		prefix := strings.TrimSuffix(route.Path, "*")
		return strings.HasPrefix(path, prefix)
	}

	return false
}

// GetPluginRoutes returns all registered plugin routes
func (r *Router) GetPluginRoutes() map[string][]Route {
	routes := make(map[string][]Route)

	for _, plugin := range r.manager.ListPlugins() {
		if plugin.Enabled {
			routes[plugin.Manifest.Name] = plugin.Routes
		}
	}

	return routes
}
