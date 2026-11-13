package aitools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"windshift/internal/auth"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/services/actioncatalog"
	"windshift/internal/services/actiontemplates"
)

// actionWriteMeta builds the transaction-coupled audit metadata for an
// agent-driven definition write. AuditDetails carry adapter correlation ids;
// the definition service persists them with the revision.
func (e *Env) actionWriteMeta(actionType string, details map[string]any) services.ActionWriteMeta {
	merged := make(map[string]any, len(e.AuditDetails)+len(details))
	for key, value := range e.AuditDetails {
		merged[key] = value
	}
	for key, value := range details {
		merged[key] = value
	}
	return services.ActionWriteMeta{
		Actor:             services.AuditActor{UserID: e.UserID, Username: e.Username, Source: e.Source},
		PrimaryActionType: actionType,
		PrimaryDetails:    merged,
	}
}

// catalogNodeDTO exposes each node's JSON Schema directly to the LLM.
type catalogNodeDTO struct {
	Type         models.ActionNodeType `json:"type"`
	Label        string                `json:"label"`
	Description  string                `json:"description"`
	Category     string                `json:"category"`
	ConfigSchema json.RawMessage       `json:"config_schema"`
	IsIterator   bool                  `json:"is_iterator"`
	Outputs      []string              `json:"outputs"`
}

type catalogTriggerDTO struct {
	Type         models.ActionTriggerType `json:"type"`
	Label        string                   `json:"label"`
	Description  string                   `json:"description"`
	ConfigSchema json.RawMessage          `json:"config_schema"`
}

type catalogCapabilityDTO struct {
	ID             int                   `json:"id"`
	Name           string                `json:"name"`
	CapabilityType models.CapabilityType `json:"capability_type"`
}

type catalogOut struct {
	Scope        string                 `json:"scope"`
	Triggers     []catalogTriggerDTO    `json:"triggers"`
	Nodes        []catalogNodeDTO       `json:"nodes"`
	Capabilities []catalogCapabilityDTO `json:"capabilities"`
}

// actionDefinitionArg exposes an action graph; node schemas come from the catalog.
type actionDefinitionArg struct {
	Name          string                   `json:"name" jsonschema:"Human-readable action name"`
	Description   string                   `json:"description,omitempty" jsonschema:"Optional description"`
	TriggerType   models.ActionTriggerType `json:"trigger_type" jsonschema:"Trigger type — call describe_action_catalog to see valid values and their config schemas"`
	TriggerConfig string                   `json:"trigger_config,omitempty" jsonschema:"JSON-encoded trigger config string. Empty string means no config. Validate against the trigger's config_schema from the catalog."`
	Nodes         []actionNodeArg          `json:"nodes" jsonschema:"Action node graph. Each node has a client-side id used by edges; the server assigns persistent IDs at create time."`
	Edges         []actionEdgeArg          `json:"edges,omitempty" jsonschema:"Edges connecting nodes. Use edge_type \"true\" / \"false\" for branches out of a condition node, \"default\" everywhere else."`
}

type actionNodeArg struct {
	ID         int                   `json:"id" jsonschema:"Client-side node ID (any positive int unique within this action). Server reassigns at persist time and rewrites edges accordingly."`
	NodeType   models.ActionNodeType `json:"node_type" jsonschema:"Node type — see describe_action_catalog for the list of valid values"`
	NodeConfig string                `json:"node_config" jsonschema:"JSON-encoded config string for this node. Validate against the node's config_schema from the catalog."`
	PositionX  float64               `json:"position_x,omitempty" jsonschema:"Optional X coordinate on the visual canvas (cosmetic only)"`
	PositionY  float64               `json:"position_y,omitempty" jsonschema:"Optional Y coordinate on the visual canvas (cosmetic only)"`
}

type actionEdgeArg struct {
	SourceNodeID int    `json:"source_node_id" jsonschema:"Client-side ID of the source node"`
	TargetNodeID int    `json:"target_node_id" jsonschema:"Client-side ID of the target node"`
	EdgeType     string `json:"edge_type,omitempty" jsonschema:"\"default\" (omittable) for unconditional flow; \"true\" / \"false\" for condition-node branches"`
}

func (a actionDefinitionArg) toDefinition() actioncatalog.ActionDefinition {
	nodes := make([]models.ActionNode, 0, len(a.Nodes))
	for _, n := range a.Nodes {
		nodes = append(nodes, models.ActionNode{
			ID:         n.ID,
			NodeType:   n.NodeType,
			NodeConfig: n.NodeConfig,
			PositionX:  n.PositionX,
			PositionY:  n.PositionY,
		})
	}
	edges := make([]models.ActionEdge, 0, len(a.Edges))
	for _, e := range a.Edges {
		t := e.EdgeType
		if t == "" {
			t = "default"
		}
		edges = append(edges, models.ActionEdge{
			SourceNodeID: e.SourceNodeID,
			TargetNodeID: e.TargetNodeID,
			EdgeType:     t,
		})
	}
	return actioncatalog.ActionDefinition{
		Name:          a.Name,
		Description:   a.Description,
		TriggerType:   a.TriggerType,
		TriggerConfig: a.TriggerConfig,
		Nodes:         nodes,
		Edges:         edges,
	}
}

func compactJSONForCompare(s string) string {
	if s == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		return s
	}
	return buf.String()
}

func actionDefinitionMatches(def actioncatalog.ActionDefinition, existing *models.Action) bool {
	if existing == nil || existing.Name != def.Name || existing.Description != def.Description ||
		existing.TriggerType != def.TriggerType || compactJSONForCompare(existing.TriggerConfig) != compactJSONForCompare(def.TriggerConfig) ||
		len(existing.Nodes) != len(def.Nodes) || len(existing.Edges) != len(def.Edges) {
		return false
	}
	idMap := map[int]int{}
	for i, n := range def.Nodes {
		existingNode := existing.Nodes[i]
		if existingNode.NodeType != n.NodeType || compactJSONForCompare(existingNode.NodeConfig) != compactJSONForCompare(n.NodeConfig) ||
			existingNode.PositionX != n.PositionX || existingNode.PositionY != n.PositionY {
			return false
		}
		idMap[n.ID] = existingNode.ID
	}
	for i, e := range def.Edges {
		existingEdge := existing.Edges[i]
		if existingEdge.SourceNodeID != idMap[e.SourceNodeID] || existingEdge.TargetNodeID != idMap[e.TargetNodeID] || existingEdge.EdgeType != e.EdgeType {
			return false
		}
	}
	return true
}

func findExistingEquivalentAction(repo *repository.ActionRepository, workspaceID int, def actioncatalog.ActionDefinition, createdBy int) (*models.Action, error) {
	actions, err := repo.ListByWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	for _, candidate := range actions {
		if candidate.CreatedBy == nil || *candidate.CreatedBy != createdBy || candidate.Name != def.Name || candidate.TriggerType != def.TriggerType {
			continue
		}
		full, err := repo.GetByID(candidate.ID)
		if err != nil {
			continue
		}
		if actionDefinitionMatches(def, full) {
			return full, nil
		}
	}
	return nil, nil
}

type capabilityResolverForEnv struct {
	repo *repository.ActionRepository
}

func (c capabilityResolverForEnv) HasCapability(workspaceID, capabilityID int) bool {
	capability, err := c.repo.GetCapabilityByID(capabilityID)
	if err != nil || capability == nil || !capability.IsEnabled {
		return false
	}
	scoped, err := c.repo.IsCapabilityScopedToWorkspace(capabilityID, workspaceID)
	return err == nil && scoped
}

func (c capabilityResolverForEnv) HasCapabilityOfType(workspaceID, capabilityID int, capabilityType models.CapabilityType) bool {
	capability, err := c.repo.GetCapabilityByID(capabilityID)
	if err != nil || capability == nil || !capability.IsEnabled || capability.CapabilityType != capabilityType {
		return false
	}
	scoped, err := c.repo.IsCapabilityScopedToWorkspace(capabilityID, workspaceID)
	return err == nil && scoped
}

type describeActionCatalogArgs struct {
	WorkspaceID int `json:"workspace_id" jsonschema:"Workspace ID to scope the capabilities list to. Triggers and node types are workspace-independent."`
}

type validateActionArgs struct {
	WorkspaceID int                 `json:"workspace_id" jsonschema:"Workspace the action would belong to"`
	Action      actionDefinitionArg `json:"action" jsonschema:"The candidate action graph. Same shape create_action accepts."`
}

type validateActionOut struct {
	Errors actioncatalog.ValidationErrors `json:"errors"`
}

type createActionArgs struct {
	WorkspaceID int                 `json:"workspace_id" jsonschema:"Workspace to create the action in. Caller must have action.manage on this workspace."`
	Action      actionDefinitionArg `json:"action" jsonschema:"The action graph to create. Validate it via validate_action first if you're unsure."`
}

type createActionOut struct {
	ID            int                      `json:"id"`
	WorkspaceID   int                      `json:"workspace_id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description,omitempty"`
	IsEnabled     bool                     `json:"is_enabled"`
	TriggerType   models.ActionTriggerType `json:"trigger_type"`
	TriggerConfig string                   `json:"trigger_config,omitempty"`
	NodeCount     int                      `json:"node_count"`
	EdgeCount     int                      `json:"edge_count"`
}

type getActionArgs struct {
	WorkspaceID int `json:"workspace_id" jsonschema:"Workspace the action lives in. Mismatches return 'action not found' (no cross-workspace probing)."`
	ActionID    int `json:"action_id" jsonschema:"ID of the action to fetch"`
}

type actionNodeOut struct {
	ID         int                   `json:"id"`
	NodeType   models.ActionNodeType `json:"node_type"`
	NodeConfig string                `json:"node_config"`
	PositionX  float64               `json:"position_x,omitempty"`
	PositionY  float64               `json:"position_y,omitempty"`
}

type actionEdgeOut struct {
	ID           int    `json:"id"`
	SourceNodeID int    `json:"source_node_id"`
	TargetNodeID int    `json:"target_node_id"`
	EdgeType     string `json:"edge_type,omitempty"`
}

type getActionOut struct {
	ID            int                      `json:"id"`
	WorkspaceID   int                      `json:"workspace_id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description,omitempty"`
	IsEnabled     bool                     `json:"is_enabled"`
	TriggerType   models.ActionTriggerType `json:"trigger_type"`
	TriggerConfig string                   `json:"trigger_config,omitempty"`
	Nodes         []actionNodeOut          `json:"nodes"`
	Edges         []actionEdgeOut          `json:"edges"`
}

type updateActionArgs struct {
	WorkspaceID int                 `json:"workspace_id" jsonschema:"Workspace the action lives in. Must match the action's stored workspace; mismatches return 'workspace not found' to avoid leaking that a cross-workspace id exists."`
	ActionID    int                 `json:"action_id" jsonschema:"ID of the action to replace"`
	Action      actionDefinitionArg `json:"action" jsonschema:"Full replacement graph. Updates are not partial — call get_action first to read the existing graph, mutate, and send the complete definition back. Anything you omit is deleted."`
}

type listActionTemplatesArgs struct{}

type templateDTO struct {
	Key         string                   `json:"key"`
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Category    string                   `json:"category,omitempty"`
	TriggerType models.ActionTriggerType `json:"trigger_type"`
	NodeCount   int                      `json:"node_count"`
}

type listActionTemplatesOut struct {
	Templates []templateDTO `json:"templates"`
}

func init() {
	Register(Default, Tool[describeActionCatalogArgs]{
		Name:        "describe_action_catalog",
		Group:       CapabilityActions,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "Discover what kinds of actions can be built in a workspace. Returns every trigger type and every node type with its JSON Schema config, plus the action capabilities (LLM connections, HTTP clients, docker environments) reachable from this workspace. Call this before constructing an action so you know exactly what configs each node accepts.",
		Scopes:      []string{auth.ScopeActionsRead},
		Run: func(_ context.Context, env *Env, args describeActionCatalogArgs) (any, error) {
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, args.WorkspaceID, models.PermissionActionManage)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			cat := actioncatalog.Default()
			out := catalogOut{Scope: "workspace"}
			for _, t := range cat.Triggers() {
				schemaJSON, _ := marshalSchema(t.ConfigSchema)
				out.Triggers = append(out.Triggers, catalogTriggerDTO{
					Type:         t.Type,
					Label:        t.Label,
					Description:  t.Description,
					ConfigSchema: schemaJSON,
				})
			}
			for _, n := range cat.Nodes() {
				schemaJSON, _ := marshalSchema(n.ConfigSchema)
				out.Nodes = append(out.Nodes, catalogNodeDTO{
					Type:         n.Type,
					Label:        n.Label,
					Description:  n.Description,
					Category:     n.Category,
					ConfigSchema: schemaJSON,
					IsIterator:   n.IsIterator,
					Outputs:      n.Outputs,
				})
			}
			repo := repository.NewActionRepository(env.DB)
			caps, err := repo.ListCapabilitiesForWorkspace(args.WorkspaceID, "")
			if err != nil {
				return nil, fmt.Errorf("list capabilities: %w", err)
			}
			for _, c := range caps {
				out.Capabilities = append(out.Capabilities, catalogCapabilityDTO{
					ID:             c.ID,
					Name:           c.Name,
					CapabilityType: c.CapabilityType,
				})
			}
			if out.Capabilities == nil {
				out.Capabilities = []catalogCapabilityDTO{}
			}
			return out, nil
		},
	})

	Register(Default, Tool[validateActionArgs]{
		Name:        "validate_action",
		Group:       CapabilityActions,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "Dry-run validate an action definition without persisting. Returns a structured list of errors (empty when the definition would persist successfully). Use this to iterate before calling create_action.",
		Scopes:      []string{auth.ScopeActionsRead},
		Run: func(_ context.Context, env *Env, args validateActionArgs) (any, error) {
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, args.WorkspaceID, models.PermissionActionManage)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			resolver := capabilityResolverForEnv{repo: repository.NewActionRepository(env.DB)}
			errs := actioncatalog.Validate(actioncatalog.Default(), args.Action.toDefinition(), args.WorkspaceID, resolver)
			if errs == nil {
				errs = actioncatalog.ValidationErrors{}
			}
			return validateActionOut{Errors: errs}, nil
		},
	})

	Register(Default, Tool[createActionArgs]{
		Name:        "create_action",
		Group:       CapabilityActions,
		Access:      AccessWrite,
		Risk:        RiskHigh,
		Description: "Create a new automation action in a workspace from a complete graph definition. The definition is validated against the catalog (schema shapes, edge references, cycles, iterator-body containment, capability scope) before persisting. Returns the created action's summary; the new action is enabled and starts firing immediately.",
		Scopes:      []string{auth.ScopeActionsWrite},
		Run: func(_ context.Context, env *Env, args createActionArgs) (any, error) {
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, args.WorkspaceID, models.PermissionActionManage)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			def := args.Action.toDefinition()
			repo := repository.NewActionRepository(env.DB)
			resolver := capabilityResolverForEnv{repo: repo}
			if errs := actioncatalog.Validate(actioncatalog.Default(), def, args.WorkspaceID, resolver); len(errs) > 0 {
				return map[string]any{"error": errs[0].Message, "validation_errors": errs}, nil
			}
			if existing, err := findExistingEquivalentAction(repo, args.WorkspaceID, def, env.UserID); err != nil {
				return nil, fmt.Errorf("check duplicate action: %w", err)
			} else if existing != nil {
				return createActionOut{
					ID:            existing.ID,
					WorkspaceID:   existing.WorkspaceID,
					Name:          existing.Name,
					Description:   existing.Description,
					IsEnabled:     existing.IsEnabled,
					TriggerType:   existing.TriggerType,
					TriggerConfig: existing.TriggerConfig,
					NodeCount:     len(existing.Nodes),
					EdgeCount:     len(existing.Edges),
				}, nil
			}

			action := &models.Action{
				WorkspaceID:   args.WorkspaceID,
				Name:          def.Name,
				Description:   def.Description,
				IsEnabled:     true,
				TriggerType:   def.TriggerType,
				TriggerConfig: def.TriggerConfig,
				CreatedBy:     &env.UserID,
			}
			definitions := services.NewActionDefinitionService(env.DB, repo, env.ActionService)
			created, validation, err := definitions.Create(action, def.Nodes, def.Edges, env.actionWriteMeta(logger.ActionAutomationCreate, map[string]any{"workspace_id": args.WorkspaceID}))
			if len(validation) > 0 {
				return map[string]any{"error": validation[0].Message, "validation_errors": validation}, nil
			}
			if err != nil {
				return nil, fmt.Errorf("create action: %w", err)
			}
			return createActionOut{
				ID:            created.ID,
				WorkspaceID:   created.WorkspaceID,
				Name:          created.Name,
				Description:   created.Description,
				IsEnabled:     created.IsEnabled,
				TriggerType:   created.TriggerType,
				TriggerConfig: created.TriggerConfig,
				NodeCount:     len(created.Nodes),
				EdgeCount:     len(created.Edges),
			}, nil
		},
	})

	Register(Default, Tool[getActionArgs]{
		Name:        "get_action",
		Group:       CapabilityActions,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "Fetch the full definition of an existing action — trigger, trigger config, every node with its node_config, and every edge. Use this before update_action so you know exactly what graph you are replacing. Caller must have action.manage on the workspace.",
		Scopes:      []string{auth.ScopeActionsRead},
		Run: func(_ context.Context, env *Env, args getActionArgs) (any, error) {
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, args.WorkspaceID, models.PermissionActionManage)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			repo := repository.NewActionRepository(env.DB)
			action, err := repo.GetByID(args.ActionID)
			if err != nil || action == nil || action.WorkspaceID != args.WorkspaceID {
				return map[string]string{"error": "action not found"}, nil //nolint:nilerr // intentional: don't leak existence of cross-workspace actions
			}
			out := getActionOut{
				ID:            action.ID,
				WorkspaceID:   action.WorkspaceID,
				Name:          action.Name,
				Description:   action.Description,
				IsEnabled:     action.IsEnabled,
				TriggerType:   action.TriggerType,
				TriggerConfig: action.TriggerConfig,
				Nodes:         make([]actionNodeOut, 0, len(action.Nodes)),
				Edges:         make([]actionEdgeOut, 0, len(action.Edges)),
			}
			for _, n := range action.Nodes {
				out.Nodes = append(out.Nodes, actionNodeOut{
					ID:         n.ID,
					NodeType:   n.NodeType,
					NodeConfig: n.NodeConfig,
					PositionX:  n.PositionX,
					PositionY:  n.PositionY,
				})
			}
			for _, e := range action.Edges {
				out.Edges = append(out.Edges, actionEdgeOut{
					ID:           e.ID,
					SourceNodeID: e.SourceNodeID,
					TargetNodeID: e.TargetNodeID,
					EdgeType:     e.EdgeType,
				})
			}
			return out, nil
		},
	})

	Register(Default, Tool[updateActionArgs]{
		Name:        "update_action",
		Group:       CapabilityActions,
		Access:      AccessWrite,
		Risk:        RiskHigh,
		Description: "Replace an existing action's full definition. The new graph is validated against the catalog before persisting (same checks as create_action). On success the workspace action cache is invalidated so the new automation takes effect immediately, and any in-app editor open on this action live-reloads. Caller must have action.manage on the workspace.",
		Scopes:      []string{auth.ScopeActionsWrite},
		Run: func(_ context.Context, env *Env, args updateActionArgs) (any, error) {
			if !env.HasWorkspaceAccess(args.WorkspaceID) {
				return map[string]string{"error": "workspace not found"}, nil
			}
			ok, err := env.PermService.HasWorkspacePermission(env.UserID, args.WorkspaceID, models.PermissionActionManage)
			if err != nil {
				return nil, err
			}
			if !ok {
				return map[string]string{"error": "permission denied"}, nil
			}
			repo := repository.NewActionRepository(env.DB)
			existing, err := repo.GetByID(args.ActionID)
			if err != nil || existing == nil || existing.WorkspaceID != args.WorkspaceID {
				// 404 disclosure rule: same response for "not found", "wrong
				// workspace", and "soft permission". An LLM should not be able
				// to probe action ids by varying workspace_id.
				return map[string]string{"error": "action not found"}, nil //nolint:nilerr // intentional: don't leak existence of cross-workspace actions
			}

			def := args.Action.toDefinition()
			resolver := capabilityResolverForEnv{repo: repo}
			if errs := actioncatalog.Validate(actioncatalog.Default(), def, args.WorkspaceID, resolver); len(errs) > 0 {
				return map[string]any{"error": errs[0].Message, "validation_errors": errs}, nil
			}

			// Replacing the executable configuration of an action that runs under
			// a configured actor is delegated code execution: action.manage alone
			// is not enough on any adapter. The authorizer runs inside the write
			// transaction against the locked actor.
			existing.Name = def.Name
			existing.Description = def.Description
			existing.TriggerType = def.TriggerType
			existing.TriggerConfig = def.TriggerConfig

			definitions := services.NewActionDefinitionService(env.DB, repo, env.ActionService)
			meta := env.actionWriteMeta(logger.ActionAutomationUpdate, map[string]any{"workspace_id": args.WorkspaceID})
			meta.AuthorizeActor = func(_ *int, effectiveActor *int, actorChanging bool) error {
				if !actorChanging && effectiveActor == nil {
					return nil
				}
				setActor, permErr := env.PermService.HasGlobalPermission(env.UserID, models.PermissionActionSetActor)
				if permErr != nil {
					return permErr
				}
				if setActor {
					return nil
				}
				return &services.PrivilegedActionEditDenied{
					AttemptedFields:    []string{"definition"},
					RequiredPermission: models.PermissionActionSetActor,
				}
			}
			updated, validation, err := definitions.Save(existing, def.Nodes, def.Edges, true, true, models.ActionActorUpdate{}, meta)
			if len(validation) > 0 {
				return map[string]any{"error": validation[0].Message, "validation_errors": validation}, nil
			}
			if err != nil {
				var denied *services.PrivilegedActionEditDenied
				if errors.As(err, &denied) {
					return map[string]string{"error": "action.set_actor permission required to edit a privileged action"}, nil
				}
				return nil, fmt.Errorf("save action: %w", err)
			}
			return createActionOut{
				ID:            updated.ID,
				WorkspaceID:   updated.WorkspaceID,
				Name:          updated.Name,
				Description:   updated.Description,
				IsEnabled:     updated.IsEnabled,
				TriggerType:   updated.TriggerType,
				TriggerConfig: updated.TriggerConfig,
				NodeCount:     len(updated.Nodes),
				EdgeCount:     len(updated.Edges),
			}, nil
		},
	})

	Register(Default, Tool[listActionTemplatesArgs]{
		Name:        "list_action_templates",
		Group:       CapabilityActions,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List the embedded action templates shipped with the server. These are known-good blueprints you can start from rather than constructing a graph from scratch. Use the apply endpoint (or copy the template's nodes/edges into create_action) to instantiate one.",
		Scopes:      []string{auth.ScopeActionsRead},
		Run: func(_ context.Context, _ *Env, _ listActionTemplatesArgs) (any, error) {
			tmpls := actiontemplates.Registry()
			out := listActionTemplatesOut{Templates: make([]templateDTO, 0, len(tmpls))}
			for _, t := range tmpls {
				out.Templates = append(out.Templates, templateDTO{
					Key:         t.Key,
					Name:        t.Name,
					Description: t.Description,
					Category:    t.Category,
					TriggerType: t.TriggerType,
					NodeCount:   len(t.Nodes),
				})
			}
			return out, nil
		},
	})
}

func marshalSchema(s *jsonschema.Schema) (json.RawMessage, error) {
	if s == nil {
		return json.RawMessage(`{}`), nil
	}
	return json.Marshal(s)
}
