package services

import (
	"errors"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

const defaultLabelColor = "#3B82F6"

var (
	ErrLabelNameRequired = errors.New("label name is required")
	ErrLabelIDRequired   = errors.New("label ID is required")
)

// LabelUpdate describes the optional fields accepted by label updates.
type LabelUpdate struct {
	Name  *string
	Color *string
}

// LabelApplicationService owns global label and item-assignment behavior.
type LabelApplicationService struct {
	db     database.Database
	labels *repository.LabelRepository
}

// NewLabelApplicationService creates the shared label application boundary.
func NewLabelApplicationService(db database.Database) *LabelApplicationService {
	return &LabelApplicationService{
		db:     db,
		labels: repository.NewLabelRepository(db),
	}
}

// List returns the global label catalog.
func (s *LabelApplicationService) List() ([]models.Label, error) {
	return s.labels.ListAll()
}

// Get returns one global label.
func (s *LabelApplicationService) Get(id int) (*models.Label, error) {
	return s.labels.GetByID(id)
}

// Create validates and creates a global label.
func (s *LabelApplicationService) Create(actor AuditActor, name, color string) (*models.Label, error) {
	name = sanitize.ShortIdentifier.Sanitize(name)
	if name == "" {
		return nil, ErrLabelNameRequired
	}
	if color == "" {
		color = defaultLabelColor
	}

	id, _, err := s.labels.Create(name, color)
	if err != nil {
		return nil, err
	}
	label, err := s.labels.GetByID(int(id))
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionLabelCreate, label)
	return label, nil
}

// Update validates and applies a partial global-label update.
func (s *LabelApplicationService) Update(actor AuditActor, id int, update LabelUpdate) (*models.Label, error) {
	existing, err := s.labels.GetByID(id)
	if err != nil {
		return nil, err
	}

	name := existing.Name
	if update.Name != nil {
		name = sanitize.ShortIdentifier.Sanitize(*update.Name)
		if name == "" {
			return nil, ErrLabelNameRequired
		}
	}
	color := existing.Color
	if update.Color != nil {
		color = *update.Color
		if color == "" {
			color = defaultLabelColor
		}
	}

	if err := s.labels.Update(id, name, color); err != nil {
		return nil, err
	}

	updated, err := s.labels.GetByID(id)
	if err != nil {
		return nil, err
	}
	s.emitAudit(actor, logger.ActionLabelUpdate, updated)
	return updated, nil
}

// Delete removes a global label and its item assignments.
func (s *LabelApplicationService) Delete(actor AuditActor, id int) error {
	existing, err := s.labels.GetByID(id)
	if err != nil {
		return err
	}
	if err := s.labels.Delete(id); err != nil {
		return err
	}
	s.emitAudit(actor, logger.ActionLabelDelete, existing)
	return nil
}

// ListForItem returns the labels assigned to an item.
func (s *LabelApplicationService) ListForItem(itemID int) ([]models.Label, error) {
	return s.labels.ListForItem(itemID)
}

// SetForItem validates and replaces an item's complete label set.
func (s *LabelApplicationService) SetForItem(itemID int, labelIDs []int) ([]models.Label, error) {
	if err := s.requireLabels(labelIDs); err != nil {
		return nil, err
	}
	if err := s.labels.ReplaceItemLabels(itemID, labelIDs); err != nil {
		return nil, err
	}
	return s.labels.ListForItem(itemID)
}

// AddToItem validates and assigns one label to an item.
func (s *LabelApplicationService) AddToItem(itemID, labelID int) ([]models.Label, error) {
	if labelID == 0 {
		return nil, ErrLabelIDRequired
	}
	if err := s.requireLabels([]int{labelID}); err != nil {
		return nil, err
	}
	if err := s.labels.AddItemLabel(itemID, labelID); err != nil {
		return nil, err
	}
	return s.labels.ListForItem(itemID)
}

// RemoveFromItem detaches one label from an item.
func (s *LabelApplicationService) RemoveFromItem(itemID, labelID int) error {
	return s.labels.RemoveItemLabel(itemID, labelID)
}

func (s *LabelApplicationService) requireLabels(labelIDs []int) error {
	for _, labelID := range labelIDs {
		if _, err := s.labels.GetByID(labelID); err != nil {
			return err
		}
	}
	return nil
}

func (s *LabelApplicationService) emitAudit(actor AuditActor, action string, label *models.Label) {
	if actor.UserID == 0 {
		return
	}
	labelID := label.ID
	emitServiceAudit(s.db, actor, action, logger.ResourceLabel, &labelID, label.Name, nil)
}
