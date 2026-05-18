package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"windshift/internal/aitools"
	"windshift/internal/database"
	"windshift/internal/llm"
	"windshift/internal/services"
)

// ToolExecutor executes tool calls on behalf of the agentic chat loop.
// It enforces workspace access via a pre-computed list of accessible workspace IDs.
type ToolExecutor struct {
	db                     database.Database
	accessibleWorkspaceIDs []int
	userID                 int
	timePermService        *services.TimePermissionService
	permService            *services.PermissionService
	commentService         *services.CommentService
	timerService           *services.TimerService
	actionService          *services.ActionService
}

// NewToolExecutor creates a tool executor scoped to the given user's accessible workspaces.
func NewToolExecutor(
	db database.Database,
	accessibleWorkspaceIDs []int,
	userID int,
	timePermService *services.TimePermissionService,
	permService *services.PermissionService,
	commentService *services.CommentService,
	timerService *services.TimerService,
	actionService *services.ActionService,
) *ToolExecutor {
	return &ToolExecutor{
		db:                     db,
		accessibleWorkspaceIDs: accessibleWorkspaceIDs,
		userID:                 userID,
		timePermService:        timePermService,
		permService:            permService,
		commentService:         commentService,
		timerService:           timerService,
		actionService:          actionService,
	}
}

// Execute dispatches a tool call by name and returns the JSON result.
// All tools live in the shared internal/aitools/ registry; this is now
// just protocol glue.
func (e *ToolExecutor) Execute(ctx context.Context, name, arguments string) (string, error) {
	entry, ok := aitools.Default.Lookup(name)
	if !ok {
		return `{"error": "unknown tool"}`, nil
	}
	return e.runRegistryTool(ctx, entry, arguments)
}

// env builds the aitools.Env that registry-driven tools consume. The
// executor's pre-computed accessibleWorkspaceIDs and injected services
// are reused so behavior matches the legacy in-line handlers exactly.
func (e *ToolExecutor) env() *aitools.Env {
	return &aitools.Env{
		DB:                     e.db,
		UserID:                 e.userID,
		AccessibleWorkspaceIDs: e.accessibleWorkspaceIDs,
		PermService:            e.permService,
		TimePermService:        e.timePermService,
		TimerService:           e.timerService,
		CommentService:         e.commentService,
		ActionService:          e.actionService,
	}
}

// runRegistryTool unmarshals raw JSON args into the entry's typed Args,
// invokes Run, and marshals the result. Errors are returned as a tool
// error JSON object so the agent loop can surface them to the model.
func (e *ToolExecutor) runRegistryTool(ctx context.Context, entry aitools.Entry, arguments string) (string, error) {
	args := entry.NewArgs()
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), args); err != nil {
			return `{"error": "invalid arguments"}`, nil
		}
	}
	out, err := entry.Run(ctx, e.env(), args)
	if err != nil {
		b, _ := json.Marshal(map[string]string{"error": err.Error()})
		return string(b), nil
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshal tool result: %w", err)
	}
	return string(b), nil
}

// BuildLLMTools merges the legacy hand-written tool definitions in
// llm.BuiltinTools() with the typed registry in aitools.Default. Every
// registry entry is exposed to the agent loop with the JSON Schema
// derived from its Args struct.
func BuildLLMTools() []llm.ToolDefinition {
	out := llm.BuiltinTools()
	for _, e := range aitools.Default.All() {
		out = append(out, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        e.Name,
				Description: e.Description,
				Parameters:  e.Schema,
			},
		})
	}
	return out
}
