package services

import (
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type ItemTemplateMutationAccess interface {
	CanAdminWorkspace(userID, workspaceID int) (bool, error)
}

// ItemTemplateApplicationService owns template authorization and audit effects.
type ItemTemplateApplicationService struct {
	db      database.Database
	service *ItemTemplateService
	access  ItemTemplateMutationAccess
}

func NewItemTemplateApplicationService(db database.Database, access ItemTemplateMutationAccess) *ItemTemplateApplicationService {
	return &ItemTemplateApplicationService{db: db, service: NewItemTemplateService(db), access: access}
}

func (s *ItemTemplateApplicationService) Create(actor AuditActor, workspaceID int, input ItemTemplateInput) (*models.ItemTemplate, error) {
	if err := s.requireAdmin(actor.UserID, workspaceID); err != nil {
		return nil, err
	}
	result, err := s.service.Create(workspaceID, actor.UserID, input)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTemplateCreate, logger.ResourceItemTemplate, &result.ID, result.Name, map[string]any{"workspace_id": workspaceID})
	return result, nil
}

func (s *ItemTemplateApplicationService) Update(actor AuditActor, workspaceID, templateID int, patch ItemTemplatePatch) (*models.ItemTemplate, error) {
	if err := s.requireTemplateAdmin(actor.UserID, workspaceID, templateID); err != nil {
		return nil, err
	}
	result, err := s.service.Update(templateID, actor.UserID, patch)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionTemplateUpdate, logger.ResourceItemTemplate, &result.ID, result.Name, map[string]any{"workspace_id": workspaceID})
	return result, nil
}

func (s *ItemTemplateApplicationService) Delete(actor AuditActor, workspaceID, templateID int) error {
	if err := s.requireTemplateAdmin(actor.UserID, workspaceID, templateID); err != nil {
		return err
	}
	result, err := s.service.Delete(templateID)
	if err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionTemplateDelete, logger.ResourceItemTemplate, &result.ID, result.Name, map[string]any{"workspace_id": workspaceID})
	return nil
}

func (s *ItemTemplateApplicationService) requireTemplateAdmin(userID, workspaceID, templateID int) error {
	template, err := s.service.Get(templateID)
	if err != nil {
		return err
	}
	if template.WorkspaceID != workspaceID {
		return repository.ErrNotFound
	}
	return s.requireAdmin(userID, workspaceID)
}

func (s *ItemTemplateApplicationService) requireAdmin(userID, workspaceID int) error {
	allowed, err := s.access.CanAdminWorkspace(userID, workspaceID)
	if err != nil {
		return err
	}
	if !allowed {
		return repository.ErrNotFound
	}
	return nil
}
