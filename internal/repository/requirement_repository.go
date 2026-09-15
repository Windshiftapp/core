package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// requirementNumberAdvisoryLockClass serializes Postgres number allocation
// per workspace. Distinct from itemNumberAdvisoryLockClass (0x4954).
const requirementNumberAdvisoryLockClass = 0x5251 // 'RQ'

const requirementColumns = `id, page_id, workspace_id, requirement_number, requirement_type, status,
	owner_id, created_by, created_at, updated_by, updated_at`

const requirementAliasedColumns = `r.id, r.page_id, r.workspace_id, r.requirement_number, r.requirement_type, r.status,
	r.owner_id, r.created_by, r.created_at, r.updated_by, r.updated_at`

const requirementLinkedItemCountSubquery = `
	(SELECT COUNT(*) FROM item_links il
	 WHERE (il.source_type = 'page' AND il.source_id = r.page_id AND il.target_type = 'item')
	    OR (il.target_type = 'page' AND il.target_id = r.page_id AND il.source_type = 'item'))`

// RequirementLinkedTestCountSubquery counts page↔test_case links for requirement row alias r.
const RequirementLinkedTestCountSubquery = `
	(SELECT COUNT(*) FROM item_links il
	 WHERE (il.source_type = 'page' AND il.source_id = r.page_id AND il.target_type = 'test_case')
	    OR (il.target_type = 'page' AND il.target_id = r.page_id AND il.source_type = 'test_case'))`

const requirementLinkedTestCountSubquery = RequirementLinkedTestCountSubquery

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

// GetByPageID loads the requirement backing pageID, or ErrNotFound.
func (r *RequirementRepository) GetByPageID(pageID int) (*models.Requirement, error) {
	req, err := scanRequirement(r.db.QueryRow("SELECT "+requirementColumns+" FROM requirements WHERE page_id = ?", pageID))
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get requirement for page %d", pageID))
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

// GetByWorkspaceAndNumberTx loads a requirement inside a caller-owned transaction.
func (r *RequirementRepository) GetByWorkspaceAndNumberTx(tx database.Tx, workspaceID, number int) (*models.Requirement, error) {
	req, err := scanRequirement(tx.QueryRow(
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

// RequirementListFilter scopes workspace requirement registry queries.
type RequirementListFilter struct {
	Query           string
	RequirementType string
	Status          string
	OwnerID         *int
	HasItemLinks    *bool
	HasTestLinks    *bool
	LabelIDs        []int
	ExcludeArchived bool
	Limit           int
	Offset          int
}

// RequirementListRow is a requirement joined with its backing page metadata.
type RequirementListRow struct {
	Requirement     models.Requirement
	PageTitle       string
	PageArchivedAt  *time.Time
	LinkedItemCount int
	LinkedTestCount int
}

// RequirementLinkCounts holds traceability aggregates for a backing page.
type RequirementLinkCounts struct {
	LinkedItemCount int
	LinkedTestCount int
}

// RequirementUpdatePatch carries mutable requirement metadata fields.
type RequirementUpdatePatch struct {
	RequirementType string
	Status          string
	OwnerID         *int
}

// UpdateTx applies mutable metadata columns for a requirement row.
func (r *RequirementRepository) UpdateTx(tx database.Tx, id int, patch RequirementUpdatePatch, actorID int) error {
	now := time.Now().UTC()
	result, err := tx.Exec(`
		UPDATE requirements
		SET requirement_type = ?, status = ?, owner_id = ?, updated_by = ?, updated_at = ?
		WHERE id = ?
	`, patch.RequirementType, patch.Status, nullInt(patch.OwnerID), actorID, now, id)
	if err != nil {
		return fmt.Errorf("update requirement %d: %w", id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update requirement %d rows affected: %w", id, err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetLinkCountsByPageID returns traceability aggregates for a backing page.
func (r *RequirementRepository) GetLinkCountsByPageID(pageID int) (RequirementLinkCounts, error) {
	var counts RequirementLinkCounts
	err := r.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM item_links il
			 WHERE (il.source_type = 'page' AND il.source_id = ? AND il.target_type = 'item')
			    OR (il.target_type = 'page' AND il.target_id = ? AND il.source_type = 'item')),
			(SELECT COUNT(*) FROM item_links il
			 WHERE (il.source_type = 'page' AND il.source_id = ? AND il.target_type = 'test_case')
			    OR (il.target_type = 'page' AND il.target_id = ? AND il.source_type = 'test_case'))
	`, pageID, pageID, pageID, pageID).Scan(&counts.LinkedItemCount, &counts.LinkedTestCount)
	if err != nil {
		return RequirementLinkCounts{}, fmt.Errorf("get requirement link counts for page %d: %w", pageID, err)
	}
	return counts, nil
}

// RequirementKeyRow is a minimal requirement identity for key maps.
type RequirementKeyRow struct {
	PageID            int
	RequirementNumber int
}

func (r *RequirementRepository) appendListWhere(query string, args []any, filter RequirementListFilter) (queryOut string, argsOut []any) {
	if filter.ExcludeArchived {
		query += ` AND p.archived_at IS NULL`
	}
	if filter.RequirementType != "" {
		query += ` AND r.requirement_type = ?`
		args = append(args, filter.RequirementType)
	}
	if filter.Status != "" {
		query += ` AND r.status = ?`
		args = append(args, filter.Status)
	}
	if filter.OwnerID != nil {
		query += ` AND r.owner_id = ?`
		args = append(args, *filter.OwnerID)
	}
	if filter.HasItemLinks != nil {
		if *filter.HasItemLinks {
			query += ` AND ` + requirementLinkedItemCountSubquery + ` > 0`
		} else {
			query += ` AND ` + requirementLinkedItemCountSubquery + ` = 0`
		}
	}
	if filter.HasTestLinks != nil {
		if *filter.HasTestLinks {
			query += ` AND ` + requirementLinkedTestCountSubquery + ` > 0`
		} else {
			query += ` AND ` + requirementLinkedTestCountSubquery + ` = 0`
		}
	}
	if len(filter.LabelIDs) > 0 {
		placeholders := strings.Repeat("?,", len(filter.LabelIDs))
		placeholders = placeholders[:len(placeholders)-1]
		query += ` AND EXISTS (
			SELECT 1 FROM page_label_assignments pla
			WHERE pla.page_id = r.page_id AND pla.page_label_id IN (` + placeholders + `)
		)`
		for _, labelID := range filter.LabelIDs {
			args = append(args, labelID)
		}
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		query += ` AND (LOWER(p.title) LIKE ? OR CAST(r.requirement_number AS TEXT) LIKE ?)`
		args = append(args, like, like)
	}
	queryOut = query
	argsOut = args
	return
}

// ListPageIDsByWorkspace returns backing page IDs matching filters in requirement-number order.
func (r *RequirementRepository) ListPageIDsByWorkspace(workspaceID int, filter RequirementListFilter) ([]int, error) {
	query := `
		SELECT r.page_id
		FROM requirements r
		JOIN pages p ON p.id = r.page_id AND p.workspace_id = r.workspace_id
		WHERE r.workspace_id = ?`
	args := []any{workspaceID}
	query, args = r.appendListWhere(query, args, filter)
	query += ` ORDER BY r.requirement_number ASC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list requirement page ids in workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []int
	for rows.Next() {
		var pageID int
		if err := rows.Scan(&pageID); err != nil {
			return nil, fmt.Errorf("scan requirement page id: %w", err)
		}
		out = append(out, pageID)
	}
	return out, rows.Err()
}

// ListKeysByWorkspace returns minimal requirement identity rows for key maps.
func (r *RequirementRepository) ListKeysByWorkspace(workspaceID int) ([]RequirementKeyRow, error) {
	filter := RequirementListFilter{ExcludeArchived: true}
	query := `
		SELECT r.page_id, r.requirement_number
		FROM requirements r
		JOIN pages p ON p.id = r.page_id AND p.workspace_id = r.workspace_id
		WHERE r.workspace_id = ?`
	args := []any{workspaceID}
	query, args = r.appendListWhere(query, args, filter)
	query += ` ORDER BY r.requirement_number ASC`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list requirement keys in workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []RequirementKeyRow
	for rows.Next() {
		var row RequirementKeyRow
		if err := rows.Scan(&row.PageID, &row.RequirementNumber); err != nil {
			return nil, fmt.Errorf("scan requirement key row: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListByPageIDs returns full list rows for the given backing pages in requirement-number order.
func (r *RequirementRepository) ListByPageIDs(workspaceID int, pageIDs []int) ([]RequirementListRow, error) {
	if len(pageIDs) == 0 {
		return nil, nil
	}
	clause, args := inPlaceholders(pageIDs)
	query := `
		SELECT ` + requirementAliasedColumns + `, p.title, p.archived_at,
			` + requirementLinkedItemCountSubquery + ` AS linked_item_count,
			` + requirementLinkedTestCountSubquery + ` AS linked_test_count
		FROM requirements r
		JOIN pages p ON p.id = r.page_id AND p.workspace_id = r.workspace_id
		WHERE r.workspace_id = ? AND r.page_id IN (` + clause + `)
		ORDER BY r.requirement_number ASC`
	args = append([]any{workspaceID}, args...)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list requirements by page ids in workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []RequirementListRow
	for rows.Next() {
		row, scanErr := scanRequirementListRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan requirement list row: %w", scanErr)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListByWorkspace returns requirements in a workspace with optional filters.
func (r *RequirementRepository) ListByWorkspace(workspaceID int, filter RequirementListFilter) ([]RequirementListRow, error) {
	query := `
		SELECT ` + requirementAliasedColumns + `, p.title, p.archived_at,
			` + requirementLinkedItemCountSubquery + ` AS linked_item_count,
			` + requirementLinkedTestCountSubquery + ` AS linked_test_count
		FROM requirements r
		JOIN pages p ON p.id = r.page_id AND p.workspace_id = r.workspace_id
		WHERE r.workspace_id = ?`
	args := []any{workspaceID}
	query, args = r.appendListWhere(query, args, filter)
	query += ` ORDER BY r.requirement_number ASC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list requirements in workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []RequirementListRow
	for rows.Next() {
		row, scanErr := scanRequirementListRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan requirement list row: %w", scanErr)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListHistory returns requirement metadata history newest first.
func (r *RequirementRepository) ListHistory(requirementID int) ([]models.RequirementHistoryEntry, error) {
	rows, err := r.db.Query(`
		SELECT id, requirement_id, user_id, field_name, old_value, new_value, changed_at
		FROM requirement_history
		WHERE requirement_id = ?
		ORDER BY changed_at DESC, id DESC
	`, requirementID)
	if err != nil {
		return nil, fmt.Errorf("list requirement history %d: %w", requirementID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.RequirementHistoryEntry
	for rows.Next() {
		var entry models.RequirementHistoryEntry
		var oldValue, newValue sql.NullString
		if err := rows.Scan(
			&entry.ID, &entry.RequirementID, &entry.UserID, &entry.FieldName,
			&oldValue, &newValue, &entry.ChangedAt,
		); err != nil {
			return nil, fmt.Errorf("scan requirement history: %w", err)
		}
		if oldValue.Valid {
			entry.OldValue = &oldValue.String
		}
		if newValue.Valid {
			entry.NewValue = &newValue.String
		}
		out = append(out, entry)
	}
	return out, rows.Err()
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

func scanRequirementListRow(rows *sql.Rows) (RequirementListRow, error) {
	var row RequirementListRow
	var ownerID, updatedBy sql.NullInt64
	var archivedAt sql.NullTime
	if err := rows.Scan(
		&row.Requirement.ID, &row.Requirement.PageID, &row.Requirement.WorkspaceID,
		&row.Requirement.RequirementNumber, &row.Requirement.RequirementType, &row.Requirement.Status,
		&ownerID, &row.Requirement.CreatedBy, &row.Requirement.CreatedAt, &updatedBy, &row.Requirement.UpdatedAt,
		&row.PageTitle, &archivedAt, &row.LinkedItemCount, &row.LinkedTestCount,
	); err != nil {
		return RequirementListRow{}, err
	}
	assignNullableInt(&row.Requirement.OwnerID, ownerID)
	assignNullableInt(&row.Requirement.UpdatedBy, updatedBy)
	if archivedAt.Valid {
		row.PageArchivedAt = &archivedAt.Time
	}
	return row, nil
}
