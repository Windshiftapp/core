package aitools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"windshift/internal/auth"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type listCommentsArgs struct {
	ItemID   int        `json:"item_id,omitempty" jsonschema:"Item ID to list comments for. Provide either this or item_key."`
	ItemKey  string     `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42. Provide either this or item_id."`
	ItemKeys []string   `json:"item_keys,omitempty" jsonschema:"Batch alternative with up to 20 work-item keys"`
	Limit    int        `json:"limit,omitempty" jsonschema:"Most recent comments per item, maximum 25 in batch mode"`
	BeforeAt *time.Time `json:"before_at,omitempty" jsonschema:"Older page timestamp from the previous result, for a single item"`
	BeforeID *int       `json:"before_id,omitempty" jsonschema:"Older page comment ID from the previous result, for a single item"`
}

type addCommentArgs struct {
	ItemID   int    `json:"item_id,omitempty" jsonschema:"Item ID to add comment to. Provide either this or item_key."`
	ItemKey  string `json:"item_key,omitempty" jsonschema:"Item key like PROJ-42. Provide either this or item_id."`
	Content  string `json:"content" jsonschema:"Comment content (plain text or TipTap JSON)"`
	Internal bool   `json:"internal,omitempty" jsonschema:"Set true for an internal note that must stay private to Windshift staff"`
}

type updateCommentArgs struct {
	CommentID int    `json:"comment_id" jsonschema:"Comment ID to update"`
	Content   string `json:"content" jsonschema:"Replacement comment content (plain text or TipTap JSON)"`
}

type deleteCommentArgs struct {
	CommentID int `json:"comment_id" jsonschema:"Comment ID to delete"`
}

type commentDTO struct {
	ID        int    `json:"id"`
	ItemID    int    `json:"item_id"`
	Content   string `json:"content"`
	AuthorID  *int   `json:"author_id,omitempty"`
	Author    string `json:"author_name,omitempty"`
	IsPrivate bool   `json:"is_private,omitempty"`
	CreatedAt string `json:"created_at"`
	URL       string `json:"url,omitempty"`
}

type listCommentsOut struct {
	Comments []commentDTO `json:"comments"`
}

type addCommentOut struct {
	ID        int64  `json:"id"`
	ItemID    int    `json:"item_id"`
	Content   string `json:"content"`
	IsPrivate bool   `json:"is_private"`
}

type updateCommentOut struct {
	ID      int    `json:"id"`
	ItemID  int    `json:"item_id"`
	Content string `json:"content"`
}

func listRecentItemComments(ctx context.Context, env *Env, args listCommentsArgs) (any, error) {
	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}
	limit = min(limit, 25)
	options := services.CommentFeedOptions{Limit: limit}
	if args.BeforeAt != nil || args.BeforeID != nil {
		if args.BeforeAt == nil || args.BeforeAt.IsZero() || args.BeforeID == nil || *args.BeforeID == 0 || len(args.ItemKeys) != 0 {
			return nil, fmt.Errorf("before_at and before_id must be supplied together for one item")
		}
		options.Before = &services.CommentFeedCursor{CreatedAt: *args.BeforeAt, ID: *args.BeforeID}
	}
	if len(args.ItemKeys) == 0 {
		item, err := (itemRef{ItemID: args.ItemID, ItemKey: args.ItemKey}).read(ctx, env)
		if err != nil {
			return nil, err
		}
		page, err := env.CommentService.GetFeedByItemID(item.ID, false, options)
		if err != nil {
			return nil, err
		}
		return commentPage(item, page), nil
	}
	if args.ItemID != 0 || args.ItemKey != "" || len(args.ItemKeys) > 20 {
		return nil, fmt.Errorf("provide one item reference or up to 20 item keys")
	}
	ids := make([]int, 0, len(args.ItemKeys))
	items := make([]*models.Item, 0, len(args.ItemKeys))
	for _, key := range args.ItemKeys {
		item, err := (itemRef{ItemKey: key}).read(ctx, env)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		ids = append(ids, item.ID)
	}
	pages, err := env.CommentService.GetFeedByItemIDs(ids, false, services.CommentFeedOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		page := pages[item.ID]
		result = append(result, commentPage(item, page))
	}
	return map[string]any{"items": result}, nil
}

func commentPage(item *models.Item, page *services.CommentFeedPage) map[string]any {
	out := map[string]any{"comments": mapCommentsDTO(page.Comments), "has_more": page.HasMore,
		"item_url": itemURL(item), "item_key": itemToSummary(item).Key}
	if page.HasMore && len(page.Comments) != 0 {
		last := page.Comments[len(page.Comments)-1]
		out["next_before_at"], out["next_before_id"] = last.CreatedAt, last.ID
	}
	return out
}

func searchDiscussions(ctx context.Context, env *Env, workspaces []int, query string, limit int) (any, error) {
	matches, err := repository.NewItemRepository(env.DB).SearchDiscussions(ctx, workspaces, query, limit+1)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]any, 0, min(limit, len(matches)))
	for _, m := range matches[:min(len(matches), limit)] {
		item, err := (itemRef{ItemID: m.ItemID}).read(ctx, env)
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, services.ErrItemForbidden) {
			continue
		}
		if err != nil {
			return nil, err
		}
		content := []rune(m.Content)
		folded := strings.ToLower(m.Content)
		start := 0
		if position := strings.Index(folded, strings.ToLower(strings.TrimSpace(query))); position > 0 {
			start = max(0, min(len(content), utf8.RuneCountInString(folded[:position]))-200)
		}
		results = append(results, map[string]any{"comment_id": m.ID, "item_key": itemToSummary(item).Key, "url": fmt.Sprintf("/api/v2/comments/%d", m.ID), "item_url": itemURL(item),
			"excerpt": string(content[start:min(len(content), start+700)]), "created_at": m.CreatedAt, "is_private": m.IsPrivate})
	}
	return map[string]any{"matches": results, "truncated": len(matches) > limit}, nil
}

func init() {
	registerRead("get_comment", "Read a comment by the positive comment ID from list_comments or discussion search, with its author, visibility, and source link.", CapabilityReadComment, []string{auth.ScopeItemsRead},
		func(ctx context.Context, env *Env, args struct {
			CommentID int `json:"comment_id"`
		}) (any, error) {
			if args.CommentID <= 0 {
				return nil, errors.New("comment_id must be a positive comment ID")
			}
			if env.CommentService == nil {
				return nil, errors.New("comment service is unavailable")
			}
			comment, err := env.CommentService.Get(args.CommentID)
			if errors.Is(err, repository.ErrNotFound) {
				return nil, repository.ErrNotFound
			}
			if err != nil {
				return nil, err
			}
			item, err := (itemRef{ItemID: comment.ItemID}).read(ctx, env)
			if err != nil {
				return nil, err
			}
			return map[string]any{"comment": commentToDTO(comment.Comment), "item_key": itemToSummary(item).Key,
				"item_url": itemURL(item), "updated_at": comment.UpdatedAt}, nil
		})

	Register(Default, Tool[listCommentsArgs]{
		Name:        "list_comments",
		Group:       CapabilityReadComment,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "Read comments on a work item by ID or key, or retrieve recent visible comments for a batch of up to 20 items.",
		Scopes:      []string{auth.ScopeItemsRead},
		Run: func(ctx context.Context, env *Env, args listCommentsArgs) (any, error) {
			if len(args.ItemKeys) > 0 || args.Limit > 0 || args.BeforeAt != nil || args.BeforeID != nil {
				return listRecentItemComments(ctx, env, args)
			}
			item, err := (itemRef{ItemID: args.ItemID, ItemKey: args.ItemKey}).read(ctx, env)
			if err != nil {
				return nil, err
			}
			comments, err := env.CommentService.GetByItemID(item.ID)
			if err != nil {
				return nil, err
			}
			return listCommentsOut{Comments: mapCommentsDTO(comments)}, nil
		},
	})

	Register(Default, Tool[addCommentArgs]{
		Name:        "add_comment",
		Group:       CapabilityReadComment,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Add a comment to a work item. Identifies the item by numeric ID or key (e.g. PROJ-42).",
		Scopes:      []string{auth.ScopeItemsWrite},
		Run: func(ctx context.Context, env *Env, args addCommentArgs) (any, error) {
			if strings.TrimSpace(args.Content) == "" {
				return map[string]string{"error": "content is required"}, nil
			}
			item, err := (itemRef{ItemID: args.ItemID, ItemKey: args.ItemKey}).read(ctx, env)
			if err != nil {
				return nil, err
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, item.WorkspaceID, models.PermissionItemComment)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			result, err := env.CommentService.CreateContext(ctx, services.CreateCommentParams{
				ItemID:        item.ID,
				AuthorID:      env.UserID,
				Content:       args.Content,
				IsPrivate:     args.Internal,
				ActorUserID:   env.UserID,
				EventMetadata: env.eventMetadata(),
			})
			if err != nil {
				return nil, err
			}
			env.AuditWrite(logger.ResourceComment, int(result.CommentID), "add_comment", fmt.Sprintf("item %d", item.ID))
			return addCommentOut{ID: result.CommentID, ItemID: item.ID, Content: args.Content, IsPrivate: args.Internal}, nil
		},
	})

	Register(Default, Tool[updateCommentArgs]{
		Name:        "update_comment",
		Group:       CapabilityCommentEditing,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Scopes:      []string{auth.ScopeItemsWrite},
		Description: "Update the content of an existing comment. Only the comment's author can edit it, unless the caller holds the edit-others-comments permission in the item's workspace.",
		Run: func(_ context.Context, env *Env, args updateCommentArgs) (any, error) {
			if args.CommentID <= 0 {
				return map[string]string{"error": "comment_id is required"}, nil
			}
			if strings.TrimSpace(args.Content) == "" {
				return map[string]string{"error": "content is required"}, nil
			}
			if denied, err := checkCommentEditAccess(env, args.CommentID); denied != nil || err != nil {
				return denied, err
			}
			// CommentService.Update applies the same sanitization policy as
			// Create (sanitize.Comment), so add_comment and update_comment
			// store content under identical rules.
			updated, err := env.CommentService.Update(args.CommentID, args.Content, env.UserID)
			if err != nil {
				return nil, err
			}
			env.AuditWrite(logger.ResourceComment, updated.ID, "update_comment", fmt.Sprintf("item %d", updated.ItemID))
			return updateCommentOut{ID: updated.ID, ItemID: updated.ItemID, Content: updated.Content}, nil
		},
	})

	Register(Default, Tool[deleteCommentArgs]{
		Name:        "delete_comment",
		Group:       CapabilityCommentEditing,
		Access:      AccessDestructive,
		Risk:        RiskHigh,
		Scopes:      []string{auth.ScopeItemsDelete},
		Description: "Delete a comment. Only the comment's author can delete it, unless the caller holds the edit-others-comments permission in the item's workspace.",
		Run: func(_ context.Context, env *Env, args deleteCommentArgs) (any, error) {
			if args.CommentID <= 0 {
				return map[string]string{"error": "comment_id is required"}, nil
			}
			if denied, err := checkCommentEditAccess(env, args.CommentID); denied != nil || err != nil {
				return denied, err
			}
			if err := env.CommentService.Delete(args.CommentID); err != nil {
				return nil, err
			}
			env.AuditWrite(logger.ResourceComment, args.CommentID, "delete_comment", "")
			return map[string]any{"deleted": true, "comment_id": args.CommentID}, nil
		},
	})
}

// checkCommentEditAccess mirrors the HTTP comment handlers' authorization
// rule (CommentHandler.requireCommentEditAccess): the caller must be the
// comment's author or hold comment.edit_others in the item's workspace.
// Every access failure — missing comment, workspace the caller can't see,
// or insufficient permission — collapses into the same generic not-found
// value so callers can't probe for comment existence (the 404-not-403
// invariant). A nil, nil return means access is granted.
func checkCommentEditAccess(env *Env, commentID int) (any, error) {
	notFound := map[string]string{"error": "comment not found"}
	comment, err := env.CommentService.Get(commentID)
	if err != nil {
		return notFound, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
	}
	if !env.HasWorkspaceAccess(comment.WorkspaceID) {
		return notFound, nil
	}
	if comment.AuthorID != nil && *comment.AuthorID == env.UserID {
		return nil, nil
	}
	canEditOthers, err := env.PermService.HasWorkspacePermission(env.UserID, comment.WorkspaceID, models.PermissionCommentEditOthers)
	if err != nil {
		return nil, err
	}
	if !canEditOthers {
		return notFound, nil
	}
	return nil, nil
}

func mapCommentsDTO(comments []models.Comment) []commentDTO {
	out := make([]commentDTO, len(comments))
	for i, c := range comments {
		out[i] = commentToDTO(c)
	}
	return out
}

func commentToDTO(c models.Comment) commentDTO {
	out := commentDTO{
		ID:        c.ID,
		ItemID:    c.ItemID,
		Content:   c.Content,
		AuthorID:  c.AuthorID,
		Author:    c.AuthorName,
		IsPrivate: c.IsPrivate,
		CreatedAt: c.CreatedAt.Format(time.RFC3339),
	}
	if c.ID > 0 {
		out.URL = fmt.Sprintf("/api/v2/comments/%d", c.ID)
	}
	return out
}
