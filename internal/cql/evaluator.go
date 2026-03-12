package cql

import (
	"fmt"
	"strings"
)

// Evaluator evaluates QL queries against SQL database
type Evaluator struct {
	sqlGenerator *SQLGenerator
}

// NewEvaluator creates a new QL evaluator
func NewEvaluator(workspaceMap map[string]int, dbDriver string) *Evaluator {
	return &Evaluator{
		sqlGenerator: NewSQLGenerator(workspaceMap, dbDriver),
	}
}

// EvaluateToSQL converts a QL query string to SQL WHERE clause
func (e *Evaluator) EvaluateToSQL(cqlQuery string) (string, []interface{}, error) { //nolint:gocritic // unnamedResult
	if strings.TrimSpace(cqlQuery) == "" {
		return "", nil, nil
	}

	// Tokenize
	tokenizer := NewTokenizer(cqlQuery)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		return "", nil, fmt.Errorf("tokenization error: %w", err)
	}

	// Parse
	parser := NewParser(tokens)
	ast, err := parser.Parse()
	if err != nil {
		return "", nil, fmt.Errorf("parse error: %w", err)
	}

	// Generate SQL
	sql, args, err := e.sqlGenerator.GenerateSQL(ast)
	if err != nil {
		return "", nil, fmt.Errorf("SQL generation error: %w", err)
	}

	return sql, args, nil
}

// AssetEvaluator evaluates QL queries for assets
type AssetEvaluator struct {
	sqlGenerator *SQLGenerator
	workspaceMap map[string]int // For linkedOf() inner queries against items
}

// NewAssetEvaluator creates a new QL evaluator for assets
func NewAssetEvaluator(setMap, workspaceMap, customFieldMap map[string]int, dbDriver string) *AssetEvaluator {
	return &AssetEvaluator{
		sqlGenerator: NewAssetSQLGenerator(setMap, customFieldMap, dbDriver),
		workspaceMap: workspaceMap,
	}
}

// EvaluateToSQL converts a QL query string to SQL WHERE clause for assets
func (e *AssetEvaluator) EvaluateToSQL(cqlQuery string) (string, []interface{}, error) { //nolint:gocritic // unnamedResult
	if strings.TrimSpace(cqlQuery) == "" {
		return "", nil, nil
	}

	// Tokenize
	tokenizer := NewTokenizer(cqlQuery)
	tokens, err := tokenizer.Tokenize()
	if err != nil {
		return "", nil, fmt.Errorf("tokenization error: %w", err)
	}

	// Parse
	parser := NewParser(tokens)
	ast, err := parser.Parse()
	if err != nil {
		return "", nil, fmt.Errorf("parse error: %w", err)
	}

	// Generate SQL using asset generator with workspace map for linkedOf
	e.sqlGenerator.workspaceMap = e.workspaceMap
	sql, args, err := e.sqlGenerator.GenerateSQL(ast)
	if err != nil {
		return "", nil, fmt.Errorf("SQL generation error: %w", err)
	}

	return sql, args, nil
}
