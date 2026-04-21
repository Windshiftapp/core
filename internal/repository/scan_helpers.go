package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// inPlaceholders returns a "?,?,?" placeholder string and an []interface{}
// args slice for the given values. Intended for building IN (...) clauses.
// Returns ("", nil) when values is empty — callers must guard against that
// before splicing the result into a query.
func inPlaceholders[T any](values []T) (clause string, args []interface{}) {
	if len(values) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(values))
	args = make([]interface{}, len(values))
	for i, v := range values {
		placeholders[i] = "?"
		args[i] = v
	}
	return strings.Join(placeholders, ","), args
}

// assignNullableInt copies src.Int64 into *dest as *int when src.Valid.
func assignNullableInt(dest **int, src sql.NullInt64) {
	if src.Valid {
		val := int(src.Int64)
		*dest = &val
	}
}

// assignNullableString copies src.String into *dest when src.Valid.
func assignNullableString(dest *string, src sql.NullString) {
	if src.Valid {
		*dest = src.String
	}
}

// assignNullableTime copies src.Time into *dest when src.Valid.
func assignNullableTime(dest **time.Time, src sql.NullTime) {
	if src.Valid {
		t := src.Time
		*dest = &t
	}
}

// assignNullableFloat64 copies src.Float64 into *dest when src.Valid.
func assignNullableFloat64(dest **float64, src sql.NullFloat64) {
	if src.Valid {
		v := src.Float64
		*dest = &v
	}
}

// parseCustomFieldsJSON parses a custom_field_values JSON blob into a map.
// Returns an empty, non-nil map for NULL, empty string, or malformed JSON —
// matching the permissive behavior used throughout the item scan paths.
func parseCustomFieldsJSON(raw sql.NullString) map[string]interface{} {
	out := make(map[string]interface{})
	if !raw.Valid || raw.String == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw.String), &out); err != nil {
		return make(map[string]interface{})
	}
	return out
}

// marshalCustomFields serializes a custom_field_values map to a sql.NullString.
// Returns {Valid:false} for an empty map so that the column stores NULL
// instead of "{}".
func marshalCustomFields(customFields map[string]interface{}) (sql.NullString, error) {
	if len(customFields) == 0 {
		return sql.NullString{Valid: false}, nil
	}
	data, err := json.Marshal(customFields)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("failed to marshal custom fields: %w", err)
	}
	return sql.NullString{String: string(data), Valid: true}, nil
}
