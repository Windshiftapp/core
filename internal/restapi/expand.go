package restapi

import (
	"net/http"
	"strings"
)

// ExpandOptions holds flags for which related resources to expand
type ExpandOptions struct {
	// Item-related expansions
	Comments    bool
	Attachments bool
	History     bool
	Children    bool
	Transitions bool

	// Reference expansions
	Assignee  bool
	Creator   bool
	Status    bool
	Priority  bool
	ItemType  bool
	Workspace bool
	Milestone bool
	Iteration bool
	Project   bool

	// Workflow expansions
	WorkflowTransitions bool
	StatusCategory      bool
}

// ParseExpand extracts expand options from request query parameter
// Usage: ?expand=comments,assignee,status
func ParseExpand(r *http.Request) ExpandOptions {
	expandParam := r.URL.Query().Get("expand")
	if expandParam == "" {
		return ExpandOptions{}
	}

	opts := ExpandOptions{}
	parts := strings.Split(expandParam, ",")

	for _, p := range parts {
		switch strings.TrimSpace(strings.ToLower(p)) {
		// Item-related
		case "comments":
			opts.Comments = true
		case "attachments":
			opts.Attachments = true
		case "history":
			opts.History = true
		case "children":
			opts.Children = true
		case "transitions":
			opts.Transitions = true

		// Reference types
		case "assignee":
			opts.Assignee = true
		case "creator":
			opts.Creator = true
		case "status":
			opts.Status = true
		case "priority":
			opts.Priority = true
		case "item_type", "itemtype", "type":
			opts.ItemType = true
		case "workspace":
			opts.Workspace = true
		case "milestone":
			opts.Milestone = true
		case "iteration":
			opts.Iteration = true
		case "project":
			opts.Project = true

		// Workflow
		case "workflow_transitions", "workflowtransitions":
			opts.WorkflowTransitions = true
		case "status_category", "statuscategory", "category":
			opts.StatusCategory = true
		}
	}

	return opts
}

// HasAnyExpansion returns true if any expansion is requested
func (o ExpandOptions) HasAnyExpansion() bool {
	return o.Comments || o.Attachments || o.History || o.Children || o.Transitions ||
		o.Assignee || o.Creator || o.Status || o.Priority || o.ItemType ||
		o.Workspace || o.Milestone || o.Iteration || o.Project ||
		o.WorkflowTransitions || o.StatusCategory
}

// ValidExpandFields returns a list of valid expand field names for documentation
func ValidExpandFields() []string {
	return []string{
		"comments",
		"attachments",
		"history",
		"children",
		"transitions",
		"assignee",
		"creator",
		"status",
		"priority",
		"item_type",
		"workspace",
		"milestone",
		"iteration",
		"project",
		"workflow_transitions",
		"status_category",
	}
}
