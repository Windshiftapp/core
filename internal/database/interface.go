package database

import (
	"context"
	"database/sql"
)

// Database is the main interface that all database implementations must satisfy
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

	// EnsureDefaultNotificationSettings creates default notification settings if they don't exist
	EnsureDefaultNotificationSettings() error

	// Sequence management for workspace item numbers
	// PostgreSQL: creates/drops actual sequences, uses nextval() for atomic number generation
	// SQLite: no-op for create/drop, uses MAX+1 for number generation

	// CreateWorkspaceItemSequence creates a sequence for workspace item numbers (PostgreSQL only)
	CreateWorkspaceItemSequence(workspaceID int64) error

	// DropWorkspaceItemSequence drops the sequence when workspace is deleted (PostgreSQL only)
	DropWorkspaceItemSequence(workspaceID int64) error

	// NextWorkspaceItemNumber gets the next item number for a workspace
	// PostgreSQL: uses nextval() on workspace sequence (atomic, no locking)
	// SQLite: uses MAX(workspace_item_number) + 1
	NextWorkspaceItemNumber(workspaceID int64) (int, error)
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
