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

func TestGenerator_NOTSafeNotEqualForCustomFields(t *testing.T) {
	ast, err := parseQL(t, `cf_status != "active"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, "sqlite")
	sqlStr, _, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(sqlStr, "IS NULL OR") {
		t.Fatalf("expected NULL-safe SQL for cf_x != value, got: %s", sqlStr)
	}
}

func TestGenerator_TildeOnCustomTextField(t *testing.T) {
	ast, err := parseQL(t, `cf_notes ~ "todo"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, "sqlite")
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

func TestGenerator_BooleanCustomField(t *testing.T) {
	ast, err := parseQL(t, `cf_done = true`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, "sqlite")
	_, args, err := gen.GenerateSQL(ast)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// The right-side bound arg should be the string "true", not int64(1),
	// so the comparison round-trips against JSON ->> "true"/"false" text.
	found := false
	for _, a := range args {
		if s, ok := a.(string); ok && (s == "true" || s == "false") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a string arg \"true\" for boolean comparison, got: %#v", args)
	}
}

func TestGenerator_TildeEscapesLikeWildcards(t *testing.T) {
	ast, err := parseQL(t, `title ~ "50%"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	gen := NewSQLGenerator(map[string]int{}, "sqlite")
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
