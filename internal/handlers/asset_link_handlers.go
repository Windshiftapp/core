package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// GetAssetLinks returns all links for an asset (incoming and outgoing)
func (h *AssetHandler) GetAssetLinks(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid asset ID", http.StatusBadRequest)
		return
	}

	// Get asset to check permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check view permission
	canView, err := h.canViewSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canView {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	// Get outgoing links (where this asset is the source)
	outgoingQuery := `
		SELECT il.id, il.link_type_id, il.source_type, il.source_id, il.target_type, il.target_id,
		       il.created_by, il.created_at,
		       lt.name as link_type_name, lt.color as link_type_color, lt.forward_label, lt.reverse_label,
		       CASE
		           WHEN il.target_type = 'item' THEN (SELECT title FROM items WHERE id = il.target_id)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer outgoingRows.Close()

	var outgoingLinks []models.ItemLink
	for outgoingRows.Next() {
		var link models.ItemLink
		err := outgoingRows.Scan(
			&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.CreatedBy, &link.CreatedAt,
			&link.LinkTypeName, &link.LinkTypeColor, &link.LinkTypeForwardLabel, &link.LinkTypeReverseLabel,
			&link.TargetTitle, &link.CreatedByName,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		outgoingLinks = append(outgoingLinks, link)
	}

	// Get incoming links (where this asset is the target)
	incomingQuery := `
		SELECT il.id, il.link_type_id, il.source_type, il.source_id, il.target_type, il.target_id,
		       il.created_by, il.created_at,
		       lt.name as link_type_name, lt.color as link_type_color, lt.forward_label, lt.reverse_label,
		       CASE
		           WHEN il.source_type = 'item' THEN (SELECT title FROM items WHERE id = il.source_id)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer incomingRows.Close()

	var incomingLinks []models.ItemLink
	for incomingRows.Next() {
		var link models.ItemLink
		err := incomingRows.Scan(
			&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID,
			&link.TargetType, &link.TargetID, &link.CreatedBy, &link.CreatedAt,
			&link.LinkTypeName, &link.LinkTypeColor, &link.LinkTypeForwardLabel, &link.LinkTypeReverseLabel,
			&link.SourceTitle, &link.CreatedByName,
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		incomingLinks = append(incomingLinks, link)
	}

	response := map[string]interface{}{
		"outgoing": outgoingLinks,
		"incoming": incomingLinks,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CreateAssetLinkRequest represents the request body for creating an asset link
type CreateAssetLinkRequest struct {
	LinkTypeID int    `json:"link_type_id"`
	TargetType string `json:"target_type"` // item, asset, test_case
	TargetID   int    `json:"target_id"`
}

// CreateAssetLink creates a link from an asset to another entity
func (h *AssetHandler) CreateAssetLink(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	assetID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid asset ID", http.StatusBadRequest)
		return
	}

	// Get asset to check permissions
	var setID int
	err = h.db.QueryRow("SELECT set_id FROM assets WHERE id = ?", assetID).Scan(&setID)
	if err == sql.ErrNoRows {
		http.Error(w, "Asset not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check edit permission
	canEdit, err := h.canEditSet(currentUser.ID, setID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !canEdit {
		http.Error(w, "Edit permission required", http.StatusForbidden)
		return
	}

	var req CreateAssetLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate target type
	validTargetTypes := map[string]bool{"item": true, "asset": true, "test_case": true}
	if !validTargetTypes[req.TargetType] {
		http.Error(w, "Invalid target_type. Must be 'item', 'asset', or 'test_case'", http.StatusBadRequest)
		return
	}

	// Prevent self-links
	if req.TargetType == "asset" && req.TargetID == assetID {
		http.Error(w, "Cannot create link to self", http.StatusBadRequest)
		return
	}

	// Verify link type exists and is active
	var linkTypeActive bool
	err = h.db.QueryRow("SELECT active FROM link_types WHERE id = ?", req.LinkTypeID).Scan(&linkTypeActive)
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

	now := time.Now()

	var linkID int64
	err = h.db.QueryRow(`
		INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by, created_at)
		VALUES (?, 'asset', ?, ?, ?, ?, ?) RETURNING id
	`, req.LinkTypeID, assetID, req.TargetType, req.TargetID, currentUser.ID, now).Scan(&linkID)

	if err != nil {
		// Check for unique constraint violation
		if err.Error() == "UNIQUE constraint failed: item_links.link_type_id, item_links.source_type, item_links.source_id, item_links.target_type, item_links.target_id" {
			http.Error(w, "Link already exists", http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}
