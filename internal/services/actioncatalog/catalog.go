// Package actioncatalog registers action triggers and nodes for REST, MCP,
// and the frontend palette. New node types need a model, a registration, and
// an executor; TestNodeTypeCatalogCoverage detects model/catalog drift.
package actioncatalog

import (
	"fmt"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"

	"windshift/internal/models"
)

// NodeCategory groups palette entries.
const (
	CategoryFlow        = "flow"        // trigger
	CategoryMutation    = "mutation"    // set_field, set_status, transition_item, add_comment
	CategoryBranching   = "branching"   // condition
	CategoryIterator    = "iterator"    // related_items
	CategoryAssignment  = "assignment"  // round_robin_assign, notify_user
	CategoryAsset       = "asset"       // update_asset, create_asset
	CategoryIntegration = "integration" // http_request, container_run
	CategoryAI          = "ai"          // ai_extract, ai_agent
)

// NodeTypeMetadata describes one action-node kind to discovery clients.
// ConfigSchema is the JSON Schema (derived from the Go config struct) and
// resolved is the validator-ready form retained internally for the
// ValidateConfig path.
type NodeTypeMetadata struct {
	Type        models.ActionNodeType `json:"type"`
	Label       string                `json:"label"`
	Description string                `json:"description"`
	Category    string                `json:"category"`
	// ConfigSchema is always non-nil, including empty node configs.
	ConfigSchema *jsonschema.Schema `json:"config_schema"`
	// IsIterator enables iterator-body UI affordances.
	IsIterator bool `json:"is_iterator"`
	// Outputs lists the edge-source handle names a node emits. For most
	// nodes this is ["default"]; condition nodes emit ["true", "false"];
	// the trigger node also emits a single default edge.
	Outputs []string `json:"outputs"`

	resolved *jsonschema.Resolved
}

// TriggerTypeMetadata is the trigger-side counterpart of NodeTypeMetadata.
// Triggers don't have outputs in the same sense (the trigger node itself
// owns the default outgoing edge), so the shape is intentionally smaller.
type TriggerTypeMetadata struct {
	Type         models.ActionTriggerType `json:"type"`
	Label        string                   `json:"label"`
	Description  string                   `json:"description"`
	ConfigSchema *jsonschema.Schema       `json:"config_schema"`

	resolved *jsonschema.Resolved
}

// Catalog is the resolved, validated registry. Build it once at startup
// via Build(); both the v1 handler and the MCP tools share a single
// instance.
type Catalog struct {
	nodes    map[models.ActionNodeType]*NodeTypeMetadata
	nodeKeys []models.ActionNodeType // preserved registration order for stable API output

	triggers    map[models.ActionTriggerType]*TriggerTypeMetadata
	triggerKeys []models.ActionTriggerType
}

var (
	defaultOnce sync.Once
	defaultCat  *Catalog
	defaultErr  error
)

// Default returns the process-wide catalog, building it on first call.
// Schemas come from compile-time generics so the build cannot fail
// unless someone introduces a config struct the jsonschema library can't
// represent — in that case Default panics, since recovering at runtime
// would leave callers staring at an unexplained empty palette.
func Default() *Catalog {
	defaultOnce.Do(func() {
		defaultCat, defaultErr = Build()
		if defaultErr != nil {
			panic(fmt.Sprintf("actioncatalog: build default catalog: %v", defaultErr))
		}
	})
	return defaultCat
}

// Build constructs a fresh catalog. Exposed for tests so they can build a
// throwaway catalog without touching the process-wide singleton.
func Build() (*Catalog, error) {
	c := &Catalog{
		nodes:    map[models.ActionNodeType]*NodeTypeMetadata{},
		triggers: map[models.ActionTriggerType]*TriggerTypeMetadata{},
	}

	// Triggers ---------------------------------------------------------
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerStatusTransition,
		Label:       "Status transition",
		Description: "Fires when an item moves from one status to another. Use from_status_id / to_status_id to narrow the match, or to_status_category_completed to react to any terminal transition without enumerating per-workflow IDs.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerStatusLoop,
		Label:       "Status loop",
		Description: "Fires when an item is set to the status it already has and its workflow declares a self-transition for that status. to_status_id optionally filters the looped status; item_type_id optionally filters the item type.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerItemCreated,
		Label:       "Item created",
		Description: "Fires when a new work item is created. Optional item_type_id filter narrows the match to one type.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerItemUpdated,
		Label:       "Item updated",
		Description: "Fires when a work item changes. Optional field_name filter restricts to a single field; optional item_type_id filter narrows by type.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerItemLinked,
		Label:       "Item linked",
		Description: "Fires when a link is created between items. Optional link_type_id filter restricts to one link type.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerSLABreached,
		Label:       "SLA: breached",
		Description: "Fires when an SLA cycle misses its goal. The {{sla.metric}} variable carries the metric name.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerSLAWarning,
		Label:       "SLA: warning",
		Description: "Fires when an SLA cycle reaches a configured percent-of-goal warning threshold.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerManual,
		Label:       "Manual",
		Description: "Action does not auto-fire — it must be invoked explicitly via the execute endpoint. Useful for human-in-the-loop or scripted automations.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerSCMTagCreated,
		Label:       "SCM: tag created",
		Description: "Fires when a synced source-control repository discovers a new tag/release ref. SCM triggers have no authenticated actor, so the action must set an actor_user_id override.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerSCMReleaseBranchCreated,
		Label:       "SCM: release branch created",
		Description: "Fires when a synced source-control repository discovers a new release branch ref. SCM triggers have no authenticated actor, so the action must set an actor_user_id override.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerSCMPRLinked,
		Label:       "SCM: pull request linked",
		Description: "Fires when a synced repository discovers a new pull request linked to a work item. SCM triggers have no authenticated actor, so the action must set an actor_user_id override.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerSCMPRMerged,
		Label:       "SCM: pull request merged",
		Description: "Fires when a linked pull request transitions to merged. SCM triggers have no authenticated actor, so the action must set an actor_user_id override.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerCommentCreated,
		Label:       "Comment created",
		Description: "Fires when a comment is added to an item. Use from_customer to react only to customer replies (portal customer or email-intake sender) or only to agent comments.",
	}); err != nil {
		return nil, err
	}
	if err := registerTrigger[models.ActionTriggerConfig](c, triggerSpec{
		Type:        models.ActionTriggerItemInactive,
		Label:       "Item inactive",
		Description: "Fires when an open item has had no comment or update for the configured number of hours (inactive_hours). The trigger re-arms after new activity.",
	}); err != nil {
		return nil, err
	}

	// Nodes ------------------------------------------------------------
	// Order here is the order the frontend palette shows. Trigger is
	// implicit (created by the editor when a new action is built) but
	// still registered so introspection clients see the complete set.
	if err := registerNode[models.ActionTriggerConfig](c, nodeSpec{
		Type:        models.ActionNodeTrigger,
		Label:       "Trigger",
		Description: "Entry point of the action graph — emitted by the system when the action's trigger fires. Its config mirrors the action trigger_config so editors can store trigger filters on the visual trigger node.",
		Category:    CategoryFlow,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.SetFieldNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeSetField,
		Label:       "Set field",
		Description: "Update a column or custom field on the current item. Value supports {{variable}} template interpolation.",
		Category:    CategoryMutation,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.SetStatusNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeSetStatus,
		Label:       "Set status",
		Description: "Set the current item's status by literal status ID. Use transition_item if you need dynamic per-workflow resolution.",
		Category:    CategoryMutation,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.TransitionItemNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeTransitionItem,
		Label:       "Transition item",
		Description: "Transition whatever item is currently in execution context to a status picked dynamically: explicit ID, by category name, or by mirroring the trigger's terminal category.",
		Category:    CategoryMutation,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.AddCommentNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeAddComment,
		Label:       "Add comment",
		Description: "Post a comment on the current item. Content supports {{variable}} template interpolation; mark is_private for restricted visibility. Comments triggered by a private comment are posted as internal notes.",
		Category:    CategoryMutation,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.NotifyUserNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeNotifyUser,
		Label:       "Notify user",
		Description: "Send an in-app notification to assignee, creator, or specific user IDs.",
		Category:    CategoryAssignment,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.ConditionNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeCondition,
		Label:       "Condition",
		Description: "Branch on a field comparison. The downstream graph is reached via the `true` and `false` edge handles depending on the result.",
		Category:    CategoryBranching,
		Outputs:     []string{"true", "false"},
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.RelatedItemsNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeRelatedItems,
		Label:       "Related items",
		Description: "Iterator: fan out from the current item to its descendants / direct_children / ancestors / linked items, running the downstream body subgraph once per emitted item.",
		Category:    CategoryIterator,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.RoundRobinAssignNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeRoundRobinAssign,
		Label:       "Round-robin assign",
		Description: "Pick the next assignee from a team rotation and apply it to the current item.",
		Category:    CategoryAssignment,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.UpdateAssetNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeUpdateAsset,
		Label:       "Update asset",
		Description: "Mutate fields on an existing asset referenced by an item's asset field.",
		Category:    CategoryAsset,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.CreateAssetNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeCreateAsset,
		Label:       "Create asset",
		Description: "Create a new asset in the target set with title/description/field mappings driven by templates and item fields.",
		Category:    CategoryAsset,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.HTTPRequestNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeHTTPRequest,
		Label:       "HTTP request",
		Description: "Issue an HTTP request via a configured http_client capability. URL, headers, and body support {{variable}} templates; response stored in output_field.",
		Category:    CategoryIntegration,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.ContainerRunNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeContainerRun,
		Label:       "Run container",
		Description: "Run a sandboxed Docker container from a configured docker_environment capability. Runtime info (id, host, port) is stored in output_field.",
		Category:    CategoryIntegration,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.AIExtractNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeAIExtract,
		Label:       "AI extract",
		Description: "Sandboxed LLM extraction: process untrusted input with no tools, structured-output only. Output schema enforced server-side.",
		Category:    CategoryAI,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.AIAgentNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeAIAgent,
		Label:       "AI agent",
		Description: "Agentic LLM execution with a purpose-built tool set. Never receives raw untrusted input; bounded by max_steps.",
		Category:    CategoryAI,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[createMilestoneNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeCreateMilestone,
		Label:       "Create milestone",
		Description: "Upsert a workspace milestone from SCM branch/tag events, optionally promoting status and attaching release information.",
		Category:    CategoryMutation,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.InsertCannedResponseNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeInsertCannedResponse,
		Label:       "Insert canned response",
		Description: "Render a workspace canned response with the execution variables and post it as a comment on the current item. Private snippets, and any insert triggered by a private comment, are posted as private comments.",
		Category:    CategoryMutation,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.NotifyCustomerNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeNotifyCustomer,
		Label:       "Notify customer",
		Description: "Email the portal customer who created the item through the threaded reply transport. Skips safely when the item has no customer with an email address, or when the triggering comment is private.",
		Category:    CategoryMutation,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.AdjustLabelsNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeAdjustLabels,
		Label:       "Adjust labels",
		Description: "Add or remove labels on the current item.",
		Category:    CategoryMutation,
	}); err != nil {
		return nil, err
	}
	if err := registerNode[models.ExecuteScriptNodeConfig](c, nodeSpec{
		Type:        models.ActionNodeExecuteScript,
		Label:       "Run script",
		Description: "Run a JavaScript function body with the effective executor's identity. The script can call the v2 REST API through a server-minted, short-lived token; the return value is stored in the configured output variable.",
		Category:    CategoryIntegration,
	}); err != nil {
		return nil, err
	}

	// Drift detector: every ActionNodeType constant must be registered.
	// AllActionNodeTypes is the authoritative list (kept next to the
	// constant block); when a new constant is added, the catalog build
	// fails until metadata is registered here.
	for _, t := range AllActionNodeTypes {
		if _, ok := c.nodes[t]; !ok {
			return nil, fmt.Errorf("actioncatalog: node type %q has no metadata registered", t)
		}
	}
	for _, t := range AllActionTriggerTypes {
		if _, ok := c.triggers[t]; !ok {
			return nil, fmt.Errorf("actioncatalog: trigger type %q has no metadata registered", t)
		}
	}

	return c, nil
}

// createMilestoneNodeConfig mirrors services.CreateMilestoneNodeConfig without
// importing the parent services package into the catalog subpackage.
type createMilestoneNodeConfig struct {
	NameTemplate        string `json:"name_template,omitempty"`
	UpsertKeyTemplate   string `json:"upsert_key_template"`
	StatusOnBranch      string `json:"status_on_branch,omitempty"`
	StatusOnTag         string `json:"status_on_tag,omitempty"`
	AttachReleaseOnTag  *bool  `json:"attach_release_on_tag,omitempty"`
	CategoryID          *int   `json:"category_id,omitempty"`
	DescriptionTemplate string `json:"description_template,omitempty"`
	AttachCommitIssues  *bool  `json:"attach_commit_issues,omitempty"`
}

type nodeSpec struct {
	Type        models.ActionNodeType
	Label       string
	Description string
	Category    string
	// Outputs defaults to []string{"default"} when nil. Condition nodes
	// override with ["true", "false"].
	Outputs []string
}

type triggerSpec struct {
	Type        models.ActionTriggerType
	Label       string
	Description string
}

func registerNode[Cfg any](c *Catalog, spec nodeSpec) error {
	schema, err := jsonschema.For[Cfg](nil)
	if err != nil {
		return fmt.Errorf("schema for node %q: %w", spec.Type, err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve schema for node %q: %w", spec.Type, err)
	}
	outputs := spec.Outputs
	if outputs == nil {
		outputs = []string{"default"}
	}
	c.nodes[spec.Type] = &NodeTypeMetadata{
		Type:         spec.Type,
		Label:        spec.Label,
		Description:  spec.Description,
		Category:     spec.Category,
		ConfigSchema: schema,
		IsIterator:   spec.Type.IsIterator(),
		Outputs:      outputs,
		resolved:     resolved,
	}
	c.nodeKeys = append(c.nodeKeys, spec.Type)
	return nil
}

func registerTrigger[Cfg any](c *Catalog, spec triggerSpec) error {
	schema, err := jsonschema.For[Cfg](nil)
	if err != nil {
		return fmt.Errorf("schema for trigger %q: %w", spec.Type, err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve schema for trigger %q: %w", spec.Type, err)
	}
	c.triggers[spec.Type] = &TriggerTypeMetadata{
		Type:         spec.Type,
		Label:        spec.Label,
		Description:  spec.Description,
		ConfigSchema: schema,
		resolved:     resolved,
	}
	c.triggerKeys = append(c.triggerKeys, spec.Type)
	return nil
}

// Nodes returns every registered node type in registration order. Callers
// must not mutate the returned slice.
func (c *Catalog) Nodes() []*NodeTypeMetadata {
	out := make([]*NodeTypeMetadata, 0, len(c.nodeKeys))
	for _, k := range c.nodeKeys {
		out = append(out, c.nodes[k])
	}
	return out
}

// Triggers returns every registered trigger type in registration order.
func (c *Catalog) Triggers() []*TriggerTypeMetadata {
	out := make([]*TriggerTypeMetadata, 0, len(c.triggerKeys))
	for _, k := range c.triggerKeys {
		out = append(out, c.triggers[k])
	}
	return out
}

// Node looks up metadata for the given type. Returns nil for unknown types.
func (c *Catalog) Node(t models.ActionNodeType) *NodeTypeMetadata {
	return c.nodes[t]
}

// Trigger looks up metadata for the given trigger type. Returns nil for
// unknown types.
func (c *Catalog) Trigger(t models.ActionTriggerType) *TriggerTypeMetadata {
	return c.triggers[t]
}

// AllActionNodeTypes enumerates every persisted node-type constant so the
// drift detector in catalog_test.go can ensure none is left unregistered.
// Kept next to the catalog rather than next to the constants because the
// catalog is the only consumer that needs the exhaustive list — the
// executor switches on the string and tolerates unknowns by reporting a
// node-level error at runtime.
var AllActionNodeTypes = []models.ActionNodeType{
	models.ActionNodeTrigger,
	models.ActionNodeSetField,
	models.ActionNodeSetStatus,
	models.ActionNodeAddComment,
	models.ActionNodeNotifyUser,
	models.ActionNodeCondition,
	models.ActionNodeUpdateAsset,
	models.ActionNodeCreateAsset,
	models.ActionNodeRoundRobinAssign,
	models.ActionNodeAIExtract,
	models.ActionNodeAIAgent,
	models.ActionNodeContainerRun,
	models.ActionNodeHTTPRequest,
	models.ActionNodeTransitionItem,
	models.ActionNodeRelatedItems,
	models.ActionNodeCreateMilestone,
	models.ActionNodeExecuteScript,
}

// AllActionTriggerTypes is the trigger counterpart of AllActionNodeTypes.
var AllActionTriggerTypes = []models.ActionTriggerType{
	models.ActionTriggerStatusTransition,
	models.ActionTriggerStatusLoop,
	models.ActionTriggerItemCreated,
	models.ActionTriggerItemUpdated,
	models.ActionTriggerItemLinked,
	models.ActionTriggerManual,
	models.ActionTriggerSCMTagCreated,
	models.ActionTriggerSCMReleaseBranchCreated,
	models.ActionTriggerSCMPRLinked,
	models.ActionTriggerSCMPRMerged,
}
