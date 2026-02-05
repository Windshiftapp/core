// Package plugins provides WebAssembly plugin support for extending Windshift functionality.
// It includes types for plugin metadata, routes, and extensions, as well as the CLI host
// implementation for running plugins in a sandboxed environment.
package plugins

// PluginMetadata describes plugin-provided metadata returned from exports like get_metadata/get_routes.
type PluginMetadata struct {
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	Description  string      `json:"description"`
	Author       string      `json:"author"`
	Capabilities []string    `json:"capabilities,omitempty"`
	Routes       []Route     `json:"routes,omitempty"`
	Extensions   []Extension `json:"extensions,omitempty"`
}

// Route describes an HTTP route the plugin handles.
type Route struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

// Extension represents a UI extension point provided by a plugin.
type Extension struct {
	Point       string `json:"point"`            // Extension point type (e.g., "admin.tab")
	ID          string `json:"id"`               // Unique identifier for this extension
	Label       string `json:"label"`            // Display label
	Description string `json:"description"`      // Description of what this extension does
	Icon        string `json:"icon,omitempty"`   // Icon name (Lucide icon)
	Component   string `json:"component"`        // Path to the frontend component (e.g., "frontend.js")
	Styles      string `json:"styles,omitempty"` // Optional path to CSS file
	Group       string `json:"group,omitempty"`  // Grouping for organization (e.g., "Security & Audit")
	Order       int    `json:"order,omitempty"`  // Display order (higher = later)
	PluginName  string `json:"pluginName"`       // Name of the plugin this extension belongs to
}

// HTTPRequest represents an incoming HTTP request forwarded to a plugin.
type HTTPRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Query   map[string]string `json:"query"`
	Params  map[string]string `json:"params"`
}

// HTTPResponse represents the plugin's HTTP response.
type HTTPResponse struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
}

// LogRequest is the payload for the host log function.
type LogRequest struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

// SMTPSendRequest is the payload for the smtp_send host function.
type SMTPSendRequest struct {
	To      []string          `json:"to"`
	Cc      []string          `json:"cc,omitempty"`
	Bcc     []string          `json:"bcc,omitempty"`
	Subject string            `json:"subject"`
	Text    string            `json:"text,omitempty"`
	HTML    string            `json:"html,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// SMTPSendResponse is returned from smtp_send host function.
type SMTPSendResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// HTTPFetchRequest is the payload for the http_fetch host function.
type HTTPFetchRequest struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      []byte            `json:"body,omitempty"`
	TimeoutMs int               `json:"timeout_ms,omitempty"`
}

// HTTPFetchResponse is returned from http_fetch host function.
type HTTPFetchResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    []byte            `json:"body,omitempty"`
}

// PluginManifest represents the manifest.json file authored by plugin developers.
type PluginManifest struct {
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Description  string          `json:"description"`
	Author       string          `json:"author"`
	EntryPoint   string          `json:"entryPoint"`
	Capabilities []string        `json:"capabilities,omitempty"`
	Extensions   []Extension     `json:"extensions,omitempty"` // UI extensions provided by this plugin
	Routes       []Route         `json:"routes,omitempty"`     // Inline route metadata
	Webhooks     []PluginWebhook `json:"webhooks,omitempty"`   // Webhooks the plugin wants to receive
}

// PluginWebhook represents a webhook registration from a plugin.
type PluginWebhook struct {
	ID      string   `json:"id"`      // Unique identifier within the plugin
	Events  []string `json:"events"`  // Events to subscribe to (item.created, item.updated, etc.)
	Handler string   `json:"handler"` // Name of the WASM function to call
}

// CLIExecRequest is the payload for the cli_exec host function.
type CLIExecRequest struct {
	Command    string            `json:"command"`
	Args       []string          `json:"args,omitempty"`
	WorkingDir string            `json:"working_dir,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	TimeoutMs  int               `json:"timeout_ms,omitempty"` // Default 30000 (30s)
}

// CLIExecResponse is returned from the cli_exec host function.
type CLIExecResponse struct {
	Status   string `json:"status"`          // "ok" or "error"
	ExitCode int    `json:"exit_code"`       // Process exit code (0 = success)
	Stdout   string `json:"stdout"`          // Captured stdout
	Stderr   string `json:"stderr"`          // Captured stderr
	Error    string `json:"error,omitempty"` // Error message if status is "error"
}

// KVGetRequest is the payload for the kv_get host function.
type KVGetRequest struct {
	Key string `json:"key"`
}

// KVGetResponse is returned from the kv_get host function.
type KVGetResponse struct {
	Status string `json:"status"`          // "ok" or "not_found"
	Value  string `json:"value,omitempty"` // Value if found
	Error  string `json:"error,omitempty"` // Error message if status is "error"
}

// KVSetRequest is the payload for the kv_set host function.
type KVSetRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// KVSetResponse is returned from the kv_set host function.
type KVSetResponse struct {
	Status string `json:"status"`          // "ok" or "error"
	Error  string `json:"error,omitempty"` // Error message if failed
}

// KVDeleteRequest is the payload for the kv_delete host function.
type KVDeleteRequest struct {
	Key string `json:"key"`
}

// KVDeleteResponse is returned from the kv_delete host function.
type KVDeleteResponse struct {
	Status string `json:"status"`          // "ok" or "error"
	Error  string `json:"error,omitempty"` // Error message if failed
}

// CreateCommentRequest is the payload for the create_comment host function.
type CreateCommentRequest struct {
	ItemID   int    `json:"item_id"`
	AuthorID int    `json:"author_id"`
	Content  string `json:"content"` // Plain text content (will be converted to TipTap JSON)
}

// CreateCommentResponse is returned from the create_comment host function.
type CreateCommentResponse struct {
	Status    string `json:"status"`               // "ok" or "error"
	CommentID int    `json:"comment_id,omitempty"` // ID of created comment
	Error     string `json:"error,omitempty"`      // Error message if failed
}

// SCMCreateBranchRequest is the payload for the scm_create_branch host function.
type SCMCreateBranchRequest struct {
	WorkspaceRepositoryID int    `json:"workspace_repository_id"` // ID of the workspace_repositories entry
	BranchName            string `json:"branch_name"`             // Name of the branch to create
	BaseBranch            string `json:"base_branch,omitempty"`   // Base branch (defaults to repo's default branch)
}

// SCMCreateBranchResponse is returned from the scm_create_branch host function.
type SCMCreateBranchResponse struct {
	Status    string `json:"status"`               // "ok" or "error"
	BranchURL string `json:"branch_url,omitempty"` // URL to the created branch
	Error     string `json:"error,omitempty"`      // Error message if failed
}

// SCMCreateItemLinkRequest is the payload for the scm_create_item_link host function.
type SCMCreateItemLinkRequest struct {
	ItemID                int    `json:"item_id"`                 // ID of the item to link
	WorkspaceRepositoryID int    `json:"workspace_repository_id"` // ID of the workspace_repositories entry
	LinkType              string `json:"link_type"`               // "branch", "pull_request", or "commit"
	ExternalID            string `json:"external_id"`             // Branch name, PR number, or commit SHA
	ExternalURL           string `json:"external_url,omitempty"`  // URL to the external resource
	Title                 string `json:"title,omitempty"`         // Optional title/description
}

// SCMCreateItemLinkResponse is returned from the scm_create_item_link host function.
type SCMCreateItemLinkResponse struct {
	Status string `json:"status"`            // "ok" or "error"
	LinkID int    `json:"link_id,omitempty"` // ID of the created link
	Error  string `json:"error,omitempty"`   // Error message if failed
}
