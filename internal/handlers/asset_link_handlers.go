package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// requireAssetAccess authenticates the user, extracts the asset ID from the "id" route
// param, looks up the asset's set, and checks permission using the provided checker.
// Returns the user, asset ID, and true on success; writes the appropriate error
// response and returns false on failure.
func (h *AssetHandler) requireAssetAccess(w http.ResponseWriter, r *http.Request, checkPerm func(userID, setID int) (bool, error)) (*models.User, int, bool) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return nil, 0, false
	}

	assetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return nil, 0, false
	}

	var setID int
	err := h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "asset")
		return nil, 0, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, 0, false
	}

	allowed, err := checkPerm(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, 0, false
	}
	if !allowed {
		respondNotFound(w, r, "asset")
		return nil, 0, false
	}

	return currentUser, assetID, true
}

// requireAssetViewAccess authenticates the user, extracts the asset ID from the "id" route
// param, looks up the asset's set, and checks view permission.
func (h *AssetHandler) requireAssetViewAccess(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	return h.requireAssetAccess(w, r, h.canViewSet)
}

// requireAssetEditAccess authenticates the user, extracts the asset ID from the "id" route
// param, looks up the asset's set, and checks edit permission.
func (h *AssetHandler) requireAssetEditAccess(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	return h.requireAssetAccess(w, r, h.canEditSet)
}

// GetAssetLinks returns all links for an asset (incoming and outgoing)
func (h *AssetHandler) GetAssetLinks(w http.ResponseWriter, r *http.Request) {
	_, assetID, ok := h.requireAssetViewAccess(w, r)
	if !ok {
		return
	}

	// Get outgoing links (where this asset is the source). Item titles are
	// hydrated via ItemRepository.GetTitles after the main scan.
	outgoingQuery := `
		SELECT il.id, il.link_type_id, il.source_type, il.source_id, il.target_type, il.target_id,
		       il.created_by, il.created_at,
		       lt.name as link_type_name, lt.color as link_type_color, lt.forward_label, lt.reverse_label,
		       CASE
		           WHEN il.target_type = 'asset' THEN (SELECT title FROM assets WHERE id = il.target_id)
		           WHEN il.target_type = 'test_case' THEN (SELECT title FROM test_cases WHERE id = il.target_id)
		           ELSE ''
		       END as target_title,
		       COALESCE(u.username, '') as created_by_name
		FROM item_links il
		JOIN link_types lt ON il.link_type_id = lt.id
		LEFT JOIN users u ON il.created_by = u.id
		WHERE il.source_type = 'asset' AND il.source_id = ?
		ORDER BY lt.name, il.created_at DESC
	`

	outgoingRows, err := h.db.Query(outgoingQuery, assetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = outgoingRows.Close() }()

	var outgoingLinks []models.ItemLink
	var outgoingItemTargetIDs []int
	for outgoingRows.Next() {
		var link models.ItemLink
		err = outgoingRows.Scan(
			&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.CreatedBy, &link.CreatedAt,
			&link.LinkTypeName, &link.LinkTypeColor, &link.LinkTypeForwardLabel, &link.LinkTypeReverseLabel,
			&link.TargetTitle, &link.CreatedByName,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if link.TargetType == "item" {
			outgoingItemTargetIDs = append(outgoingItemTargetIDs, link.TargetID)
		}
		outgoingLinks = append(outgoingLinks, link)
	}
	if err := outgoingRows.Err(); err != nil {
		respondInternalError(w, r, fmt.Errorf("error iterating outgoing link rows: %w", err))
		return
	}

	if len(outgoingItemTargetIDs) > 0 {
		titles, err := repository.NewItemRepository(h.db).GetTitles(outgoingItemTargetIDs)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		for i := range outgoingLinks {
			if outgoingLinks[i].TargetType == "item" {
				outgoingLinks[i].TargetTitle = titles[outgoingLinks[i].TargetID]
			}
		}
	}

	// Get incoming links (where this asset is the target). Item titles are
	// hydrated via ItemRepository.GetTitles after the main scan.
	incomingQuery := `
		SELECT il.id, il.link_type_id, il.source_type, il.source_id, il.target_type, il.target_id,
		       il.created_by, il.created_at,
		       lt.name as link_type_name, lt.color as link_type_color, lt.forward_label, lt.reverse_label,
		       CASE
		           WHEN il.source_type = 'asset' THEN (SELECT title FROM assets WHERE id = il.source_id)
		           WHEN il.source_type = 'test_case' THEN (SELECT title FROM test_cases WHERE id = il.source_id)
		           ELSE ''
		       END as source_title,
		       COALESCE(u.username, '') as created_by_name
		FROM item_links il
		JOIN link_types lt ON il.link_type_id = lt.id
		LEFT JOIN users u ON il.created_by = u.id
		WHERE il.target_type = 'asset' AND il.target_id = ?
		ORDER BY lt.name, il.created_at DESC
	`

	incomingRows, err := h.db.Query(incomingQuery, assetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = incomingRows.Close() }()

	var incomingLinks []models.ItemLink
	var incomingItemSourceIDs []int
	for incomingRows.Next() {
		var link models.ItemLink
		err := incomingRows.Scan(
			&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.CreatedBy, &link.CreatedAt,
			&link.LinkTypeName, &link.LinkTypeColor, &link.LinkTypeForwardLabel, &link.LinkTypeReverseLabel,
			&link.SourceTitle, &link.CreatedByName,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if link.SourceType == "item" {
			incomingItemSourceIDs = append(incomingItemSourceIDs, link.SourceID)
		}
		incomingLinks = append(incomingLinks, link)
	}
	if err := incomingRows.Err(); err != nil {
		respondInternalError(w, r, fmt.Errorf("error iterating incoming link rows: %w", err))
		return
	}

	if len(incomingItemSourceIDs) > 0 {
		titles, err := repository.NewItemRepository(h.db).GetTitles(incomingItemSourceIDs)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		for i := range incomingLinks {
			if incomingLinks[i].SourceType == "item" {
				incomingLinks[i].SourceTitle = titles[incomingLinks[i].SourceID]
			}
		}
	}

	response := map[string]interface{}{
		"outgoing": outgoingLinks,
		"incoming": incomingLinks,
	}

	respondJSONOK(w, response)
}

// CreateAssetLinkRequest represents the request body for creating an asset link
type CreateAssetLinkRequest struct {
	LinkTypeID int    `json:"link_type_id"`
	TargetType string `json:"target_type"` // item, asset, test_case
	TargetID   int    `json:"target_id"`
}

// CreateAssetLink creates a link from an asset to another entity
func (h *AssetHandler) CreateAssetLink(w http.ResponseWriter, r *http.Request) {
	currentUser, assetID, ok := h.requireAssetEditAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[CreateAssetLinkRequest](w, r)
	if !ok {
		return
	}

	// Validate target type
	validTargetTypes := map[string]bool{"item": true, "asset": true, "test_case": true}
	if !validTargetTypes[req.TargetType] {
		respondValidationError(w, r, "Invalid target_type. Must be 'item', 'asset', or 'test_case'")
		return
	}

	// Prevent self-links
	if req.TargetType == "asset" && req.TargetID == assetID {
		respondValidationError(w, r, "Cannot create link to self")
		return
	}

	// Verify link type exists and is active
	var linkTypeActive bool
	err := h.db.QueryRow("SELECT active FROM link_types WHERE id = ?", req.LinkTypeID).Scan(&linkTypeActive)
	if errors.Is(err, sql.ErrNoRows) {
		respondValidationError(w, r, "Link type not found")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !linkTypeActive {
		respondValidationError(w, r, "Link type is not active")
		return
	}

	// Check uniqueness before insert
	var linkExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_links WHERE link_type_id = ? AND source_type = 'asset' AND source_id = ? AND target_type = ? AND target_id = ?)",
		req.LinkTypeID, assetID, req.TargetType, req.TargetID).Scan(&linkExists)
	if linkExists {
		respondConflict(w, r, "Link already exists")
		return
	}

	now := time.Now()

	var linkID int64
	err = h.db.QueryRow(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by, created_at)
		VALUES (?, 'asset', ?, ?, ?, ?, ?) RETURNING id
	`, req.LinkTypeID, assetID, req.TargetType, req.TargetID, currentUser.ID, now).Scan(&linkID)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Link already exists")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	response := map[string]interface{}{
		"id":           linkID,
		"link_type_id": req.LinkTypeID,
		"source_type":  "asset",
		"source_id":    assetID,
		"target_type":  req.TargetType,
		"target_id":    req.TargetID,
		"created_at":   now,
	}

	respondJSONCreated(w, response)
}

// GetAssetRelationshipGraph returns a graph of relationships for an asset via BFS up to 2 hops.
func (h *AssetHandler) GetAssetRelationshipGraph(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	assetID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Verify asset exists and user can view it
	var originSetID int
	var originTitle string
	err := h.db.QueryRow("SELECT set_id, title FROM assets WHERE id = ?", assetID).Scan(&originSetID, &originTitle)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "asset")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	canView, err := h.canViewSet(currentUser.ID, originSetID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondNotFound(w, r, "asset")
		return
	}

	// Build accessible workspace IDs for item permission filtering
	accessibleWS, err := GetAccessibleWorkspaceIDs(&models.User{ID: currentUser.ID}, h.db, h.permissionService)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	wsSet := make(map[int]bool, len(accessibleWS))
	for _, id := range accessibleWS {
		wsSet[id] = true
	}

	// Cache for asset set view permissions
	setViewCache := map[int]bool{originSetID: true}
	canViewCached := func(setID int) bool {
		if v, ok := setViewCache[setID]; ok {
			return v
		}
		ok, err := h.canViewSet(currentUser.ID, setID)
		if err != nil {
			ok = false
		}
		setViewCache[setID] = ok
		return ok
	}

	const maxNodes = 100
	const maxHops = 2

	type nodeKey = string // "{type}-{id}"
	makeKey := func(entityType string, entityID int) string {
		return fmt.Sprintf("%s-%d", entityType, entityID)
	}

	visited := map[nodeKey]bool{}
	nodeMap := map[nodeKey]*models.RelationshipGraphNode{}
	var edges []models.RelationshipGraphEdge
	edgeIDCounter := 0
	truncated := false

	// Edge deduplication to prevent duplicates from circular custom field references
	edgeSeen := map[string]bool{}
	addEdge := func(source, target, label, edgeType, color string) {
		ek := source + ":" + target + ":" + label + ":" + edgeType
		if edgeSeen[ek] {
			return
		}
		edgeSeen[ek] = true
		edgeIDCounter++
		edges = append(edges, models.RelationshipGraphEdge{
			ID:       fmt.Sprintf("e%d", edgeIDCounter),
			Source:   source,
			Target:   target,
			Label:    label,
			Color:    color,
			EdgeType: edgeType,
		})
	}

	// BFS queue entry
	type bfsEntry struct {
		key        nodeKey
		entityType string
		entityID   int
		hop        int
	}

	queue := []bfsEntry{{
		key:        makeKey("asset", assetID),
		entityType: "asset",
		entityID:   assetID,
		hop:        0,
	}}
	visited[queue[0].key] = true
	nodeMap[queue[0].key] = &models.RelationshipGraphNode{
		ID:       queue[0].key,
		EntityID: assetID,
		Type:     "asset",
		Title:    originTitle,
		IsOrigin: true,
		Hop:      0,
	}

	// tryVisitNode adds a newly discovered node to the graph if it hasn't been visited yet.
	// Returns true if the node was added (or already existed), false if the graph is at capacity.
	tryVisitNode := func(nKey, entityType string, entityID int, title string, hop int) {
		if visited[nKey] {
			return
		}
		if len(nodeMap) >= maxNodes {
			truncated = true
			return
		}
		visited[nKey] = true
		nodeMap[nKey] = &models.RelationshipGraphNode{
			ID:       nKey,
			EntityID: entityID,
			Type:     entityType,
			Title:    title,
			Hop:      hop,
		}
		queue = append(queue, bfsEntry{
			key:        nKey,
			entityType: entityType,
			entityID:   entityID,
			hop:        hop,
		})
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current.hop >= maxHops {
			continue
		}

		// Find neighbors via item_links (outgoing). Item titles are hydrated
		// via ItemRepository.GetTitles below.
		outRows, err := h.db.Query(`
			SELECT il.id, il.target_type, il.target_id,
			       lt.forward_label, lt.color,
			       CASE
			           WHEN il.target_type = 'asset' THEN (SELECT title FROM assets WHERE id = il.target_id)
			           WHEN il.target_type = 'test_case' THEN (SELECT title FROM test_cases WHERE id = il.target_id)
			           ELSE ''
			       END as target_title
			FROM item_links il
			JOIN link_types lt ON il.link_type_id = lt.id
			WHERE il.source_type = ? AND il.source_id = ?
		`, current.entityType, current.entityID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		type linkNeighbor struct {
			linkID     int
			entityType string
			entityID   int
			label      string
			color      string
			title      string
		}
		var neighbors []linkNeighbor
		var pendingItemIdx []int

		for outRows.Next() {
			var n linkNeighbor
			if err := outRows.Scan(&n.linkID, &n.entityType, &n.entityID, &n.label, &n.color, &n.title); err != nil {
				_ = outRows.Close()
				respondInternalError(w, r, err)
				return
			}
			if n.entityType == "item" {
				pendingItemIdx = append(pendingItemIdx, len(neighbors))
			}
			neighbors = append(neighbors, n)
		}
		if err := outRows.Err(); err != nil {
			_ = outRows.Close()
			respondInternalError(w, r, fmt.Errorf("error iterating outgoing link rows in BFS: %w", err))
			return
		}
		_ = outRows.Close()

		// Find neighbors via item_links (incoming). Item titles are hydrated
		// via ItemRepository.GetTitles below.
		inRows, err := h.db.Query(`
			SELECT il.id, il.source_type, il.source_id,
			       lt.reverse_label, lt.color,
			       CASE
			           WHEN il.source_type = 'asset' THEN (SELECT title FROM assets WHERE id = il.source_id)
			           WHEN il.source_type = 'test_case' THEN (SELECT title FROM test_cases WHERE id = il.source_id)
			           ELSE ''
			       END as source_title
			FROM item_links il
			JOIN link_types lt ON il.link_type_id = lt.id
			WHERE il.target_type = ? AND il.target_id = ?
		`, current.entityType, current.entityID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		for inRows.Next() {
			var n linkNeighbor
			if err := inRows.Scan(&n.linkID, &n.entityType, &n.entityID, &n.label, &n.color, &n.title); err != nil {
				_ = inRows.Close()
				respondInternalError(w, r, err)
				return
			}
			if n.entityType == "item" {
				pendingItemIdx = append(pendingItemIdx, len(neighbors))
			}
			neighbors = append(neighbors, n)
		}
		if err := inRows.Err(); err != nil {
			_ = inRows.Close()
			respondInternalError(w, r, fmt.Errorf("error iterating incoming link rows in BFS: %w", err))
			return
		}
		_ = inRows.Close()

		if len(pendingItemIdx) > 0 {
			itemIDs := make([]int, len(pendingItemIdx))
			for i, idx := range pendingItemIdx {
				itemIDs[i] = neighbors[idx].entityID
			}
			titles, err := repository.NewItemRepository(h.db).GetTitles(itemIDs)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			for _, idx := range pendingItemIdx {
				neighbors[idx].title = titles[neighbors[idx].entityID]
			}
		}

		// Process link neighbors
		for _, n := range neighbors {
			nKey := makeKey(n.entityType, n.entityID)

			// Permission check
			if !h.canAccessEntity(n.entityType, n.entityID, wsSet, canViewCached) {
				continue
			}

			tryVisitNode(nKey, n.entityType, n.entityID, n.title, current.hop+1)
			addEdge(current.key, nKey, n.label, "link", n.color)
		}

		// Find custom field references (items/assets with field_type="asset" pointing to current entity)
		// Only relevant when current entity is an asset
		if current.entityType == "asset" {
			fieldRefNeighbors := h.findCustomFieldReferences(current.entityID, wsSet, canViewCached)
			for _, fr := range fieldRefNeighbors {
				nKey := makeKey(fr.entityType, fr.entityID)
				tryVisitNode(nKey, fr.entityType, fr.entityID, fr.title, current.hop+1)
				addEdge(nKey, current.key, "Field: "+fr.fieldName, "field_reference", "")
			}

			// Outgoing custom field references (this asset's own fields pointing to other assets)
			outgoingRefs := h.findOutgoingCustomFieldReferences(current.entityID, canViewCached)
			for _, fr := range outgoingRefs {
				nKey := makeKey(fr.entityType, fr.entityID)
				tryVisitNode(nKey, fr.entityType, fr.entityID, fr.title, current.hop+1)
				addEdge(current.key, nKey, "Field: "+fr.fieldName, "field_reference", "")
			}
		}
	}

	// Collect nodes
	nodes := make([]models.RelationshipGraphNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		// Enrich metadata
		n.Metadata = h.getEntityMetadata(n.Type, n.EntityID)
		nodes = append(nodes, *n)
	}

	respondJSONOK(w, models.RelationshipGraphResponse{
		Nodes:      nodes,
		Edges:      edges,
		Truncated:  truncated,
		TotalCount: len(nodes),
	})
}

// canAccessEntity checks if the user can view an entity based on its type.
func (h *AssetHandler) canAccessEntity(entityType string, entityID int, wsSet map[int]bool, canViewSet func(int) bool) bool {
	switch entityType {
	case "item":
		wsID, err := repository.NewItemRepository(h.db).GetWorkspaceID(entityID)
		if err != nil {
			return false
		}
		return wsSet[wsID]
	case "asset":
		var setID int
		err := h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", entityID).Scan(&setID)
		if err != nil {
			return false
		}
		return canViewSet(setID)
	case "test_case":
		// Test cases use workspace permissions
		var wsID int
		err := h.db.QueryRow("SELECT workspace_id FROM test_cases WHERE id = ?", entityID).Scan(&wsID)
		if err != nil {
			return false
		}
		return wsSet[wsID]
	}
	return false
}

type fieldRefResult struct {
	entityType string
	entityID   int
	title      string
	fieldName  string
}

// findCustomFieldReferences finds items and assets whose custom_field_values reference the given asset ID
// via custom fields with field_type='asset'.
func (h *AssetHandler) findCustomFieldReferences(assetID int, wsSet map[int]bool, canViewSet func(int) bool) []fieldRefResult {
	var results []fieldRefResult

	// Get all asset-type custom fields
	fieldRows, err := h.db.Query(`
		SELECT id, name FROM custom_field_definitions WHERE field_type = 'asset'
	`)
	if err != nil {
		return results
	}
	defer func() { _ = fieldRows.Close() }()

	type fieldInfo struct {
		id   int
		name string
	}
	var fields []fieldInfo
	for fieldRows.Next() {
		var f fieldInfo
		if err := fieldRows.Scan(&f.id, &f.name); err != nil {
			slog.Warn("failed to scan custom field definition", slog.String("component", "asset_links"), slog.Any("error", err))
			continue
		}
		fields = append(fields, f)
	}
	if err := fieldRows.Err(); err != nil {
		slog.Warn("error iterating custom field definitions", slog.String("component", "asset_links"), slog.Any("error", err))
		return results
	}

	assetIDStr := strconv.Itoa(assetID)

	for _, f := range fields {
		fieldKey := strconv.Itoa(f.id)
		// Check items: value could be plain int or {"id": N}
		itemRefs, err := repository.NewItemRepository(h.db).ListItemsReferencingAssetInCustomField(fieldKey, assetIDStr)
		if err != nil {
			slog.Warn("error loading item refs for custom field", slog.String("component", "asset_links"), slog.Any("error", err))
		}
		for _, ref := range itemRefs {
			if wsSet[ref.WorkspaceID] {
				results = append(results, fieldRefResult{
					entityType: "item",
					entityID:   ref.ID,
					title:      ref.Title,
					fieldName:  f.name,
				})
			}
		}

		// Check assets: same pattern
		var directExpr, nestedExpr string
		if h.db.GetDriverName() == "postgres" {
			directExpr = fmt.Sprintf("a.custom_field_values->>'%s'", fieldKey)
			nestedExpr = fmt.Sprintf("a.custom_field_values->'%s'->>'id'", fieldKey)
		} else {
			directExpr = fmt.Sprintf(`NULLIF(a.custom_field_values,'') ->> '$."%s"'`, fieldKey)    //nolint:gocritic // SQL JSON path, not Go quoting
			nestedExpr = fmt.Sprintf(`NULLIF(a.custom_field_values,'') ->> '$."%s".id'`, fieldKey) //nolint:gocritic // SQL JSON path, not Go quoting
		}
		assetRows, err := h.db.Query(fmt.Sprintf(`
			SELECT a.id, a.title, a.set_id
			FROM assets a
			WHERE (%s = ? OR %s = ?)
		`, directExpr, nestedExpr), assetIDStr, assetIDStr)
		if err != nil {
			continue
		}
		for assetRows.Next() {
			var id int
			var title string
			var setID int
			if err := assetRows.Scan(&id, &title, &setID); err != nil {
				slog.Warn("failed to scan asset row for custom field reference", slog.String("component", "asset_links"), slog.Any("error", err))
				continue
			}
			if id == assetID {
				continue // Skip self
			}
			if canViewSet(setID) {
				results = append(results, fieldRefResult{
					entityType: "asset",
					entityID:   id,
					title:      title,
					fieldName:  f.name,
				})
			}
		}
		if err := assetRows.Err(); err != nil {
			slog.Warn("error iterating asset rows for custom field references", slog.String("component", "asset_links"), slog.Any("error", err))
		}
		_ = assetRows.Close()
	}

	return results
}

// findOutgoingCustomFieldReferences finds assets referenced by the given asset's own custom_field_values
// via custom fields with field_type='asset'.
func (h *AssetHandler) findOutgoingCustomFieldReferences(assetID int, canViewSet func(int) bool) []fieldRefResult {
	var results []fieldRefResult

	// Get the asset's custom_field_values JSON
	var cfvRaw sql.NullString
	err := h.db.QueryRow("SELECT custom_field_values FROM assets WHERE id = ?", assetID).Scan(&cfvRaw)
	if err != nil || !cfvRaw.Valid || cfvRaw.String == "" {
		return results
	}

	var cfv map[string]json.RawMessage
	if err := json.Unmarshal([]byte(cfvRaw.String), &cfv); err != nil {
		return results
	}

	// Get all asset-type custom field definitions
	fieldRows, err := h.db.Query("SELECT id, name FROM custom_field_definitions WHERE field_type = 'asset'")
	if err != nil {
		return results
	}
	defer func() { _ = fieldRows.Close() }()

	type fieldInfo struct {
		id   int
		name string
	}
	var fields []fieldInfo
	for fieldRows.Next() {
		var f fieldInfo
		if err := fieldRows.Scan(&f.id, &f.name); err != nil {
			continue
		}
		fields = append(fields, f)
	}
	if err := fieldRows.Err(); err != nil {
		slog.Warn("error iterating custom field definitions", slog.String("component", "asset_links"), slog.Any("error", err))
		return results
	}

	for _, f := range fields {
		fieldKey := strconv.Itoa(f.id)
		raw, ok := cfv[fieldKey]
		if !ok {
			continue
		}

		// Try plain int first
		var refID int
		if err := json.Unmarshal(raw, &refID); err != nil {
			// Try {"id": N} object format
			var obj struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(raw, &obj); err != nil || obj.ID == 0 {
				continue
			}
			refID = obj.ID
		}

		if refID == 0 || refID == assetID {
			continue
		}

		// Look up the target asset
		var title string
		var setID int
		err := h.db.QueryRow("SELECT title, set_id FROM assets WHERE id = ?", refID).Scan(&title, &setID)
		if err != nil {
			continue
		}
		if !canViewSet(setID) {
			continue
		}

		results = append(results, fieldRefResult{
			entityType: "asset",
			entityID:   refID,
			title:      title,
			fieldName:  f.name,
		})
	}

	return results
}

// getEntityMetadata returns metadata for a graph node based on its entity type.
func (h *AssetHandler) getEntityMetadata(entityType string, entityID int) map[string]interface{} {
	meta := map[string]interface{}{}
	switch entityType {
	case "item":
		if m, err := repository.NewItemRepository(h.db).GetItemGraphMetadata(entityID); err == nil {
			meta["display_key"] = fmt.Sprintf("%s-%d", m.WorkspaceKey, m.WorkspaceItemNumber)
			meta["workspace_id"] = m.WorkspaceID
			if m.StatusName != "" {
				meta["status"] = m.StatusName
			}
		}
	case "asset":
		var statusName, typeName string
		var setID int
		err := h.db.QueryRow(`
			SELECT a.set_id, COALESCE(s.name, ''), COALESCE(at.name, '')
			FROM assets a
			LEFT JOIN asset_statuses s ON a.status_id = s.id
			LEFT JOIN asset_types at ON a.asset_type_id = at.id
			WHERE a.id = ?
		`, entityID).Scan(&setID, &statusName, &typeName)
		if err == nil {
			meta["set_id"] = setID
			if statusName != "" {
				meta["status"] = statusName
			}
			if typeName != "" {
				meta["asset_type"] = typeName
			}
		}
	case "test_case":
		var wsKey string
		var wsID int
		err := h.db.QueryRow(`
			SELECT tc.workspace_id, w.key FROM test_cases tc
			JOIN workspaces w ON tc.workspace_id = w.id
			WHERE tc.id = ?
		`, entityID).Scan(&wsID, &wsKey)
		if err == nil {
			meta["workspace_id"] = wsID
			meta["workspace_key"] = wsKey
		}
	}
	return meta
}
