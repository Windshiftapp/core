package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	v2 "windshift/internal/restapi/v2"
)

type document struct {
	OpenAPI    string                                `json:"openapi"`
	Tags       []tag                                 `json:"tags"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Parameters map[string]parameter       `json:"parameters"`
		Schemas    map[string]json.RawMessage `json:"schemas"`
		Responses  map[string]json.RawMessage `json:"responses"`
	} `json:"components"`
}

type tag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type operation struct {
	OperationID    string                `json:"operationId"`
	Tags           []string              `json:"tags"`
	Summary        string                `json:"summary"`
	Description    string                `json:"description"`
	Security       []map[string][]string `json:"security"`
	RequiredScopes []string              `json:"x-required-scopes"`
	Parameters     []parameter           `json:"parameters"`
	RequestBody    *requestBody          `json:"requestBody"`
	Responses      map[string]response   `json:"responses"`
}

type requestBody struct {
	Required bool                   `json:"required"`
	Content  map[string]mediaObject `json:"content"`
}

type response struct {
	Ref         string                 `json:"$ref"`
	Description string                 `json:"description"`
	Content     map[string]mediaObject `json:"content"`
}

type mediaObject struct {
	Schema json.RawMessage `json:"schema"`
}

type parameter struct {
	Ref         string          `json:"$ref"`
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Description string          `json:"description"`
	Required    bool            `json:"required"`
	Schema      json.RawMessage `json:"schema"`
}

func main() {
	specPath := flag.String("spec", "api/openapi-v2.json", "v2 OpenAPI JSON path")
	flag.Parse()

	data, err := os.ReadFile(*specPath)
	if err != nil {
		fail("read spec: %v", err)
	}
	var spec document
	if err := json.Unmarshal(data, &spec); err != nil {
		fail("decode spec: %v", err)
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		fail("openapi version %q is not 3.x", spec.OpenAPI)
	}
	loader := openapi3.NewLoader()
	document, err := loader.LoadFromData(data)
	if err != nil {
		fail("parse OpenAPI document: %v", err)
	}
	if err := document.Validate(context.Background()); err != nil {
		fail("validate OpenAPI document: %v", err)
	}
	declaredTags := validateTags(spec.Tags)

	want := make(map[string]v2.Route)
	for _, route := range v2.BearerInventory() {
		key := strings.ToLower(route.Method) + " " + route.Path
		want[key] = route
		item, ok := spec.Paths[route.Path]
		if !ok {
			fail("route %s is missing", key)
		}
		rawOperation, ok := item[strings.ToLower(route.Method)]
		if !ok {
			fail("route %s is missing", key)
		}
		var operation operation
		if err := json.Unmarshal(rawOperation, &operation); err != nil {
			fail("decode operation %s: %v", key, err)
		}
		validateDocumentation(route, operation, declaredTags)
		validateSecurity(route, operation)
		validateParameters(route, operation, item, spec.Components.Parameters)
		validateTransportContract(route, operation, spec.Components.Schemas)
	}
	if bytes.Contains(data, []byte("#/components/schemas/DataResponse")) ||
		bytes.Contains(data, []byte("#/components/schemas/PageResponse")) ||
		bytes.Contains(data, []byte("#/components/responses/DataResponse")) ||
		bytes.Contains(data, []byte("#/components/responses/PageResponse")) {
		fail("generic successful response contracts remain")
	}

	for path, item := range spec.Paths {
		for method := range item {
			if !isHTTPMethod(method) {
				continue
			}
			key := method + " " + path
			if _, ok := want[key]; !ok {
				fail("OpenAPI operation %s is not in the v2 inventory", key)
			}
		}
	}
	fmt.Printf("API v2 OpenAPI parity is valid (%d operations).\n", len(want))
}

func validateParameters(route v2.Route, operation operation, item map[string]json.RawMessage, components map[string]parameter) {
	parameters := make([]parameter, 0, len(operation.Parameters)+4)
	if raw := item["parameters"]; len(raw) > 0 {
		var inherited []parameter
		if err := json.Unmarshal(raw, &inherited); err != nil {
			fail("decode path parameters for %s: %v", route.Path, err)
		}
		parameters = append(parameters, inherited...)
	}
	parameters = append(parameters, operation.Parameters...)

	declaredPath := make(map[string]bool)
	resolvedParameters := make([]parameter, 0, len(parameters))
	for _, candidate := range parameters {
		resolved := candidate
		if candidate.Ref != "" {
			const prefix = "#/components/parameters/"
			if !strings.HasPrefix(candidate.Ref, prefix) {
				fail("route %s %s parameter has unsupported reference %q", route.Method, route.Path, candidate.Ref)
			}
			var ok bool
			resolved, ok = components[strings.TrimPrefix(candidate.Ref, prefix)]
			if !ok {
				fail("route %s %s parameter reference %q is missing", route.Method, route.Path, candidate.Ref)
			}
		}
		if strings.TrimSpace(resolved.Description) == "" {
			fail("route %s %s parameter %q has no description", route.Method, route.Path, resolved.Name)
		}
		if len(resolved.Schema) == 0 {
			fail("route %s %s parameter %q has no schema", route.Method, route.Path, resolved.Name)
		}
		if resolved.In == "path" {
			declaredPath[resolved.Name] = true
		}
		resolvedParameters = append(resolvedParameters, resolved)
	}
	if len(resolvedParameters) != len(route.Parameters) {
		fail("route %s %s parameter metadata count differs: route=%d document=%d", route.Method, route.Path, len(route.Parameters), len(resolvedParameters))
	}
	for index, documented := range resolvedParameters {
		metadata := route.Parameters[index]
		var documentedSchema map[string]any
		if err := json.Unmarshal(documented.Schema, &documentedSchema); err != nil {
			fail("route %s %s parameter %q schema is invalid: %v", route.Method, route.Path, documented.Name, err)
		}
		documentedJSON, _ := json.Marshal(documentedSchema)
		metadataJSON, _ := json.Marshal(metadata.Schema)
		if documented.Name != metadata.Name || documented.In != metadata.In || documented.Description != metadata.Description || documented.Required != metadata.Required || !bytes.Equal(documentedJSON, metadataJSON) {
			fail("route %s %s parameter %q differs from canonical route metadata", route.Method, route.Path, documented.Name)
		}
	}

	for _, part := range strings.Split(route.Path, "/") {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			if !declaredPath[name] {
				fail("route %s %s does not declare path parameter %q", route.Method, route.Path, name)
			}
		}
	}
}

func validateTransportContract(route v2.Route, operation operation, schemas map[string]json.RawMessage) {
	if route.RequestType == nil {
		if operation.RequestBody != nil {
			fail("route %s %s declares a request body not present in route metadata", route.Method, route.Path)
		}
	} else {
		if operation.RequestBody == nil || !operation.RequestBody.Required {
			fail("route %s %s must declare its required request body", route.Method, route.Path)
		}
		media, ok := operation.RequestBody.Content[route.RequestMediaType]
		if !ok || len(media.Schema) == 0 {
			fail("route %s %s must declare request media type %q with a schema", route.Method, route.Path, route.RequestMediaType)
		}
		name := exportedOperationName(operation.OperationID) + "Payload"
		if _, ok := schemas[name]; !ok {
			fail("route %s %s generated request schema %q is missing", route.Method, route.Path, name)
		}
	}

	if route.SuccessStatus == 0 {
		fail("route %s %s has no success status metadata", route.Method, route.Path)
	}
	status := strconv.Itoa(route.SuccessStatus)
	success, ok := operation.Responses[status]
	if !ok {
		fail("route %s %s does not declare metadata success status %s", route.Method, route.Path, status)
	}
	for candidate := range operation.Responses {
		if len(candidate) == 3 && candidate[0] == '2' && candidate != status {
			fail("route %s %s declares unexpected success status %s", route.Method, route.Path, candidate)
		}
	}
	if route.ResponseShape == v2.ResponseEmpty {
		if len(success.Content) != 0 || success.Ref != "" {
			fail("route %s %s must not declare a success body", route.Method, route.Path)
		}
		return
	}
	media, ok := success.Content[route.ResponseMediaType]
	if !ok || len(media.Schema) == 0 {
		fail("route %s %s must declare response media type %q with a schema", route.Method, route.Path, route.ResponseMediaType)
	}
	name := exportedOperationName(operation.OperationID) + "Result"
	if _, ok := schemas[name]; !ok {
		fail("route %s %s generated response schema %q is missing", route.Method, route.Path, name)
	}
	if route.Auth == v2.AuthAuthenticated {
		for _, errorStatus := range []string{"401", "403", "429", "500"} {
			if _, ok := operation.Responses[errorStatus]; !ok {
				fail("route %s %s does not declare shared %s behavior", route.Method, route.Path, errorStatus)
			}
		}
	}
}

func exportedOperationName(value string) string {
	if value == "" {
		return "Operation"
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func validateTags(tags []tag) map[string]struct{} {
	declared := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			fail("OpenAPI tag name is empty")
		}
		if strings.TrimSpace(tag.Description) == "" {
			fail("OpenAPI tag %q has no description", name)
		}
		if _, exists := declared[name]; exists {
			fail("OpenAPI tag %q is declared more than once", name)
		}
		declared[name] = struct{}{}
	}
	if len(declared) == 0 {
		fail("OpenAPI document has no declared tags")
	}
	return declared
}

func validateDocumentation(route v2.Route, operation operation, declaredTags map[string]struct{}) {
	if len(operation.Tags) != 1 {
		fail("route %s %s must declare exactly one domain tag", route.Method, route.Path)
	}
	if _, ok := declaredTags[operation.Tags[0]]; !ok {
		fail("route %s %s uses undeclared tag %q", route.Method, route.Path, operation.Tags[0])
	}
	summary := strings.TrimSpace(operation.Summary)
	if summary == "" {
		fail("route %s %s has no summary", route.Method, route.Path)
	}
	if len(summary) > 80 {
		fail("route %s %s summary is longer than 80 characters", route.Method, route.Path)
	}
	if strings.TrimSpace(operation.Description) == "" {
		fail("route %s %s has no description", route.Method, route.Path)
	}
	if operation.OperationID != route.OperationID || operation.Tags[0] != route.Tag || operation.Summary != route.Summary || operation.Description != route.Description {
		fail("route %s %s documentation differs from canonical route metadata", route.Method, route.Path)
	}
	for _, code := range route.DocumentedErrors {
		if _, ok := operation.Responses[strconv.Itoa(code)]; !ok {
			fail("route %s %s does not declare metadata error %d", route.Method, route.Path, code)
		}
	}
}

func validateSecurity(route v2.Route, operation operation) {
	if route.Auth == v2.AuthPublic {
		if len(operation.Security) != 0 {
			fail("public route %s %s declares security", route.Method, route.Path)
		}
		return
	}
	if len(operation.Security) != 1 {
		fail("authenticated route %s %s must declare one security requirement", route.Method, route.Path)
	}
	scopes, ok := operation.Security[0]["BearerAuth"]
	if !ok || len(scopes) != 0 {
		fail("route %s %s must use an empty scope array for its HTTP bearer security scheme", route.Method, route.Path)
	}
	if strings.Join(operation.RequiredScopes, "\x00") != strings.Join(route.Scopes, "\x00") {
		fail("route %s %s x-required-scopes do not match its inventory", route.Method, route.Path)
	}
}

func isHTTPMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "openapi-v2-check: "+format+"\n", args...)
	os.Exit(1)
}
