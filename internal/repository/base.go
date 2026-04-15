package repository

import (
	"database/sql"
	"fmt"
	"strings"

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

// DynamicUpdateBuilder helps build parameterized UPDATE statements dynamically.
type DynamicUpdateBuilder struct {
	sets []string
	args []interface{}
}

// NewDynamicUpdateBuilder creates a new dynamic update builder.
func NewDynamicUpdateBuilder() *DynamicUpdateBuilder {
	return &DynamicUpdateBuilder{}
}

// AddBool adds a boolean field update if the value is non-nil.
func (b *DynamicUpdateBuilder) AddBool(field string, value *bool) {
	if value != nil {
		b.sets = append(b.sets, field+" = ?")
		b.args = append(b.args, *value)
	}
}

// AddInt adds an integer field update if the value is non-nil.
func (b *DynamicUpdateBuilder) AddInt(field string, value *int) {
	if value != nil {
		b.sets = append(b.sets, field+" = ?")
		b.args = append(b.args, *value)
	}
}

// AddInt64 adds an int64 field update if the value is non-nil.
func (b *DynamicUpdateBuilder) AddInt64(field string, value *int64) {
	if value != nil {
		b.sets = append(b.sets, field+" = ?")
		b.args = append(b.args, *value)
	}
}

// AddString adds a string field update if the value is non-nil and non-empty.
func (b *DynamicUpdateBuilder) AddString(field string, value *string) {
	if value != nil && *value != "" {
		b.sets = append(b.sets, field+" = ?")
		b.args = append(b.args, *value)
	}
}

// AddNullableString adds a string field update if the pointer is non-nil.
func (b *DynamicUpdateBuilder) AddNullableString(field string, value *string) {
	if value != nil {
		b.sets = append(b.sets, field+" = ?")
		b.args = append(b.args, *value)
	}
}

// AddNullableInt adds an integer field update if the pointer is non-nil.
func (b *DynamicUpdateBuilder) AddNullableInt(field string, value *int) {
	if value != nil {
		b.sets = append(b.sets, field+" = ?")
		b.args = append(b.args, *value)
	}
}

// AddField adds a raw field update with a value.
func (b *DynamicUpdateBuilder) AddField(field string, value interface{}) {
	b.sets = append(b.sets, field+" = ?")
	b.args = append(b.args, value)
}

// IsEmpty returns true if no fields have been added.
func (b *DynamicUpdateBuilder) IsEmpty() bool {
	return len(b.sets) == 0
}

// Build constructs the UPDATE query string and returns args.
// The caller must append the WHERE arg(s) to args.
func (b *DynamicUpdateBuilder) Build(table string) (query string, args []interface{}) {
	if len(b.sets) == 0 {
		return "", nil
	}
	query = fmt.Sprintf("UPDATE %s SET %s WHERE ", table, strings.Join(b.sets, ", "))
	return query, b.args
}

// BuildWithTimestamp is like Build but also adds an updated_at = CURRENT_TIMESTAMP clause.
func (b *DynamicUpdateBuilder) BuildWithTimestamp(table string) (query string, args []interface{}) {
	b.AddField("updated_at", "CURRENT_TIMESTAMP")
	return b.Build(table)
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
