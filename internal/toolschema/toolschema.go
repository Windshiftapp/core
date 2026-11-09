// Package toolschema canonicalizes tool parameter schemas for the strict
// validators some providers and MCP clients use.
//
// JSON Schema makes "properties" optional on an object: {"type":"object"} and
// {"type":"object","properties":{}} describe the same documents. OpenAI accepts
// either, but stricter validators — llama.cpp behind LM Studio, and Zod-style
// request validators generally — reject a function whose parameters object has
// no "properties" member. Emitting the explicit form is semantically lossless
// and is the shape internal/llm/probe.go already sends on every connection
// test, so it stays inside the compatibility set of every supported provider.
package toolschema

import "encoding/json"

// EnsureProperties adds an explicit empty "properties" member to a tool
// parameter schema that has none. It mutates schema in place and is idempotent.
func EnsureProperties(schema map[string]any) {
	if schema == nil {
		return
	}
	if _, ok := schema["properties"]; ok {
		return
	}
	schema["properties"] = map[string]any{}
}

// NormalizeParameters is EnsureProperties for a marshaled schema. It returns
// raw unchanged when properties is already present so schemas that do not need
// it keep their exact bytes.
func NormalizeParameters(raw json.RawMessage) (json.RawMessage, error) {
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	if _, ok := schema["properties"]; ok {
		return raw, nil
	}
	schema["properties"] = json.RawMessage("{}")
	return json.Marshal(schema)
}
