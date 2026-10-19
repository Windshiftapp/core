package services

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

// ErrMilestoneCommentNotAuthor rejects edits and deletes of another user's
// milestone comment. Only the author may change their own comment.
var ErrMilestoneCommentNotAuthor = errors.New("only the author can change this milestone comment")

// MilestoneReadAuthorizer confirms that a user may view a milestone.
// PlanningApplicationService implements it.
type MilestoneReadAuthorizer interface {
	AuthorizeMilestoneRead(userID, milestoneID int) (*MilestoneResult, error)
}

// MilestoneCommentStore is the persistence the comment service needs.
// repository.MilestoneCommentRepository implements it.
type MilestoneCommentStore interface {
	ListFeed(milestoneID int, feed repository.MilestoneCommentFeedQuery) ([]models.MilestoneComment, error)
	CountByMilestone(milestoneID int) (int, error)
	GetByID(id int) (*models.MilestoneComment, error)
	Create(milestoneID, authorID int, content string) (int, error)
	UpdateContent(id int, content string) error
	Delete(id int) error
}

// MilestoneCommentService owns Markdown comments on milestones.
// Anyone who can view the milestone may read and add comments; authors edit
// and delete their own. Milestone comments deliberately emit no item events,
// notifications, mention records, or webhooks.
type MilestoneCommentService struct {
	comments MilestoneCommentStore
	access   MilestoneReadAuthorizer
}

// NewMilestoneCommentService creates a MilestoneCommentService.
func NewMilestoneCommentService(comments MilestoneCommentStore, access MilestoneReadAuthorizer) *MilestoneCommentService {
	return &MilestoneCommentService{comments: comments, access: access}
}

// MilestoneCommentFeedPage is a bounded slice of a milestone's comment feed,
// like CommentFeedPage for items. HasMore applies in the requested direction:
// older for default and Before reads, newer for Since reads.
type MilestoneCommentFeedPage struct {
	Comments []models.MilestoneComment
	HasMore  bool
}

// GetFeed returns one page of a milestone's comment feed. It pages like the
// item comment feed: newest first, Before for older comments, and Since for
// newer ones, oldest first.
func (s *MilestoneCommentService) GetFeed(userID, milestoneID int, options CommentFeedOptions) (*MilestoneCommentFeedPage, error) {
	if _, err := s.access.AuthorizeMilestoneRead(userID, milestoneID); err != nil {
		return nil, err
	}
	limit := normalizeCommentFeedLimit(options.Limit)
	// One row past the page shows whether more follow.
	feed := repository.MilestoneCommentFeedQuery{Limit: limit + 1}
	switch {
	case options.Before != nil:
		feed.Before = &repository.MilestoneCommentPosition{CreatedAt: options.Before.CreatedAt, ID: options.Before.ID}
	case options.Since != nil:
		feed.Since = &repository.MilestoneCommentPosition{CreatedAt: options.Since.CreatedAt, ID: options.Since.ID}
	}
	rows, err := s.comments.ListFeed(milestoneID, feed)
	if err != nil {
		return nil, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return &MilestoneCommentFeedPage{Comments: rows, HasMore: hasMore}, nil
}

// CountFeed counts a milestone's comments. Like CountFeedByItemID it does not
// check access; callers count only after GetFeed authorized the read.
func (s *MilestoneCommentService) CountFeed(milestoneID int) (int, error) {
	return s.comments.CountByMilestone(milestoneID)
}

// Create adds a comment authored by userID to a milestone the user can view.
func (s *MilestoneCommentService) Create(userID, milestoneID int, content string) (*models.MilestoneComment, error) {
	if _, err := s.access.AuthorizeMilestoneRead(userID, milestoneID); err != nil {
		return nil, err
	}
	clean, err := cleanMilestoneCommentContent(content)
	if err != nil {
		return nil, err
	}
	id, err := s.comments.Create(milestoneID, userID, clean)
	if err != nil {
		return nil, err
	}
	return s.comments.GetByID(id)
}

// Update replaces the Markdown of the caller's own comment.
func (s *MilestoneCommentService) Update(userID, milestoneID, commentID int, content string) (*models.MilestoneComment, error) {
	if err := s.requireAuthor(userID, milestoneID, commentID); err != nil {
		return nil, err
	}
	clean, err := cleanMilestoneCommentContent(content)
	if err != nil {
		return nil, err
	}
	if err := s.comments.UpdateContent(commentID, clean); err != nil {
		return nil, err
	}
	return s.comments.GetByID(commentID)
}

// Delete removes the caller's own comment.
func (s *MilestoneCommentService) Delete(userID, milestoneID, commentID int) error {
	if err := s.requireAuthor(userID, milestoneID, commentID); err != nil {
		return err
	}
	return s.comments.Delete(commentID)
}

// requireAuthor confirms the user can view the milestone and wrote the
// comment. A comment on a different milestone reads as not found.
func (s *MilestoneCommentService) requireAuthor(userID, milestoneID, commentID int) error {
	if _, err := s.access.AuthorizeMilestoneRead(userID, milestoneID); err != nil {
		return err
	}
	comment, err := s.comments.GetByID(commentID)
	if err != nil {
		return err
	}
	if comment.MilestoneID != milestoneID {
		return fmt.Errorf("milestone comment %d not on milestone %d: %w", commentID, milestoneID, repository.ErrNotFound)
	}
	if comment.AuthorID != userID {
		return ErrMilestoneCommentNotAuthor
	}
	return nil
}

// milestoneCommentBreakRegex matches the `<br />` hard breaks RichText keeps
// for the Markdown editor; a comment of only breaks counts as empty.
var milestoneCommentBreakRegex = regexp.MustCompile(`(?i)<br\s*/?>`)

// cleanMilestoneCommentContent uses RichText rather than Comment so the
// editor's `<br />` hard breaks survive; Comment strips them.
func cleanMilestoneCommentContent(content string) (string, error) {
	clean := sanitize.RichText.Sanitize(content)
	if strings.TrimSpace(milestoneCommentBreakRegex.ReplaceAllString(clean, "")) == "" {
		return "", planningValidationError("content", "content is required")
	}
	return clean, nil
}
