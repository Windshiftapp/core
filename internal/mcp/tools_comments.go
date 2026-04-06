package mcp

import (
	"context"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"windshift/internal/models"
	"windshift/internal/services"
)

func (ms *MCPServer) registerCommentTools() {
	type listCommentsInput struct {
		ItemID int `json:"item_id" jsonschema:"Item ID to list comments for"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "list_comments",
		Description: "List all comments on a work item.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listCommentsInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		// Check item exists and user can view it
		crud := services.NewItemCRUDService(ms.deps.DB)
		item, err := crud.GetByID(args.ItemID)
		if err != nil {
			return toolErrorResult("item not found")
		}
		if ok, _ := ms.canViewItem(user.ID, item.WorkspaceID); !ok {
			return toolErrorResult("item not found")
		}

		comments, err := ms.deps.CommentService.GetByItemID(args.ItemID)
		if err != nil {
			return errInternal("list comments", err)
		}

		return toolJSON(map[string]any{"comments": mapComments(comments)})
	})

	type addCommentInput struct {
		ItemID  int    `json:"item_id" jsonschema:"Item ID to add comment to"`
		Content string `json:"content" jsonschema:"Comment content (plain text or TipTap JSON)"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "add_comment",
		Description: "Add a comment to a work item.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addCommentInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		if strings.TrimSpace(args.Content) == "" {
			return toolErrorResult("content is required")
		}

		// Check item exists and user can edit it
		crud := services.NewItemCRUDService(ms.deps.DB)
		item, err := crud.GetByID(args.ItemID)
		if err != nil {
			return toolErrorResult("item not found")
		}
		if ok, _ := ms.canEditItem(user.ID, item.WorkspaceID); !ok {
			return toolErrorResult("item not found")
		}

		result, err := ms.deps.CommentService.Create(services.CreateCommentParams{
			ItemID:      args.ItemID,
			AuthorID:    user.ID,
			Content:     args.Content,
			ActorUserID: user.ID,
		})
		if err != nil {
			return errInternal("add comment", err)
		}

		return toolJSON(map[string]any{
			"id":      result.CommentID,
			"item_id": args.ItemID,
			"content": args.Content,
			"author":  user.Username,
		})
	})
}

func mapComments(comments []models.Comment) []map[string]any {
	result := make([]map[string]any, len(comments))
	for i, c := range comments {
		m := map[string]any{
			"id":         c.ID,
			"item_id":    c.ItemID,
			"content":    c.Content,
			"created_at": c.CreatedAt.Format(time.RFC3339),
		}
		if c.AuthorID != nil {
			m["author_id"] = *c.AuthorID
		}
		if c.AuthorName != "" {
			m["author_name"] = c.AuthorName
		}
		if c.IsPrivate {
			m["is_private"] = true
		}
		result[i] = m
	}
	return result
}
