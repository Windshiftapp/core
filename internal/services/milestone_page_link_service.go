package services

import (
	"errors"
	"fmt"

	"windshift/internal/models"
	"windshift/internal/repository"
)

var (
	// ErrMilestonePageLinksGlobal rejects page links on global milestones. A
	// global milestone has no workspace, and page links stay inside one
	// workspace, so only workspace (local) milestones link pages.
	ErrMilestonePageLinksGlobal = errors.New("page links are only available on workspace milestones")

	// ErrMilestonePageNotFound covers a page that does not exist, lives in
	// another workspace, or is hidden from the user. They share one response
	// so page existence does not leak.
	ErrMilestonePageNotFound = errors.New("page not found in the milestone's workspace")
)

// MilestonePageLinkAccess loads a milestone and checks the user's rights on
// it. PlanningApplicationService implements it.
type MilestonePageLinkAccess interface {
	AuthorizeMilestoneRead(userID, milestoneID int) (*MilestoneResult, error)
	AuthorizeMilestoneWrite(userID, milestoneID int) (*MilestoneResult, error)
}

// MilestonePageLinkStore is the persistence the page-link service needs.
// repository.MilestonePageLinkRepository implements it.
type MilestonePageLinkStore interface {
	ListByMilestone(milestoneID int) ([]models.MilestonePageLink, error)
	GetByID(id int) (*models.MilestonePageLink, error)
	Create(milestoneID, pageID, createdBy int) (int, error)
	Delete(id, deletedBy int) error
	ListHistory(milestoneID int) ([]models.MilestonePageLinkEvent, error)
}

// MilestonePageLinkService links pages to workspace milestones.
// Anyone who can view the milestone sees the linked pages they may view;
// users with edit rights on the milestone link and unlink pages from the
// milestone's own workspace. Global milestones have no page links. Each link
// and unlink is kept in the milestone history.
type MilestonePageLinkService struct {
	links  MilestonePageLinkStore
	access MilestonePageLinkAccess
	pages  PagePermissionChecker
}

// NewMilestonePageLinkService creates a MilestonePageLinkService. A nil page
// checker fails closed: no page is visible or linkable.
func NewMilestonePageLinkService(links MilestonePageLinkStore, access MilestonePageLinkAccess, pages PagePermissionChecker) *MilestonePageLinkService {
	return &MilestonePageLinkService{links: links, access: access, pages: pages}
}

// List returns the milestone's linked pages that the user may view, oldest
// link first. Global milestones always return an empty list.
func (s *MilestonePageLinkService) List(userID, milestoneID int) ([]models.MilestonePageLink, error) {
	milestone, err := s.access.AuthorizeMilestoneRead(userID, milestoneID)
	if err != nil {
		return nil, err
	}
	result := []models.MilestonePageLink{}
	if milestone.IsGlobal || milestone.WorkspaceID == nil || s.pages == nil {
		return result, nil
	}
	workspaceID := *milestone.WorkspaceID
	rows, err := s.links.ListByMilestone(milestoneID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return result, nil
	}
	pageIDs := make([]int, 0, len(rows))
	for _, row := range rows {
		pageIDs = append(pageIDs, row.PageID)
	}
	visible, err := s.pages.ListVisiblePageIDs(userID, workspaceID, pageIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.WorkspaceID == workspaceID && visible[row.PageID] {
			result = append(result, row)
		}
	}
	return result, nil
}

// History returns the milestone's recorded page links and unlinks for pages
// the user may view, oldest first. Archived, deleted, and hidden
// pages are left out; global milestones always return an empty list.
func (s *MilestonePageLinkService) History(userID, milestoneID int) ([]models.MilestonePageLinkEvent, error) {
	milestone, err := s.access.AuthorizeMilestoneRead(userID, milestoneID)
	if err != nil {
		return nil, err
	}
	result := []models.MilestonePageLinkEvent{}
	if milestone.IsGlobal || milestone.WorkspaceID == nil || s.pages == nil {
		return result, nil
	}
	workspaceID := *milestone.WorkspaceID
	events, err := s.links.ListHistory(milestoneID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return result, nil
	}
	pageIDs := make([]int, 0, len(events))
	for _, event := range events {
		pageIDs = append(pageIDs, event.PageID)
	}
	visible, err := s.pages.ListVisiblePageIDs(userID, workspaceID, pageIDs)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		if event.WorkspaceID == workspaceID && visible[event.PageID] {
			result = append(result, event)
		}
	}
	return result, nil
}

// Create links a page from the milestone's workspace to the milestone.
func (s *MilestonePageLinkService) Create(userID, milestoneID, pageID int) (*models.MilestonePageLink, error) {
	workspaceID, err := s.requireEditable(userID, milestoneID)
	if err != nil {
		return nil, err
	}
	if !s.pageVisible(userID, workspaceID, pageID) {
		return nil, ErrMilestonePageNotFound
	}
	id, err := s.links.Create(milestoneID, pageID, userID)
	if err != nil {
		return nil, err
	}
	return s.links.GetByID(id)
}

// Delete unlinks a page from the milestone. A link on another milestone, or
// to a page the user cannot view, reads as not found.
func (s *MilestonePageLinkService) Delete(userID, milestoneID, linkID int) error {
	workspaceID, err := s.requireEditable(userID, milestoneID)
	if err != nil {
		return err
	}
	link, err := s.links.GetByID(linkID)
	if err != nil {
		return err
	}
	if link.MilestoneID != milestoneID || !s.pageVisible(userID, workspaceID, link.PageID) {
		return fmt.Errorf("milestone page link %d not on milestone %d: %w", linkID, milestoneID, repository.ErrNotFound)
	}
	return s.links.Delete(linkID, userID)
}

// requireEditable confirms the user can view the milestone, that it is a
// workspace milestone, and that the user may edit it. It returns the
// milestone's workspace ID.
func (s *MilestonePageLinkService) requireEditable(userID, milestoneID int) (int, error) {
	milestone, err := s.access.AuthorizeMilestoneRead(userID, milestoneID)
	if err != nil {
		return 0, err
	}
	if milestone.IsGlobal || milestone.WorkspaceID == nil {
		return 0, ErrMilestonePageLinksGlobal
	}
	if _, err := s.access.AuthorizeMilestoneWrite(userID, milestoneID); err != nil {
		return 0, err
	}
	return *milestone.WorkspaceID, nil
}

// pageVisible reports whether the page exists in the workspace and the user
// may view it. Errors fail closed.
func (s *MilestonePageLinkService) pageVisible(userID, workspaceID, pageID int) bool {
	if s.pages == nil || pageID <= 0 {
		return false
	}
	ok, err := s.pages.Can(userID, workspaceID, pageID, PageOpView)
	return err == nil && ok
}
