package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONSchemaToGBNF_String(t *testing.T) {
	schema := json.RawMessage(`{"type": "string"}`)
	grammar, err := JSONSchemaToGBNF(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(grammar, "root ::= string") {
		t.Errorf("expected root to reference string, got: %s", grammar)
	}
}

func TestJSONSchemaToGBNF_Integer(t *testing.T) {
	schema := json.RawMessage(`{"type": "integer"}`)
	grammar, err := JSONSchemaToGBNF(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(grammar, "root ::= integer") {
		t.Errorf("expected root to reference integer, got: %s", grammar)
	}
}

func TestJSONSchemaToGBNF_Boolean(t *testing.T) {
	schema := json.RawMessage(`{"type": "boolean"}`)
	grammar, err := JSONSchemaToGBNF(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(grammar, "root ::= boolean") {
		t.Errorf("expected root to reference boolean, got: %s", grammar)
	}
}

func TestJSONSchemaToGBNF_Object(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"age": {"type": "integer"}
		},
		"required": ["name", "age"]
	}`)
	grammar, err := JSONSchemaToGBNF(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check it contains object structure with properties
	if !strings.Contains(grammar, `"{"`) {
		t.Errorf("expected object structure with braces, got: %s", grammar)
	}
	// Properties appear as escaped quotes in GBNF: "\"name\""
	if !strings.Contains(grammar, `\"name\"`) {
		t.Errorf("expected 'name' property, got: %s", grammar)
	}
	if !strings.Contains(grammar, `\"age\"`) {
		t.Errorf("expected 'age' property, got: %s", grammar)
	}
}

func TestJSONSchemaToGBNF_Array(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "array",
		"items": {"type": "string"}
	}`)
	grammar, err := JSONSchemaToGBNF(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check it contains array structure
	if !strings.Contains(grammar, `"["`) {
		t.Errorf("expected array structure with brackets, got: %s", grammar)
	}
}

func TestJSONSchemaToGBNF_Nested(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"title": {"type": "string"}
					}
				}
			},
			"summary": {"type": "string"}
		}
	}`)
	grammar, err := JSONSchemaToGBNF(schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have multiple rules
	lines := strings.Split(grammar, "\n")
	ruleCount := 0
	for _, line := range lines {
		if strings.Contains(line, "::=") {
			ruleCount++
		}
	}
	if ruleCount < 3 {
		t.Errorf("expected at least 3 rules for nested schema, got %d: %s", ruleCount, grammar)
	}
}

func TestJSONSchemaToGBNF_PlanMyDay(t *testing.T) {
	grammar, err := JSONSchemaToGBNF(SchemaPlanMyDay)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check for key elements (properties appear as escaped quotes in GBNF)
	if !strings.Contains(grammar, `\"activities\"`) {
		t.Errorf("expected 'activities' property, got: %s", grammar)
	}
	if !strings.Contains(grammar, `\"summary\"`) {
		t.Errorf("expected 'summary' property, got: %s", grammar)
	}
}

func TestJSONSchemaToGBNF_FindSimilar(t *testing.T) {
	grammar, err := JSONSchemaToGBNF(SchemaFindSimilar)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(grammar, `\"similar_items\"`) {
		t.Errorf("expected 'similar_items' property, got: %s", grammar)
	}
}

func TestJSONSchemaToGBNF_Decompose(t *testing.T) {
	grammar, err := JSONSchemaToGBNF(SchemaDecompose)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(grammar, `\"sub_tasks\"`) {
		t.Errorf("expected 'sub_tasks' property, got: %s", grammar)
	}
	if !strings.Contains(grammar, `\"reasoning\"`) {
		t.Errorf("expected 'reasoning' property, got: %s", grammar)
	}
}

func TestJSONSchemaToGBNF_InvalidSchema(t *testing.T) {
	schema := json.RawMessage(`{invalid json`)
	_, err := JSONSchemaToGBNF(schema)
	if err == nil {
		t.Error("expected error for invalid JSON schema")
	}
}
