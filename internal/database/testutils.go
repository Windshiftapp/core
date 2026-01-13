//go:build test

package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestDB wraps a database connection for testing
type TestDB struct {
	*DB
	TempFile string
	IsMemory bool
}

// CreateTestDB creates a new test database instance
func CreateTestDB(t *testing.T, inMemory bool) *TestDB {
	var dsn string
	var tempFile string

	if inMemory {
		dsn = ":memory:"
	} else {
		// Create temporary database file
		tempDir := t.TempDir()
		tempFile = filepath.Join(tempDir, "test.db")
		dsn = tempFile
	}

	db, err := NewDB(dsn)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Initialize the database schema
	if err := db.Initialize(); err != nil {
		t.Fatalf("Failed to initialize test database: %v", err)
	}

	return &TestDB{
		DB:       db,
		TempFile: tempFile,
		IsMemory: inMemory,
	}
}

// CreateFreshDB creates a database without initialization (for testing initialization itself)
func CreateFreshDB(t *testing.T, inMemory bool) *TestDB {
	var dsn string
	var tempFile string

	if inMemory {
		dsn = ":memory:"
	} else {
		tempDir := t.TempDir()
		tempFile = filepath.Join(tempDir, "fresh.db")
		dsn = tempFile
	}

	db, err := NewDB(dsn)
	if err != nil {
		t.Fatalf("Failed to create fresh test database: %v", err)
	}

	return &TestDB{
		DB:       db,
		TempFile: tempFile,
		IsMemory: inMemory,
	}
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

// SeedTestData populates the database with basic test data
func (tdb *TestDB) SeedTestData(t *testing.T) TestDataSet {
	data := TestDataSet{}

	// Create test workspace (use high ID to avoid conflicts with default data)
	result, err := tdb.Exec(`
		INSERT INTO workspaces (name, key, description, active) 
		VALUES ('Test Workspace', 'TEST', 'Test workspace for unit tests', 1)
	`)
	if err != nil {
		t.Fatalf("Failed to seed workspace: %v", err)
	}
	workspaceID, _ := result.LastInsertId()
	data.WorkspaceID = int(workspaceID)

	// Create test user
	result, err = tdb.Exec(`
		INSERT INTO users (email, username, first_name, last_name, role, password_hash, is_active) 
		VALUES ('test@example.com', 'testuser', 'Test', 'User', 'admin', '$2a$10$hash', 1)
	`)
	if err != nil {
		t.Fatalf("Failed to seed user: %v", err)
	}
	userID, _ := result.LastInsertId()
	data.UserID = int(userID)

	// Create test status category
	result, err = tdb.Exec(`
		INSERT INTO status_categories (name, color, description, is_default, is_completed) 
		VALUES ('Test Category', '#3b82f6', 'Test status category', 0, 0)
	`)
	if err != nil {
		t.Fatalf("Failed to seed status category: %v", err)
	}
	statusCategoryID, _ := result.LastInsertId()
	data.StatusCategoryID = int(statusCategoryID)

	// Create test status
	result, err = tdb.Exec(`
		INSERT INTO statuses (name, description, category_id, is_default) 
		VALUES ('Test Status', 'Test status', ?, 0)
	`, statusCategoryID)
	if err != nil {
		t.Fatalf("Failed to seed status: %v", err)
	}
	statusID, _ := result.LastInsertId()
	data.StatusID = int(statusID)

	return data
}

// TestDataSet contains IDs of seeded test data
type TestDataSet struct {
	WorkspaceID      int
	UserID           int
	StatusCategoryID int
	StatusID         int
}

// ExecuteInTransaction executes a function within a database transaction for testing
func ExecuteInTransaction(t *testing.T, tdb *TestDB, fn func(tx *sql.Tx) error) {
	tx, err := tdb.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	err = fn(tx)
	if err != nil {
		tx.Rollback()
		return
	}

	tx.Commit()
}
