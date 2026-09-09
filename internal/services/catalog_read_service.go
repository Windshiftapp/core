package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

var (
	ErrCatalogNotFound  = errors.New("catalog resource not found")
	ErrCatalogForbidden = errors.New("catalog operation forbidden")
)

// CatalogAccess supplies principal-aware decisions without coupling services to HTTP authentication.
type CatalogAccess interface {
	CanViewWorkspace(userID, workspaceID int) (bool, error)
	CanAdminWorkspace(userID, workspaceID int) (bool, error)
	HasGlobalPermission(userID int, permission string) (bool, error)
}

// CatalogReadService owns the authorization and composition shared by API catalog readers.
type CatalogReadService struct {
	workspaces     *WorkspaceService
	workspaceRepo  *repository.WorkspaceRepository
	workflows      *WorkflowService
	statuses       *StatusService
	users          *UserReadService
	workspaceUsers *WorkspaceUserResolver
	labels         *LabelApplicationService
	templates      *ItemTemplateService
	activity       *ActivityTracker
	access         CatalogAccess
}

func NewCatalogReadService(db database.Database, permissions *PermissionService, access CatalogAccess, activity *ActivityTracker) *CatalogReadService {
	return &CatalogReadService{
		workspaces: NewWorkspaceService(db), workspaceRepo: repository.NewWorkspaceRepository(db),
		workflows: NewWorkflowService(db), statuses: NewStatusService(db), users: NewUserReadService(db),
		workspaceUsers: NewWorkspaceUserResolver(db, permissions), labels: NewLabelApplicationService(db),
		templates: NewItemTemplateService(db), activity: activity, access: access,
	}
}

type CatalogPageParams struct {
	Limit  int
	Offset int
	Sort   string
	Desc   bool
}

func (s *CatalogReadService) ListWorkspaces(userID int, page CatalogPageParams) ([]models.Workspace, int, error) {
	candidates, err := s.workspaceRepo.FindAll(userID, false)
	if err != nil {
		return nil, 0, err
	}
	visible := make([]models.Workspace, 0, len(candidates))
	for _, workspace := range candidates {
		allowed, err := s.workspaceVisible(userID, &workspace)
		if err != nil {
			return nil, 0, err
		}
		if allowed {
			visible = append(visible, workspace)
		}
	}
	slices.SortStableFunc(visible, func(a, b models.Workspace) int {
		var order int
		switch page.Sort {
		case "key":
			order = strings.Compare(strings.ToLower(a.Key), strings.ToLower(b.Key))
		case "created_at":
			order = a.CreatedAt.Compare(b.CreatedAt)
		default:
			order = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
		if page.Desc {
			order = -order
		}
		if order == 0 {
			order = a.ID - b.ID
		}
		return order
	})
	return pageSlice(visible, page), len(visible), nil
}

func (s *CatalogReadService) ListWorkspaceTemplates(ctx context.Context, userID int) ([]models.WorkspaceTemplateSummary, error) {
	items, err := s.workspaces.ListTemplateSummaries(ctx)
	if err != nil {
		return nil, err
	}
	visible := make([]models.WorkspaceTemplateSummary, 0, len(items))
	for _, item := range items {
		allowed, err := s.access.CanViewWorkspace(userID, item.ID)
		if err != nil {
			return nil, err
		}
		if allowed {
			visible = append(visible, item)
		}
	}
	slices.SortStableFunc(visible, func(a, b models.WorkspaceTemplateSummary) int {
		if order := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); order != 0 {
			return order
		}
		return a.ID - b.ID
	})
	return visible, nil
}

func (s *CatalogReadService) GetWorkspace(userID, workspaceID int) (*models.Workspace, error) {
	workspace, err := s.workspaces.GetByID(workspaceID)
	if err != nil {
		return nil, catalogReadError(err)
	}
	allowed, err := s.workspaceVisible(userID, workspace)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrCatalogNotFound
	}
	return workspace, nil
}

// VisitWorkspace returns a visible workspace and records the user-facing visit.
func (s *CatalogReadService) VisitWorkspace(userID, workspaceID int) (*models.Workspace, error) {
	workspace, err := s.GetWorkspace(userID, workspaceID)
	if err != nil {
		return nil, err
	}
	if s.activity != nil {
		if err := s.activity.TrackWorkspaceVisit(userID, workspaceID); err != nil {
			slog.Warn("failed to track workspace visit", "user_id", userID, "workspace_id", workspaceID, "error", err)
		}
	}
	return workspace, nil
}

func (s *CatalogReadService) ListWorkspaceStatuses(userID, workspaceID int, itemTypeID *int) ([]StatusResult, error) {
	if _, err := s.GetWorkspace(userID, workspaceID); err != nil {
		return nil, err
	}
	if itemTypeID == nil {
		statusModels, err := s.workspaces.GetStatuses(workspaceID)
		if err != nil {
			return nil, err
		}
		items := make([]StatusResult, len(statusModels))
		for i := range statusModels {
			items[i] = statusResult(statusModels[i])
		}
		return items, nil
	}
	if err := s.requireEffectiveItemType(workspaceID, *itemTypeID); err != nil {
		return nil, err
	}
	workflowID, err := s.workflows.GetWorkflowIDForItem(workspaceID, itemTypeID)
	if err != nil {
		return nil, err
	}
	if workflowID == nil {
		workflowID, err = s.workflows.GetDefaultWorkflowID()
		if err != nil {
			return nil, err
		}
	}
	if workflowID == nil {
		return []StatusResult{}, nil
	}
	return s.statuses.ListWorkflowStatuses(*workflowID)
}

func (s *CatalogReadService) ListWorkspaceItemTypes(userID, workspaceID int) ([]ItemTypeResult, error) {
	if _, err := s.GetWorkspace(userID, workspaceID); err != nil {
		return nil, err
	}
	return s.workspaces.GetItemTypes(workspaceID)
}

func (s *CatalogReadService) ListWorkspaceWorkflows(userID, workspaceID int) ([]WorkflowResult, error) {
	if _, err := s.GetWorkspace(userID, workspaceID); err != nil {
		return nil, err
	}
	return s.workflows.ListForWorkspace(workspaceID)
}

func (s *CatalogReadService) ListWorkspacePriorities(userID, workspaceID int) ([]PriorityResult, error) {
	if _, err := s.GetWorkspace(userID, workspaceID); err != nil {
		return nil, err
	}
	return s.workspaces.GetPriorities(workspaceID)
}

func (s *CatalogReadService) ListAssignableUsers(ctx context.Context, userID, workspaceID int) ([]models.User, error) {
	if _, err := s.GetWorkspace(userID, workspaceID); err != nil {
		return nil, err
	}
	return s.workspaceUsers.List(ctx, workspaceID)
}

func (s *CatalogReadService) ListUsers(userID int, page CatalogPageParams) ([]models.User, int, error) {
	allowed, err := s.access.HasGlobalPermission(userID, models.PermissionUserList)
	if err != nil {
		return nil, 0, err
	}
	if !allowed {
		return nil, 0, ErrCatalogForbidden
	}
	users, err := s.users.ListAll()
	if err != nil {
		return nil, 0, err
	}
	slices.SortStableFunc(users, func(a, b models.User) int {
		var order int
		switch page.Sort {
		case "full_name":
			order = strings.Compare(strings.ToLower(a.FullName), strings.ToLower(b.FullName))
		case "created_at":
			order = a.CreatedAt.Compare(b.CreatedAt)
		default:
			order = strings.Compare(strings.ToLower(a.Username), strings.ToLower(b.Username))
		}
		if page.Desc {
			order = -order
		}
		if order == 0 {
			order = a.ID - b.ID
		}
		return order
	})
	return pageSlice(users, page), len(users), nil
}

func (s *CatalogReadService) GetUser(userID, targetID int) (*models.User, error) {
	allowed, err := s.access.HasGlobalPermission(userID, models.PermissionUserList)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrCatalogForbidden
	}
	user, err := s.users.GetByID(targetID)
	if err != nil {
		return nil, catalogReadError(err)
	}
	if !user.IsActive {
		return nil, ErrCatalogNotFound
	}
	return user, nil
}

func (s *CatalogReadService) ListLabels(userID, workspaceID int) ([]models.Label, error) {
	if _, err := s.GetWorkspace(userID, workspaceID); err != nil {
		return nil, err
	}
	labels, err := s.labels.List()
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(labels, func(a, b models.Label) int {
		if order := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); order != 0 {
			return order
		}
		return a.ID - b.ID
	})
	return labels, nil
}

func (s *CatalogReadService) GetLabel(userID, workspaceID, labelID int) (*models.Label, error) {
	if _, err := s.GetWorkspace(userID, workspaceID); err != nil {
		return nil, err
	}
	label, err := s.labels.Get(labelID)
	if err != nil {
		return nil, catalogReadError(err)
	}
	return label, nil
}

type ItemTemplateListResult struct {
	Items               []models.ItemTemplate
	MandatoryTemplateID *int
}

func (s *CatalogReadService) ListItemTemplates(userID, workspaceID int, itemTypeID *int) (ItemTemplateListResult, error) {
	if _, err := s.GetWorkspace(userID, workspaceID); err != nil {
		return ItemTemplateListResult{}, err
	}
	if itemTypeID != nil {
		if err := s.requireEffectiveItemType(workspaceID, *itemTypeID); err != nil {
			return ItemTemplateListResult{}, err
		}
	}
	items, err := s.templates.List(workspaceID, itemTypeID)
	if err != nil {
		return ItemTemplateListResult{}, err
	}
	slices.SortStableFunc(items, func(a, b models.ItemTemplate) int {
		if order := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); order != 0 {
			return order
		}
		return a.ID - b.ID
	})
	result := ItemTemplateListResult{Items: items}
	if itemTypeID != nil {
		mandatory, err := s.templates.GetMandatory(workspaceID, *itemTypeID)
		if err == nil {
			result.MandatoryTemplateID = &mandatory.ID
		} else if !errors.Is(err, repository.ErrNotFound) {
			return ItemTemplateListResult{}, err
		}
	}
	return result, nil
}

func (s *CatalogReadService) GetItemTemplate(userID, workspaceID, templateID int) (*models.ItemTemplate, error) {
	if _, err := s.GetWorkspace(userID, workspaceID); err != nil {
		return nil, err
	}
	item, err := s.templates.Get(templateID)
	if err != nil || item.WorkspaceID != workspaceID {
		return nil, ErrCatalogNotFound
	}
	return item, nil
}

func (s *CatalogReadService) workspaceVisible(userID int, workspace *models.Workspace) (bool, error) {
	if workspace.Active {
		return s.access.CanViewWorkspace(userID, workspace.ID)
	}
	return s.access.CanAdminWorkspace(userID, workspace.ID)
}

func (s *CatalogReadService) requireEffectiveItemType(workspaceID, itemTypeID int) error {
	items, err := s.workspaces.GetItemTypes(workspaceID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == itemTypeID {
			return nil
		}
	}
	return ErrCatalogNotFound
}

func catalogReadError(err error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrCatalogNotFound
	}
	return err
}

func pageSlice[T any](items []T, page CatalogPageParams) []T {
	start := page.Offset
	if start > len(items) {
		start = len(items)
	}
	end := start + page.Limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func (s *CatalogReadService) Validate() error {
	if s == nil || s.access == nil {
		return fmt.Errorf("catalog read service is not configured")
	}
	return nil
}
