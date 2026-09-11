package services

import (
	"errors"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	ErrItemTemplateNameRequired = errors.New("item template name is required")
	ErrItemTemplateTypeNotFound = errors.New("item template item type not found")
)

type ItemTemplateInput struct {
	Name            string
	DescriptionBody string
	Mode            string
	IsActive        *bool
	ItemTypeIDs     []int
}

type ItemTemplatePatch struct {
	Name            *string
	DescriptionBody *string
	Mode            *string
	IsActive        *bool
	ItemTypeIDs     *[]int
}

// ItemTemplateService owns template defaults, validation, and persistence.
type ItemTemplateService struct {
	repo      *repository.TemplateRepository
	itemTypes *repository.ItemTypeRepository
}

func NewItemTemplateService(db database.Database) *ItemTemplateService {
	return &ItemTemplateService{
		repo:      repository.NewTemplateRepository(db),
		itemTypes: repository.NewItemTypeRepository(db),
	}
}

func (s *ItemTemplateService) List(workspaceID int, itemTypeID *int) ([]models.ItemTemplate, error) {
	if itemTypeID != nil {
		return s.repo.ListForType(workspaceID, *itemTypeID)
	}
	return s.repo.ListByWorkspace(workspaceID)
}

func (s *ItemTemplateService) Get(id int) (*models.ItemTemplate, error) {
	return s.repo.GetByID(id)
}

func (s *ItemTemplateService) GetMandatory(workspaceID, itemTypeID int) (*models.ItemTemplate, error) {
	return s.repo.GetMandatoryForType(workspaceID, itemTypeID)
}

func (s *ItemTemplateService) Create(workspaceID, actorID int, input ItemTemplateInput) (*models.ItemTemplate, error) {
	name, err := normalizeItemTemplateName(input.Name)
	if err != nil {
		return nil, err
	}
	mode := input.Mode
	if mode == "" {
		mode = models.TemplateModeSelectable
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	if err := s.validateItemTypes(input.ItemTypeIDs); err != nil {
		return nil, err
	}
	exists, err := s.repo.NameExistsInWorkspace(workspaceID, name, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, repository.ErrDuplicateEntry
	}
	return s.repo.Create(&models.ItemTemplate{
		WorkspaceID: workspaceID, Name: name, DescriptionBody: input.DescriptionBody,
		Mode: mode, IsActive: isActive, ItemTypeIDs: input.ItemTypeIDs,
		CreatedBy: &actorID, UpdatedBy: &actorID,
	})
}

func (s *ItemTemplateService) Update(id, actorID int, patch ItemTemplatePatch) (*models.ItemTemplate, error) {
	template, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		template.Name, err = normalizeItemTemplateName(*patch.Name)
		if err != nil {
			return nil, err
		}
	}
	if patch.DescriptionBody != nil {
		template.DescriptionBody = *patch.DescriptionBody
	}
	if patch.Mode != nil {
		template.Mode = *patch.Mode
	}
	if patch.IsActive != nil {
		template.IsActive = *patch.IsActive
	}
	if patch.ItemTypeIDs != nil {
		if err := s.validateItemTypes(*patch.ItemTypeIDs); err != nil {
			return nil, err
		}
		template.ItemTypeIDs = *patch.ItemTypeIDs
	}
	exists, err := s.repo.NameExistsInWorkspace(template.WorkspaceID, template.Name, id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, repository.ErrDuplicateEntry
	}
	template.UpdatedBy = &actorID
	if err := s.repo.Update(template); err != nil {
		return nil, err
	}
	return s.repo.GetByID(id)
}

func (s *ItemTemplateService) Delete(id int) (*models.ItemTemplate, error) {
	template, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Delete(id); err != nil {
		return nil, err
	}
	return template, nil
}

func normalizeItemTemplateName(name string) (string, error) {
	name = sanitize.ShortIdentifier.Sanitize(name)
	if name == "" {
		return "", ErrItemTemplateNameRequired
	}
	return name, nil
}

func (s *ItemTemplateService) validateItemTypes(ids []int) error {
	for _, id := range ids {
		exists, err := s.itemTypes.Exists(id)
		if err != nil {
			return err
		}
		if !exists {
			return ErrItemTemplateTypeNotFound
		}
	}
	return nil
}
