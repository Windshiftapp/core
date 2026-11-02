package v2

import (
	"errors"
	"net/http"
	"strings"

	"windshift/internal/markdown"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/validation"
)

// registerMilestoneCommentRoutes publishes Markdown comments on milestones.
// The same routes serve global and workspace-local milestones. Viewers read
// comments; local milestones apply their workspace's item comment
// permissions (see MilestoneCommentService). Milestone comments send no
// notifications.
func registerMilestoneCommentRoutes(builder *routeBuilder, deps Deps) {
	const comments = "/milestones/{milestone_id}/comments"
	const comment = comments + "/{comment_id}"
	builder.Read(comments, AuthAuthenticated, []string{"milestones:read"}, listMilestoneComments(deps.MilestoneComments))
	builder.JSON(http.MethodPost, comments, http.StatusCreated, false, AuthAuthenticated, []string{"milestones:write"}, createMilestoneComment(deps.MilestoneComments))
	builder.JSON(http.MethodPatch, comment, http.StatusOK, true, AuthAuthenticated, []string{"milestones:write"}, updateMilestoneComment(deps.MilestoneComments))
	builder.Command(http.MethodDelete, comment, AuthAuthenticated, []string{"milestones:delete"}, deleteMilestoneComment(deps.MilestoneComments))
}

// milestoneCommentCursorKind binds cursors to the milestone comment feed.
const milestoneCommentCursorKind = "milestone_comments"

// milestoneCommentFeedDTO pages like the item comment feed (commentFeedDTO):
// newest first, opaque cursors, and a total on the first page only.
type milestoneCommentFeedDTO struct {
	Comments      []models.MilestoneComment `json:"comments"`
	NextCursor    string                    `json:"next_cursor,omitempty"`
	RefreshCursor string                    `json:"refresh_cursor,omitempty"`
	HasMore       bool                      `json:"has_more"`
	Total         *int                      `json:"total"`
}

type milestoneCommentCreateRequest struct {
	Content string `json:"content"`
}

type milestoneCommentPatchRequest struct {
	Content Optional[string] `json:"content"`
}

func listMilestoneComments(comments milestoneCommentApplication) readOperation[milestoneCommentFeedDTO] {
	return func(r *http.Request) (milestoneCommentFeedDTO, error) {
		user, milestoneID, err := planningTarget(r, "milestone_id")
		if err != nil {
			return milestoneCommentFeedDTO{}, err
		}
		options, err := parseCommentFeedOptions(r, milestoneCommentCursorKind)
		if err != nil {
			return milestoneCommentFeedDTO{}, err
		}
		page, err := comments.GetFeed(user.ID, milestoneID, options)
		if err != nil {
			return milestoneCommentFeedDTO{}, milestoneCommentError(err)
		}
		positions := make([]services.CommentFeedCursor, len(page.Comments))
		for i := range page.Comments {
			if err := renderMilestoneCommentHTML(&page.Comments[i]); err != nil {
				return milestoneCommentFeedDTO{}, internalError(err)
			}
			positions[i] = services.CommentFeedCursor{CreatedAt: page.Comments[i].CreatedAt, ID: page.Comments[i].ID}
		}
		var total *int
		if options.Before == nil && options.Since == nil {
			count, err := comments.CountFeed(milestoneID)
			if err != nil {
				return milestoneCommentFeedDTO{}, internalError(err)
			}
			total = &count
		}
		nextCursor, refreshCursor := commentFeedCursors(milestoneCommentCursorKind, positions, options, page.HasMore)
		return milestoneCommentFeedDTO{
			Comments: page.Comments, NextCursor: nextCursor, RefreshCursor: refreshCursor,
			HasMore: page.HasMore, Total: total,
		}, nil
	}
}

func createMilestoneComment(comments milestoneCommentApplication) jsonOperation[milestoneCommentCreateRequest, models.MilestoneComment] {
	return func(r *http.Request, input milestoneCommentCreateRequest) (models.MilestoneComment, error) {
		user, milestoneID, err := planningTarget(r, "milestone_id")
		if err != nil {
			return models.MilestoneComment{}, err
		}
		if strings.TrimSpace(input.Content) == "" {
			return models.MilestoneComment{}, newError(http.StatusBadRequest, "invalid_request", "content is required")
		}
		created, err := comments.Create(user.ID, milestoneID, input.Content)
		if err != nil {
			return models.MilestoneComment{}, milestoneCommentError(err)
		}
		if err := renderMilestoneCommentHTML(created); err != nil {
			return models.MilestoneComment{}, internalError(err)
		}
		return *created, nil
	}
}

func updateMilestoneComment(comments milestoneCommentApplication) jsonOperation[milestoneCommentPatchRequest, models.MilestoneComment] {
	return func(r *http.Request, input milestoneCommentPatchRequest) (models.MilestoneComment, error) {
		if !input.Content.Set || input.Content.Null || strings.TrimSpace(input.Content.Value) == "" {
			return models.MilestoneComment{}, newError(http.StatusBadRequest, "invalid_request", "content is required")
		}
		user, milestoneID, commentID, err := milestoneCommentTarget(r)
		if err != nil {
			return models.MilestoneComment{}, err
		}
		updated, err := comments.Update(user.ID, milestoneID, commentID, input.Content.Value)
		if err != nil {
			return models.MilestoneComment{}, milestoneCommentError(err)
		}
		if err := renderMilestoneCommentHTML(updated); err != nil {
			return models.MilestoneComment{}, internalError(err)
		}
		return *updated, nil
	}
}

func deleteMilestoneComment(comments milestoneCommentApplication) commandOperation {
	return func(r *http.Request) error {
		user, milestoneID, commentID, err := milestoneCommentTarget(r)
		if err != nil {
			return err
		}
		return milestoneCommentError(comments.Delete(user.ID, milestoneID, commentID))
	}
}

func milestoneCommentTarget(r *http.Request) (user *models.User, milestoneID, commentID int, err error) {
	user, milestoneID, err = planningTarget(r, "milestone_id")
	if err != nil {
		return nil, 0, 0, err
	}
	commentID, err = pathID(r, "comment_id")
	return user, milestoneID, commentID, err
}

func renderMilestoneCommentHTML(comment *models.MilestoneComment) error {
	html, err := markdown.Render(comment.Content)
	if err != nil {
		return err
	}
	comment.ContentHTML = html
	return nil
}

// milestoneCommentError maps service errors. Invalid Markdown is a bad
// request, like item comments. Missing permissions and comments on other
// milestones read as not found (planningError).
func milestoneCommentError(err error) error {
	var invalid *validation.ValidationError
	if errors.As(err, &invalid) {
		return newError(http.StatusBadRequest, "invalid_request", invalid.Message)
	}
	return planningError(err)
}
