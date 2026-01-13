package database

import (
	"context"
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteDB wraps the existing DB struct to implement the Database interface
type SQLiteDB struct {
	*DB
}

// NewSQLiteDB creates a new SQLite database connection
func NewSQLiteDB(dataSourceName string) (Database, error) {
	return NewSQLiteDBWithPoolSizes(dataSourceName, 120, 1)
}

// NewSQLiteDBWithPoolSizes creates a new SQLite database connection with custom pool sizes
func NewSQLiteDBWithPoolSizes(dataSourceName string, readConns, writeConns int) (Database, error) {
	db, err := NewDB(dataSourceName)
	if err != nil {
		return nil, err
	}
	return &SQLiteDB{DB: db}, nil
}

// GetDB returns the underlying *sql.DB for backward compatibility
func (s *SQLiteDB) GetDB() *sql.DB {
	return s.DB.DB
}

// GetDriverName returns the database driver name
func (s *SQLiteDB) GetDriverName() string {
	return "sqlite3"
}

// Query executes a query that returns rows
func (s *SQLiteDB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return s.DB.Query(query, args...)
}

// QueryRow executes a query that returns at most one row
func (s *SQLiteDB) QueryRow(query string, args ...interface{}) *sql.Row {
	return s.DB.QueryRow(query, args...)
}

// Exec executes a query that doesn't return rows
// Always uses write connection for safety (all Exec operations are writes)
func (s *SQLiteDB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return s.DB.writeConn.Exec(query, args...)
}

// QueryContext executes a query with context that returns rows
func (s *SQLiteDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return s.DB.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query with context that returns at most one row
func (s *SQLiteDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return s.DB.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a query with context that doesn't return rows
// Always uses write connection for safety (all Exec operations are writes)
func (s *SQLiteDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.DB.writeConn.ExecContext(ctx, query, args...)
}

// ExecWrite explicitly executes a write query using the dedicated write connection
func (s *SQLiteDB) ExecWrite(query string, args ...interface{}) (sql.Result, error) {
	return s.DB.writeConn.Exec(query, args...)
}

// ExecWriteContext explicitly executes a write query with context using the dedicated write connection
func (s *SQLiteDB) ExecWriteContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.DB.writeConn.ExecContext(ctx, query, args...)
}

// Begin starts a new transaction (returns wrapped transaction)
func (s *SQLiteDB) Begin() (Tx, error) {
	tx, err := s.DB.writeConn.Begin()
	if err != nil {
		return nil, err
	}
	return NewSQLiteTx(tx), nil
}

// BeginTx starts a new transaction with options (returns wrapped transaction)
func (s *SQLiteDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	tx, err := s.DB.writeConn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return NewSQLiteTx(tx), nil
}

// Close closes the database connection
func (s *SQLiteDB) Close() error {
	return s.DB.Close()
}

// Initialize sets up the database schema
func (s *SQLiteDB) Initialize() error {
	return s.DB.Initialize()
}

// CreateWorkspaceItemSequence is a no-op for SQLite (no sequences)
func (s *SQLiteDB) CreateWorkspaceItemSequence(workspaceID int64) error {
	return nil
}

// DropWorkspaceItemSequence is a no-op for SQLite (no sequences)
func (s *SQLiteDB) DropWorkspaceItemSequence(workspaceID int64) error {
	return nil
}

// NextWorkspaceItemNumber gets the next item number using MAX+1 for SQLite
// SQLite's write connection serialization ensures no race conditions
func (s *SQLiteDB) NextWorkspaceItemNumber(workspaceID int64) (int, error) {
	var nextNum int
	err := s.DB.writeConn.QueryRow(`
		SELECT COALESCE(MAX(workspace_item_number), 0) + 1
		FROM items WHERE workspace_id = ?
	`, workspaceID).Scan(&nextNum)
	return nextNum, err
}
