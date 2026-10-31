package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"windshift/internal/contextkeys"
	"windshift/internal/utils"
)

// maxPluginRequestBodyBytes caps the body the host will read for a plugin
// request. Plugin routes accept arbitrary content types, so the JSON body
// limiter used by the /api group does not apply.
const maxPluginRequestBodyBytes = 2 << 20 // 2 MiB

// CSRFValidator reports whether a request's Origin/Referer is allowed. The
// server passes the same validator the /api chain uses, or an always-allow
// validator when CSRF protection is disabled.
type CSRFValidator interface {
	Allows(*http.Request) bool
}

// PrincipalSource resolves the caller's admin flag and group memberships for
// the Principal forwarded to a plugin.
type PrincipalSource interface {
	IsSystemAdmin(userID int) (bool, error)
	GetGroupMemberships(userID int) ([]int, error)
}

// RouterDeps are the host-side authorization hooks the router applies per
// route. A nil hook denies, so a misconfigured router fails closed.
type RouterDeps struct {
	RequireAuth  func(http.Handler) http.Handler
	RequireAdmin func(http.Handler) http.Handler
	CSRF         CSRFValidator
	PublicLimit  func(http.Handler) http.Handler
	Permissions  PrincipalSource
	Logger       *slog.Logger
}

// PluginSource is the subset of the manager the router needs. The manager
// satisfies it; tests inject a fake so route authorization can be exercised
// without a WASM handler.
type PluginSource interface {
	GetPlugin(name string) (*LoadedPlugin, bool)
	HandleRequest(pluginName string, req *HTTPRequest) (*HTTPResponse, error)
}

// Router handles HTTP routing for plugins
type Router struct {
	plugins PluginSource
	deps    RouterDeps
}

// NewRouter creates a new plugin router
func NewRouter(plugins PluginSource, deps RouterDeps) *Router {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &Router{
		plugins: plugins,
		deps:    deps,
	}
}

// RegisterRoutes registers plugin routes with the main ServeMux
// Uses catch-all pattern {path...} for plugin path matching
func (r *Router) RegisterRoutes(mux *http.ServeMux) {
	// Match every method while preserving the plugin name and trailing path.
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

	pluginPath := "/" + req.PathValue("path")
	if pluginPath == "/" {
		pluginPath = "/"
	}

	plugin, exists := r.plugins.GetPlugin(pluginName)
	if !exists {
		http.Error(w, fmt.Sprintf("Plugin not found: %s", pluginName), http.StatusNotFound)
		return
	}

	if !plugin.Enabled {
		http.Error(w, fmt.Sprintf("Plugin is disabled: %s", pluginName), http.StatusForbidden)
		return
	}

	route, found := findRoute(plugin.Routes, req.Method, pluginPath)
	if !found {
		http.Error(w, fmt.Sprintf("Route not found in plugin: %s %s", req.Method, pluginPath), http.StatusNotFound)
		return
	}

	// Cap the body before any handler reads it. Plugin routes accept arbitrary
	// content types, so this replaces the JSON-only limiter used elsewhere.
	req.Body = http.MaxBytesReader(w, req.Body, maxPluginRequestBodyBytes)

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.forward(w, req, pluginName, pluginPath, route.Auth)
	}))

	switch route.Auth {
	case RouteAuthNone:
		// Public routes are explicit opt-ins: never forward credentials, and
		// mark the request CSRF-exempt (there is no session to protect).
		req.Header.Del("Cookie")
		req.Header.Del("Authorization")
		req = req.WithContext(context.WithValue(req.Context(), contextkeys.CSRFExempt, true))
		if r.deps.PublicLimit != nil {
			handler = r.deps.PublicLimit(handler)
		}
	case RouteAuthAdmin:
		if r.deps.RequireAuth == nil || r.deps.RequireAdmin == nil {
			http.Error(w, "Plugin route authorization is not configured", http.StatusForbidden)
			return
		}
		handler = r.deps.RequireAuth(r.deps.RequireAdmin(handler))
	case RouteAuthSession:
		if r.deps.RequireAuth == nil {
			http.Error(w, "Plugin route authorization is not configured", http.StatusForbidden)
			return
		}
		handler = r.deps.RequireAuth(r.csrfGuard(handler))
	default:
		// Normalized at load time; fail closed if that invariant is ever broken.
		http.Error(w, "Plugin route authorization is not configured", http.StatusForbidden)
		return
	}

	handler.ServeHTTP(w, req)
}

// csrfGuard rejects a state-changing session request whose Origin/Referer is
// not allowed. A nil validator denies.
func (r *Router) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if r.deps.CSRF == nil || !r.deps.CSRF.Allows(req) {
			http.Error(w, "CSRF validation failed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, req)
	})
}

// forward adapts the authorized request to the plugin protocol.
func (r *Router) forward(w http.ResponseWriter, req *http.Request, pluginName, pluginPath string, auth RouteAuth) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	headers := make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	query := make(map[string]string)
	for key, values := range req.URL.Query() {
		if len(values) > 0 {
			query[key] = values[0]
		}
	}

	pluginReq := &HTTPRequest{
		Method:    req.Method,
		Path:      pluginPath,
		Headers:   headers,
		Body:      string(body),
		Query:     query,
		Params:    map[string]string{"plugin": pluginName},
		Principal: r.principal(req, auth),
	}

	pluginResp, err := r.plugins.HandleRequest(pluginName, pluginReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Plugin error: %v", err), http.StatusInternalServerError)
		return
	}

	for key, value := range pluginResp.Headers {
		w.Header().Set(key, value)
	}

	if w.Header().Get("Content-Type") == "" {
		// Infer a default only when the plugin did not provide one.
		var js json.RawMessage
		if err := json.Unmarshal([]byte(pluginResp.Body), &js); err == nil {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/plain")
		}
	}

	if pluginResp.StatusCode != 0 {
		w.WriteHeader(pluginResp.StatusCode)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	if _, err := w.Write([]byte(pluginResp.Body)); err != nil { //nolint:gosec // G705: plugin responses from trusted/verified plugin code
		// Log error but response is already partially written
		r.deps.Logger.Error("failed to write plugin response", slog.Any("error", err))
	}
}

// principal builds the authoritative caller identity for a private route. It is
// nil on public routes and when no user is in context.
func (r *Router) principal(req *http.Request, auth RouteAuth) *Principal {
	if auth == RouteAuthNone {
		return nil
	}
	user := utils.GetCurrentUser(req)
	if user == nil {
		return nil
	}
	principal := &Principal{UserID: user.ID, GroupIDs: []int{}}
	if r.deps.Permissions == nil {
		return principal
	}
	if isAdmin, err := r.deps.Permissions.IsSystemAdmin(user.ID); err != nil {
		r.deps.Logger.Error("failed to resolve plugin principal admin flag", "user_id", user.ID, "error", err)
	} else {
		principal.IsAdmin = isAdmin
	}
	if groups, err := r.deps.Permissions.GetGroupMemberships(user.ID); err != nil {
		r.deps.Logger.Error("failed to resolve plugin principal groups", "user_id", user.ID, "error", err)
	} else {
		principal.GroupIDs = groups
	}
	return principal
}

// findRoute selects the most specific route that matches. An exact path beats a
// trailing-star prefix, and a longer prefix beats a shorter one, so the
// credential of the narrowest matching route applies.
func findRoute(routes []Route, method, path string) (Route, bool) {
	var best Route
	found := false
	bestSpecificity := -1
	for _, route := range routes {
		if !matchRoute(route, method, path) {
			continue
		}
		specificity := routeSpecificity(route)
		if !found || specificity > bestSpecificity {
			best = route
			bestSpecificity = specificity
			found = true
		}
	}
	return best, found
}

func routeSpecificity(route Route) int {
	if !strings.HasSuffix(route.Path, "*") {
		return 1 << 30
	}
	return len(strings.TrimSuffix(route.Path, "*"))
}

// matchRoute checks if a route matches the request
func matchRoute(route Route, method, path string) bool {
	if route.Method != "" && route.Method != method {
		return false
	}

	if route.Path == path {
		return true
	}

	if strings.HasSuffix(route.Path, "*") {
		// A trailing star matches the route prefix.
		prefix := strings.TrimSuffix(route.Path, "*")
		return strings.HasPrefix(path, prefix)
	}

	return false
}
