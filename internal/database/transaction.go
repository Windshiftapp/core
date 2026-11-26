package database

import (
	"context"
	"database/sql"
)

// SQLiteTx wraps a *sql.Tx for SQLite (no placeholder conversion needed)
// last review: ser, 210426
type SQLiteTx struct {
	tx *sql.Tx
	txEffectState
}

// NewSQLiteTx creates a new SQLite transaction wrapper
func NewSQLiteTx(tx *sql.Tx) Tx {
	return &SQLiteTx{tx: tx}
}

func (t *SQLiteTx) Query(query string, args ...any) (*sql.Rows, error) {
	return t.tx.Query(query, toUTCArgs(args)...)
}

func (t *SQLiteTx) QueryRow(query string, args ...any) *sql.Row {
	return t.tx.QueryRow(query, toUTCArgs(args)...)
}

func (t *SQLiteTx) Exec(query string, args ...any) (sql.Result, error) {
	return t.tx.Exec(query, toUTCArgs(args)...)
}

func (t *SQLiteTx) ExecWrite(query string, args ...any) (sql.Result, error) {
	return t.tx.Exec(query, toUTCArgs(args)...)
}

func (t *SQLiteTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, query, toUTCArgs(args)...)
}

func (t *SQLiteTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, toUTCArgs(args)...)
}

func (t *SQLiteTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, toUTCArgs(args)...)
}

func (t *SQLiteTx) ExecWriteContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, toUTCArgs(args)...)
}

func (t *SQLiteTx) QueryWrite(query string, args ...any) (*sql.Rows, error) {
	return t.tx.Query(query, toUTCArgs(args)...)
}

func (t *SQLiteTx) QueryWriteContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, query, toUTCArgs(args)...)
}

func (t *SQLiteTx) QueryRowWrite(query string, args ...any) *sql.Row {
	return t.tx.QueryRow(query, toUTCArgs(args)...)
}

func (t *SQLiteTx) QueryRowWriteContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, toUTCArgs(args)...)
}

func (t *SQLiteTx) Prepare(query string) (*sql.Stmt, error) {
	return t.tx.Prepare(query)
}

func (t *SQLiteTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.tx.PrepareContext(ctx, query)
}

func (t *SQLiteTx) Commit() error {
	if err := t.tx.Commit(); err != nil {
		t.discard()
		return err
	}
	t.publish()
	return nil
}

func (t *SQLiteTx) Rollback() error {
	err := t.tx.Rollback()
	t.discard()
	return err
}

// PostgresTx wraps a *sql.Tx for PostgreSQL with placeholder conversion
type PostgresTx struct {
	tx *sql.Tx
	txEffectState
}

// NewPostgresTx creates a new PostgreSQL transaction wrapper
func NewPostgresTx(tx *sql.Tx) Tx {
	return &PostgresTx{tx: tx}
}

func (t *PostgresTx) Query(query string, args ...any) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return t.tx.Query(query, args...)
}

func (t *PostgresTx) QueryRow(query string, args ...any) *sql.Row {
	query = ConvertPlaceholders(query)
	return t.tx.QueryRow(query, args...)
}

func (t *PostgresTx) Exec(query string, args ...any) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return t.tx.Exec(query, args...)
}

func (t *PostgresTx) ExecWrite(query string, args ...any) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return t.tx.Exec(query, args...)
}

func (t *PostgresTx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return t.tx.QueryContext(ctx, query, args...)
}

func (t *PostgresTx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	query = ConvertPlaceholders(query)
	return t.tx.QueryRowContext(ctx, query, args...)
}

func (t *PostgresTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return t.tx.ExecContext(ctx, query, args...)
}

func (t *PostgresTx) ExecWriteContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return t.tx.ExecContext(ctx, query, args...)
}

func (t *PostgresTx) QueryWrite(query string, args ...any) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return t.tx.Query(query, args...)
}

func (t *PostgresTx) QueryWriteContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return t.tx.QueryContext(ctx, query, args...)
}

func (t *PostgresTx) QueryRowWrite(query string, args ...any) *sql.Row {
	query = ConvertPlaceholders(query)
	return t.tx.QueryRow(query, args...)
}

func (t *PostgresTx) QueryRowWriteContext(ctx context.Context, query string, args ...any) *sql.Row {
	query = ConvertPlaceholders(query)
	return t.tx.QueryRowContext(ctx, query, args...)
}

func (t *PostgresTx) Prepare(query string) (*sql.Stmt, error) {
	query = ConvertPlaceholders(query)
	return t.tx.Prepare(query)
}

func (t *PostgresTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	query = ConvertPlaceholders(query)
	return t.tx.PrepareContext(ctx, query)
}

func (t *PostgresTx) Commit() error {
	if err := t.tx.Commit(); err != nil {
		t.discard()
		return err
	}
	t.publish()
	return nil
}

func (t *PostgresTx) Rollback() error {
	err := t.tx.Rollback()
	t.discard()
	return err
}
