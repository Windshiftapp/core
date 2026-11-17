package services

import (
	"windshift/internal/models"
	"windshift/internal/operatorcontrols"
)

// NodeExecutor runs a single action node. New node types should be added as
// standalone NodeExecutor implementations registered via
// ActionService.RegisterNodeExecutor at startup, rather than by extending
// the giant switch in executeNode. Each executor is free to hold its own
// narrow deps in its struct fields, which keeps ActionService from being
// the dumping ground for every collaborator.
//
// Existing built-in node types (set_field, set_status, …) still live as
// methods on ActionService; they will be migrated to the registry pattern
// when there is a reason to touch them. Until then the registry is opt-in:
// executeNode consults the registry first and falls through to the legacy
// switch on miss.
type NodeExecutor interface {
	// NodeType identifies which ActionNodeType this executor handles.
	// One executor per type; registering a second for the same type
	// replaces the first.
	NodeType() models.ActionNodeType

	// Execute runs the node. Return non-nil error to fail the action;
	// stepResult.Output is the executor's per-step audit payload.
	Execute(node *models.ActionNode, ctx *models.ExecutionContext, stepResult *models.StepResult) error
}

// NodeAPI is the narrow service surface a NodeExecutor can use without
// having to import the whole ActionService. Methods are added here only
// when a registered executor actually needs them. ActionService satisfies
// this interface so a real executor receives the live service; tests can
// pass a fake.
type NodeAPI interface {
	// SubstituteVariables renders {{var}} placeholders against the
	// execution context's Variables map. Executors call this on every
	// user-supplied template field (names, descriptions, URLs, etc.).
	SubstituteVariables(template string, ctx *models.ExecutionContext) string

	// EmitActionEvent queues a follow-up event for the action engine.
	// Use sparingly — emitting from a node creates a cascade that is
	// subject to depth limits enforced by the engine.
	EmitActionEvent(event *models.ActionEvent)

	// AuthorizeWorkspaceMutation verifies the effective actor may mutate
	// items in the workspace with the given permission. Executors that
	// change item state must call it before writing.
	AuthorizeWorkspaceMutation(actorUserID, workspaceID int, permission string) error
}

// RegisterNodeExecutor wires an executor into the per-service registry.
// Safe to call multiple times for different node types at startup; calling
// it twice for the same type replaces the prior registration (last call
// wins, intentional so tests can swap an executor for a stub).
func (as *ActionService) RegisterNodeExecutor(e NodeExecutor) {
	// Bind a private copy to this instance's policy, including alternate service callers.
	if script, ok := e.(*ExecuteScriptExecutor); ok {
		bound := *script
		bound.controls = operatorcontrols.From(as.db)
		e = &bound
	}
	as.nodeExecMu.Lock()
	defer as.nodeExecMu.Unlock()
	if as.nodeExecutors == nil {
		as.nodeExecutors = make(map[models.ActionNodeType]NodeExecutor)
	}
	as.nodeExecutors[e.NodeType()] = e
}

// lookupNodeExecutor returns the registered executor for a node type, if
// any. Snapshot read under the lock so a concurrent registration can't
// race with execution dispatch.
func (as *ActionService) lookupNodeExecutor(t models.ActionNodeType) (NodeExecutor, bool) {
	as.nodeExecMu.RLock()
	defer as.nodeExecMu.RUnlock()
	if as.nodeExecutors == nil {
		return nil, false
	}
	e, ok := as.nodeExecutors[t]
	return e, ok
}

// SubstituteVariables is the public wrapper around the (lowercase)
// implementation so registered NodeExecutors can reuse the engine's
// variable substitution without ActionService having to expose its
// private method to outside packages.
func (as *ActionService) SubstituteVariables(template string, ctx *models.ExecutionContext) string {
	return as.substituteVariables(template, ctx)
}
