// Package testdb opens optional PostgreSQL databases for integration tests.
// Tests skip when no DSN is configured or the server is unreachable.
package testdb

import (
	"os"
	"strings"
	"testing"

	"windshift/internal/database"
)

// PostgresDSNFromEnv returns a PostgreSQL connection string when
// POSTGRES_CONNECTION_STRING or POSTGRES_* variables are set.
func PostgresDSNFromEnv() (string, bool) {
	if conn := strings.TrimSpace(os.Getenv("POSTGRES_CONNECTION_STRING")); conn != "" {
		return conn, true
	}
	host := strings.TrimSpace(os.Getenv("POSTGRES_HOST"))
	if host == "" {
		return "", false
	}
	env := database.PostgresEnv{
		Host:     host,
		Port:     firstNonEmpty(os.Getenv("POSTGRES_PORT"), "5432"),
		User:     firstNonEmpty(os.Getenv("POSTGRES_USER"), "windshift"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
		Database: firstNonEmpty(os.Getenv("POSTGRES_DB"), "windshift"),
		SSLMode:  firstNonEmpty(os.Getenv("POSTGRES_SSLMODE"), database.DefaultPostgresSSLMode),
	}
	return database.BuildPostgresConnString(env), true
}

// OpenPostgres connects to PostgreSQL, runs Initialize, and skips the test when
// no DSN is configured or the database is unreachable.
func OpenPostgres(t *testing.T) database.Database {
	t.Helper()
	dsn, ok := PostgresDSNFromEnv()
	if !ok {
		t.Skip("postgres integration tests skipped: set POSTGRES_CONNECTION_STRING or POSTGRES_HOST")
	}
	db, err := database.NewPostgresDB(dsn, 2)
	if err != nil {
		t.Skipf("postgres integration tests skipped: %v", err)
	}
	if err := db.Initialize(); err != nil {
		_ = db.Close()
		t.Skipf("postgres integration tests skipped: initialize failed: %v", err)
	}
	return db
}

// OpenSQLite opens an isolated SQLite database for tests.
func OpenSQLite(t *testing.T, name string) database.Database {
	t.Helper()
	db, err := database.NewSQLiteDB(t.TempDir() + "/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Initialize(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db
}

// RunPostgres opens PostgreSQL, runs fn, and closes the database afterward.
func RunPostgres(t *testing.T, fn func(database.Database)) {
	t.Helper()
	db := OpenPostgres(t)
	t.Cleanup(func() { _ = db.Close() })
	fn(db)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
