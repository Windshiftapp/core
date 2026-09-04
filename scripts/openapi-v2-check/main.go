package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	v2 "windshift/internal/restapi/v2"
)

type document struct {
	OpenAPI    string                                `json:"openapi"`
	Tags       []tag                                 `json:"tags"`
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Parameters map[string]parameter `json:"parameters"`
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
}

type parameter struct {
	Ref         string `json:"$ref"`
	Name        string `json:"name"`
	In          string `json:"in"`
	Description string `json:"description"`
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
	for _, route := range v2.Inventory() {
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
		if resolved.In == "path" {
			declaredPath[resolved.Name] = true
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
