package services

import (
	"context"
	"database/sql"
	"errors"

	"windshift/internal/database"
	"windshift/internal/validation"
)

// Item validation and its permission collaborators must read the connection
// already owned by the update. This view is local to those read-only checks.
type itemValidationDatabase struct {
	database.Database
	tx  database.Tx
	ctx context.Context
}

var errItemValidationWrite = errors.New("item validation cannot write or own transactions")

func (db *itemValidationDatabase) Query(query string, args ...any) (*sql.Rows, error) {
	return db.tx.QueryContext(db.ctx, query, args...)
}

func (db *itemValidationDatabase) QueryRow(query string, args ...any) *sql.Row {
	return db.tx.QueryRowContext(db.ctx, query, args...)
}

func (db *itemValidationDatabase) QueryContext(_ context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.tx.QueryContext(db.ctx, query, args...)
}

func (db *itemValidationDatabase) QueryRowContext(_ context.Context, query string, args ...any) *sql.Row {
	return db.tx.QueryRowContext(db.ctx, query, args...)
}

func (db *itemValidationDatabase) Begin() (database.Tx, error) {
	return nil, errItemValidationWrite
}

func (db *itemValidationDatabase) BeginTx(context.Context, *sql.TxOptions) (database.Tx, error) {
	return db.Begin()
}

func (db *itemValidationDatabase) Exec(string, ...any) (sql.Result, error) {
	return nil, errItemValidationWrite
}

func (db *itemValidationDatabase) ExecWrite(string, ...any) (sql.Result, error) {
	return nil, errItemValidationWrite
}

func (db *itemValidationDatabase) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errItemValidationWrite
}

func (db *itemValidationDatabase) ExecWriteContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errItemValidationWrite
}

func (db *itemValidationDatabase) Close() error { return errItemValidationWrite }

func (db *itemValidationDatabase) Initialize() error { return errItemValidationWrite }

type itemUpdateCycleChecker struct {
	hierarchy *HierarchyService
	tx        database.Tx
	ctx       context.Context
}

func (c itemUpdateCycleChecker) WouldCreateCycle(itemID, parentID int) (bool, error) {
	return c.hierarchy.WouldCreateCycleTxContext(c.ctx, c.tx, itemID, parentID)
}

func (s *ItemUpdateService) transactionValidator(ctx context.Context, tx database.Tx, updates ...map[string]any) (*validation.ItemFieldValidator, func(), error) {
	db := &itemValidationDatabase{Database: s.db, tx: tx, ctx: ctx}
	v := s.validator.WithDatabase(db).WithCycleChecker(itemUpdateCycleChecker{NewHierarchyService(s.db), tx, ctx})
	needsPermissions := false
	for _, fields := range updates {
		if hasAnyItemUpdateField(fields, "parent_id", "related_work_item_id", "project_id", "time_project_id", "assignee_id") {
			needsPermissions = true
			break
		}
	}
	if s.permissions == nil || !needsPermissions {
		return v, func() {}, nil
	}
	// Do not reuse or publish shared permission snapshots from an uncommitted
	// transaction. The short-lived service has no background warmup.
	permissions, err := NewPermissionService(db, PermissionCacheConfig{MaxEntries: 32, BatchSize: 100})
	if err != nil {
		return nil, nil, err
	}
	v.WithPermissionChecker(permissions).
		WithProjectAccessChecker(NewTimePermissionService(db, permissions)).
		WithWorkspaceAssigneeChecker(NewWorkspaceUserResolver(db, permissions))
	return v, func() { _ = permissions.Close() }, nil
}
