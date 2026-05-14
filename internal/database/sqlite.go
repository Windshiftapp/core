package database

import (
	"context"
	"database/sql"
	"strings"

	_ "modernc.org/sqlite"
)

// writeQueryPrefixes are statement starts that always need the dedicated
// write connection. WITH is included as an accepted false-positive: a
// read-only CTE will get routed to the writer (cheap, harmless), but a
// write-returning CTE that wasn't routed (the old behavior) would lose
// single-writer serialization (expensive, broken).
var writeQueryPrefixes = []string{
	"INSERT",
	"UPDATE",
	"DELETE",
	"REPLACE",
	"MERGE",
	"WITH",
	"CREATE",
	"ALTER",
	"DROP",
	"VACUUM",
	"TRUNCATE",
}

// isWriteQuery returns true if the query is a write operation. SQLite's
// single-writer model means we must route writes through the dedicated
// write connection; missing a write here silently loses serialization.
func isWriteQuery(query string) bool {
	trimmed := strings.ToUpper(strings.TrimSpace(query))
	for _, p := range writeQueryPrefixes {
		if strings.HasPrefix(trimmed, p) {
			return true
		}
	}
	return false
}

// SQLiteDB wraps the existing DB struct to implement the Database interface
type SQLiteDB struct {
	*DB
}

// NewSQLiteDB creates a new SQLite database connection.
// deadcode-keep: called by core-tests overlay (multiple test fixtures).
func NewSQLiteDB(dataSourceName string) (Database, error) {
	return NewSQLiteDBWithPoolSizes(dataSourceName, 120, 1)
}

// NewSQLiteDBWithPoolSizes creates a new SQLite database connection with custom pool sizes
func NewSQLiteDBWithPoolSizes(dataSourceName string, readConns, writeConns int) (Database, error) {
	db, err := NewDB(dataSourceName, readConns, writeConns)
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
	return "sqlite"
}

// Query executes a query that returns rows
func (s *SQLiteDB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return s.DB.Query(query, args...)
}

// QueryRow executes a query that returns at most one row.
// Write queries (INSERT/UPDATE/DELETE) are routed through the dedicated write
// connection so that INSERT ... RETURNING does not race with other writers.
// last review: ser, 210426, OPTIMIZE: Check whether the write fallback is still needed
func (s *SQLiteDB) QueryRow(query string, args ...interface{}) *sql.Row {
	if isWriteQuery(query) {
		return s.writeConn.QueryRow(query, args...)
	}
	return s.DB.QueryRow(query, args...)
}

// Exec executes a query that doesn't return rows
// Always uses write connection for safety (all Exec operations are writes)
func (s *SQLiteDB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return s.writeConn.Exec(query, args...)
}

// QueryContext executes a query with context that returns rows.
// Write-returning queries (e.g. WITH ... INSERT/UPDATE/DELETE ... RETURNING)
// must route through the write connection so they don't lose single-writer
// serialization. Mirrors QueryRowContext, which already applies isWriteQuery.
func (s *SQLiteDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	if isWriteQuery(query) {
		return s.writeConn.QueryContext(ctx, query, args...)
	}
	return s.DB.QueryContext(ctx, query, args...)
}

// QueryRowContext executes a query with context that returns at most one row.
// Write queries (INSERT/UPDATE/DELETE) are routed through the dedicated write
// connection so that INSERT ... RETURNING does not race with other writers.
func (s *SQLiteDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	if isWriteQuery(query) {
		return s.writeConn.QueryRowContext(ctx, query, args...)
	}
	return s.DB.QueryRowContext(ctx, query, args...)
}

// ExecContext executes a query with context that doesn't return rows
// Always uses write connection for safety (all Exec operations are writes)
func (s *SQLiteDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.writeConn.ExecContext(ctx, query, args...)
}

// ExecWrite explicitly executes a write query using the dedicated write connection
func (s *SQLiteDB) ExecWrite(query string, args ...interface{}) (sql.Result, error) {
	return s.writeConn.Exec(query, args...)
}

// ExecWriteContext explicitly executes a write query with context using the dedicated write connection
func (s *SQLiteDB) ExecWriteContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.writeConn.ExecContext(ctx, query, args...)
}

// Begin starts a new transaction (returns wrapped transaction)
func (s *SQLiteDB) Begin() (Tx, error) {
	tx, err := s.writeConn.Begin()
	if err != nil {
		return nil, err
	}
	return NewSQLiteTx(tx), nil
}

// BeginTx starts a new transaction with options (returns wrapped transaction)
func (s *SQLiteDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (Tx, error) {
	tx, err := s.writeConn.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return NewSQLiteTx(tx), nil
}

// Close closes the database connection
func (s *SQLiteDB) Close() error {
	return s.DB.Close()
}

// Initialize sets up the database schema. Runs the legacy bootstrap in
// DB.Initialize first, then the catalog-based migrations (see
// internal/database/migrations.go). Catalog migrations need the Database
// interface, which only this wrapper implements — the underlying *DB
// exposes the embedded *sql.DB's Begin, not the wrapped Tx.
func (s *SQLiteDB) Initialize() error {
	if err := s.DB.Initialize(); err != nil {
		return err
	}
	return runPendingMigrations(s, Catalog)
}
