// Package aitooladapter contains protocol-neutral glue between the canonical
// executable aitools registry and llm.RunAgent.
package aitooladapter

import (
	"context"
	"encoding/json"
	"fmt"

	"windshift/internal/aitools"
	"windshift/internal/auth"
	"windshift/internal/llm"
)

// Executor dispatches only the entries admitted when it was constructed.
type Executor struct {
	env     *aitools.Env
	allowed map[string]aitools.Entry
}

func NewExecutor(env *aitools.Env, entries []aitools.Entry) *Executor {
	allowed := make(map[string]aitools.Entry, len(entries))
	for _, entry := range entries {
		if !entry.AvailableFor(env.Source) {
			continue
		}
		allowed[entry.Name] = entry
	}
	return &Executor{env: env, allowed: allowed}
}

func (e *Executor) Execute(ctx context.Context, name, arguments string) (string, error) {
	entry, ok := e.allowed[name]
	if !ok {
		return `{"error":"tool is not available to this agent"}`, nil
	}
	args := entry.NewArgs()
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), args); err != nil {
			return `{"error":"invalid arguments"}`, nil //nolint:nilerr // Preserve recoverable tool-result semantics.
		}
	}
	out, err := entry.Run(ctx, e.env, args)
	if err != nil {
		body, _ := json.Marshal(map[string]string{"error": err.Error()})
		return string(body), nil //nolint:nilerr // Preserve recoverable tool-result semantics.
	}
	body, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshal tool result: %w", err)
	}
	return string(body), nil
}

// EntriesForStandard returns the executable safe subset for the mandatory
// Read/comment preset plus selected groups.
func EntriesForStandard(registry *aitools.Registry, selected []string) []aitools.Entry {
	groups := map[aitools.CapabilityGroup]bool{aitools.CapabilityReadComment: true}
	for _, group := range selected {
		groups[aitools.CapabilityGroup(group)] = true
	}
	var out []aitools.Entry
	for _, entry := range registry.All() {
		if !entry.AvailableFor(aitools.SourceStandardAgent) || !groups[entry.Group] || entry.Access == aitools.AccessDestructive || entry.Access == aitools.AccessAdmin {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// EntriesForScopes returns the registry entries whose declared token scopes
// are all satisfied by the given scope set. The in-product chat authenticates
// with a cookie rather than a token, so passing auth.DefaultAgentScopes here
// is what keeps its capability surface equal to the one a `ws` CLI or MCP
// token gets by default instead of the entire registry (WI-962).
func EntriesForScopes(registry *aitools.Registry, scopes []string) []aitools.Entry {
	all := registry.All()
	out := make([]aitools.Entry, 0, len(all))
	for _, entry := range all {
		if !entry.AvailableFor(aitools.SourceAIChat) || !auth.ScopesSatisfy(scopes, entry.Scopes) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func BuildTools(entries []aitools.Entry) []llm.ToolDefinition {
	out := make([]llm.ToolDefinition, 0, len(entries))
	for _, entry := range entries {
		out = append(out, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        entry.Name,
				Description: entry.Description,
				Parameters:  entry.Schema,
			},
		})
	}
	return out
}

func TerminalTools(entries []aitools.Entry) map[string]bool {
	out := make(map[string]bool)
	for _, entry := range entries {
		if entry.Access == aitools.AccessWrite {
			out[entry.Name] = true
		}
	}
	return out
}
