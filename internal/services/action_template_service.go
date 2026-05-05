package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services/actiontemplates"
)

// ActionTemplateService wraps the embedded template registry with the
// "instantiate this template into a workspace" operation. The registry
// itself is read-only and shipped with the binary; this service is the
// only place that writes derived rows.
type ActionTemplateService struct {
	db      database.Database
	actions *repository.ActionRepository
}

// NewActionTemplateService constructs a template service backed by the
// shared action repository.
func NewActionTemplateService(db database.Database) *ActionTemplateService {
	return &ActionTemplateService{
		db:      db,
		actions: repository.NewActionRepository(db),
	}
}

// ApplyToWorkspaceResult describes the action created from a template.
type ApplyToWorkspaceResult struct {
	ActionID    int    `json:"action_id"`
	WorkspaceID int    `json:"workspace_id"`
	TemplateKey string `json:"template_key"`
	Name        string `json:"name"`
}

// ApplyToWorkspace snapshot-copies the template into the workspace as a new
// Action. The blueprint's node and edge graph is materialized: each
// template node becomes a fresh action_nodes row, each template edge a
// fresh action_edges row referencing the new node IDs. The action is
// stamped with template_key for lineage display.
//
// Workspace-specific references (user IDs, channel IDs) are NOT supported
// in v1 — the registry validator rejects templates that contain such
// placeholders. The first template (close_subtasks_on_parent_close) needs
// none. Future templates that do will introduce a parameters block.
func (s *ActionTemplateService) ApplyToWorkspace(
	ctx context.Context,
	templateKey string,
	workspaceID int,
	creatorUserID int,
) (*ApplyToWorkspaceResult, error) {
	tmpl, ok := actiontemplates.Get(templateKey)
	if !ok {
		return nil, fmt.Errorf("template not found: %q", templateKey)
	}

	triggerConfigJSON, err := json.Marshal(tmpl.TriggerConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal trigger_config: %w", err)
	}

	creator := &creatorUserID
	if creatorUserID <= 0 {
		creator = nil
	}

	action := &models.Action{
		WorkspaceID:   workspaceID,
		Name:          tmpl.Name,
		Description:   tmpl.Description,
		IsEnabled:     true,
		TriggerType:   tmpl.TriggerType,
		TriggerConfig: string(triggerConfigJSON),
		CreatedBy:     creator,
	}

	actionID, err := s.actions.Create(action)
	if err != nil {
		return nil, fmt.Errorf("create action: %w", err)
	}

	// Stamp template_key for lineage. Done as a follow-up UPDATE so we don't
	// have to thread template_key through ActionRepository.Create's signature
	// (the column is an automation-history concern, not part of the action's
	// runtime semantics).
	if _, err := s.db.Exec(`UPDATE actions SET template_key = ? WHERE id = ?`, tmpl.Key, actionID); err != nil {
		return nil, fmt.Errorf("stamp template_key: %w", err)
	}

	// Materialize nodes. Track yaml-id → DB-id so edges can be rewritten.
	nodeIDByYAML := make(map[string]int, len(tmpl.Nodes))
	for _, tn := range tmpl.Nodes {
		nodeConfigJSON, err := json.Marshal(tn.NodeConfig)
		if err != nil {
			return nil, fmt.Errorf("marshal node %q config: %w", tn.ID, err)
		}
		dbNode := &models.ActionNode{
			ActionID:   actionID,
			NodeType:   models.ActionNodeType(tn.NodeType),
			NodeConfig: string(nodeConfigJSON),
			PositionX:  tn.Position.X,
			PositionY:  tn.Position.Y,
		}
		newID, err := s.actions.CreateNode(dbNode)
		if err != nil {
			return nil, fmt.Errorf("create node %q: %w", tn.ID, err)
		}
		nodeIDByYAML[tn.ID] = newID
	}

	for _, te := range tmpl.Edges {
		srcID, ok := nodeIDByYAML[te.SourceNodeID]
		if !ok {
			return nil, fmt.Errorf("edge references unknown node id %q", te.SourceNodeID)
		}
		dstID, ok := nodeIDByYAML[te.TargetNodeID]
		if !ok {
			return nil, fmt.Errorf("edge references unknown node id %q", te.TargetNodeID)
		}
		edgeType := te.EdgeType
		if edgeType == "" {
			edgeType = "default"
		}
		dbEdge := &models.ActionEdge{
			ActionID:     actionID,
			SourceNodeID: srcID,
			TargetNodeID: dstID,
			EdgeType:     edgeType,
		}
		if _, err := s.actions.CreateEdge(dbEdge); err != nil {
			return nil, fmt.Errorf("create edge %s→%s: %w", te.SourceNodeID, te.TargetNodeID, err)
		}
	}

	_ = ctx // ctx reserved for future audit-log call
	_ = time.Now

	return &ApplyToWorkspaceResult{
		ActionID:    actionID,
		WorkspaceID: workspaceID,
		TemplateKey: tmpl.Key,
		Name:        tmpl.Name,
	}, nil
}
