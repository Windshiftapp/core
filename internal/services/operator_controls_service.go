package services

import (
	"context"
	"time"

	"windshift/internal/blobstore"
	"windshift/internal/database"
	"windshift/internal/operatorcontrols"
)

type OperatorBlockedDefinition struct {
	Kind string `json:"kind"`
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type OperatorControlsStatus struct {
	Policy                      operatorcontrols.Snapshot   `json:"policy"`
	Usage                       map[string]int64            `json:"usage"`
	BlockedDefinitions          []OperatorBlockedDefinition `json:"blocked_definitions"`
	BlockedDefinitionsTruncated bool                        `json:"blocked_definitions_truncated"`
}

type OperatorControlsService struct {
	db      database.Database
	storage *blobstore.Store
}

func NewOperatorControlsService(db database.Database, stores ...*blobstore.Store) *OperatorControlsService {
	s := &OperatorControlsService{db: db}
	if len(stores) > 0 {
		s.storage = stores[0]
	}
	return s
}

// Status projects non-secret deployment policy and migration blockers without
// modifying persisted organization preferences or executable definitions.
func (s *OperatorControlsService) Status(ctx context.Context) (OperatorControlsStatus, error) {
	controls := operatorcontrols.From(s.db)
	status := OperatorControlsStatus{Policy: controls.Snapshot(), Usage: map[string]int64{}, BlockedDefinitions: []OperatorBlockedDefinition{}}
	for resource, interval := range map[string]time.Duration{"requests": time.Minute, "automation_steps": 24 * time.Hour, "mail_recipients": 24 * time.Hour, "ai_tokens": 24 * time.Hour} {
		used, err := controls.Used(ctx, resource, interval)
		if err != nil {
			return status, err
		}
		status.Usage[resource] = used
	}
	if controls.Limits().BlobBytes > 0 && s.storage.Enabled() {
		usage, err := s.storage.Usage(ctx)
		if err != nil {
			return status, err
		}
		status.Usage["blob_bytes"] = usage.Bytes
	}
	if !controls.Allowed(operatorcontrols.Scripts) {
		rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT 'condition_set' AS kind, cs.id AS definition_id, cs.name FROM condition_sets cs
			JOIN condition_set_transitions transitions ON transitions.condition_set_id=cs.id
			JOIN conditions conditions ON conditions.condition_set_transition_id=transitions.id
			WHERE conditions.condition_type='script'
			UNION SELECT DISTINCT 'action' AS kind, a.id, a.name FROM actions a JOIN action_nodes n ON n.action_id=a.id WHERE n.node_type='execute_script'
			UNION SELECT DISTINCT 'asset_action' AS kind, a.id, a.name FROM asset_actions a JOIN asset_action_nodes n ON n.action_id=a.id WHERE n.node_type='execute_script'
			ORDER BY kind, definition_id LIMIT 1001`)
		if err != nil {
			return status, err
		}
		defer rows.Close()
		for rows.Next() {
			definition := OperatorBlockedDefinition{}
			if err := rows.Scan(&definition.Kind, &definition.ID, &definition.Name); err != nil {
				return status, err
			}
			status.BlockedDefinitions = append(status.BlockedDefinitions, definition)
		}
		if err := rows.Err(); err != nil {
			return status, err
		}
		if len(status.BlockedDefinitions) > 1000 {
			status.BlockedDefinitions = status.BlockedDefinitions[:1000]
			status.BlockedDefinitionsTruncated = true
		}
	}
	return status, nil
}
