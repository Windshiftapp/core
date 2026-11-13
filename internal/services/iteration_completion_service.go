package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
)

var (
	ErrIterationCompletionNotFound  = errors.New("iteration not found")
	ErrIterationCompletionForbidden = errors.New("iteration completion forbidden")
	ErrIterationCompletionConflict  = errors.New("iteration completion conflict")
	ErrIterationCompletionLimit     = errors.New("iteration completion item limit exceeded")
)

//nolint:misspell // British spelling is the persisted iteration status value.
const iterationStatusCancelled = "cancelled"

type CompleteIterationRequest struct {
	IterationID        int
	TargetIterationID  *int
	UserID             int
	AuthorizeWorkspace func(workspaceID int) (bool, error)
	AuthorizeGlobal    func() (bool, error)
}

type CompleteIterationResult struct {
	IterationID       int                `json:"iteration_id"`
	TargetIterationID *int               `json:"target_iteration_id"`
	Status            string             `json:"status"`
	AlreadyCompleted  bool               `json:"already_completed"`
	MovedCount        int                `json:"moved_count"`
	Items             []*models.Item     `json:"items"`
	Updates           []UpdateItemResult `json:"-"`
	SQLStatements     int                `json:"sql_statements"`
	DurationMS        int64              `json:"duration_ms"`
}

type iterationCompletionScope struct {
	id          int
	status      string
	isGlobal    bool
	workspaceID *int
}

type incompleteIterationItem struct {
	id          int
	workspaceID int
}

type completionAuthorization struct {
	req              CompleteIterationRequest
	workspaceResults map[int]bool
	globalChecked    bool
	globalAllowed    bool
	sealed           bool
}

type iterationCompletionReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type IterationCompletionService struct {
	db database.Database
}

func NewIterationCompletionService(db database.Database) *IterationCompletionService {
	return &IterationCompletionService{db: db}
}

// Complete atomically moves every incomplete item to the requested iteration
// (or backlog when nil) and marks the source iteration completed. Selection,
// history, the item move, and the iteration state change are set-based and
// share one transaction. Assignment semantics make retries idempotent; once
// completed, the endpoint returns success without replaying item side effects.
func (s *IterationCompletionService) Complete(ctx context.Context, req CompleteIterationRequest) (*CompleteIterationResult, error) {
	started := time.Now()
	result := &CompleteIterationResult{
		IterationID:       req.IterationID,
		TargetIterationID: req.TargetIterationID,
		Status:            "completed",
		Items:             []*models.Item{},
	}
	authorization, preflightQueries, err := s.prepareCompletionAuthorization(ctx, req)
	if err != nil {
		return nil, err
	}
	result.SQLStatements = preflightQueries
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("start iteration completion: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	source, err := s.loadScopeForUpdate(ctx, tx, req.IterationID)
	result.SQLStatements++
	if err != nil {
		return nil, err
	}
	if err := authorization.authorizeIterationScope(source); err != nil {
		return nil, err
	}
	if source.status == "completed" {
		result.AlreadyCompleted = true
		result.DurationMS = time.Since(started).Milliseconds()
		return result, nil
	}
	if source.status == iterationStatusCancelled {
		return nil, fmt.Errorf("%w: canceled iterations cannot be completed", ErrIterationCompletionConflict)
	}

	var target *iterationCompletionScope
	if req.TargetIterationID != nil {
		if *req.TargetIterationID == req.IterationID {
			return nil, fmt.Errorf("%w: target iteration must differ from source", ErrIterationCompletionConflict)
		}
		target, err = s.loadScopeForUpdate(ctx, tx, *req.TargetIterationID)
		result.SQLStatements++
		if err != nil {
			return nil, err
		}
		if target.status == "completed" || target.status == iterationStatusCancelled {
			return nil, fmt.Errorf("%w: target iteration is not open", ErrIterationCompletionConflict)
		}
		if err := authorization.authorizeTargetScope(target); err != nil {
			return nil, err
		}
	}

	incomplete, err := s.loadIncompleteItemsForUpdate(ctx, tx, req.IterationID)
	result.SQLStatements++
	if err != nil {
		return nil, err
	}
	if len(incomplete) > MaxBulkItemUpdates {
		return nil, ErrIterationCompletionLimit
	}
	workspaceIDs := map[int]struct{}{}
	for _, item := range incomplete {
		workspaceIDs[item.workspaceID] = struct{}{}
		if target != nil && !target.isGlobal && (target.workspaceID == nil || *target.workspaceID != item.workspaceID) {
			return nil, fmt.Errorf("%w: target iteration does not belong to every item workspace", ErrIterationCompletionConflict)
		}
	}
	for workspaceID := range workspaceIDs {
		if err := authorization.authorizeWorkspace(workspaceID); err != nil {
			return nil, err
		}
	}

	changedIDs := make([]int, len(incomplete))
	for i, item := range incomplete {
		changedIDs[i] = item.id
	}
	if len(changedIDs) > 0 {
		now := time.Now()
		newValue := ""
		if req.TargetIterationID != nil {
			newValue = fmt.Sprintf("%d", *req.TargetIterationID)
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(changedIDs)), ",")
		historyArgs := make([]any, 0, len(changedIDs)+4)
		historyArgs = append(historyArgs, req.UserID, newValue, now)
		for _, id := range changedIDs {
			historyArgs = append(historyArgs, id)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO item_history (item_id, user_id, field_name, old_value, new_value, changed_at)
			SELECT i.id, ?, 'iteration_id', CAST(i.iteration_id AS TEXT), ?, ?
			FROM items i
			WHERE i.id IN (`+placeholders+`)
		`, historyArgs...)
		result.SQLStatements++
		if err != nil {
			return nil, fmt.Errorf("record iteration completion history: %w", err)
		}

		updateArgs := make([]any, 0, len(changedIDs)+3)
		updateArgs = append(updateArgs, req.TargetIterationID, now, now)
		for _, id := range changedIDs {
			updateArgs = append(updateArgs, id)
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE items
			SET iteration_id = ?, updated_at = ?, last_active_at = ?
			WHERE id IN (`+placeholders+`)
		`, updateArgs...)
		result.SQLStatements++
		if err != nil {
			return nil, fmt.Errorf("move incomplete iteration items: %w", err)
		}

		updatedItems, err := repository.NewItemRepository(s.db).FindByIDsForUpdateContext(ctx, tx, changedIDs)
		result.SQLStatements++
		if err != nil {
			return nil, fmt.Errorf("load iteration completion item events: %w", err)
		}
		metadata := itemevents.User(req.UserID, "application")
		metadata.OccurredAt = now
		newIterationValue := any(nil)
		if req.TargetIterationID != nil {
			newIterationValue = *req.TargetIterationID
		}
		records := make([]itemevents.UpdateRecord, 0, len(updatedItems))
		for _, item := range updatedItems {
			records = append(records, itemevents.UpdateRecord{
				Item: item,
				Changes: []itemevents.FieldChange{{
					Field: "iteration_id", OldValue: req.IterationID, NewValue: newIterationValue,
				}},
				Metadata: metadata,
			})
		}
		if _, err := itemevents.NewRecorder(s.db).UpdatedBatch(ctx, tx, records); err != nil {
			return nil, err
		}
		result.SQLStatements += 2
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE iterations SET status = 'completed', updated_at = ? WHERE id = ?
	`, time.Now(), req.IterationID)
	result.SQLStatements++
	if err != nil {
		return nil, fmt.Errorf("complete iteration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit iteration completion: %w", err)
	}

	if len(changedIDs) > 0 {
		items, loadErr := repository.NewItemRepository(s.db).FindByIDsWithDetails(changedIDs)
		result.SQLStatements += 2 // detail rows plus batched milestone attachment
		if loadErr != nil {
			return nil, fmt.Errorf("load completed iteration items: %w", loadErr)
		}
		result.Items = items
		result.MovedCount = len(items)
		result.Updates = make([]UpdateItemResult, 0, len(items))
		for _, updated := range items {
			original := *updated
			original.IterationID = intPtr(req.IterationID)
			history := []HistoryEntry{{
				ItemID: updated.ID, UserID: req.UserID, FieldName: "iteration_id",
				OldValue: fmt.Sprintf("%d", req.IterationID), NewValue: nullableIntString(req.TargetIterationID),
				ChangedAt: time.Now(),
			}}
			result.Updates = append(result.Updates, UpdateItemResult{OriginalItem: &original, Item: updated, FieldChanges: history})
			PublishItemChange(updated.ID)
			PublishWorkspaceChange(updated.WorkspaceID)
		}
	}
	result.DurationMS = time.Since(started).Milliseconds()
	return result, nil
}

func (s *IterationCompletionService) loadScopeForUpdate(ctx context.Context, tx database.Tx, id int) (*iterationCompletionScope, error) {
	return s.loadCompletionScope(ctx, tx, id, true)
}

func (s *IterationCompletionService) loadCompletionScope(ctx context.Context, q iterationCompletionReader, id int, lock bool) (*iterationCompletionScope, error) {
	query := `SELECT id, status, is_global, workspace_id FROM iterations WHERE id = ?`
	if lock && s.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE"
	}
	var scope iterationCompletionScope
	var workspaceID sql.NullInt64
	if err := q.QueryRowContext(ctx, query, id).Scan(&scope.id, &scope.status, &scope.isGlobal, &workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrIterationCompletionNotFound
		}
		return nil, fmt.Errorf("load iteration completion scope: %w", err)
	}
	if workspaceID.Valid {
		scope.workspaceID = intPtr(int(workspaceID.Int64))
	}
	return &scope, nil
}

func (s *IterationCompletionService) loadIncompleteItemsForUpdate(ctx context.Context, tx database.Tx, iterationID int) ([]incompleteIterationItem, error) {
	return s.loadIncompleteItems(ctx, tx, iterationID, true)
}

func (s *IterationCompletionService) loadIncompleteItems(ctx context.Context, q iterationCompletionReader, iterationID int, lock bool) ([]incompleteIterationItem, error) {
	query := `
		SELECT i.id, i.workspace_id
		FROM items i
		LEFT JOIN statuses st ON st.id = i.status_id
		LEFT JOIN status_categories sc ON sc.id = st.category_id
		WHERE i.iteration_id = ? AND COALESCE(sc.is_completed, false) = false
		ORDER BY i.id
		LIMIT ?`
	if lock && s.db.GetDriverName() == "postgres" {
		query += " FOR UPDATE OF i"
	}
	rows, err := q.QueryContext(ctx, query, iterationID, MaxBulkItemUpdates+1)
	if err != nil {
		return nil, fmt.Errorf("load incomplete iteration items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]incompleteIterationItem, 0)
	for rows.Next() {
		var item incompleteIterationItem
		if err := rows.Scan(&item.id, &item.workspaceID); err != nil {
			return nil, fmt.Errorf("scan incomplete iteration item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incomplete iteration items: %w", err)
	}
	return items, nil
}

// Resolve callback-backed permissions without owning a transaction. The locked
// reads below may use only these decisions; newly observed scopes fail closed.
func (s *IterationCompletionService) prepareCompletionAuthorization(ctx context.Context, req CompleteIterationRequest) (*completionAuthorization, int, error) {
	a := &completionAuthorization{req: req, workspaceResults: map[int]bool{}}
	source, err := s.loadCompletionScope(ctx, s.db, req.IterationID, false)
	queries := 1
	if err != nil {
		return nil, queries, err
	}
	if err := a.authorizeIterationScope(source); err != nil {
		return nil, queries, err
	}
	if source.status == "completed" {
		a.sealed = true
		return a, queries, nil
	}
	if source.status == iterationStatusCancelled {
		return nil, queries, fmt.Errorf("%w: canceled iterations cannot be completed", ErrIterationCompletionConflict)
	}
	var target *iterationCompletionScope
	if req.TargetIterationID != nil {
		if *req.TargetIterationID == req.IterationID {
			return nil, queries, fmt.Errorf("%w: target iteration must differ from source", ErrIterationCompletionConflict)
		}
		target, err = s.loadCompletionScope(ctx, s.db, *req.TargetIterationID, false)
		queries++
		if err != nil {
			return nil, queries, err
		}
		if target.status == "completed" || target.status == iterationStatusCancelled {
			return nil, queries, fmt.Errorf("%w: target iteration is not open", ErrIterationCompletionConflict)
		}
		if err := a.authorizeTargetScope(target); err != nil {
			return nil, queries, err
		}
	}
	items, err := s.loadIncompleteItems(ctx, s.db, req.IterationID, false)
	queries++
	if err != nil {
		return nil, queries, err
	}
	if len(items) > MaxBulkItemUpdates {
		return nil, queries, ErrIterationCompletionLimit
	}
	for _, item := range items {
		if target != nil && !target.isGlobal && (target.workspaceID == nil || *target.workspaceID != item.workspaceID) {
			return nil, queries, fmt.Errorf("%w: target iteration does not belong to every item workspace", ErrIterationCompletionConflict)
		}
	}
	for _, item := range items {
		if err := a.authorizeWorkspace(item.workspaceID); err != nil {
			return nil, queries, err
		}
	}
	a.sealed = true
	return a, queries, nil
}

func (a *completionAuthorization) authorizeIterationScope(scope *iterationCompletionScope) error {
	if scope.isGlobal {
		if a.globalChecked {
			if !a.globalAllowed {
				return ErrIterationCompletionForbidden
			}
			return nil
		}
		if a.sealed {
			return ErrIterationCompletionForbidden
		}
		a.globalChecked = true
		if a.req.AuthorizeGlobal == nil {
			return ErrIterationCompletionForbidden
		}
		allowed, err := a.req.AuthorizeGlobal()
		if err != nil {
			return fmt.Errorf("authorize global iteration: %w", err)
		}
		a.globalAllowed = allowed
		if !allowed {
			return ErrIterationCompletionForbidden
		}
		return nil
	}
	if scope.workspaceID == nil {
		return ErrIterationCompletionForbidden
	}
	return a.authorizeWorkspace(*scope.workspaceID)
}

// Referencing a global target iteration is allowed to authenticated item
// editors, matching normal item assignment. Completing a global source still
// requires iteration.manage because that mutates the global iteration itself.
func (a *completionAuthorization) authorizeTargetScope(scope *iterationCompletionScope) error {
	if scope.isGlobal {
		return nil
	}
	if scope.workspaceID == nil {
		return ErrIterationCompletionForbidden
	}
	return a.authorizeWorkspace(*scope.workspaceID)
}

func (a *completionAuthorization) authorizeWorkspace(workspaceID int) error {
	if allowed, checked := a.workspaceResults[workspaceID]; checked {
		if !allowed {
			return ErrIterationCompletionForbidden
		}
		return nil
	}
	if a.sealed {
		return ErrIterationCompletionForbidden
	}
	if a.req.AuthorizeWorkspace == nil {
		return ErrIterationCompletionForbidden
	}
	allowed, err := a.req.AuthorizeWorkspace(workspaceID)
	if err != nil {
		return fmt.Errorf("authorize iteration workspace %d: %w", workspaceID, err)
	}
	a.workspaceResults[workspaceID] = allowed
	if !allowed {
		return ErrIterationCompletionForbidden
	}
	return nil
}

func intPtr(value int) *int { return &value }

func nullableIntString(value *int) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}
