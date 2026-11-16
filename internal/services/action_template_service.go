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
	db        database.Database
	repo      *repository.ActionRepository
	revisions *repository.ActionRevisionRepository
}

// NewActionTemplateService constructs a template service backed by the
// shared action repository.
func NewActionTemplateService(db database.Database, repo *repository.ActionRepository) *ActionTemplateService {
	return &ActionTemplateService{
		db:        db,
		repo:      repo,
		revisions: repository.NewActionRevisionRepository(db),
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
	meta ActionWriteMeta,
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

	// Marshal and cross-check the immutable blueprint before opening the write
	// transaction. The registry validates at startup too, but keeping the
	// materializer self-contained makes it safe if another registry source is
	// introduced later.
	type pendingNode struct {
		yamlID     string
		nodeType   models.ActionNodeType
		nodeConfig string
		positionX  float64
		positionY  float64
	}
	pendingNodes := make([]pendingNode, 0, len(tmpl.Nodes))
	knownNodeIDs := make(map[string]struct{}, len(tmpl.Nodes))
	for _, tn := range tmpl.Nodes {
		if _, exists := knownNodeIDs[tn.ID]; exists {
			return nil, fmt.Errorf("template contains duplicate node id %q", tn.ID)
		}
		knownNodeIDs[tn.ID] = struct{}{}
		nodeConfigJSON, err := json.Marshal(tn.NodeConfig)
		if err != nil {
			return nil, fmt.Errorf("marshal node %q config: %w", tn.ID, err)
		}
		pendingNodes = append(pendingNodes, pendingNode{
			yamlID:     tn.ID,
			nodeType:   models.ActionNodeType(tn.NodeType),
			nodeConfig: string(nodeConfigJSON),
			positionX:  tn.Position.X,
			positionY:  tn.Position.Y,
		})
	}
	for _, te := range tmpl.Edges {
		if _, ok := knownNodeIDs[te.SourceNodeID]; !ok {
			return nil, fmt.Errorf("edge references unknown node id %q", te.SourceNodeID)
		}
		if _, ok := knownNodeIDs[te.TargetNodeID]; !ok {
			return nil, fmt.Errorf("edge references unknown node id %q", te.TargetNodeID)
		}
	}

	var actionID int
	err = database.WithTx(s.db, func(tx database.Tx) error {
		now := time.Now()
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO actions (
				workspace_id, name, description, is_enabled, trigger_type,
				trigger_config, created_by, template_key, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
		`, workspaceID, tmpl.Name, tmpl.Description, true, tmpl.TriggerType,
			string(triggerConfigJSON), creator, tmpl.Key, now, now,
		).Scan(&actionID); err != nil {
			return fmt.Errorf("create action: %w", err)
		}

		// Track YAML ID → DB ID so edges can be rewritten inside the same
		// transaction as the parent and nodes.
		nodeIDByYAML := make(map[string]int, len(pendingNodes))
		for _, node := range pendingNodes {
			var nodeID int
			if err := tx.QueryRowContext(ctx, `
				INSERT INTO action_nodes (
					action_id, node_type, node_config, position_x, position_y,
					created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
			`, actionID, node.nodeType, node.nodeConfig, node.positionX, node.positionY, now, now).Scan(&nodeID); err != nil {
				return fmt.Errorf("create node %q: %w", node.yamlID, err)
			}
			nodeIDByYAML[node.yamlID] = nodeID
		}

		for _, edge := range tmpl.Edges {
			edgeType := edge.EdgeType
			if edgeType == "" {
				edgeType = "default"
			}
			if _, err := tx.ExecWriteContext(ctx, `
				INSERT INTO action_edges (
					action_id, source_node_id, target_node_id, edge_type, created_at
				) VALUES (?, ?, ?, ?, ?)
			`, actionID, nodeIDByYAML[edge.SourceNodeID], nodeIDByYAML[edge.TargetNodeID], edgeType, now); err != nil {
				return fmt.Errorf("create edge %s→%s: %w", edge.SourceNodeID, edge.TargetNodeID, err)
			}
		}

		// Snapshot the materialized definition and record its audit row in the
		// same transaction as the template instantiation.
		action := &models.Action{
			ID: actionID, WorkspaceID: workspaceID, Name: tmpl.Name, Description: tmpl.Description,
			IsEnabled: true, TriggerType: tmpl.TriggerType, TriggerConfig: string(triggerConfigJSON),
			CreatedBy: creator,
		}
		materializedNodes, err := s.repo.GetNodesByActionIDTx(tx, actionID)
		if err != nil {
			return err
		}
		materializedEdges, err := s.repo.GetEdgesByActionIDTx(tx, actionID)
		if err != nil {
			return err
		}
		if meta.Source == "" {
			meta.Source = models.ActionRevisionSourceTemplate
		}
		if err := persistActionRevisionTx(ctx, tx, s.revisions, action, materializedNodes, materializedEdges, meta.Source, meta); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ApplyToWorkspaceResult{
		ActionID:    actionID,
		WorkspaceID: workspaceID,
		TemplateKey: tmpl.Key,
		Name:        tmpl.Name,
	}, nil
}
