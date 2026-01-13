package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"

)

type ItemLinkHandler struct {
	db                  database.Database
	notificationService interface {
		EmitEvent(event *services.NotificationEvent)
	} // Notification service for async notification processing (optional, can be nil)
}

func NewItemLinkHandler(db database.Database, notificationService interface {
	EmitEvent(event *services.NotificationEvent)
}) *ItemLinkHandler {
	return &ItemLinkHandler{
		db:                  db,
		notificationService: notificationService,
	}
}

// GetLinksForItem returns all links for a specific item (work item or test case)
func (h *ItemLinkHandler) GetLinksForItem(w http.ResponseWriter, r *http.Request) {
	itemType := r.PathValue("type") // "items" or "test-cases"
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// Convert URL path to internal type
	internalType := "item"
	if itemType == "test-cases" {
		internalType = "test_case"
	}

	// Get outgoing links (where this item is the source)
	outgoingLinks, err := h.getLinksWhere("source_type = ? AND source_id = ?", internalType, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get incoming links (where this item is the target)
	incomingLinks, err := h.getLinksWhere("target_type = ? AND target_id = ?", internalType, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"outgoing": outgoingLinks,
		"incoming": incomingLinks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CreateLink creates a new link between items
func (h *ItemLinkHandler) CreateLink(w http.ResponseWriter, r *http.Request) {
	var link models.ItemLink
	if err := json.NewDecoder(r.Body).Decode(&link); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if link.LinkTypeID == 0 || link.SourceType == "" || link.SourceID == 0 ||
		link.TargetType == "" || link.TargetID == 0 {
		http.Error(w, "link_type_id, source_type, source_id, target_type, and target_id are required", http.StatusBadRequest)
		return
	}

	// Validate source and target types
	if !isValidLinkType(link.SourceType) || !isValidLinkType(link.TargetType) {
		http.Error(w, "Invalid source_type or target_type. Must be 'item', 'test_case', or 'asset'", http.StatusBadRequest)
		return
	}

	// Prevent self-links
	if link.SourceType == link.TargetType && link.SourceID == link.TargetID {
		http.Error(w, "Cannot create link to self", http.StatusBadRequest)
		return
	}

	// Verify the link type exists and is active
	var linkTypeActive bool
	err := h.db.QueryRow("SELECT active FROM link_types WHERE id = ?", link.LinkTypeID).Scan(&linkTypeActive)
	if err == sql.ErrNoRows {
		http.Error(w, "Link type not found", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !linkTypeActive {
		http.Error(w, "Link type is not active", http.StatusBadRequest)
		return
	}

	// Special validation for "Tests" link type (ID = 1)
	// This link type can only link between items and test cases, not between same entity types
	if link.LinkTypeID == 1 {
		if link.SourceType == link.TargetType {
			http.Error(w, "The 'Tests' link type can only link between items and test cases, not between the same entity types", http.StatusBadRequest)
			return
		}
		// Ensure one is test_case and other is item
		if !((link.SourceType == "test_case" && link.TargetType == "item") ||
			(link.SourceType == "item" && link.TargetType == "test_case")) {
			http.Error(w, "The 'Tests' link type requires one entity to be a test case and the other to be an item", http.StatusBadRequest)
			return
		}
	}

	// Check if link already exists (in either direction)
	var existingID int
	err = h.db.QueryRow(`
		SELECT id FROM item_links
		WHERE (source_type = ? AND source_id = ? AND target_type = ? AND target_id = ?)
		   OR (source_type = ? AND source_id = ? AND target_type = ? AND target_id = ?)
	`, link.SourceType, link.SourceID, link.TargetType, link.TargetID,
		link.TargetType, link.TargetID, link.SourceType, link.SourceID).Scan(&existingID)

	if err == nil {
		http.Error(w, "A link between these items already exists", http.StatusConflict)
		return
	}
	if err != sql.ErrNoRows {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get created_by from authentication context
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}
	createdBy := currentUser.ID

	now := time.Now()

	var id int64
	err = h.db.QueryRow(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, link.LinkTypeID, link.SourceType, link.SourceID, link.TargetType, link.TargetID, createdBy, now).Scan(&id)

	if err != nil {
		// Check for unique constraint violation
		if err.Error() == "UNIQUE constraint failed: item_links.link_type_id, item_links.source_type, item_links.source_id, item_links.target_type, item_links.target_id" {
			http.Error(w, "Link already exists", http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	link.ID = int(id)
	link.CreatedAt = now

	// Get the created link with full details
	createdLink, err := h.getLinkByID(int(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Emit notification event (only for work item links)
	if h.notificationService != nil && link.SourceType == "item" {
		user := h.getUserFromContext(r)
		if user != nil {
			// Fetch source item details for notification
			var workspaceID int
			var itemTitle string
			var assigneeID, creatorID sql.NullInt64
			err := h.db.QueryRow("SELECT workspace_id, title, assignee_id, creator_id FROM items WHERE id = ?", link.SourceID).Scan(&workspaceID, &itemTitle, &assigneeID, &creatorID)
			if err == nil {
				var assigneeIDPtr, creatorIDPtr *int
				if assigneeID.Valid {
					val := int(assigneeID.Int64)
					assigneeIDPtr = &val
				}
				if creatorID.Valid {
					val := int(creatorID.Int64)
					creatorIDPtr = &val
				}

				h.notificationService.EmitEvent(&services.NotificationEvent{
					EventType:   models.EventItemLinked,
					WorkspaceID: workspaceID,
					ActorUserID: user.ID,
					ItemID:      link.SourceID,
					AssigneeID:  assigneeIDPtr,
					CreatorID:   creatorIDPtr,
					Title:       "Item Linked",
					TemplateData: map[string]interface{}{
						"item.title":   itemTitle,
						"item.id":      link.SourceID,
						"target.title": createdLink.TargetTitle,
						"target.id":    link.TargetID,
						"user.name":    user.Username,
					},
				})
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createdLink)
}

// DeleteLink removes a link
func (h *ItemLinkHandler) DeleteLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

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
		http.Error(w, "Link not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to fetch link details: "+err.Error(), http.StatusInternalServerError)
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM item_links WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Link not found", http.StatusNotFound)
		return
	}

	// Emit notification event (only for work item links)
	if h.notificationService != nil && sourceType == "item" {
		user := h.getUserFromContext(r)
		if user != nil {
			// Fetch source item details for notification
			var workspaceID int
			var itemTitle string
			var assigneeID, creatorID sql.NullInt64
			err := h.db.QueryRow("SELECT workspace_id, title, assignee_id, creator_id FROM items WHERE id = ?", sourceID).Scan(&workspaceID, &itemTitle, &assigneeID, &creatorID)
			if err == nil {
				var assigneeIDPtr, creatorIDPtr *int
				if assigneeID.Valid {
					val := int(assigneeID.Int64)
					assigneeIDPtr = &val
				}
				if creatorID.Valid {
					val := int(creatorID.Int64)
					creatorIDPtr = &val
				}

				h.notificationService.EmitEvent(&services.NotificationEvent{
					EventType:   models.EventItemUnlinked,
					WorkspaceID: workspaceID,
					ActorUserID: user.ID,
					ItemID:      sourceID,
					AssigneeID:  assigneeIDPtr,
					CreatorID:   creatorIDPtr,
					Title:       "Item Unlinked",
					TemplateData: map[string]interface{}{
						"item.title":   itemTitle,
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
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

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
		ID               int    `json:"id"`
		Title            string `json:"title"`
		Description      string `json:"description"`
		SetID            int    `json:"set_id"`
		SetName          string `json:"set_name"`
		TypeName         string `json:"type_name"`
		CategoryName     string `json:"category_name"`
		LinkID           int    `json:"link_id"`
		LinkTypeName     string `json:"link_type_name"`
		LinkLabel        string `json:"link_label"`
		Direction        string `json:"direction"` // "outgoing" or "incoming"
	}

	var linkedAssets []LinkedAsset

	// Process outgoing links
	rows, err := h.db.Query(outgoingQuery, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var asset LinkedAsset
		var description, setName, typeName, categoryName, linkLabel sql.NullString
		err := rows.Scan(&asset.ID, &asset.Title, &description, &asset.SetID, &setName,
			&typeName, &categoryName, &asset.LinkID, &asset.LinkTypeName, &linkLabel)
		if err != nil {
			rows.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		asset.Description = description.String
		asset.SetName = setName.String
		asset.TypeName = typeName.String
		asset.CategoryName = categoryName.String
		asset.LinkLabel = linkLabel.String
		asset.Direction = "outgoing"
		linkedAssets = append(linkedAssets, asset)
	}
	rows.Close()

	// Process incoming links
	rows, err = h.db.Query(incomingQuery, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for rows.Next() {
		var asset LinkedAsset
		var description, setName, typeName, categoryName, linkLabel sql.NullString
		err := rows.Scan(&asset.ID, &asset.Title, &description, &asset.SetID, &setName,
			&typeName, &categoryName, &asset.LinkID, &asset.LinkTypeName, &linkLabel)
		if err != nil {
			rows.Close()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		asset.Description = description.String
		asset.SetName = setName.String
		asset.TypeName = typeName.String
		asset.CategoryName = categoryName.String
		asset.LinkLabel = linkLabel.String
		asset.Direction = "incoming"
		linkedAssets = append(linkedAssets, asset)
	}
	rows.Close()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(linkedAssets)
}

// SearchLinkableItems searches for items that can be linked
func (h *ItemLinkHandler) SearchLinkableItems(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	itemType := r.URL.Query().Get("type") // "item", "test_case", "asset", or empty for all
	limit := 20

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	var items []models.LinkableItem

	// Search work items
	if itemType == "" || itemType == "item" {
		workItems, err := h.searchWorkItems(query, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items = append(items, workItems...)
	}

	// Search test cases
	if itemType == "" || itemType == "test_case" {
		testCases, err := h.searchTestCases(query, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items = append(items, testCases...)
	}

	// Search assets
	if itemType == "" || itemType == "asset" {
		assets, err := h.searchAssets(query, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items = append(items, assets...)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
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
		       ti.status_id as target_status_id,
		       COALESCE(ts.name, '') as target_status_name,
		       ti.item_type_id as target_item_type_id,
		       COALESCE(tit.name, '') as target_item_type_name,
		       COALESCE(tit.icon, '') as target_item_type_icon,
		       COALESCE(tit.color, '') as target_item_type_color,
		       COALESCE(tw.key, '') as target_workspace_key
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
		WHERE ` + whereClause + `
		ORDER BY lt.name, il.created_at DESC
	`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []models.ItemLink
	for rows.Next() {
		var link models.ItemLink
		err := rows.Scan(&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.CreatedBy, &link.CreatedAt,
			&link.LinkTypeName, &link.LinkTypeColor, &link.LinkTypeForwardLabel, &link.LinkTypeReverseLabel,
			&link.SourceTitle, &link.TargetTitle, &link.CreatedByName,
			&link.SourceStatusID, &link.SourceStatusName,
			&link.SourceItemTypeID, &link.SourceItemTypeName, &link.SourceItemTypeIcon, &link.SourceItemTypeColor,
			&link.SourceWorkspaceKey,
			&link.TargetStatusID, &link.TargetStatusName,
			&link.TargetItemTypeID, &link.TargetItemTypeName, &link.TargetItemTypeIcon, &link.TargetItemTypeColor,
			&link.TargetWorkspaceKey)
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

func (h *ItemLinkHandler) searchWorkItems(query string, limit int) ([]models.LinkableItem, error) {
	sqlQuery := `
		SELECT 
			i.id, 
			i.title, 
			COALESCE(i.description, '') AS description,
			i.workspace_id,
			w.name AS workspace_name,
			COALESCE(s.name, '') AS status_name,
			COALESCE(p.name, '') AS priority_name
		FROM items i
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		WHERE i.title LIKE ? OR i.description LIKE ?
		ORDER BY i.title
		LIMIT ?
	`

	searchTerm := "%" + query + "%"
	rows, err := h.db.Query(sqlQuery, searchTerm, searchTerm, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.LinkableItem
	for rows.Next() {
		var item models.LinkableItem
		var description sql.NullString
		var workspaceID sql.NullInt64
		var workspaceName sql.NullString
		var statusName sql.NullString
		var priorityName sql.NullString

		err := rows.Scan(
			&item.ID,
			&item.Title,
			&description,
			&workspaceID,
			&workspaceName,
			&statusName,
			&priorityName,
		)
		if err != nil {
			return nil, err
		}

		item.Description = description.String
		item.WorkspaceID = utils.NullInt64ToPtr(workspaceID)
		item.WorkspaceName = workspaceName.String
		item.Status = statusName.String
		item.Priority = priorityName.String

		item.Type = "item"
		items = append(items, item)
	}

	return items, nil
}

func (h *ItemLinkHandler) searchTestCases(query string, limit int) ([]models.LinkableItem, error) {
	sqlQuery := `
		SELECT id, title, COALESCE(preconditions, '') AS summary
		FROM test_cases
		WHERE title LIKE ? OR preconditions LIKE ?
		ORDER BY title
		LIMIT ?
	`

	searchTerm := "%" + query + "%"
	rows, err := h.db.Query(sqlQuery, searchTerm, searchTerm, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

func (h *ItemLinkHandler) searchAssets(query string, limit int) ([]models.LinkableItem, error) {
	sqlQuery := `
		SELECT a.id, a.title, COALESCE(a.description, '') AS description,
		       a.set_id, ams.name AS set_name,
		       COALESCE(at.name, '') AS type_name,
		       COALESCE(ac.name, '') AS category_name
		FROM assets a
		LEFT JOIN asset_management_sets ams ON a.set_id = ams.id
		LEFT JOIN asset_types at ON a.asset_type_id = at.id
		LEFT JOIN asset_categories ac ON a.category_id = ac.id
		WHERE a.title LIKE ? OR a.description LIKE ?
		ORDER BY a.title
		LIMIT ?
	`

	searchTerm := "%" + query + "%"
	rows, err := h.db.Query(sqlQuery, searchTerm, searchTerm, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

// getUserFromContext retrieves the authenticated user from request context
func (h *ItemLinkHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value("user"); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}
