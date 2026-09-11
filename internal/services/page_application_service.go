package services

import (
	"encoding/json"
	"errors"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ErrPageMutationForbidden is returned when a caller can address the
// workspace but lacks a workspace-level permission required by a page
// mutation. HTTP adapters existence-mask it; tools may render an explicit
// permission denial.
var ErrPageMutationForbidden = errors.New("page mutation forbidden")

// ErrPageParentNotFound masks a missing or inaccessible destination parent
// while allowing non-HTTP adapters to keep their more specific message.
var ErrPageParentNotFound = errors.New("parent page not found")

// ErrPageNoChanges is returned when a partial update supplies no mutable
// fields.
var ErrPageNoChanges = errors.New("no page fields to update")

// PageApplicationUpdateInput is the transport-neutral partial page-update
// shape. Nil fields retain their persisted values.
type PageApplicationUpdateInput struct {
	ID                  int
	Title               *string
	Content             *string
	Metadata            *json.RawMessage
	ExpectedContentHash *string
}

// PageApplicationService owns the permission-aware page mutation pipeline
// shared by cookie HTTP, REST v1, and agent tools. PageService remains the
// transaction/domain implementation; this layer composes authorization,
// partial-update behavior, and post-commit audit emission.
type PageApplicationService struct {
	pages    *PageService
	pageAuth *PagePermissionService
}

type PagePermissionsResult struct {
	PageID             int                     `json:"page_id"`
	InheritPermissions bool                    `json:"inherit_permissions"`
	EffectiveLevel     string                  `json:"effective_level"`
	ACL                []models.PagePermission `json:"acl"`
}

func (s *PageApplicationService) List(userID, workspaceID int) ([]models.Page, error) {
	pages, err := s.pages.ListTreeMeta(workspaceID, false)
	if err != nil {
		return nil, err
	}
	ids := make([]int, len(pages))
	for i := range pages {
		ids[i] = pages[i].ID
	}
	visible, err := s.pageAuth.ListVisiblePageIDs(userID, workspaceID, ids)
	if err != nil {
		return nil, err
	}
	filtered := make([]models.Page, 0, len(pages))
	for i := range pages {
		if visible[pages[i].ID] {
			filtered = append(filtered, pages[i])
		}
	}
	if err := s.pages.PreloadLabels(filtered); err != nil {
		return nil, err
	}
	return filtered, nil
}

func (s *PageApplicationService) Get(userID, workspaceID, pageID int) (*models.Page, error) {
	if err := s.requirePageOp(userID, workspaceID, pageID, PageOpView); err != nil {
		return nil, err
	}
	page, err := s.pages.GetByID(pageID)
	if err != nil || page.WorkspaceID != workspaceID {
		if err == nil {
			err = ErrPageNotFound
		}
		return nil, err
	}
	if err := s.pages.PreloadLabelsForPage(page); err != nil {
		return nil, err
	}
	return page, nil
}

func (s *PageApplicationService) Search(userID, workspaceID int, query string, limit int) ([]models.Page, error) {
	pages, err := s.pages.SearchByKeyword(workspaceID, query, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]int, len(pages))
	for i := range pages {
		ids[i] = pages[i].ID
	}
	visible, err := s.pageAuth.ListVisiblePageIDs(userID, workspaceID, ids)
	if err != nil {
		return nil, err
	}
	result := make([]models.Page, 0, len(pages))
	for i := range pages {
		if visible[pages[i].ID] {
			pages[i].Content = ""
			result = append(result, pages[i])
		}
	}
	return result, nil
}

func (s *PageApplicationService) ListArchived(userID, workspaceID int) ([]repository.ArchivedPageRow, error) {
	admin, err := s.pageAuth.IsSystemAdmin(userID)
	if err != nil {
		return nil, err
	}
	if !admin {
		admin, err = s.pageAuth.HasWorkspacePermissionFor(userID, workspaceID, models.PermissionWorkspaceAdmin)
		if err != nil {
			return nil, err
		}
	}
	if !admin {
		return nil, ErrPageNotFound
	}
	return s.pages.ListArchived(workspaceID)
}

func (s *PageApplicationService) ListHistory(userID, workspaceID, pageID, limit, offset int) ([]models.PageRevision, int, error) {
	if err := s.requirePageOp(userID, workspaceID, pageID, PageOpView); err != nil {
		return nil, 0, err
	}
	total, err := s.pages.CountRevisions(pageID)
	if err != nil {
		return nil, 0, err
	}
	revisions, err := s.pages.ListRevisions(pageID, limit, offset)
	return revisions, total, err
}

func (s *PageApplicationService) GetRevision(userID, workspaceID, pageID, revisionID int) (*models.PageRevision, error) {
	if err := s.requirePageOp(userID, workspaceID, pageID, PageOpView); err != nil {
		return nil, err
	}
	revision, err := s.pages.GetRevision(revisionID)
	if err != nil || revision.PageID != pageID {
		if err == nil {
			err = ErrPageNotFound
		}
		return nil, err
	}
	return revision, nil
}

func (s *PageApplicationService) GetPermissions(userID, workspaceID, pageID int) (PagePermissionsResult, error) {
	page, err := s.Get(userID, workspaceID, pageID)
	if err != nil {
		return PagePermissionsResult{}, err
	}
	effective := ""
	for _, op := range []string{PageOpAdmin, PageOpEdit, PageOpView} {
		allowed, checkErr := s.pageAuth.Can(userID, workspaceID, pageID, op)
		if checkErr != nil {
			return PagePermissionsResult{}, checkErr
		}
		if allowed {
			effective = op
			break
		}
	}
	acl, err := s.pages.ListOwnACL(pageID)
	if err != nil {
		return PagePermissionsResult{}, err
	}
	if acl == nil {
		acl = []models.PagePermission{}
	}
	return PagePermissionsResult{PageID: page.ID, InheritPermissions: page.InheritPermissions, EffectiveLevel: effective, ACL: acl}, nil
}

// NewPageApplicationService constructs the shared page mutation pipeline.
func NewPageApplicationService(pages *PageService, pageAuth *PagePermissionService) *PageApplicationService {
	return &PageApplicationService{pages: pages, pageAuth: pageAuth}
}

// PageService returns the underlying domain service for read-only adapters.
func (s *PageApplicationService) PageService() *PageService {
	return s.pages
}

// Create validates workspace/parent permissions, creates the page, and emits
// one committed audit row.
func (s *PageApplicationService) Create(actor AuditActor, in CreatePageInput) (*models.Page, error) {
	allowed, err := s.canCreate(actor.UserID, in.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPageMutationForbidden
	}
	if in.ParentID != nil {
		if err := s.requirePageOp(actor.UserID, in.WorkspaceID, *in.ParentID, PageOpEdit); err != nil {
			if errors.Is(err, ErrPageNotFound) {
				return nil, ErrPageParentNotFound
			}
			return nil, err
		}
	}
	page, err := s.pages.Create(actor.UserID, in)
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionPageCreate, page.ID, page.Title, nil)
	return page, nil
}

// Update applies a partial title/content/metadata update after page.edit.
func (s *PageApplicationService) Update(actor AuditActor, workspaceID int, in PageApplicationUpdateInput) (*models.Page, error) {
	if in.Title == nil && in.Content == nil && in.Metadata == nil {
		return nil, ErrPageNoChanges
	}
	if err := s.requirePageOp(actor.UserID, workspaceID, in.ID, PageOpEdit); err != nil {
		return nil, err
	}
	existing, err := s.pages.GetByID(in.ID)
	if err != nil {
		return nil, err
	}
	title := existing.Title
	content := existing.Content
	if in.Title != nil {
		title = *in.Title
	}
	if in.Content != nil {
		content = *in.Content
	}
	updated, err := s.pages.Update(actor.UserID, UpdatePageInput{
		ID:                  in.ID,
		Title:               title,
		Content:             content,
		Metadata:            in.Metadata,
		ExpectedContentHash: in.ExpectedContentHash,
	})
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionPageUpdate, updated.ID, updated.Title, nil)
	return updated, nil
}

// Move reparents/reorders a page after checking edit access on both the page
// and its destination parent. A nil destinationWorkspaceID preserves the
// existing same-workspace contract.
func (s *PageApplicationService) Move(actor AuditActor, workspaceID, pageID int, destinationWorkspaceID, parentID, prevSiblingID, nextSiblingID *int) (*models.Page, error) {
	if err := s.requirePageOp(actor.UserID, workspaceID, pageID, PageOpEdit); err != nil {
		return nil, err
	}
	destinationID := workspaceID
	if destinationWorkspaceID != nil {
		destinationID = *destinationWorkspaceID
	}
	crossWorkspace := destinationID != workspaceID
	if crossWorkspace {
		allowed, err := s.canCreate(actor.UserID, destinationID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrPageMutationForbidden
		}
	}
	if parentID != nil {
		if err := s.requirePageOp(actor.UserID, destinationID, *parentID, PageOpEdit); err != nil {
			if errors.Is(err, ErrPageNotFound) {
				return nil, ErrPageParentNotFound
			}
			return nil, err
		}
	}
	var moved *models.Page
	var err error
	if crossWorkspace {
		moved, err = s.pages.MoveAcrossWorkspace(actor.UserID, pageID, destinationID, parentID, prevSiblingID, nextSiblingID)
	} else {
		moved, err = s.pages.Move(actor.UserID, pageID, parentID, prevSiblingID, nextSiblingID)
	}
	if err != nil {
		return nil, err
	}
	details := map[string]any{
		"source_workspace_id":      workspaceID,
		"destination_workspace_id": destinationID,
	}
	s.emitAudit(actor, logger.ActionPageMove, moved.ID, moved.Title, details)
	return moved, nil
}

// Archive checks root admin, workspace delete, and every live descendant
// against the same locked subtree that PageService archives.
func (s *PageApplicationService) Archive(actor AuditActor, workspaceID, pageID int) (*models.Page, error) {
	if err := s.requirePageOp(actor.UserID, workspaceID, pageID, PageOpAdmin); err != nil {
		return nil, err
	}
	hasDelete, err := s.pageAuth.HasWorkspacePermissionFor(actor.UserID, workspaceID, models.PermissionPageDelete)
	if err != nil {
		return nil, err
	}
	if !hasDelete {
		return nil, ErrPageMutationForbidden
	}
	page, err := s.pages.GetByID(pageID)
	if err != nil {
		return nil, err
	}
	if err := s.pages.ArchiveChecked(actor.UserID, pageID, func(subtree []models.Page) error {
		for i := range subtree {
			if err := s.requirePageOp(actor.UserID, workspaceID, subtree[i].ID, PageOpAdmin); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionPageArchive, page.ID, page.Title, nil)
	return page, nil
}

// Restore applies a revision after the page restore policy succeeds.
func (s *PageApplicationService) Restore(actor AuditActor, workspaceID, pageID, revisionID int) (*models.Page, error) {
	if err := s.requirePageOp(actor.UserID, workspaceID, pageID, PageOpRestore); err != nil {
		return nil, err
	}
	restored, err := s.pages.Restore(actor.UserID, pageID, revisionID)
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionPageRestore, restored.ID, restored.Title, map[string]any{"revision_id": revisionID})
	return restored, nil
}

// GrantPermission adds an ACL row after page.admin.
func (s *PageApplicationService) GrantPermission(actor AuditActor, workspaceID, pageID int, principalType string, principalID int, level string) (*models.PagePermission, error) {
	if err := s.requirePageOp(actor.UserID, workspaceID, pageID, PageOpAdmin); err != nil {
		return nil, err
	}
	row, err := s.pages.GrantPermission(actor.UserID, pageID, principalType, principalID, level)
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionPagePermissionGrant, pageID, "", map[string]any{
		"permission_id":    row.ID,
		"principal_type":   row.PrincipalType,
		"principal_id":     row.PrincipalID,
		"permission_level": row.PermissionLevel,
	})
	return row, nil
}

// RevokePermission removes an ACL row after page.admin.
func (s *PageApplicationService) RevokePermission(actor AuditActor, workspaceID, pageID, permissionID int) error {
	if err := s.requirePageOp(actor.UserID, workspaceID, pageID, PageOpAdmin); err != nil {
		return err
	}
	if err := s.pages.RevokePermission(actor.UserID, pageID, permissionID); err != nil {
		return err
	}
	s.emitAudit(actor, logger.ActionPagePermissionRevoke, pageID, "", map[string]any{"permission_id": permissionID})
	return nil
}

// SetInheritance changes the ACL inheritance flag after page.admin.
func (s *PageApplicationService) SetInheritance(actor AuditActor, workspaceID, pageID int, inherit bool) (*models.Page, error) {
	if err := s.requirePageOp(actor.UserID, workspaceID, pageID, PageOpAdmin); err != nil {
		return nil, err
	}
	page, err := s.pages.SetInheritPermissions(actor.UserID, pageID, inherit)
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionPageInheritanceSet, page.ID, page.Title, map[string]any{"inherit_permissions": inherit})
	return page, nil
}

// Unarchive restores one page after the archived-page restore policy succeeds.
func (s *PageApplicationService) Unarchive(actor AuditActor, workspaceID, pageID int) (*models.Page, error) {
	if err := s.requirePageOp(actor.UserID, workspaceID, pageID, PageOpRestore); err != nil {
		return nil, err
	}
	page, err := s.pages.Unarchive(actor.UserID, pageID)
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionPageUnarchive, page.ID, page.Title, nil)
	return page, nil
}

func (s *PageApplicationService) canCreate(userID, workspaceID int) (bool, error) {
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

func (s *PageApplicationService) requirePageOp(userID, workspaceID, pageID int, op string) error {
	allowed, err := s.pageAuth.Can(userID, workspaceID, pageID, op)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPageNotFound
	}
	return nil
}

func (s *PageApplicationService) emitAudit(actor AuditActor, action string, pageID int, pageTitle string, extra map[string]any) {
	id := pageID
	_ = logger.LogAudit(s.pages.db, logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		ActionType:   action,
		ResourceType: logger.ResourcePage,
		ResourceID:   &id,
		ResourceName: pageTitle,
		Details:      mergeAuditDetails(extra, actor),
		Success:      true,
	})
}
