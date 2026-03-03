//go:build test

package testutils

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"windshift/internal/database"
)

// SharedMemoryDSN is the recommended DSN for in-memory test databases.
// Uses shared cache so multiple connections see the same data.
// Required because DB struct uses separate read pool and write connection.
const SharedMemoryDSN = "file::memory:?cache=shared&mode=memory"

// TestDB wraps a database connection for testing
type TestDB struct {
	*database.DB
	TempFile    string
	IsMemory    bool
	dbInterface database.Database // Cached Database interface for service layer tests
}

// CreateTestDB creates a new test database instance.
// If inMemory is true, uses an in-memory SQLite database with shared cache.
// Otherwise creates a temporary file database.
func CreateTestDB(t *testing.T, inMemory bool) *TestDB {
	var dsn string
	var tempFile string

	if inMemory {
		dsn = SharedMemoryDSN
	} else {
		// Create temporary database file
		tempDir := t.TempDir()
		tempFile = filepath.Join(tempDir, "test.db")
		dsn = tempFile
	}

	db, err := database.NewDB(dsn)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Initialize the database schema
	if err := db.Initialize(); err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	// Wrap in SQLiteDB to get Database interface
	sqliteDB := &database.SQLiteDB{DB: db}

	return &TestDB{
		DB:          db,
		TempFile:    tempFile,
		IsMemory:    inMemory,
		dbInterface: sqliteDB,
	}
}

// CreateFreshDB creates a database without initialization (for testing initialization itself)
func CreateFreshDB(t *testing.T, inMemory bool) *TestDB {
	var dsn string
	var tempFile string

	if inMemory {
		dsn = SharedMemoryDSN
	} else {
		tempDir := t.TempDir()
		tempFile = filepath.Join(tempDir, "fresh.db")
		dsn = tempFile
	}

	db, err := database.NewDB(dsn)
	if err != nil {
		t.Fatalf("Failed to create fresh test database: %v", err)
	}

	// Wrap in SQLiteDB to get Database interface
	sqliteDB := &database.SQLiteDB{DB: db}

	return &TestDB{
		DB:          db,
		TempFile:    tempFile,
		IsMemory:    inMemory,
		dbInterface: sqliteDB,
	}
}

// GetDatabase returns the Database interface for use with service layer
func (tdb *TestDB) GetDatabase() database.Database {
	return tdb.dbInterface
}

// Close closes the database connection and cleans up temp files
func (tdb *TestDB) Close() error {
	if err := tdb.DB.Close(); err != nil {
		return err
	}

	if !tdb.IsMemory && tdb.TempFile != "" {
		// Clean up temp file if it exists
		if _, err := os.Stat(tdb.TempFile); err == nil {
			os.Remove(tdb.TempFile)
		}
	}

	return nil
}

// AssertTableExists verifies that a table exists in the database
func (tdb *TestDB) AssertTableExists(t *testing.T, tableName string) {
	var exists bool
	query := `SELECT EXISTS(SELECT name FROM sqlite_master WHERE type='table' AND name=?)`
	err := tdb.QueryRow(query, tableName).Scan(&exists)
	if err != nil {
		t.Fatalf("Failed to check if table %s exists: %v", tableName, err)
	}

	if !exists {
		t.Fatalf("Table %s does not exist", tableName)
	}
}

// AssertTableNotExists verifies that a table does not exist in the database
func (tdb *TestDB) AssertTableNotExists(t *testing.T, tableName string) {
	var exists bool
	query := `SELECT EXISTS(SELECT name FROM sqlite_master WHERE type='table' AND name=?)`
	err := tdb.QueryRow(query, tableName).Scan(&exists)
	if err != nil {
		t.Fatalf("Failed to check if table %s exists: %v", tableName, err)
	}

	if exists {
		t.Fatalf("Table %s should not exist but does", tableName)
	}
}

// AssertColumnExists verifies that a column exists in a table
func (tdb *TestDB) AssertColumnExists(t *testing.T, tableName, columnName string) {
	query := `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`
	var count int
	err := tdb.QueryRow(query, tableName, columnName).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check column %s.%s: %v", tableName, columnName, err)
	}

	if count == 0 {
		t.Fatalf("Column %s.%s does not exist", tableName, columnName)
	}
}

// AssertIndexExists verifies that an index exists
func (tdb *TestDB) AssertIndexExists(t *testing.T, indexName string) {
	var exists bool
	query := `SELECT EXISTS(SELECT name FROM sqlite_master WHERE type='index' AND name=?)`
	err := tdb.QueryRow(query, indexName).Scan(&exists)
	if err != nil {
		t.Fatalf("Failed to check if index %s exists: %v", indexName, err)
	}

	if !exists {
		t.Fatalf("Index %s does not exist", indexName)
	}
}

// AssertForeignKeyEnabled verifies that foreign key constraints are enabled
func (tdb *TestDB) AssertForeignKeyEnabled(t *testing.T) {
	var enabled bool
	err := tdb.QueryRow("PRAGMA foreign_keys").Scan(&enabled)
	if err != nil {
		t.Fatalf("Failed to check foreign key status: %v", err)
	}

	if !enabled {
		t.Fatal("Foreign key constraints are not enabled")
	}
}

// GetTableCount returns the number of tables in the database
func (tdb *TestDB) GetTableCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`
	err := tdb.QueryRow(query).Scan(&count)
	return count, err
}

// TestDataSet contains IDs of seeded test data
type TestDataSet struct {
	WorkspaceID      int
	UserID           int
	StatusCategoryID int
	StatusID         int
	PriorityID       int
}

// SeedTestData populates the database with basic test data
func (tdb *TestDB) SeedTestData(t *testing.T) TestDataSet {
	data := TestDataSet{}

	// Create test workspace
	_, err := tdb.Exec(`
		INSERT INTO workspaces (id, name, key, description, active)
		VALUES (1, 'Test Workspace', 'TEST', 'Test workspace for unit tests', 1)
	`)
	if err != nil {
		t.Fatalf("Failed to seed workspace: %v", err)
	}
	data.WorkspaceID = 1

	// Create test user
	_, err = tdb.Exec(`
		INSERT INTO users (id, email, username, first_name, last_name, password_hash, is_active)
		VALUES (1, 'test@example.com', 'testuser', 'Test', 'User', '$2a$10$hash', 1)
	`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}
	data.UserID = 1

	// Use existing default status category (created during database initialization)
	var categoryID int
	err = tdb.QueryRow("SELECT id FROM status_categories WHERE is_default = true LIMIT 1").Scan(&categoryID)
	if err != nil {
		t.Fatalf("Failed to find default status category: %v", err)
	}
	data.StatusCategoryID = categoryID

	// Use existing default status (created during database initialization)
	var statusID int
	err = tdb.QueryRow("SELECT id FROM statuses WHERE is_default = true LIMIT 1").Scan(&statusID)
	if err != nil {
		t.Fatalf("Failed to find default status: %v", err)
	}
	data.StatusID = statusID

	// Use existing default priority (created during database initialization)
	var priorityID int
	err = tdb.QueryRow("SELECT id FROM priorities WHERE is_default = true LIMIT 1").Scan(&priorityID)
	if err != nil {
		// If no default priority exists, try to get any priority
		err = tdb.QueryRow("SELECT id FROM priorities LIMIT 1").Scan(&priorityID)
		if err != nil {
			t.Fatalf("Failed to find any priority: %v", err)
		}
	}
	data.PriorityID = priorityID

	// Grant test user Administrator role on test workspace
	var adminRoleID int
	err = tdb.QueryRow(`SELECT id FROM workspace_roles WHERE name = 'Administrator'`).Scan(&adminRoleID)
	if err != nil {
		t.Fatalf("Failed to get Administrator role: %v", err)
	}

	_, err = tdb.Exec(`
		INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
	`, data.UserID, data.WorkspaceID, adminRoleID)
	if err != nil {
		t.Fatalf("Failed to assign workspace role: %v", err)
	}

	return data
}

// ClearAllTables removes all data from all tables (for cleanup)
func (tdb *TestDB) ClearAllTables(t *testing.T) {
	// Get all table names
	rows, err := tdb.Query(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'migrations'
	`)
	if err != nil {
		t.Fatalf("Failed to get table names: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			t.Fatalf("Failed to scan table name: %v", err)
		}
		tables = append(tables, tableName)
	}

	// Disable foreign key constraints temporarily
	if _, err := tdb.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		t.Fatalf("Failed to disable foreign keys: %v", err)
	}

	// Clear all tables
	for _, table := range tables {
		if _, err := tdb.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			t.Fatalf("Failed to clear table %s: %v", table, err)
		}
	}

	// Re-enable foreign key constraints
	if _, err := tdb.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("Failed to re-enable foreign keys: %v", err)
	}
}

// ExecuteInTransaction executes a function within a database transaction
func (tdb *TestDB) ExecuteInTransaction(t *testing.T, fn func(*sql.Tx) error) {
	tx, err := tdb.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			t.Fatalf("Failed to rollback transaction: %v (original error: %v)", rollbackErr, err)
		}
		t.Fatalf("Transaction function failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit transaction: %v", err)
	}
}
