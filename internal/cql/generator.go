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
	workspaceMap map[string]int // Maps workspace names/keys to IDs
	aliasPrefix  string         // Prefix for table aliases ("" for outer, "inner_" for inner queries)
	entityType   EntityType     // Type of entity being queried (item or asset)
	setMap       map[string]int // Maps asset set names to IDs (for asset queries)
}

// NewSQLGenerator creates a new SQL generator for outer queries (work items)
func NewSQLGenerator(workspaceMap map[string]int) *SQLGenerator {
	return &SQLGenerator{
		workspaceMap: workspaceMap,
		aliasPrefix:  "",
		entityType:   EntityTypeItem,
	}
}

// NewInnerSQLGenerator creates a new SQL generator for inner/nested queries (work items)
// Uses "inner_" prefix for table aliases to avoid collision with outer query
func NewInnerSQLGenerator(workspaceMap map[string]int) *SQLGenerator {
	return &SQLGenerator{
		workspaceMap: workspaceMap,
		aliasPrefix:  "inner_",
		entityType:   EntityTypeItem,
	}
}

// NewAssetSQLGenerator creates a new SQL generator for asset queries
func NewAssetSQLGenerator(setMap map[string]int) *SQLGenerator {
	return &SQLGenerator{
		setMap:      setMap,
		aliasPrefix: "",
		entityType:  EntityTypeAsset,
	}
}

// NewInnerAssetSQLGenerator creates a new SQL generator for inner asset queries
func NewInnerAssetSQLGenerator(setMap map[string]int) *SQLGenerator {
	return &SQLGenerator{
		setMap:      setMap,
		aliasPrefix: "inner_",
		entityType:  EntityTypeAsset,
	}
}

// GenerateSQL converts a QL AST to SQL WHERE clause
func (g *SQLGenerator) GenerateSQL(ast *ASTNode) (string, []interface{}, error) {
	if ast == nil {
		return "", nil, nil
	}

	return g.generateNode(ast)
}

// generateNode generates SQL for a single AST node
func (g *SQLGenerator) generateNode(node *ASTNode) (string, []interface{}, error) {
	switch node.Type {
	case NodeBinaryOp:
		return g.generateBinaryOp(node)
	case NodeComparison:
		return g.generateComparison(node)
	case NodeInExpression:
		return g.generateInExpression(node)
	case NodeIdentifier:
		sql, err := g.mapFieldName(node.Value)
		if err != nil {
			return "", nil, err
		}
		return sql, nil, nil
	case NodeLiteral:
		return "?", []interface{}{g.convertLiteral(node)}, nil
	case NodeFunction:
		return g.generateFunction(node)
	default:
		return "", nil, fmt.Errorf("unsupported node type: %v", node.Type)
	}
}

// generateBinaryOp generates SQL for binary operations (AND, OR, NOT)
func (g *SQLGenerator) generateBinaryOp(node *ASTNode) (string, []interface{}, error) {
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
		args := append(leftArgs, rightArgs...)
		return fmt.Sprintf("(%s AND %s)", leftSQL, rightSQL), args, nil

	case "OR":
		leftSQL, leftArgs, err := g.generateNode(node.Left)
		if err != nil {
			return "", nil, err
		}
		rightSQL, rightArgs, err := g.generateNode(node.Right)
		if err != nil {
			return "", nil, err
		}
		args := append(leftArgs, rightArgs...)
		return fmt.Sprintf("(%s OR %s)", leftSQL, rightSQL), args, nil

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

// getNameFieldForIdField returns the corresponding name field for an ID field
// Returns the name field and true if this is a reference field, or empty string and false if not
func (g *SQLGenerator) getNameFieldForIdField(fieldName string) (string, bool) {
	lowerField := strings.ToLower(fieldName)

	switch lowerField {
	case "project", "project_id", "projectid":
		return "proj.name", true
	case "milestone", "milestone_id", "milestoneid":
		return "m.name", true
	case "itemtype", "item_type_id", "itemtypeid":
		return "it.name", true
	case "timeproject", "time_project_id", "timeprojectid":
		return "tp.name", true
	default:
		return "", false
	}
}

// generateComparison generates SQL for comparison operations
func (g *SQLGenerator) generateComparison(node *ASTNode) (string, []interface{}, error) {
	leftSQL, leftArgs, err := g.generateNode(node.Left)
	if err != nil {
		return "", nil, err
	}

	rightSQL, rightArgs, err := g.generateNode(node.Right)
	if err != nil {
		return "", nil, err
	}

	args := append(leftArgs, rightArgs...)

	// Smart reference field handling: if comparing an ID field with a string value,
	// automatically use the corresponding name field instead
	isReferenceFieldComparison := false
	if node.Left.Type == NodeIdentifier && node.Right.Type == NodeLiteral && node.Right.DataType == STRING {
		if nameField, isReferenceField := g.getNameFieldForIdField(node.Left.Value); isReferenceField {
			// Replace the ID field with the name field for string comparisons
			leftSQL = nameField
			isReferenceFieldComparison = true
		}
	}

	// Check if we're comparing status, priority, or type fields - make them case-insensitive
	isCaseInsensitiveField := false
	if node.Left.Type == NodeIdentifier {
		fieldName := strings.ToLower(node.Left.Value)
		// status and priority apply to items, status and type apply to assets
		if fieldName == "status" || fieldName == "priority" || fieldName == "type" || fieldName == "assettype" || fieldName == "asset_type" || fieldName == "category" {
			isCaseInsensitiveField = true
		}
	}

	// If comparing case-insensitive field with an unquoted identifier (e.g., "priority = high"),
	// treat the right side as a string value, not a column name
	if isCaseInsensitiveField && node.Right.Type == NodeIdentifier {
		rightSQL = "?"
		// Append to existing args (which may include leftArgs), replacing rightArgs
		args = append(leftArgs, node.Right.Value)
	}

	switch node.Operator {
	case "=":
		if isCaseInsensitiveField {
			// Make status, priority, type, category comparisons case-insensitive
			return fmt.Sprintf("LOWER(%s) = LOWER(%s)", leftSQL, rightSQL), args, nil
		}
		if isReferenceFieldComparison {
			// For reference field comparisons, add NULL check to exclude items without the field
			return fmt.Sprintf("(%s IS NOT NULL AND %s = %s)", leftSQL, leftSQL, rightSQL), args, nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), args, nil
	case "!=", "<>":
		if isCaseInsensitiveField {
			// Make status, priority, type, category comparisons case-insensitive
			return fmt.Sprintf("LOWER(%s) != LOWER(%s)", leftSQL, rightSQL), args, nil
		}
		if isReferenceFieldComparison {
			// For reference field comparisons, add NULL check to exclude items without the field
			return fmt.Sprintf("(%s IS NOT NULL AND %s != %s)", leftSQL, leftSQL, rightSQL), args, nil
		}
		return fmt.Sprintf("%s != %s", leftSQL, rightSQL), args, nil
	case "<":
		return fmt.Sprintf("%s < %s", leftSQL, rightSQL), args, nil
	case "<=":
		return fmt.Sprintf("%s <= %s", leftSQL, rightSQL), args, nil
	case ">":
		return fmt.Sprintf("%s > %s", leftSQL, rightSQL), args, nil
	case ">=":
		return fmt.Sprintf("%s >= %s", leftSQL, rightSQL), args, nil
	case "~":
		// Only allow contains operator for text fields (title, description, tag/asset_tag)
		isTextFieldComparison := false
		if node.Left.Type == NodeIdentifier {
			fieldName := strings.ToLower(node.Left.Value)
			if fieldName == "title" || fieldName == "description" || fieldName == "tag" || fieldName == "assettag" || fieldName == "asset_tag" {
				isTextFieldComparison = true
			}
		}

		if !isTextFieldComparison {
			return "", nil, fmt.Errorf("contains operator (~) can only be used with text fields (title, description, tag)")
		}

		// Convert to SQL LIKE with wildcards
		if isReferenceFieldComparison {
			// For reference field comparisons, add NULL check to exclude items without the field
			return fmt.Sprintf("(%s IS NOT NULL AND %s LIKE %s)", leftSQL, leftSQL, "'%' || ? || '%'"), args, nil
		}
		return fmt.Sprintf("%s LIKE %s", leftSQL, "'%' || ? || '%'"), args, nil
	default:
		return "", nil, fmt.Errorf("unsupported comparison operator: %s", node.Operator)
	}
}

// generateInExpression generates SQL for IN expressions
func (g *SQLGenerator) generateInExpression(node *ASTNode) (string, []interface{}, error) {
	fieldSQL, fieldArgs, err := g.generateNode(node.Field)
	if err != nil {
		return "", nil, err
	}

	if node.Values.Type != NodeList {
		return "", nil, errors.New("IN expression requires a list of values")
	}

	// Smart reference field handling: check if any value is a string and this is a reference field
	hasStringValue := false
	for _, valueNode := range node.Values.Arguments {
		if valueNode.DataType == STRING {
			hasStringValue = true
			break
		}
	}

	// If we have string values and this is a reference field, use the name field
	isReferenceFieldIn := false
	if node.Field.Type == NodeIdentifier && hasStringValue {
		if nameField, isReferenceField := g.getNameFieldForIdField(node.Field.Value); isReferenceField {
			// Replace the ID field with the name field for string comparisons
			fieldSQL = nameField
			isReferenceFieldIn = true
		}
	}

	// Check if we're comparing status, priority, type, or category fields - make them case-insensitive
	isCaseInsensitiveField := false
	if node.Field.Type == NodeIdentifier {
		fieldName := strings.ToLower(node.Field.Value)
		if fieldName == "status" || fieldName == "priority" || fieldName == "type" || fieldName == "assettype" || fieldName == "asset_type" || fieldName == "category" {
			isCaseInsensitiveField = true
		}
	}

	var placeholders []string
	var args []interface{}
	args = append(args, fieldArgs...)

	for _, valueNode := range node.Values.Arguments {
		if isCaseInsensitiveField {
			placeholders = append(placeholders, "LOWER(?)")
		} else {
			placeholders = append(placeholders, "?")
		}
		args = append(args, g.convertLiteral(valueNode))
	}

	placeholderList := strings.Join(placeholders, ", ")

	if isCaseInsensitiveField {
		// Make status, priority, type, category IN comparisons case-insensitive
		if strings.ToUpper(node.Operator) == "NOT IN" {
			return fmt.Sprintf("LOWER(%s) NOT IN (%s)", fieldSQL, placeholderList), args, nil
		}
		return fmt.Sprintf("LOWER(%s) IN (%s)", fieldSQL, placeholderList), args, nil
	}

	if isReferenceFieldIn {
		// For reference field IN comparisons, add NULL check to exclude items without the field
		if strings.ToUpper(node.Operator) == "NOT IN" {
			return fmt.Sprintf("(%s IS NOT NULL AND %s NOT IN (%s))", fieldSQL, fieldSQL, placeholderList), args, nil
		}
		return fmt.Sprintf("(%s IS NOT NULL AND %s IN (%s))", fieldSQL, fieldSQL, placeholderList), args, nil
	}

	if strings.ToUpper(node.Operator) == "NOT IN" {
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
func (g *SQLGenerator) generateFunction(node *ASTNode) (string, []interface{}, error) {
	switch strings.ToLower(node.Value) {
	case "currentuser":
		// This would need to be filled in with actual user context
		return "?", []interface{}{"current-user-id"}, nil
	case "now":
		return "datetime('now')", nil, nil
	case "startofday":
		return "date('now')", nil, nil
	case "endofday":
		return "datetime('now', '+1 day', '-1 second')", nil, nil

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

		innerGenerator := NewInnerSQLGenerator(g.workspaceMap)
		innerSQL, innerArgs, err := innerGenerator.GenerateSQL(innerAST)
		if err != nil {
			return "", nil, fmt.Errorf("childrenOf() inner query SQL generation error: %w", err)
		}

		// Generate recursive CTE to find all descendants (children only, not the parents)
		// Base case: find direct children of items matching the inner query
		// Recursive case: find children of those children
		// Note: Uses inner_ prefix for all table aliases to avoid collision with outer query's aliases
		sql := fmt.Sprintf(`i.id IN (
			WITH RECURSIVE descendants AS (
				-- Base case: direct children of items matching the inner query
				SELECT child.id FROM items child
				WHERE child.parent_id IN (
					SELECT inner_i.id FROM items inner_i
					LEFT JOIN workspaces inner_w ON inner_i.workspace_id = inner_w.id
					LEFT JOIN item_types inner_it ON inner_i.item_type_id = inner_it.id
					LEFT JOIN items inner_p ON inner_i.parent_id = inner_p.id
					LEFT JOIN milestones inner_m ON inner_i.milestone_id = inner_m.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				)
				UNION ALL
				-- Recursive case: children of descendants
				SELECT rec_i.id FROM items rec_i
				JOIN descendants d ON rec_i.parent_id = d.id
			)
			SELECT id FROM descendants
		)`, innerSQL)

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
func (g *SQLGenerator) generateItemLinkedOf(node *ASTNode) (string, []interface{}, error) {
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

	innerGenerator := NewInnerSQLGenerator(g.workspaceMap)
	innerSQL, innerArgs, err := innerGenerator.GenerateSQL(innerAST)
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() inner query SQL generation error: %w", err)
	}

	// Generate SQL that:
	// 1. Finds the link type by matching the label against forward_label or reverse_label
	// 2. If forward_label matches: return target items (source -> target direction)
	// 3. If reverse_label matches: return source items (target <- source direction)
	sql := fmt.Sprintf(`i.id IN (
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
					LEFT JOIN milestones inner_m ON inner_i.milestone_id = inner_m.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				))
				OR
				(lt.reverse_label = ? AND il.target_id IN (
					SELECT inner_i.id FROM items inner_i
					LEFT JOIN workspaces inner_w ON inner_i.workspace_id = inner_w.id
					LEFT JOIN item_types inner_it ON inner_i.item_type_id = inner_it.id
					LEFT JOIN items inner_p ON inner_i.parent_id = inner_p.id
					LEFT JOIN milestones inner_m ON inner_i.milestone_id = inner_m.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				))
			)
	)`, innerSQL, innerSQL)

	// Add link label arguments (used multiple times in the query)
	args := []interface{}{linkLabel, linkLabel, linkLabel, linkLabel, linkLabel}
	args = append(args, innerArgs...) // First occurrence of inner query
	args = append(args, linkLabel)    // One more label for reverse check
	args = append(args, innerArgs...) // Second occurrence of inner query

	return sql, args, nil
}

// generateAssetLinkedOf generates SQL for finding assets linked to items matching a query
func (g *SQLGenerator) generateAssetLinkedOf(node *ASTNode) (string, []interface{}, error) {
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

	// Parse and generate SQL for the inner QL query (queries items)
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

	// Use item SQL generator for the inner query (querying items, not assets)
	innerGenerator := NewInnerSQLGenerator(g.workspaceMap)
	innerSQL, innerArgs, err := innerGenerator.GenerateSQL(innerAST)
	if err != nil {
		return "", nil, fmt.Errorf("linkedOf() inner query SQL generation error: %w", err)
	}

	// Generate SQL to find assets linked to items matching the inner query
	// Assets can be linked to items via item_links table where:
	// - source_type='asset' and target_type='item' (asset links to item)
	// - source_type='item' and target_type='asset' (item links to asset)
	sql := fmt.Sprintf(`a.id IN (
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
					LEFT JOIN milestones inner_m ON inner_i.milestone_id = inner_m.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				))
				OR
				(il.target_type = 'asset' AND il.source_type = 'item' AND il.source_id IN (
					SELECT inner_i.id FROM items inner_i
					LEFT JOIN workspaces inner_w ON inner_i.workspace_id = inner_w.id
					LEFT JOIN item_types inner_it ON inner_i.item_type_id = inner_it.id
					LEFT JOIN items inner_p ON inner_i.parent_id = inner_p.id
					LEFT JOIN milestones inner_m ON inner_i.milestone_id = inner_m.id
					LEFT JOIN iterations inner_iter ON inner_i.iteration_id = inner_iter.id
					LEFT JOIN time_projects inner_proj ON inner_i.project_id = inner_proj.id
					LEFT JOIN time_projects inner_tp ON inner_i.time_project_id = inner_tp.id
					LEFT JOIN users inner_assignee ON inner_i.assignee_id = inner_assignee.id
					LEFT JOIN users inner_creator ON inner_i.creator_id = inner_creator.id
					LEFT JOIN statuses inner_st ON inner_i.status_id = inner_st.id
					LEFT JOIN priorities inner_pri ON inner_i.priority_id = inner_pri.id
					WHERE %s
				))
			)
	)`, innerSQL, innerSQL)

	// Add link label arguments
	args := []interface{}{linkLabel, linkLabel}
	args = append(args, innerArgs...) // First occurrence of inner query
	args = append(args, innerArgs...) // Second occurrence of inner query

	return sql, args, nil
}

// validCustomFieldName validates that a custom field name contains only safe characters
// for use in JSON paths. Returns true if the name is safe.
var validCustomFieldNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// mapFieldName maps QL field names to SQL column names
// Dispatches to entity-specific mapping based on entityType
func (g *SQLGenerator) mapFieldName(fieldName string) (string, error) {
	if g.entityType == EntityTypeAsset {
		return g.mapAssetFieldName(fieldName)
	}
	return g.mapItemFieldName(fieldName)
}

// mapAssetFieldName maps QL field names to asset SQL column names
// Supports custom fields using syntax: cf_fieldname or custom.fieldname
func (g *SQLGenerator) mapAssetFieldName(fieldName string) (string, error) {
	lowerField := strings.ToLower(fieldName)
	prefix := g.aliasPrefix

	// Check for custom field syntax: cf_fieldname or custom.fieldname
	if strings.HasPrefix(lowerField, "cf_") {
		customFieldName := fieldName[3:]
		if !validCustomFieldNameRegex.MatchString(customFieldName) {
			return "", fmt.Errorf("invalid custom field name: %s", customFieldName)
		}
		return fmt.Sprintf("json_extract(%sa.custom_field_values, '$.%s')", prefix, customFieldName), nil
	}

	if strings.HasPrefix(lowerField, "custom.") {
		customFieldName := fieldName[7:]
		if !validCustomFieldNameRegex.MatchString(customFieldName) {
			return "", fmt.Errorf("invalid custom field name: %s", customFieldName)
		}
		return fmt.Sprintf("json_extract(%sa.custom_field_values, '$.%s')", prefix, customFieldName), nil
	}

	// Standard asset field mappings
	switch lowerField {
	// Set fields (equivalent to workspace for items)
	case "set", "setname", "set_name":
		return prefix + "ams.name", nil
	case "setid", "set_id":
		return prefix + "a.set_id", nil

	// Status fields
	case "status":
		return prefix + "ast.name", nil
	case "statusid", "status_id":
		return prefix + "a.status_id", nil

	// Type fields
	case "type", "assettype", "asset_type":
		return prefix + "at.name", nil
	case "typeid", "type_id", "assettypeid", "asset_type_id":
		return prefix + "a.asset_type_id", nil

	// Category fields
	case "category":
		return prefix + "ac.name", nil
	case "categoryid", "category_id":
		return prefix + "a.category_id", nil
	case "categorypath", "category_path":
		return prefix + "ac.path", nil

	// Basic text fields
	case "title":
		return prefix + "a.title", nil
	case "description":
		return prefix + "a.description", nil
	case "tag", "assettag", "asset_tag":
		return prefix + "a.asset_tag", nil

	// Date fields
	case "created", "created_at", "createdat":
		return prefix + "a.created_at", nil
	case "updated", "updated_at", "updatedat":
		return prefix + "a.updated_at", nil

	// Creator fields
	case "creator", "creatorid", "creator_id", "createdby", "created_by":
		return prefix + "a.created_by", nil
	case "creatorname", "creator_name":
		return prefix + "u.first_name || ' ' || " + prefix + "u.last_name", nil

	// ID
	case "id":
		return prefix + "a.id", nil

	default:
		return "", fmt.Errorf("unknown field: %s", fieldName)
	}
}

// mapItemFieldName maps QL field names to work item SQL column names
// Supports custom fields using syntax: cf_fieldname or custom.fieldname
func (g *SQLGenerator) mapItemFieldName(fieldName string) (string, error) {
	lowerField := strings.ToLower(fieldName)
	prefix := g.aliasPrefix

	// Check for custom field syntax: cf_fieldname or custom.fieldname
	if strings.HasPrefix(lowerField, "cf_") {
		// Extract field name after "cf_" prefix
		customFieldName := fieldName[3:]
		if !validCustomFieldNameRegex.MatchString(customFieldName) {
			return "", fmt.Errorf("invalid custom field name: %s", customFieldName)
		}
		return fmt.Sprintf("json_extract(%si.custom_field_values, '$.%s')", prefix, customFieldName), nil
	}

	if strings.HasPrefix(lowerField, "custom.") {
		// Extract field name after "custom." prefix
		customFieldName := fieldName[7:]
		if !validCustomFieldNameRegex.MatchString(customFieldName) {
			return "", fmt.Errorf("invalid custom field name: %s", customFieldName)
		}
		return fmt.Sprintf("json_extract(%si.custom_field_values, '$.%s')", prefix, customFieldName), nil
	}

	// Standard field mappings
	switch lowerField {
	// Workspace fields
	case "workspace":
		return prefix + "w.name", nil
	case "workspaceid", "workspace_id":
		return prefix + "i.workspace_id", nil
	case "workspacekey":
		return prefix + "w.key", nil

	// Status and priority
	case "status":
		return prefix + "st.name", nil
	case "statusid", "status_id":
		return prefix + "i.status_id", nil
	case "priorityid", "priority_id":
		return prefix + "i.priority_id", nil
	case "priority":
		return prefix + "pri.name", nil

	// Basic text fields
	case "title":
		return prefix + "i.title", nil
	case "description":
		return prefix + "i.description", nil

	// Date fields
	case "created", "created_at", "createdat":
		return prefix + "i.created_at", nil
	case "updated", "updated_at", "updatedat":
		return prefix + "i.updated_at", nil
	case "due_date", "due-date", "duedate":
		return prefix + "i.due_date", nil

	// User assignments
	case "assignee", "assignee_id", "assigneeid":
		return prefix + "i.assignee_id", nil
	case "creator", "creator_id", "creatorid":
		return prefix + "i.creator_id", nil

	// Milestone fields
	case "milestone", "milestone_id", "milestoneid":
		return prefix + "i.milestone_id", nil
	case "milestonename":
		return prefix + "m.name", nil

	// Iteration fields
	case "iteration", "iteration_id", "iterationid":
		return prefix + "i.iteration_id", nil
	case "iterationname":
		return prefix + "iter.name", nil

	// Project fields
	case "project", "project_id", "projectid":
		return prefix + "i.project_id", nil
	case "projectname":
		return prefix + "proj.name", nil
	case "timeproject", "time_project_id", "timeprojectid":
		return prefix + "i.time_project_id", nil
	case "inheritproject", "inherit_project":
		return prefix + "i.inherit_project", nil

	// Item type fields
	case "itemtype", "item_type_id", "itemtypeid":
		return prefix + "i.item_type_id", nil
	case "itemtypename":
		return prefix + "it.name", nil

	// Hierarchy fields
	case "parent", "parent_id", "parentid":
		return prefix + "i.parent_id", nil

	// Task flag
	case "istask", "is_task":
		return prefix + "i.is_task", nil

	// Ranking
	case "rank":
		return prefix + "i.rank", nil

	// ID
	case "id":
		return prefix + "i.id", nil

	// Item Key (workspace_key + "-" + workspace_item_number)
	case "key":
		return prefix + "w.key || '-' || " + prefix + "i.workspace_item_number", nil

	default:
		return "", fmt.Errorf("unknown field: %s", fieldName)
	}
}

// convertLiteral converts AST literal values to appropriate Go types
func (g *SQLGenerator) convertLiteral(node *ASTNode) interface{} {
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
	case BOOLEAN:
		// Convert to int64 for consistent database compatibility
		// SQLite stores booleans as integers, this ensures proper comparison
		if strings.ToLower(node.Value) == "true" {
			return int64(1)
		}
		return int64(0)
	case IDENTIFIER:
		// For identifier literals in IN clauses, try to resolve workspace names
		if id, exists := g.workspaceMap[strings.ToLower(node.Value)]; exists {
			return id
		}
		return node.Value
	default:
		return node.Value
	}
}
