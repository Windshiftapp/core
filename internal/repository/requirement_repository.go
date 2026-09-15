package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// requirementNumberAdvisoryLockClass serializes Postgres number allocation
// per workspace. Distinct from itemNumberAdvisoryLockClass (0x4954).
const requirementNumberAdvisoryLockClass = 0x5251 // 'RQ'

const requirementColumns = `id, page_id, workspace_id, requirement_number, requirement_type, status,
	owner_id, created_by, created_at, updated_by, updated_at`

// RequirementRepository persists page-backed requirements and their
// workspace-scoped number sequences.
type RequirementRepository struct {
	db database.Database
}

// NewRequirementRepository creates a RequirementRepository.
func NewRequirementRepository(db database.Database) *RequirementRepository {
	return &RequirementRepository{db: db}
}

// NextNumberTx allocates the next immutable requirement number for workspaceID.
// Sequences only increase, so deleting a requirement row never recycles its
// number. Postgres takes a transaction-scoped advisory lock so concurrent
// first inserts on an empty sequence row cannot race; SQLite writers are
// already serialized.
func (r *RequirementRepository) NextNumberTx(tx database.Tx, workspaceID int) (int, error) {
	if database.IsPostgresDriver(r.db.GetDriverName()) {
		if _, err := tx.Exec(`SELECT pg_advisory_xact_lock(?, ?)`, requirementNumberAdvisoryLockClass, workspaceID); err != nil {
			return 0, fmt.Errorf("failed to acquire requirement-number lock: %w", err)
		}
	}
	var last int
	err := tx.QueryRow(`
		INSERT INTO requirement_sequences (workspace_id, last_number)
		VALUES (?, 1)
		ON CONFLICT (workspace_id) DO UPDATE SET last_number = requirement_sequences.last_number + 1
		RETURNING last_number
	`, workspaceID).Scan(&last)
	if err != nil {
		return 0, fmt.Errorf("allocate requirement number for workspace %d: %w", workspaceID, err)
	}
	return last, nil
}

// InsertTx persists a requirement. Unique violations (page already
// promoted, or a duplicate number) map to ErrDuplicateEntry.
func (r *RequirementRepository) InsertTx(tx database.Tx, req *models.Requirement) (int, error) {
	now := time.Now().UTC()
	if req.CreatedAt.IsZero() {
		req.CreatedAt = now
	}
	if req.UpdatedAt.IsZero() {
		req.UpdatedAt = now
	}
	var id int
	err := tx.QueryRow(`
		INSERT INTO requirements (
			page_id, workspace_id, requirement_number, requirement_type, status,
			owner_id, created_by, created_at, updated_by, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		req.PageID, req.WorkspaceID, req.RequirementNumber, req.RequirementType, req.Status,
		nullInt(req.OwnerID), req.CreatedBy, req.CreatedAt, nullInt(req.UpdatedBy), req.UpdatedAt,
	).Scan(&id)
	if err != nil {
		if isUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("insert requirement: %w", err)
	}
	req.ID = id
	return id, nil
}

// GetByIDTx loads a requirement by primary key.
func (r *RequirementRepository) GetByIDTx(tx database.Tx, id int) (*models.Requirement, error) {
	req, err := scanRequirement(tx.QueryRow("SELECT "+requirementColumns+" FROM requirements WHERE id = ?", id))
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get requirement %d", id))
	}
	return req, nil
}

// GetByPageIDTx loads the requirement backing pageID, or ErrNotFound.
func (r *RequirementRepository) GetByPageIDTx(tx database.Tx, pageID int) (*models.Requirement, error) {
	req, err := scanRequirement(tx.QueryRow("SELECT "+requirementColumns+" FROM requirements WHERE page_id = ?", pageID))
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get requirement for page %d", pageID))
	}
	return req, nil
}

// GetByWorkspaceAndNumber loads a requirement by its immutable workspace-local number.
func (r *RequirementRepository) GetByWorkspaceAndNumber(workspaceID, number int) (*models.Requirement, error) {
	req, err := scanRequirement(r.db.QueryRow(
		"SELECT "+requirementColumns+" FROM requirements WHERE workspace_id = ? AND requirement_number = ?",
		workspaceID, number,
	))
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get requirement %d in workspace %d", number, workspaceID))
	}
	return req, nil
}

// ExistsInPageIDsTx reports whether any of the given pages is a requirement.
func (r *RequirementRepository) ExistsInPageIDsTx(tx database.Tx, pageIDs []int) (bool, error) {
	if len(pageIDs) == 0 {
		return false, nil
	}
	clause, args := inPlaceholders(pageIDs)
	var exists int
	err := tx.QueryRow(
		`SELECT 1 FROM requirements WHERE page_id IN (`+clause+`) LIMIT 1`,
		args...,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check requirements in page set: %w", err)
	}
	return true, nil
}

// InsertHistoryTx appends an immutable history row for a requirement field change.
func (r *RequirementRepository) InsertHistoryTx(tx database.Tx, requirementID int, fieldName, oldValue, newValue string, userID int) error {
	_, err := tx.Exec(`
		INSERT INTO requirement_history (requirement_id, user_id, field_name, old_value, new_value)
		VALUES (?, ?, ?, ?, ?)
	`, requirementID, userID, fieldName, nullStringArg(oldValue), nullStringArg(newValue))
	if err != nil {
		return fmt.Errorf("insert requirement history: %w", err)
	}
	return nil
}

func scanRequirement(s rowScanner) (*models.Requirement, error) {
	var req models.Requirement
	var ownerID, updatedBy sql.NullInt64
	if err := s.Scan(
		&req.ID, &req.PageID, &req.WorkspaceID, &req.RequirementNumber, &req.RequirementType, &req.Status,
		&ownerID, &req.CreatedBy, &req.CreatedAt, &updatedBy, &req.UpdatedAt,
	); err != nil {
		return nil, err
	}
	assignNullableInt(&req.OwnerID, ownerID)
	assignNullableInt(&req.UpdatedBy, updatedBy)
	return &req, nil
}
