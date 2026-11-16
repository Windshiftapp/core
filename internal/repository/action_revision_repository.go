package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ActionRevisionRepository reads and writes immutable action definition
// revisions. Revisions are never updated or deleted by application code, and
// they intentionally survive action deletion so an execution log can always be
// traced back to the definition that ran.
type ActionRevisionRepository struct {
	db database.Database
}

// NewActionRevisionRepository creates an ActionRevisionRepository.
func NewActionRevisionRepository(db database.Database) *ActionRevisionRepository {
	return &ActionRevisionRepository{db: db}
}

// CreateTx inserts a revision inside the caller's transaction. The caller owns
// the transaction so the revision commits or rolls back atomically with the
// action mutation and its audit event.
func (r *ActionRevisionRepository) CreateTx(tx database.Tx, revision *models.ActionDefinitionRevision) (int, error) {
	encoded, err := json.Marshal(revision.Snapshot)
	if err != nil {
		return 0, fmt.Errorf("encode action definition snapshot: %w", err)
	}
	changedFields, err := json.Marshal(revision.ChangedFields)
	if err != nil {
		return 0, fmt.Errorf("encode changed fields: %w", err)
	}
	createdAt := revision.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	var id int
	if err := tx.QueryRow(`
		INSERT INTO action_definition_revisions (
			action_id, workspace_id, revision_number, source, editor_user_id,
			editor_username, editor_source, change_summary, changed_fields, snapshot, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		revision.ActionID, revision.WorkspaceID, revision.RevisionNumber, revision.Source,
		revision.EditorUserID, revision.EditorUsername, revision.EditorSource,
		revision.ChangeSummary, string(changedFields), string(encoded), createdAt,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("insert action definition revision: %w", err)
	}
	return id, nil
}

// NextRevisionNumberTx returns the next per-action revision number inside the
// caller's transaction. The action row must already be locked by a preceding
// UPDATE (or be newly created), which serializes concurrent edits per action.
func (r *ActionRevisionRepository) NextRevisionNumberTx(tx database.Tx, actionID int) (int, error) {
	var next int
	if err := tx.QueryRow(`
		SELECT COALESCE(MAX(revision_number), 0) + 1
		FROM action_definition_revisions
		WHERE action_id = ?
	`, actionID).Scan(&next); err != nil {
		return 0, fmt.Errorf("compute next revision number for action %d: %w", actionID, err)
	}
	return next, nil
}

// Get returns one revision including its full executable snapshot.
func (r *ActionRevisionRepository) Get(id int) (*models.ActionDefinitionRevision, error) {
	row := r.db.QueryRow(`
		SELECT id, action_id, workspace_id, revision_number, source, editor_user_id,
		       editor_username, editor_source, change_summary, changed_fields, snapshot, created_at
		FROM action_definition_revisions
		WHERE id = ?
	`, id)
	revision, err := scanActionDefinitionRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return revision, nil
}

// GetByActionAndNumber returns one revision scoped to its action.
func (r *ActionRevisionRepository) GetByActionAndNumber(actionID, revisionNumber int) (*models.ActionDefinitionRevision, error) {
	row := r.db.QueryRow(`
		SELECT id, action_id, workspace_id, revision_number, source, editor_user_id,
		       editor_username, editor_source, change_summary, changed_fields, snapshot, created_at
		FROM action_definition_revisions
		WHERE action_id = ? AND revision_number = ?
	`, actionID, revisionNumber)
	revision, err := scanActionDefinitionRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return revision, nil
}

// GetByIDForAction returns a revision by primary key, but only when it belongs
// to the given action and workspace. The workspace predicate keeps retained
// history of a deleted action readable without trusting a surviving row.
func (r *ActionRevisionRepository) GetByIDForAction(id, actionID, workspaceID int) (*models.ActionDefinitionRevision, error) {
	row := r.db.QueryRow(`
		SELECT id, action_id, workspace_id, revision_number, source, editor_user_id,
		       editor_username, editor_source, change_summary, changed_fields, snapshot, created_at
		FROM action_definition_revisions
		WHERE id = ? AND action_id = ? AND workspace_id = ?
	`, id, actionID, workspaceID)
	revision, err := scanActionDefinitionRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return revision, nil
}

// LatestByAction returns the highest-numbered revision for an action.
func (r *ActionRevisionRepository) LatestByAction(actionID int) (*models.ActionDefinitionRevision, error) {
	return r.latestByAction(r.db, actionID)
}

// LatestByActionTx returns the highest-numbered revision for an action inside
// the caller's transaction, so an update can diff against the prior snapshot
// without leaving the transaction.
func (r *ActionRevisionRepository) LatestByActionTx(tx database.Tx, actionID int) (*models.ActionDefinitionRevision, error) {
	return r.latestByAction(tx, actionID)
}

func (r *ActionRevisionRepository) latestByAction(query interface {
	QueryRow(query string, args ...any) *sql.Row
}, actionID int) (*models.ActionDefinitionRevision, error) {
	row := query.QueryRow(`
		SELECT id, action_id, workspace_id, revision_number, source, editor_user_id,
		       editor_username, editor_source, change_summary, changed_fields, snapshot, created_at
		FROM action_definition_revisions
		WHERE action_id = ?
		ORDER BY revision_number DESC
		LIMIT 1
	`, actionID)
	revision, err := scanActionDefinitionRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return revision, nil
}

// ListSummariesByAction returns a page of revision metadata, newest first,
// plus the total count for the action within the workspace. Filtering by
// workspace keeps the history readable after the action row is deleted.
func (r *ActionRevisionRepository) ListSummariesByAction(actionID, workspaceID, limit, offset int) ([]models.ActionDefinitionRevisionSummary, int, error) {
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM action_definition_revisions WHERE action_id = ? AND workspace_id = ?`, actionID, workspaceID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count action definition revisions: %w", err)
	}
	rows, err := r.db.Query(`
		SELECT id, action_id, workspace_id, revision_number, source, editor_user_id,
		       editor_username, editor_source, change_summary, changed_fields, created_at
		FROM action_definition_revisions
		WHERE action_id = ? AND workspace_id = ?
		ORDER BY revision_number DESC
		LIMIT ? OFFSET ?
	`, actionID, workspaceID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list action definition revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	summaries := make([]models.ActionDefinitionRevisionSummary, 0)
	for rows.Next() {
		var summary models.ActionDefinitionRevisionSummary
		var editorUserID sql.NullInt64
		var changedFieldsJSON string
		if err := rows.Scan(
			&summary.ID, &summary.ActionID, &summary.WorkspaceID, &summary.RevisionNumber,
			&summary.Source, &editorUserID, &summary.EditorUsername, &summary.EditorSource,
			&summary.ChangeSummary, &changedFieldsJSON, &summary.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan action definition revision: %w", err)
		}
		if editorUserID.Valid {
			value := int(editorUserID.Int64)
			summary.EditorUserID = &value
		}
		summary.ChangedFields = decodeChangedFields(changedFieldsJSON)
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate action definition revisions: %w", err)
	}
	return summaries, total, nil
}

// CountByAction reports how many revisions an action has.
func (r *ActionRevisionRepository) CountByAction(actionID int) (int, error) {
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM action_definition_revisions WHERE action_id = ?`, actionID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count action definition revisions: %w", err)
	}
	return count, nil
}

func scanActionDefinitionRevision(scanner interface{ Scan(dest ...any) error }) (*models.ActionDefinitionRevision, error) {
	var revision models.ActionDefinitionRevision
	var editorUserID sql.NullInt64
	var changedFieldsJSON, snapshotJSON string
	if err := scanner.Scan(
		&revision.ID, &revision.ActionID, &revision.WorkspaceID, &revision.RevisionNumber,
		&revision.Source, &editorUserID, &revision.EditorUsername, &revision.EditorSource,
		&revision.ChangeSummary, &changedFieldsJSON, &snapshotJSON, &revision.CreatedAt,
	); err != nil {
		return nil, err
	}
	if editorUserID.Valid {
		value := int(editorUserID.Int64)
		revision.EditorUserID = &value
	}
	revision.ChangedFields = decodeChangedFields(changedFieldsJSON)
	if err := json.Unmarshal([]byte(snapshotJSON), &revision.Snapshot); err != nil {
		return nil, fmt.Errorf("decode action definition snapshot for revision %d: %w", revision.ID, err)
	}
	return &revision, nil
}

func decodeChangedFields(raw string) []string {
	fields := []string{}
	if raw == "" {
		return fields
	}
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return []string{}
	}
	if fields == nil {
		return []string{}
	}
	return fields
}
