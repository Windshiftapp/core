package validation

import (
	"fmt"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ValidateAndNormalizeCustomFieldValues vets a cfv map against the
// current custom_fields schema:
//
//   - For select fields: value must be a numeric id that exists in the
//     field's option set. String-encoded numbers are accepted (e.g. "3").
//   - For multiselect fields: value must be an array; each element must
//     be a known option id. Duplicate ids are removed (order-preserving).
//   - For non-select fields: values are left untouched.
//   - cfv keys that do not match an existing custom field are left
//     untouched — they accumulate harmlessly until the async cleanup job
//     (see scheduler.CFVCleanupScheduler) runs.
//
// The function mutates the cfv map in place and returns a ValidationError
// on the first invalid value it finds (so the caller's 400 response
// surfaces a clear, single problem).
//
// This is the single source of truth for cfv shape on writes — both the
// items create handler and the update validator route through it.
func ValidateAndNormalizeCustomFieldValues(db database.Database, cfv map[string]interface{}) error {
	if len(cfv) == 0 {
		return nil
	}

	// Bulk-load every custom field referenced by the cfv map. Single query
	// keeps the validation O(1) round-trip regardless of how many fields
	// the cfv touches.
	fields, err := loadFieldsForCFV(db, cfv)
	if err != nil {
		return fmt.Errorf("load custom fields for validation: %w", err)
	}

	for fieldKey, raw := range cfv {
		def, ok := fields[fieldKey]
		if !ok {
			// Unknown field id — left in cfv. The async cleanup
			// scheduler is responsible for removing these so this
			// path stays cheap on the hot write path.
			continue
		}
		switch def.FieldType {
		case "select":
			if err := validateSelectValue(fieldKey, def, raw); err != nil {
				return err
			}
		case "multiselect":
			normalized, err := validateAndDedupeMultiselect(fieldKey, def, raw)
			if err != nil {
				return err
			}
			cfv[fieldKey] = normalized
		}
	}
	return nil
}

func loadFieldsForCFV(db database.Database, cfv map[string]interface{}) (map[string]*models.CustomFieldDefinition, error) {
	if len(cfv) == 0 {
		return nil, nil
	}

	// Collect numeric ids from cfv keys; non-numeric keys are skipped.
	ids := make([]any, 0, len(cfv))
	for key := range cfv {
		if _, err := strconv.Atoi(key); err == nil {
			ids = append(ids, key)
		}
	}
	if len(ids) == 0 {
		return map[string]*models.CustomFieldDefinition{}, nil
	}

	placeholders := ""
	for i := range ids {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
	}

	query := "SELECT id, field_type, options FROM custom_field_definitions WHERE id IN (" + placeholders + ")"
	rows, err := db.Query(query, ids...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]*models.CustomFieldDefinition, len(ids))
	for rows.Next() {
		var def models.CustomFieldDefinition
		if err := rows.Scan(&def.ID, &def.FieldType, &def.Options); err != nil {
			return nil, err
		}
		out[strconv.Itoa(def.ID)] = &def
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// validateSelectValue accepts numeric or numeric-string values that match
// a known option id on the field. Anything else is a ValidationError.
func validateSelectValue(fieldKey string, def *models.CustomFieldDefinition, raw interface{}) error {
	if raw == nil {
		return nil
	}
	id, ok := coerceOptionID(raw)
	if !ok {
		return &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: fmt.Sprintf("select option id must be a number, got %T", raw),
		}
	}
	allowed, err := optionIDSet(def)
	if err != nil {
		return err
	}
	if !allowed[id] {
		return &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: fmt.Sprintf("option id %d is not in the field's option set", id),
		}
	}
	return nil
}

// validateAndDedupeMultiselect insists raw is an array (or a single value
// the JSON decoder turned into a non-array). It checks every element
// against the option set and returns a new []int containing each accepted
// id at most once, preserving first-seen order.
func validateAndDedupeMultiselect(fieldKey string, def *models.CustomFieldDefinition, raw interface{}) ([]int, error) {
	if raw == nil {
		return nil, nil
	}
	values, err := coerceToSlice(raw)
	if err != nil {
		return nil, &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: fmt.Sprintf("multiselect value must be an array, got %T", raw),
		}
	}
	allowed, err := optionIDSet(def)
	if err != nil {
		return nil, err
	}

	seen := make(map[int]bool, len(values))
	out := make([]int, 0, len(values))
	for _, v := range values {
		id, ok := coerceOptionID(v)
		if !ok {
			return nil, &ValidationError{
				Field:   "custom_field_values." + fieldKey,
				Message: fmt.Sprintf("multiselect option id must be a number, got %T", v),
			}
		}
		if !allowed[id] {
			return nil, &ValidationError{
				Field:   "custom_field_values." + fieldKey,
				Message: fmt.Sprintf("option id %d is not in the field's option set", id),
			}
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// optionIDSet parses the field's options JSON once and returns the set of
// known option ids for fast membership checks.
func optionIDSet(def *models.CustomFieldDefinition) (map[int]bool, error) {
	opts, err := models.ParseSelectOptions(def.Options)
	if err != nil {
		return nil, fmt.Errorf("parse options for field %d: %w", def.ID, err)
	}
	set := make(map[int]bool, len(opts.Items))
	for _, item := range opts.Items {
		set[item.ID] = true
	}
	return set, nil
}

// coerceOptionID accepts the option-id forms JSON can produce: float64
// (from JSON numbers), int (from Go-side construction), and numeric
// strings (legacy clients). Returns false for any other type so the
// caller can reject the request with a clear message.
func coerceOptionID(v interface{}) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	case string:
		n, err := strconv.Atoi(x)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// coerceToSlice tolerates the two JSON-decoded array shapes the caller
// might pass: []interface{} (from json.Unmarshal of a JSON array) and
// []int (rare, from Go-side construction in tests). Anything else is an
// error.
func coerceToSlice(v interface{}) ([]interface{}, error) {
	switch x := v.(type) {
	case []interface{}:
		return x, nil
	case []int:
		out := make([]interface{}, len(x))
		for i, n := range x {
			out[i] = n
		}
		return out, nil
	}
	return nil, fmt.Errorf("not an array")
}
