package services

import (
	"errors"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

const defaultPageLabelColor = "#3B82F6"

var (
	ErrPageLabelNameRequired      = errors.New("page label name is required")
	ErrPageLabelWorkspaceMismatch = errors.New("page label does not belong to workspace")
	ErrPageLabelIDRequired        = errors.New("page label ID is required")
)

// PageLabelUpdate describes the optional fields accepted by page-label updates.
type PageLabelUpdate struct {
	Name  *string
	Color *string
}

// PageLabelService owns workspace page-label and page-assignment behavior.
type PageLabelService struct {
	labels  *repository.PageLabelRepository
	auditor *logger.Auditor
}

// NewPageLabelService creates the shared page-label application boundary.
func NewPageLabelService(labels *repository.PageLabelRepository, auditors ...*logger.Auditor) *PageLabelService {
	service := &PageLabelService{labels: labels}
	if len(auditors) > 0 {
		service.auditor = auditors[0]
	}
	return service
}

// List returns the page-label catalog for a workspace.
func (s *PageLabelService) List(workspaceID int) ([]models.PageLabel, error) {
	return s.labels.ListByWorkspace(workspaceID)
}

// Get returns a page label when it belongs to the requested workspace.
func (s *PageLabelService) Get(workspaceID, labelID int) (*models.PageLabel, error) {
	label, err := s.labels.GetByID(labelID)
	if err != nil {
		return nil, err
	}
	if label.WorkspaceID != workspaceID {
		return nil, ErrPageLabelWorkspaceMismatch
	}
	return label, nil
}

// Create validates and creates a page label in a workspace.
func (s *PageLabelService) Create(workspaceID int, name, color string, actors ...AuditActor) (*models.PageLabel, error) {
	name = sanitize.ShortIdentifier.Sanitize(name)
	if name == "" {
		return nil, ErrPageLabelNameRequired
	}
	if color == "" {
		color = defaultPageLabelColor
	}

	exists, err := s.labels.NameExistsInWorkspace(workspaceID, name, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, repository.ErrDuplicateEntry
	}

	id, _, err := s.labels.Create(name, color, workspaceID)
	if err != nil {
		return nil, err
	}
	label, err := s.labels.GetByID(id)
	if err == nil {
		s.emitAudit(optionalAuditActor(actors), logger.ActionPageLabelCreate, label)
	}
	return label, err
}

// Update validates and applies a partial page-label update.
func (s *PageLabelService) Update(workspaceID, labelID int, update PageLabelUpdate, actors ...AuditActor) (*models.PageLabel, error) {
	existing, err := s.Get(workspaceID, labelID)
	if err != nil {
		return nil, err
	}

	name := existing.Name
	if update.Name != nil {
		name = sanitize.ShortIdentifier.Sanitize(*update.Name)
		if name == "" {
			return nil, ErrPageLabelNameRequired
		}
	}
	color := existing.Color
	if update.Color != nil {
		color = *update.Color
		if color == "" {
			color = defaultPageLabelColor
		}
	}

	if name != existing.Name {
		exists, err := s.labels.NameExistsInWorkspace(workspaceID, name, labelID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, repository.ErrDuplicateEntry
		}
	}
	if err := s.labels.Update(labelID, name, color); err != nil {
		return nil, err
	}
	label, err := s.labels.GetByID(labelID)
	if err == nil {
		s.emitAudit(optionalAuditActor(actors), logger.ActionPageLabelUpdate, label)
	}
	return label, err
}

// Delete removes a page label after checking workspace ownership.
func (s *PageLabelService) Delete(workspaceID, labelID int, actors ...AuditActor) (*models.PageLabel, error) {
	existing, err := s.Get(workspaceID, labelID)
	if err != nil {
		return nil, err
	}
	if err := s.labels.Delete(labelID); err != nil {
		return nil, err
	}
	s.emitAudit(optionalAuditActor(actors), logger.ActionPageLabelDelete, existing)
	return existing, nil
}

func (s *PageLabelService) emitAudit(actor *AuditActor, action string, label *models.PageLabel) {
	if actor == nil || s.auditor == nil || label == nil {
		return
	}
	s.auditor.LogEvent(logger.AuditEvent{
		UserID: actor.UserID, Username: actor.Username, IPAddress: actor.IPAddress,
		UserAgent: actor.UserAgent, ActionType: action, ResourceType: logger.ResourcePageLabel,
		ResourceID: &label.ID, ResourceName: label.Name,
		Details: mergeAuditDetails(map[string]any{"workspace_id": label.WorkspaceID}, *actor), Success: true,
	})
}

// ListForPage returns labels assigned to a page.
func (s *PageLabelService) ListForPage(pageID int) ([]models.PageLabel, error) {
	return s.labels.ListForPage(pageID)
}

// SetForPage validates and replaces a page's complete label set.
func (s *PageLabelService) SetForPage(workspaceID, pageID int, labelIDs []int) ([]models.PageLabel, error) {
	if err := s.requireWorkspaceLabels(workspaceID, labelIDs); err != nil {
		return nil, err
	}
	if err := s.labels.ReplaceAssignments(pageID, labelIDs); err != nil {
		return nil, err
	}
	return s.labels.ListForPage(pageID)
}

// AddToPage validates and assigns one page label.
func (s *PageLabelService) AddToPage(workspaceID, pageID, labelID int) ([]models.PageLabel, error) {
	if labelID == 0 {
		return nil, ErrPageLabelIDRequired
	}
	if err := s.requireWorkspaceLabels(workspaceID, []int{labelID}); err != nil {
		return nil, err
	}
	if err := s.labels.AddAssignment(pageID, labelID); err != nil {
		return nil, err
	}
	return s.labels.ListForPage(pageID)
}

// RemoveFromPage detaches one page label.
func (s *PageLabelService) RemoveFromPage(pageID, labelID int) error {
	return s.labels.RemoveAssignment(pageID, labelID)
}

func (s *PageLabelService) requireWorkspaceLabels(workspaceID int, labelIDs []int) error {
	for _, labelID := range labelIDs {
		labelWorkspaceID, err := s.labels.GetWorkspaceID(labelID)
		if err != nil {
			return err
		}
		if labelWorkspaceID != workspaceID {
			return ErrPageLabelWorkspaceMismatch
		}
	}
	return nil
}
