package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"windshift/internal/services"
)

func (ms *MCPServer) registerLabelTools() {
	type listLabelsInput struct {
		WorkspaceID int `json:"workspace_id" jsonschema:"Workspace ID to list labels for"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "list_labels",
		Description: "List all labels in a workspace.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listLabelsInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		if ok, _ := ms.canViewItem(user.ID, args.WorkspaceID); !ok {
			return toolErrorResult("workspace not found")
		}

		rows, err := ms.deps.DB.Query(
			"SELECT id, name, color, workspace_id FROM labels WHERE workspace_id = ? ORDER BY name",
			args.WorkspaceID,
		)
		if err != nil {
			return errInternal("list labels", err)
		}
		defer rows.Close()

		var labels []map[string]any
		for rows.Next() {
			var id, wsID int
			var name, color string
			if err := rows.Scan(&id, &name, &color, &wsID); err != nil {
				continue
			}
			labels = append(labels, map[string]any{"id": id, "name": name, "color": color, "workspace_id": wsID})
		}
		if labels == nil {
			labels = []map[string]any{}
		}

		return toolJSON(map[string]any{"labels": labels})
	})

	type setItemLabelsInput struct {
		ItemID   int   `json:"item_id" jsonschema:"Item ID to set labels on"`
		LabelIDs []int `json:"label_ids" jsonschema:"Label IDs to set (replaces all existing labels)"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "set_item_labels",
		Description: "Set labels on a work item (replaces existing labels).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args setItemLabelsInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		crud := services.NewItemCRUDService(ms.deps.DB)
		item, err := crud.GetByID(args.ItemID)
		if err != nil {
			return toolErrorResult("item not found")
		}
		if ok, _ := ms.canEditItem(user.ID, item.WorkspaceID); !ok {
			return toolErrorResult("item not found")
		}

		tx, err := ms.deps.DB.Begin()
		if err != nil {
			return errInternal("set labels", err)
		}
		defer tx.Rollback() //nolint:errcheck

		// Remove existing labels
		if _, err := tx.Exec("DELETE FROM item_labels WHERE item_id = ?", args.ItemID); err != nil {
			return errInternal("set labels", err)
		}

		// Insert new labels
		for _, labelID := range args.LabelIDs {
			// Verify label belongs to the same workspace
			var wsID int
			err := tx.QueryRow("SELECT workspace_id FROM labels WHERE id = ?", labelID).Scan(&wsID)
			if err == sql.ErrNoRows {
				return toolErrorResult(fmt.Sprintf("label %d not found", labelID))
			}
			if err != nil {
				return errInternal("set labels", err)
			}
			if wsID != item.WorkspaceID {
				return toolErrorResult(fmt.Sprintf("label %d belongs to a different workspace", labelID))
			}

			if _, err := tx.Exec("INSERT INTO item_labels (item_id, label_id) VALUES (?, ?)", args.ItemID, labelID); err != nil {
				if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
					continue // skip duplicates
				}
				return errInternal("set labels", err)
			}
		}

		if err := tx.Commit(); err != nil {
			return errInternal("set labels", err)
		}

		return toolJSON(map[string]any{
			"item_id":   args.ItemID,
			"label_ids": args.LabelIDs,
			"updated":   true,
		})
	})
}
