package mcp

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"windshift/internal/aitools"
	"windshift/internal/models"
)

// registerAITools registers every tool from the shared aitools registry
// with the MCP server. We use the SDK's raw AddTool path (Server.AddTool,
// not the generic mcp.AddTool) so we can pass the JSON Schema we already
// computed in the registry instead of having the SDK derive it from a
// typed In parameter. This keeps the schema source of truth on the
// aitools side: both surfaces see exactly the same schema bytes.
//
// The trade-off is that we do unmarshalling and validation ourselves;
// good enough for now since the registry produces well-formed schemas.
func (ms *MCPServer) registerAITools() {
	for _, e := range aitools.Default.All() {
		entry := e // capture per iteration
		ms.server.AddTool(&mcp.Tool{
			Name:        entry.Name,
			Description: entry.Description,
			InputSchema: entry.Schema,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			user := userFromContext(ctx)
			if user == nil {
				res, _, _ := errNoAuth()
				return res, nil
			}
			env, err := ms.buildEnv(user)
			if err != nil {
				res, _, _ := errInternal("build env", err)
				return res, nil
			}
			parsed := entry.NewArgs()
			if len(req.Params.Arguments) > 0 {
				if err := json.Unmarshal(req.Params.Arguments, parsed); err != nil {
					res, _, _ := toolErrorf("invalid arguments: %v", err)
					return res, nil
				}
			}
			out, err := entry.Run(ctx, env, parsed)
			if err != nil {
				res, _, _ := errInternal(entry.Name, err)
				return res, nil
			}
			b, err := json.Marshal(out)
			if err != nil {
				res, _, _ := toolErrorf("marshal result: %v", err)
				return res, nil
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: string(b)}},
			}, nil
		})
	}
}

// buildEnv constructs an aitools.Env scoped to the calling user. Permissions
// are resolved fresh on each call (no per-session caching) — fine because
// MCP requests are usually one-shot.
func (ms *MCPServer) buildEnv(user *models.User) (*aitools.Env, error) {
	wsIDs, err := ms.accessibleWorkspaceIDs(user.ID)
	if err != nil {
		return nil, err
	}
	return &aitools.Env{
		DB:                     ms.deps.DB,
		UserID:                 user.ID,
		Username:               user.FullName,
		Source:                 aitools.SourceMCP,
		AccessibleWorkspaceIDs: wsIDs,
		PermService:            ms.deps.PermissionService,
		TimePermService:        ms.deps.TimePermissionService,
		TimerService:           ms.deps.TimerService,
		CommentService:         ms.deps.CommentService,
		ActionService:          ms.deps.ActionService,
	}, nil
}
