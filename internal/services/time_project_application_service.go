package services

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	ErrTimeProjectForbidden = errors.New("time project operation forbidden")
	ErrTimeProjectConflict  = errors.New("time project conflict")
)

type TimeProjectValidationError struct{ Message string }

func (e *TimeProjectValidationError) Error() string { return e.Message }

type TimeProjectWorkspaceAccess interface {
	CanViewWorkspace(userID, workspaceID int) (bool, error)
}

type TimeProjectPatch struct {
	CustomerIDSet bool
	CustomerID    *int
	CategoryIDSet bool
	CategoryID    *int
	Name          *string
	Description   *string
	Status        *string
	Color         *string
	HourlyRate    *float64
	SettingsSet   bool
	Settings      map[string]any
}

type TimeProjectCategoryPatch struct {
	Name         *string
	Description  *string
	Color        *string
	DisplayOrder *int
}

type TimeProjectOrder struct {
	ID           int
	DisplayOrder int
}

// TimeProjectApplicationService owns time-project policy shared by HTTP surfaces.
type TimeProjectApplicationService struct {
	db          database.Database
	projects    *repository.TimeProjectRepository
	categories  *repository.TimeProjectCategoryRepository
	customers   *repository.CustomerOrganisationRepository
	permissions *TimePermissionService
	workspaces  TimeProjectWorkspaceAccess
}

func NewTimeProjectApplicationService(db database.Database, permissions *TimePermissionService, workspaces TimeProjectWorkspaceAccess) *TimeProjectApplicationService {
	return &TimeProjectApplicationService{
		db: db, projects: repository.NewTimeProjectRepository(db),
		categories:  repository.NewTimeProjectCategoryRepository(db),
		customers:   repository.NewCustomerOrganisationRepository(db),
		permissions: permissions, workspaces: workspaces,
	}
}

func (s *TimeProjectApplicationService) List(userID int, status string) ([]repository.TimeProjectDetail, error) {
	accessible, err := s.permissions.GetAccessibleProjects(userID)
	if err != nil {
		return nil, err
	}
	return s.projects.ListDetails(accessible, strings.TrimSpace(status))
}

func (s *TimeProjectApplicationService) ListForWorkspace(userID, workspaceID int, status string) ([]repository.TimeProjectDetail, error) {
	allowed, err := s.workspaces.CanViewWorkspace(userID, workspaceID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, repository.ErrNotFound
	}
	accessible, err := s.permissions.GetAccessibleProjects(userID)
	if err != nil {
		return nil, err
	}
	categories, err := s.categories.ListIDsForWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	return s.projects.ListDetailsFiltered(repository.TimeProjectListFilter{
		AccessibleIDs: accessible, CategoryIDs: categories, Status: strings.TrimSpace(status),
	})
}

func (s *TimeProjectApplicationService) Get(userID, projectID int) (*repository.TimeProjectDetail, error) {
	allowed, err := s.permissions.CanViewProject(userID, projectID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, repository.ErrNotFound
	}
	return s.projects.GetDetail(projectID)
}

func (s *TimeProjectApplicationService) ListCategories() ([]models.TimeProjectCategory, error) {
	return s.categories.List()
}

func (s *TimeProjectApplicationService) GetCategory(id int) (*models.TimeProjectCategory, error) {
	return s.categories.FindByID(id)
}

func (s *TimeProjectApplicationService) CreateCategory(actor AuditActor, category models.TimeProjectCategory) (*models.TimeProjectCategory, error) {
	if err := s.requireCategoryManagement(actor.UserID); err != nil {
		return nil, err
	}
	sanitizeTimeProjectCategory(&category)
	if category.Name == "" {
		return nil, &TimeProjectValidationError{Message: "category name is required"}
	}
	if err := s.categories.Create(&category); err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrTimeProjectConflict
		}
		return nil, err
	}
	id := category.ID
	emitServiceAudit(s.db, actor, logger.ActionTimeCategoryCreate, logger.ResourceTimeCategory, &id, category.Name, nil)
	return &category, nil
}

func (s *TimeProjectApplicationService) PatchCategory(actor AuditActor, id int, patch TimeProjectCategoryPatch) (*models.TimeProjectCategory, error) {
	if err := s.requireCategoryManagement(actor.UserID); err != nil {
		return nil, err
	}
	category, err := s.categories.FindByID(id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		category.Name = *patch.Name
	}
	if patch.Description != nil {
		category.Description = *patch.Description
	}
	if patch.Color != nil {
		category.Color = *patch.Color
	}
	if patch.DisplayOrder != nil {
		category.DisplayOrder = *patch.DisplayOrder
	}
	sanitizeTimeProjectCategory(category)
	if category.Name == "" {
		return nil, &TimeProjectValidationError{Message: "category name is required"}
	}
	if err := s.categories.Update(id, category); err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrTimeProjectConflict
		}
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTimeCategoryUpdate, logger.ResourceTimeCategory, &id, category.Name, nil)
	return s.categories.FindByID(id)
}

func (s *TimeProjectApplicationService) DeleteCategory(actor AuditActor, id int) error {
	if err := s.requireCategoryManagement(actor.UserID); err != nil {
		return err
	}
	category, err := s.categories.FindByID(id)
	if err != nil {
		return err
	}
	count, err := s.categories.CountProjectsUsing(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: category is used by %d projects", ErrTimeProjectConflict, count)
	}
	if _, err := s.categories.Delete(id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionTimeCategoryDelete, logger.ResourceTimeCategory, &id, category.Name, nil)
	return nil
}

func (s *TimeProjectApplicationService) ReorderCategories(userID int, order []TimeProjectOrder) ([]models.TimeProjectCategory, error) {
	if err := s.requireCategoryManagement(userID); err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return nil, &TimeProjectValidationError{Message: "items are required"}
	}
	ids := make([]int, len(order))
	for i, item := range order {
		if item.ID <= 0 || item.DisplayOrder < 0 || slices.Contains(ids[:i], item.ID) {
			return nil, &TimeProjectValidationError{Message: "items must contain unique positive IDs and non-negative display_order values"}
		}
		if _, err := s.categories.FindByID(item.ID); err != nil {
			return nil, err
		}
		ids[i] = item.ID
	}
	err := database.WithTx(s.db, func(tx database.Tx) error {
		now := time.Now()
		for _, item := range order {
			if _, err := tx.ExecWrite("UPDATE time_project_categories SET display_order = ?, updated_at = ? WHERE id = ?", item.DisplayOrder, now, item.ID); err != nil {
				return fmt.Errorf("reorder time project category %d: %w", item.ID, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.categories.List()
}

func (s *TimeProjectApplicationService) CreateProject(actor AuditActor, project models.TimeProject) (*repository.TimeProjectDetail, error) {
	allowed, err := s.permissions.HasProjectManagePermission(actor.UserID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrTimeProjectForbidden
	}
	if project.Status == "" {
		project.Status = "Active"
	}
	if err := s.prepareProject(&project); err != nil {
		return nil, err
	}
	if err := s.projects.CreateWithManager(&project, actor.UserID); err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrTimeProjectConflict
		}
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTimeProjectCreate, logger.ResourceTimeProject, &project.ID, project.Name, nil)
	return s.projects.GetDetail(project.ID)
}

func (s *TimeProjectApplicationService) PatchProject(actor AuditActor, id int, patch TimeProjectPatch) (*repository.TimeProjectDetail, error) {
	allowed, err := s.permissions.IsTimeProjectManager(actor.UserID, id)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrTimeProjectForbidden
	}
	detail, err := s.projects.GetDetail(id)
	if err != nil {
		return nil, err
	}
	project := timeProjectModel(*detail)
	if patch.CustomerIDSet {
		project.CustomerID = patch.CustomerID
	}
	if patch.CategoryIDSet {
		project.CategoryID = patch.CategoryID
	}
	if patch.Name != nil {
		project.Name = *patch.Name
	}
	if patch.Description != nil {
		project.Description = *patch.Description
	}
	if patch.Status != nil {
		project.Status = *patch.Status
	}
	if patch.Color != nil {
		project.Color = *patch.Color
	}
	if patch.HourlyRate != nil {
		project.HourlyRate = *patch.HourlyRate
	}
	if patch.SettingsSet {
		project.Settings = patch.Settings
	}
	if err := s.prepareProject(&project); err != nil {
		return nil, err
	}
	if err := s.projects.Update(id, &project); err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrTimeProjectConflict
		}
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTimeProjectUpdate, logger.ResourceTimeProject, &id, project.Name, nil)
	return s.projects.GetDetail(id)
}

func (s *TimeProjectApplicationService) DeleteProject(actor AuditActor, id int) error {
	allowed, err := s.permissions.HasProjectManagePermission(actor.UserID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrTimeProjectForbidden
	}
	project, err := s.projects.GetDetail(id)
	if err != nil {
		return err
	}
	if err := s.projects.Delete(id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionTimeProjectDelete, logger.ResourceTimeProject, &id, project.Name, nil)
	return nil
}

func (s *TimeProjectApplicationService) ListManagers(userID, projectID int) ([]models.TimeProjectManager, error) {
	if err := s.requireProjectView(userID, projectID); err != nil {
		return nil, err
	}
	items, err := s.permissions.GetProjectManagers(projectID)
	if items == nil && err == nil {
		items = []models.TimeProjectManager{}
	}
	return items, err
}

func (s *TimeProjectApplicationService) AddManager(actor AuditActor, projectID int, input models.TimeProjectManagerRequest) (*models.TimeProjectManager, error) {
	if err := s.requireGrantAuthority(actor.UserID, projectID); err != nil {
		return nil, err
	}
	if input.ManagerType != "user" && input.ManagerType != "group" {
		return nil, &TimeProjectValidationError{Message: "manager_type must be 'user' or 'group'"}
	}
	manager, err := s.permissions.AddProjectManager(projectID, input.ManagerType, input.ManagerID, actor.UserID)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrTimeProjectConflict
		}
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTimeProjectAddManager, logger.ResourceTimeProject, &projectID, "", map[string]any{"manager_id": input.ManagerID})
	return manager, nil
}

func (s *TimeProjectApplicationService) RemoveManager(actor AuditActor, projectID, assignmentID int) error {
	if err := s.requireGrantAuthority(actor.UserID, projectID); err != nil {
		return err
	}
	items, err := s.permissions.GetProjectManagers(projectID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(items, func(item models.TimeProjectManager) bool { return item.ID == assignmentID }) {
		return repository.ErrNotFound
	}
	if err := s.permissions.RemoveProjectManager(assignmentID); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionTimeProjectRemoveManager, logger.ResourceTimeProject, &projectID, "", map[string]any{"manager_id": assignmentID})
	return nil
}

func (s *TimeProjectApplicationService) ListMembers(userID, projectID int) ([]models.TimeProjectMember, error) {
	if err := s.requireProjectView(userID, projectID); err != nil {
		return nil, err
	}
	items, err := s.permissions.GetProjectMembers(projectID)
	if items == nil && err == nil {
		items = []models.TimeProjectMember{}
	}
	return items, err
}

func (s *TimeProjectApplicationService) AddMember(actor AuditActor, projectID int, input models.TimeProjectMemberRequest) (*models.TimeProjectMember, error) {
	if err := s.requireGrantAuthority(actor.UserID, projectID); err != nil {
		return nil, err
	}
	if input.MemberType != "user" && input.MemberType != "group" {
		return nil, &TimeProjectValidationError{Message: "member_type must be 'user' or 'group'"}
	}
	member, err := s.permissions.AddProjectMember(projectID, input.MemberType, input.MemberID, actor.UserID)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrTimeProjectConflict
		}
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTimeProjectAddMember, logger.ResourceTimeProject, &projectID, "", map[string]any{"member_id": input.MemberID})
	return member, nil
}

func (s *TimeProjectApplicationService) RemoveMember(actor AuditActor, projectID, assignmentID int) error {
	if err := s.requireGrantAuthority(actor.UserID, projectID); err != nil {
		return err
	}
	items, err := s.permissions.GetProjectMembers(projectID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(items, func(item models.TimeProjectMember) bool { return item.ID == assignmentID }) {
		return repository.ErrNotFound
	}
	if err := s.permissions.RemoveProjectMember(assignmentID); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionTimeProjectRemoveMember, logger.ResourceTimeProject, &projectID, "", map[string]any{"member_id": assignmentID})
	return nil
}

func (s *TimeProjectApplicationService) requireCategoryManagement(userID int) error {
	allowed, err := s.permissions.HasCustomersManagePermission(userID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrTimeProjectForbidden
	}
	return nil
}

func (s *TimeProjectApplicationService) requireProjectView(userID, projectID int) error {
	allowed, err := s.permissions.CanViewProject(userID, projectID)
	if err != nil {
		return err
	}
	if !allowed {
		return repository.ErrNotFound
	}
	return nil
}

func (s *TimeProjectApplicationService) requireGrantAuthority(userID, projectID int) error {
	allowed, err := s.permissions.CanGrantProjectAccess(userID, projectID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrTimeProjectForbidden
	}
	return nil
}

func (s *TimeProjectApplicationService) prepareProject(project *models.TimeProject) error {
	project.Name = sanitize.PlainTextField.Sanitize(project.Name)
	project.Description = sanitize.RichText.Sanitize(project.Description)
	project.Color = sanitize.ShortIdentifier.Sanitize(project.Color)
	if project.Name == "" {
		return &TimeProjectValidationError{Message: "project name is required"}
	}
	if project.CustomerID == nil || *project.CustomerID <= 0 {
		return &TimeProjectValidationError{Message: "customer is required"}
	}
	if _, err := s.customers.GetByID(*project.CustomerID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return &TimeProjectValidationError{Message: "customer was not found"}
		}
		return err
	}
	if project.CategoryID != nil {
		exists, err := s.categories.Exists(*project.CategoryID)
		if err != nil {
			return err
		}
		if !exists {
			return &TimeProjectValidationError{Message: "category was not found"}
		}
	}
	return nil
}

func sanitizeTimeProjectCategory(category *models.TimeProjectCategory) {
	category.Name = sanitize.PlainTextField.Sanitize(category.Name)
	category.Description = sanitize.RichText.Sanitize(category.Description)
	category.Color = sanitize.ShortIdentifier.Sanitize(category.Color)
}

func timeProjectModel(detail repository.TimeProjectDetail) models.TimeProject {
	return models.TimeProject{
		ID: detail.ID, CustomerID: detail.CustomerID, CategoryID: detail.CategoryID,
		Name: detail.Name, Description: detail.Description, Status: detail.Status,
		Color: detail.Color, HourlyRate: detail.HourlyRate, Settings: detail.Settings,
		CreatedAt: detail.CreatedAt, UpdatedAt: detail.UpdatedAt,
	}
}
