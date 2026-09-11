package services

import (
	"errors"
	"fmt"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	ErrDiagramNameRequired = errors.New("diagram name is required")
	ErrDiagramDataRequired = errors.New("diagram data is required")
)

// ItemDiagramService owns diagram validation, persistence, and item history.
type ItemDiagramService struct {
	repo *repository.DiagramRepository
}

func NewItemDiagramService(repo *repository.DiagramRepository) *ItemDiagramService {
	return &ItemDiagramService{repo: repo}
}

func NormalizeDiagramInput(rawName, rawData string) (name, data string, err error) {
	name = sanitize.ShortIdentifier.Sanitize(rawName)
	if name == "" {
		return "", "", ErrDiagramNameRequired
	}
	if rawData == "" {
		return "", "", ErrDiagramDataRequired
	}
	return name, rawData, nil
}

func (s *ItemDiagramService) List(itemID int) ([]models.ItemDiagram, error) {
	return s.repo.ListByItem(itemID)
}

func (s *ItemDiagramService) Get(id int) (*models.ItemDiagram, error) {
	return s.repo.GetByID(id)
}

func (s *ItemDiagramService) Create(itemID int, name, data string, actorID int) (*models.ItemDiagram, error) {
	name, data, err := NormalizeDiagramInput(name, data)
	if err != nil {
		return nil, err
	}
	id, _, err := s.repo.Create(itemID, name, data, &actorID)
	if err != nil {
		return nil, err
	}
	_ = s.repo.RecordHistory(itemID, actorID, "diagram_created", nil, fmt.Sprintf("diagram:%d:%s", id, name))
	return s.repo.GetByID(int(id))
}

func (s *ItemDiagramService) Update(id int, name, data string, actorID int) (*models.ItemDiagram, error) {
	name, data, err := NormalizeDiagramInput(name, data)
	if err != nil {
		return nil, err
	}
	current, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Update(id, name, data, &actorID); err != nil {
		return nil, err
	}
	var oldName *string
	if current.Name != name {
		oldName = &current.Name
	}
	_ = s.repo.RecordHistory(current.ItemID, actorID, "diagram_updated", oldName, fmt.Sprintf("diagram:%d:%s", id, name))
	return s.repo.GetByID(id)
}

// Patch updates only the supplied diagram fields.
func (s *ItemDiagramService) Patch(id int, name, data *string, actorID int) (*models.ItemDiagram, error) {
	current, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if name == nil {
		name = &current.Name
	}
	if data == nil {
		data = &current.DiagramData
	}
	return s.Update(id, *name, *data, actorID)
}

func (s *ItemDiagramService) Delete(id, actorID int) (*models.ItemDiagram, error) {
	diagram, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	_ = s.repo.RecordHistory(diagram.ItemID, actorID, "diagram_deleted", &diagram.Name, diagram.Name)
	if err := s.repo.Delete(id); err != nil {
		return nil, err
	}
	return diagram, nil
}
