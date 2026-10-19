package services

import (
	"fmt"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// MilestoneReadAuthorizer confirms that a user may view a milestone.
// PlanningApplicationService implements it.
type MilestoneReadAuthorizer interface {
	AuthorizeMilestoneRead(userID, milestoneID int) (*MilestoneResult, error)
}

// MilestoneCommentPermissions checks workspace permissions.
// PermissionService implements it.
type MilestoneCommentPermissions interface {
	HasWorkspacePermission(userID, workspaceID int, permission string) (bool, error)
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
// Anyone who can view the milestone may read its comments. On a local
// milestone the item comment permissions of its workspace apply: item.comment
// to add a comment, comment.edit_others to change someone else's. Global
// milestones have no workspace, so any viewer may comment and only authors
// change their own. Milestone comments deliberately emit no item events,
// notifications, mention records, or webhooks.
type MilestoneCommentService struct {
	comments    MilestoneCommentStore
	access      MilestoneReadAuthorizer
	permissions MilestoneCommentPermissions
}

// NewMilestoneCommentService creates a MilestoneCommentService.
func NewMilestoneCommentService(comments MilestoneCommentStore, access MilestoneReadAuthorizer, permissions MilestoneCommentPermissions) *MilestoneCommentService {
	return &MilestoneCommentService{comments: comments, access: access, permissions: permissions}
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

// Create adds a comment authored by userID. The Markdown is stored as
// written, like item comments; it is made safe when rendered.
func (s *MilestoneCommentService) Create(userID, milestoneID int, content string) (*models.MilestoneComment, error) {
	milestone, err := s.access.AuthorizeMilestoneRead(userID, milestoneID)
	if err != nil {
		return nil, err
	}
	if err := s.requireWorkspacePermission(userID, milestone, models.PermissionItemComment); err != nil {
		return nil, err
	}
	if err := validateCommentSource(content); err != nil {
		return nil, err
	}
	id, err := s.comments.Create(milestoneID, userID, content)
	if err != nil {
		return nil, err
	}
	return s.comments.GetByID(id)
}

// Update replaces a comment's Markdown.
func (s *MilestoneCommentService) Update(userID, milestoneID, commentID int, content string) (*models.MilestoneComment, error) {
	if err := s.requireEdit(userID, milestoneID, commentID); err != nil {
		return nil, err
	}
	if err := validateCommentSource(content); err != nil {
		return nil, err
	}
	if err := s.comments.UpdateContent(commentID, content); err != nil {
		return nil, err
	}
	return s.comments.GetByID(commentID)
}

// Delete removes a comment.
func (s *MilestoneCommentService) Delete(userID, milestoneID, commentID int) error {
	if err := s.requireEdit(userID, milestoneID, commentID); err != nil {
		return err
	}
	return s.comments.Delete(commentID)
}

// requireEdit mirrors the item comment rule: authors change their own
// comments, comment.edit_others holders anyone's. A comment on a different
// milestone reads as not found.
func (s *MilestoneCommentService) requireEdit(userID, milestoneID, commentID int) error {
	milestone, err := s.access.AuthorizeMilestoneRead(userID, milestoneID)
	if err != nil {
		return err
	}
	comment, err := s.comments.GetByID(commentID)
	if err != nil {
		return err
	}
	if comment.MilestoneID != milestoneID {
		return fmt.Errorf("milestone comment %d not on milestone %d: %w", commentID, milestoneID, repository.ErrNotFound)
	}
	if comment.AuthorID == userID {
		return nil
	}
	if milestone.IsGlobal {
		return ErrPlanningForbidden
	}
	return s.requireWorkspacePermission(userID, milestone, models.PermissionCommentEditOthers)
}

// requireWorkspacePermission checks a permission in a local milestone's
// workspace. Global milestones have no workspace and pass.
func (s *MilestoneCommentService) requireWorkspacePermission(userID int, milestone *MilestoneResult, permission string) error {
	if milestone.IsGlobal {
		return nil
	}
	if milestone.WorkspaceID == nil {
		return ErrPlanningForbidden
	}
	allowed, err := s.permissions.HasWorkspacePermission(userID, *milestone.WorkspaceID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPlanningForbidden
	}
	return nil
}
