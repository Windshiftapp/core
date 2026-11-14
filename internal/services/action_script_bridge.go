package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ScriptAPIRequest is the one request shape an execute_script node can make.
// The script supplies method, path, and body only; the bridge supplies the
// credential and every header, so a script cannot inject an Authorization
// header, cookie, forwarding header, alternate host, or redirect target.
type ScriptAPIRequest struct {
	Method string
	Path   string
	Body   []byte
}

// ScriptAPIResponse is the bounded, detached result handed back to the script.
// Truncated is true when the response body exceeded the per-call cap.
type ScriptAPIResponse struct {
	Status    int
	Body      []byte
	Truncated bool
}

// ScriptAPIDispatcher executes one validated request against the canonical v2
// API. Implementations must preserve the bearer auth and scope middleware; the
// in-process implementation does so by invoking the real mounted handler.
type ScriptAPIDispatcher interface {
	Dispatch(ctx context.Context, bearer string, req ScriptAPIRequest) (ScriptAPIResponse, error)
}

// ErrScriptAPIPolicy marks a request the bridge refuses before dispatch —
// alternate hosts, path escapes, disallowed routes, or unsupported methods.
// It is fatal to the node: a script cannot catch it and continue.
type ErrScriptAPIPolicy struct{ Reason string }

func (e *ErrScriptAPIPolicy) Error() string { return "script api policy violation: " + e.Reason }

func scriptAPIPolicyErrorf(format string, args ...any) error {
	return &ErrScriptAPIPolicy{Reason: fmt.Sprintf(format, args...)}
}

// actionScriptAllowedResources is the initial bridge allowlist of v2 top-level
// resources. It deliberately excludes admin, actions, agent-runs, api-tokens,
// oauth, mcp, configuration-sets, screens, workspace-roles, packs, assets, and
// tests: those surfaces can mint credentials, delegate identity, or author
// further automations, which a script must never reach even when the executor
// could. Reads and writes inside the allowlist still require the matching
// token scope and the executor's own resource permissions.
var actionScriptAllowedResources = map[string]bool{
	"items":             true,
	"comments":          true,
	"links":             true,
	"link-types":        true,
	"watchers":          true,
	"milestones":        true,
	"iterations":        true,
	"time":              true,
	"users":             true,
	"workspaces":        true,
	"statuses":          true,
	"status-categories": true,
	"workflows":         true,
	"item-types":        true,
	"priorities":        true,
	"custom-fields":     true,
	"labels":            true,
	"planning":          true,
	"story-points":      true,
	"pages":             true,
	"attachments":       true,
}

// actionScriptAllowedMethods excludes DELETE: destructive writes need explicit
// scopes the action allowlist does not grant, and denying the method here is a
// second, independent guard.
var actionScriptAllowedMethods = map[string]bool{
	http.MethodGet:   true,
	http.MethodHead:  true,
	http.MethodPost:  true,
	http.MethodPut:   true,
	http.MethodPatch: true,
}

// actionScriptDeniedPaths lists v2 routes whose mutation path does not preserve
// the action cascade (bulk item writes emit update events without an
// ActionContext), so they are excluded from the initial bridge even though
// their top-level resource is allowed. Bulk transition is not listed: it flows
// through the same per-item transition path that carries provenance.
var actionScriptDeniedPaths = []string{
	"/items/bulk-update",
	"/items/bulk-patch",
	"/items/bulk-delete",
}

// scriptMutationRule is one audited mutating route. A script may only change
// state through routes whose persistence and emitted action events carry the
// action cascade provenance. Every other mutation is denied: an unaudited
// mutation would emit a fresh user-origin event and reset the chain, depth, and
// private-trigger flag, which is the unbounded-loop vector WI-2174 closes.
//
// "*" matches one non-empty path segment. Reads are not governed by this list.
type scriptMutationRule struct {
	method   string
	segments []string
}

var actionScriptAllowedMutations = []scriptMutationRule{
	{http.MethodPost, []string{"items"}},                    // create item
	{http.MethodPatch, []string{"items", "*"}},              // update item
	{http.MethodPost, []string{"items", "*", "transition"}}, // workflow transition
	{http.MethodPost, []string{"items", "*", "comments"}},   // comment on item
	{http.MethodPost, []string{"items", "bulk-transition"}}, // per-item transition
}

func scriptMutationAllowed(method string, segments []string) bool {
	for _, rule := range actionScriptAllowedMutations {
		if rule.method != method || len(rule.segments) != len(segments) {
			continue
		}
		matched := true
		for i, segment := range rule.segments {
			if segment != "*" && segment != segments[i] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// ValidateScriptAPIRequest enforces the bridge policy on a script-supplied
// method and path and returns the normalized path (leading slash, optional
// query preserved) ready to prefix with the v2 REST root.
func ValidateScriptAPIRequest(method, path string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if !actionScriptAllowedMethods[method] {
		return "", scriptAPIPolicyErrorf("method %q is not permitted", method)
	}
	if path == "" || !strings.HasPrefix(path, "/") {
		return "", scriptAPIPolicyErrorf("path must be a relative v2 path starting with /")
	}
	if strings.ContainsAny(path, "\\\x00\r\n\t") {
		return "", scriptAPIPolicyErrorf("path contains control or backslash characters")
	}
	if strings.Contains(path, "://") {
		return "", scriptAPIPolicyErrorf("absolute URLs are not permitted")
	}
	if strings.HasPrefix(path, "//") {
		return "", scriptAPIPolicyErrorf("protocol-relative paths are not permitted")
	}
	parsed, err := url.Parse(path)
	if err != nil {
		return "", scriptAPIPolicyErrorf("path is not a valid URL reference")
	}
	if parsed.IsAbs() || parsed.Host != "" || parsed.Scheme != "" {
		return "", scriptAPIPolicyErrorf("absolute URLs and alternate hosts are not permitted")
	}
	if parsed.Fragment != "" {
		return "", scriptAPIPolicyErrorf("URL fragments are not permitted")
	}
	if parsed.RawPath != "" && parsed.RawPath != parsed.Path {
		// A distinct escaped path is the classic encoded-traversal vector.
		return "", scriptAPIPolicyErrorf("escaped paths are not permitted")
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) == 0 || segments[0] == "" {
		return "", scriptAPIPolicyErrorf("path must name a v2 resource")
	}
	for _, segment := range segments {
		switch segment {
		case "", ".", "..":
			return "", scriptAPIPolicyErrorf("path traversal is not permitted")
		}
	}
	resource := segments[0]
	if resource == "rest" || resource == "api" {
		return "", scriptAPIPolicyErrorf("path must be relative to the v2 REST root")
	}
	if !actionScriptAllowedResources[resource] {
		return "", scriptAPIPolicyErrorf("resource %q is not available to scripts", resource)
	}
	for _, denied := range actionScriptDeniedPaths {
		if parsed.Path == denied || strings.HasPrefix(parsed.Path, denied+"/") {
			return "", scriptAPIPolicyErrorf("route %q is not available to scripts", parsed.Path)
		}
	}
	// Mutations are restricted to the audited provenance-preserving routes;
	// reads are not, because they cannot reset the cascade.
	if method != http.MethodGet && method != http.MethodHead && !scriptMutationAllowed(method, segments) {
		return "", scriptAPIPolicyErrorf("mutation route %s %q is not available to scripts", method, parsed.Path)
	}
	if parsed.RawQuery != "" {
		return parsed.Path + "?" + parsed.RawQuery, nil
	}
	return parsed.Path, nil
}

// InProcessV2Dispatcher runs script requests through the real mounted v2
// handler, so bearer authentication, scope checks, rate limits, and every
// handler-level authorization decision run exactly as they do for a normal
// API client. It never uses net/http.Client, so redirects are impossible.
type InProcessV2Dispatcher struct {
	handler          http.Handler
	maxResponseBytes int
}

// NewInProcessV2Dispatcher wraps the server mux. maxResponseBytes caps how much
// of a response is buffered; zero uses the default cap.
func NewInProcessV2Dispatcher(handler http.Handler, maxResponseBytes int) *InProcessV2Dispatcher {
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultScriptAPIResponseBytes
	}
	return &InProcessV2Dispatcher{handler: handler, maxResponseBytes: maxResponseBytes}
}

// Dispatch invokes the mounted handler for /rest/api/v2<path>. The request
// carries only the minted bearer token; the script cannot influence headers.
func (d *InProcessV2Dispatcher) Dispatch(ctx context.Context, bearer string, req ScriptAPIRequest) (ScriptAPIResponse, error) {
	if d == nil || d.handler == nil {
		return ScriptAPIResponse{}, errors.New("script api dispatcher is not configured")
	}
	normalized, err := ValidateScriptAPIRequest(req.Method, req.Path)
	if err != nil {
		return ScriptAPIResponse{}, err
	}
	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(req.Method), "http://action-script.internal/rest/api/v2"+normalized, body)
	if err != nil {
		return ScriptAPIResponse{}, fmt.Errorf("build script api request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+bearer)
	httpReq.Header.Set("Accept", "application/json")
	if len(req.Body) > 0 {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	recorder := newBoundedRecorder(d.maxResponseBytes)
	d.handler.ServeHTTP(recorder, httpReq)
	status := recorder.status
	if status == 0 {
		status = http.StatusOK
	}
	return ScriptAPIResponse{Status: status, Body: recorder.body.Bytes(), Truncated: recorder.truncated}, nil
}

// boundedRecorder captures at most limit bytes of a response and reports
// truncation instead of buffering an unbounded body into memory.
type boundedRecorder struct {
	header    http.Header
	status    int
	body      bytes.Buffer
	limit     int
	truncated bool
}

func newBoundedRecorder(limit int) *boundedRecorder {
	return &boundedRecorder{header: make(http.Header), limit: limit}
}

func (r *boundedRecorder) Header() http.Header { return r.header }

func (r *boundedRecorder) WriteHeader(statusCode int) {
	if r.status == 0 {
		r.status = statusCode
	}
}

func (r *boundedRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	remaining := r.limit - r.body.Len()
	if remaining <= 0 {
		r.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = r.body.Write(p[:remaining])
		r.truncated = true
		return len(p), nil
	}
	_, _ = r.body.Write(p)
	return len(p), nil
}

// Flush satisfies http.Flusher for handlers that stream; the bounded buffer
// makes it a no-op.
func (r *boundedRecorder) Flush() {}
