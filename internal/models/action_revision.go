package models

import (
	"encoding/json"
	"time"
)

// ActionDefinitionSnapshot is the complete executable configuration of an
// action at one point in time. It deliberately excludes volatile identity and
// join fields so two revisions compare equal iff the executable definition is
// unchanged. Script source (once execute_script nodes exist) travels in the
// node configs, so it is captured here too.
type ActionDefinitionSnapshot struct {
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	IsEnabled      bool              `json:"is_enabled"`
	TriggerType    ActionTriggerType `json:"trigger_type"`
	TriggerConfig  string            `json:"trigger_config,omitempty"`
	ActorUserID    *int              `json:"actor_user_id,omitempty"`
	AllowedRoleIDs []int             `json:"allowed_role_ids"`
	Nodes          []ActionNode      `json:"nodes"`
	Edges          []ActionEdge      `json:"edges"`
}

// NewActionDefinitionSnapshot builds the executable snapshot from the live
// action row and its persisted nodes/edges. Timestamps are stripped so a
// resave that changes nothing produces an identical snapshot; node IDs are
// kept because execution traces and the editor reference them.
func NewActionDefinitionSnapshot(action *Action, nodes []ActionNode, edges []ActionEdge) ActionDefinitionSnapshot {
	allowedRoles := make([]int, len(action.AllowedRoleIDs))
	copy(allowedRoles, action.AllowedRoleIDs)
	snapshot := ActionDefinitionSnapshot{
		Name:           action.Name,
		Description:    action.Description,
		IsEnabled:      action.IsEnabled,
		TriggerType:    action.TriggerType,
		TriggerConfig:  action.TriggerConfig,
		ActorUserID:    action.ActorUserID,
		AllowedRoleIDs: allowedRoles,
		Nodes:          make([]ActionNode, 0, len(nodes)),
		Edges:          make([]ActionEdge, 0, len(edges)),
	}
	for _, node := range nodes {
		snapshot.Nodes = append(snapshot.Nodes, ActionNode{
			ID: node.ID, ActionID: node.ActionID, NodeType: node.NodeType,
			NodeConfig: node.NodeConfig, PositionX: node.PositionX, PositionY: node.PositionY,
		})
	}
	for _, edge := range edges {
		snapshot.Edges = append(snapshot.Edges, ActionEdge{
			ID: edge.ID, ActionID: edge.ActionID,
			SourceNodeID: edge.SourceNodeID, TargetNodeID: edge.TargetNodeID,
			EdgeType: edge.EdgeType, SourceHandle: edge.SourceHandle, TargetHandle: edge.TargetHandle,
		})
	}
	return snapshot
}

// ActionDefinitionRevision is one immutable, attributable snapshot of an
// action's definition. Revisions are retained after the action is deleted so
// an execution log can always be traced back to the definition that ran.
type ActionDefinitionRevision struct {
	ID             int                      `json:"id"`
	ActionID       int                      `json:"action_id"`
	WorkspaceID    int                      `json:"workspace_id"`
	RevisionNumber int                      `json:"revision_number"`
	Source         string                   `json:"source"`
	EditorUserID   *int                     `json:"editor_user_id,omitempty"`
	EditorUsername string                   `json:"editor_username,omitempty"`
	EditorSource   string                   `json:"editor_source,omitempty"`
	ChangeSummary  string                   `json:"change_summary,omitempty"`
	ChangedFields  []string                 `json:"changed_fields"`
	Snapshot       ActionDefinitionSnapshot `json:"snapshot"`
	CreatedAt      time.Time                `json:"created_at"`
}

// ActionDefinitionRevisionSummary is the list projection of a revision: enough
// metadata for an operator to pick a revision without shipping the full
// executable snapshot on every list request.
type ActionDefinitionRevisionSummary struct {
	ID             int       `json:"id"`
	ActionID       int       `json:"action_id"`
	WorkspaceID    int       `json:"workspace_id"`
	RevisionNumber int       `json:"revision_number"`
	Source         string    `json:"source"`
	EditorUserID   *int      `json:"editor_user_id,omitempty"`
	EditorUsername string    `json:"editor_username,omitempty"`
	EditorSource   string    `json:"editor_source,omitempty"`
	ChangeSummary  string    `json:"change_summary,omitempty"`
	ChangedFields  []string  `json:"changed_fields"`
	CreatedAt      time.Time `json:"created_at"`
}

// Revision sources. They describe why a revision was written, not who wrote it.
const (
	ActionRevisionSourceCreate        = "create"
	ActionRevisionSourceUpdate        = "update"
	ActionRevisionSourceTemplate      = "template_apply"
	ActionRevisionSourceBaseline      = "baseline"
	ActionRevisionSourceImport        = "import"
	ActionRevisionSourceRestore       = "restore"
	ActionRevisionSourceActorChange   = "actor_change"
	ActionRevisionSourceEnabledChange = "enabled_change"
)

// ActionDefinitionFieldNames is the ordered list of executable fields a diff
// reports. Order is stable so callers and tests can rely on it.
var ActionDefinitionFieldNames = []string{
	"name", "description", "is_enabled", "trigger_type", "trigger_config",
	"actor_user_id", "allowed_role_ids", "nodes", "edges",
}

// DiffActionDefinitionSnapshots reports which executable fields differ between
// two snapshots. A nil previous snapshot (first revision) reports every field
// as changed.
func DiffActionDefinitionSnapshots(previous *ActionDefinitionSnapshot, next ActionDefinitionSnapshot) []string {
	if previous == nil {
		return append([]string{}, ActionDefinitionFieldNames...)
	}
	changed := []string{}
	for _, field := range ActionDefinitionFieldNames {
		if !actionSnapshotFieldEqual(*previous, next, field) {
			changed = append(changed, field)
		}
	}
	return changed
}

func actionSnapshotFieldEqual(a, b ActionDefinitionSnapshot, field string) bool {
	encode := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(raw)
	}
	switch field {
	case "name":
		return a.Name == b.Name
	case "description":
		return a.Description == b.Description
	case "is_enabled":
		return a.IsEnabled == b.IsEnabled
	case "trigger_type":
		return a.TriggerType == b.TriggerType
	case "trigger_config":
		return a.TriggerConfig == b.TriggerConfig
	case "actor_user_id":
		return encode(a.ActorUserID) == encode(b.ActorUserID)
	case "allowed_role_ids":
		return encode(a.AllowedRoleIDs) == encode(b.AllowedRoleIDs)
	case "nodes":
		return encode(a.Nodes) == encode(b.Nodes)
	case "edges":
		return encode(a.Edges) == encode(b.Edges)
	default:
		return true
	}
}
