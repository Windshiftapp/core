package services

import (
	"context"

	"windshift/internal/models"
)

// ActionScriptProvenance carries the trusted automation context for REST calls
// a script makes. The execute_script bridge stamps it on the in-process request
// so the domain services that emit action events keep the same cascade depth,
// execution chain, and private-trigger provenance as the node that caused them.
// It is server-side metadata: scripts cannot set, read, or override it, and it
// is what keeps a script-driven mutation from resetting the cascade depth to
// zero and looping forever.
type ActionScriptProvenance struct {
	ActionID                int
	ExecutionLogID          int
	NodeID                  int
	DefinitionRevisionID    *int
	ExecutionChainID        string
	CascadeDepth            int
	TriggerCommentIsPrivate bool
	SourceApplication       string
}

type actionScriptProvenanceKey struct{}

// WithActionScriptProvenance attaches trusted provenance to a request context.
func WithActionScriptProvenance(ctx context.Context, p ActionScriptProvenance) context.Context {
	return context.WithValue(ctx, actionScriptProvenanceKey{}, p)
}

// ActionScriptProvenanceFrom returns the trusted provenance, if the request
// originated from an execute_script node.
func ActionScriptProvenanceFrom(ctx context.Context) (ActionScriptProvenance, bool) {
	if ctx == nil {
		return ActionScriptProvenance{}, false
	}
	p, ok := ctx.Value(actionScriptProvenanceKey{}).(ActionScriptProvenance)
	return p, ok
}

// ActionContext renders the provenance as the cascade context the domain
// services already understand, so script side effects stay inside the action
// cascade budget instead of looking like a fresh user action.
func (p ActionScriptProvenance) ActionContext() ActionContext {
	source := p.SourceApplication
	if source == "" {
		source = "automation"
	}
	return ActionContext{
		TriggeredByAction:       true,
		ExecutionChainID:        p.ExecutionChainID,
		CascadeDepth:            p.CascadeDepth,
		SourceApplication:       source,
		TriggerCommentIsPrivate: p.TriggerCommentIsPrivate,
	}
}

// provenanceFromExecution builds the provenance for a node in the given
// execution. The script's REST calls are one cascade step below the trigger
// that started the action, matching how in-process node mutations are stamped.
func provenanceFromExecution(ctx *models.ExecutionContext, nodeID int) ActionScriptProvenance {
	p := ActionScriptProvenance{NodeID: nodeID}
	if ctx == nil {
		return p
	}
	if ctx.Action != nil {
		p.ActionID = ctx.Action.ID
	}
	p.ExecutionLogID = ctx.ExecutionLogID
	p.DefinitionRevisionID = ctx.DefinitionRevisionID
	p.ExecutionChainID = ctx.ChainID
	p.TriggerCommentIsPrivate = ctx.TriggerCommentIsPrivate
	p.SourceApplication = "automation"
	if ctx.Event != nil {
		p.CascadeDepth = ctx.Event.CascadeDepth + 1
	}
	return p
}
