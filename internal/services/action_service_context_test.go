package services

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

type contextInputProbe struct {
	service *ActionService
	message string
}

func (p *contextInputProbe) NodeType() models.ActionNodeType { return models.ActionNodeAIAgent }
func (p *contextInputProbe) Execute(_ *models.ActionNode, ctx *models.ExecutionContext, _ *models.StepResult) error {
	message, err := p.service.buildAIAgentUserMessage(ctx, []string{"item.id", "item.status", "item.priority", "item.custom_field_17", "user.id", "user.name"})
	p.message = message
	if err != nil {
		return err
	}
	if ctx.Item != nil {
		return fmt.Errorf("top-level context must not retain a stale item snapshot")
	}
	if _, err := p.service.db.Exec(`UPDATE items SET status_id = (SELECT id FROM statuses WHERE name = 'Context updated') WHERE id = ?`, ctx.Event.ItemID); err != nil {
		return err
	}
	// The next input must observe a preceding step's mutation. An iterator
	// snapshot must also preserve identity without pinning joined status names.
	for _, snapshot := range []*models.Item{nil, {ID: ctx.Event.ItemID, StatusName: "Old"}} {
		current := *ctx
		current.Item = snapshot
		value, found := p.service.resolveExecutionValue(&current, "item.status")
		if !found || value != "Context updated" {
			return fmt.Errorf("status after mutation = %v, found=%v", value, found)
		}
	}
	return nil
}

func TestActionExecutionResolvesTriggerItemAndEffectiveActor(t *testing.T) {
	db, err := database.NewSQLiteDB(filepath.Join(t.TempDir(), "action-context.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO users (id, email, username, first_name, last_name) VALUES (7, 'trigger@example.test', 'trigger', 'Trigger', 'User'), (8, 'effective@example.test', 'effective', 'Effective', 'User')`,
		`INSERT INTO workspaces (id, name, key) VALUES (42, 'Context', 'CTX')`,
		`INSERT INTO items (id, workspace_id, workspace_item_number, title, description, frac_index, status_id, priority_id, custom_field_values) VALUES (42, 42, 1, 'Context item', '', 'a0', (SELECT id FROM statuses LIMIT 1), (SELECT id FROM priorities LIMIT 1), '{"17":"custom context"}')`,
		`INSERT INTO actions (id, workspace_id, name, trigger_type, is_enabled) VALUES (42, 42, 'Context action', 'manual', true)`,
		`INSERT INTO statuses (name, category_id) VALUES ('Context updated', (SELECT id FROM status_categories LIMIT 1))`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	service := &ActionService{db: db, repo: repository.NewActionRepository(db), itemRepo: repository.NewItemRepository(db), chainStore: NewExecutionChainStore()}
	probe := &contextInputProbe{service: service}
	service.RegisterNodeExecutor(probe)
	actorID := 8
	action := &models.Action{ID: 42, WorkspaceID: 42, IsEnabled: true, TriggerType: models.ActionTriggerManual, ActorUserID: &actorID, Nodes: []models.ActionNode{{ID: 1, NodeType: models.ActionNodeAIAgent, NodeConfig: `{}`}}}
	if err := service.executeAction(action, &models.ActionEvent{ItemID: 42, WorkspaceID: 42, ActorUserID: 7}, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<input field="item.id" trust="untrusted">42</input>`, `custom context`, `<input field="user.id" trust="untrusted">8</input>`, `Effective User`} {
		if !strings.Contains(probe.message, want) {
			t.Errorf("message missing %q: %s", want, probe.message)
		}
	}
}

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
