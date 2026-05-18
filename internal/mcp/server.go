// Package mcp provides a Model Context Protocol server for Windshift Core.
// It exposes work management capabilities (items, workspaces, comments,
// labels, time tracking) as MCP tools over Streamable HTTP.
package mcp

import (
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/services"
)

// Deps holds the dependencies needed by the MCP server.
type Deps struct {
	DB                    database.Database
	TokenManager          *auth.TokenManager
	PermissionService     *services.PermissionService
	TimePermissionService *services.TimePermissionService
	TimerService          *services.TimerService
	CommentService        *services.CommentService
	// ActionService is the optional cache-invalidation hook used by the
	// create_action tool. Nil-safe — when unset, newly created actions
	// fire after the next periodic cache refresh instead of immediately.
	ActionService *services.ActionService
}

// MCPServer wraps the MCP SDK server and its HTTP handler.
type MCPServer struct {
	server  *mcp.Server
	handler http.Handler
	deps    Deps
}

// NewMCPServer creates and configures the MCP server with all tools registered.
func NewMCPServer(deps Deps) *MCPServer {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "windshift",
			Version: "1.0.0",
		},
		nil,
	)

	ms := &MCPServer{
		server: server,
		deps:   deps,
	}

	// Register all tools from the shared aitools registry.
	ms.registerAITools()

	// Create the HTTP handler with auth wrapper
	streamHandler := mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless: true,
			Logger:    slog.Default(),
		},
	)

	ms.handler = bearerAuthMiddleware(deps.TokenManager, streamHandler)

	return ms
}

// Handler returns the http.Handler for mounting on a mux.
func (ms *MCPServer) Handler() http.Handler {
	return ms.handler
}
