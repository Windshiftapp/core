package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	v2 "windshift/internal/restapi/v2"
)

const defaultSpecPath = "api/openapi-v2.json"

func main() {
	specPath := flag.String("spec", defaultSpecPath, "v2 OpenAPI JSON path")
	check := flag.Bool("check", false, "fail when the checked document is stale")
	flag.Parse()

	original, err := os.ReadFile(*specPath)
	if err != nil {
		fail("read spec: %v", err)
	}
	generated, err := generate(original)
	if err != nil {
		fail("generate spec: %v", err)
	}
	if *check {
		if !bytes.Equal(original, generated) {
			fail("%s is stale; run go run ./scripts/openapi-v2-generate", *specPath)
		}
		fmt.Printf("API v2 OpenAPI document is reproducible (%d operations).\n", len(v2.BearerInventory()))
		return
	}
	// The checked API artifact must remain readable by packaging and CI users.
	if err := os.WriteFile(*specPath, generated, 0o644); err != nil { //nolint:gosec // Packaging requires a world-readable generated artifact.
		fail("write spec: %v", err)
	}
	fmt.Printf("Generated %s from canonical v2 route metadata.\n", *specPath)
}

func generate(data []byte) ([]byte, error) {
	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	paths := object(spec, "paths")
	components := object(spec, "components")
	schemas := object(components, "schemas")
	responses := object(components, "responses")
	ensureSharedResponses(responses)
	routes := v2.BearerInventory()
	allowed := make(map[string]bool, len(routes))
	removeGeneratedOperationSchemas(paths, schemas)
	for _, route := range routes {
		allowed[strings.ToLower(route.Method)+" "+route.Path] = true
	}

	for _, route := range routes {
		pathItem := object(paths, route.Path)
		operation := object(pathItem, strings.ToLower(route.Method))
		if len(operation) == 0 {
			for _, candidate := range []string{"get", "post", "put", "patch", "delete"} {
				if template, ok := pathItem[candidate].(map[string]any); ok && len(template) > 0 {
					operation = cloneObject(template)
					pathItem[strings.ToLower(route.Method)] = operation
					break
				}
			}
		}
		operationID := route.OperationID
		operation["operationId"] = operationID
		operation["summary"] = route.Summary
		operation["description"] = route.Description
		if route.Tag != "" {
			operation["tags"] = []any{route.Tag}
		}
		operation["parameters"] = parameterDocuments(route.Parameters)
		delete(pathItem, "parameters")
		if route.RequestType != nil {
			name := exportedName(operationID) + "Payload"
			schemas[name] = schemaFor(route.RequestType, make(map[reflect.Type]bool))
			operation["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{
					route.RequestMediaType: map[string]any{"schema": ref(name)},
				},
			}
		} else {
			delete(operation, "requestBody")
		}

		operationResponses := object(operation, "responses")
		if route.ResponseShape == v2.ResponseEmpty {
			removeSuccessResponses(operationResponses)
			operationResponses[strconv.Itoa(route.SuccessStatus)] = map[string]any{"description": "Request completed successfully with no response body."}
		} else if route.ResponseShape != v2.ResponseRaw && route.ResponseType != nil {
			name := exportedName(operationID) + "Result"
			schemas[name] = responseSchema(route)
			removeSuccessResponses(operationResponses)
			operationResponses[strconv.Itoa(route.SuccessStatus)] = map[string]any{
				"description": successDescription(operation),
				"content": map[string]any{
					route.ResponseMediaType: map[string]any{"schema": ref(name)},
				},
			}
		}
		ensureErrors(operationResponses, route)
		normalizeParameters(pathItem, operation)
	}
	removeStaleOperations(paths, allowed)

	delete(schemas, "DataResponse")
	delete(schemas, "PageResponse")
	delete(schemas, "MetadataResponse")
	delete(responses, "DataResponse")
	delete(responses, "PageResponse")
	delete(responses, "MetadataResponse")

	generated, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return append(generated, '\n'), nil
}

func removeGeneratedOperationSchemas(paths, schemas map[string]any) {
	for _, rawPath := range paths {
		path, ok := rawPath.(map[string]any)
		if !ok {
			continue
		}
		for method, rawOperation := range path {
			if !isHTTPMethod(method) {
				continue
			}
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				continue
			}
			operationID := stringValue(operation["operationId"])
			delete(schemas, exportedName(operationID)+"Request")
			delete(schemas, exportedName(operationID)+"Response")
			delete(schemas, exportedName(operationID)+"Payload")
			delete(schemas, exportedName(operationID)+"Result")
		}
	}
}

func removeStaleOperations(paths map[string]any, allowed map[string]bool) {
	for pathName, rawPath := range paths {
		path, ok := rawPath.(map[string]any)
		if !ok {
			continue
		}
		for method := range path {
			if isHTTPMethod(method) && !allowed[method+" "+pathName] {
				delete(path, method)
			}
		}
	}
}

func isHTTPMethod(method string) bool {
	switch method {
	case "get", "post", "put", "patch", "delete", "head", "options":
		return true
	default:
		return false
	}
}

func cloneObject(input map[string]any) map[string]any {
	encoded, _ := json.Marshal(input)
	var result map[string]any
	_ = json.Unmarshal(encoded, &result)
	return result
}

func parameterDocuments(parameters []v2.ParameterMetadata) []any {
	result := make([]any, 0, len(parameters))
	for _, parameter := range parameters {
		result = append(result, map[string]any{
			"name": parameter.Name, "in": parameter.In, "description": parameter.Description,
			"required": parameter.Required, "schema": parameter.Schema,
		})
	}
	return result
}

func responseSchema(route v2.Route) map[string]any {
	data := schemaFor(route.ResponseType, make(map[reflect.Type]bool))
	if route.ResponseShape == v2.ResponseDirect {
		if route.ResponseMediaType == "application/octet-stream" {
			return map[string]any{"type": "string", "format": "binary"}
		}
		return data
	}
	properties := map[string]any{}
	required := []string{"data"}
	switch route.ResponseShape {
	case v2.ResponseDocument:
		properties["data"] = data
	case v2.ResponsePage:
		properties["data"] = map[string]any{"type": "array", "items": data}
		properties["pagination"] = ref("Pagination")
		required = append(required, "pagination")
	case v2.ResponseMetadata:
		properties["data"] = data
		properties["meta"] = schemaFor(route.MetadataType, make(map[reflect.Type]bool))
		required = append(required, "meta")
	case v2.ResponsePageMetadata:
		properties["data"] = map[string]any{"type": "array", "items": data}
		properties["pagination"] = ref("Pagination")
		properties["meta"] = schemaFor(route.MetadataType, make(map[reflect.Type]bool))
		required = append(required, "pagination", "meta")
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}

func schemaFor(t reflect.Type, active map[reflect.Type]bool) map[string]any {
	if t == nil {
		return map[string]any{}
	}
	nullable := false
	for t.Kind() == reflect.Pointer {
		nullable = true
		t = t.Elem()
	}
	if isOptional(t) {
		value, _ := t.FieldByName("Value")
		schema := schemaFor(value.Type, active)
		schema["nullable"] = true
		return schema
	}
	if t.PkgPath() == "time" && t.Name() == "Time" {
		return nullableSchema(map[string]any{"type": "string", "format": "date-time"}, nullable)
	}
	if t.PkgPath() == "encoding/json" && t.Name() == "RawMessage" {
		return map[string]any{"description": "Arbitrary JSON extension value."}
	}
	if active[t] {
		return map[string]any{"type": "object", "description": "Nested resource reference."}
	}

	var schema map[string]any
	switch t.Kind() {
	case reflect.Interface:
		schema = map[string]any{"description": "Arbitrary JSON extension value."}
	case reflect.Bool:
		schema = map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		schema = map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		schema = map[string]any{"type": "number"}
	case reflect.String:
		schema = map[string]any{"type": "string"}
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			schema = map[string]any{"type": "string", "format": "byte"}
		} else {
			schema = map[string]any{"type": "array", "items": schemaFor(t.Elem(), active)}
		}
	case reflect.Map:
		additional := schemaFor(t.Elem(), active)
		schema = map[string]any{"type": "object", "additionalProperties": additional}
	case reflect.Struct:
		active[t] = true
		properties := make(map[string]any)
		required := make([]string, 0, t.NumField())
		for fieldIndex := range t.NumField() {
			field := t.Field(fieldIndex)
			if !field.IsExported() {
				continue
			}
			name, options := jsonField(field)
			if name == "-" {
				continue
			}
			if field.Anonymous && name == "" {
				embedded := schemaFor(field.Type, active)
				if embeddedProperties, ok := embedded["properties"].(map[string]any); ok {
					for key, value := range embeddedProperties {
						properties[key] = value
					}
				}
				if embeddedRequired, ok := embedded["required"].([]string); ok {
					required = append(required, embeddedRequired...)
				}
				continue
			}
			if name == "" {
				name = field.Name
			}
			propertySchema := schemaFor(field.Type, active)
			properties[name] = propertySchema
			applyFieldDescription(name, propertySchema)
			if !options["omitempty"] && field.Type.Kind() != reflect.Pointer && !isOptional(field.Type) {
				required = append(required, name)
			}
		}
		delete(active, t)
		slices.Sort(required)
		schema = map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
		if len(required) > 0 {
			schema["required"] = slices.Compact(required)
		}
	default:
		schema = map[string]any{}
	}
	return nullableSchema(schema, nullable)
}

func applyFieldDescription(name string, schema map[string]any) {
	schema["description"] = "The " + strings.ReplaceAll(name, "_", " ") + " value."
	switch name {
	case "is_task":
		schema["description"] = "Whether the item is a personal task. True is valid only in the authenticated caller's personal workspace and only with the Open or Done system status."
	case "ids":
		schema["description"] = "Resource IDs in preferred response order. At most 500 IDs are accepted; duplicates use their first occurrence and missing or invisible resources are omitted."
	}
}

func jsonField(field reflect.StructField) (name string, options map[string]bool) {
	parts := strings.Split(field.Tag.Get("json"), ",")
	options = make(map[string]bool, len(parts))
	for _, option := range parts[1:] {
		options[option] = true
	}
	return parts[0], options
}

func isOptional(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.PkgPath() == "windshift/internal/restapi/v2" && strings.HasPrefix(t.Name(), "Optional[")
}

func nullableSchema(schema map[string]any, nullable bool) map[string]any {
	if nullable {
		schema["nullable"] = true
	}
	return schema
}

func ensureSharedResponses(responses map[string]any) {
	responses["RateLimitError"] = map[string]any{
		"description": "The authenticated principal has too many concurrent API requests.",
		"headers": map[string]any{
			"Retry-After": map[string]any{
				"description": "Seconds to wait before retrying.",
				"schema":      map[string]any{"type": "integer", "minimum": 1},
			},
		},
		"content": map[string]any{"application/json": map[string]any{"schema": ref("ErrorDocument")}},
	}
	responses["PayloadTooLargeError"] = errorResponse("The request body exceeds the operation's documented size limit.")
	responses["UnsupportedMediaTypeError"] = errorResponse("The request Content-Type is not supported by this operation.")
}

func errorResponse(description string) map[string]any {
	return map[string]any{
		"description": description,
		"content":     map[string]any{"application/json": map[string]any{"schema": ref("ErrorDocument")}},
	}
}

func ensureErrors(responses map[string]any, route v2.Route) {
	for _, status := range route.DocumentedErrors {
		code := strconv.Itoa(status)
		if _, exists := responses[code]; exists {
			continue
		}
		switch status {
		case http.StatusBadRequest:
			responses[code] = responseRef("InvalidRequestError")
		case http.StatusUnauthorized:
			responses[code] = responseRef("AuthenticationError")
		case http.StatusForbidden:
			responses[code] = responseRef("PermissionError")
		case http.StatusNotFound:
			responses[code] = responseRef("NotFoundError")
		case http.StatusConflict:
			responses[code] = responseRef("ConflictError")
		case http.StatusTooManyRequests:
			responses[code] = responseRef("RateLimitError")
		case http.StatusInternalServerError:
			responses[code] = responseRef("InternalError")
		default:
			responses[code] = errorResponse(http.StatusText(status) + ".")
		}
	}
	if route.Auth == v2.AuthAuthenticated {
		responses["401"] = responseRef("AuthenticationError")
		responses["403"] = responseRef("PermissionError")
		responses["429"] = responseRef("RateLimitError")
	}
	responses["500"] = responseRef("InternalError")
	if route.RequestType != nil {
		responses["400"] = responseRef("InvalidRequestError")
		responses["413"] = responseRef("PayloadTooLargeError")
		responses["415"] = responseRef("UnsupportedMediaTypeError")
	}
	if strings.Contains(route.Path, "{") {
		if _, exists := responses["404"]; !exists {
			responses["404"] = responseRef("NotFoundError")
		}
	}
}

func normalizeParameters(pathItem, operation map[string]any) {
	for _, owner := range []map[string]any{pathItem, operation} {
		raw, ok := owner["parameters"].([]any)
		if !ok {
			continue
		}
		for _, entry := range raw {
			parameter, ok := entry.(map[string]any)
			if !ok || parameter["$ref"] != nil {
				continue
			}
			if strings.TrimSpace(stringValue(parameter["description"])) == "" {
				parameter["description"] = "Filters the operation by " + strings.ReplaceAll(stringValue(parameter["name"]), "_", " ") + "."
			}
		}
	}
}

func successDescription(operation map[string]any) string {
	summary := strings.TrimSuffix(strings.TrimSpace(stringValue(operation["summary"])), ".")
	if summary == "" {
		return "Successful response."
	}
	return summary + " response."
}

func removeSuccessResponses(responses map[string]any) {
	for status := range responses {
		if len(status) == 3 && status[0] == '2' {
			delete(responses, status)
		}
	}
}

func exportedName(value string) string {
	if value == "" {
		return "Operation"
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func ref(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func responseRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/responses/" + name}
}

func object(parent map[string]any, key string) map[string]any {
	value, ok := parent[key].(map[string]any)
	if !ok {
		value = make(map[string]any)
		parent[key] = value
	}
	return value
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "openapi-v2-generate: "+format+"\n", args...)
	os.Exit(1)
}
