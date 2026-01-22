// Package cql implements Windshift Query Language (QL) - a JQL-like query language for Windshift
//
// Supported Fields:
//   - workspace, workspaceId, workspaceKey
//   - status, priority
//   - title, description
//   - key (item key in format "WK-123")
//   - created, updated
//   - assignee, creator
//   - milestone, milestoneName
//   - project, projectName, timeProject, inheritProject
//   - itemType, itemTypeName
//   - parent, hasChildren, childrenCount, descendantsCount
//   - isTask, rank, id
//   - Custom fields: cf_fieldname or custom.fieldname (e.g., cf_sprint, custom.epicLink)
//
// Supported Operators:
//   - Comparison: =, !=, <, <=, >, >=, ~  (contains)
//   - Logical: AND, OR, NOT
//   - Set membership: IN, NOT IN
//
// Supported Functions:
//   - currentUser(), now(), startOfDay(), endOfDay()
//   - childrenOf("ql query") - Find all descendants of items matching the query
//   - linkedOf("link_label", "ql query") - Find items linked via the specified link type
//
// Examples:
//   - workspace = "My Project" AND status = "open"
//   - priority IN ("high", "critical") AND assignee = 5
//   - cf_sprint = "Sprint 1" AND status != "done"
//   - custom.epicLink = "PROJ-123" AND milestone = 1
//   - created >= "2024-01-01" AND updated < now()
//   - childrenOf("priority = high") - Find all descendants of high priority items
//   - linkedOf("blocks", "status = open") - Find items blocked by open items
package cql

// EntityType represents the type of entity being queried
type EntityType string

const (
	// EntityTypeItem is for work item queries (default)
	EntityTypeItem EntityType = "item"
	// EntityTypeAsset is for asset queries
	EntityTypeAsset EntityType = "asset"
)

// Token represents a QL token
type Token struct {
	Type  TokenType
	Value string
	Pos   int
}

// TokenType represents the type of a QL token
type TokenType int

const (
	// Literals
	IDENTIFIER TokenType = iota
	STRING
	NUMBER
	DATE
	BOOLEAN

	// Operators
	EQUALS        // =
	NOT_EQUALS    // !=, <>
	LESS_THAN     // <
	LESS_EQUAL    // <=
	GREATER_THAN  // >
	GREATER_EQUAL // >=
	CONTAINS      // ~
	IN            // IN
	NOT_IN        // NOT IN

	// Logical operators
	AND
	OR
	NOT

	// Punctuation
	LPAREN // (
	RPAREN // )
	COMMA  // ,

	// Special
	EOF
	FUNCTION
)

// String returns a string representation of the token type
func (t TokenType) String() string {
	names := []string{
		"IDENTIFIER", "STRING", "NUMBER", "DATE", "BOOLEAN",
		"EQUALS", "NOT_EQUALS", "LESS_THAN", "LESS_EQUAL", "GREATER_THAN", "GREATER_EQUAL", "CONTAINS", "IN", "NOT_IN",
		"AND", "OR", "NOT",
		"LPAREN", "RPAREN", "COMMA",
		"EOF", "FUNCTION",
	}
	if int(t) < len(names) {
		return names[t]
	}
	return "UNKNOWN"
}

// AST Node types
type NodeType int

const (
	NodeBinaryOp NodeType = iota
	NodeComparison
	NodeInExpression
	NodeIdentifier
	NodeLiteral
	NodeFunction
	NodeList
)

// ASTNode represents a node in the Abstract Syntax Tree
type ASTNode struct {
	Type      NodeType
	Value     string
	DataType  TokenType   // For literals
	Operator  string
	Left      *ASTNode
	Right     *ASTNode
	Field     *ASTNode    // For IN expressions
	Values    *ASTNode    // For IN expressions
	Arguments []*ASTNode  // For function calls
}
