package cql

import (
	"encoding/json"
	"strings"
)

// CustomFieldKind classifies a custom field by how its value is stored, which
// determines the SQL lowering strategy used by the generator.
type CustomFieldKind int

const (
	// CFKindScalar covers text, textarea, number, date, select, milestone, iteration —
	// any field whose stored JSON value is a scalar comparable directly with ->>.
	CFKindScalar CustomFieldKind = iota
	// CFKindMultiselect: value is a JSON array of option IDs (or strings); requires
	// array containment semantics.
	CFKindMultiselect
	// CFKindReference: value is either a scalar id OR an object {id, name, ...}.
	// Comparisons must check both the direct scalar and the nested .id.
	CFKindReference
	// CFKindLinking: value is not in custom_field_values at all — relations live in
	// the item_links table keyed by custom_field_id.
	CFKindLinking
	// CFKindBoolean: checkbox fields. Currently disabled in validFieldTypes but kept
	// here so the dispatcher is complete.
	CFKindBoolean
)

// CustomFieldInfo identifies a custom field by its numeric ID and how its value
// is stored. Used by the QL generator to route comparisons to the right extractor.
//
// MirrorOfFieldID is non-zero for mirror linking fields: the row's id refers to
// the mirror definition, but the actual link rows in item_links live under the
// primary's id. The generator must query in the reverse direction (source ↔
// target) and use the primary id for the custom_field_id filter.
//
// AllowedTargetTypes lists the entity types valid as link targets (e.g.
// ["item"] or ["item","asset"]). When non-empty the generator constrains
// target_type (or source_type, for mirrors) to that set so a target_id of 42
// doesn't ambiguously match across entity types.
type CustomFieldInfo struct {
	ID   int
	Kind CustomFieldKind
	// FieldType is the lowercase field_type string from the DB (e.g. "date",
	// "number", "text"). The generator uses this to match the per-field
	// Postgres expression indexes created in handlers/custom_fields.go —
	// date fields wrap the extract in CAST(... AS TEXT), so QL must too.
	FieldType          string
	MirrorOfFieldID    int
	AllowedTargetTypes []string
}

// LinkingFieldOptions extracts the linking-relevant fields from a custom field
// definition's options JSON. Returns zero values when the JSON is empty or
// invalid — callers should treat that as "no mirror, no entity-type constraint."
func LinkingFieldOptions(optionsJSON string) (mirrorOfFieldID int, allowedTargetTypes []string) {
	if strings.TrimSpace(optionsJSON) == "" {
		return 0, nil
	}
	var opts struct {
		MirrorOfFieldID    int      `json:"mirror_of_field_id"`
		AllowedEntityTypes []string `json:"allowed_entity_types"`
	}
	if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
		return 0, nil
	}
	return opts.MirrorOfFieldID, opts.AllowedEntityTypes
}

// CustomFieldMap maps a lowercase custom-field name to its info. The generator
// uses this to resolve UI-supplied names (cf_<name>) to numeric JSON keys and to
// pick the right lowering strategy per field type.
type CustomFieldMap map[string]CustomFieldInfo

// ClassifyCustomFieldKind maps a field_type string from custom_field_definitions
// to the kind used by the QL generator. Unknown types fall back to scalar so the
// generator continues to behave as today.
func ClassifyCustomFieldKind(fieldType string) CustomFieldKind {
	switch strings.ToLower(fieldType) {
	case "multiselect":
		return CFKindMultiselect
	case "user", "asset", "portalcustomer", "customerorganisation":
		return CFKindReference
	case "linking":
		return CFKindLinking
	case "checkbox", "boolean":
		return CFKindBoolean
	default:
		return CFKindScalar
	}
}
