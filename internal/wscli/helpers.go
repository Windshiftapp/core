package wscli

import (
	"fmt"
	"strconv"
	"strings"
)

// resolveOptionalWorkspace resolves the workspace ID from config/flag if present.
// Returns nil if no workspace is configured.
func resolveOptionalWorkspace(client *Client) (*int, error) {
	wsKey := cfg.GetEffectiveWorkspace()
	if wsKey == "" {
		return nil, nil
	}
	wsID, err := client.ResolveWorkspaceID(wsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve workspace: %w", err)
	}
	return &wsID, nil
}

// resolveRequiredWorkspace resolves the workspace ID from config/flag,
// returning an error if no workspace is configured.
func resolveRequiredWorkspace(client *Client) (int, error) {
	wsKey := cfg.GetEffectiveWorkspace()
	if wsKey == "" {
		return 0, fmt.Errorf("workspace is required: use -w flag or set defaults.workspace_key in config")
	}
	wsID, err := client.ResolveWorkspaceID(wsKey)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve workspace: %w", err)
	}
	return wsID, nil
}

// TestResultSummary holds aggregated counts of test result statuses.
type TestResultSummary struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Blocked int `json:"blocked"`
	Skipped int `json:"skipped"`
	NotRun  int `json:"not_run"`
}

// calculateTestResultSummary counts test results by status.
func calculateTestResultSummary(results []TestResult) TestResultSummary {
	var s TestResultSummary
	for _, r := range results {
		s.Total++
		switch r.Status {
		case "passed":
			s.Passed++
		case "failed":
			s.Failed++
		case "blocked":
			s.Blocked++
		case "skipped":
			s.Skipped++
		case "not_run":
			s.NotRun++
		}
	}
	return s
}

// itemDisplayFields extracts the common display fields (key, status, assignee,
// item type) from an Item, returning safe string values for table/CSV output.
func itemDisplayFields(item *Item) (key, status, assignee, itemType string) {
	key = item.Key
	if key == "" {
		key = fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
	}
	status = item.StatusName
	assignee = item.AssigneeName
	itemType = item.ItemTypeName
	return
}

// parentRef returns a reference to an item's parent for display. It prefers
// the server-provided ParentKey (e.g. "WI-120"), which the server computes
// from the parent's own workspace and withholds when the caller may not view
// the parent. When the key is absent it falls back to the resolvable
// "item:<id>" form — never a key reconstructed from ParentID, which is a DB id
// in a different namespace. Returns "" when the item has no parent.
func parentRef(item *Item) string {
	if item == nil || item.ParentID == nil {
		return ""
	}
	if item.ParentKey != "" {
		return item.ParentKey
	}
	return fmt.Sprintf("item:%d", *item.ParentID)
}

// childrenSummary renders an inline list of child keys with a total count,
// e.g. "WI-385, WI-386, WI-387 (3)". The list is capped so big epics stay
// scannable; remaining children are summarized as "… (+N more)". Each child
// already carries its own correct Key, so no key arithmetic is needed.
func childrenSummary(children []Item) string {
	const maxInline = 10
	keys := make([]string, 0, len(children))
	for i := range children {
		k, _, _, _ := itemDisplayFields(&children[i])
		keys = append(keys, k)
		if len(keys) == maxInline && len(children) > maxInline {
			break
		}
	}
	joined := strings.Join(keys, ", ")
	if len(children) > len(keys) {
		joined = fmt.Sprintf("%s … (+%d more)", joined, len(children)-len(keys))
	}
	return fmt.Sprintf("%s (%d)", joined, len(children))
}

// parentDisplay renders the parent for a human-facing detail line, e.g.
// "WI-120 (Login epic)", or just the reference when no title is available.
// Returns "" when the item has no parent.
func parentDisplay(item *Item) string {
	key := parentRef(item)
	if key == "" {
		return ""
	}
	if item.ParentTitle != "" {
		return fmt.Sprintf("%s (%s)", key, item.ParentTitle)
	}
	return key
}

// newFiltersWithWorkspace creates a filter map and applies the optional
// workspace filter when one is configured. The optional seed entries are
// copied into the returned map before the workspace filter is applied.
func newFiltersWithWorkspace(client *Client, seed map[string]string) (map[string]string, error) {
	filters := make(map[string]string, len(seed)+1)
	for k, v := range seed {
		filters[k] = v
	}
	if wsID, err := resolveOptionalWorkspace(client); err != nil {
		return nil, err
	} else if wsID != nil {
		filters["workspace_id"] = fmt.Sprintf("%d", *wsID)
	}
	return filters, nil
}

// applyStatusFilter resolves and adds a status filter (with optional ~negation)
// to the supplied filters map. It is a no-op when statusVal is empty.
func applyStatusFilter(filters map[string]string, statusVal string, client *Client) error {
	if statusVal == "" {
		return nil
	}
	key := "status_id"
	if isNegatedFilter(statusVal) {
		key, statusVal = "status_id_not", stripNegation(statusVal)
	}
	resolved := cfg.ResolveStatusWithFallback(statusVal, client)
	if validStatusIDs(resolved) {
		filters[key] = resolved
		return nil
	}
	workspaceID, err := strconv.Atoi(filters["workspace_id"])
	if err != nil || workspaceID <= 0 {
		return fmt.Errorf("status names require a workspace: use -w, or supply a numeric status ID")
	}
	statuses, err := client.GetWorkspaceStatuses(workspaceID)
	if err != nil {
		return fmt.Errorf("resolve status: %w", err)
	}
	var ids, names []string
	for _, status := range statuses {
		names = append(names, status.Name)
		if strings.EqualFold(status.Name, resolved) {
			ids = append(ids, strconv.Itoa(status.ID))
		}
	}
	if len(ids) == 0 {
		return fmt.Errorf("unknown status %q; valid statuses: %s", statusVal, strings.Join(names, ", "))
	}
	filters[key] = strings.Join(ids, ",")
	return nil
}

func validStatusIDs(value string) bool {
	for _, part := range strings.Split(value, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || id <= 0 {
			return false
		}
	}
	return true
}

func resolveParentID(client *Client, value string) (int, error) {
	if value == "0" {
		return 0, nil
	}
	id, err := client.ResolveItemID(value)
	if err != nil {
		return 0, fmt.Errorf("resolve parent: %w", err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("parent must be an item key, positive ID, or 0 to clear")
	}
	return id, nil
}

// WorkspaceContext holds the commonly fetched workspace configuration data.
type WorkspaceContext struct {
	Workspace *Workspace
	Statuses  []Status
	ItemTypes []ItemType
	Workflows []Workflow
}

// fetchWorkspaceContext retrieves workspace details, statuses, item types, and
// workflows for the given workspace ID.
func fetchWorkspaceContext(client *Client, wsID int) (*WorkspaceContext, error) {
	workspace, err := client.GetWorkspace(wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	statuses, err := client.GetWorkspaceStatuses(wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get statuses: %w", err)
	}

	itemTypes, err := client.GetWorkspaceItemTypes(wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get item types: %w", err)
	}

	workflows, err := client.GetWorkspaceWorkflows(wsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflows: %w", err)
	}

	return &WorkspaceContext{
		Workspace: workspace,
		Statuses:  statuses,
		ItemTypes: itemTypes,
		Workflows: workflows,
	}, nil
}
