package services

import (
	"encoding/json"
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
	// ErrRequirementNotFound masks missing requirements for HTTP adapters.
	ErrRequirementNotFound = errors.New("requirement not found")
	// ErrRequirementNoChanges is returned when an update supplies no fields.
	ErrRequirementNoChanges = errors.New("no requirement fields to update")
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
	pageService  *PageService
	requirements *repository.RequirementRepository
}

// NewRequirementService creates a RequirementService backed by db.
func NewRequirementService(db database.Database, pageService *PageService) *RequirementService {
	return &RequirementService{
		db:           db,
		pages:        repository.NewPageRepository(db),
		pageService:  pageService,
		requirements: repository.NewRequirementRepository(db),
	}
}

// CreateRequirementInput is the request shape for atomic page + requirement
// creation.
type CreateRequirementInput struct {
	WorkspaceID     int
	ParentID        *int
	Title           string
	Content         string
	Metadata        json.RawMessage
	RequirementType string
	Status          string
	OwnerID         *int
}

// RequirementUpdateInput carries partial metadata updates.
type RequirementUpdateInput struct {
	RequirementType *string
	Status          *string
	OwnerIDSet      bool
	OwnerID         *int
}

// RequirementListFilter scopes workspace requirement registry queries.
type RequirementListFilter struct {
	Query           string
	RequirementType string
	Status          string
	OwnerID         *int
	Limit           int
	Offset          int
}

// Create inserts a backing page and requirement row in one transaction.
func (s *RequirementService) Create(actorID int, in CreateRequirementInput) (*models.Requirement, error) {
	if in.Status == "" {
		in.Status = models.RequirementStatusDraft
	}
	if !models.IsValidRequirementType(in.RequirementType) {
		return nil, ErrRequirementTypeInvalid
	}
	if !models.IsValidRequirementStatus(in.Status) {
		return nil, ErrRequirementStatusInvalid
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Requirement, error) {
		page, err := s.pageService.createPageTx(tx, actorID, CreatePageInput{
			WorkspaceID: in.WorkspaceID,
			ParentID:    in.ParentID,
			Title:       in.Title,
			Content:     in.Content,
			Metadata:    in.Metadata,
		})
		if err != nil {
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
			RequirementType:   in.RequirementType,
			Status:            in.Status,
			OwnerID:           in.OwnerID,
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

// GetByPageID loads the requirement backing pageID without ACL checks.
func (s *RequirementService) GetByPageID(pageID int) (*models.Requirement, error) {
	req, err := s.requirements.GetByPageID(pageID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrRequirementNotFound
		}
		return nil, err
	}
	return req, nil
}

// GetByWorkspaceAndNumber loads a requirement without ACL checks.
func (s *RequirementService) GetByWorkspaceAndNumber(workspaceID, number int) (*models.Requirement, error) {
	req, err := s.requirements.GetByWorkspaceAndNumber(workspaceID, number)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrRequirementNotFound
		}
		return nil, err
	}
	return req, nil
}

// Update applies mutable metadata and records one history row per changed field.
func (s *RequirementService) Update(actorID, workspaceID, number int, patch RequirementUpdateInput) (*models.Requirement, error) {
	if patch.RequirementType == nil && patch.Status == nil && !patch.OwnerIDSet {
		return nil, ErrRequirementNoChanges
	}
	if patch.RequirementType != nil && !models.IsValidRequirementType(*patch.RequirementType) {
		return nil, ErrRequirementTypeInvalid
	}
	if patch.Status != nil && !models.IsValidRequirementStatus(*patch.Status) {
		return nil, ErrRequirementStatusInvalid
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Requirement, error) {
		current, err := s.requirements.GetByWorkspaceAndNumberTx(tx, workspaceID, number)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, ErrRequirementNotFound
			}
			return nil, err
		}

		updated := *current
		if patch.RequirementType != nil {
			updated.RequirementType = *patch.RequirementType
		}
		if patch.Status != nil {
			updated.Status = *patch.Status
		}
		if patch.OwnerIDSet {
			updated.OwnerID = patch.OwnerID
		}

		if updated.RequirementType == current.RequirementType &&
			updated.Status == current.Status &&
			ownerIDsEqual(updated.OwnerID, current.OwnerID) {
			return nil, ErrRequirementNoChanges
		}

		if err := s.requirements.UpdateTx(tx, current.ID, repository.RequirementUpdatePatch{
			RequirementType: updated.RequirementType,
			Status:          updated.Status,
			OwnerID:         updated.OwnerID,
		}, actorID); err != nil {
			return nil, err
		}

		if err := s.recordRequirementHistoryTx(tx, current, &updated, actorID); err != nil {
			return nil, err
		}
		return s.requirements.GetByIDTx(tx, current.ID)
	})
}

// List returns workspace requirements without ACL filtering.
func (s *RequirementService) List(workspaceID int, filter RequirementListFilter) ([]repository.RequirementListRow, error) {
	return s.requirements.ListByWorkspace(workspaceID, repository.RequirementListFilter{
		Query:           filter.Query,
		RequirementType: filter.RequirementType,
		Status:          filter.Status,
		OwnerID:         filter.OwnerID,
		ExcludeArchived: true,
		Limit:           filter.Limit,
		Offset:          filter.Offset,
	})
}

// ListHistory returns metadata history for a requirement.
func (s *RequirementService) ListHistory(requirementID int) ([]models.RequirementHistoryEntry, error) {
	return s.requirements.ListHistory(requirementID)
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

func (s *RequirementService) recordRequirementHistoryTx(tx database.Tx, original, updated *models.Requirement, actorID int) error {
	if original.RequirementType != updated.RequirementType {
		if err := s.requirements.InsertHistoryTx(
			tx, original.ID, models.RequirementHistoryFieldType,
			original.RequirementType, updated.RequirementType, actorID,
		); err != nil {
			return err
		}
	}
	if original.Status != updated.Status {
		if err := s.requirements.InsertHistoryTx(
			tx, original.ID, models.RequirementHistoryFieldStatus,
			original.Status, updated.Status, actorID,
		); err != nil {
			return err
		}
	}
	if !ownerIDsEqual(original.OwnerID, updated.OwnerID) {
		if err := s.requirements.InsertHistoryTx(
			tx, original.ID, models.RequirementHistoryFieldOwner,
			intPtrToString(original.OwnerID), intPtrToString(updated.OwnerID), actorID,
		); err != nil {
			return err
		}
	}
	return nil
}

func ownerIDsEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
