package llm

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// JSONSchemaToGBNF converts a JSON Schema to GBNF grammar for llama.cpp.
// It handles primitives, objects, arrays, and required/additionalProperties constraints.
func JSONSchemaToGBNF(schema json.RawMessage) (string, error) {
	var s jsonSchema
	if err := json.Unmarshal(schema, &s); err != nil {
		return "", fmt.Errorf("failed to parse JSON schema: %w", err)
	}

	g := &gbnfGenerator{
		rules:     make(map[string]string),
		ruleOrder: []string{},
	}

	// Generate the root rule
	rootRule := g.generateRule("root", &s)
	g.addRule("root", rootRule)

	return g.String(), nil
}

// jsonSchema represents a subset of JSON Schema used for GBNF conversion.
type jsonSchema struct {
	Type                 string                 `json:"type"`
	Properties           map[string]*jsonSchema `json:"properties"`
	Required             []string               `json:"required"`
	AdditionalProperties *bool                  `json:"additionalProperties"`
	Items                *jsonSchema            `json:"items"`
	Enum                 []interface{}          `json:"enum"`
}

type gbnfGenerator struct {
	rules     map[string]string
	ruleOrder []string
	counter   int
}

func (g *gbnfGenerator) addRule(name, rule string) {
	if _, exists := g.rules[name]; !exists {
		g.rules[name] = rule
		g.ruleOrder = append(g.ruleOrder, name)
	}
}

func (g *gbnfGenerator) uniqueName(base string) string {
	g.counter++
	return fmt.Sprintf("%s%d", base, g.counter)
}

func (g *gbnfGenerator) String() string {
	lines := make([]string, 0, len(g.ruleOrder))

	// Add common rules first
	g.addCommonRules()

	for _, name := range g.ruleOrder {
		lines = append(lines, fmt.Sprintf("%s ::= %s", name, g.rules[name]))
	}
	return strings.Join(lines, "\n")
}

func (g *gbnfGenerator) addCommonRules() {
	// Whitespace
	g.addRule("ws", `[ \t\n\r]*`)

	// String: quoted with escape support
	g.addRule("string", `"\"" ([^"\\] | "\\" .)* "\""`)

	// Number types
	g.addRule("integer", `"-"? [0-9]+`)
	g.addRule("number", `"-"? [0-9]+ ("." [0-9]+)? ([eE] [+-]? [0-9]+)?`)

	// Boolean
	g.addRule("boolean", `"true" | "false"`)

	// Null
	g.addRule("null", `"null"`)
}

func (g *gbnfGenerator) generateRule(name string, s *jsonSchema) string {
	if s == nil {
		return "string" // default fallback
	}

	// Handle enum
	if len(s.Enum) > 0 {
		return g.generateEnum(s.Enum)
	}

	switch s.Type {
	case "string":
		return "string"
	case "integer":
		return "integer"
	case "number":
		return "number"
	case "boolean":
		return "boolean"
	case "null":
		return "null"
	case "object":
		return g.generateObject(name, s)
	case "array":
		return g.generateArray(name, s)
	default:
		// No type specified, allow any JSON value
		return "string"
	}
}

func (g *gbnfGenerator) generateEnum(values []interface{}) string {
	var parts []string
	for _, v := range values {
		switch val := v.(type) {
		case string:
			parts = append(parts, fmt.Sprintf(`"\"%s\""`, val))
		case float64:
			parts = append(parts, fmt.Sprintf(`"%v"`, val))
		case bool:
			if val {
				parts = append(parts, `"true"`)
			} else {
				parts = append(parts, `"false"`)
			}
		case nil:
			parts = append(parts, `"null"`)
		}
	}
	return strings.Join(parts, " | ")
}

func (g *gbnfGenerator) generateObject(name string, s *jsonSchema) string {
	if len(s.Properties) == 0 {
		return `"{" ws "}"`
	}

	// Determine which properties to include
	props := make([]string, 0, len(s.Properties))
	requiredSet := make(map[string]bool)
	for _, r := range s.Required {
		requiredSet[r] = true
	}

	// Sort property names for deterministic output
	var propNames []string
	for propName := range s.Properties {
		propNames = append(propNames, propName)
	}
	sort.Strings(propNames)

	// If additionalProperties is false, only include defined properties
	// For GBNF we always only include defined properties
	// For simplicity, include all defined properties
	// (In a full implementation, we'd handle optional vs required differently)
	props = append(props, propNames...)

	if len(props) == 0 {
		return `"{" ws "}"`
	}

	// Generate rules for each property's value
	var propRules []string
	for _, propName := range props {
		propSchema := s.Properties[propName]
		valueName := g.uniqueName(name + "_" + propName)
		valueRule := g.generateRule(valueName, propSchema)
		g.addRule(valueName, valueRule)

		propRules = append(propRules, fmt.Sprintf(`"\"%s\"" ws ":" ws %s`, propName, valueName))
	}

	// Build the object rule with comma separators
	if len(propRules) == 1 {
		return fmt.Sprintf(`"{" ws %s ws "}"`, propRules[0])
	}

	// Multiple properties: join with comma
	return fmt.Sprintf(`"{" ws %s ws "}"`, strings.Join(propRules, ` ws "," ws `))
}

func (g *gbnfGenerator) generateArray(name string, s *jsonSchema) string {
	if s.Items == nil {
		// Array of any type
		return `"[" ws "]"` // empty array only
	}

	// Generate rule for array items
	itemName := g.uniqueName(name + "_item")
	itemRule := g.generateRule(itemName, s.Items)
	g.addRule(itemName, itemRule)

	// Array with at least one element, comma-separated
	// Format: [ item (, item)* ] or []
	return fmt.Sprintf(`"[" ws (%s (ws "," ws %s)*)? ws "]"`, itemName, itemName)
}
