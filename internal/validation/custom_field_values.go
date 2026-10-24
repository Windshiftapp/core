package validation

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/sanitize"
)

// ValidateAndNormalizeCustomFieldValues mutates CFV values to match field
// definitions. It validates choices, deduplicates multiselects, sanitizes text,
// and returns the first invalid value; unknown keys await async cleanup.
func ValidateAndNormalizeCustomFieldValues(db database.Database, cfv map[string]any) error {
	if len(cfv) == 0 {
		return nil
	}

	// Load referenced fields in one query.
	fields, err := loadFieldsForCFV(db, cfv)
	if err != nil {
		return fmt.Errorf("load custom fields for validation: %w", err)
	}

	for fieldKey, raw := range cfv {
		def, ok := fields[fieldKey]
		if !ok {
			// Unknown keys are removed asynchronously.
			continue
		}
		switch def.FieldType {
		case "select":
			normalized, err := normalizeSelectValue(fieldKey, def, raw)
			if err != nil {
				return err
			}
			cfv[fieldKey] = normalized
		case "multiselect":
			normalized, err := validateAndDedupeMultiselect(fieldKey, def, raw)
			if err != nil {
				return err
			}
			cfv[fieldKey] = normalized
		case models.CustomFieldTypeBoolean, models.CustomFieldTypeCheckbox:
			if _, err := ValidateCheckboxValue(fieldKey, raw); err != nil {
				return &ValidationError{
					Field:   "custom_field_values." + fieldKey,
					Message: fmt.Sprintf("boolean value must be true, false, or empty; got %T", raw),
				}
			}
		case "text", "textarea":
			cfv[fieldKey] = sanitizeTextValue(def.FieldType, raw)
		case "number":
			normalized, err := normalizeNumberValue(fieldKey, raw)
			if err != nil {
				return err
			}
			cfv[fieldKey] = normalized
		case "date":
			normalized, err := normalizeDateValue(fieldKey, raw)
			if err != nil {
				return err
			}
			cfv[fieldKey] = normalized
		case "milestone":
			normalized, err := normalizeMilestoneValue(db, fieldKey, raw, false)
			if err != nil {
				return err
			}
			cfv[fieldKey] = normalized
		case "multi_milestone":
			normalized, err := normalizeMilestoneValue(db, fieldKey, raw, true)
			if err != nil {
				return err
			}
			cfv[fieldKey] = normalized
		}
	}
	return nil
}

// normalizeMilestoneValue validates milestone references and keeps first-seen
// order. Multi-value fields store an array of ids; single-value fields store
// one id.
func normalizeMilestoneValue(db database.Database, fieldKey string, raw any, multi bool) (any, error) {
	if raw == nil {
		if multi {
			return []int{}, nil
		}
		return nil, nil
	}

	if multi {
		if s, ok := raw.(string); ok && strings.TrimSpace(s) == "" {
			return []int{}, nil
		}
		values, err := coerceToSlice(raw)
		if err != nil {
			return nil, &ValidationError{
				Field:   "custom_field_values." + fieldKey,
				Message: fmt.Sprintf("milestone value must be an array, got %T", raw),
			}
		}
		seen := make(map[int]bool, len(values))
		ids := make([]int, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
				continue
			}
			id, ok := CoerceOptionID(value)
			if !ok {
				return nil, &ValidationError{
					Field:   "custom_field_values." + fieldKey,
					Message: fmt.Sprintf("milestone id must be an integer, got %v (%T)", value, value),
				}
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) == 0 {
			return []int{}, nil
		}
		if err := ensureMilestonesExist(db, fieldKey, ids); err != nil {
			return nil, err
		}
		return ids, nil
	}

	if s, ok := raw.(string); ok && strings.TrimSpace(s) == "" {
		return nil, nil
	}
	id, ok := CoerceOptionID(raw)
	if !ok {
		return nil, &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: fmt.Sprintf("milestone id must be an integer, got %v (%T)", raw, raw),
		}
	}
	if err := ensureMilestonesExist(db, fieldKey, []int{id}); err != nil {
		return nil, err
	}
	return id, nil
}

func ensureMilestonesExist(db database.Database, fieldKey string, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := db.Query("SELECT id FROM milestones WHERE id IN ("+placeholders+")", args...)
	if err != nil {
		return fmt.Errorf("load milestones for validation: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found := make(map[int]bool, len(ids))
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan milestone id: %w", err)
		}
		found[id] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load milestones for validation: %w", err)
	}
	for _, id := range ids {
		if !found[id] {
			return &ValidationError{
				Field:   "custom_field_values." + fieldKey,
				Message: fmt.Sprintf("milestone %d not found", id),
			}
		}
	}
	return nil
}

func normalizeNumberValue(fieldKey string, raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}

	var value float64
	var err error
	switch typed := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil, nil
		}
		value, err = strconv.ParseFloat(trimmed, 64)
	case json.Number:
		value, err = typed.Float64()
	case float64:
		value = typed
	case float32:
		value = float64(typed)
	case int:
		value = float64(typed)
	case int8:
		value = float64(typed)
	case int16:
		value = float64(typed)
	case int32:
		value = float64(typed)
	case int64:
		value = float64(typed)
	case uint:
		value = float64(typed)
	case uint8:
		value = float64(typed)
	case uint16:
		value = float64(typed)
	case uint32:
		value = float64(typed)
	case uint64:
		value = float64(typed)
	default:
		return nil, &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: fmt.Sprintf("number value must be numeric or empty, got %T", raw),
		}
	}
	if err != nil {
		return nil, &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: "number value must be numeric or empty",
		}
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: "number value must be finite",
		}
	}
	return value, nil
}

func normalizeDateValue(fieldKey string, raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil, &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: "date value must use YYYY-MM-DD format or be empty",
		}
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		return nil, &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: "date value must use YYYY-MM-DD format or be empty",
		}
	}
	return value, nil
}

// ValidateCheckboxValue enforces the asset-aligned boolean value contract. An
// empty value is valid, and a supplied value must be an actual Go/JSON boolean.
// Requiredness never changes boolean semantics: both true and false are valid.
func ValidateCheckboxValue(fieldID string, raw any) (bool, error) {
	if raw == nil {
		return false, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("field %s must be a boolean value", fieldID)
	}
	return value, nil
}

// CustomFieldTypes bulk-resolves field types for known numeric CFV keys.
func CustomFieldTypes(db database.Database, cfv map[string]any) (map[string]string, error) {
	fields, err := loadFieldsForCFV(db, cfv)
	if err != nil {
		return nil, err
	}
	types := make(map[string]string, len(fields))
	for key, def := range fields {
		types[key] = def.FieldType
	}
	return types, nil
}

// sanitizeTextValue applies the matching text policy; non-strings pass through.
func sanitizeTextValue(fieldType string, raw any) any {
	s, ok := raw.(string)
	if !ok {
		return raw
	}
	if fieldType == "textarea" {
		return sanitize.RichText.Sanitize(s)
	}
	return sanitize.PlainTextField.Sanitize(s)
}

func loadFieldsForCFV(db database.Database, cfv map[string]any) (map[string]*models.CustomFieldDefinition, error) {
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

	// options is nullable for every non-choice field. Scan a stable empty value
	// so validating ordinary text/textarea fields does not fail before their
	// type-specific sanitizer runs.
	query := "SELECT id, field_type, COALESCE(options, '') FROM custom_field_definitions WHERE id IN (" + placeholders + ")"
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

// normalizeSelectValue accepts an integer or numeric-string value that matches
// a known option id on the field and returns the canonical integer id. Anything
// else is a ValidationError.
func normalizeSelectValue(fieldKey string, def *models.CustomFieldDefinition, raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	id, ok := CoerceOptionID(raw)
	if !ok {
		return nil, &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: fmt.Sprintf("select option id must be an integer, got %v (%T)", raw, raw),
		}
	}
	allowed, err := optionIDSet(def)
	if err != nil {
		return nil, err
	}
	if !allowed[id] {
		return nil, &ValidationError{
			Field:   "custom_field_values." + fieldKey,
			Message: fmt.Sprintf("option id %d is not in the field's option set", id),
		}
	}
	return id, nil
}

// validateAndDedupeMultiselect validates option IDs and preserves first-seen order.
func validateAndDedupeMultiselect(fieldKey string, def *models.CustomFieldDefinition, raw any) ([]int, error) {
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
		id, ok := CoerceOptionID(v)
		if !ok {
			return nil, &ValidationError{
				Field:   "custom_field_values." + fieldKey,
				Message: fmt.Sprintf("multiselect option id must be an integer, got %v (%T)", v, v),
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

// CoerceOptionID accepts integral JSON numbers, Go ints, and legacy numeric
// strings as a select/multiselect/milestone option id. Fractional, NaN, and
// infinite floats are rejected rather than truncated so an invalid id can never
// match a real option.
func CoerceOptionID(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0, false
		}
		id := int(x)
		// Round-tripping catches fractional values and floats outside int range.
		if float64(id) != x {
			return 0, false
		}
		return id, true
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

// coerceToSlice accepts JSON-decoded arrays and Go-side []int values.
func coerceToSlice(v any) ([]any, error) {
	switch x := v.(type) {
	case []any:
		return x, nil
	case []int:
		out := make([]any, len(x))
		for i, n := range x {
			out[i] = n
		}
		return out, nil
	}
	return nil, fmt.Errorf("not an array")
}
