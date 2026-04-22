package repository

import (
	"database/sql"

	"windshift/internal/database"
)

// BaseRepository provides shared data access utilities for all repositories.
type BaseRepository struct {
	db database.Database
}

// NewBaseRepository creates a new base repository.
func NewBaseRepository(db database.Database) *BaseRepository {
	return &BaseRepository{db: db}
}

// DB returns the underlying database connection.
func (b *BaseRepository) DB() database.Database {
	return b.db
}

// ApplyActionNullFieldsToPtr sets nullable fields on an action struct given field pointers.
// This avoids duplicating the same null-handling logic in multiple repositories.
func ApplyActionNullFieldsToPtr(
	description, triggerConfig *string,
	createdBy **int,
	descVal, triggerVal sql.NullString,
	createdVal sql.NullInt64,
) {
	if descVal.Valid {
		*description = descVal.String
	}
	if triggerVal.Valid {
		*triggerConfig = triggerVal.String
	}
	if createdVal.Valid {
		val := int(createdVal.Int64)
		*createdBy = &val
	}
}

// QueryExec executes the built update and returns rows affected.
// Caller provides the full query and appends WHERE args.
func (b *BaseRepository) QueryExec(query string, args ...interface{}) (sql.Result, error) {
	return b.db.ExecWrite(query, args...)
}

// QueryRow executes a query that returns a single row.
func (b *BaseRepository) QueryRow(query string, args ...interface{}) *sql.Row {
	return b.db.QueryRow(query, args...)
}

// Query executes a query that returns rows.
func (b *BaseRepository) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return b.db.Query(query, args...)
}

// Count executes a COUNT(*) query and returns the total.
func (b *BaseRepository) Count(query string, args ...interface{}) (int, error) {
	var total int
	err := b.db.QueryRow(query, args...).Scan(&total)
	return total, err
}
