package cql

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SQLGenerator converts QL AST to SQL WHERE clause
type SQLGenerator struct {
	workspaceMap          map[string]int // Maps workspace names/keys to IDs
	aliasPrefix           string         // Prefix for table aliases ("" for outer, "inner_" for inner queries)
	entityType            EntityType     // Type of entity being queried (item or asset)
	setMap                map[string]int // Maps asset set names to IDs (for asset queries)
	dbDriver              string         // Database driver name ("sqlite" or "postgres")
	customFieldMap        CustomFieldMap // Maps lowercase custom field name to {ID, Kind} for the entity being queried
	itemCustomFieldMap    CustomFieldMap // Item-side custom field map, used by inner item queries inside asset linkedOf()
	legacyNameKeyFallback bool           // Also read legacy custom_field_values keyed by field name
	evaluationTime        time.Time      // Captured once per top-level SQL generation
	itemWorkspaceScope    []int
	itemWorkspaceScoped   bool
}

// NewSQLGenerator creates an outer work-item query generator. A nil field map
// uses legacy name-based JSON extraction.
func NewSQLGenerator(workspaceMap map[string]int, customFieldMap CustomFieldMap, dbDriver string) *SQLGenerator {
	return &SQLGenerator{
		workspaceMap:       workspaceMap,
		aliasPrefix:        "",
		entityType:         EntityTypeItem,
		dbDriver:           dbDriver,
		customFieldMap:     customFieldMap,
		itemCustomFieldMap: customFieldMap,
	}
}

// NewInnerSQLGenerator creates a nested work-item generator with noncolliding
// table aliases.
func NewInnerSQLGenerator(workspaceMap map[string]int, customFieldMap CustomFieldMap, dbDriver string) *SQLGenerator {
	return &SQLGenerator{
		workspaceMap:       workspaceMap,
		aliasPrefix:        "inner_",
		entityType:         EntityTypeItem,
		dbDriver:           dbDriver,
		customFieldMap:     customFieldMap,
		itemCustomFieldMap: customFieldMap,
	}
}

// NewAssetSQLGenerator creates an asset generator; linkedOf inner queries use
// itemCustomFieldMap.
func NewAssetSQLGenerator(setMap map[string]int, assetCustomFieldMap, itemCustomFieldMap CustomFieldMap, dbDriver string) *SQLGenerator {
	return &SQLGenerator{
		setMap:             setMap,
		aliasPrefix:        "",
		entityType:         EntityTypeAsset,
		dbDriver:           dbDriver,
		customFieldMap:     assetCustomFieldMap,
		itemCustomFieldMap: itemCustomFieldMap,
	}
}

// EnableLegacyCustomFieldNameFallback reads name-keyed values when the numeric
// key is absent, for older custom_field_values rows.
func (g *SQLGenerator) EnableLegacyCustomFieldNameFallback() {
	g.legacyNameKeyFallback = true
}

// jsonExtract returns a parameterized DB-specific JSON expression.
func (g *SQLGenerator) jsonExtract(column, field string) (expr string, args []any) {
	if g.dbDriver == "postgres" {
		return fmt.Sprintf("%s->>?", column), []any{field}
	}
	// NULLIF prevents malformed-JSON errors from legacy empty strings.
	path := fmt.Sprintf("$.\"%s\"", field) //nolint:gocritic // JSON path requires quoted field name
	return fmt.Sprintf("NULLIF(%s, '') ->> '%s'", column, path), nil
}

// jsonExtractLiteralKey inlines a trusted field ID so Postgres can use its
// per-field expression index.
func (g *SQLGenerator) jsonExtractLiteralKey(column string, fieldID int) string {
	if g.dbDriver == "postgres" {
		return fmt.Sprintf("%s->>'%d'", column, fieldID)
	}
	return fmt.Sprintf(`NULLIF(%s, '') ->> '$."%d"'`, column, fieldID)
}

// GenerateSQL converts a QL AST to SQL WHERE clause
func (g *SQLGenerator) GenerateSQL(ast *ASTNode) (sql string, args []any, err error) {
	return g.GenerateSQLAt(ast, time.Now().UTC())
}

// GenerateSQLAt converts an AST using one caller-provided evaluation time.
// The generator copy keeps request-specific state out of reusable generators.
func (g *SQLGenerator) GenerateSQLAt(ast *ASTNode, evaluationTime time.Time) (sql string, args []any, err error) {
	if ast == nil {
		return "", nil, nil
	}

	local := *g
	local.evaluationTime = evaluationTime.UTC()
	clause, args, err := local.generateNode(ast)
	if err != nil || !local.itemWorkspaceScoped || local.entityType != EntityTypeItem {
		return clause, args, err
	}
	if len(local.itemWorkspaceScope) == 0 {
		return "1 = 0", nil, nil
	}
	placeholders := make([]string, len(local.itemWorkspaceScope))
	for i, id := range local.itemWorkspaceScope {
		placeholders[i] = "?"
		args = append(args, id)
	}
	return fmt.Sprintf("(%s) AND %si.workspace_id IN (%s)", clause, local.aliasPrefix, strings.Join(placeholders, ",")), args, nil
}

func (g *SQLGenerator) innerGenerator(fields CustomFieldMap) *SQLGenerator {
	inner := NewInnerSQLGenerator(g.workspaceMap, fields, g.dbDriver)
	inner.legacyNameKeyFallback = g.legacyNameKeyFallback
	inner.itemWorkspaceScope = g.itemWorkspaceScope
	inner.itemWorkspaceScoped = g.itemWorkspaceScoped
	return inner
}

// generateNode generates SQL for a single AST node
func (g *SQLGenerator) generateNode(node *ASTNode) (sql string, args []any, err error) {
	switch node.Type {
	case NodeBinaryOp:
		return g.generateBinaryOp(node)
	case NodeComparison:
		return g.generateComparison(node)
	case NodeInExpression:
		return g.generateInExpression(node)
	case NodeNullCheck:
		return g.generateNullCheck(node)
	case NodeIdentifier:
		sql, args, err := g.mapFieldName(node.Value)
		if err != nil {
			return "", nil, err
		}
		return sql, args, nil
	case NodeLiteral:
		return "?", []any{g.convertLiteral(node)}, nil
	case NodeFunction:
		return g.generateFunction(node)
	default:
		return "", nil, fmt.Errorf("unsupported node type: %v", node.Type)
	}
}

// generateNullCheck rewrites non-scalar custom fields as (NOT) EXISTS.
func (g *SQLGenerator) generateNullCheck(node *ASTNode) (sql string, args []any, err error) {
	negated := strings.EqualFold(node.Operator, "IS NOT NULL")
	if node.Left.Type == NodeIdentifier {
		if info, ok := g.lookupCustomFieldInfo(node.Left.Value); ok {
			switch info.Kind {
			case CFKindMultiselect:
				column := g.customFieldColumn()
				// Multiselect nullness means whether the array has an element.
				expr := g.multiselectAnyValueExpression(column, info)
				if negated {
					return expr, nil, nil
				}
				return "NOT " + expr, nil, nil
			case CFKindLinking:
				sourceID := g.aliasPrefix + "i.id"
				expr := fmt.Sprintf(
					"EXISTS (SELECT 1 FROM item_links il WHERE il.source_type = 'item' AND il.source_id = %s AND il.custom_field_id = %d)",
					sourceID, info.ID,
				)
				if negated {
					return expr, nil, nil
				}
				return "NOT " + expr, nil, nil
			case CFKindReference:
				column := g.customFieldColumn()
				directExprs, nestedExprs := g.referenceExtractExpressionsForInfo(column, info)
				allExprs := append(append([]string{}, directExprs...), nestedExprs...)
				if negated {
					clauses := make([]string, 0, len(allExprs))
					for _, expr := range allExprs {
						clauses = append(clauses, fmt.Sprintf("%s IS NOT NULL", expr))
					}
					return "(" + strings.Join(clauses, " OR ") + ")", nil, nil
				}
				clauses := make([]string, 0, len(allExprs))
				for _, expr := range allExprs {
					clauses = append(clauses, fmt.Sprintf("%s IS NULL", expr))
				}
				return "(" + strings.Join(clauses, " AND ") + ")", nil, nil
			}
		}
	}
	leftSQL, leftArgs, err := g.generateNode(node.Left)
	if err != nil {
		return "", nil, err
	}
	if negated {
		return fmt.Sprintf("%s IS NOT NULL", leftSQL), leftArgs, nil
	}
	return fmt.Sprintf("%s IS NULL", leftSQL), leftArgs, nil
}

func isRelativeInstantField(fieldName string) bool {
	switch strings.ToLower(fieldName) {
	case "created", "created_at", "createdat", "updated", "updated_at", "updatedat", "completed_at":
		return true
	default:
		return false
	}
}

func validateRelativeComparison(node *ASTNode) error {
	if node.Right == nil || node.Right.Type != NodeLiteral {
		return nil
	}
	if node.Right.DataType == NUMBER {
		if node.Left.Type == NodeIdentifier && isRelativeInstantField(node.Left.Value) {
			return fmt.Errorf("relative date literal requires a unit")
		}
		return nil
	}
	if node.Right.DataType != RelativeDate {
		return nil
	}
	if node.Left.Type != NodeIdentifier || !isRelativeInstantField(node.Left.Value) {
		return fmt.Errorf("relative date literal requires an instant field")
	}
	return nil
}

// generateBinaryOp generates SQL for binary operations (AND, OR, NOT)
func (g *SQLGenerator) generateBinaryOp(node *ASTNode) (sql string, args []any, err error) {
	switch strings.ToUpper(node.Operator) {
	case "AND":
		leftSQL, leftArgs, err := g.generateNode(node.Left)
		if err != nil {
			return "", nil, err
		}
		rightSQL, rightArgs, err := g.generateNode(node.Right)
		if err != nil {
			return "", nil, err
		}
		leftArgs = append(leftArgs, rightArgs...)
		return fmt.Sprintf("(%s AND %s)", leftSQL, rightSQL), leftArgs, nil

	case "OR":
		leftSQL, leftArgs, err := g.generateNode(node.Left)
		if err != nil {
			return "", nil, err
		}
		rightSQL, rightArgs, err := g.generateNode(node.Right)
		if err != nil {
			return "", nil, err
		}
		leftArgs = append(leftArgs, rightArgs...)
		return fmt.Sprintf("(%s OR %s)", leftSQL, rightSQL), leftArgs, nil

	case "NOT":
		rightSQL, rightArgs, err := g.generateNode(node.Right)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("NOT (%s)", rightSQL), rightArgs, nil

	default:
		return "", nil, fmt.Errorf("unsupported binary operator: %s", node.Operator)
	}
}

type fieldSemantics struct {
	caseInsensitive     bool
	bareIdentifierValue bool
	referenceNameField  string
	workspaceReference  bool
}

// fieldSemanticsFor keeps value handling consistent across comparison and IN.
func fieldSemanticsFor(entityType EntityType, fieldName string) fieldSemantics {
	fieldName = strings.ToLower(fieldName)
	if entityType == EntityTypeItem {
		switch fieldName {
		case "workspace":
			return fieldSemantics{workspaceReference: true}
		case "workspacekey":
			return fieldSemantics{caseInsensitive: true, bareIdentifierValue: true}
		case "itemtypename", "type":
			return fieldSemantics{caseInsensitive: true, bareIdentifierValue: true}
		case "project", "project_id", "projectid":
			return fieldSemantics{referenceNameField: "proj.name"}
		// milestone fields are handled by generateMilestoneComparison (M2M via
		// item_milestones); no name-substitution shortcut applies here.
		case "itemtype", "item_type_id", "itemtypeid":
			return fieldSemantics{caseInsensitive: true, bareIdentifierValue: true, referenceNameField: "it.name"}
		case "timeproject", "time_project_id", "timeprojectid":
			return fieldSemantics{referenceNameField: "tp.name"}
		case "iteration", "iteration_id", "iterationid":
			return fieldSemantics{referenceNameField: "iter.name"}
		}
	}

	switch fieldName {
	case "status", "priority", "type", "assettype", "asset_type", "category":
		return fieldSemantics{caseInsensitive: true, bareIdentifierValue: true}
	}
	return fieldSemantics{}
}

func fieldValue(node *ASTNode) (any, error) {
	if node == nil {
		return nil, errors.New("field comparison requires a value")
	}
	switch node.Type {
	case NodeLiteral:
		return node.Value, nil
	case NodeIdentifier:
		return node.Value, nil
	default:
		return nil, fmt.Errorf("field comparison requires a literal value, got %v", node.Type)
	}
}

func (g *SQLGenerator) generateWorkspaceComparison(node *ASTNode) (sql string, args []any, err error) {
	value, err := fieldValue(node.Right)
	if err != nil {
		return "", nil, err
	}
	prefix := g.aliasPrefix
	nameMatch := fmt.Sprintf("LOWER(%sw.name) = LOWER(?)", prefix)
	keyMatch := fmt.Sprintf("LOWER(%sw.key) = LOWER(?)", prefix)

	switch node.Operator {
	case "=":
		return fmt.Sprintf("(%s OR %s)", nameMatch, keyMatch), []any{value, value}, nil
	case "!=", "<>":
		return fmt.Sprintf("(NOT (%s) AND NOT (%s))", nameMatch, keyMatch), []any{value, value}, nil
	default:
		return "", nil, fmt.Errorf("operator %q is not supported on workspace references", node.Operator)
	}
}

func (g *SQLGenerator) generateWorkspaceInExpression(node *ASTNode) (sql string, args []any, err error) {
	if node.Values == nil || node.Values.Type != NodeList || len(node.Values.Arguments) == 0 {
		return "", nil, errors.New("workspace IN requires at least one value")
	}

	values := make([]any, 0, len(node.Values.Arguments))
	placeholders := make([]string, 0, len(node.Values.Arguments))
	for _, valueNode := range node.Values.Arguments {
		value, err := fieldValue(valueNode)
		if err != nil {
			return "", nil, err
		}
		values = append(values, value)
		placeholders = append(placeholders, "LOWER(?)")
	}

	prefix := g.aliasPrefix
	list := strings.Join(placeholders, ", ")
	nameMatch := fmt.Sprintf("LOWER(%sw.name) IN (%s)", prefix, list)
	keyMatch := fmt.Sprintf("LOWER(%sw.key) IN (%s)", prefix, list)
	args = append(args, values...)
	args = append(args, values...)
	if strings.EqualFold(node.Operator, "NOT IN") {
		return fmt.Sprintf("(NOT (%s) AND NOT (%s))", nameMatch, keyMatch), args, nil
	}
	return fmt.Sprintf("(%s OR %s)", nameMatch, keyMatch), args, nil
}

// generateLabelComparison matches global labels attached through item_labels.
func (g *SQLGenerator) generateLabelComparison(node *ASTNode) (sql string, args []any, err error) {
	prefix := g.aliasPrefix
	rightValue := node.Right.Value

	switch node.Operator {
	case "=":
		sql := fmt.Sprintf(`EXISTS (SELECT 1 FROM item_labels lbl_il JOIN labels lbl_l ON lbl_il.label_id = lbl_l.id WHERE lbl_il.item_id = %si.id AND LOWER(lbl_l.name) = LOWER(?))`, prefix)
		return sql, []any{rightValue}, nil
	case "!=", "<>":
		sql := fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM item_labels lbl_il JOIN labels lbl_l ON lbl_il.label_id = lbl_l.id WHERE lbl_il.item_id = %si.id AND LOWER(lbl_l.name) = LOWER(?))`, prefix)
		return sql, []any{rightValue}, nil
	case "~":
		sql := fmt.Sprintf(`EXISTS (SELECT 1 FROM item_labels lbl_il JOIN labels lbl_l ON lbl_il.label_id = lbl_l.id WHERE lbl_il.item_id = %si.id AND LOWER(lbl_l.name) LIKE '%%' || LOWER(?) || '%%')`, prefix)
		return sql, []any{rightValue}, nil
	default:
		return "", nil, fmt.Errorf("unsupported operator for label field: %s", node.Operator)
	}
}

// generateLabelInExpression uses EXISTS for label IN expressions.
func (g *SQLGenerator) generateLabelInExpression(node *ASTNode) (sql string, args []any, err error) {
	prefix := g.aliasPrefix

	if node.Values.Type != NodeList {
		return "", nil, errors.New("IN expression requires a list of values")
	}

	var placeholders []string
	for _, valueNode := range node.Values.Arguments {
		placeholders = append(placeholders, "LOWER(?)")
		args = append(args, g.convertLiteral(valueNode))
	}
	placeholderList := strings.Join(placeholders, ", ")

	if strings.EqualFold(node.Operator, "NOT IN") {
		sql = fmt.Sprintf(`NOT EXISTS (SELECT 1 FROM item_labels lbl_il JOIN labels lbl_l ON lbl_il.label_id = lbl_l.id WHERE lbl_il.item_id = %si.id AND LOWER(lbl_l.name) IN (%s))`, prefix, placeholderList)
		return sql, args, nil
	}

	sql = fmt.Sprintf(`EXISTS (SELECT 1 FROM item_labels lbl_il JOIN labels lbl_l ON lbl_il.label_id = lbl_l.id WHERE lbl_il.item_id = %si.id AND LOWER(lbl_l.name) IN (%s))`, prefix, placeholderList)
	return sql, args, nil
}

// isLabelField accepts the current `labels` and legacy `label` aliases.
func isLabelField(fieldName string) bool {
	switch strings.ToLower(fieldName) {
	case "label", "labels":
		return true
	}
	return false
}

// isMilestoneField identifies fields evaluated through item_milestones.
func isMilestoneField(fieldName string) bool {
	switch strings.ToLower(fieldName) {
	case "milestone", "milestone_id", "milestoneid", "milestonename":
		return true
	}
	return false
}

// generateMilestoneComparison matches any item_milestones row.
func (g *SQLGenerator) generateMilestoneComparison(node *ASTNode) (sql string, args []any, err error) {
	prefix := g.aliasPrefix
	byName := strings.EqualFold(node.Left.Value, "milestonename")

	// The right side may be a literal value or an identifier (e.g. when
	// milestone = "Q1 2024" is parsed with the right as an unquoted ident).
	var rightValue any
	switch node.Right.Type {
	case NodeLiteral:
		rightValue = g.convertLiteral(node.Right)
	case NodeIdentifier:
		rightValue = node.Right.Value
	default:
		return "", nil, fmt.Errorf("unsupported right-hand side for milestone comparison")
	}

	// Generic milestone strings match names; numbers match IDs.
	if !byName && strings.EqualFold(node.Left.Value, "milestone") {
		switch node.Right.Type {
		case NodeLiteral:
			byName = node.Right.DataType == STRING
		case NodeIdentifier:
			byName = true
		}
	}

	var matchExpr string
	if byName {
		matchExpr = "LOWER(ms.name) = LOWER(?)"
	} else {
		matchExpr = "ms_im.milestone_id = ?"
	}
	if node.Operator == "~" {
		// Substring match — only meaningful for name comparisons.
		if byName {
			return fmt.Sprintf(
				`EXISTS (SELECT 1 FROM item_milestones ms_im JOIN milestones ms ON ms.id = ms_im.milestone_id WHERE ms_im.item_id = %si.id AND LOWER(ms.name) LIKE '%%' || LOWER(?) || '%%')`,
				prefix,
			), []any{rightValue}, nil
		}
		// For id ~ value: fall through and treat like equality on the id.
	}

	switch node.Operator {
	case "=", "==", "~":
		if byName {
			return fmt.Sprintf(
				`EXISTS (SELECT 1 FROM item_milestones ms_im JOIN milestones ms ON ms.id = ms_im.milestone_id WHERE ms_im.item_id = %si.id AND %s)`,
				prefix, matchExpr,
			), []any{rightValue}, nil
		}
		return fmt.Sprintf(
			`EXISTS (SELECT 1 FROM item_milestones ms_im WHERE ms_im.item_id = %si.id AND %s)`,
			prefix, matchExpr,
		), []any{rightValue}, nil
	case "!=", "<>":
		if byName {
			return fmt.Sprintf(
				`NOT EXISTS (SELECT 1 FROM item_milestones ms_im JOIN milestones ms ON ms.id = ms_im.milestone_id WHERE ms_im.item_id = %si.id AND %s)`,
				prefix, matchExpr,
			), []any{rightValue}, nil
		}
		return fmt.Sprintf(
			`NOT EXISTS (SELECT 1 FROM item_milestones ms_im WHERE ms_im.item_id = %si.id AND %s)`,
			prefix, matchExpr,
		), []any{rightValue}, nil
	default:
		return "", nil, fmt.Errorf("unsupported operator %q for milestone field", node.Operator)
	}
}

// generateMilestoneInExpression generates SQL for milestone IN/NOT IN.
func (g *SQLGenerator) generateMilestoneInExpression(node *ASTNode) (sql string, args []any, err error) {
	prefix := g.aliasPrefix
	byName := strings.EqualFold(node.Field.Value, "milestonename")

	if node.Values.Type != NodeList {
		return "", nil, errors.New("IN expression requires a list of values")
	}

	// Generic milestone strings match names; mixed string/ID lists are rejected.
	if !byName && strings.EqualFold(node.Field.Value, "milestone") {
		allString, allNumeric := true, true
		for _, v := range node.Values.Arguments {
			if v.Type != NodeLiteral {
				allString, allNumeric = false, false
				break
			}
			if v.DataType != STRING {
				allString = false
			}
			if v.DataType != NUMBER {
				allNumeric = false
			}
		}
		switch {
		case allString && len(node.Values.Arguments) > 0:
			byName = true
		case !allNumeric && !allString && len(node.Values.Arguments) > 0:
			return "", nil, errors.New("milestone IN values must all be numeric IDs or all be string names")
		}
	}

	var placeholders []string
	for _, valueNode := range node.Values.Arguments {
		if byName {
			placeholders = append(placeholders, "LOWER(?)")
			args = append(args, g.convertLiteral(valueNode))
		} else {
			placeholders = append(placeholders, "?")
			args = append(args, g.convertLiteral(valueNode))
		}
	}
	placeholderList := strings.Join(placeholders, ", ")

	negate := strings.EqualFold(node.Operator, "NOT IN")
	existsKW := "EXISTS"
	if negate {
		existsKW = "NOT EXISTS"
	}

	if byName {
		return fmt.Sprintf(
			`%s (SELECT 1 FROM item_milestones ms_im JOIN milestones ms ON ms.id = ms_im.milestone_id WHERE ms_im.item_id = %si.id AND LOWER(ms.name) IN (%s))`,
			existsKW, prefix, placeholderList,
		), args, nil
	}
	return fmt.Sprintf(
		`%s (SELECT 1 FROM item_milestones ms_im WHERE ms_im.item_id = %si.id AND ms_im.milestone_id IN (%s))`,
		existsKW, prefix, placeholderList,
	), args, nil
}

// generateComparison generates SQL for comparison operations
func (g *SQLGenerator) generateComparison(node *ASTNode) (sql string, args []any, err error) {
	if err := validateRelativeComparison(node); err != nil {
		return "", nil, err
	}
	semantics := fieldSemantics{}
	if node.Left.Type == NodeIdentifier {
		semantics = fieldSemanticsFor(g.entityType, node.Left.Value)
		if semantics.workspaceReference {
			return g.generateWorkspaceComparison(node)
		}
	}

	// Labels use a many-to-many EXISTS query.
	if node.Left.Type == NodeIdentifier && isLabelField(node.Left.Value) {
		return g.generateLabelComparison(node)
	}

	// Milestones use the item_milestones junction.
	if node.Left.Type == NodeIdentifier && isMilestoneField(node.Left.Value) {
		return g.generateMilestoneComparison(node)
	}

	// Non-scalar custom fields need storage-shape-specific SQL.
	if node.Left.Type == NodeIdentifier {
		if info, ok := g.lookupCustomFieldInfo(node.Left.Value); ok {
			switch info.Kind {
			case CFKindReference:
				return g.generateReferenceCustomFieldComparison(node, info)
			case CFKindMultiselect:
				return g.generateMultiselectCustomFieldComparison(node, info)
			case CFKindLinking:
				return g.generateLinkingCustomFieldComparison(node, info)
			}
		}
	}

	leftSQL, leftArgs, err := g.generateNode(node.Left)
	if err != nil {
		return "", nil, err
	}

	var rightSQL string
	var rightArgs []any
	if semantics.bareIdentifierValue && node.Right.Type == NodeIdentifier {
		rightSQL = "?"
		rightArgs = []any{node.Right.Value}
	} else {
		rightSQL, rightArgs, err = g.generateNode(node.Right)
		if err != nil {
			return "", nil, err
		}
	}

	// Preserve left args for rewrites that duplicate the expression.
	leftOnlyArgs := append([]any(nil), leftArgs...)
	leftArgs = append(leftArgs, rightArgs...)

	// Name values compare against the corresponding reference name field.
	isReferenceFieldComparison := false
	if node.Left.Type == NodeIdentifier &&
		(node.Right.Type == NodeLiteral && node.Right.DataType == STRING ||
			node.Right.Type == NodeIdentifier && semantics.bareIdentifierValue) {
		if nameField := semantics.referenceNameField; nameField != "" {
			// Replace the ID field with the name field for name comparisons.
			leftSQL = nameField
			isReferenceFieldComparison = true
			// The name field substitution drops the original left expression,
			// so the left-only arg list (which described the JSON extract) is
			// no longer relevant for this branch.
			leftOnlyArgs = nil
		}
	}

	isCustomFieldComparison := false
	if !isReferenceFieldComparison && node.Left.Type == NodeIdentifier {
		fieldLower := strings.ToLower(node.Left.Value)
		if strings.HasPrefix(fieldLower, "cf_") || strings.HasPrefix(fieldLower, "custom.") {
			isCustomFieldComparison = true
		}
	}
	caseInsensitive := semantics.caseInsensitive &&
		(semantics.referenceNameField == "" || isReferenceFieldComparison)

	// Match Postgres date expression indexes; SQLite's string cast below matches
	// its equivalent index shape.
	if isCustomFieldComparison && g.dbDriver == "postgres" && node.Left.Type == NodeIdentifier {
		if info, ok := g.lookupCustomFieldInfo(node.Left.Value); ok && info.FieldType == "date" {
			leftSQL = fmt.Sprintf("CAST(%s AS TEXT)", leftSQL)
		}
	}

	// Custom field type casting for cross-type comparisons.
	// PostgreSQL ->> always returns text; SQLite ->> preserves JSON types
	// (string→TEXT, number→INTEGER). SQLite treats different storage classes as unequal,
	// so we CAST to normalize types for reliable comparisons.
	if isCustomFieldComparison && node.Right.Type == NodeLiteral {
		switch node.Right.DataType {
		case NUMBER:
			// CAST to NUMERIC for number comparisons on both databases
			leftSQL = fmt.Sprintf("CAST(%s AS NUMERIC)", leftSQL)
		case STRING:
			if g.dbDriver != "postgres" {
				// SQLite: CAST to TEXT so JSON number 20 matches string "20"
				leftSQL = fmt.Sprintf("CAST(%s AS TEXT)", leftSQL)
			}
		case BOOLEAN:
			// Postgres JSON booleans are text; SQLite values remain integer-compatible.
			if g.dbDriver == "postgres" && len(leftArgs) > len(leftOnlyArgs) {
				rightIdx := len(leftOnlyArgs)
				if v, ok := leftArgs[rightIdx].(int64); ok {
					if v == 1 {
						leftArgs[rightIdx] = "true"
					} else {
						leftArgs[rightIdx] = "false"
					}
				}
			}
		}
	}

	switch node.Operator {
	case "=":
		if caseInsensitive {
			// Make status, priority, type, category comparisons case-insensitive
			return fmt.Sprintf("LOWER(%s) = LOWER(%s)", leftSQL, rightSQL), leftArgs, nil
		}
		if isReferenceFieldComparison {
			// For reference field comparisons, add NULL check to exclude items without the field
			return fmt.Sprintf("(%s IS NOT NULL AND %s = %s)", leftSQL, leftSQL, rightSQL), leftArgs, nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), leftArgs, nil
	case "!=", "<>":
		if caseInsensitive {
			// Make status, priority, type, category comparisons case-insensitive
			return fmt.Sprintf("LOWER(%s) != LOWER(%s)", leftSQL, rightSQL), leftArgs, nil
		}
		if isReferenceFieldComparison {
			// For reference field comparisons, add NULL check to exclude items without the field
			return fmt.Sprintf("(%s IS NOT NULL AND %s != %s)", leftSQL, leftSQL, rightSQL), leftArgs, nil
		}
		// Standard SQL NULL semantics: NULL != X is NULL (filtered out), so
		// items without the custom field set don't match `cf_x != y`.
		return fmt.Sprintf("%s != %s", leftSQL, rightSQL), leftArgs, nil
	case "<":
		return fmt.Sprintf("%s < %s", leftSQL, rightSQL), leftArgs, nil
	case "<=":
		return fmt.Sprintf("%s <= %s", leftSQL, rightSQL), leftArgs, nil
	case ">":
		return fmt.Sprintf("%s > %s", leftSQL, rightSQL), leftArgs, nil
	case ">=":
		return fmt.Sprintf("%s >= %s", leftSQL, rightSQL), leftArgs, nil
	case "~":
		// Allow contains for built-in text fields (title, description, tag/asset_tag)
		// and for custom fields (cf_*, custom.*) — those are stored as JSON text
		// and LIKE works against the extracted value.
		isTextFieldComparison := false
		if node.Left.Type == NodeIdentifier {
			fieldName := strings.ToLower(node.Left.Value)
			switch fieldName {
			case "title", "description", "tag", "assettag", "asset_tag":
				isTextFieldComparison = true
			default:
				if strings.HasPrefix(fieldName, "cf_") || strings.HasPrefix(fieldName, "custom.") {
					isTextFieldComparison = true
				}
			}
		}

		if !isTextFieldComparison {
			return "", nil, fmt.Errorf("contains operator (~) can only be used with text fields (title, description, tag, custom fields)")
		}

		// Escape LIKE wildcards (% and _) in the user-provided pattern so a search
		// for a literal "50%" doesn't turn into "match anything starting with 50".
		// We swap the bound argument with one that has wildcards escaped, and add
		// ESCAPE '\' to the SQL fragment.
		escapedArgs := make([]any, len(leftArgs)-1, len(leftArgs))
		copy(escapedArgs, leftArgs[:len(leftArgs)-1])
		escapedArgs = append(escapedArgs, escapeLikePattern(leftArgs[len(leftArgs)-1]))

		if isReferenceFieldComparison {
			// For reference field comparisons, add NULL check to exclude items without the field
			return fmt.Sprintf("(%s IS NOT NULL AND %s LIKE %s ESCAPE '\\')", leftSQL, leftSQL, "'%' || ? || '%'"), escapedArgs, nil
		}
		return fmt.Sprintf("%s LIKE %s ESCAPE '\\'", leftSQL, "'%' || ? || '%'"), escapedArgs, nil
	default:
		return "", nil, fmt.Errorf("unsupported comparison operator: %s", node.Operator)
	}
}

// generateInExpression generates SQL for IN expressions
func (g *SQLGenerator) generateInExpression(node *ASTNode) (sql string, args []any, err error) {
	semantics := fieldSemantics{}
	if node.Field.Type == NodeIdentifier {
		semantics = fieldSemanticsFor(g.entityType, node.Field.Value)
		if semantics.workspaceReference {
			return g.generateWorkspaceInExpression(node)
		}
	}

	// Special handling for label field — uses EXISTS subqueries for many-to-many.
	// Accept both `label` (canonical) and `labels` (UI plural) as aliases.
	if node.Field.Type == NodeIdentifier && isLabelField(node.Field.Value) {
		return g.generateLabelInExpression(node)
	}

	// Milestones moved to a junction table — see comment in generateComparison.
	if node.Field.Type == NodeIdentifier && isMilestoneField(node.Field.Value) {
		return g.generateMilestoneInExpression(node)
	}

	// Per-kind dispatch for non-scalar custom fields. Same rationale as in
	// generateComparison: multiselect/reference/linking need shape-aware SQL.
	if node.Field.Type == NodeIdentifier {
		if info, ok := g.lookupCustomFieldInfo(node.Field.Value); ok {
			switch info.Kind {
			case CFKindMultiselect:
				return g.generateMultiselectCustomFieldInExpression(node, info)
			case CFKindLinking:
				return g.generateLinkingCustomFieldInExpression(node, info)
			case CFKindReference:
				return g.generateReferenceCustomFieldInExpression(node, info)
			}
		}
	}

	fieldSQL, fieldArgs, err := g.generateNode(node.Field)
	if err != nil {
		return "", nil, err
	}

	if node.Values.Type != NodeList {
		return "", nil, errors.New("IN expression requires a list of values")
	}

	// Smart reference field handling: use the name field when any value is a name.
	hasStringValue := false
	hasReferenceNameValue := false
	for _, valueNode := range node.Values.Arguments {
		if valueNode.DataType == STRING {
			hasStringValue = true
			hasReferenceNameValue = true
		}
		if valueNode.DataType == IDENTIFIER && semantics.referenceNameField != "" {
			hasReferenceNameValue = true
			break
		}
	}

	// If we have name values and this is a reference field, use the name field.
	isReferenceFieldIn := false
	if node.Field.Type == NodeIdentifier && hasReferenceNameValue {
		if nameField := semantics.referenceNameField; nameField != "" {
			// Replace the ID field with the name field for name comparisons.
			fieldSQL = nameField
			isReferenceFieldIn = true
		}
	}

	// User field IN with string values: resolve group names to member user IDs via subquery
	isUserFieldIn := false
	if node.Field.Type == NodeIdentifier && hasStringValue {
		fn := strings.ToLower(node.Field.Value)
		if fn == "assignee" || fn == "assignee_id" || fn == "assigneeid" ||
			fn == "creator" || fn == "creator_id" || fn == "creatorid" ||
			fn == "reporter" || fn == "reporter_id" || fn == "reporterid" {
			isUserFieldIn = true
		}
	}

	if isUserFieldIn {
		var placeholders []string
		var valueArgs []any
		args = append(args, fieldArgs...)
		for _, valueNode := range node.Values.Arguments {
			placeholders = append(placeholders, "LOWER(?)")
			valueArgs = append(valueArgs, g.convertLiteral(valueNode))
		}
		placeholderList := strings.Join(placeholders, ", ")

		// Append value args 3x: group name match, username match, email match
		args = append(args, valueArgs...)
		args = append(args, valueArgs...)
		args = append(args, valueArgs...)

		groupSubquery := fmt.Sprintf(
			"SELECT gm.user_id FROM group_members gm JOIN groups g ON gm.group_id = g.id WHERE LOWER(g.name) IN (%s)",
			placeholderList,
		)
		userSubquery := fmt.Sprintf(
			"SELECT u.id FROM users u WHERE LOWER(u.username) IN (%s) OR LOWER(u.email) IN (%s)",
			placeholderList, placeholderList,
		)
		subquery := groupSubquery + " UNION " + userSubquery

		if strings.EqualFold(node.Operator, "NOT IN") {
			return fmt.Sprintf("(%s IS NOT NULL AND %s NOT IN (%s))", fieldSQL, fieldSQL, subquery), args, nil
		}
		return fmt.Sprintf("(%s IS NOT NULL AND %s IN (%s))", fieldSQL, fieldSQL, subquery), args, nil
	}

	// Custom field NUMERIC cast for IN expressions with number values.
	// Both PostgreSQL ->> and SQLite ->> return TEXT, so CAST for numeric comparisons.
	if node.Field.Type == NodeIdentifier {
		fieldLower := strings.ToLower(node.Field.Value)
		if strings.HasPrefix(fieldLower, "cf_") || strings.HasPrefix(fieldLower, "custom.") {
			if len(node.Values.Arguments) > 0 && node.Values.Arguments[0].DataType == NUMBER {
				fieldSQL = fmt.Sprintf("CAST(%s AS NUMERIC)", fieldSQL)
			}
		}
	}

	var placeholders []string
	args = append(args, fieldArgs...)

	caseInsensitive := semantics.caseInsensitive &&
		(semantics.referenceNameField == "" || isReferenceFieldIn)
	for _, valueNode := range node.Values.Arguments {
		if caseInsensitive {
			placeholders = append(placeholders, "LOWER(?)")
		} else {
			placeholders = append(placeholders, "?")
		}
		args = append(args, g.convertLiteral(valueNode))
	}

	placeholderList := strings.Join(placeholders, ", ")

	if caseInsensitive {
		// Make status, priority, type, category IN comparisons case-insensitive
		if strings.EqualFold(node.Operator, "NOT IN") {
			return fmt.Sprintf("LOWER(%s) NOT IN (%s)", fieldSQL, placeholderList), args, nil
		}
		return fmt.Sprintf("LOWER(%s) IN (%s)", fieldSQL, placeholderList), args, nil
	}

	if isReferenceFieldIn {
		// For reference field IN comparisons, add NULL check to exclude items without the field
		if strings.EqualFold(node.Operator, "NOT IN") {
			return fmt.Sprintf("(%s IS NOT NULL AND %s NOT IN (%s))", fieldSQL, fieldSQL, placeholderList), args, nil
		}
		return fmt.Sprintf("(%s IS NOT NULL AND %s IN (%s))", fieldSQL, fieldSQL, placeholderList), args, nil
	}

	if strings.EqualFold(node.Operator, "NOT IN") {
		return fmt.Sprintf("%s NOT IN (%s)", fieldSQL, placeholderList), args, nil
	}
	return fmt.Sprintf("%s IN (%s)", fieldSQL, placeholderList), args, nil
}

// extractStringLiteral extracts a string value from an AST node
// Returns the string value and an error if the node is not a string literal
func extractStringLiteral(node *ASTNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("argument is nil")
	}
	if node.Type != NodeLiteral {
		return "", fmt.Errorf("argument must be a string literal, got %v", node.Type)
	}
	if node.DataType != STRING {
		return "", fmt.Errorf("argument must be a string, got %v", node.DataType)
	}
	return node.Value, nil
}

// generateFunction generates SQL for function calls
func (g *SQLGenerator) generateFunction(node *ASTNode) (sql string, args []any, err error) {
	switch strings.ToLower(node.Value) {
	case "currentuser":
		// This would need to be filled in with actual user context
		return "?", []any{"current-user-id"}, nil
	case "currentcustomer":
		// Portal customer ID - resolved at handler level before CQL parsing
		return "?", []any{"current-customer-id"}, nil
	case "currentorganisation":
		// Customer organization ID - resolved at handler level before CQL parsing
		return "?", []any{"current-organisation-id"}, nil //nolint:misspell // CQL function name uses British spelling
	case "now":
		return "?", []any{g.evaluationTime}, nil
	case "startofday":
		now := g.evaluationTime
		return "?", []any{time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)}, nil
	case "endofday":
		now := g.evaluationTime
		nextDay := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		return "?", []any{nextDay.Add(-time.Nanosecond)}, nil

	case "childrenof":
		// childrenOf("ql query") - Find all descendants of items matching the inner query
		if len(node.Arguments) != 1 {
			return "", nil, fmt.Errorf("childrenOf() requires exactly 1 argument (QL query string)")
		}

		innerQL, err := extractStringLiteral(node.Arguments[0])
		if err != nil {
			return "", nil, fmt.Errorf("childrenOf() argument error: %w", err)
		}

		// Parse and generate SQL for the inner QL query
		innerTokenizer := NewTokenizer(innerQL)
		innerTokens, err := innerTokenizer.Tokenize()
		if err != nil {
			return "", nil, fmt.Errorf("childrenOf() inner query tokenization error: %w", err)
		}

		innerParser := NewParser(innerTokens)
		innerAST, err := innerParser.Parse()
		if err != nil {
			return "", nil, fmt.Errorf("childrenOf() inner query parse error: %w", err)
		}

		innerGenerator := g.innerGenerator(g.customFieldMap)
		innerSQL, innerArgs, err := innerGenerator.GenerateSQLAt(innerAST, g.evaluationTime)
		if err != nil {
			return "", nil, fmt.Errorf("childrenOf() inner query SQL generation error: %w", err)
		}

		// Generate recursive CTE to find all descendants (children only, not the parents)
		// Base case: find direct children of items matching the inner query
		// Recursive case: find children of those children
		// Note: Uses inner_ prefix for all table aliases to avoid collision with outer query's aliases
		sql := fmt.Sprintf(`%si.id IN (
			WITH RECURSIVE descendants AS (
				-- Base case: direct children of items matching the inner query
				SELECT child.id FROM items child
				WHERE child.parent_id IN (
					SELECT inner_i.id FROM items inner_i
					LEFT JOIN workspaces inner_w ON inner_i.workspace_id = inner_w.id
					LEFT JOIN item_types inner_it ON inner_i.item_type_id = inner_it.id
					LEFT JOIN items inner_p ON inner_i.parent_id = inner_p.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN status_categories inner_sc ON inner_st.category_id = inner_sc.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				)
				UNION ALL
				-- Recursive case: children of descendants
				SELECT rec_i.id FROM items rec_i
				JOIN descendants d ON rec_i.parent_id = d.id
			)
			SELECT id FROM descendants
		)`, g.aliasPrefix, innerSQL)

		return sql, innerArgs, nil

	case "linkedof":
		// Dispatch based on entity type
		if g.entityType == EntityTypeAsset {
			return g.generateAssetLinkedOf(node)
		}
		return g.generateItemLinkedOf(node)

	default:
		return "", nil, fmt.Errorf("unsupported function: %s", node.Value)
	}
}

// generateItemLinkedOf generates SQL for finding items linked to other items matching a query
func (g *SQLGenerator) generateItemLinkedOf(node *ASTNode) (sql string, args []any, err error) {
	if len(node.Arguments) != 2 {
		return "", nil, fmt.Errorf("linkedOf() requires exactly 2 arguments (link label and QL query string)")
	}

	linkLabel, err := extractStringLiteral(node.Arguments[0])
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() first argument (link label) error: %w", err)
	}

	innerQL, err := extractStringLiteral(node.Arguments[1])
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() second argument (QL query) error: %w", err)
	}

	// Parse and generate SQL for the inner QL query
	innerTokenizer := NewTokenizer(innerQL)
	innerTokens, err := innerTokenizer.Tokenize()
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() inner query tokenization error: %w", err)
	}

	innerParser := NewParser(innerTokens)
	innerAST, err := innerParser.Parse()
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() inner query parse error: %w", err)
	}

	innerGenerator := g.innerGenerator(g.customFieldMap)
	innerSQL, innerArgs, err := innerGenerator.GenerateSQLAt(innerAST, g.evaluationTime)
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() inner query SQL generation error: %w", err)
	}

	// Generate SQL that:
	// 1. Finds the link type by matching the label against forward_label or reverse_label
	// 2. If forward_label matches: return target items (source -> target direction)
	// 3. If reverse_label matches: return source items (target <- source direction)
	sql = fmt.Sprintf(`%si.id IN (
		SELECT CASE
			WHEN lt.forward_label = ? THEN il.target_id
			WHEN lt.reverse_label = ? THEN il.source_id
		END AS linked_item_id
		FROM item_links il
		JOIN link_types lt ON il.link_type_id = lt.id
		WHERE (lt.forward_label = ? OR lt.reverse_label = ?)
			AND il.source_type = 'item'
			AND il.target_type = 'item'
			AND (
				(lt.forward_label = ? AND il.source_id IN (
					SELECT inner_i.id FROM items inner_i
					LEFT JOIN workspaces inner_w ON inner_i.workspace_id = inner_w.id
					LEFT JOIN item_types inner_it ON inner_i.item_type_id = inner_it.id
					LEFT JOIN items inner_p ON inner_i.parent_id = inner_p.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN status_categories inner_sc ON inner_st.category_id = inner_sc.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				))
				OR
				(lt.reverse_label = ? AND il.target_id IN (
					SELECT inner_i.id FROM items inner_i
					LEFT JOIN workspaces inner_w ON inner_i.workspace_id = inner_w.id
					LEFT JOIN item_types inner_it ON inner_i.item_type_id = inner_it.id
					LEFT JOIN items inner_p ON inner_i.parent_id = inner_p.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN status_categories inner_sc ON inner_st.category_id = inner_sc.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				))
			)
	)`, g.aliasPrefix, innerSQL, innerSQL)

	args = make([]any, 0, 6+2*len(innerArgs))
	args = append(args, linkLabel, linkLabel, linkLabel, linkLabel, linkLabel)
	args = append(args, innerArgs...) // First occurrence of inner query
	args = append(args, linkLabel)    // One more label for reverse check
	args = append(args, innerArgs...) // Second occurrence of inner query

	return sql, args, nil
}

// generateAssetLinkedOf generates SQL for finding assets linked to items matching a query
func (g *SQLGenerator) generateAssetLinkedOf(node *ASTNode) (sql string, args []any, err error) {
	if len(node.Arguments) != 2 {
		return "", nil, fmt.Errorf("linkedOf() requires exactly 2 arguments (link label and QL query string)")
	}

	linkLabel, err := extractStringLiteral(node.Arguments[0])
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() first argument (link label) error: %w", err)
	}

	innerQL, err := extractStringLiteral(node.Arguments[1])
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() second argument (QL query) error: %w", err)
	}

	innerTokenizer := NewTokenizer(innerQL)
	innerTokens, err := innerTokenizer.Tokenize()
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() inner query tokenization error: %w", err)
	}

	innerParser := NewParser(innerTokens)
	innerAST, err := innerParser.Parse()
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() inner query parse error: %w", err)
	}

	// itemCustomFieldMap is the item-side custom-field map supplied by the
	// asset evaluator's caller — required for cf_<name> inside linkedOf() to
	// resolve to the numeric JSON key used in items.custom_field_values.
	innerGenerator := g.innerGenerator(g.itemCustomFieldMap)
	innerSQL, innerArgs, err := innerGenerator.GenerateSQLAt(innerAST, g.evaluationTime)
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() inner query SQL generation error: %w", err)
	}

	// Generate SQL to find assets linked to items matching the inner query
	// Assets can be linked to items via item_links table where:
	// - source_type='asset' and target_type='item' (asset links to item)
	// - source_type='item' and target_type='asset' (item links to asset)
	sql = fmt.Sprintf(`a.id IN (
		SELECT CASE
			WHEN il.source_type = 'asset' THEN il.source_id
			WHEN il.target_type = 'asset' THEN il.target_id
		END AS linked_asset_id
		FROM item_links il
		JOIN link_types lt ON il.link_type_id = lt.id
		WHERE (lt.forward_label = ? OR lt.reverse_label = ?)
			AND (
				(il.source_type = 'asset' AND il.target_type = 'item' AND il.target_id IN (
					SELECT inner_i.id FROM items inner_i
					LEFT JOIN workspaces inner_w ON inner_i.workspace_id = inner_w.id
					LEFT JOIN item_types inner_it ON inner_i.item_type_id = inner_it.id
					LEFT JOIN items inner_p ON inner_i.parent_id = inner_p.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN status_categories inner_sc ON inner_st.category_id = inner_sc.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				))
				OR
				(il.target_type = 'asset' AND il.source_type = 'item' AND il.source_id IN (
					SELECT inner_i.id FROM items inner_i
					LEFT JOIN workspaces inner_w ON inner_i.workspace_id = inner_w.id
					LEFT JOIN item_types inner_it ON inner_i.item_type_id = inner_it.id
					LEFT JOIN items inner_p ON inner_i.parent_id = inner_p.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN status_categories inner_sc ON inner_st.category_id = inner_sc.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				))
			)
	)`, innerSQL, innerSQL)

	args = make([]any, 0, 2+2*len(innerArgs))
	args = append(args, linkLabel, linkLabel)
	args = append(args, innerArgs...) // First occurrence of inner query
	args = append(args, innerArgs...) // Second occurrence of inner query

	return sql, args, nil
}

// validCustomFieldNameRegex validates that a custom field name contains only
// safe characters for inline use in JSON paths (the no-map fallback path
// interpolates the name into SQL). Allows leading digits — admins are free to
// create fields named like "123 Score" and they should still be queryable.
// JSON-path-breaking characters (quotes, backslashes) remain rejected.
var validCustomFieldNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_ -]*$`)

// mapFieldName maps QL field names to SQL column names
// Dispatches to entity-specific mapping based on entityType
func (g *SQLGenerator) mapFieldName(fieldName string) (expr string, args []any, err error) {
	if g.entityType == EntityTypeAsset {
		return g.mapAssetFieldName(fieldName)
	}
	return g.mapItemFieldName(fieldName)
}

// extractCustomFieldScalar produces the SQL expression to extract a scalar
// custom-field value from the given JSON column. When the name resolves through
// customFieldMap, the numeric ID is inlined into the SQL (lets the Postgres
// planner match the per-field expression indexes from handlers/custom_fields.go).
// When the name does not resolve, the generator falls back to parameterized
// name-based extraction (preserves legacy behavior for callers without a map).
func (g *SQLGenerator) extractCustomFieldScalar(column, customFieldName string) (sql string, args []any) {
	if g.customFieldMap != nil {
		if info, ok := g.customFieldMap[strings.ToLower(customFieldName)]; ok {
			idSQL := g.jsonExtractLiteralKey(column, info.ID)
			if !g.legacyNameKeyFallback {
				return idSQL, nil
			}
			nameSQL, nameArgs := g.jsonExtract(column, customFieldName)
			// Prefer the canonical numeric-key storage used by current writes, but
			// keep the legacy name-key fallback so older rows (and API clients that
			// still submit name-keyed custom_field_values) remain queryable via
			// cf_<name>/custom.<name>.
			return fmt.Sprintf("COALESCE(%s, %s)", idSQL, nameSQL), nameArgs
		}
	}
	return g.jsonExtract(column, customFieldName)
}

// extractItemCustomFieldScalar is the item-table-scoped wrapper of extractCustomFieldScalar.
func (g *SQLGenerator) extractItemCustomFieldScalar(prefix, customFieldName string) (sql string, args []any) {
	return g.extractCustomFieldScalar(prefix+"i.custom_field_values", customFieldName)
}

// customFieldNameFromIdentifier extracts the field name from a `cf_<name>` or
// `custom.<name>` identifier value. Returns the trimmed name and true if the
// identifier matches one of those custom-field prefixes.
func customFieldNameFromIdentifier(identifier string) (string, bool) {
	lower := strings.ToLower(identifier)
	if strings.HasPrefix(lower, "cf_") {
		return identifier[3:], true
	}
	if strings.HasPrefix(lower, "custom.") {
		return identifier[7:], true
	}
	return "", false
}

// customFieldIDFromIdentifier extracts the numeric ID from a `cfid_<id>`
// identifier. This is the stable, collision-free form: the UI (or a user)
// can address a custom field by its DB id directly, bypassing name lookup
// entirely. Returns the id and true when the identifier is a valid cfid_*
// reference.
func customFieldIDFromIdentifier(identifier string) (int, bool) {
	lower := strings.ToLower(identifier)
	if !strings.HasPrefix(lower, "cfid_") {
		return 0, false
	}
	rest := identifier[5:]
	if rest == "" {
		return 0, false
	}
	id, err := strconv.Atoi(rest)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// lookupCustomFieldByID returns the field kind for a numeric id, reading from
// the custom-field map when one is supplied. When no map is available, falls
// back to CFKindScalar — safe default that preserves legacy behavior.
func (g *SQLGenerator) lookupCustomFieldByID(id int) CustomFieldInfo {
	if g.customFieldMap != nil {
		for _, info := range g.customFieldMap {
			if info.ID == id {
				return info
			}
		}
	}
	return CustomFieldInfo{ID: id, Kind: CFKindScalar}
}

// lookupCustomFieldInfo resolves a cf_/custom./cfid_ identifier to its info.
// Returns the field info and true when the identifier is a custom-field
// reference resolvable to a numeric ID — either via the customFieldMap (for
// name-based forms) or directly (for the cfid_<id> form).
func (g *SQLGenerator) lookupCustomFieldInfo(identifier string) (CustomFieldInfo, bool) {
	if id, ok := customFieldIDFromIdentifier(identifier); ok {
		return g.lookupCustomFieldByID(id), true
	}
	name, ok := customFieldNameFromIdentifier(identifier)
	if !ok {
		return CustomFieldInfo{}, false
	}
	if !validCustomFieldNameRegex.MatchString(name) {
		return CustomFieldInfo{}, false
	}
	if g.customFieldMap == nil {
		return CustomFieldInfo{}, false
	}
	info, ok := g.customFieldMap[strings.ToLower(name)]
	if !ok {
		return CustomFieldInfo{}, false
	}
	info.LegacyName = name
	return info, true
}

// itemCustomFieldColumn returns the JSON column expression for the active entity.
// Items: i.custom_field_values; assets: a.custom_field_values, with aliasPrefix
// applied for nested queries.
func (g *SQLGenerator) customFieldColumn() string {
	if g.entityType == EntityTypeAsset {
		return g.aliasPrefix + "a.custom_field_values"
	}
	return g.aliasPrefix + "i.custom_field_values"
}

// generateReferenceCustomFieldComparison emits SQL for a comparison on a
// reference-kind custom field (user, asset, portalcustomer, customerorganisation).
// Reference values may be stored as a direct scalar ID or as an object
// {id, name, ...}. The same pattern is already used by the asset link graph in
// internal/handlers/asset_link_handlers.go for graph traversal.
//
// For =/!=, both forms are compared against the same RHS.
// For ~, only the direct form is matched with LIKE (object JSON LIKE is misleading).
func (g *SQLGenerator) generateReferenceCustomFieldComparison(node *ASTNode, info CustomFieldInfo) (sql string, args []any, err error) {
	rightSQL, rightArgs, err := g.generateNode(node.Right)
	if err != nil {
		return "", nil, err
	}
	column := g.customFieldColumn()
	directExprs, nestedExprs := g.referenceExtractExpressionsForInfo(column, info)
	allExprs := append(append([]string{}, nestedExprs...), directExprs...)

	switch node.Operator {
	case "=":
		clauses := make([]string, 0, len(allExprs))
		args := make([]any, 0, len(allExprs)*len(rightArgs))
		for _, expr := range allExprs {
			clauses = append(clauses, fmt.Sprintf("%s = %s", expr, rightSQL))
			args = append(args, rightArgs...)
		}
		return "(" + strings.Join(clauses, " OR ") + ")", args, nil
	case "!=", "<>":
		// Pick the most specific available form (nested.id if it's an object,
		// direct scalar otherwise, with legacy name-keyed storage as fallback)
		// and compare. The naive (direct != ? AND nested != ?) drops rows where
		// nested is NULL: `true AND NULL` is NULL, filtered out. COALESCE
		// collapses the dual/legacy storage into a single effective ID and
		// preserves "missing field doesn't match !=" semantics for free.
		return fmt.Sprintf("COALESCE(%s) != %s", strings.Join(allExprs, ", "), rightSQL), rightArgs, nil
	case "~":
		// LIKE on object JSON is misleading; restrict to the direct scalar form.
		if len(rightArgs) != 1 {
			return "", nil, errors.New("~ on reference custom field requires a single string value")
		}
		escaped := escapeLikePattern(rightArgs[0])
		return fmt.Sprintf("(COALESCE(%s) LIKE '%%' || ? || '%%' ESCAPE '\\')", strings.Join(directExprs, ", ")), []any{escaped}, nil
	default:
		return "", nil, fmt.Errorf("operator %q is not supported on reference custom fields", node.Operator)
	}
}

// generateReferenceCustomFieldInExpression handles `cf_x IN (...)` and
// `cf_x NOT IN (...)` for reference kind. Like the equality path, this checks
// both the direct scalar and the nested .id so legacy scalar storage and
// object-backed storage both match.
func (g *SQLGenerator) generateReferenceCustomFieldInExpression(node *ASTNode, info CustomFieldInfo) (sql string, args []any, err error) {
	if node.Values.Type != NodeList {
		return "", nil, errors.New("IN expression requires a list of values")
	}
	placeholders := make([]string, 0, len(node.Values.Arguments))
	values := make([]any, 0, len(node.Values.Arguments))
	for _, v := range node.Values.Arguments {
		placeholders = append(placeholders, "?")
		values = append(values, g.convertLiteral(v))
	}
	placeholderList := strings.Join(placeholders, ", ")
	column := g.customFieldColumn()
	directExprs, nestedExprs := g.referenceExtractExpressionsForInfo(column, info)
	allExprs := append(append([]string{}, nestedExprs...), directExprs...)

	if strings.EqualFold(node.Operator, "NOT IN") {
		// COALESCE collapses dual storage (scalar 7 vs {"id":7,...}) plus
		// legacy name-keyed storage into one effective ID, avoiding the
		// `true AND NULL = NULL` row-drop trap of the naive
		// (direct NOT IN ... AND nested NOT IN ...) form.
		return fmt.Sprintf("COALESCE(%s) NOT IN (%s)", strings.Join(allExprs, ", "), placeholderList), values, nil
	}
	// For IN keep the OR form: matches if ANY storage form is in the list.
	// IN doesn't suffer the NULL-and-true bug, and OR lets legacy scalar,
	// numeric-keyed scalar, and object-backed rows match.
	clauses := make([]string, 0, len(allExprs))
	args = make([]any, 0, len(values)*len(allExprs))
	for _, expr := range allExprs {
		clauses = append(clauses, fmt.Sprintf("%s IN (%s)", expr, placeholderList))
		args = append(args, values...)
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args, nil
}

// referenceExtractExpressions returns the SQL expressions to read the direct
// scalar value and the nested .id from a reference custom-field JSON value.
func (g *SQLGenerator) referenceExtractExpressions(column string, fieldID int) (direct, nested string) {
	return g.referenceExtractExpressionsForKey(column, fmt.Sprintf("%d", fieldID))
}

func (g *SQLGenerator) referenceExtractExpressionsForInfo(column string, info CustomFieldInfo) (directExprs, nestedExprs []string) {
	direct, nested := g.referenceExtractExpressions(column, info.ID)
	directExprs = append(directExprs, direct)
	nestedExprs = append(nestedExprs, nested)
	if legacyNameKeyFallbackEnabled(g, info) {
		legacyDirect, legacyNested := g.referenceExtractExpressionsForKey(column, info.LegacyName)
		directExprs = append(directExprs, legacyDirect)
		nestedExprs = append(nestedExprs, legacyNested)
	}
	return directExprs, nestedExprs
}

func (g *SQLGenerator) referenceExtractExpressionsForKey(column, key string) (direct, nested string) {
	key = sqlStringLiteralKey(key)
	if g.dbDriver == "postgres" {
		direct = fmt.Sprintf("%s->>'%s'", column, key)
		nested = fmt.Sprintf("%s->'%s'->>'id'", column, key)
		return direct, nested
	}
	direct = fmt.Sprintf(`NULLIF(%s, '') ->> '$.%q'`, column, key)
	nested = fmt.Sprintf(`NULLIF(%s, '') ->> '$.%q.id'`, column, key)
	return direct, nested
}

func legacyNameKeyFallbackEnabled(g *SQLGenerator, info CustomFieldInfo) bool {
	return g.legacyNameKeyFallback && info.LegacyName != "" && validCustomFieldNameRegex.MatchString(info.LegacyName)
}

func sqlStringLiteralKey(key string) string {
	return strings.ReplaceAll(key, `'`, `''`)
}

// multiselectExistsExpression returns an EXISTS subquery that yields a row when
// the multiselect custom-field array contains ANY of the placeholder-bound
// values. SQLite uses json_each over the JSON path; Postgres uses
// jsonb_array_elements_text. Values are compared as TEXT so integer option IDs
// (the common case) and string option IDs both round-trip.
func (g *SQLGenerator) multiselectExistsExpression(column string, info CustomFieldInfo, placeholders []string) string {
	exprs := []string{g.multiselectExistsExpressionForKey(column, fmt.Sprintf("%d", info.ID), placeholders)}
	if legacyNameKeyFallbackEnabled(g, info) {
		exprs = append(exprs, g.multiselectExistsExpressionForKey(column, info.LegacyName, placeholders))
	}
	if len(exprs) == 1 {
		return exprs[0]
	}
	return "(" + strings.Join(exprs, " OR ") + ")"
}

func (g *SQLGenerator) multiselectAnyValueExpression(column string, info CustomFieldInfo) string {
	exprs := []string{g.multiselectAnyValueExpressionForKey(column, fmt.Sprintf("%d", info.ID))}
	if legacyNameKeyFallbackEnabled(g, info) {
		exprs = append(exprs, g.multiselectAnyValueExpressionForKey(column, info.LegacyName))
	}
	if len(exprs) == 1 {
		return exprs[0]
	}
	return "(" + strings.Join(exprs, " OR ") + ")"
}

func multiselectStorageKeyCount(g *SQLGenerator, info CustomFieldInfo) int {
	if legacyNameKeyFallbackEnabled(g, info) {
		return 2
	}
	return 1
}

func repeatArgs(args []any, copies int) []any {
	if copies <= 1 || len(args) == 0 {
		return args
	}
	out := make([]any, 0, len(args)*copies)
	for range copies {
		out = append(out, args...)
	}
	return out
}

func (g *SQLGenerator) multiselectExistsExpressionForKey(column, key string, placeholders []string) string {
	key = sqlStringLiteralKey(key)
	values := strings.Join(placeholders, ", ")
	if g.dbDriver == "postgres" {
		return fmt.Sprintf(
			"EXISTS (SELECT 1 FROM jsonb_array_elements_text(%s->'%s') v WHERE v IN (%s))",
			column, key, values,
		)
	}
	return fmt.Sprintf(
		`EXISTS (SELECT 1 FROM json_each(%s, '$.%q') WHERE CAST(value AS TEXT) IN (%s))`,
		column, key, values,
	)
}

func (g *SQLGenerator) multiselectAnyValueExpressionForKey(column, key string) string {
	key = sqlStringLiteralKey(key)
	if g.dbDriver == "postgres" {
		return fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements_text(%s->'%s'))", column, key)
	}
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM json_each(%s, '$.%q'))`, column, key)
}

// generateMultiselectCustomFieldComparison handles =/!=/~/IN/NOT IN on a
// multiselect custom field. Values are arrays of option IDs; semantics are
// "contains any" (=/~/IN) or "contains none" (!=/NOT IN).
func (g *SQLGenerator) generateMultiselectCustomFieldComparison(node *ASTNode, info CustomFieldInfo) (sql string, args []any, err error) {
	_, rightArgs, err := g.generateNode(node.Right)
	if err != nil {
		return "", nil, err
	}
	if len(rightArgs) == 0 {
		return "", nil, errors.New("multiselect custom field comparison requires a bound value")
	}
	textArgs := make([]any, len(rightArgs))
	for i, a := range rightArgs {
		textArgs[i] = multiselectValueAsText(a)
	}
	column := g.customFieldColumn()
	expr := g.multiselectExistsExpression(column, info, []string{"?"})
	storageArgs := repeatArgs(textArgs, multiselectStorageKeyCount(g, info))

	switch node.Operator {
	case "=", "~":
		return expr, storageArgs, nil
	case "!=", "<>":
		return "NOT " + expr, storageArgs, nil
	default:
		return "", nil, fmt.Errorf("operator %q is not supported on multiselect custom fields", node.Operator)
	}
}

// generateMultiselectCustomFieldInExpression handles `cf_x IN (...)` and
// `cf_x NOT IN (...)` for multiselect kind. "Contains any of" semantics.
func (g *SQLGenerator) generateMultiselectCustomFieldInExpression(node *ASTNode, info CustomFieldInfo) (sql string, args []any, err error) {
	if node.Values.Type != NodeList {
		return "", nil, errors.New("IN expression requires a list of values")
	}
	placeholders := make([]string, 0, len(node.Values.Arguments))
	args = make([]any, 0, len(node.Values.Arguments))
	for _, v := range node.Values.Arguments {
		placeholders = append(placeholders, "?")
		args = append(args, multiselectValueAsText(g.convertLiteral(v)))
	}
	column := g.customFieldColumn()
	expr := g.multiselectExistsExpression(column, info, placeholders)
	storageArgs := repeatArgs(args, multiselectStorageKeyCount(g, info))
	if strings.EqualFold(node.Operator, "NOT IN") {
		return "NOT " + expr, storageArgs, nil
	}
	return expr, storageArgs, nil
}

// linkingSubquery builds the EXISTS-subquery body for linking custom fields.
// Handles both primary fields (link rows store custom_field_id = info.ID) and
// mirror fields (link rows live under MirrorOfFieldID with source/target
// swapped). AllowedTargetTypes, when present, constrains the type column on
// the opposite side to prevent cross-entity target_id collisions.
func (g *SQLGenerator) linkingSubquery(info CustomFieldInfo, valuePlaceholders string) string {
	currentItemID := g.aliasPrefix + "i.id"
	customFieldID := info.ID
	if info.MirrorOfFieldID > 0 {
		customFieldID = info.MirrorOfFieldID
	}
	// Primary: current item is the SOURCE; user-supplied value matches target_id.
	// Mirror: current item is the TARGET; user-supplied value matches source_id.
	// The non-current side gets the type constraint when AllowedTargetTypes is set.
	currentSide, otherSide, currentType, otherType := "source_id", "target_id", "source_type", "target_type"
	if info.MirrorOfFieldID > 0 {
		currentSide, otherSide = otherSide, currentSide
		currentType, otherType = otherType, currentType
	}
	clauses := []string{
		fmt.Sprintf("il.%s = 'item'", currentType),
		fmt.Sprintf("il.%s = %s", currentSide, currentItemID),
		fmt.Sprintf("il.custom_field_id = %d", customFieldID),
		fmt.Sprintf("il.%s IN (%s)", otherSide, valuePlaceholders),
	}
	if typeFilter := linkingTargetTypeFilter(otherType, info.AllowedTargetTypes); typeFilter != "" {
		clauses = append(clauses, typeFilter)
	}
	return "EXISTS (SELECT 1 FROM item_links il WHERE " + strings.Join(clauses, " AND ") + ")"
}

// linkingTargetTypeFilter returns a SQL clause constraining the link's type
// column to the allowed set, or "" when no constraint should be applied.
// Quoting is safe because the allowed values come from a vetted JSON option
// list, but defensively we still filter to alphanumeric values.
func linkingTargetTypeFilter(column string, allowed []string) string {
	if len(allowed) == 0 {
		return ""
	}
	safe := make([]string, 0, len(allowed))
	for _, t := range allowed {
		t = strings.TrimSpace(t)
		if t == "" || !isSafeEntityType(t) {
			continue
		}
		safe = append(safe, "'"+t+"'")
	}
	if len(safe) == 0 {
		return ""
	}
	if len(safe) == 1 {
		return fmt.Sprintf("il.%s = %s", column, safe[0])
	}
	return fmt.Sprintf("il.%s IN (%s)", column, strings.Join(safe, ", "))
}

// isSafeEntityType matches the small set of entity tags used in item_links —
// "item", "asset", "test_case", etc. Defensive guard against ever inlining
// arbitrary characters into SQL even though the value comes from a vetted DB
// row.
func isSafeEntityType(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			return false
		}
	}
	return true
}

// generateLinkingCustomFieldComparison emits SQL for a linking-kind custom field.
// Linking relations live in item_links keyed by custom_field_id. For primary
// fields the current item is the source; for mirror fields it is the target,
// and the link uses the primary's id.
func (g *SQLGenerator) generateLinkingCustomFieldComparison(node *ASTNode, info CustomFieldInfo) (sql string, args []any, err error) {
	if node.Operator == "~" {
		return "", nil, errors.New("contains operator (~) is not supported on linking custom fields")
	}
	_, rightArgs, err := g.generateNode(node.Right)
	if err != nil {
		return "", nil, err
	}
	if len(rightArgs) == 0 {
		return "", nil, errors.New("linking custom field comparison requires a bound value")
	}
	expr := g.linkingSubquery(info, "?")
	switch node.Operator {
	case "=":
		return expr, rightArgs, nil
	case "!=", "<>":
		return "NOT " + expr, rightArgs, nil
	default:
		return "", nil, fmt.Errorf("operator %q is not supported on linking custom fields", node.Operator)
	}
}

// generateLinkingCustomFieldInExpression handles `cf_x IN (...)` for linking kind.
func (g *SQLGenerator) generateLinkingCustomFieldInExpression(node *ASTNode, info CustomFieldInfo) (sql string, args []any, err error) {
	if node.Values.Type != NodeList {
		return "", nil, errors.New("IN expression requires a list of values")
	}
	placeholders := make([]string, 0, len(node.Values.Arguments))
	args = make([]any, 0, len(node.Values.Arguments))
	for _, v := range node.Values.Arguments {
		placeholders = append(placeholders, "?")
		args = append(args, g.convertLiteral(v))
	}
	expr := g.linkingSubquery(info, strings.Join(placeholders, ", "))
	if strings.EqualFold(node.Operator, "NOT IN") {
		return "NOT " + expr, args, nil
	}
	return expr, args, nil
}

// multiselectValueAsText normalizes a bound value to its TEXT form for the
// "value IN (?)" comparison inside the EXISTS subquery. Both option IDs stored
// as JSON integers ([1,2]) and as JSON strings (["a","b"]) round-trip when the
// bound arg is text and the row value is cast to TEXT.
func multiselectValueAsText(v any) any {
	switch x := v.(type) {
	case string:
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// mapAssetFieldName maps QL field names to asset SQL column names
// Supports custom fields using syntax: cf_fieldname, custom.fieldname, or
// cfid_<numeric_id> (collision-free, bypasses name lookup).
func (g *SQLGenerator) mapAssetFieldName(fieldName string) (expr string, args []any, err error) {
	lowerField := strings.ToLower(fieldName)
	prefix := g.aliasPrefix

	// cfid_<id>: stable numeric form, resolves directly to the JSON key.
	if id, ok := customFieldIDFromIdentifier(fieldName); ok {
		return g.jsonExtractLiteralKey(prefix+"a.custom_field_values", id), nil, nil
	}

	if strings.HasPrefix(lowerField, "cf_") {
		customFieldName := fieldName[3:]
		if !validCustomFieldNameRegex.MatchString(customFieldName) {
			return "", nil, fmt.Errorf("invalid custom field name: %s", customFieldName)
		}
		sql, args := g.extractCustomFieldScalar(prefix+"a.custom_field_values", customFieldName)
		return sql, args, nil
	}

	if strings.HasPrefix(lowerField, "custom.") {
		customFieldName := fieldName[7:]
		if !validCustomFieldNameRegex.MatchString(customFieldName) {
			return "", nil, fmt.Errorf("invalid custom field name: %s", customFieldName)
		}
		sql, args := g.extractCustomFieldScalar(prefix+"a.custom_field_values", customFieldName)
		return sql, args, nil
	}

	switch lowerField {
	case "set", "setname", "set_name":
		return prefix + "ams.name", nil, nil
	case "setid", "set_id":
		return prefix + "a.set_id", nil, nil

	case "status":
		return prefix + "ast.name", nil, nil
	case "statusid", "status_id":
		return prefix + "a.status_id", nil, nil

	case "type", "assettype", "asset_type":
		return prefix + "at.name", nil, nil
	case "typeid", "type_id", "assettypeid", "asset_type_id":
		return prefix + "a.asset_type_id", nil, nil

	case "category":
		return prefix + "ac.name", nil, nil
	case "categoryid", "category_id":
		return prefix + "a.category_id", nil, nil
	case "categorypath", "category_path":
		return prefix + "ac.path", nil, nil

	case "title":
		return prefix + "a.title", nil, nil
	case "description":
		return prefix + "a.description", nil, nil
	case "tag", "assettag", "asset_tag":
		return prefix + "a.asset_tag", nil, nil

	case "created", "created_at", "createdat":
		return prefix + "a.created_at", nil, nil
	case "updated", "updated_at", "updatedat":
		return prefix + "a.updated_at", nil, nil

	case "creator", "creatorid", "creator_id", "createdby", "created_by":
		return prefix + "a.created_by", nil, nil
	case "creatorname", "creator_name":
		return prefix + "u.first_name || ' ' || " + prefix + "u.last_name", nil, nil

	case "id":
		return prefix + "a.id", nil, nil

	default:
		return "", nil, fmt.Errorf("unknown field: %s", fieldName)
	}
}

// mapItemFieldName maps QL field names to work item SQL column names
// Supports custom fields using syntax: cf_fieldname, custom.fieldname, or
// cfid_<numeric_id> (collision-free, bypasses name lookup).
func (g *SQLGenerator) mapItemFieldName(fieldName string) (expr string, args []any, err error) {
	lowerField := strings.ToLower(fieldName)
	prefix := g.aliasPrefix

	// cfid_<id>: stable numeric form, resolves directly to the JSON key.
	if id, ok := customFieldIDFromIdentifier(fieldName); ok {
		return g.jsonExtractLiteralKey(prefix+"i.custom_field_values", id), nil, nil
	}

	if strings.HasPrefix(lowerField, "cf_") {
		// Extract field name after "cf_" prefix
		customFieldName := fieldName[3:]
		if !validCustomFieldNameRegex.MatchString(customFieldName) {
			return "", nil, fmt.Errorf("invalid custom field name: %s", customFieldName)
		}
		sql, args := g.extractItemCustomFieldScalar(prefix, customFieldName)
		return sql, args, nil
	}

	if strings.HasPrefix(lowerField, "custom.") {
		// Extract field name after "custom." prefix
		customFieldName := fieldName[7:]
		if !validCustomFieldNameRegex.MatchString(customFieldName) {
			return "", nil, fmt.Errorf("invalid custom field name: %s", customFieldName)
		}
		sql, args := g.extractItemCustomFieldScalar(prefix, customFieldName)
		return sql, args, nil
	}

	switch lowerField {
	case "workspace":
		return prefix + "w.name", nil, nil
	case "workspaceid", "workspace_id":
		return prefix + "i.workspace_id", nil, nil
	case "workspacekey":
		return prefix + "w.key", nil, nil

	case "status":
		return prefix + "st.name", nil, nil
	case "statusid", "status_id":
		return prefix + "i.status_id", nil, nil
	case "statuscategory", "status_category":
		return prefix + "sc.name", nil, nil
	case "statuscompleted", "status_completed":
		return prefix + "sc.is_completed", nil, nil
	case "priorityid", "priority_id":
		return prefix + "i.priority_id", nil, nil
	case "priority":
		return prefix + "pri.name", nil, nil

	case "title":
		return prefix + "i.title", nil, nil
	case "description":
		return prefix + "i.description", nil, nil

	case "created", "created_at", "createdat":
		return prefix + "i.created_at", nil, nil
	case "updated", "updated_at", "updatedat":
		return prefix + "i.updated_at", nil, nil
	case "completed_at":
		return CurrentCompletedAtExpr(prefix), nil, nil
	case "due_date", "due-date", "duedate":
		return prefix + "i.due_date", nil, nil

	case "assignee", "assignee_id", "assigneeid":
		return prefix + "i.assignee_id", nil, nil
	case "creator", "creator_id", "creatorid":
		return prefix + "i.creator_id", nil, nil
	case "reporter", "reporter_id", "reporterid":
		return prefix + "i.reporter_id", nil, nil

	// Milestone fields. Comparisons (=, !=, IN, ~) are intercepted in
	// generateComparison/generateInExpression and routed through the
	// item_milestones junction. The mappings below are only reached when
	// the field appears as a bare identifier (e.g., for sorting), and
	// return a scalar from the junction so existing call sites keep working.
	case "milestone", "milestone_id", "milestoneid":
		return "(SELECT MIN(ms_im.milestone_id) FROM item_milestones ms_im WHERE ms_im.item_id = " + prefix + "i.id)", nil, nil
	case "milestonename":
		return "(SELECT MIN(ms.name) FROM item_milestones ms_im JOIN milestones ms ON ms.id = ms_im.milestone_id WHERE ms_im.item_id = " + prefix + "i.id)", nil, nil

	// Iteration fields
	case "iteration", "iteration_id", "iterationid":
		return prefix + "i.iteration_id", nil, nil
	case "iterationname":
		return prefix + "iter.name", nil, nil

	// Project fields
	case "project", "project_id", "projectid":
		return prefix + "i.project_id", nil, nil
	case "projectname":
		return prefix + "proj.name", nil, nil
	case "timeproject", "time_project_id", "timeprojectid":
		return prefix + "i.time_project_id", nil, nil
	case "inheritproject", "inherit_project":
		return prefix + "i.inherit_project", nil, nil

	// Item type fields
	case "itemtype", "item_type_id", "itemtypeid":
		return prefix + "i.item_type_id", nil, nil
	case "type":
		return prefix + "it.name", nil, nil
	case "itemtypename":
		return prefix + "it.name", nil, nil

	// Hierarchy fields
	case "parent", "parent_id", "parentid":
		return prefix + "i.parent_id", nil, nil

	// Task flag
	case "istask", "is_task":
		return prefix + "i.is_task", nil, nil

	// Ranking
	case "rank":
		return prefix + "i.rank", nil, nil

	// ID
	case "id":
		return prefix + "i.id", nil, nil

	// Item Key (workspace_key + "-" + workspace_item_number)
	case "key":
		return prefix + "w.key || '-' || " + prefix + "i.workspace_item_number", nil, nil

	default:
		return "", nil, fmt.Errorf("unknown field: %s", fieldName)
	}
}

// convertLiteral converts AST literal values to appropriate Go types
func (g *SQLGenerator) convertLiteral(node *ASTNode) any {
	switch node.DataType {
	case NUMBER:
		if val, err := strconv.ParseFloat(node.Value, 64); err == nil {
			if val == float64(int64(val)) {
				return int64(val)
			}
			return val
		}
		return node.Value
	case DATE:
		if t, err := time.Parse("2006-01-02", node.Value); err == nil {
			return t
		}
		return node.Value
	case RelativeDate:
		duration, err := parseRelativeLiteral(node.Value)
		if err != nil {
			return node.Value
		}
		return g.evaluationTime.Add(duration)
	case BOOLEAN:
		// Convert to int64 for consistent database compatibility
		// SQLite stores booleans as integers, this ensures proper comparison
		if strings.EqualFold(node.Value, "true") {
			return int64(1)
		}
		return int64(0)
	case IDENTIFIER:
		return node.Value
	case NULL:
		return nil
	default:
		return node.Value
	}
}

// escapeLikePattern escapes the SQL LIKE wildcards (%, _) and the escape
// character (\) in a user-supplied search string so that a search for a
// literal "50%" doesn't behave like a wildcard match. Used for the ~ contains
// operator together with `LIKE … ESCAPE '\'`.
func escapeLikePattern(arg any) any {
	s, ok := arg.(string)
	if !ok {
		return arg
	}
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}
