package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/utils"
)

type CollectionHandler struct {
	db database.Database
}

func NewCollectionHandler(db database.Database) *CollectionHandler {
	return &CollectionHandler{db: db}
}

// GetAll returns all collections accessible to the user
func (h *CollectionHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Support filtering by workspace_id and category_id
	workspaceIDParam := r.URL.Query().Get("workspace_id")
	categoryIDParam := r.URL.Query().Get("category_id")

	query := `
		SELECT c.id, c.name, c.description, c.ql_query, c.is_public, c.workspace_id, c.category_id, c.created_by,
		       c.created_at, c.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as creator_name,
		       COALESCE(u.email, '') as creator_email,
		       COALESCE(cc.name, '') as category_name,
		       COALESCE(cc.color, '') as category_color
		FROM collections c
		LEFT JOIN users u ON c.created_by = u.id
		LEFT JOIN collection_categories cc ON c.category_id = cc.id
		WHERE (c.is_public = true OR c.created_by = ?)`

	var args []interface{}
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}
	args = append(args, currentUser.ID)

	// Add workspace filter if provided
	if workspaceIDParam != "" {
		query += " AND c.workspace_id = ?"
		workspaceID, err := strconv.Atoi(workspaceIDParam)
		if err != nil {
			respondInvalidID(w, r, "workspace_id")
			return
		}
		args = append(args, workspaceID)
	}

	// Add category filter if provided
	if categoryIDParam != "" {
		query += " AND c.category_id = ?"
		categoryID, err := strconv.Atoi(categoryIDParam)
		if err != nil {
			respondInvalidID(w, r, "category_id")
			return
		}
		args = append(args, categoryID)
	}

	query += " ORDER BY c.created_at DESC"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	var collections []models.Collection
	for rows.Next() {
		var collection models.Collection
		var workspaceID sql.NullInt64
		var categoryID sql.NullInt64
		var createdBy sql.NullInt64

		err := rows.Scan(
			&collection.ID, &collection.Name, &collection.Description,
			&collection.QLQuery, &collection.IsPublic, &workspaceID, &categoryID, &createdBy,
			&collection.CreatedAt, &collection.UpdatedAt,
			&collection.CreatorName, &collection.CreatorEmail,
			&collection.CategoryName, &collection.CategoryColor,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if workspaceID.Valid {
			collection.WorkspaceID = new(int)
			*collection.WorkspaceID = int(workspaceID.Int64)
		}

		if categoryID.Valid {
			collection.CategoryID = new(int)
			*collection.CategoryID = int(categoryID.Int64)
		}

		if createdBy.Valid {
			collection.CreatedBy = new(int)
			*collection.CreatedBy = int(createdBy.Int64)
		}

		collections = append(collections, collection)
	}

	respondJSONOK(w, collections)
}

// Get returns a specific collection by ID
func (h *CollectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	query := `
		SELECT c.id, c.name, c.description, c.ql_query, c.is_public, c.workspace_id, c.category_id, c.created_by,
		       c.created_at, c.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as creator_name,
		       COALESCE(u.email, '') as creator_email,
		       COALESCE(cc.name, '') as category_name,
		       COALESCE(cc.color, '') as category_color
		FROM collections c
		LEFT JOIN users u ON c.created_by = u.id
		LEFT JOIN collection_categories cc ON c.category_id = cc.id
		WHERE c.id = ? AND (c.is_public = true OR c.created_by = ?)`

	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	var collection models.Collection
	var workspaceID sql.NullInt64
	var categoryID sql.NullInt64
	var createdBy sql.NullInt64

	err := h.db.QueryRow(query, id, currentUser.ID).Scan(
		&collection.ID, &collection.Name, &collection.Description,
		&collection.QLQuery, &collection.IsPublic, &workspaceID, &categoryID, &createdBy,
		&collection.CreatedAt, &collection.UpdatedAt,
		&collection.CreatorName, &collection.CreatorEmail,
		&collection.CategoryName, &collection.CategoryColor,
	)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "collection")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if workspaceID.Valid {
		collection.WorkspaceID = new(int)
		*collection.WorkspaceID = int(workspaceID.Int64)
	}

	if categoryID.Valid {
		collection.CategoryID = new(int)
		*collection.CategoryID = int(categoryID.Int64)
	}

	if createdBy.Valid {
		collection.CreatedBy = new(int)
		*collection.CreatedBy = int(createdBy.Int64)
	}

	respondJSONOK(w, collection)
}

// Create creates a new collection
func (h *CollectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var collection models.Collection
	if err := json.NewDecoder(r.Body).Decode(&collection); err != nil {
		respondBadRequest(w, r, "Invalid JSON: "+err.Error())
		return
	}

	// Validate required fields
	if collection.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	// CQL query is now optional for initial creation - can be empty for partial creation

	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	// Validate workspace_id if provided
	if collection.WorkspaceID != nil {
		var exists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", *collection.WorkspaceID).Scan(&exists)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to validate workspace: %w", err))
			return
		}
		if !exists {
			respondValidationError(w, r, "Workspace not found")
			return
		}
	}

	// Validate category_id if provided (only for global collections)
	if collection.CategoryID != nil {
		if collection.WorkspaceID != nil {
			respondValidationError(w, r, "Categories can only be applied to global collections (workspace_id must be null)")
			return
		}
		var exists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM collection_categories WHERE id = ?)", *collection.CategoryID).Scan(&exists)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to validate category: %w", err))
			return
		}
		if !exists {
			respondValidationError(w, r, "Category not found")
			return
		}
	}

	// Insert the collection
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO collections (name, description, ql_query, is_public, workspace_id, category_id, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'), datetime('now')) RETURNING id
	`, collection.Name, collection.Description, collection.QLQuery, collection.IsPublic, collection.WorkspaceID, collection.CategoryID, currentUser.ID).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the created collection
	collection.ID = int(id)
	collection.CreatedBy = &currentUser.ID

	respondJSONCreated(w, collection)
}

// Update updates an existing collection
func (h *CollectionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		respondBadRequest(w, r, "Failed to read request body: "+err.Error())
		return
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		respondBadRequest(w, r, "Invalid JSON: "+err.Error())
		return
	}

	var collection models.Collection
	if err := json.Unmarshal(bodyBytes, &collection); err != nil {
		respondBadRequest(w, r, "Invalid JSON: "+err.Error())
		return
	}

	_, workspaceProvided := payload["workspace_id"]
	_, categoryProvided := payload["category_id"]

	// Validate required fields
	if collection.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	// CQL query validation removed - allow updating collections without CQL query set

	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	// Check if collection exists and user has permission to edit
	var existingCreatedBy sql.NullInt64
	var existingWorkspaceID sql.NullInt64
	var existingCategoryID sql.NullInt64
	err = h.db.QueryRow("SELECT created_by, workspace_id, category_id FROM collections WHERE id = ?", id).Scan(&existingCreatedBy, &existingWorkspaceID, &existingCategoryID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "collection")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Only allow the creator to update their collection
	if !existingCreatedBy.Valid || int(existingCreatedBy.Int64) != currentUser.ID {
		respondForbidden(w, r)
		return
	}

	// Preserve workspace association unless the field is explicitly sent in the payload
	if !workspaceProvided {
		if existingWorkspaceID.Valid {
			val := int(existingWorkspaceID.Int64)
			collection.WorkspaceID = &val
		} else {
			collection.WorkspaceID = nil
		}
	}

	// Preserve category association unless the field is explicitly sent in the payload
	if !categoryProvided {
		if existingCategoryID.Valid {
			val := int(existingCategoryID.Int64)
			collection.CategoryID = &val
		} else {
			collection.CategoryID = nil
		}
	}

	// Validate workspace_id if provided
	if workspaceProvided && collection.WorkspaceID != nil {
		var exists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", *collection.WorkspaceID).Scan(&exists)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to validate workspace: %w", err))
			return
		}
		if !exists {
			respondValidationError(w, r, "Workspace not found")
			return
		}
	}

	// Validate category_id if provided (only for global collections)
	if categoryProvided && collection.CategoryID != nil {
		if collection.WorkspaceID != nil {
			respondValidationError(w, r, "Categories can only be applied to global collections (workspace_id must be null)")
			return
		}
		var exists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM collection_categories WHERE id = ?)", *collection.CategoryID).Scan(&exists)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to validate category: %w", err))
			return
		}
		if !exists {
			respondValidationError(w, r, "Category not found")
			return
		}
	}

	// Update the collection
	_, err = h.db.ExecWrite(`
		UPDATE collections
		SET name = ?, description = ?, ql_query = ?, is_public = ?, workspace_id = ?, category_id = ?, updated_at = datetime('now')
		WHERE id = ?
	`, collection.Name, collection.Description, collection.QLQuery, collection.IsPublic, collection.WorkspaceID, collection.CategoryID, id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return success
	respondJSONOK(w, map[string]string{"message": "Collection updated successfully"})
}

// Delete deletes a collection
func (h *CollectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get actual user ID from context/session
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}
	userID := currentUser.ID

	// Check if collection exists and user has permission to delete
	var existingCreatedBy sql.NullInt64
	err := h.db.QueryRow("SELECT created_by FROM collections WHERE id = ?", id).Scan(&existingCreatedBy)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "collection")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Only allow the creator to delete their collection
	if !existingCreatedBy.Valid || int(existingCreatedBy.Int64) != userID {
		respondForbidden(w, r)
		return
	}

	// Delete the collection
	_, err = h.db.ExecWrite("DELETE FROM collections WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]string{"message": "Collection deleted successfully"})
}
