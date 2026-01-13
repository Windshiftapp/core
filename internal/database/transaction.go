package database

import (
	"context"
	"database/sql"
	"strings"
)

// SQLiteTx wraps a *sql.Tx for SQLite (no placeholder conversion needed)
type SQLiteTx struct {
	tx *sql.Tx
}

// NewSQLiteTx creates a new SQLite transaction wrapper
func NewSQLiteTx(tx *sql.Tx) Tx {
	return &SQLiteTx{tx: tx}
}

func (t *SQLiteTx) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return t.tx.Query(query, args...)
}

func (t *SQLiteTx) QueryRow(query string, args ...interface{}) *sql.Row {
	return t.tx.QueryRow(query, args...)
}

func (t *SQLiteTx) Exec(query string, args ...interface{}) (sql.Result, error) {
	return t.tx.Exec(query, args...)
}

func (t *SQLiteTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, query, args...)
}

func (t *SQLiteTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, args...)
}

func (t *SQLiteTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}

func (t *SQLiteTx) Prepare(query string) (*sql.Stmt, error) {
	return t.tx.Prepare(query)
}

func (t *SQLiteTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.tx.PrepareContext(ctx, query)
}

func (t *SQLiteTx) Commit() error {
	return t.tx.Commit()
}

func (t *SQLiteTx) Rollback() error {
	return t.tx.Rollback()
}

// PostgresTx wraps a *sql.Tx for PostgreSQL with placeholder conversion
type PostgresTx struct {
	tx *sql.Tx
}

// NewPostgresTx creates a new PostgreSQL transaction wrapper
func NewPostgresTx(tx *sql.Tx) Tx {
	return &PostgresTx{tx: tx}
}

func (t *PostgresTx) Query(query string, args ...interface{}) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return t.tx.Query(query, args...)
}

func (t *PostgresTx) QueryRow(query string, args ...interface{}) *sql.Row {
	query = ConvertPlaceholders(query)
	return t.tx.QueryRow(query, args...)
}

func (t *PostgresTx) Exec(query string, args ...interface{}) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return t.tx.Exec(query, args...)
}

func (t *PostgresTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	query = ConvertPlaceholders(query)
	return t.tx.QueryContext(ctx, query, args...)
}

func (t *PostgresTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	query = ConvertPlaceholders(query)
	return t.tx.QueryRowContext(ctx, query, args...)
}

func (t *PostgresTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	query = ConvertPlaceholders(query)
	return t.tx.ExecContext(ctx, query, args...)
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
	return t.tx.Commit()
}

func (t *PostgresTx) Rollback() error {
	return t.tx.Rollback()
}

// Helper function to check if a query uses ? placeholders
func usesQuestionMarkPlaceholders(query string) bool {
	return strings.Contains(query, "?")
}
