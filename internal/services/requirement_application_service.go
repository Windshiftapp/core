package services

import (
	"errors"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// RequirementView is the application-level response envelope for requirements.
type RequirementView struct {
	models.Requirement
	Key             string `json:"key"`
	PageTitle       string `json:"page_title"`
	PageID          int    `json:"page_id"`
	LinkedItemCount int    `json:"linked_item_count"`
	LinkedTestCount int    `json:"linked_test_count"`
	IsTestCovered   bool   `json:"is_test_covered"`
}

// RequirementKeyView is a minimal requirement identity for navigation maps.
type RequirementKeyView struct {
	PageID            int    `json:"page_id"`
	RequirementNumber int    `json:"requirement_number"`
	Key               string `json:"key"`
}

// RequirementApplicationService composes requirement domain operations with
// page ACL checks and audit emission.
type RequirementApplicationService struct {
	requirements *RequirementService
	pages        *PageService
	pageAuth     *PagePermissionService
	workspaces   *repository.WorkspaceRepository
	auditor      *logger.Auditor
}

// NewRequirementApplicationService creates a RequirementApplicationService.
func NewRequirementApplicationService(
	requirements *RequirementService,
	pages *PageService,
	pageAuth *PagePermissionService,
	workspaces *repository.WorkspaceRepository,
	auditor *logger.Auditor,
) *RequirementApplicationService {
	return &RequirementApplicationService{
		requirements: requirements,
		pages:        pages,
		pageAuth:     pageAuth,
		workspaces:   workspaces,
		auditor:      auditor,
	}
}

// List returns visible requirements in a workspace and the ACL-aware total for the filter.
func (s *RequirementApplicationService) List(userID, workspaceID int, filter RequirementListFilter) ([]RequirementView, int, error) {
	visiblePageIDs, err := s.visiblePageIDsForFilter(userID, workspaceID, filter)
	if err != nil {
		return nil, 0, err
	}
	total := len(visiblePageIDs)
	if total == 0 {
		return []RequirementView{}, 0, nil
	}

	start := filter.Offset
	if start > total {
		start = total
	}
	end := total
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	pageSlice := visiblePageIDs[start:end]
	if len(pageSlice) == 0 {
		return []RequirementView{}, total, nil
	}

	rows, err := s.requirements.ListByPageIDs(workspaceID, pageSlice)
	if err != nil {
		return nil, 0, err
	}
	workspaceKey, err := s.workspaces.GetKey(workspaceID)
	if err != nil {
		return nil, 0, err
	}

	views := make([]RequirementView, 0, len(rows))
	for _, row := range rows {
		views = append(views, requirementViewFromRow(row, workspaceKey))
	}
	return views, total, nil
}

// ListKeys returns visible requirement keys for navigation maps.
func (s *RequirementApplicationService) ListKeys(userID, workspaceID int) ([]RequirementKeyView, error) {
	rows, err := s.requirements.ListKeys(workspaceID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []RequirementKeyView{}, nil
	}

	pageIDs := make([]int, len(rows))
	for i := range rows {
		pageIDs[i] = rows[i].PageID
	}
	visible, err := s.pageAuth.ListVisiblePageIDs(userID, workspaceID, pageIDs)
	if err != nil {
		return nil, err
	}
	workspaceKey, err := s.workspaces.GetKey(workspaceID)
	if err != nil {
		return nil, err
	}

	out := make([]RequirementKeyView, 0, len(rows))
	for _, row := range rows {
		if !visible[row.PageID] {
			continue
		}
		out = append(out, RequirementKeyView{
			PageID:            row.PageID,
			RequirementNumber: row.RequirementNumber,
			Key:               models.FormatRequirementKey(workspaceKey, row.RequirementNumber),
		})
	}
	return out, nil
}

func (s *RequirementApplicationService) visiblePageIDsForFilter(userID, workspaceID int, filter RequirementListFilter) ([]int, error) {
	pageIDs, err := s.requirements.ListPageIDs(workspaceID, filter)
	if err != nil {
		return nil, err
	}
	if len(pageIDs) == 0 {
		return nil, nil
	}
	visible, err := s.pageAuth.ListVisiblePageIDs(userID, workspaceID, pageIDs)
	if err != nil {
		return nil, err
	}
	out := make([]int, 0, len(pageIDs))
	for _, pageID := range pageIDs {
		if visible[pageID] {
			out = append(out, pageID)
		}
	}
	return out, nil
}

// GetByPage returns the requirement backing a page when the caller can view it.
func (s *RequirementApplicationService) GetByPage(userID, workspaceID, pageID int) (*RequirementView, error) {
	if err := s.requirePageOp(userID, workspaceID, pageID, PageOpView); err != nil {
		return nil, err
	}
	req, err := s.requirements.GetByPageID(pageID)
	if err != nil {
		return nil, err
	}
	if req.WorkspaceID != workspaceID {
		return nil, ErrRequirementNotFound
	}
	return s.buildView(req)
}

// Get returns one requirement when the caller can view its backing page.
func (s *RequirementApplicationService) Get(userID, workspaceID, number int) (*RequirementView, error) {
	req, err := s.requirements.GetByWorkspaceAndNumber(workspaceID, number)
	if err != nil {
		return nil, err
	}
	if err := s.requirePageOp(userID, workspaceID, req.PageID, PageOpView); err != nil {
		return nil, err
	}
	return s.buildView(req)
}

// Create validates page permissions, creates the requirement, and emits audit.
func (s *RequirementApplicationService) Create(actor AuditActor, in CreateRequirementInput) (*RequirementView, error) {
	allowed, err := s.canCreate(actor.UserID, in.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrRequirementNotFound
	}
	if in.ParentID != nil {
		if err := s.requirePageOp(actor.UserID, in.WorkspaceID, *in.ParentID, PageOpEdit); err != nil {
			if errors.Is(err, ErrPageNotFound) {
				return nil, ErrRequirementNotFound
			}
			return nil, err
		}
	}

	req, err := s.requirements.Create(actor.UserID, in)
	if err != nil {
		return nil, err
	}
	view, err := s.buildView(req)
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionRequirementCreate, req.ID, view.Key, map[string]any{
		"requirement_number": req.RequirementNumber,
		"page_id":            req.PageID,
	})
	return view, nil
}

// Promote upgrades an existing page after page.edit on the backing page.
func (s *RequirementApplicationService) Promote(actor AuditActor, workspaceID, pageID int, requirementType, status string, ownerID *int) (*RequirementView, error) {
	if err := s.requirePageOp(actor.UserID, workspaceID, pageID, PageOpEdit); err != nil {
		return nil, err
	}
	req, err := s.requirements.Promote(actor.UserID, pageID, requirementType, status, ownerID)
	if err != nil {
		return nil, err
	}
	view, err := s.buildView(req)
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionRequirementPromote, req.ID, view.Key, map[string]any{
		"requirement_number": req.RequirementNumber,
		"page_id":            req.PageID,
	})
	return view, nil
}

// Update applies mutable metadata after page.edit on the backing page.
func (s *RequirementApplicationService) Update(actor AuditActor, workspaceID, number int, patch RequirementUpdateInput) (*RequirementView, error) {
	req, err := s.requirements.GetByWorkspaceAndNumber(workspaceID, number)
	if err != nil {
		return nil, err
	}
	if err := s.requirePageOp(actor.UserID, workspaceID, req.PageID, PageOpEdit); err != nil {
		return nil, err
	}

	updated, err := s.requirements.Update(actor.UserID, workspaceID, number, patch)
	if err != nil {
		return nil, err
	}
	view, err := s.buildView(updated)
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionRequirementUpdate, updated.ID, view.Key, map[string]any{
		"requirement_number": updated.RequirementNumber,
		"page_id":            updated.PageID,
	})
	return view, nil
}

// ListHistory returns metadata history when the caller can view the backing page.
func (s *RequirementApplicationService) ListHistory(userID, workspaceID, number int) ([]models.RequirementHistoryEntry, error) {
	req, err := s.requirements.GetByWorkspaceAndNumber(workspaceID, number)
	if err != nil {
		return nil, err
	}
	if err := s.requirePageOp(userID, workspaceID, req.PageID, PageOpView); err != nil {
		return nil, err
	}
	return s.requirements.ListHistory(req.ID)
}

func (s *RequirementApplicationService) buildView(req *models.Requirement) (*RequirementView, error) {
	page, err := s.pages.GetByID(req.PageID)
	if err != nil {
		return nil, err
	}
	workspaceKey, err := s.workspaces.GetKey(req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	counts, err := s.requirements.GetLinkCountsByPageID(req.PageID)
	if err != nil {
		return nil, err
	}
	return &RequirementView{
		Requirement:     *req,
		Key:             models.FormatRequirementKey(workspaceKey, req.RequirementNumber),
		PageTitle:       page.Title,
		PageID:          req.PageID,
		LinkedItemCount: counts.LinkedItemCount,
		LinkedTestCount: counts.LinkedTestCount,
		IsTestCovered:   counts.LinkedTestCount > 0,
	}, nil
}

func requirementViewFromRow(row repository.RequirementListRow, workspaceKey string) RequirementView {
	return RequirementView{
		Requirement:     row.Requirement,
		Key:             models.FormatRequirementKey(workspaceKey, row.Requirement.RequirementNumber),
		PageTitle:       row.PageTitle,
		PageID:          row.Requirement.PageID,
		LinkedItemCount: row.LinkedItemCount,
		LinkedTestCount: row.LinkedTestCount,
		IsTestCovered:   row.LinkedTestCount > 0,
	}
}

func (s *RequirementApplicationService) canCreate(userID, workspaceID int) (bool, error) {
	for _, key := range []string{models.PermissionPageCreate, models.PermissionPageAdmin, models.PermissionWorkspaceAdmin} {
		has, err := s.pageAuth.HasWorkspacePermissionFor(userID, workspaceID, key)
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}

func (s *RequirementApplicationService) requirePageOp(userID, workspaceID, pageID int, op string) error {
	allowed, err := s.pageAuth.Can(userID, workspaceID, pageID, op)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRequirementNotFound
	}
	return nil
}

func (s *RequirementApplicationService) emitAudit(actor AuditActor, action string, requirementID int, resourceName string, extra map[string]any) {
	id := requirementID
	s.auditor.LogEvent(logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		ActionType:   action,
		ResourceType: logger.ResourceRequirement,
		ResourceID:   &id,
		ResourceName: resourceName,
		Details:      mergeAuditDetails(extra, actor),
		Success:      true,
	})
}
