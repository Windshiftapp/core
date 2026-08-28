package services

import (
	"reflect"
	"strings"
	"testing"

	"windshift/internal/models"
)

func TestResolveExecutionValue(t *testing.T) {
	assigneeID := 0
	service := &ActionService{}
	ctx := &models.ExecutionContext{
		Item: &models.Item{
			ID:           42,
			StatusName:   "In Progress",
			PriorityName: "High",
			AssigneeID:   &assigneeID,
		},
		Actor: &models.User{ID: 7, FirstName: "Ada", LastName: "Lovelace", Email: "ada@example.com"},
		Variables: map[string]any{
			"classification": map[string]any{
				"project_id": float64(12),
				"labels":     []any{"backend", map[string]any{"name": "urgent"}},
				"nullable":   nil,
			},
			"classification.project_id": "exact",
			"old_metadata":              map[string]any{"owner": "before"},
			"new_ref.short":             "main",
			"known":                     "trigger value",
			"old":                       map[string]any{"status": "wrong fallback"},
			"trigger":                   map[string]any{"unknown": "wrong fallback"},
		},
	}

	tests := []struct {
		name  string
		path  string
		want  any
		found bool
	}{
		{name: "exact dotted variable wins", path: "classification.project_id", want: "exact", found: true},
		{name: "composite value", path: "classification.labels", want: []any{"backend", map[string]any{"name": "urgent"}}, found: true},
		{name: "nested slice and map", path: "classification.labels.1.name", want: "urgent", found: true},
		{name: "known null", path: "classification.nullable", want: nil, found: true},
		{name: "item id", path: "item.id", want: 42, found: true},
		{name: "item status name", path: "item.status", want: "In Progress", found: true},
		{name: "item priority name", path: "item.priority", want: "High", found: true},
		{name: "item known zero", path: "item.assignee_id", want: 0, found: true},
		{name: "user id", path: "user.id", want: 7, found: true},
		{name: "trigger value", path: "trigger.known", want: "trigger value", found: true},
		{name: "old nested value", path: "old.metadata.owner", want: "before", found: true},
		{name: "SCM dotted key", path: "ref.short", want: "main", found: true},
		{name: "old namespace does not fall through", path: "old.status", found: false},
		{name: "trigger namespace does not fall through", path: "trigger.unknown", found: false},
		{name: "missing map key", path: "classification.unknown", found: false},
		{name: "invalid slice index", path: "classification.labels.first", found: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := service.resolveExecutionValue(ctx, tt.path)
			if found != tt.found {
				t.Fatalf("resolveExecutionValue(%q) found = %v, want %v", tt.path, found, tt.found)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resolveExecutionValue(%q) = %#v, want %#v", tt.path, got, tt.want)
			}
		})
	}
}

func TestSubstituteVariablesUsesResolverAndKeepsMissingPlaceholders(t *testing.T) {
	service := &ActionService{}
	ctx := &models.ExecutionContext{Variables: map[string]any{
		"classification": map[string]any{
			"project_id": 12,
			"labels":     []any{"backend", "urgent"},
			"nullable":   nil,
		},
	}}

	got := service.substituteVariables("project={{classification.project_id}} labels={{classification.labels}} null={{classification.nullable}} missing={{classification.owner}}", ctx)
	want := `project=12 labels=["backend","urgent"] null= missing={{classification.owner}}`
	if got != want {
		t.Fatalf("substituteVariables() = %q, want %q", got, want)
	}
}

func TestStringifyExecutionValueUsesJSONForStructuredValues(t *testing.T) {
	got, err := stringifyExecutionValue(map[string]any{"project_id": 12}, "null")
	if err != nil {
		t.Fatalf("stringifyExecutionValue() error = %v", err)
	}
	if got != `{"project_id":12}` {
		t.Fatalf("stringifyExecutionValue() = %q, want JSON object", got)
	}
}

func TestBuildAIAgentUserMessageResolvesNestedValues(t *testing.T) {
	service := &ActionService{}
	ctx := &models.ExecutionContext{Variables: map[string]any{
		"classification": map[string]any{"project_id": 12, "nullable": nil},
	}}

	message, err := service.buildAIAgentUserMessage(ctx, []string{"classification.project_id", "classification.nullable"})
	if err != nil {
		t.Fatalf("buildAIAgentUserMessage() error = %v", err)
	}
	if !strings.Contains(message, `<input field="classification.project_id" trust="untrusted">12</input>`) {
		t.Fatalf("buildAIAgentUserMessage() missing nested value: %q", message)
	}
	if !strings.Contains(message, `<input field="classification.nullable" trust="untrusted">null</input>`) {
		t.Fatalf("buildAIAgentUserMessage() missing known null value: %q", message)
	}
}

func TestBuildAIAgentUserMessageRejectsMissingInput(t *testing.T) {
	service := &ActionService{}
	ctx := &models.ExecutionContext{Variables: map[string]any{}}

	_, err := service.buildAIAgentUserMessage(ctx, []string{"classification.project_id"})
	if err == nil || !strings.Contains(err.Error(), `input field "classification.project_id" not found`) {
		t.Fatalf("buildAIAgentUserMessage() error = %v, want missing input error", err)
	}
}
