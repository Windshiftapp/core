package services

import (
	"errors"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

var (
	// ErrRequirementAlreadyExists is returned when the page already has a
	// requirement number. Numbers are allocated once and never recycled.
	ErrRequirementAlreadyExists = errors.New("page is already a requirement")
	// ErrRequirementTypeInvalid is returned when the type is not a system value.
	ErrRequirementTypeInvalid = errors.New("requirement type is invalid")
	// ErrRequirementStatusInvalid is returned when the status is not a system value.
	ErrRequirementStatusInvalid = errors.New("requirement status is invalid")
	// ErrRequirementCrossWorkspaceMove is returned when a requirement-backed
	// page (or a descendant that is a requirement) would leave its workspace.
	// Silent renumbering during a generic page move is not allowed.
	ErrRequirementCrossWorkspaceMove = errors.New("requirement-backed pages cannot move across workspaces")
)

// RequirementService owns promotion, numbering, and identity invariants for
// page-backed requirements. Page content stays on PageService.
type RequirementService struct {
	db           database.Database
	pages        *repository.PageRepository
	requirements *repository.RequirementRepository
}

// NewRequirementService creates a RequirementService backed by db.
func NewRequirementService(db database.Database) *RequirementService {
	return &RequirementService{
		db:           db,
		pages:        repository.NewPageRepository(db),
		requirements: repository.NewRequirementRepository(db),
	}
}

// Promote allocates an immutable workspace-local number for an existing page.
// workspace_id is taken from the page, never from the caller. An empty status
// defaults to draft. A page can be promoted only once.
func (s *RequirementService) Promote(actorID, pageID int, requirementType, status string, ownerID *int) (*models.Requirement, error) {
	if status == "" {
		status = models.RequirementStatusDraft
	}
	if !models.IsValidRequirementType(requirementType) {
		return nil, ErrRequirementTypeInvalid
	}
	if !models.IsValidRequirementStatus(status) {
		return nil, ErrRequirementStatusInvalid
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Requirement, error) {
		page, err := s.pages.GetByIDTx(tx, pageID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, ErrPageNotFound
			}
			return nil, err
		}

		_, err = s.requirements.GetByPageIDTx(tx, page.ID)
		if err == nil {
			return nil, ErrRequirementAlreadyExists
		}
		if !errors.Is(err, repository.ErrNotFound) {
			return nil, err
		}

		number, err := s.requirements.NextNumberTx(tx, page.WorkspaceID)
		if err != nil {
			return nil, err
		}

		req := &models.Requirement{
			PageID:            page.ID,
			WorkspaceID:       page.WorkspaceID,
			RequirementNumber: number,
			RequirementType:   requirementType,
			Status:            status,
			OwnerID:           ownerID,
			CreatedBy:         actorID,
			UpdatedBy:         &actorID,
		}
		id, err := s.requirements.InsertTx(tx, req)
		if err != nil {
			if errors.Is(err, repository.ErrDuplicateEntry) {
				return nil, ErrRequirementAlreadyExists
			}
			return nil, err
		}
		if err := s.requirements.InsertHistoryTx(
			tx, id, models.RequirementHistoryFieldPromoted, "", strconv.Itoa(number), actorID,
		); err != nil {
			return nil, err
		}
		return s.requirements.GetByIDTx(tx, id)
	})
}
