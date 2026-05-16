package cql

import (
	"strings"
	"testing"
)

func parseQL(t *testing.T, query string) (*ASTNode, error) {
	t.Helper()
	tokens, err := NewTokenizer(query).Tokenize()
	if err != nil {
		return nil, err
	}
	return NewParser(tokens).Parse()
}

func TestParser_ChainedNOT(t *testing.T) {
	cases := []string{
		`NOT status = "open"`,
		`NOT NOT status = "open"`,
		`NOT NOT NOT status = "open"`,
	}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			if _, err := parseQL(t, q); err != nil {
				t.Fatalf("expected %q to parse, got error: %v", q, err)
			}
		})
	}
}

func TestParser_UnclosedParen(t *testing.T) {
	cases := []struct {
		query string
		// We at least want the error to mention `)` somewhere — the exact
		// wording is allowed to drift.
		expectMention string
	}{
		{`(status = "open"`, `)`},
		{`((status = "open" AND priority = "high")`, `)`},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			_, err := parseQL(t, tc.query)
			if err == nil {
				t.Fatalf("expected error parsing %q, got none", tc.query)
			}
			if !strings.Contains(err.Error(), tc.expectMention) {
				t.Fatalf("expected error to mention %q, got: %v", tc.expectMention, err)
			}
		})
	}
}

// cf_x != y uses standard SQL NULL semantics: items without the field set
// (NULL/missing JSON key) do NOT match. A query author who wants those rows
// included must say so explicitly (e.g. `cf_x IS NULL OR cf_x != "y"`).
func TestGenerator_NotEqualForCustomFieldsUsesStandardNullSemantics(t *testing.T) {
	ast, err := parseQL(t, `cf_status != "active"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(sqlStr, "IS NULL OR") {
		t.Fatalf("expected standard != without NULL inclusion, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "!=") {
		t.Fatalf("expected != in generated SQL, got: %s", sqlStr)
	}
}

func TestGenerator_TildeOnCustomTextField(t *testing.T) {
	ast, err := parseQL(t, `cf_notes ~ "todo"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "LIKE") {
		t.Fatalf("expected LIKE clause, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `ESCAPE '\'`) {
		t.Fatalf("expected ESCAPE clause to be present, got: %s", sqlStr)
	}
	if len(args) == 0 {
		t.Fatalf("expected at least one bound arg")
	}
}

// SQLite ->> on a JSON boolean returns INTEGER 1/0, so the bound arg stays an
// int64 to match the storage form.
func TestGenerator_BooleanCustomField_SQLite(t *testing.T) {
	ast, err := parseQL(t, `cf_done = true`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	_, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	found := false
	for _, a := range args {
		if v, ok := a.(int64); ok && (v == 1 || v == 0) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected an int64 1/0 arg for SQLite boolean comparison, got: %#v", args)
	}
}

// Postgres ->> on a JSON boolean returns text "true"/"false", so the int bound
// arg is rewritten to the matching string form.
func TestGenerator_BooleanCustomField_Postgres(t *testing.T) {
	ast, err := parseQL(t, `cf_done = true`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "postgres")
	_, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	found := false
	for _, a := range args {
		if s, ok := a.(string); ok && (s == "true" || s == "false") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a string \"true\" arg for Postgres boolean comparison, got: %#v", args)
	}
}

// When a CustomFieldMap is supplied, cf_<name> resolves to the numeric custom
// field ID and the JSON key is inlined into the SQL (no `?` for the key). This
// matches the storage shape (custom_field_values keyed by numeric ID) and lets
// the Postgres planner match the per-field expression indexes.
func TestGenerator_CustomFieldMapResolvesNameToInlinedID_SQLite(t *testing.T) {
	ast, err := parseQL(t, `cf_Severity = "High"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"severity": {ID: 123, Kind: CFKindScalar}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, `'$."123"'`) {
		t.Fatalf("expected inlined numeric JSON key '$.\"123\"' in SQL, got: %s", sqlStr)
	}
	if strings.Contains(sqlStr, "Severity") || strings.Contains(sqlStr, `"severity"`) {
		t.Fatalf("expected name to be replaced by numeric ID, got: %s", sqlStr)
	}
	// Right-hand value remains parameterized.
	if len(args) == 0 {
		t.Fatalf("expected RHS arg to be parameterized, got 0 args")
	}
}

func TestGenerator_CustomFieldMapResolvesNameToInlinedID_Postgres(t *testing.T) {
	ast, err := parseQL(t, `cf_Severity = "High"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"severity": {ID: 123, Kind: CFKindScalar}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "postgres")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "->>'123'") {
		t.Fatalf("expected inlined numeric JSON key ->>'123' in SQL, got: %s", sqlStr)
	}
}

// Backward compatibility: with no map, fall back to name-based extraction.
func TestGenerator_CustomFieldNoMapFallsBackToName(t *testing.T) {
	ast, err := parseQL(t, `cf_Severity = "High"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, `"Severity"`) {
		t.Fatalf("expected legacy name-based JSON key, got: %s", sqlStr)
	}
}

// Reference custom fields can store either a direct scalar id or an object
// {id, name, ...}. The comparison must check both forms so a query like
// cf_Owner = 7 matches both {"123":7} and {"123":{"id":7,"name":"A"}}.
func TestGenerator_ReferenceCustomField_EmitsDirectAndNestedID_SQLite(t *testing.T) {
	ast, err := parseQL(t, `cf_Owner = 7`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"owner": {ID: 123, Kind: CFKindReference}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, `'$."123"'`) {
		t.Fatalf("expected direct extract '$.\"123\"' in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `'$."123".id'`) {
		t.Fatalf("expected nested .id extract '$.\"123\".id' in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, " OR ") {
		t.Fatalf("expected an OR joining the two extractors, got: %s", sqlStr)
	}
}

func TestGenerator_ReferenceCustomField_EmitsDirectAndNestedID_Postgres(t *testing.T) {
	ast, err := parseQL(t, `cf_Owner = 7`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"owner": {ID: 123, Kind: CFKindReference}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "postgres")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "->>'123'") {
		t.Fatalf("expected direct extract ->>'123' in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "->'123'->>'id'") {
		t.Fatalf("expected nested ->'123'->>'id' in SQL, got: %s", sqlStr)
	}
}

// Without a customFieldMap, the reference dispatcher does not engage and the
// query falls through to the generic scalar path (legacy behavior).
func TestGenerator_ReferenceCustomField_NoMapFallsThrough(t *testing.T) {
	ast, err := parseQL(t, `cf_Owner = 7`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(sqlStr, ".id'") {
		t.Fatalf("expected legacy single-extract path with no map, got nested form: %s", sqlStr)
	}
}

// Multiselect custom fields store JSON arrays of option IDs. =/~ mean
// "contains"; != means "does not contain"; IN (...) means "contains any of".
func TestGenerator_MultiselectCustomField_EqualsEmitsExistsJsonEach_SQLite(t *testing.T) {
	ast, err := parseQL(t, `cf_Tags = 1`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"tags": {ID: 123, Kind: CFKindMultiselect}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "json_each") {
		t.Fatalf("expected json_each subquery, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `'$."123"'`) {
		t.Fatalf("expected JSON path '$.\"123\"', got: %s", sqlStr)
	}
	if len(args) != 1 || args[0] != "1" {
		t.Fatalf("expected text-coerced bound arg \"1\", got: %#v", args)
	}
}

func TestGenerator_MultiselectCustomField_INEmitsExistsAnyOf_Postgres(t *testing.T) {
	ast, err := parseQL(t, `cf_Tags IN (1, 3)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"tags": {ID: 123, Kind: CFKindMultiselect}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "postgres")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "jsonb_array_elements_text") {
		t.Fatalf("expected jsonb_array_elements_text subquery, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "->'123'") {
		t.Fatalf("expected ->'123' for array extract, got: %s", sqlStr)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got: %#v", args)
	}
}

func TestGenerator_MultiselectCustomField_NotEqualsNegates(t *testing.T) {
	ast, err := parseQL(t, `cf_Tags != 9`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"tags": {ID: 123, Kind: CFKindMultiselect}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(sqlStr), "NOT EXISTS") {
		t.Fatalf("expected NOT EXISTS prefix for !=, got: %s", sqlStr)
	}
}

// Linking custom fields live in item_links, not custom_field_values. =/!= lower
// to EXISTS / NOT EXISTS against item_links scoped to the current item id.
func TestGenerator_LinkingCustomField_EqualsEmitsExistsItemLinks(t *testing.T) {
	ast, err := parseQL(t, `cf_LinkedFeature = 42`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"linkedfeature": {ID: 7, Kind: CFKindLinking}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "item_links") {
		t.Fatalf("expected item_links subquery, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "custom_field_id = 7") {
		t.Fatalf("expected custom_field_id = 7 in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "source_type = 'item'") {
		t.Fatalf("expected source_type = 'item' filter, got: %s", sqlStr)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 bound arg, got: %#v", args)
	}
}

func TestGenerator_LinkingCustomField_INEmitsTargetIDList(t *testing.T) {
	ast, err := parseQL(t, `cf_LinkedFeature IN (40, 41, 42)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"linkedfeature": {ID: 7, Kind: CFKindLinking}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "postgres")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "il.target_id IN (?, ?, ?)") {
		t.Fatalf("expected target_id IN list, got: %s", sqlStr)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %#v", len(args), args)
	}
}

// ~ doesn't make sense on linking (target ids are not strings to LIKE against);
// reject with a clear error rather than emitting nonsense SQL.
func TestGenerator_LinkingCustomField_TildeRejected(t *testing.T) {
	ast, err := parseQL(t, `cf_LinkedFeature ~ "42"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"linkedfeature": {ID: 7, Kind: CFKindLinking}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	if _, _, err := gen.GenerateSQL(ast); err == nil {
		t.Fatalf("expected ~ on linking field to error, got none")
	}
}

// The frontend FieldSelector exposes the label filter as `labels` (plural).
// Backend must accept both `label` and `labels` so saved UI queries don't fail.
func TestGenerator_LabelsAliasResolvesLikeLabel(t *testing.T) {
	ast, err := parseQL(t, `labels = "bug"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "item_labels") {
		t.Fatalf("expected labels alias to dispatch to item_labels EXISTS subquery, got: %s", sqlStr)
	}
}

// `milestone IN (...)` with string values should compare by milestone name
// instead of by id — the UI emits names for multi-select milestone pickers.
func TestGenerator_MilestoneInWithStringsCoercesToName(t *testing.T) {
	ast, err := parseQL(t, `milestone IN ("Release A", "Release B")`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "LOWER(ms.name)") {
		t.Fatalf("expected name comparison via LOWER(ms.name), got: %s", sqlStr)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 bound args, got: %#v", args)
	}
}

// `milestone IN (1, 2)` with numeric values keeps the existing by-id path.
func TestGenerator_MilestoneInWithNumbersStaysByID(t *testing.T) {
	ast, err := parseQL(t, `milestone IN (1, 2)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "ms_im.milestone_id IN") {
		t.Fatalf("expected milestone_id IN, got: %s", sqlStr)
	}
}

// Mixed types are ambiguous (numeric IDs vs name strings) and rejected.
func TestGenerator_MilestoneInMixedTypesErrors(t *testing.T) {
	ast, err := parseQL(t, `milestone IN (1, "Release B")`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	if _, _, err := gen.GenerateSQL(ast); err == nil {
		t.Fatalf("expected error for mixed milestone IN values, got none")
	}
}

// `<field> IS NULL` parses and lowers to standard `IS NULL` SQL for scalar
// fields.
func TestGenerator_IsNullOnScalarCustomField(t *testing.T) {
	ast, err := parseQL(t, `cf_Severity IS NULL`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"severity": {ID: 123, Kind: CFKindScalar}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasSuffix(strings.TrimSpace(sqlStr), "IS NULL") {
		t.Fatalf("expected SQL to end with IS NULL, got: %s", sqlStr)
	}
}

func TestGenerator_IsNotNullOnScalarCustomField(t *testing.T) {
	ast, err := parseQL(t, `cf_Severity IS NOT NULL`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"severity": {ID: 123, Kind: CFKindScalar}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "IS NOT NULL") {
		t.Fatalf("expected IS NOT NULL in SQL, got: %s", sqlStr)
	}
}

// `cf_x = null` is sugar for `cf_x IS NULL`.
func TestGenerator_EqualsNullRewritesToIsNull(t *testing.T) {
	ast, err := parseQL(t, `cf_Severity = null`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"severity": {ID: 123, Kind: CFKindScalar}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.HasSuffix(strings.TrimSpace(sqlStr), "IS NULL") {
		t.Fatalf("expected IS NULL rewrite, got: %s", sqlStr)
	}
	if len(args) != 0 {
		t.Fatalf("expected no bound args for IS NULL, got %d: %#v", len(args), args)
	}
}

// Negative numeric literals tokenize after operators / IN / comma.
func TestTokenizer_NegativeNumberAfterOperator(t *testing.T) {
	ast, err := parseQL(t, `cf_score < -1`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	_, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	found := false
	for _, a := range args {
		if v, ok := a.(int64); ok && v == -1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected -1 in bound args, got: %#v", args)
	}
}

func TestTokenizer_NegativeNumberInIN(t *testing.T) {
	if _, err := parseQL(t, `cf_score IN (-1, -2, 0)`); err != nil {
		t.Fatalf("expected negative numbers in IN to parse, got: %v", err)
	}
}

// `custom.<name>` (without backticks) tokenizes as a single identifier.
func TestTokenizer_DottedCustomIdentifier(t *testing.T) {
	if _, err := parseQL(t, `custom.epicLink = "PROJ-123"`); err != nil {
		t.Fatalf("expected dotted custom.* identifier to parse, got: %v", err)
	}
}

// linkedOf() inside an asset query spawns an inner item-side generator. That
// inner generator must receive the item-side custom-field map so cf_<name>
// resolves to the right numeric JSON key — not the asset map (which would map
// asset CFs) and not nil (which would fall back to legacy name extraction).
func TestGenerator_AssetLinkedOfInnerQueryUsesItemCustomFieldMap(t *testing.T) {
	ast, err := parseQL(t, `linkedOf("relates", "cf_Severity = \"High\"")`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	assetMap := CustomFieldMap{"size": {ID: 7, Kind: CFKindScalar}}
	itemMap := CustomFieldMap{"severity": {ID: 123, Kind: CFKindScalar}}
	gen := NewAssetSQLGenerator(map[string]int{}, assetMap, itemMap, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, `'$."123"'`) {
		t.Fatalf("expected inner item query to resolve cf_Severity to id 123, got: %s", sqlStr)
	}
	if strings.Contains(sqlStr, "Severity") {
		t.Fatalf("expected name to be replaced by numeric id in inner SQL, got: %s", sqlStr)
	}
}

// Reference custom fields can store either a direct scalar id or an object
// {id, name, ...}; IN must check both forms, like the equality path.
func TestGenerator_ReferenceCustomField_INChecksDirectAndNested_SQLite(t *testing.T) {
	ast, err := parseQL(t, `cf_Owner IN (7, 8)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"owner": {ID: 123, Kind: CFKindReference}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, `'$."123"'`) {
		t.Fatalf("expected direct extract '$.\"123\"' in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, `'$."123".id'`) {
		t.Fatalf("expected nested .id extract '$.\"123\".id' in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, " OR ") {
		t.Fatalf("expected OR joining direct/nested branches, got: %s", sqlStr)
	}
	// Each of 7 and 8 should appear twice (once per branch).
	if len(args) != 4 {
		t.Fatalf("expected 4 bound args (2 values x 2 branches), got %d: %#v", len(args), args)
	}
}

// Postgres date custom-field indexes use CAST(... AS TEXT) per the DDL in
// handlers/custom_fields.go. The generator must mirror that wrapper so the
// planner can match the index expression.
func TestGenerator_DateCustomField_PostgresWrapsInCastText(t *testing.T) {
	ast, err := parseQL(t, `cf_BirthDate = "2020-01-01"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"birthdate": {ID: 123, Kind: CFKindScalar, FieldType: "date"}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "postgres")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "CAST(") || !strings.Contains(sqlStr, "AS TEXT") {
		t.Fatalf("expected CAST(... AS TEXT) wrap to match index, got: %s", sqlStr)
	}
}

// Non-date scalar fields don't get the extra CAST wrap on Postgres.
func TestGenerator_NonDateScalarOnPostgresNoExtraCast(t *testing.T) {
	ast, err := parseQL(t, `cf_Severity = "High"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"severity": {ID: 123, Kind: CFKindScalar, FieldType: "text"}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "postgres")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(sqlStr, "AS TEXT") {
		t.Fatalf("expected no AS TEXT wrap for text field on PG, got: %s", sqlStr)
	}
}

// Mirror linking fields store nothing on their own side — the actual link rows
// live under the primary field's id, with source/target swapped. QL must query
// in the reverse direction (current item = target, value = source).
func TestGenerator_LinkingCustomField_MirrorReversesDirection(t *testing.T) {
	ast, err := parseQL(t, `cf_RelatedTo = 42`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"relatedto": {ID: 99, Kind: CFKindLinking, MirrorOfFieldID: 7}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// custom_field_id should match the PRIMARY's id (7), not the mirror's (99).
	if !strings.Contains(sqlStr, "custom_field_id = 7") {
		t.Fatalf("expected custom_field_id = 7 (mirror_of_field_id), got: %s", sqlStr)
	}
	// Current item should appear on the target side, value on the source side.
	if !strings.Contains(sqlStr, "il.target_id = i.id") {
		t.Fatalf("expected mirror to put current item on target side, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "il.source_id IN (?)") {
		t.Fatalf("expected mirror to put bound value on source side, got: %s", sqlStr)
	}
}

// AllowedTargetTypes constrains the type column on the other side of the link
// so a target_id of 42 doesn't accidentally match across entity types.
func TestGenerator_LinkingCustomField_TargetTypeConstraint(t *testing.T) {
	ast, err := parseQL(t, `cf_Asset = 42`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"asset": {ID: 7, Kind: CFKindLinking, AllowedTargetTypes: []string{"asset"}}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "il.target_type = 'asset'") {
		t.Fatalf("expected target_type = 'asset' constraint, got: %s", sqlStr)
	}
}

// Multi-value AllowedTargetTypes becomes an IN clause.
func TestGenerator_LinkingCustomField_TargetTypeMultiAllowedUsesIN(t *testing.T) {
	ast, err := parseQL(t, `cf_Reference = 42`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"reference": {ID: 7, Kind: CFKindLinking, AllowedTargetTypes: []string{"item", "asset"}}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "il.target_type IN ('item', 'asset')") {
		t.Fatalf("expected target_type IN list, got: %s", sqlStr)
	}
}

// cfid_<id> is the stable, collision-free QL form: addresses the field by its
// numeric DB id directly, with no name lookup. Useful when custom-field names
// collide across scopes.
func TestGenerator_CFIDIdentifierResolvesDirectlyToJSONKey_SQLite(t *testing.T) {
	ast, err := parseQL(t, `cfid_123 = "High"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, `'$."123"'`) {
		t.Fatalf("expected inlined '$.\"123\"' JSON path, got: %s", sqlStr)
	}
}

func TestGenerator_CFIDIdentifierResolvesDirectlyToJSONKey_Postgres(t *testing.T) {
	ast, err := parseQL(t, `cfid_123 = "High"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "postgres")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "->>'123'") {
		t.Fatalf("expected inlined ->>'123', got: %s", sqlStr)
	}
}

// cfid_<id> picks up the Kind from the map when one is provided, so per-kind
// dispatch (reference, multiselect, linking) routes correctly.
func TestGenerator_CFIDIdentifierUsesMapKindForReferenceDispatch(t *testing.T) {
	ast, err := parseQL(t, `cfid_123 = 7`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"owner": {ID: 123, Kind: CFKindReference}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, `'$."123".id'`) {
		t.Fatalf("expected reference dispatch (nested .id check), got: %s", sqlStr)
	}
}

// Backticked custom-field names starting with a digit (e.g. "123 Score")
// previously failed validation. The relaxed regex allows them through.
func TestGenerator_CustomFieldNameWithLeadingDigit(t *testing.T) {
	ast, err := parseQL(t, "`cf_123 Score` = 5")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	if _, _, err := gen.GenerateSQL(ast); err != nil {
		t.Fatalf("expected generation to succeed for digit-prefixed CF name, got: %v", err)
	}
}

// Reference != uses COALESCE(nested, direct) so legacy scalar storage works
// alongside object-backed storage. The previous (direct != ? AND nested != ?)
// form returned NULL (filtering the row out) when nested was NULL.
func TestGenerator_ReferenceCustomField_NotEqualUsesCoalesce(t *testing.T) {
	ast, err := parseQL(t, `cf_Owner != 7`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"owner": {ID: 123, Kind: CFKindReference}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "COALESCE(") {
		t.Fatalf("expected COALESCE wrapper for null-safe !=, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "!=") {
		t.Fatalf("expected != in SQL, got: %s", sqlStr)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 bound arg, got %d: %#v", len(args), args)
	}
}

// NOT IN uses COALESCE(nested, direct) to collapse object/scalar dual storage
// into one effective ID — the naive AND of two != branches drops rows where the
// nested form is NULL (true AND NULL = NULL, filtered out).
func TestGenerator_ReferenceCustomField_NOTINUsesCoalesce(t *testing.T) {
	ast, err := parseQL(t, `cf_Owner NOT IN (7, 8)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"owner": {ID: 123, Kind: CFKindReference}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "sqlite")
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "NOT IN") {
		t.Fatalf("expected NOT IN in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "COALESCE(") {
		t.Fatalf("expected COALESCE wrapper for null-safe NOT IN, got: %s", sqlStr)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 bound args (single IN list), got %d: %#v", len(args), args)
	}
}

func TestGenerator_ReferenceCustomField_INPostgres(t *testing.T) {
	ast, err := parseQL(t, `cf_Owner IN (7, 8)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfMap := CustomFieldMap{"owner": {ID: 123, Kind: CFKindReference}}
	gen := NewSQLGenerator(map[string]int{}, cfMap, "postgres")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "->>'123'") {
		t.Fatalf("expected direct ->>'123' in SQL, got: %s", sqlStr)
	}
	if !strings.Contains(sqlStr, "->'123'->>'id'") {
		t.Fatalf("expected nested ->'123'->>'id' in SQL, got: %s", sqlStr)
	}
}

func TestGenerator_TildeEscapesLikeWildcards(t *testing.T) {
	ast, err := parseQL(t, `title ~ "50%"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, nil, "sqlite")
	_, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(args) != 1 {
		t.Fatalf("expected 1 bound arg, got %d: %v", len(args), args)
	}
	got, ok := args[0].(string)
	if !ok {
		t.Fatalf("expected string arg, got %T", args[0])
	}
	if got != `50\%` {
		t.Fatalf("expected escaped pattern %q, got %q", `50\%`, got)
	}
}
