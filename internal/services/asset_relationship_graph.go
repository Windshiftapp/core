package services

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

const (
	assetGraphMaxNodes     = 100
	assetGraphMaxHops      = 2
	assetGraphMaxNeighbors = 50
	assetGraphMaxFields    = 50
	assetGraphTimeout      = 2 * time.Second
)

type assetGraphReference struct {
	entityType string
	entityID   int
	title      string
	fieldName  string
}

func (s *AssetApplicationService) RelationshipGraph(ctx context.Context, userID, assetID int) (models.RelationshipGraphResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.RelationshipGraphResponse{}, err
	}
	readCtx, cancel := context.WithTimeout(ctx, assetGraphTimeout)
	defer cancel()
	budget := &assetGraphReadBudget{Database: s.db, ctx: readCtx, cancel: cancel}
	request := *s
	request.db = budget
	request.repo = repository.NewAssetRepository(budget)
	request.assetPermissions = NewAssetPermissionService(request.repo, s.permissions)
	s = &request
	asset, err := s.repo.GetAssetByID(assetID)
	if err != nil {
		return models.RelationshipGraphResponse{}, err
	}
	if err := s.require(userID, asset.SetID, AssetPermissionKeyView); err != nil {
		return models.RelationshipGraphResponse{}, err
	}
	workspaceIDs, err := s.permissions.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return models.RelationshipGraphResponse{}, err
	}
	workspaces := make(map[int]bool, len(workspaceIDs))
	for _, id := range workspaceIDs {
		workspaces[id] = true
	}
	setAccess := map[int]bool{asset.SetID: true}
	canViewSet := func(setID int) bool {
		if allowed, ok := setAccess[setID]; ok {
			return allowed
		}
		allowed := s.require(userID, setID, AssetPermissionKeyView) == nil
		setAccess[setID] = allowed
		return allowed
	}

	fields, fieldsTruncated, err := s.assetReferenceFields()
	if err != nil {
		return models.RelationshipGraphResponse{}, err
	}
	entityAccess := map[string]bool{fmt.Sprintf("asset-%d", assetID): true}
	canAccess := func(entityType string, id int) bool {
		key := fmt.Sprintf("%s-%d", entityType, id)
		if allowed, ok := entityAccess[key]; ok {
			return allowed
		}
		allowed := s.canAccessAssetGraphEntity(userID, entityType, id, workspaces, canViewSet)
		entityAccess[key] = allowed
		return allowed
	}
	type queueEntry struct {
		key, entityType string
		entityID, hop   int
	}
	key := func(entityType string, entityID int) string { return fmt.Sprintf("%s-%d", entityType, entityID) }
	originKey := key("asset", assetID)
	queue := []queueEntry{{key: originKey, entityType: "asset", entityID: assetID}}
	visited := map[string]bool{originKey: true}
	nodes := map[string]*models.RelationshipGraphNode{originKey: {ID: originKey, EntityID: assetID, Type: "asset", Title: asset.Title, IsOrigin: true}}
	edges, seenEdges, edgeNumber, truncated := make([]models.RelationshipGraphEdge, 0), make(map[string]bool), 0, fieldsTruncated
	addNode := func(nodeKey, entityType string, entityID int, title string, hop int) {
		if visited[nodeKey] {
			return
		}
		if len(nodes) >= assetGraphMaxNodes {
			truncated = true
			return
		}
		visited[nodeKey] = true
		nodes[nodeKey] = &models.RelationshipGraphNode{ID: nodeKey, EntityID: entityID, Type: entityType, Title: title, Hop: hop}
		queue = append(queue, queueEntry{key: nodeKey, entityType: entityType, entityID: entityID, hop: hop})
	}
	addEdge := func(source, target, label, edgeType, color string) {
		dedup := source + ":" + target + ":" + label + ":" + edgeType
		if seenEdges[dedup] || nodes[source] == nil || nodes[target] == nil {
			return
		}
		seenEdges[dedup] = true
		edgeNumber++
		edges = append(edges, models.RelationshipGraphEdge{ID: fmt.Sprintf("e%d", edgeNumber), Source: source, Target: target, Label: label, Color: color, EdgeType: edgeType})
	}

	for len(queue) > 0 {
		if readCtx.Err() != nil || len(nodes) >= assetGraphMaxNodes {
			truncated = true
			break
		}

		current := queue[0]
		queue = queue[1:]
		if current.hop >= assetGraphMaxHops {
			continue
		}
		candidates := &assetGraphCandidates{remaining: assetGraphMaxNeighbors}
		neighbors, err := s.assetGraphLinkNeighbors(current.entityType, current.entityID, candidates)
		if err != nil {
			if readCtx.Err() != nil {
				truncated = true
				break
			}
			return models.RelationshipGraphResponse{}, err
		}
		for _, neighbor := range neighbors {
			if !canAccess(neighbor.entityType, neighbor.entityID) {
				continue
			}
			nodeKey := key(neighbor.entityType, neighbor.entityID)
			addNode(nodeKey, neighbor.entityType, neighbor.entityID, neighbor.title, current.hop+1)
			addEdge(current.key, nodeKey, neighbor.fieldName, "link", neighbor.color)
		}
		truncated = truncated || candidates.truncated
		if len(nodes) >= assetGraphMaxNodes {
			truncated = true
			break
		}
		if current.entityType != "asset" {
			continue
		}
		incoming, err := s.assetGraphIncomingFieldReferences(current.entityID, workspaces, canViewSet, fields, candidates)
		if err != nil {
			if readCtx.Err() != nil {
				truncated = true
				break
			}
			return models.RelationshipGraphResponse{}, err
		}
		for _, ref := range incoming {
			nodeKey := key(ref.entityType, ref.entityID)
			addNode(nodeKey, ref.entityType, ref.entityID, ref.title, current.hop+1)
			addEdge(nodeKey, current.key, "Field: "+ref.fieldName, "field_reference", "")
		}
		outgoing, err := s.assetGraphOutgoingFieldReferences(current.entityID, canViewSet, fields, candidates)
		if err != nil {
			if readCtx.Err() != nil {
				truncated = true
				break
			}
			return models.RelationshipGraphResponse{}, err
		}
		for _, ref := range outgoing {
			nodeKey := key(ref.entityType, ref.entityID)
			addNode(nodeKey, ref.entityType, ref.entityID, ref.title, current.hop+1)
			addEdge(current.key, nodeKey, "Field: "+ref.fieldName, "field_reference", "")
		}
		truncated = truncated || candidates.truncated
	}

	result := make([]models.RelationshipGraphNode, 0, len(nodes))
	nodeKeys := make([]string, 0, len(nodes))
	for key := range nodes {
		nodeKeys = append(nodeKeys, key)
	}
	slices.Sort(nodeKeys)
	for _, key := range nodeKeys {
		node := nodes[key]
		if readCtx.Err() == nil {
			node.Metadata = s.assetGraphMetadata(node.Type, node.EntityID)
		}
		result = append(result, *node)
	}
	if err := ctx.Err(); err != nil {
		return models.RelationshipGraphResponse{}, err
	}
	return models.RelationshipGraphResponse{Nodes: result, Edges: edges, Truncated: truncated || budget.truncated || readCtx.Err() != nil, TotalCount: len(result)}, nil
}

type assetGraphNeighbor struct {
	entityType string
	entityID   int
	fieldName  string
	color      string
	title      string
}

func (s *AssetApplicationService) assetGraphLinkNeighbors(entityType string, entityID int, candidates *assetGraphCandidates) ([]assetGraphNeighbor, error) {
	queries := []string{
		`SELECT il.target_type, il.target_id, lt.forward_label, lt.color,
		 CASE WHEN il.target_type = 'asset' THEN (SELECT title FROM assets WHERE id = il.target_id)
		 WHEN il.target_type = 'test_case' THEN (SELECT title FROM test_cases WHERE id = il.target_id) ELSE '' END
		 FROM item_links il JOIN link_types lt ON il.link_type_id = lt.id
		 WHERE il.source_type = ? AND il.source_id = ?`,
		`SELECT il.source_type, il.source_id, lt.reverse_label, lt.color,
		 CASE WHEN il.source_type = 'asset' THEN (SELECT title FROM assets WHERE id = il.source_id)
		 WHEN il.source_type = 'test_case' THEN (SELECT title FROM test_cases WHERE id = il.source_id) ELSE '' END
		 FROM item_links il JOIN link_types lt ON il.link_type_id = lt.id
		 WHERE il.target_type = ? AND il.target_id = ?`,
	}
	result := make([]assetGraphNeighbor, 0)
	for _, query := range queries {
		if candidates.remaining == 0 {
			candidates.truncated = true
			break
		}
		rows, err := s.db.Query(query+" ORDER BY il.id LIMIT ?", entityType, entityID, candidates.remaining+1)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			if !candidates.take() {
				break
			}
			var neighbor assetGraphNeighbor
			if err := rows.Scan(&neighbor.entityType, &neighbor.entityID, &neighbor.fieldName, &neighbor.color, &neighbor.title); err != nil {
				_ = rows.Close()
				return nil, err
			}
			result = append(result, neighbor)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
	}
	itemIndexes, itemIDs := make([]int, 0), make([]int, 0)
	for i := range result {
		if result[i].entityType == "item" {
			itemIndexes, itemIDs = append(itemIndexes, i), append(itemIDs, result[i].entityID)
		}
	}
	if len(itemIDs) > 0 {
		titles, err := repository.NewItemRepository(s.db).GetTitles(itemIDs)
		if err != nil {
			return nil, err
		}
		for _, index := range itemIndexes {
			result[index].title = titles[result[index].entityID]
		}
	}
	return result, nil
}

func (s *AssetApplicationService) canAccessAssetGraphEntity(userID int, entityType string, entityID int, workspaces map[int]bool, canViewSet func(int) bool) bool {
	switch entityType {
	case "item":
		workspaceID, err := repository.NewItemRepository(s.db).GetWorkspaceID(entityID)
		return err == nil && workspaces[workspaceID]
	case "asset":
		setID, err := s.repo.GetAssetSetID(entityID)
		return err == nil && canViewSet(setID)
	case "test_case":
		var workspaceID int
		if err := s.db.QueryRow("SELECT workspace_id FROM test_cases WHERE id = ?", entityID).Scan(&workspaceID); err != nil {
			return false
		}
		allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, models.PermissionTestView)
		return err == nil && allowed
	default:
		return false
	}
}

type assetGraphField struct {
	id   int
	name string
}

func (s *AssetApplicationService) assetGraphIncomingFieldReferences(assetID int, workspaces map[int]bool, canViewSet func(int) bool, fields []assetGraphField, candidates *assetGraphCandidates) ([]assetGraphReference, error) {
	result := make([]assetGraphReference, 0)
	workspaceIDs := make([]int, 0, len(workspaces))
	for id := range workspaces {
		workspaceIDs = append(workspaceIDs, id)
	}
	slices.Sort(workspaceIDs)
	for _, field := range fields {
		for _, entity := range []string{"item", "asset"} {
			if candidates.remaining == 0 {
				candidates.truncated = true
				return result, nil
			}
			if entity == "item" && len(workspaceIDs) == 0 {
				continue
			}
			query := s.assetFieldReferenceQuery(strconv.Itoa(field.id), entity)
			idString := strconv.Itoa(assetID)
			args := []any{idString, idString, idString, idString}
			if entity == "item" {
				query += " AND a.workspace_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(workspaceIDs)), ",") + ")"
				for _, id := range workspaceIDs {
					args = append(args, id)
				}
			}
			query += " ORDER BY a.id LIMIT ?"
			args = append(args, candidates.remaining+1)
			rows, err := s.db.Query(query, args...)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				if !candidates.take() {
					break
				}
				var id, scopeID int
				var title string
				if err := rows.Scan(&id, &title, &scopeID); err != nil {
					_ = rows.Close()
					return nil, err
				}
				if entity == "item" || (id != assetID && canViewSet(scopeID)) {
					result = append(result, assetGraphReference{entityType: entity, entityID: id, title: title, fieldName: field.name})
				}
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func (s *AssetApplicationService) assetFieldReferenceQuery(fieldKey, entity string) string {
	table, scope, title := "assets", "set_id", "a.title"
	if entity == "item" {
		table, scope, title = "items", "workspace_id", "a.title"
	}
	prefix := fmt.Sprintf("SELECT a.id, %s, a.%s FROM %s a WHERE (", title, scope, table)
	if s.db.GetDriverName() == "postgres" {
		return prefix + fmt.Sprintf(`a.custom_field_values->>'%s' = ? OR a.custom_field_values->'%s'->>'id' = ? OR EXISTS (
   SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(a.custom_field_values->'%s') = 'array' THEN a.custom_field_values->'%s' ELSE '[]'::jsonb END) elem
   WHERE elem #>> '{}' = ? OR elem->>'id' = ?))`, fieldKey, fieldKey, fieldKey, fieldKey)
	}
	return prefix + fmt.Sprintf(`CAST(NULLIF(a.custom_field_values,'') ->> '$."%s"' AS TEXT) = ? OR CAST(NULLIF(a.custom_field_values,'') ->> '$."%s".id' AS TEXT) = ? OR EXISTS (
  SELECT 1 FROM json_each(NULLIF(a.custom_field_values,'') -> '$."%s"') elem
  WHERE CAST(elem.value AS TEXT) = ? OR CASE WHEN elem.type = 'object' THEN CAST(elem.value ->> '$.id' AS TEXT) END = ?))`, fieldKey, fieldKey, fieldKey)
}

func (s *AssetApplicationService) assetGraphOutgoingFieldReferences(assetID int, canViewSet func(int) bool, fields []assetGraphField, candidates *assetGraphCandidates) ([]assetGraphReference, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	if candidates.remaining == 0 {
		candidates.truncated = true
		return nil, nil
	}
	var raw sql.NullString
	if err := s.db.QueryRow("SELECT custom_field_values FROM assets WHERE id = ?", assetID).Scan(&raw); err != nil {
		return nil, err
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	values := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(raw.String), &values); err != nil {
		return nil, err
	}
	result := make([]assetGraphReference, 0)
	for _, field := range fields {
		for _, id := range extractAssetReferenceIDs(values[strconv.Itoa(field.id)]) {
			if id == 0 || id == assetID {
				continue
			}
			if !candidates.take() {
				return result, nil
			}
			var title string
			var setID int
			err := s.db.QueryRow("SELECT title, set_id FROM assets WHERE id = ?", id).Scan(&title, &setID)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if canViewSet(setID) {
				result = append(result, assetGraphReference{entityType: "asset", entityID: id, title: title, fieldName: field.name})
			}
		}
	}
	return result, nil
}

func (s *AssetApplicationService) assetReferenceFields() ([]assetGraphField, bool, error) {
	rows, err := s.db.Query("SELECT id, name FROM custom_field_definitions WHERE field_type = 'asset' ORDER BY id LIMIT ?", assetGraphMaxFields+1)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	fields := make([]assetGraphField, 0)
	for rows.Next() {
		if len(fields) == assetGraphMaxFields {
			return fields, true, nil
		}
		var field assetGraphField
		if err := rows.Scan(&field.id, &field.name); err != nil {
			return nil, false, err
		}
		fields = append(fields, field)
	}
	return fields, false, rows.Err()
}

func extractAssetReferenceIDs(raw json.RawMessage) []int {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	if raw[0] != '[' {
		if id, ok := extractAssetReferenceIDRaw(raw); ok && id != 0 {
			return []int{id}
		}
		return nil
	}
	var values []any
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	result := make([]int, 0, len(values))
	for _, value := range values {
		if id, ok := extractAssetReferenceID(value); ok && id != 0 {
			result = append(result, id)
		}
	}
	return result
}

func extractAssetReferenceIDRaw(raw json.RawMessage) (int, bool) {
	var id int
	if json.Unmarshal(raw, &id) == nil {
		return id, true
	}
	var value struct {
		ID int `json:"id"`
	}
	if json.Unmarshal(raw, &value) == nil {
		return value.ID, true
	}
	return 0, false
}

func extractAssetReferenceID(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case map[string]any:
		return extractAssetReferenceID(typed["id"])
	default:
		return 0, false
	}
}

func (s *AssetApplicationService) assetGraphMetadata(entityType string, entityID int) map[string]any {
	metadata := make(map[string]any)
	switch entityType {
	case "item":
		if item, err := repository.NewItemRepository(s.db).GetItemGraphMetadata(entityID); err == nil {
			metadata["display_key"] = fmt.Sprintf("%s-%d", item.WorkspaceKey, item.WorkspaceItemNumber)
			metadata["workspace_id"] = item.WorkspaceID
			if item.StatusName != "" {
				metadata["status"] = item.StatusName
			}
		}
	case "asset":
		var setID int
		var status, assetType string
		if s.db.QueryRow(`SELECT a.set_id, COALESCE(s.name, ''), COALESCE(t.name, '') FROM assets a LEFT JOIN asset_statuses s ON a.status_id = s.id LEFT JOIN asset_types t ON a.asset_type_id = t.id WHERE a.id = ?`, entityID).Scan(&setID, &status, &assetType) == nil {
			metadata["set_id"] = setID
			if status != "" {
				metadata["status"] = status
			}
			if assetType != "" {
				metadata["asset_type"] = assetType
			}
		}
	case "test_case":
		var workspaceID int
		var workspaceKey string
		if s.db.QueryRow("SELECT tc.workspace_id, w.key FROM test_cases tc JOIN workspaces w ON tc.workspace_id = w.id WHERE tc.id = ?", entityID).Scan(&workspaceID, &workspaceKey) == nil {
			metadata["workspace_id"] = workspaceID
			metadata["workspace_key"] = workspaceKey
		}
	}
	return metadata
}
