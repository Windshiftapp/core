package cql

import (
	"fmt"
	"strings"
)

// Evaluator evaluates QL queries against SQL database
type Evaluator struct {
	sqlGenerator *SQLGenerator
}

// NewEvaluator creates a new QL evaluator. customFieldMap may be nil; when nil
// the generator falls back to name-based JSON extraction (legacy behavior).
func NewEvaluator(workspaceMap map[string]int, customFieldMap CustomFieldMap, dbDriver string) *Evaluator {
	return &Evaluator{
		sqlGenerator: NewSQLGenerator(workspaceMap, customFieldMap, dbDriver),
	}
}

// evaluateQL tokenizes and parses a CQL query, then generates SQL using the given generator.
// This is the shared pipeline for both item and asset evaluators.
func evaluateQL(cqlQuery string, gen *SQLGenerator) (string, []interface{}, error) { //nolint:gocritic // unnamedResult
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
	sqlStr, args, err := gen.GenerateSQL(ast)
	if err != nil {
		return "", nil, fmt.Errorf("SQL generation error: %w", err)
	}

	return sqlStr, args, nil
}

// EvaluateToSQL converts a QL query string to SQL WHERE clause
func (e *Evaluator) EvaluateToSQL(cqlQuery string) (string, []interface{}, error) { //nolint:gocritic // unnamedResult
	return evaluateQL(cqlQuery, e.sqlGenerator)
}

// AssetEvaluator evaluates QL queries for assets
type AssetEvaluator struct {
	sqlGenerator *SQLGenerator
	workspaceMap map[string]int // For linkedOf() inner queries against items
}

// NewAssetEvaluator creates a new QL evaluator for assets. assetCustomFieldMap
// covers asset-side custom fields; itemCustomFieldMap is passed through to
// inner item queries spawned by linkedOf() and may be nil if those are not
// expected to filter on item custom fields.
func NewAssetEvaluator(setMap, workspaceMap map[string]int, assetCustomFieldMap, itemCustomFieldMap CustomFieldMap, dbDriver string) *AssetEvaluator {
	return &AssetEvaluator{
		sqlGenerator: NewAssetSQLGenerator(setMap, assetCustomFieldMap, itemCustomFieldMap, dbDriver),
		workspaceMap: workspaceMap,
	}
}

// EvaluateToSQL converts a QL query string to SQL WHERE clause for assets
func (e *AssetEvaluator) EvaluateToSQL(cqlQuery string) (string, []interface{}, error) { //nolint:gocritic // unnamedResult
	// Inject workspace map for linkedOf() inner queries
	e.sqlGenerator.workspaceMap = e.workspaceMap
	return evaluateQL(cqlQuery, e.sqlGenerator)
}
