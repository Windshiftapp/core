package aitools

import (
	"context"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/services"
)

type listCommentsArgs struct {
	ItemID int `json:"item_id" jsonschema:"Item ID to list comments for"`
}

type addCommentArgs struct {
	ItemID  int    `json:"item_id" jsonschema:"Item ID to add comment to"`
	Content string `json:"content" jsonschema:"Comment content (plain text or TipTap JSON)"`
}

type commentDTO struct {
	ID        int    `json:"id"`
	ItemID    int    `json:"item_id"`
	Content   string `json:"content"`
	AuthorID  *int   `json:"author_id,omitempty"`
	Author    string `json:"author_name,omitempty"`
	IsPrivate bool   `json:"is_private,omitempty"`
	CreatedAt string `json:"created_at"`
}

type listCommentsOut struct {
	Comments []commentDTO `json:"comments"`
}

type addCommentOut struct {
	ID      int64  `json:"id"`
	ItemID  int    `json:"item_id"`
	Content string `json:"content"`
}

func init() {
	Register(Default, Tool[listCommentsArgs]{
		Name:        "list_comments",
		Description: "List all comments on a work item.",
		Run: func(_ context.Context, env *Env, args listCommentsArgs) (any, error) {
			if args.ItemID <= 0 {
				return map[string]string{"error": "item_id is required"}, nil
			}
			item, err := services.NewItemCRUDService(env.DB).GetByID(args.ItemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if !env.HasWorkspaceAccess(item.WorkspaceID) {
				return map[string]string{"error": "item not found"}, nil
			}
			comments, err := env.CommentService.GetByItemID(args.ItemID)
			if err != nil {
				return nil, err
			}
			return listCommentsOut{Comments: mapCommentsDTO(comments)}, nil
		},
	})

	Register(Default, Tool[addCommentArgs]{
		Name:        "add_comment",
		Description: "Add a comment to a work item.",
		Run: func(_ context.Context, env *Env, args addCommentArgs) (any, error) {
			if args.ItemID <= 0 {
				return map[string]string{"error": "item_id is required"}, nil
			}
			if strings.TrimSpace(args.Content) == "" {
				return map[string]string{"error": "content is required"}, nil
			}
			item, err := services.NewItemCRUDService(env.DB).GetByID(args.ItemID)
			if err != nil {
				return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if !env.HasWorkspaceAccess(item.WorkspaceID) {
				return map[string]string{"error": "item not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, item.WorkspaceID, models.PermissionItemEdit)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			result, err := env.CommentService.Create(services.CreateCommentParams{
				ItemID:      args.ItemID,
				AuthorID:    env.UserID,
				Content:     args.Content,
				ActorUserID: env.UserID,
			})
			if err != nil {
				return nil, err
			}
			return addCommentOut{ID: result.CommentID, ItemID: args.ItemID, Content: args.Content}, nil
		},
	})
}

func mapCommentsDTO(comments []models.Comment) []commentDTO {
	out := make([]commentDTO, len(comments))
	for i, c := range comments {
		out[i] = commentDTO{
			ID:        c.ID,
			ItemID:    c.ItemID,
			Content:   c.Content,
			AuthorID:  c.AuthorID,
			Author:    c.AuthorName,
			IsPrivate: c.IsPrivate,
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
		}
	}
	return out
}
