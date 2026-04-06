package mcp

import (
	"context"
	"database/sql"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"windshift/internal/models"
	"windshift/internal/repository"
)

func (ms *MCPServer) registerWorkspaceTools() {
	type listWorkspacesInput struct{}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "list_workspaces",
		Description: "List all workspaces the authenticated user has access to.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listWorkspacesInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		wsIDs, err := ms.accessibleWorkspaceIDs(user.ID)
		if err != nil {
			return errInternal("list workspaces", err)
		}

		if len(wsIDs) == 0 {
			return toolJSON(map[string]any{"workspaces": []any{}})
		}

		repo := repository.NewWorkspaceRepository(ms.deps.DB)
		var workspaces []map[string]any
		for _, id := range wsIDs {
			ws, err := repo.FindByID(id)
			if err != nil {
				continue
			}
			workspaces = append(workspaces, mapWorkspace(ws))
		}

		return toolJSON(map[string]any{"workspaces": workspaces})
	})

	type getWorkspaceInput struct {
		WorkspaceID int `json:"workspace_id" jsonschema:"Workspace ID to retrieve"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "get_workspace",
		Description: "Get detailed information about a specific workspace.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getWorkspaceInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		if ok, _ := ms.canViewItem(user.ID, args.WorkspaceID); !ok {
			return toolErrorResult("workspace not found")
		}

		repo := repository.NewWorkspaceRepository(ms.deps.DB)
		ws, err := repo.FindByID(args.WorkspaceID)
		if err != nil {
			if err == sql.ErrNoRows {
				return toolErrorResult("workspace not found")
			}
			return errInternal("get workspace", err)
		}

		return toolJSON(mapWorkspace(ws))
	})
}

func mapWorkspace(ws *models.Workspace) map[string]any {
	m := map[string]any{
		"id":          ws.ID,
		"name":        ws.Name,
		"key":         ws.Key,
		"active":      ws.Active,
		"is_personal": ws.IsPersonal,
	}
	if ws.Description != "" {
		m["description"] = ws.Description
	}
	if ws.Icon != "" {
		m["icon"] = ws.Icon
	}
	if ws.Color != "" {
		m["color"] = ws.Color
	}
	if ws.OwnerID != nil {
		m["owner_id"] = *ws.OwnerID
	}
	if ws.TimeProjectID != nil {
		m["time_project_id"] = *ws.TimeProjectID
	}
	return m
}

// toolErrorResult wraps a tool error for readability.
func toolErrorResult(msg string) (*mcp.CallToolResult, any, error) {
	return toolError(msg)
}
