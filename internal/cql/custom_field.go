package cql

import "strings"

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
type CustomFieldInfo struct {
	ID   int
	Kind CustomFieldKind
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
