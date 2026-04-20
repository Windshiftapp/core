package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// assetPermissionChecker is a minimal interface satisfied by *AssetHandler,
// declared here to avoid an AssetHandler import cycle. When nil (e.g. because
// wiring is missing in server setup), asset-endpoint permission checks fail
// closed — they return false / 404, never true.
type assetPermissionChecker interface {
	HasAssetSetPermission(userID, setID int, permissionKey string) (bool, error)
}

type ItemLinkHandler struct {
	db                  database.Database
	permissionService   *services.PermissionService
	assetPerm           assetPermissionChecker
	notificationService interface {
		EmitEvent(event *services.NotificationEvent)
	} // Notification service for async notification processing (optional, can be nil)
	actionService interface {
		EmitActionEvent(event *models.ActionEvent)
	} // Action service for automation workflows (optional, can be nil)
}

func NewItemLinkHandler(db database.Database, notificationService interface {
	EmitEvent(event *services.NotificationEvent)
}, permissionService *services.PermissionService) *ItemLinkHandler {
	return &ItemLinkHandler{
		db:                  db,
		notificationService: notificationService,
		permissionService:   permissionService,
	}
}

// SetAssetPermissionChecker wires in a checker (typically *AssetHandler) so
// that asset-endpoint permission checks resolve. Called after AssetHandler is
// constructed in server setup; without it, every asset endpoint fails closed.
func (h *ItemLinkHandler) SetAssetPermissionChecker(p assetPermissionChecker) {
	h.assetPerm = p
}

// SetActionService sets the action service for automation workflows
func (h *ItemLinkHandler) SetActionService(actionService interface {
	EmitActionEvent(event *models.ActionEvent)
}) {
	h.actionService = actionService
}

// resolveEntityScope looks up the scoping identifier for a link endpoint:
// items and test_cases → workspace_id; assets → set_id. found=false on
// sql.ErrNoRows so callers can 404 cleanly without leaking which kind of
// lookup failed.
func (h *ItemLinkHandler) resolveEntityScope(entityType string, entityID int) (wsID, setID int, found bool, err error) {
	switch entityType {
	case "item":
		wsID, err := repository.NewItemRepository(h.db).GetWorkspaceID(entityID)
		if errors.Is(err, repository.ErrNotFound) {
			return 0, 0, false, nil
		}
		if err != nil {
			return 0, 0, false, err
		}
		return wsID, 0, true, nil
	case "test_case":
		var scopeID int
		err = h.db.QueryRow("SELECT workspace_id FROM test_cases WHERE id = ?", entityID).Scan(&scopeID)
		if err == sql.ErrNoRows {
			return 0, 0, false, nil
		}
		if err != nil {
			return 0, 0, false, err
		}
		return scopeID, 0, true, nil
	case "asset":
		var scopeID int
		err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", entityID).Scan(&scopeID)
		if err == sql.ErrNoRows {
			return 0, 0, false, nil
		}
		if err != nil {
			return 0, 0, false, err
		}
		return 0, scopeID, true, nil
	default:
		return 0, 0, false, fmt.Errorf("unsupported entity type %q", entityType)
	}
}

// checkEntityPermission verifies the current user has the given permission on
// a link endpoint, regardless of entity type. It writes 404 (per project
// policy) for every denial path — missing user, missing entity, nil asset
// checker, permission denied — so existence is never leaked. Returns true
// only when access is permitted.
func (h *ItemLinkHandler) checkEntityPermission(w http.ResponseWriter, r *http.Request, entityType string, entityID int, workspacePerm, assetPermKey string) bool {
	if entityType == "item" {
		// Existing helper already returns 404 on all failure paths.
		return CheckItemPermission(w, r, h.db, h.permissionService, entityID, workspacePerm)
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return false
	}

	wsID, setID, found, err := h.resolveEntityScope(entityType, entityID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !found {
		respondNotFound(w, r, entityType)
		return false
	}

	switch entityType {
	case "test_case":
		hasPerm, err := h.permissionService.HasWorkspacePermission(user.ID, wsID, workspacePerm)
		if err != nil || !hasPerm {
			respondNotFound(w, r, entityType)
			return false
		}
		return true
	case "asset":
		if h.assetPerm == nil {
			respondNotFound(w, r, entityType)
			return false
		}
		hasPerm, err := h.assetPerm.HasAssetSetPermission(user.ID, setID, assetPermKey)
		if err != nil || !hasPerm {
			respondNotFound(w, r, entityType)
			return false
		}
		return true
	}
	respondValidationError(w, r, "unsupported entity type")
	return false
}

// canUserViewEntity is the silent counterpart to checkEntityPermission, used
// for filtering result sets without writing to the response. Pre-built
// allow-lists keep it cheap when called once per row. Exercised by unit
// tests; production callers use endpointVisible / filterLinksByAccess.
//
//nolint:unused // covered by item_links_test.go (tests excluded from lint)
func (h *ItemLinkHandler) canUserViewEntity(_ int, entityType string, entityID int, accessibleWs, accessibleSets map[int]bool) bool {
	wsID, setID, found, err := h.resolveEntityScope(entityType, entityID)
	if err != nil || !found {
		return false
	}
	switch entityType {
	case "item", "test_case":
		return accessibleWs[wsID]
	case "asset":
		return accessibleSets[setID]
	}
	return false
}

// GetLinksForItem returns all links for a specific item (work item or test case)
func (h *ItemLinkHandler) GetLinksForItem(w http.ResponseWriter, r *http.Request) {
	itemType := r.PathValue("type") // "items" or "test-cases"
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Convert URL path to internal type
	internalType := "item"
	if itemType == "test-cases" {
		internalType = "test_case"
	}

	// Unified view-permission check across items and test_cases. Writes 404
	// on missing entity or denial — no silent-empty branch for test_cases.
	if !h.checkEntityPermission(w, r, internalType, id, models.PermissionItemView, AssetPermissionKeyView) {
		return
	}

	// Get outgoing links (where this item is the source), excluding field-managed links
	outgoingLinks, err := h.getLinksWhere("source_type = ? AND source_id = ? AND il.custom_field_id IS NULL", internalType, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Get incoming links (where this item is the target)
	// Include field-managed links in incoming so they appear annotated with field name
	incomingLinks, err := h.getLinksWhere("target_type = ? AND target_id = ?", internalType, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Filter linked entities by what the user can actually see: workspace
	// access covers items and test_cases, asset set access covers assets.
	accessibleKeys, _ := GetAccessibleWorkspaceKeys(user, h.db, h.permissionService)
	accessibleWsIDs := h.accessibleWorkspaceIDSet(user)
	accessibleSetIDs := h.accessibleAssetSetIDSet(user)
	outgoingLinks = h.filterLinksByAccess(outgoingLinks, accessibleKeys, accessibleWsIDs, accessibleSetIDs)
	incomingLinks = h.filterLinksByAccess(incomingLinks, accessibleKeys, accessibleWsIDs, accessibleSetIDs)

	response := map[string]interface{}{
		"outgoing": outgoingLinks,
		"incoming": incomingLinks,
	}

	respondJSONOK(w, response)
}

// filterLinksByAccess drops links whose endpoint is in an inaccessible
// workspace (items, test_cases) or inaccessible asset set (assets). The bare
// workspace-key check only covered item endpoints — callers that return
// links touching test_cases and assets would otherwise leak titles.
func (h *ItemLinkHandler) filterLinksByAccess(links []models.ItemLink, accessibleKeys map[string]bool, accessibleWsIDs, accessibleSetIDs map[int]bool) []models.ItemLink {
	filtered := make([]models.ItemLink, 0, len(links))
	for _, link := range links {
		if !h.endpointVisible(link.SourceType, link.SourceID, link.SourceWorkspaceKey, accessibleKeys, accessibleWsIDs, accessibleSetIDs) {
			continue
		}
		if !h.endpointVisible(link.TargetType, link.TargetID, link.TargetWorkspaceKey, accessibleKeys, accessibleWsIDs, accessibleSetIDs) {
			continue
		}
		filtered = append(filtered, link)
	}
	return filtered
}

// endpointVisible returns true when the given endpoint of a link is
// accessible to the current user. Items use the workspace-key cache already
// joined into ItemLink; test_cases and assets are resolved via a small
// lookup (at most one extra query per link per endpoint).
func (h *ItemLinkHandler) endpointVisible(entityType string, entityID int, workspaceKey string, accessibleKeys map[string]bool, accessibleWsIDs, accessibleSetIDs map[int]bool) bool {
	switch entityType {
	case "item":
		// Keep existing fast path: trust the pre-joined workspace key.
		return workspaceKey == "" || accessibleKeys[workspaceKey]
	case "test_case":
		wsID, _, found, err := h.resolveEntityScope(entityType, entityID)
		if err != nil || !found {
			return false
		}
		return accessibleWsIDs[wsID]
	case "asset":
		_, setID, found, err := h.resolveEntityScope(entityType, entityID)
		if err != nil || !found {
			return false
		}
		return accessibleSetIDs[setID]
	}
	return false
}

// accessibleWorkspaceIDSet returns a set of workspace IDs the user can view,
// used as a fast membership check while filtering result rows.
func (h *ItemLinkHandler) accessibleWorkspaceIDSet(user *models.User) map[int]bool {
	set := make(map[int]bool)
	ids, err := GetAccessibleWorkspaceIDs(user, h.db, h.permissionService)
	if err != nil {
		return set
	}
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// accessibleAssetSetIDSet returns a set of asset_management_sets IDs the
// user can view. Iterates over every set and invokes the injected asset
// permission checker; tolerable for correctness first-pass and matches the
// pattern used by AssetHandler.canAccessEntity. If the checker is nil the
// set is empty (fail-closed).
func (h *ItemLinkHandler) accessibleAssetSetIDSet(user *models.User) map[int]bool {
	set := make(map[int]bool)
	if user == nil || h.assetPerm == nil {
		return set
	}
	rows, err := h.db.Query("SELECT id FROM asset_management_sets")
	if err != nil {
		return set
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		hasView, err := h.assetPerm.HasAssetSetPermission(user.ID, id, AssetPermissionKeyView)
		if err == nil && hasView {
			set[id] = true
		}
	}
	return set
}

// CreateLink creates a new link between items
func (h *ItemLinkHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
	link, ok := decodeJSON[models.ItemLink](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if link.LinkTypeID == 0 || link.SourceType == "" || link.SourceID == 0 ||
		link.TargetType == "" || link.TargetID == 0 {
		respondValidationError(w, r, "link_type_id, source_type, source_id, target_type, and target_id are required")
		return
	}

	// Validate source and target types
	if !isValidLinkType(link.SourceType) || !isValidLinkType(link.TargetType) {
		respondValidationError(w, r, "Invalid source_type or target_type. Must be 'item', 'test_case', or 'asset'")
		return
	}

	// Prevent self-links
	if link.SourceType == link.TargetType && link.SourceID == link.TargetID {
		respondValidationError(w, r, "Cannot create link to self")
		return
	}

	// Special validation for "Tests" link type (ID = 1)
	// This link type can only link between items and test cases, not between same entity types
	if link.LinkTypeID == 1 {
		if link.SourceType == link.TargetType {
			respondValidationError(w, r, "The 'Tests' link type can only link between items and test cases, not between the same entity types")
			return
		}
		// Ensure one is test_case and other is item
		if (link.SourceType != "test_case" || link.TargetType != "item") &&
			(link.SourceType != "item" || link.TargetType != "test_case") {
			respondValidationError(w, r, "The 'Tests' link type requires one entity to be a test case and the other to be an item")
			return
		}
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	createdBy := currentUser.ID

	// For custom-field-managed links, validate field config and apply
	// mirror-field source/target swap FIRST. Permission checks below then
	// run against the final entities that will actually own the link —
	// otherwise a mirror-field request could be authorized against its
	// pre-swap source and sneak a write to an entity it can't edit.
	var fieldPlan *fieldLinkPlan
	if link.CustomFieldID != nil {
		var fieldErr error
		fieldPlan, fieldErr = h.validateAndPrepareFieldLink(&link)
		if fieldErr != nil {
			respondValidationError(w, r, fieldErr.Error())
			return
		}
	}

	// Permission checks on the final source + target, regardless of entity
	// type. 404 on any failure — never 403 — to avoid leaking existence.
	// Must run before the duplicate probe below, otherwise a 409 vs 404
	// oracle leaks whether a given (source, target) link already exists.
	if !h.checkEntityPermission(w, r, link.SourceType, link.SourceID, models.PermissionItemEdit, AssetPermissionKeyEdit) {
		return
	}
	if !h.checkEntityPermission(w, r, link.TargetType, link.TargetID, models.PermissionItemView, AssetPermissionKeyView) {
		return
	}

	// Check if link already exists (in either direction). Runs after the
	// permission checks so unauthorized callers never get a 409 leak.
	var existingID int
	err := h.db.QueryRow(`
		SELECT id FROM item_links
		WHERE (source_type = ? AND source_id = ? AND target_type = ? AND target_id = ?)
		   OR (source_type = ? AND source_id = ? AND target_type = ? AND target_id = ?)
	`, link.SourceType, link.SourceID, link.TargetType, link.TargetID,
		link.TargetType, link.TargetID, link.SourceType, link.SourceID).Scan(&existingID)

	if err == nil {
		respondConflict(w, r, "A link between these items already exists")
		return
	}
	if err != sql.ErrNoRows {
		respondInternalError(w, r, err)
		return
	}

	// Single-value field DELETE runs only after permission checks pass,
	// so an unauthorized caller can't wipe the field on a rejected request.
	h.enforceSingleValueFieldLink(&link, fieldPlan)

	// Create link via service (handles link type validation + insert)
	linkSvc := services.NewItemLinkService(h.db)
	id, err := linkSvc.CreateLink(services.CreateItemLinkParams{
		LinkTypeID:    link.LinkTypeID,
		SourceType:    link.SourceType,
		SourceID:      link.SourceID,
		TargetType:    link.TargetType,
		TargetID:      link.TargetID,
		CreatedBy:     &createdBy,
		CustomFieldID: link.CustomFieldID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if id == 0 {
		respondConflict(w, r, "Link already exists")
		return
	}

	link.ID = int(id)
	link.CreatedAt = time.Now()

	// Get the created link with full details
	createdLink, err := h.getLinkByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Emit notification event (only for work item links)
	itemRepo := repository.NewItemRepository(h.db)

	if h.notificationService != nil && link.SourceType == "item" {
		user := utils.GetCurrentUser(r)
		if user != nil {
			// Fetch source item details for notification
			if sourceItem, err := itemRepo.FindByID(link.SourceID); err == nil {
				h.notificationService.EmitEvent(&services.NotificationEvent{
					EventType:   models.EventItemLinked,
					WorkspaceID: sourceItem.WorkspaceID,
					ActorUserID: user.ID,
					ItemID:      link.SourceID,
					AssigneeID:  sourceItem.AssigneeID,
					CreatorID:   sourceItem.CreatorID,
					Title:       "Item Linked",
					TemplateData: map[string]interface{}{
						"item.title":   sourceItem.Title,
						"item.id":      link.SourceID,
						"target.title": createdLink.TargetTitle,
						"target.id":    link.TargetID,
						"user.name":    user.Username,
					},
				})
			}
		}
	}

	// Emit action event for item linked
	if h.actionService != nil && link.SourceType == "item" {
		// Get workspace ID for the source item (ignore errors — we've already
		// dispatched the notification above and don't want to fail the request).
		workspaceID, _ := itemRepo.GetWorkspaceID(link.SourceID)

		h.actionService.EmitActionEvent(&models.ActionEvent{
			EventType:   models.ActionTriggerItemLinked,
			WorkspaceID: workspaceID,
			ItemID:      link.SourceID,
			ActorUserID: currentUser.ID,
			NewValues: map[string]interface{}{
				"link_type_id": link.LinkTypeID,
				"target_type":  link.TargetType,
				"target_id":    link.TargetID,
			},
		})
	}

	respondJSONCreated(w, createdLink)
}

// DeleteLink removes a link
func (h *ItemLinkHandler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Get link details before deletion for notification
	var sourceType string
	var sourceID, targetID int
	var targetTitle string
	err = h.db.QueryRow(`
		SELECT il.source_type, il.source_id, il.target_id,
		       COALESCE(i.title, tc.name, a.title, '') as target_title
		FROM item_links il
		LEFT JOIN items i ON il.target_type = 'item' AND il.target_id = i.id
		LEFT JOIN test_cases tc ON il.target_type = 'test_case' AND il.target_id = tc.id
		LEFT JOIN assets a ON il.target_type = 'asset' AND il.target_id = a.id
		WHERE il.id = ?
	`, id).Scan(&sourceType, &sourceID, &targetID, &targetTitle)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "link")
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to fetch link details: %w", err))
		return
	}

	// Edit permission on source (covers item, test_case, and asset endpoints).
	// 404 on any denial per CLAUDE.md policy.
	if !h.checkEntityPermission(w, r, sourceType, sourceID, models.PermissionItemEdit, AssetPermissionKeyEdit) {
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM item_links WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "link")
		return
	}

	// Emit notification event (only for work item links)
	if h.notificationService != nil && sourceType == "item" {
		user := utils.GetCurrentUser(r)
		if user != nil {
			// Fetch source item details for notification
			if sourceItem, err := repository.NewItemRepository(h.db).FindByID(sourceID); err == nil {
				h.notificationService.EmitEvent(&services.NotificationEvent{
					EventType:   models.EventItemUnlinked,
					WorkspaceID: sourceItem.WorkspaceID,
					ActorUserID: user.ID,
					ItemID:      sourceID,
					AssigneeID:  sourceItem.AssigneeID,
					CreatorID:   sourceItem.CreatorID,
					Title:       "Item Unlinked",
					TemplateData: map[string]interface{}{
						"item.title":   sourceItem.Title,
						"item.id":      sourceID,
						"target.title": targetTitle,
						"target.id":    targetID,
						"user.name":    user.Username,
					},
				})
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetLinkedAssets returns all assets linked to a specific item
func (h *ItemLinkHandler) GetLinkedAssets(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, id, models.PermissionItemView) {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	accessibleSets := h.accessibleAssetSetIDSet(user)

	// Get assets where item is the source
	outgoingQuery := `
		SELECT a.id, a.title, COALESCE(a.description, '') AS description,
		       a.set_id, ams.name AS set_name,
		       COALESCE(at.name, '') AS type_name,
		       COALESCE(ac.name, '') AS category_name,
		       il.id AS link_id, lt.name AS link_type_name, lt.forward_label
		FROM item_links il
		JOIN assets a ON il.target_type = 'asset' AND il.target_id = a.id
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		JOIN link_types lt ON il.link_type_id = lt.id
		WHERE il.source_type = 'item' AND il.source_id = ?
		ORDER BY a.title
	`

	// Get assets where item is the target
	incomingQuery := `
		SELECT a.id, a.title, COALESCE(a.description, '') AS description,
		       a.set_id, ams.name AS set_name,
		       COALESCE(at.name, '') AS type_name,
		       COALESCE(ac.name, '') AS category_name,
		       il.id AS link_id, lt.name AS link_type_name, lt.reverse_label
		FROM item_links il
		JOIN assets a ON il.source_type = 'asset' AND il.source_id = a.id
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		JOIN link_types lt ON il.link_type_id = lt.id
		WHERE il.target_type = 'item' AND il.target_id = ?
		ORDER BY a.title
	`

	type LinkedAsset struct {
		ID           int    `json:"id"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		SetID        int    `json:"set_id"`
		SetName      string `json:"set_name"`
		TypeName     string `json:"type_name"`
		CategoryName string `json:"category_name"`
		LinkID       int    `json:"link_id"`
		LinkTypeName string `json:"link_type_name"`
		LinkLabel    string `json:"link_label"`
		Direction    string `json:"direction"` // "outgoing" or "incoming"
	}

	var linkedAssets []LinkedAsset

	// Process outgoing links
	rows, err := h.db.Query(outgoingQuery, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	for rows.Next() {
		var asset LinkedAsset
		var description, setName, typeName, categoryName, linkLabel sql.NullString
		err = rows.Scan(&asset.ID, &asset.Title, &description, &asset.SetID, &setName,
			&typeName, &categoryName, &asset.LinkID, &asset.LinkTypeName, &linkLabel)
		if err != nil {
			_ = rows.Close()
			respondInternalError(w, r, err)
			return
		}
		if !accessibleSets[asset.SetID] {
			continue
		}
		asset.Description = description.String
		asset.SetName = setName.String
		asset.TypeName = typeName.String
		asset.CategoryName = categoryName.String
		asset.LinkLabel = linkLabel.String
		asset.Direction = "outgoing"
		linkedAssets = append(linkedAssets, asset)
	}
	_ = rows.Close()

	// Process incoming links
	rows, err = h.db.Query(incomingQuery, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	for rows.Next() {
		var asset LinkedAsset
		var description, setName, typeName, categoryName, linkLabel sql.NullString
		err := rows.Scan(&asset.ID, &asset.Title, &description, &asset.SetID, &setName,
			&typeName, &categoryName, &asset.LinkID, &asset.LinkTypeName, &linkLabel)
		if err != nil {
			_ = rows.Close()
			respondInternalError(w, r, err)
			return
		}
		if !accessibleSets[asset.SetID] {
			continue
		}
		asset.Description = description.String
		asset.SetName = setName.String
		asset.TypeName = typeName.String
		asset.CategoryName = categoryName.String
		asset.LinkLabel = linkLabel.String
		asset.Direction = "incoming"
		linkedAssets = append(linkedAssets, asset)
	}
	_ = rows.Close()

	respondJSONOK(w, linkedAssets)
}

// SearchLinkableItems searches for items that can be linked
func (h *ItemLinkHandler) SearchLinkableItems(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	accessibleWorkspaceIDs, err := GetAccessibleWorkspaceIDs(user, h.db, h.permissionService)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	query := r.URL.Query().Get("q")
	itemType := r.URL.Query().Get("type") // "item", "test_case", "asset", or empty for all
	limit := 20

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Parse optional item_type_ids filter
	var itemTypeIDFilter []int
	if itemTypeIDsStr := r.URL.Query().Get("item_type_ids"); itemTypeIDsStr != "" {
		for _, idStr := range strings.Split(itemTypeIDsStr, ",") {
			if id, err := strconv.Atoi(strings.TrimSpace(idStr)); err == nil && id > 0 {
				itemTypeIDFilter = append(itemTypeIDFilter, id)
			}
		}
	}

	var items []models.LinkableItem

	// Search work items
	if itemType == "" || itemType == "item" {
		workItems, err := h.searchWorkItems(query, limit, accessibleWorkspaceIDs, itemTypeIDFilter)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		items = append(items, workItems...)
	}

	// Search test cases
	if itemType == "" || itemType == "test_case" {
		testCases, err := h.searchTestCases(query, limit, accessibleWorkspaceIDs)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		items = append(items, testCases...)
	}

	// Search assets
	if itemType == "" || itemType == "asset" {
		assets, err := h.searchAssets(user, query, limit)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		items = append(items, assets...)
	}

	respondJSONOK(w, items)
}

// Helper functions

func (h *ItemLinkHandler) getLinksWhere(whereClause string, args ...interface{}) ([]models.ItemLink, error) {
	query := `
		SELECT il.id, il.link_type_id, il.source_type, il.source_id, il.target_type, il.target_id,
		       il.created_by, il.created_at,
		       lt.name, lt.color, lt.forward_label, lt.reverse_label,
		       COALESCE(si.title, stc.title, sa.title, '') as source_title,
		       COALESCE(ti.title, ttc.title, ta.title, '') as target_title,
		       COALESCE(u.username, '') as created_by_name,
		       si.status_id as source_status_id,
		       COALESCE(ss.name, '') as source_status_name,
		       si.item_type_id as source_item_type_id,
		       COALESCE(sit.name, '') as source_item_type_name,
		       COALESCE(sit.icon, '') as source_item_type_icon,
		       COALESCE(sit.color, '') as source_item_type_color,
		       COALESCE(sw.key, '') as source_workspace_key,
		       si.workspace_id as source_workspace_id,
		       ti.status_id as target_status_id,
		       COALESCE(ts.name, '') as target_status_name,
		       ti.item_type_id as target_item_type_id,
		       COALESCE(tit.name, '') as target_item_type_name,
		       COALESCE(tit.icon, '') as target_item_type_icon,
		       COALESCE(tit.color, '') as target_item_type_color,
		       COALESCE(tw.key, '') as target_workspace_key,
		       ti.workspace_id as target_workspace_id,
		       il.custom_field_id,
		       COALESCE(cfd.name, '') as custom_field_name
		FROM item_links il
		JOIN link_types lt ON il.link_type_id = lt.id
		LEFT JOIN items si ON il.source_type = 'item' AND il.source_id = si.id
		LEFT JOIN test_cases stc ON il.source_type = 'test_case' AND il.source_id = stc.id
		LEFT JOIN assets sa ON il.source_type = 'asset' AND il.source_id = sa.id
		LEFT JOIN items ti ON il.target_type = 'item' AND il.target_id = ti.id
		LEFT JOIN test_cases ttc ON il.target_type = 'test_case' AND il.target_id = ttc.id
		LEFT JOIN assets ta ON il.target_type = 'asset' AND il.target_id = ta.id
		LEFT JOIN users u ON il.created_by = u.id
		LEFT JOIN statuses ss ON si.status_id = ss.id
		LEFT JOIN statuses ts ON ti.status_id = ts.id
		LEFT JOIN item_types sit ON si.item_type_id = sit.id
		LEFT JOIN item_types tit ON ti.item_type_id = tit.id
		LEFT JOIN workspaces sw ON si.workspace_id = sw.id
		LEFT JOIN workspaces tw ON ti.workspace_id = tw.id
		LEFT JOIN custom_field_definitions cfd ON il.custom_field_id = cfd.id
		WHERE ` + whereClause + `
		ORDER BY lt.name, il.created_at DESC
	`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var links []models.ItemLink
	for rows.Next() {
		var link models.ItemLink
		err := rows.Scan(&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.CreatedBy, &link.CreatedAt,
			&link.LinkTypeName, &link.LinkTypeColor, &link.LinkTypeForwardLabel, &link.LinkTypeReverseLabel,
			&link.SourceTitle, &link.TargetTitle, &link.CreatedByName,
			&link.SourceStatusID, &link.SourceStatusName,
			&link.SourceItemTypeID, &link.SourceItemTypeName, &link.SourceItemTypeIcon, &link.SourceItemTypeColor,
			&link.SourceWorkspaceKey, &link.SourceWorkspaceID,
			&link.TargetStatusID, &link.TargetStatusName,
			&link.TargetItemTypeID, &link.TargetItemTypeName, &link.TargetItemTypeIcon, &link.TargetItemTypeColor,
			&link.TargetWorkspaceKey, &link.TargetWorkspaceID,
			&link.CustomFieldID, &link.CustomFieldName)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}

	return links, nil
}

func (h *ItemLinkHandler) getLinkByID(id int) (*models.ItemLink, error) {
	links, err := h.getLinksWhere("il.id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(links) == 0 {
		return nil, sql.ErrNoRows
	}
	return &links[0], nil
}

func (h *ItemLinkHandler) searchWorkItems(query string, limit int, accessibleWorkspaceIDs []int, itemTypeIDs ...[]int) ([]models.LinkableItem, error) {
	var typeIDs []int
	if len(itemTypeIDs) > 0 {
		typeIDs = itemTypeIDs[0]
	}
	return repository.NewItemRepository(h.db).SearchLinkableItems(query, accessibleWorkspaceIDs, typeIDs, limit)
}

func (h *ItemLinkHandler) searchTestCases(query string, limit int, accessibleWorkspaceIDs []int) ([]models.LinkableItem, error) {
	if len(accessibleWorkspaceIDs) == 0 {
		return []models.LinkableItem{}, nil
	}

	placeholders, wsArgs := BuildWorkspaceIDPlaceholders(accessibleWorkspaceIDs)
	sqlQuery := fmt.Sprintf(`
		SELECT id, title, COALESCE(preconditions, '') AS summary
		FROM test_cases
		WHERE (title LIKE ? OR preconditions LIKE ?)
		  AND workspace_id IN (%s)
		ORDER BY title
		LIMIT ?
	`, placeholders)

	searchTerm := "%" + query + "%"
	args := make([]interface{}, 0, 3+len(wsArgs))
	args = append(args, searchTerm, searchTerm)
	args = append(args, wsArgs...)
	args = append(args, limit)
	rows, err := h.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []models.LinkableItem
	for rows.Next() {
		var item models.LinkableItem
		var summary sql.NullString

		err := rows.Scan(&item.ID, &item.Title, &summary)
		if err != nil {
			return nil, err
		}

		item.Description = summary.String
		item.Type = "test_case"
		items = append(items, item)
	}

	return items, nil
}

func (h *ItemLinkHandler) searchAssets(user *models.User, query string, limit int) ([]models.LinkableItem, error) {
	// Fail-closed: unauthenticated request or missing asset checker gets nothing.
	if user == nil || h.assetPerm == nil {
		return []models.LinkableItem{}, nil
	}

	accessibleSets := h.accessibleAssetSetIDSet(user)
	if len(accessibleSets) == 0 {
		return []models.LinkableItem{}, nil
	}
	setIDs := make([]int, 0, len(accessibleSets))
	for id := range accessibleSets {
		setIDs = append(setIDs, id)
	}
	setPlaceholders := make([]string, len(setIDs))
	setArgs := make([]interface{}, len(setIDs))
	for i, id := range setIDs {
		setPlaceholders[i] = "?"
		setArgs[i] = id
	}

	sqlQuery := fmt.Sprintf(`
		SELECT a.id, a.title, COALESCE(a.description, '') AS description,
		       a.set_id, ams.name AS set_name,
		       COALESCE(at.name, '') AS type_name,
		       COALESCE(ac.name, '') AS category_name
		FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		WHERE (a.title LIKE ? OR a.description LIKE ?)
		  AND a.set_id IN (%s)
		ORDER BY a.title
		LIMIT ?
	`, strings.Join(setPlaceholders, ","))

	searchTerm := "%" + query + "%"
	args := make([]interface{}, 0, 3+len(setArgs))
	args = append(args, searchTerm, searchTerm)
	args = append(args, setArgs...)
	args = append(args, limit)
	rows, err := h.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []models.LinkableItem
	for rows.Next() {
		var item models.LinkableItem
		var description, setName, typeName, categoryName sql.NullString
		var setID sql.NullInt64

		err := rows.Scan(&item.ID, &item.Title, &description, &setID, &setName, &typeName, &categoryName)
		if err != nil {
			return nil, err
		}

		item.Description = description.String
		item.AssetSetID = utils.NullInt64ToPtr(setID)
		item.AssetSetName = setName.String
		item.AssetTypeName = typeName.String
		item.AssetCategoryName = categoryName.String

		item.Type = "asset"
		items = append(items, item)
	}

	return items, nil
}

func isValidLinkType(linkType string) bool {
	return linkType == "item" || linkType == "test_case" || linkType == "asset"
}

// GetFieldLinks returns links managed by a specific custom field for a given item
func (h *ItemLinkHandler) GetFieldLinks(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	fieldID, ok := requireIDParam(w, r, "fieldId")
	if !ok {
		return
	}

	var err error

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	// Get field options to determine if this is a primary or mirror field
	var optionsJSON sql.NullString
	var fieldType string
	err = h.db.QueryRow("SELECT field_type, options FROM custom_field_definitions WHERE id = ?", fieldID).Scan(&fieldType, &optionsJSON)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "custom_field")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if fieldType != "linking" {
		respondValidationError(w, r, "Field is not a linking type")
		return
	}

	var opts struct {
		MirrorOfFieldID int `json:"mirror_of_field_id"`
	}
	if optionsJSON.Valid {
		_ = json.Unmarshal([]byte(optionsJSON.String), &opts)
	}

	var links []models.ItemLink
	if opts.MirrorOfFieldID > 0 {
		// Mirror field: links are stored with custom_field_id = primary, target_id = this item
		links, err = h.getLinksWhere("il.custom_field_id = ? AND il.target_type = 'item' AND il.target_id = ?", opts.MirrorOfFieldID, itemID)
	} else {
		// Primary field: links are stored with custom_field_id = this field, source_id = this item
		links, err = h.getLinksWhere("il.custom_field_id = ? AND il.source_type = 'item' AND il.source_id = ?", fieldID, itemID)
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Filter by what the user can view — covers items (via workspace keys),
	// test_cases (workspace lookup), and assets (set lookup).
	user := utils.GetCurrentUser(r)
	if user != nil {
		accessibleKeys, _ := GetAccessibleWorkspaceKeys(user, h.db, h.permissionService)
		accessibleWsIDs := h.accessibleWorkspaceIDSet(user)
		accessibleSetIDs := h.accessibleAssetSetIDSet(user)
		links = h.filterLinksByAccess(links, accessibleKeys, accessibleWsIDs, accessibleSetIDs)
	}

	respondJSONOK(w, links)
}

// fieldLinkPlan captures the result of validateAndPrepareFieldLink so the
// caller can carry the "should enforce single-value" bit past the permission
// check and into enforceSingleValueFieldLink without re-reading field options.
type fieldLinkPlan struct {
	multi bool
}

// validateAndPrepareFieldLink validates custom field linking constraints and
// rewrites the link in place: mirror fields are resolved to their primary and
// source/target are swapped. It performs NO destructive writes — the caller
// must invoke enforceSingleValueFieldLink AFTER permission checks succeed, so
// an unauthorized request can't wipe existing field links on its way to
// being rejected.
func (h *ItemLinkHandler) validateAndPrepareFieldLink(link *models.ItemLink) (*fieldLinkPlan, error) {
	fieldID := *link.CustomFieldID

	// Get field definition
	var optionsJSON sql.NullString
	var fieldType string
	err := h.db.QueryRow("SELECT field_type, options FROM custom_field_definitions WHERE id = ?", fieldID).Scan(&fieldType, &optionsJSON)
	if err != nil {
		return nil, fmt.Errorf("custom field not found")
	}
	if fieldType != "linking" {
		return nil, fmt.Errorf("field is not a linking type")
	}

	if !optionsJSON.Valid {
		return nil, fmt.Errorf("field has no options configured")
	}

	var opts struct {
		LinkTypeID         int      `json:"link_type_id"`
		AllowedItemTypeIDs []int    `json:"allowed_item_type_ids"`
		AllowedEntityTypes []string `json:"allowed_entity_types"`
		Multi              bool     `json:"multi"`
		MirrorOfFieldID    int      `json:"mirror_of_field_id"`
		MirrorFieldID      int      `json:"mirror_field_id"`
	}
	if err := json.Unmarshal([]byte(optionsJSON.String), &opts); err != nil {
		return nil, fmt.Errorf("invalid field options")
	}

	isMirror := opts.MirrorOfFieldID > 0

	if isMirror {
		// Mirror field: resolve to primary field, swap source/target
		link.SourceType, link.TargetType = link.TargetType, link.SourceType
		link.SourceID, link.TargetID = link.TargetID, link.SourceID
		primaryID := opts.MirrorOfFieldID
		link.CustomFieldID = &primaryID

		// Get primary field options for validation
		var primaryOptsJSON sql.NullString
		if err := h.db.QueryRow("SELECT options FROM custom_field_definitions WHERE id = ?", primaryID).Scan(&primaryOptsJSON); err != nil {
			return nil, fmt.Errorf("primary field not found")
		}
		if primaryOptsJSON.Valid {
			_ = json.Unmarshal([]byte(primaryOptsJSON.String), &opts)
		}
	}

	// Validate link type matches
	if link.LinkTypeID != 0 && link.LinkTypeID != opts.LinkTypeID {
		return nil, fmt.Errorf("link type does not match field configuration")
	}
	link.LinkTypeID = opts.LinkTypeID

	// Validate target entity type
	if len(opts.AllowedEntityTypes) > 0 {
		allowed := false
		for _, et := range opts.AllowedEntityTypes {
			if et == link.TargetType {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("target entity type not allowed for this field")
		}
	}

	// Validate target item type
	if len(opts.AllowedItemTypeIDs) > 0 && link.TargetType == "item" {
		target, err := repository.NewItemRepository(h.db).FindByID(link.TargetID)
		if err == nil && target.ItemTypeID != nil {
			allowed := false
			for _, id := range opts.AllowedItemTypeIDs {
				if id == *target.ItemTypeID {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil, fmt.Errorf("target item type not allowed for this field")
			}
		}
	}

	return &fieldLinkPlan{multi: opts.Multi}, nil
}

// enforceSingleValueFieldLink deletes any existing links for the field on the
// same source so the caller's INSERT becomes the sole value. Called only
// after permission checks have already authorized the caller to modify the
// field's source.
func (h *ItemLinkHandler) enforceSingleValueFieldLink(link *models.ItemLink, plan *fieldLinkPlan) {
	if plan == nil || plan.multi || link.CustomFieldID == nil {
		return
	}
	_, _ = h.db.ExecWrite("DELETE FROM item_links WHERE custom_field_id = ? AND source_type = ? AND source_id = ?",
		*link.CustomFieldID, link.SourceType, link.SourceID)
}
