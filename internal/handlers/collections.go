package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

type CollectionHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewCollectionHandler(db database.Database, permissionService *services.PermissionService) *CollectionHandler {
	return &CollectionHandler{db: db, permissionService: permissionService}
}

// requireCollectionOwner authenticates the user, verifies the collection
// exists, and checks that the authenticated user is its creator.
// Returns the authenticated user and true on success, or writes an HTTP error
// and returns nil/false on failure.
func (h *CollectionHandler) requireCollectionOwner(w http.ResponseWriter, r *http.Request, collectionID int) (*models.User, bool) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return nil, false
	}

	var existingCreatedBy sql.NullInt64
	err := h.db.QueryRow("SELECT created_by FROM collections WHERE id = ?", collectionID).Scan(&existingCreatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "collection")
		return nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}

	if !existingCreatedBy.Valid || int(existingCreatedBy.Int64) != currentUser.ID {
		respondForbidden(w, r)
		return nil, false
	}

	return currentUser, true
}

// GetAll returns all collections accessible to the user
func (h *CollectionHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Support filtering by workspace_id and category_id
	workspaceIDParam := r.URL.Query().Get("workspace_id")
	categoryIDParam := r.URL.Query().Get("category_id")

	query := `
		SELECT c.id, c.name, c.description, c.ql_query, c.filter_state, c.is_public, c.workspace_id, c.category_id, c.created_by,
		       c.public_slug, c.created_at, c.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as creator_name,
		       COALESCE(u.email, '') as creator_email,
		       COALESCE(cc.name, '') as category_name,
		       COALESCE(cc.color, '') as category_color
		FROM collections c
		LEFT JOIN users u ON c.created_by = u.id
		LEFT JOIN collection_categories cc ON c.category_id = cc.id
		WHERE (c.is_public = true OR c.created_by = ?)`

	var args []interface{}
	currentUser, ok := RequireAuth(w, r)
	if !ok {
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
	defer func() { _ = rows.Close() }()

	var collections []models.Collection
	for rows.Next() {
		collection, err := scanCollection(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		collections = append(collections, collection)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
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
		SELECT c.id, c.name, c.description, c.ql_query, c.filter_state, c.is_public, c.workspace_id, c.category_id, c.created_by,
		       c.public_slug, c.created_at, c.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as creator_name,
		       COALESCE(u.email, '') as creator_email,
		       COALESCE(cc.name, '') as category_name,
		       COALESCE(cc.color, '') as category_color
		FROM collections c
		LEFT JOIN users u ON c.created_by = u.id
		LEFT JOIN collection_categories cc ON c.category_id = cc.id
		WHERE c.id = ? AND (c.is_public = true OR c.created_by = ?)`

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	collection, err := scanCollection(h.db.QueryRow(query, id, currentUser.ID))
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "collection")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, collection)
}

// Create creates a new collection
func (h *CollectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	collection, ok := decodeJSON[models.Collection](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if collection.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	// CQL query is now optional for initial creation - can be empty for partial creation

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Check public board permission if trying to make collection public or set slug
	if collection.IsPublic || collection.PublicSlug != nil {
		isAdmin, _ := h.permissionService.IsSystemAdmin(currentUser.ID)
		hasPerm, _ := h.permissionService.HasGlobalPermission(currentUser.ID, models.PermissionPublicBoardManage)
		if !isAdmin && !hasPerm {
			respondForbidden(w, r)
			return
		}
	}

	// Validate public_slug if provided
	if collection.PublicSlug != nil && *collection.PublicSlug != "" {
		if !slugRegex.MatchString(*collection.PublicSlug) {
			respondValidationError(w, r, "Public slug must be 3-64 characters, lowercase alphanumeric and hyphens only")
			return
		}
	}

	// Validate workspace_id if provided — check user has view permission
	if collection.WorkspaceID != nil {
		if !RequireWorkspacePermission(w, r, currentUser.ID, *collection.WorkspaceID,
			models.PermissionItemView, h.permissionService) {
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
		INSERT INTO collections (name, description, ql_query, filter_state, is_public, workspace_id, category_id, created_by, public_slug, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, collection.Name, collection.Description, collection.QLQuery, collection.FilterState, collection.IsPublic, collection.WorkspaceID, collection.CategoryID, currentUser.ID, collection.PublicSlug).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the created collection
	collection.ID = int(id)
	collection.CreatedBy = &currentUser.ID

	logAudit(h.db, r, currentUser, logger.ActionCollectionCreate, logger.ResourceCollection, &collection.ID, collection.Name)

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
	if err = json.Unmarshal(bodyBytes, &payload); err != nil {
		respondBadRequest(w, r, "Invalid JSON: "+err.Error())
		return
	}

	var collection models.Collection
	if err = json.Unmarshal(bodyBytes, &collection); err != nil {
		respondBadRequest(w, r, "Invalid JSON: "+err.Error())
		return
	}

	_, workspaceProvided := payload["workspace_id"]
	_, categoryProvided := payload["category_id"]
	_, isPublicProvided := payload["is_public"]
	_, publicSlugProvided := payload["public_slug"]
	_, filterStateProvided := payload["filter_state"]

	// Validate required fields
	if collection.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	// CQL query validation removed - allow updating collections without CQL query set

	currentUser, ok := h.requireCollectionOwner(w, r, id)
	if !ok {
		return
	}

	// Fetch existing values for field preservation
	var existingWorkspaceID sql.NullInt64
	var existingCategoryID sql.NullInt64
	var existingPublicSlug sql.NullString
	var existingFilterState sql.NullString
	var existingIsPublic bool
	err = h.db.QueryRow("SELECT workspace_id, category_id, public_slug, filter_state, is_public FROM collections WHERE id = ?", id).Scan(&existingWorkspaceID, &existingCategoryID, &existingPublicSlug, &existingFilterState, &existingIsPublic)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Preserve is_public unless the field is explicitly sent in the payload
	if !isPublicProvided {
		collection.IsPublic = existingIsPublic
	}

	// Check public board permission if trying to change public status or slug
	changingPublic := isPublicProvided && collection.IsPublic != existingIsPublic
	changingSlug := publicSlugProvided
	if changingPublic || changingSlug {
		isAdmin, _ := h.permissionService.IsSystemAdmin(currentUser.ID)
		hasPerm, _ := h.permissionService.HasGlobalPermission(currentUser.ID, models.PermissionPublicBoardManage)
		if !isAdmin && !hasPerm {
			respondForbidden(w, r)
			return
		}
	}

	// Validate public_slug if provided
	if publicSlugProvided && collection.PublicSlug != nil && *collection.PublicSlug != "" {
		if !slugRegex.MatchString(*collection.PublicSlug) {
			respondValidationError(w, r, "Public slug must be 3-64 characters, lowercase alphanumeric and hyphens only")
			return
		}
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

	// Preserve public_slug unless the field is explicitly sent in the payload
	if !publicSlugProvided {
		if existingPublicSlug.Valid {
			collection.PublicSlug = &existingPublicSlug.String
		} else {
			collection.PublicSlug = nil
		}
	}

	// Preserve filter_state unless the field is explicitly sent in the payload.
	// An explicit null in the payload (raw mode) clears it.
	if !filterStateProvided {
		if existingFilterState.Valid {
			collection.FilterState = &existingFilterState.String
		} else {
			collection.FilterState = nil
		}
	}

	// Validate workspace_id if provided — check user has view permission
	if workspaceProvided && collection.WorkspaceID != nil {
		if !RequireWorkspacePermission(w, r, currentUser.ID, *collection.WorkspaceID,
			models.PermissionItemView, h.permissionService) {
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
		err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM collection_categories WHERE id = ?)", *collection.CategoryID).Scan(&exists)
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
		SET name = ?, description = ?, ql_query = ?, filter_state = ?, is_public = ?, workspace_id = ?, category_id = ?, public_slug = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, collection.Name, collection.Description, collection.QLQuery, collection.FilterState, collection.IsPublic, collection.WorkspaceID, collection.CategoryID, collection.PublicSlug, id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionCollectionUpdate, logger.ResourceCollection, &id, collection.Name)

	// Return success
	respondJSONOK(w, map[string]string{"message": "Collection updated successfully"})
}

// UpdatePublicSharing updates only the public sharing fields of a collection
func (h *CollectionHandler) UpdatePublicSharing(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var payload struct {
		IsPublic   bool    `json:"is_public"`
		PublicSlug *string `json:"public_slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		respondBadRequest(w, r, "Invalid JSON: "+err.Error())
		return
	}

	currentUser, ok := h.requireCollectionOwner(w, r, id)
	if !ok {
		return
	}

	// Check public board permission
	isAdmin, _ := h.permissionService.IsSystemAdmin(currentUser.ID)
	hasPerm, _ := h.permissionService.HasGlobalPermission(currentUser.ID, models.PermissionPublicBoardManage)
	if !isAdmin && !hasPerm {
		respondForbidden(w, r)
		return
	}

	// Validate slug when enabling public sharing
	if payload.IsPublic {
		if payload.PublicSlug == nil || *payload.PublicSlug == "" {
			respondValidationError(w, r, "Public slug is required when enabling public sharing")
			return
		}
		if !slugRegex.MatchString(*payload.PublicSlug) {
			respondValidationError(w, r, "Public slug must be 3-64 characters, lowercase alphanumeric and hyphens only")
			return
		}
	}

	_, err := h.db.ExecWrite(
		"UPDATE collections SET is_public = ?, public_slug = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		payload.IsPublic, payload.PublicSlug, id,
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "This public slug is already in use")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionCollectionUpdate, logger.ResourceCollection, &id, "")

	respondJSONOK(w, map[string]interface{}{
		"is_public":   payload.IsPublic,
		"public_slug": payload.PublicSlug,
	})
}

// Delete deletes a collection
func (h *CollectionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	currentUser, ok := h.requireCollectionOwner(w, r, id)
	if !ok {
		return
	}

	// Delete the collection
	_, err := h.db.ExecWrite("DELETE FROM collections WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionCollectionDelete, logger.ResourceCollection, &id, "")

	respondJSONOK(w, map[string]string{"message": "Collection deleted successfully"})
}

// scanCollection scans a collection row (including joined user and category
// fields) and hydrates nullable pointer fields.
func scanCollection(s interface{ Scan(dest ...any) error }) (models.Collection, error) {
	var c models.Collection
	var workspaceID sql.NullInt64
	var categoryID sql.NullInt64
	var createdBy sql.NullInt64
	var publicSlug sql.NullString
	var filterState sql.NullString

	err := s.Scan(
		&c.ID, &c.Name, &c.Description,
		&c.QLQuery, &filterState, &c.IsPublic, &workspaceID, &categoryID, &createdBy,
		&publicSlug, &c.CreatedAt, &c.UpdatedAt,
		&c.CreatorName, &c.CreatorEmail,
		&c.CategoryName, &c.CategoryColor,
	)
	if err != nil {
		return c, err
	}

	if workspaceID.Valid {
		v := int(workspaceID.Int64)
		c.WorkspaceID = &v
	}
	if categoryID.Valid {
		v := int(categoryID.Int64)
		c.CategoryID = &v
	}
	if createdBy.Valid {
		v := int(createdBy.Int64)
		c.CreatedBy = &v
	}
	if publicSlug.Valid {
		c.PublicSlug = &publicSlug.String
	}
	if filterState.Valid {
		c.FilterState = &filterState.String
	}

	return c, nil
}
