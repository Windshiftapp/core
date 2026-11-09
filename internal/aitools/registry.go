package aitools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/jsonschema-go/jsonschema"

	"windshift/internal/agentstudio"
	"windshift/internal/toolschema"
)

// CapabilityGroup is the stable Agent Studio grouping for Standard-agent
// tool selection. It lives on the executable registry entry so the profile UI,
// prompt schema, and runtime cannot drift onto separate tool catalogs.
type CapabilityGroup = agentstudio.CapabilityGroup

const (
	CapabilityReadComment       = agentstudio.CapabilityReadComment
	CapabilityIssueManagement   = agentstudio.CapabilityIssueManagement
	CapabilityCommentEditing    = agentstudio.CapabilityCommentEditing
	CapabilityPlanningActivity  = agentstudio.CapabilityPlanningActivity
	CapabilityKnowledgeDiagrams = agentstudio.CapabilityKnowledgeDiagrams
	CapabilityActions           = agentstudio.CapabilityActions
	CapabilityTests             = agentstudio.CapabilityTests
	CapabilityTime              = agentstudio.CapabilityTime
	CapabilityUsersApprovals    = agentstudio.CapabilityUsersApprovals
)

// AccessLevel describes the strongest resource effect a tool can have.
type AccessLevel string

const (
	AccessRead        AccessLevel = "read"
	AccessWrite       AccessLevel = "write"
	AccessDestructive AccessLevel = "destructive"
	AccessAdmin       AccessLevel = "admin"
)

// RiskLevel is the stable policy signal used by Studio and audit summaries.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
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
	Group       CapabilityGroup
	Access      AccessLevel
	Risk        RiskLevel
	// Scopes lists the API-token scopes (auth.Scope* constants) required to
	// invoke this tool from a token-authenticated adapter — i.e. the MCP
	// server, which gates each call on the bearer token's scope set. All
	// listed scopes must be present (write implies read, per
	// auth.TokenManager.CheckTokenPermissions). The cookie-authenticated
	// in-product chat has no token and ignores this field entirely; the
	// per-user workspace permission checks inside Run remain the only gate
	// on that surface.
	Scopes []string
	Run    func(ctx context.Context, env *Env, args Args) (any, error)
}

// Entry is the type-erased view of a registered Tool, suitable for use
// by adapters that don't know the Args type at compile time. Adapters
// receive these via Registry.All() / Registry.Lookup().
type Entry struct {
	Name        string
	Description string
	Group       CapabilityGroup
	Access      AccessLevel
	Risk        RiskLevel
	// Scopes are the required API-token scopes copied from the Tool at
	// register time. See Tool.Scopes for semantics.
	Scopes []string
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
	// Every tool must declare its token-scope requirement so the MCP adapter
	// can never dispatch a tool that slipped through unmapped. Panicking at
	// init keeps the failure loud at startup instead of silently open at
	// request time.
	if len(t.Scopes) == 0 {
		panic("aitools: tool " + t.Name + " must declare required token Scopes")
	}
	if !validCapabilityGroup(t.Group) {
		panic("aitools: tool " + t.Name + " must declare a valid capability Group")
	}
	if !validAccessLevel(t.Access) {
		panic("aitools: tool " + t.Name + " must declare a valid Access level")
	}
	if !validRiskLevel(t.Risk) {
		panic("aitools: tool " + t.Name + " must declare a valid Risk level")
	}

	schema, err := jsonschema.For[Args](nil)
	if err != nil {
		panic(fmt.Sprintf("aitools: build schema for %s: %v", t.Name, err))
	}
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("aitools: marshal schema for %s: %v", t.Name, err))
	}
	schemaJSON, err = toolschema.NormalizeParameters(schemaJSON)
	if err != nil {
		panic(fmt.Sprintf("aitools: normalize schema for %s: %v", t.Name, err))
	}

	entry := Entry{
		Name:        t.Name,
		Description: t.Description,
		Group:       t.Group,
		Access:      t.Access,
		Risk:        t.Risk,
		Scopes:      t.Scopes,
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

func validCapabilityGroup(group CapabilityGroup) bool {
	switch group {
	case CapabilityReadComment, CapabilityIssueManagement,
		CapabilityCommentEditing, CapabilityPlanningActivity,
		CapabilityKnowledgeDiagrams, CapabilityActions, CapabilityTests,
		CapabilityTime, CapabilityUsersApprovals:
		return true
	default:
		return false
	}
}

func validAccessLevel(access AccessLevel) bool {
	switch access {
	case AccessRead, AccessWrite, AccessDestructive, AccessAdmin:
		return true
	default:
		return false
	}
}

func validRiskLevel(risk RiskLevel) bool {
	switch risk {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	default:
		return false
	}
}

// CapabilityGroupDefinition is the canonical Studio-facing view of a group
// and the Standard-safe executable tools it contains.
type CapabilityGroupDefinition struct {
	Key      CapabilityGroup `json:"key"`
	Label    string          `json:"label"`
	Required bool            `json:"required"`
	Tools    []ToolSummary   `json:"tools"`
}

// ToolSummary is safe configuration metadata; schemas and implementations
// remain internal to the executable registry.
type ToolSummary struct {
	Name   string      `json:"name"`
	Access AccessLevel `json:"access"`
	Risk   RiskLevel   `json:"risk"`
	Scopes []string    `json:"scopes"`
}

var capabilityGroupLabels = map[CapabilityGroup]string{
	CapabilityReadComment:       "Read and comment",
	CapabilityIssueManagement:   "Issue management",
	CapabilityCommentEditing:    "Comment editing",
	CapabilityPlanningActivity:  "Labels, links, planning, and activity",
	CapabilityKnowledgeDiagrams: "Knowledge and diagrams",
	CapabilityActions:           "Actions",
	CapabilityTests:             "Tests",
	CapabilityTime:              "Time",
	CapabilityUsersApprovals:    "Users and approvals",
}

// StandardCapabilityGroups derives the Agent Studio catalog directly from
// executable registry entries. Destructive and access-administration tools
// remain registered for existing surfaces but are never exposed to Standard
// profiles.
func StandardCapabilityGroups(r *Registry) []CapabilityGroupDefinition {
	byGroup := make(map[CapabilityGroup][]ToolSummary)
	for _, entry := range r.All() {
		if entry.Access == AccessDestructive || entry.Access == AccessAdmin {
			continue
		}
		byGroup[entry.Group] = append(byGroup[entry.Group], ToolSummary{
			Name:   entry.Name,
			Access: entry.Access,
			Risk:   entry.Risk,
			Scopes: append([]string(nil), entry.Scopes...),
		})
	}

	groups := make([]CapabilityGroupDefinition, 0, len(byGroup))
	for group, tools := range byGroup {
		sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
		groups = append(groups, CapabilityGroupDefinition{
			Key:      group,
			Label:    capabilityGroupLabels[group],
			Required: group == CapabilityReadComment,
			Tools:    tools,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Required != groups[j].Required {
			return groups[i].Required
		}
		return groups[i].Label < groups[j].Label
	})
	return groups
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
