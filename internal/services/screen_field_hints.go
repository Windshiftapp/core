package services

import (
	"database/sql"
	"strings"

	"windshift/internal/models"
	"windshift/internal/sanitize"
)

// normalizeHintText normalizes line endings, trims, and caps a hint at
// models.ScreenFieldHintMaxLength runes. Line breaks are kept: multi-line
// hints render as multi-line placeholders.
func normalizeHintText(hint string) string {
	hint = strings.ReplaceAll(hint, "\r\n", "\n")
	return strings.TrimSpace(truncateRunes(strings.TrimSpace(hint), models.ScreenFieldHintMaxLength))
}

// normalizeScreenFieldHint returns the hint a screen field stores: sanitized,
// trimmed, and capped, or "" when the field cannot show one. System fields
// qualify only for the description; custom fields only when free text (text,
// multi-line, rich text). The screen API, config-set import and conformance
// repair all go through this so stored rows and templates agree.
func normalizeScreenFieldHint(fieldType, identifier, customFieldType, hint string) string {
	hint = normalizeHintText(sanitize.PlainTextField.Sanitize(hint))
	if hint == "" {
		return ""
	}
	switch fieldType {
	case "system", "default":
		if !models.SystemFieldSupportsHint(strings.TrimSpace(identifier)) {
			return ""
		}
	case "custom":
		if !models.CustomFieldTypeSupportsHint(customFieldType) {
			return ""
		}
	default:
		return ""
	}
	return hint
}

// screenFieldHintValue is the stored form of a screen field hint: NULL when
// the field cannot show one or the hint is empty.
func screenFieldHintValue(fieldType, identifier, customFieldType, hint string) sql.NullString {
	hint = normalizeScreenFieldHint(fieldType, identifier, customFieldType, hint)
	return sql.NullString{String: hint, Valid: hint != ""}
}
