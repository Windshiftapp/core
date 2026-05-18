package actioncatalog

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"windshift/internal/models"
	"windshift/internal/repository/actionutil"
)

// ValidationError is one structured grievance against an action definition.
// Code is a short machine-readable tag clients can switch on; Path points at
// the offending element (`trigger`, `nodes[2].node_config.field_name`,
// `edges[1].source_node_id`, …) so the UI / agent can highlight exactly
// what to fix.
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

// Error implements the error interface so a single ValidationError can be
// returned where an error is expected; the surface in Validate() also
// supports aggregating multiple errors via ValidationErrors.
func (e ValidationError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s at %s: %s", e.Code, e.Path, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ValidationErrors is the multi-error surface returned by Validate. It
// implements error so callers can `return errs` cheaply; the JSON shape
// matches the per-element ValidationError above for v1 API parity.
type ValidationErrors []ValidationError

// Error renders the first error's message — the structured list is what
// callers usually iterate over, but a non-empty list must still be a
// non-empty Error() for compatibility with errors.Is/As chains.
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}
	if len(e) == 1 {
		return e[0].Error()
	}
	parts := make([]string, 0, len(e))
	for _, v := range e {
		parts = append(parts, v.Error())
	}
	return strings.Join(parts, "; ")
}

// Codes used in ValidationError.Code. Stable across the v1 surface so
// clients can dispatch on them.
const (
	CodeRequired           = "required"
	CodeUnknownTriggerType = "unknown_trigger_type"
	CodeUnknownNodeType    = "unknown_node_type"
	CodeInvalidConfigJSON  = "invalid_config_json"
	CodeInvalidConfig      = "invalid_config"
	CodeUnknownNodeID      = "unknown_node_id"
	CodeDuplicateNodeID    = "duplicate_node_id"
	CodeMissingTrigger     = "missing_trigger"
	CodeMultipleTriggers   = "multiple_triggers"
	CodeFlowCycle          = "flow_cycle"
	CodeIteratorBodyLeak   = "iterator_body_leak"
	CodeAmbiguousFlow      = "ambiguous_flow"
	CodeUnknownCapability  = "unknown_capability"
)

// ActionDefinition is the validator's input — a node-and-edge graph plus
// trigger metadata. It mirrors models.CreateActionRequest closely but
// stays separate because the validator should not depend on a request
// type (the same shape is used by the template loader and the MCP tools).
type ActionDefinition struct {
	Name          string                   `json:"name"`
	Description   string                   `json:"description,omitempty"`
	TriggerType   models.ActionTriggerType `json:"trigger_type"`
	TriggerConfig string                   `json:"trigger_config,omitempty"` // JSON string; empty == "{}"
	Nodes         []models.ActionNode      `json:"nodes"`
	Edges         []models.ActionEdge      `json:"edges,omitempty"`
}

// FromCreateRequest is a convenience adapter so HTTP handlers can hand the
// validator the same shape they already accept on the wire.
func FromCreateRequest(req *models.CreateActionRequest) ActionDefinition {
	return ActionDefinition{
		Name:          req.Name,
		Description:   req.Description,
		TriggerType:   req.TriggerType,
		TriggerConfig: req.TriggerConfig,
		Nodes:         req.Nodes,
		Edges:         req.Edges,
	}
}

// CapabilityResolver is the dependency the validator uses to confirm that
// node config references (capability_id on http_request / container_run /
// ai_extract / ai_agent) point at a capability the target workspace can
// actually reach. The HTTP handler injects an implementation backed by
// repository.ActionRepository.ListCapabilitiesForWorkspace; tests can
// inject a fake set without spinning up a database.
//
// A nil resolver means the capability-existence check is skipped — used
// by the template-loader path where capabilities live on a different
// surface and templates use placeholder IDs that get rewritten at apply
// time. The schema-shape check still runs and catches typos.
type CapabilityResolver interface {
	HasCapability(workspaceID, capabilityID int) bool
}

// Validate runs the full validation pipeline on def and returns a
// ValidationErrors list. An empty list means the action is structurally
// sound and safe to persist. The workspace argument is forwarded to the
// capability resolver; pass 0 when no resolver is supplied.
//
// Validation order: cheap structural checks first, schema checks second,
// graph-shape checks last. Each phase short-circuits subsequent ones that
// would error noisily on the same data: an unknown trigger type prevents
// trigger-config validation; missing node types prevent node-config
// validation for that specific node.
func Validate(c *Catalog, def ActionDefinition, workspaceID int, caps CapabilityResolver) ValidationErrors {
	var errs ValidationErrors

	// --- Top-level required fields ---------------------------------------
	if strings.TrimSpace(def.Name) == "" {
		errs = append(errs, ValidationError{Code: CodeRequired, Message: "Name is required", Path: "name"})
	}
	if string(def.TriggerType) == "" {
		errs = append(errs, ValidationError{Code: CodeRequired, Message: "Trigger type is required", Path: "trigger_type"})
	}

	// --- Trigger type & config -------------------------------------------
	if def.TriggerType != "" {
		trig := c.Trigger(def.TriggerType)
		if trig == nil {
			errs = append(errs, ValidationError{
				Code:    CodeUnknownTriggerType,
				Message: fmt.Sprintf("Unknown trigger type %q", def.TriggerType),
				Path:    "trigger_type",
			})
		} else {
			if err := validateConfigJSON(trig.resolved, def.TriggerConfig); err != nil {
				errs = append(errs, ValidationError{
					Code:    schemaErrCode(err),
					Message: err.Error(),
					Path:    "trigger_config",
				})
			}
		}
	}

	// --- Nodes: types + per-node config schema ---------------------------
	nodeIDsSeen := make(map[int]bool, len(def.Nodes))
	triggerNodeCount := 0
	for i, n := range def.Nodes {
		path := fmt.Sprintf("nodes[%d]", i)
		if n.ID != 0 && nodeIDsSeen[n.ID] {
			errs = append(errs, ValidationError{
				Code:    CodeDuplicateNodeID,
				Message: fmt.Sprintf("Duplicate node ID %d", n.ID),
				Path:    path + ".id",
			})
		}
		if n.ID != 0 {
			nodeIDsSeen[n.ID] = true
		}
		if n.NodeType == models.ActionNodeTrigger {
			triggerNodeCount++
		}
		meta := c.Node(n.NodeType)
		if meta == nil {
			errs = append(errs, ValidationError{
				Code:    CodeUnknownNodeType,
				Message: fmt.Sprintf("Unknown node type %q", n.NodeType),
				Path:    path + ".node_type",
			})
			continue
		}
		if err := validateConfigJSON(meta.resolved, n.NodeConfig); err != nil {
			errs = append(errs, ValidationError{
				Code:    schemaErrCode(err),
				Message: err.Error(),
				Path:    path + ".node_config",
			})
			// Skip capability check if config itself is broken — the field
			// we'd inspect might not even exist on the parsed shape.
			continue
		}
		if caps != nil && workspaceID > 0 {
			if capID, field := capabilityRef(n); capID > 0 {
				if !caps.HasCapability(workspaceID, capID) {
					errs = append(errs, ValidationError{
						Code:    CodeUnknownCapability,
						Message: fmt.Sprintf("Capability %d is not available to this workspace", capID),
						Path:    path + ".node_config." + field,
					})
				}
			}
		}
	}

	if len(def.Nodes) > 0 && triggerNodeCount > 1 {
		errs = append(errs, ValidationError{
			Code:    CodeMultipleTriggers,
			Message: "Action graph must contain at most one trigger node",
			Path:    "nodes",
		})
	}

	// --- Edges: source/target reference known nodes ----------------------
	for i, e := range def.Edges {
		path := fmt.Sprintf("edges[%d]", i)
		if e.SourceNodeID == 0 || !nodeIDsSeen[e.SourceNodeID] {
			errs = append(errs, ValidationError{
				Code:    CodeUnknownNodeID,
				Message: fmt.Sprintf("Edge source_node_id %d does not match any node", e.SourceNodeID),
				Path:    path + ".source_node_id",
			})
		}
		if e.TargetNodeID == 0 || !nodeIDsSeen[e.TargetNodeID] {
			errs = append(errs, ValidationError{
				Code:    CodeUnknownNodeID,
				Message: fmt.Sprintf("Edge target_node_id %d does not match any node", e.TargetNodeID),
				Path:    path + ".target_node_id",
			})
		}
	}

	// --- Graph-level invariants (cycles, ambiguous flow, iterator body) --
	if !errs.Has(CodeUnknownNodeID) && !errs.Has(CodeDuplicateNodeID) {
		if err := actionutil.ValidateFlowAcyclic[
			models.ActionNode, *models.ActionNode,
			models.ActionEdge, *models.ActionEdge,
		](def.Nodes, def.Edges); err != nil {
			errs = append(errs, ValidationError{
				Code:    CodeFlowCycle,
				Message: err.Error(),
				Path:    "edges",
			})
		}

		if msg := validateNonTriggerAmbiguity(def.Nodes, def.Edges); msg != "" {
			errs = append(errs, ValidationError{
				Code:    CodeAmbiguousFlow,
				Message: msg,
				Path:    "edges",
			})
		}

		if leak := validateIteratorBodies(def.Nodes, def.Edges); leak != "" {
			errs = append(errs, ValidationError{
				Code:    CodeIteratorBodyLeak,
				Message: leak,
				Path:    "edges",
			})
		}
	}

	return errs
}

// Has reports whether any error in the list uses the given code. Used by
// callers to gate later graph checks behind earlier structural checks so
// they don't emit confusing "edge references missing node" cycle errors
// when the node ID was already flagged as unknown.
func (e ValidationErrors) Has(code string) bool {
	for _, v := range e {
		if v.Code == code {
			return true
		}
	}
	return false
}

// validateConfigJSON parses a JSON config string and validates it against
// the resolved schema. An empty string is treated as `{}` — that's how
// the storage layer represents "no config" today.
func validateConfigJSON(resolved *jsonschema.Resolved, cfg string) error {
	if resolved == nil {
		return nil
	}
	cfg = strings.TrimSpace(cfg)
	if cfg == "" {
		cfg = "{}"
	}
	var instance any
	if err := json.Unmarshal([]byte(cfg), &instance); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if err := resolved.Validate(instance); err != nil {
		return err
	}
	return nil
}

// schemaErrCode classifies a Validate failure as either invalid JSON
// (parse) or invalid shape (schema). The HTTP layer surfaces both as 400
// with a structured code so the wire shape stays stable as the underlying
// library evolves.
func schemaErrCode(err error) string {
	if err == nil {
		return ""
	}
	if strings.HasPrefix(err.Error(), "invalid JSON") {
		return CodeInvalidConfigJSON
	}
	return CodeInvalidConfig
}

// validateNonTriggerAmbiguity ports the existing handler-level rule: when
// a flow declares no edges but contains multiple non-trigger nodes, the
// engine would run them in topo-sort tie-break order, which is observable
// state the user can't predict. Matches handlers/actions.go:validateActionFlow.
func validateNonTriggerAmbiguity(nodes []models.ActionNode, edges []models.ActionEdge) string {
	if len(edges) > 0 || len(nodes) == 0 {
		return ""
	}
	nonTrigger := 0
	for _, n := range nodes {
		if n.NodeType != models.ActionNodeTrigger {
			nonTrigger++
		}
	}
	if nonTrigger > 1 {
		return "Action with multiple non-trigger nodes must declare edges between them"
	}
	return ""
}

// validateIteratorBodies enforces the same iterator-body containment
// invariant the YAML template validator checks: every node downstream of
// an iterator must have all its incoming edges originating from
// iterator-body nodes (the iterator itself, or another body node).
// Non-empty return is a human-readable description of the first leak.
func validateIteratorBodies(nodes []models.ActionNode, edges []models.ActionEdge) string {
	for _, n := range nodes {
		if !n.NodeType.IsIterator() {
			continue
		}
		body := iteratorBodyClosure(n.ID, edges)
		for _, e := range edges {
			if !body[e.TargetNodeID] {
				continue
			}
			if e.SourceNodeID == n.ID {
				continue
			}
			if !body[e.SourceNodeID] {
				return fmt.Sprintf("iterator node %d body has incoming edge from outside-body node %d", n.ID, e.SourceNodeID)
			}
		}
	}
	return ""
}

func iteratorBodyClosure(iteratorID int, edges []models.ActionEdge) map[int]bool {
	body := map[int]bool{}
	queue := []int{}
	for _, e := range edges {
		if e.SourceNodeID == iteratorID && !body[e.TargetNodeID] {
			body[e.TargetNodeID] = true
			queue = append(queue, e.TargetNodeID)
		}
	}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, e := range edges {
			if e.SourceNodeID == next && !body[e.TargetNodeID] {
				body[e.TargetNodeID] = true
				queue = append(queue, e.TargetNodeID)
			}
		}
	}
	return body
}

// capabilityRef pulls the capability_id field out of a node's config
// (when applicable) so the validator can check existence. Returns 0 when
// the node type has no capability reference or when the field is absent.
// Field name is returned alongside so the error path can pinpoint
// `node_config.capability_id` for the client.
func capabilityRef(n models.ActionNode) (id int, fieldName string) {
	switch n.NodeType {
	case models.ActionNodeHTTPRequest:
		var cfg models.HTTPRequestNodeConfig
		_ = json.Unmarshal([]byte(n.NodeConfig), &cfg)
		return cfg.CapabilityID, "capability_id"
	case models.ActionNodeContainerRun:
		var cfg models.ContainerRunNodeConfig
		_ = json.Unmarshal([]byte(n.NodeConfig), &cfg)
		return cfg.CapabilityID, "capability_id"
	case models.ActionNodeAIExtract:
		var cfg models.AIExtractNodeConfig
		_ = json.Unmarshal([]byte(n.NodeConfig), &cfg)
		return cfg.CapabilityID, "capability_id"
	case models.ActionNodeAIAgent:
		var cfg models.AIAgentNodeConfig
		_ = json.Unmarshal([]byte(n.NodeConfig), &cfg)
		return cfg.CapabilityID, "capability_id"
	}
	return 0, ""
}
