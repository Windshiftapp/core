package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"windshift/internal/aitools"
	"windshift/internal/models"
	"windshift/internal/services"
)

// registerAITools uses raw AddTool so MCP and aitools share registry-generated
// JSON Schema bytes; handlers unmarshal registry-validated arguments directly.
func (ms *MCPServer) registerAITools() {
	for _, e := range aitools.Default.All() {
		if !e.AvailableFor(aitools.SourceMCP) {
			continue
		}
		entry := e // capture per iteration
		ms.server.AddTool(&mcp.Tool{
			Name:        entry.Name,
			Description: entry.Description,
			InputSchema: entry.Schema,
			Annotations: toolAnnotations(entry),
			Meta: mcp.Meta{
				"required_scopes": append([]string(nil), entry.Scopes...),
			},
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			user := userFromContext(ctx)
			if user == nil {
				return errNoAuth(), nil
			}
			// mcp:access opens MCP only; declared scopes still gate each tool.
			// In-product cookie-session chat does not use this token path.
			if res, ok := ms.checkToolScopes(ctx, entry); !ok {
				return res, nil
			}
			env, err := ms.buildEnv(user)
			if err != nil {
				return errInternal("build env", err), nil
			}
			parsed := entry.NewArgs()
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, parsed); err != nil {
					return toolErrorf("invalid arguments: %v", err), nil
				}
			}
			out, err := entry.Run(ctx, env, parsed)
			if err != nil {
				return errInternal(entry.Name, err), nil
			}
			b, err := json.Marshal(out)
			if err != nil {
				return toolErrorf("marshal result: %v", err), nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
			}, nil
		})
	}
}

func toolAnnotations(entry aitools.Entry) *mcp.ToolAnnotations {
	readOnly := true
	destructive := false
	for _, scope := range entry.Scopes {
		if !strings.HasSuffix(scope, ":read") {
			readOnly = false
		}
		if strings.HasSuffix(scope, ":delete") {
			destructive = true
		}
	}
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    readOnly,
		DestructiveHint: &destructive,
	}
}

// checkToolScopes verifies the request's validated API token carries every
// scope the tool declared (aitools registrations mirror the v1 router's
// per-route gating; write scopes imply the matching read scope). Returns a
// tool-error result naming the missing scopes when the check fails. Fails
// closed when no token is in context — only bearerAuthMiddleware puts one
// there, so a missing token means the request bypassed token auth somehow.
func (ms *MCPServer) checkToolScopes(ctx context.Context, entry aitools.Entry) (*mcp.CallToolResult, bool) {
	token := apiTokenFromContext(ctx)
	if token == nil {
		return errNoAuth(), false
	}
	var missing []string
	for _, scope := range entry.Scopes {
		if !ms.deps.TokenManager.CheckTokenPermissions(token, []string{scope}) {
			missing = append(missing, scope)
		}
	}
	if len(missing) > 0 {
		return toolErrorf("token missing required scope: %s", strings.Join(missing, ", ")), false
	}
	return nil, true
}

// buildEnv constructs an aitools.Env scoped to the calling user. Permissions
// are resolved fresh on each call (no per-session caching) — fine because
// MCP requests are usually one-shot.
func (ms *MCPServer) buildEnv(user *models.User) (*aitools.Env, error) {
	wsIDs, err := ms.accessibleWorkspaceIDs(user.ID)
	if err != nil {
		return nil, err
	}
	timezone, err := services.LookupUserTimezone(ms.deps.DB, user.ID)
	if err != nil {
		return nil, err
	}
	return &aitools.Env{
		DB:                     ms.deps.DB,
		UserID:                 user.ID,
		Username:               user.FullName,
		Timezone:               timezone,
		Source:                 aitools.SourceMCP,
		AccessibleWorkspaceIDs: wsIDs,
		PermService:            ms.deps.PermissionService,
		TimePermService:        ms.deps.TimePermissionService,
		TimerService:           ms.deps.TimerService,
		CommentService:         ms.deps.CommentService,
		ItemDeletionService:    ms.deps.ItemDeletionService,
		PageApplicationService: ms.deps.PageApplicationService,
		PageDiagramService:     ms.deps.PageDiagramService,
		ActionService:          ms.deps.ActionService,
	}, nil
}
