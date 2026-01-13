package utils

import (
	"database/sql"
	"time"
)

// NullInt64ToPtr converts sql.NullInt64 to *int.
// Returns nil if the value is not valid, otherwise returns a pointer to the int value.
func NullInt64ToPtr(n sql.NullInt64) *int {
	if n.Valid {
		v := int(n.Int64)
		return &v
	}
	return nil
}

// NullStringToPtr converts sql.NullString to *string.
// Returns nil if the value is not valid, otherwise returns a pointer to the string value.
func NullStringToPtr(n sql.NullString) *string {
	if n.Valid {
		return &n.String
	}
	return nil
}

// NullInt64ToInt converts sql.NullInt64 to int with a default value.
// Returns the int value if valid, otherwise returns the provided default.
func NullInt64ToInt(n sql.NullInt64, defaultVal int) int {
	if n.Valid {
		return int(n.Int64)
	}
	return defaultVal
}

// IntPtrToNullInt64 converts *int to sql.NullInt64.
// Returns a valid NullInt64 if the pointer is not nil, otherwise returns an invalid NullInt64.
func IntPtrToNullInt64(p *int) sql.NullInt64 {
	if p != nil {
		return sql.NullInt64{Int64: int64(*p), Valid: true}
	}
	return sql.NullInt64{}
}

// NullTimeToPtr converts sql.NullTime to *time.Time.
// Returns nil if the value is not valid, otherwise returns a pointer to the time value.
func NullTimeToPtr(n sql.NullTime) *time.Time {
	if n.Valid {
		return &n.Time
	}
	return nil
}
