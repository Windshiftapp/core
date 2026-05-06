package aitools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

// Tool is the canonical definition of a single AI tool. Args is a typed
// struct (with json/jsonschema tags) describing the parameters; Run
// implements the actual business logic and returns whatever the tool's
// JSON response should marshal to (typically a struct or map).
//
// The Run function does not parse JSON or marshal a response — adapters
// handle the wire format. Errors returned from Run surface as tool errors
// in both protocols.
type Tool[Args any] struct {
	Name        string
	Description string
	Run         func(ctx context.Context, env *Env, args Args) (any, error)
}

// Entry is the type-erased view of a registered Tool, suitable for use
// by adapters that don't know the Args type at compile time. Adapters
// receive these via Registry.All() / Registry.Lookup().
type Entry struct {
	Name        string
	Description string
	// Schema is the JSON Schema for the tool's Args, derived from the
	// typed Args struct via jsonschema.For. Pre-computed at register time.
	Schema json.RawMessage
	// NewArgs allocates a fresh zero-valued *Args so the adapter can
	// json.Unmarshal arguments into it.
	NewArgs func() any
	// Run dispatches into the typed Tool.Run after the adapter has filled
	// the args pointer returned by NewArgs.
	Run func(ctx context.Context, env *Env, args any) (any, error)
}

// Registry holds the registered tools, keyed by name. Default is the
// process-wide registry; package init functions in aitools/*.go register
// into it so adapters see one consistent set.
type Registry struct {
	entries map[string]Entry
	order   []string
}

// Default is the process-wide registry. Tools register into it via
// package-level init functions in this package's other files.
var Default = &Registry{entries: map[string]Entry{}}

// Register adds a typed Tool to the registry. The Args type's JSON
// Schema is computed once here and cached on the Entry so adapters
// don't pay the reflection cost per request.
func Register[Args any](r *Registry, t Tool[Args]) {
	if t.Name == "" {
		panic("aitools: tool name is required")
	}
	if _, exists := r.entries[t.Name]; exists {
		panic("aitools: duplicate tool name: " + t.Name)
	}

	schema, err := jsonschema.For[Args](nil)
	if err != nil {
		panic(fmt.Sprintf("aitools: build schema for %s: %v", t.Name, err))
	}
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("aitools: marshal schema for %s: %v", t.Name, err))
	}

	entry := Entry{
		Name:        t.Name,
		Description: t.Description,
		Schema:      schemaJSON,
		NewArgs:     func() any { return new(Args) },
		Run: func(ctx context.Context, env *Env, args any) (any, error) {
			typed, ok := args.(*Args)
			if !ok {
				return nil, fmt.Errorf("aitools: args type mismatch for %s", t.Name)
			}
			return t.Run(ctx, env, *typed)
		},
	}
	r.entries[t.Name] = entry
	r.order = append(r.order, t.Name)
}

// All returns every registered tool in registration order.
func (r *Registry) All() []Entry {
	out := make([]Entry, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.entries[name])
	}
	return out
}

// Lookup returns the entry for name and whether it was found.
func (r *Registry) Lookup(name string) (Entry, bool) {
	e, ok := r.entries[name]
	return e, ok
}
