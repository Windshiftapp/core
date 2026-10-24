package v2

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"windshift/internal/markdown"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/validation"
)

func registerCommentRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/items/{item_id}/comments", AuthAuthenticated, []string{"items:read"}, listComments(deps))
	builder.JSON(http.MethodPost, "/items/{item_id}/comments", http.StatusCreated, false, AuthAuthenticated, []string{"items:write"}, createComment(deps))
	builder.Read("/comments/{comment_id}", AuthAuthenticated, []string{"items:read"}, getComment(deps))
	builder.JSON(http.MethodPatch, "/comments/{comment_id}", http.StatusOK, true, AuthAuthenticated, []string{"items:write"}, updateComment(deps))
	builder.Command(http.MethodDelete, "/comments/{comment_id}", AuthAuthenticated, []string{"items:delete"}, deleteComment(deps))
	builder.JSON(http.MethodPost, "/comments/batch", http.StatusOK, false, AuthAuthenticated, []string{"items:read"}, commentsBatch(deps))
	builder.JSON(http.MethodPost, "/comments/{comment_id}/reactions", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, addCommentReaction(deps))
	builder.Command(http.MethodDelete, "/comments/{comment_id}/reactions/{reaction}", AuthAuthenticated, []string{"items:write"}, removeCommentReaction(deps))
}

type commentFeedDTO struct {
	Comments      []models.Comment `json:"comments"`
	NextCursor    string           `json:"next_cursor,omitempty"`
	RefreshCursor string           `json:"refresh_cursor,omitempty"`
	HasMore       bool             `json:"has_more"`
	Total         *int             `json:"total"`
}

type commentCursor struct {
	Direction string    `json:"direction"`
	CreatedAt time.Time `json:"created_at"`
	ID        int       `json:"id"`
}

type commentCreateRequest struct {
	Content   string `json:"content"`
	IsPrivate bool   `json:"is_private"`
}

const maxCommentBatchItems = 500

type commentBatchRequest struct {
	ItemIDs          []int `json:"item_ids"`
	PageSize         int   `json:"page_size"`
	IncludeReactions bool  `json:"include_reactions,omitempty"`
}

type commentBatchEntry struct {
	ItemID   int              `json:"item_id"`
	Comments []models.Comment `json:"comments"`
	HasMore  bool             `json:"has_more"`
}

func commentsBatch(deps Deps) jsonOperation[commentBatchRequest, []commentBatchEntry] {
	return func(r *http.Request, input commentBatchRequest) ([]commentBatchEntry, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		itemIDs, err := normalizeCommentBatchItemIDs(input.ItemIDs)
		if err != nil {
			return nil, err
		}
		pageSize := services.DefaultCommentFeedLimit
		if input.PageSize != 0 {
			if input.PageSize < 1 || input.PageSize > services.MaxCommentFeedLimit {
				return nil, newError(http.StatusBadRequest, "invalid_request", "page_size is out of range")
			}
			pageSize = input.PageSize
		}
		options := services.CommentFeedOptions{Limit: pageSize}
		includeOwner, _ := deps.CommentAccess.HasGlobalPermission(user.ID, models.PermissionUserList)
		if !includeOwner {
			includeOwner, _ = deps.SystemAdmins.IsSystemAdmin(user.ID)
		}

		// Same visibility contract as the single-item feed: workspace item view
		// or the approval-approver fallback. Unreadable items are omitted
		// without revealing whether they are missing or unauthorized.
		visible := make([]int, 0, len(itemIDs))
		workspaceAllowed := make(map[int]bool)
		for _, itemID := range itemIDs {
			item, err := deps.Items.FindByID(itemID)
			if err != nil {
				if errors.Is(err, repository.ErrNotFound) {
					continue
				}
				return nil, commentError(err)
			}
			allowed, ok := workspaceAllowed[item.WorkspaceID]
			if !ok {
				allowed, err = deps.CommentAccess.HasWorkspacePermission(user.ID, item.WorkspaceID, models.PermissionItemView)
				if err != nil {
					return nil, internalError(err)
				}
				workspaceAllowed[item.WorkspaceID] = allowed
			}
			if !allowed {
				allowed, err = deps.Comments.UserCanReadItemAsApprover(r.Context(), user.ID, item.ID)
				if err != nil {
					return nil, internalError(err)
				}
			}
			if allowed {
				visible = append(visible, itemID)
			}
		}

		pages, err := deps.Comments.GetFeedByItemIDs(visible, includeOwner, options)
		if err != nil {
			return nil, internalError(err)
		}
		if input.IncludeReactions {
			if err := attachBatchReactions(deps, pages); err != nil {
				return nil, internalError(err)
			}
		}
		entries := make([]commentBatchEntry, 0, len(visible))
		for _, itemID := range visible {
			page := pages[itemID]
			comments := []models.Comment{}
			hasMore := false
			if page != nil {
				comments = page.Comments
				hasMore = page.HasMore
			}
			if comments == nil {
				comments = []models.Comment{}
			}
			if err := renderCommentHTML(comments); err != nil {
				return nil, internalError(err)
			}
			entries = append(entries, commentBatchEntry{ItemID: itemID, Comments: comments, HasMore: hasMore})
		}
		return entries, nil
	}
}

func normalizeCommentBatchItemIDs(input []int) ([]int, error) {
	if len(input) == 0 {
		err := newError(http.StatusBadRequest, "invalid_request", "item_ids is required")
		err.Details = map[string]string{"field": "item_ids"}
		return nil, err
	}
	if len(input) > maxCommentBatchItems {
		err := newError(http.StatusBadRequest, "invalid_request", "item_ids supports at most 500 values")
		err.Details = map[string]string{"field": "item_ids"}
		return nil, err
	}
	seen := make(map[int]struct{}, len(input))
	ids := make([]int, 0, len(input))
	for _, id := range input {
		if id < 1 {
			err := newError(http.StatusBadRequest, "invalid_request", "item_ids is invalid")
			err.Details = map[string]string{"field": "item_ids"}
			return nil, err
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

type commentReactionRequest struct {
	Reaction string `json:"reaction"`
}

type commentPatchRequest struct {
	Content Optional[string] `json:"content"`
}

func listComments(deps Deps) readOperation[commentFeedDTO] {
	return func(r *http.Request) (commentFeedDTO, error) {
		user, item, err := commentItem(r, deps)
		if err != nil {
			return commentFeedDTO{}, err
		}
		if err := requireCommentRead(r, deps, user.ID, item.ID, item.WorkspaceID); err != nil {
			return commentFeedDTO{}, err
		}
		options, err := parseCommentOptions(r)
		if err != nil {
			return commentFeedDTO{}, err
		}
		includeOwner, _ := deps.CommentAccess.HasGlobalPermission(user.ID, models.PermissionUserList)
		if !includeOwner {
			includeOwner, _ = deps.SystemAdmins.IsSystemAdmin(user.ID)
		}
		page, err := deps.Comments.GetFeedByItemID(item.ID, includeOwner, options)
		if err != nil {
			return commentFeedDTO{}, commentError(err)
		}
		if err := renderCommentHTML(page.Comments); err != nil {
			return commentFeedDTO{}, internalError(err)
		}
		if includeReactions, err := optionalBoolQuery(r, "include_reactions"); err != nil {
			return commentFeedDTO{}, err
		} else if includeReactions {
			if err := deps.Comments.AttachReactions(page.Comments); err != nil {
				return commentFeedDTO{}, internalError(err)
			}
		}
		var total *int
		if options.Before == nil && options.Since == nil {
			count, err := deps.Comments.CountFeedByItemID(item.ID)
			if err != nil {
				return commentFeedDTO{}, internalError(err)
			}
			total = &count
		}
		nextCursor, refreshCursor := commentResponseCursors(page.Comments, options, page.HasMore)
		return commentFeedDTO{
			Comments: page.Comments, NextCursor: nextCursor, RefreshCursor: refreshCursor,
			HasMore: page.HasMore, Total: total,
		}, nil
	}
}

func createComment(deps Deps) jsonOperation[commentCreateRequest, models.Comment] {
	return func(r *http.Request, input commentCreateRequest) (models.Comment, error) {
		user, item, err := commentItem(r, deps)
		if err != nil {
			return models.Comment{}, err
		}
		if strings.TrimSpace(input.Content) == "" {
			return models.Comment{}, newError(http.StatusBadRequest, "invalid_request", "content is required")
		}
		if err := requireCommentPermission(deps, user.ID, item.WorkspaceID, models.PermissionItemComment); err != nil {
			return models.Comment{}, err
		}
		result, err := deps.Comments.Create(services.CreateCommentParams{ItemID: item.ID, AuthorID: user.ID, Content: input.Content, IsPrivate: input.IsPrivate, ActorUserID: user.ID})
		if err != nil {
			return models.Comment{}, commentError(err)
		}
		comment, err := deps.Comments.Get(int(result.CommentID))
		if err != nil {
			return models.Comment{}, commentError(err)
		}
		if err := renderCommentHTML([]models.Comment{comment.Comment}); err != nil {
			return models.Comment{}, internalError(err)
		}
		body := comment.Comment
		body.ContentHTML, err = markdown.Render(body.Content)
		return body, commentError(err)
	}
}

func getComment(deps Deps) readOperation[models.Comment] {
	return func(r *http.Request) (models.Comment, error) {
		user, comment, err := commentTarget(r, deps)
		if err != nil {
			return models.Comment{}, err
		}
		if err := requireCommentRead(r, deps, user.ID, comment.ItemID, comment.WorkspaceID); err != nil {
			return models.Comment{}, err
		}
		body := []models.Comment{comment.Comment}
		if includeReactions, err := optionalBoolQuery(r, "include_reactions"); err != nil {
			return models.Comment{}, err
		} else if includeReactions {
			if err := deps.Comments.AttachReactions(body); err != nil {
				return models.Comment{}, internalError(err)
			}
		}
		body[0].ContentHTML, err = markdown.Render(body[0].Content)
		return body[0], commentError(err)
	}
}

func updateComment(deps Deps) jsonOperation[commentPatchRequest, models.Comment] {
	return func(r *http.Request, input commentPatchRequest) (models.Comment, error) {
		if !input.Content.Set || input.Content.Null || strings.TrimSpace(input.Content.Value) == "" {
			return models.Comment{}, newError(http.StatusBadRequest, "invalid_request", "content is required")
		}
		user, comment, err := commentTarget(r, deps)
		if err != nil {
			return models.Comment{}, err
		}
		if err := requireCommentEdit(r, deps, user.ID, comment); err != nil {
			return models.Comment{}, err
		}
		updated, err := deps.Comments.UpdateWithEffects(services.UpdateCommentParams{
			CommentID: comment.ID,
			Content:   input.Content.Value,
			Actor:     auditActor(r, user),
		})
		if err != nil {
			return models.Comment{}, commentError(err)
		}
		body := []models.Comment{*updated}
		if includeReactions, err := optionalBoolQuery(r, "include_reactions"); err != nil {
			return models.Comment{}, err
		} else if includeReactions {
			if err := deps.Comments.AttachReactions(body); err != nil {
				return models.Comment{}, internalError(err)
			}
		}
		body[0].ContentHTML, err = markdown.Render(body[0].Content)
		return body[0], commentError(err)
	}
}

// addCommentReaction records the caller's reaction (core#298) and returns the
// comment's grouped reactions. Reacting needs the same permission as
// commenting; adding an existing reaction is a no-op.
func addCommentReaction(deps Deps) jsonOperation[commentReactionRequest, []models.CommentReaction] {
	return func(r *http.Request, input commentReactionRequest) ([]models.CommentReaction, error) {
		user, comment, err := commentReactionTarget(r, deps, input.Reaction)
		if err != nil {
			return nil, err
		}
		reactions, err := deps.Comments.AddReaction(comment.ID, user.ID, input.Reaction)
		return reactions, commentError(err)
	}
}

// removeCommentReaction withdraws the caller's own reaction; removing a
// reaction that does not exist is a no-op.
func removeCommentReaction(deps Deps) commandOperation {
	return func(r *http.Request) error {
		reaction := r.PathValue("reaction")
		user, comment, err := commentReactionTarget(r, deps, reaction)
		if err != nil {
			return err
		}
		return commentError(deps.Comments.RemoveReaction(comment.ID, user.ID, reaction))
	}
}

func commentReactionTarget(r *http.Request, deps Deps, reaction string) (*models.User, *services.CommentWithDetails, error) {
	if !services.ValidCommentReaction(reaction) {
		err := newError(http.StatusBadRequest, "invalid_request", "reaction must be one of "+strings.Join(services.CommentReactionKeys, ", "))
		err.Details = map[string]string{"field": "reaction"}
		return nil, nil, err
	}
	user, comment, err := commentTarget(r, deps)
	if err != nil {
		return nil, nil, err
	}
	if err := requireCommentRead(r, deps, user.ID, comment.ItemID, comment.WorkspaceID); err != nil {
		return nil, nil, err
	}
	if err := requireCommentPermission(deps, user.ID, comment.WorkspaceID, models.PermissionItemComment); err != nil {
		return nil, nil, err
	}
	return user, comment, nil
}

func attachBatchReactions(deps Deps, pages map[int]*services.CommentFeedPage) error {
	var all []models.Comment
	for _, page := range pages {
		if page != nil {
			all = append(all, page.Comments...)
		}
	}
	if err := deps.Comments.AttachReactions(all); err != nil {
		return err
	}
	byID := make(map[int][]models.CommentReaction, len(all))
	for _, comment := range all {
		if comment.Reactions != nil {
			byID[comment.ID] = comment.Reactions
		}
	}
	for _, page := range pages {
		if page == nil {
			continue
		}
		for i := range page.Comments {
			page.Comments[i].Reactions = byID[page.Comments[i].ID]
		}
	}
	return nil
}

func deleteComment(deps Deps) commandOperation {
	return func(r *http.Request) error {
		user, comment, err := commentTarget(r, deps)
		if err != nil {
			return err
		}
		if err := requireCommentEdit(r, deps, user.ID, comment); err != nil {
			return err
		}
		return commentError(deps.Comments.DeleteWithEffects(services.DeleteCommentParams{
			CommentID: comment.ID,
			Actor:     auditActor(r, user),
		}))
	}
}

func requireCommentRead(r *http.Request, deps Deps, userID, itemID, workspaceID int) error {
	allowed, err := deps.CommentAccess.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	if err != nil {
		return internalError(err)
	}
	if allowed {
		return nil
	}
	allowed, err = deps.Comments.UserCanReadItemAsApprover(r.Context(), userID, itemID)
	if err != nil {
		return internalError(err)
	}
	if !allowed {
		return newError(http.StatusNotFound, "not_found", "Item was not found")
	}
	return nil
}

func commentItem(r *http.Request, deps Deps) (*models.User, *models.Item, error) {
	user, err := principal(r)
	if err != nil {
		return nil, nil, err
	}
	itemID, err := pathID(r, "item_id")
	if err != nil {
		return nil, nil, err
	}
	item, err := deps.Items.FindByID(itemID)
	if err != nil {
		return nil, nil, commentError(err)
	}
	return user, item, nil
}

func commentTarget(r *http.Request, deps Deps) (*models.User, *services.CommentWithDetails, error) {
	user, err := principal(r)
	if err != nil {
		return nil, nil, err
	}
	commentID, err := pathID(r, "comment_id")
	if err != nil {
		return nil, nil, err
	}
	comment, err := deps.Comments.Get(commentID)
	if err != nil {
		return nil, nil, commentError(err)
	}
	return user, comment, nil
}

func requireCommentPermission(deps Deps, userID, workspaceID int, permission string) error {
	allowed, err := deps.CommentAccess.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil {
		return internalError(err)
	}
	if !allowed {
		return newError(http.StatusNotFound, "not_found", "Item was not found")
	}
	return nil
}

func requireCommentEdit(r *http.Request, deps Deps, userID int, comment *services.CommentWithDetails) error {
	if err := requireCommentRead(r, deps, userID, comment.ItemID, comment.WorkspaceID); err != nil {
		return err
	}
	if comment.AuthorID != nil && *comment.AuthorID == userID {
		return nil
	}
	return requireCommentPermission(deps, userID, comment.WorkspaceID, models.PermissionCommentEditOthers)
}

func parseCommentOptions(r *http.Request) (services.CommentFeedOptions, error) {
	return parseCommentFeedOptions(r, "comments")
}

// parseCommentFeedOptions reads page_size and an opaque cursor of the given
// kind, so a cursor from one comment feed is rejected by another.
func parseCommentFeedOptions(r *http.Request, kind string) (services.CommentFeedOptions, error) {
	limit, err := parsePositiveInt(r, "page_size", services.DefaultCommentFeedLimit, services.MaxCommentFeedLimit)
	if err != nil {
		return services.CommentFeedOptions{}, err
	}
	options := services.CommentFeedOptions{Limit: limit}
	query := r.URL.Query()
	for _, removed := range []string{"before", "before_id", "since", "since_id"} {
		if query.Has(removed) {
			return options, newError(http.StatusBadRequest, "invalid_request", "Use the opaque cursor parameter")
		}
	}
	raw := query.Get("cursor")
	if raw == "" {
		return options, nil
	}
	var cursor commentCursor
	if err := decodeOpaqueCursor(kind, raw, &cursor); err != nil || cursor.ID == 0 || cursor.CreatedAt.IsZero() {
		return options, newError(http.StatusBadRequest, "invalid_request", "cursor is invalid")
	}
	value := services.CommentFeedCursor{CreatedAt: cursor.CreatedAt, ID: cursor.ID}
	switch cursor.Direction {
	case "older":
		options.Before = &value
	case "newer":
		options.Since = &value
	default:
		return options, newError(http.StatusBadRequest, "invalid_request", "cursor is invalid")
	}
	return options, nil
}

func commentResponseCursors(comments []models.Comment, options services.CommentFeedOptions, hasMore bool) (nextCursor, refreshCursor string) {
	positions := make([]services.CommentFeedCursor, len(comments))
	for i, comment := range comments {
		positions[i] = services.CommentFeedCursor{CreatedAt: comment.CreatedAt, ID: comment.ID}
	}
	return commentFeedCursors("comments", positions, options, hasMore)
}

// commentFeedCursors returns the next and refresh cursors of a comment feed
// page, given the positions of its rows.
func commentFeedCursors(kind string, positions []services.CommentFeedCursor, options services.CommentFeedOptions, hasMore bool) (nextCursor, refreshCursor string) {
	if len(positions) == 0 {
		return "", ""
	}
	oldest, newest := positions[0], positions[0]
	for _, position := range positions[1:] {
		if position.CreatedAt.Before(oldest.CreatedAt) || position.CreatedAt.Equal(oldest.CreatedAt) && position.ID < oldest.ID {
			oldest = position
		}
		if position.CreatedAt.After(newest.CreatedAt) || position.CreatedAt.Equal(newest.CreatedAt) && position.ID > newest.ID {
			newest = position
		}
	}
	refresh := encodeOpaqueCursor(kind, commentCursor{Direction: "newer", CreatedAt: newest.CreatedAt, ID: newest.ID})
	if !hasMore {
		return "", refresh
	}
	if options.Since != nil {
		return encodeOpaqueCursor(kind, commentCursor{Direction: "newer", CreatedAt: newest.CreatedAt, ID: newest.ID}), refresh
	}
	return encodeOpaqueCursor(kind, commentCursor{Direction: "older", CreatedAt: oldest.CreatedAt, ID: oldest.ID}), refresh
}

func renderCommentHTML(comments []models.Comment) error {
	for i := range comments {
		html, err := markdown.Render(comments[i].Content)
		if err != nil {
			return err
		}
		comments[i].ContentHTML = html
	}
	return nil
}

func commentError(err error) error {
	if err == nil {
		return nil
	}
	var invalid *validation.ValidationError
	var serviceErr *services.ServiceError
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Comment was not found")
	case errors.As(err, &invalid):
		return newError(http.StatusBadRequest, "invalid_request", invalid.Message)
	case errors.Is(err, services.ErrInvalidCommentReaction):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.As(err, &serviceErr):
		return newError(serviceErr.StatusCode, "conflict", serviceErr.Message)
	default:
		return internalError(err)
	}
}
