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

// NullTimeToPtr converts sql.NullTime to *time.Time.
// Returns nil if the value is not valid, otherwise returns a pointer to the time value.
func NullTimeToPtr(n sql.NullTime) *time.Time {
	if n.Valid {
		return &n.Time
	}
	return nil
}

// InterfaceToIntPtr extracts an int value from an interface{} that could be int, *int, or other numeric types.
// Useful for extracting values from map[string]interface{} where the underlying type may vary.
// Returns nil if the value is nil or cannot be converted to int.
func InterfaceToIntPtr(v interface{}) *int {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case int:
		return &val
	case *int:
		return val
	case int64:
		i := int(val)
		return &i
	case *int64:
		if val == nil {
			return nil
		}
		i := int(*val)
		return &i
	case float64:
		i := int(val)
		return &i
	default:
		return nil
	}
}
