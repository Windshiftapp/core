package database

import (
	"context"
	"database/sql"
	"fmt"
)

// Database is the main interface that all database implementations must satisfy
// last review: ser, 210426
type Database interface {
	// Query executes a query that returns rows (SELECT)
	Query(query string, args ...interface{}) (*sql.Rows, error)

	// QueryRow executes a query that returns at most one row
	QueryRow(query string, args ...interface{}) *sql.Row

	// Exec executes a query that doesn't return rows (INSERT, UPDATE, DELETE)
	// For SQLite: routes to write connection for safety
	// For PostgreSQL: uses standard connection pool
	Exec(query string, args ...interface{}) (sql.Result, error)

	// QueryContext executes a query with context that returns rows
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)

	// QueryRowContext executes a query with context that returns at most one row
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row

	// ExecContext executes a query with context that doesn't return rows
	// For SQLite: routes to write connection for safety
	// For PostgreSQL: uses standard connection pool
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)

	// ExecWrite explicitly executes a write query using the write connection
	// For SQLite: uses dedicated write connection (serialized)
	// For PostgreSQL: uses standard connection pool (MVCC handles concurrency)
	ExecWrite(query string, args ...interface{}) (sql.Result, error)

	// ExecWriteContext explicitly executes a write query with context using the write connection
	// For SQLite: uses dedicated write connection (serialized)
	// For PostgreSQL: uses standard connection pool (MVCC handles concurrency)
	ExecWriteContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)

	// Begin starts a new transaction (returns wrapped transaction)
	Begin() (Tx, error)

	// BeginTx starts a new transaction with options (returns wrapped transaction)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error)

	// Close closes the database connection
	Close() error

	// Initialize sets up the database schema
	Initialize() error

	// GetDB returns the underlying *sql.DB for legacy compatibility
	GetDB() *sql.DB

	// GetDriverName returns the database driver name ("sqlite3" or "postgres")
	GetDriverName() string

	// MigrateSelectFieldOptions migrates legacy string-array options to ID-based format
	MigrateSelectFieldOptions() error
}

// Tx is a database transaction interface that supports placeholder conversion
type Tx interface {
	// Query executes a query that returns rows within the transaction
	Query(query string, args ...interface{}) (*sql.Rows, error)

	// QueryRow executes a query that returns at most one row within the transaction
	QueryRow(query string, args ...interface{}) *sql.Row

	// Exec executes a query that doesn't return rows within the transaction
	Exec(query string, args ...interface{}) (sql.Result, error)

	// QueryContext executes a query with context that returns rows
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)

	// QueryRowContext executes a query with context that returns at most one row
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row

	// ExecContext executes a query with context that doesn't return rows
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)

	// Prepare prepares a statement within the transaction
	Prepare(query string) (*sql.Stmt, error)

	// PrepareContext prepares a statement with context within the transaction
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)

	// Commit commits the transaction
	Commit() error

	// Rollback rolls back the transaction
	Rollback() error
}

// WithTx runs fn inside a transaction. It begins a transaction, executes fn,
// and commits on success. If fn returns an error or commit fails, the
// transaction is rolled back. This eliminates the repeated
// begin/defer-rollback/commit boilerplate across service methods.
func WithTx(db Database, fn func(tx Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// WithTxResult runs fn inside a transaction and returns a value along with
// any error. Like WithTx but for operations that produce a result.
func WithTxResult[T any](db Database, fn func(tx Tx) (T, error)) (T, error) {
	tx, err := db.Begin()
	if err != nil {
		var zero T
		return zero, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := fn(tx)
	if err != nil {
		var zero T
		return zero, err
	}

	if err := tx.Commit(); err != nil {
		var zero T
		return zero, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return result, nil
}
