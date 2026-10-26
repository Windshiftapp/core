package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// QueueRepository persists support-queue definitions (WI-1603). Queues are
// scoped to a collection or, when collectionID is nil, to the workspace
// default view.
type QueueRepository struct {
	db database.Database
}

// NewQueueRepository creates a queue repository.
func NewQueueRepository(db database.Database) *QueueRepository {
	return &QueueRepository{db: db}
}

const queueColumns = `id, workspace_id, collection_id, name, ql_query, filter_state,
	position, created_by, builtin_key, is_hidden, created_at, updated_at`

// queueScopeClause builds the WHERE fragment for a queue scope. A nil
// collectionID selects the workspace default view (collection_id IS NULL). A
// collection scope is identified by collection_id alone so a collection that
// spans multiple workspaces keeps one set of views.
func queueScopeClause(workspaceID int, collectionID *int) (where string, args []any) {
	if collectionID == nil {
		return "workspace_id = ? AND collection_id IS NULL", []any{workspaceID}
	}
	return "collection_id = ?", []any{*collectionID}
}

// nullableWorkspaceID maps the zero value to SQL NULL so a collection scope
// with no single workspace stores no workspace reference.
func nullableWorkspaceID(id int) any {
	if id <= 0 {
		return nil
	}
	return id
}

// ListByScope returns every queue row in the scope ordered by position, id.
// The result mixes custom queues and built-in dismissal rows; callers split
// them on BuiltinKey.
func (r *QueueRepository) ListByScope(workspaceID int, collectionID *int) ([]models.Queue, error) {
	where, args := queueScopeClause(workspaceID, collectionID)
	rows, err := r.db.Query(
		"SELECT "+queueColumns+" FROM queues WHERE "+where+" ORDER BY position, id", args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list queues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanQueues(rows)
}

// GetByID loads one queue by primary key. Returns ErrNotFound when missing.
func (r *QueueRepository) GetByID(id int) (*models.Queue, error) {
	return r.getQueue("id = ?", id)
}

// GetByBuiltinKey loads the dismissal or override row for a built-in preset
// in the scope. Returns ErrNotFound when the preset is not overridden.
func (r *QueueRepository) GetByBuiltinKey(workspaceID int, collectionID *int, builtinKey string) (*models.Queue, error) {
	where, args := queueScopeClause(workspaceID, collectionID)
	return r.getQueue(where+" AND builtin_key = ?", append(args, builtinKey)...)
}

func (r *QueueRepository) getQueue(where string, args ...any) (*models.Queue, error) {
	var q models.Queue
	var workspaceID, collectionID, createdBy sql.NullInt64
	var filterState, builtinKey sql.NullString
	err := r.db.QueryRow(
		"SELECT "+queueColumns+" FROM queues WHERE "+where, args...,
	).Scan(&q.ID, &workspaceID, &collectionID, &q.Name, &q.QLQuery, &filterState,
		&q.Position, &createdBy, &builtinKey, &q.IsHidden, &q.CreatedAt, &q.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get queue: %w", err)
	}
	if workspaceID.Valid {
		q.WorkspaceID = int(workspaceID.Int64)
	}
	applyQueueNullables(&q, collectionID, createdBy, filterState, builtinKey)
	return &q, nil
}

// NextPosition returns the position after the last queue in the scope, used
// when appending a newly created queue.
func (r *QueueRepository) NextPosition(workspaceID int, collectionID *int) (int, error) {
	where, args := queueScopeClause(workspaceID, collectionID)
	var maxPosition sql.NullInt64
	if err := r.db.QueryRow(
		"SELECT MAX(position) FROM queues WHERE "+where, args...,
	).Scan(&maxPosition); err != nil {
		return 0, fmt.Errorf("next queue position: %w", err)
	}
	if !maxPosition.Valid {
		return 0, nil
	}
	return int(maxPosition.Int64) + 1, nil
}

// Create inserts a queue and returns the persisted row. Returns
// ErrDuplicateEntry when a dismissal already exists for the built-in key.
func (r *QueueRepository) Create(q *models.Queue) (*models.Queue, error) {
	now := time.Now()
	q.CreatedAt, q.UpdatedAt = now, now
	var id int
	err := database.WithTx(r.db, func(tx database.Tx) error {
		if err := lockQueueCollectionScope(tx, q.WorkspaceID, q.CollectionID); err != nil {
			return err
		}
		return tx.QueryRow(`
			INSERT INTO queues
				(workspace_id, collection_id, name, ql_query, filter_state, position, created_by, builtin_key, is_hidden, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
		`, nullableWorkspaceID(q.WorkspaceID), q.CollectionID, q.Name, q.QLQuery, q.FilterState, q.Position,
			q.CreatedBy, q.BuiltinKey, q.IsHidden, now, now).Scan(&id)
	})
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrDuplicateEntry
		}
		return nil, fmt.Errorf("create queue: %w", err)
	}
	q.ID = id
	return q, nil
}

// lockQueueCollectionScope serializes queue writes with collection scope
// changes. A stale workspace is rejected after any in-flight move commits; a
// collection with no single workspace locks by id alone.
func lockQueueCollectionScope(tx database.Tx, workspaceID int, collectionID *int) error {
	if collectionID == nil {
		return nil
	}
	if workspaceID <= 0 {
		var currentID int
		err := tx.QueryRow(`
			UPDATE collections SET updated_at = updated_at
			WHERE id = ? RETURNING id
		`, *collectionID).Scan(&currentID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	var currentWorkspace int
	err := tx.QueryRow(`
		UPDATE collections SET updated_at = updated_at
		WHERE id = ? AND workspace_id = ? RETURNING workspace_id
	`, *collectionID, workspaceID).Scan(&currentWorkspace)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// SetBuiltinHidden persists or removes a built-in queue dismissal while
// holding the collection scope lock through the whole operation.
func (r *QueueRepository) SetBuiltinHidden(q *models.Queue, hidden bool) error {
	if q == nil || q.BuiltinKey == nil {
		return errors.New("built-in queue key is required")
	}
	return database.WithTx(r.db, func(tx database.Tx) error {
		if err := lockQueueCollectionScope(tx, q.WorkspaceID, q.CollectionID); err != nil {
			return err
		}
		where, args := queueScopeClause(q.WorkspaceID, q.CollectionID)
		args = append(args, *q.BuiltinKey)
		if !hidden {
			_, err := tx.Exec("DELETE FROM queues WHERE "+where+" AND builtin_key = ?", args...)
			return err
		}
		var id int
		var isHidden bool
		err := tx.QueryRow("SELECT id, is_hidden FROM queues WHERE "+where+" AND builtin_key = ?", args...).Scan(&id, &isHidden)
		if err == nil {
			if isHidden {
				return nil
			}
			_, err = tx.Exec("UPDATE queues SET is_hidden = ?, updated_at = ? WHERE id = ?", true, time.Now(), id)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(`
			INSERT INTO queues
				(workspace_id, collection_id, name, ql_query, position, created_by, builtin_key, is_hidden, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, nullableWorkspaceID(q.WorkspaceID), q.CollectionID, q.Name, q.QLQuery, q.Position, q.CreatedBy, q.BuiltinKey, true)
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return err
	})
}

// Update overwrites the mutable fields of a queue row.
func (r *QueueRepository) Update(q *models.Queue) error {
	q.UpdatedAt = time.Now()
	if _, err := r.db.ExecWrite(`
		UPDATE queues
		SET name = ?, ql_query = ?, filter_state = ?, position = ?, is_hidden = ?, updated_at = ?
		WHERE id = ?
	`, q.Name, q.QLQuery, q.FilterState, q.Position, q.IsHidden, q.UpdatedAt, q.ID); err != nil {
		return fmt.Errorf("update queue %d: %w", q.ID, err)
	}
	return nil
}

// Delete removes a queue row by primary key.
func (r *QueueRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM queues WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete queue %d: %w", id, err)
	}
	return nil
}

// DeleteByBuiltinKey removes a built-in override or dismissal from the scope,
// restoring the virtual preset.
func (r *QueueRepository) DeleteByBuiltinKey(workspaceID int, collectionID *int, builtinKey string) error {
	where, args := queueScopeClause(workspaceID, collectionID)
	if _, err := r.db.ExecWrite(
		"DELETE FROM queues WHERE "+where+" AND builtin_key = ?", append(args, builtinKey)...,
	); err != nil {
		return fmt.Errorf("delete queue override %q: %w", builtinKey, err)
	}
	return nil
}

// Reorder assigns sequential positions to the given queue ids within the
// scope in one transaction. Ids outside the scope are ignored.
func (r *QueueRepository) Reorder(workspaceID int, collectionID *int, orderedIDs []int) error {
	where, args := queueScopeClause(workspaceID, collectionID)
	return database.WithTx(r.db, func(tx database.Tx) error {
		now := time.Now()
		for position, id := range orderedIDs {
			if _, err := tx.Exec(
				"UPDATE queues SET position = ?, updated_at = ? WHERE id = ? AND "+where,
				append([]any{position, now, id}, args...)...,
			); err != nil {
				return fmt.Errorf("reorder queue %d: %w", id, err)
			}
		}
		return nil
	})
}

func scanQueues(rows *sql.Rows) ([]models.Queue, error) {
	result := []models.Queue{}
	for rows.Next() {
		var q models.Queue
		var workspaceID, collectionID, createdBy sql.NullInt64
		var filterState, builtinKey sql.NullString
		if err := rows.Scan(&q.ID, &workspaceID, &collectionID, &q.Name, &q.QLQuery, &filterState,
			&q.Position, &createdBy, &builtinKey, &q.IsHidden, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan queue: %w", err)
		}
		if workspaceID.Valid {
			q.WorkspaceID = int(workspaceID.Int64)
		}
		applyQueueNullables(&q, collectionID, createdBy, filterState, builtinKey)
		result = append(result, q)
	}
	return result, rows.Err()
}

func applyQueueNullables(q *models.Queue, collectionID, createdBy sql.NullInt64, filterState, builtinKey sql.NullString) {
	if collectionID.Valid {
		v := int(collectionID.Int64)
		q.CollectionID = &v
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		q.CreatedBy = &v
	}
	if filterState.Valid {
		v := filterState.String
		q.FilterState = &v
	}
	if builtinKey.Valid {
		v := builtinKey.String
		q.BuiltinKey = &v
	}
}
