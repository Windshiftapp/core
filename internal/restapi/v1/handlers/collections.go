package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/dto"
	"windshift/internal/services"
)

// CollectionHandler exposes read-only collection access via the v1 REST API.
// Collections are addressed by their `public_slug` so external embed clients
// (notably the planned docmost integration) can persist a stable handle that
// survives renumbering or recreation of the underlying row.
type CollectionHandler struct {
	BaseHandler
	itemCRUD *services.ItemCRUDService
}

func NewCollectionHandler(db database.Database, permissionService *services.PermissionService) *CollectionHandler {
	return &CollectionHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		itemCRUD:    services.NewItemCRUDService(db),
	}
}

// CollectionResponse is the public v1 representation of a Collection.
// Field set is deliberately small — it's what an embed needs to render a header
// and ask for items, not the full edit-time payload.
type CollectionResponse struct {
	ID          int    `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	WorkspaceID *int   `json:"workspace_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CollectionListResponse is the response shape of GET /collections — a
// pre-pagination search response with a flat result count rather than the
// standard PaginatedResponse envelope.
type CollectionListResponse struct {
	Items []CollectionResponse `json:"items"`
	Total int                  `json:"total"`
}

// collectionRow holds the slug-addressed collection's gating-relevant columns
// alongside the response fields. It's the result of a single SELECT so we
// don't issue a second query to check visibility.
type collectionRow struct {
	resp      CollectionResponse
	isPublic  bool
	createdBy *int
}

// Get handles GET /rest/api/v1/collections/{key}. `key` is either a numeric
// id or a public_slug — the handler dispatches based on whether the path
// value parses as an integer. Both addressing modes are supported because
// public_slug is rarely set in practice (it's an opt-in for anonymous
// public-share boards), so embed clients usually persist the numeric id;
// slugs remain useful when present because they survive re-creation of
// the underlying row.
//
// @Summary      Get a collection by id or slug
// @Description  `key` is either a numeric collection id or its `public_slug`. Permission failures return 404 — collection existence is never leaked.
// @Tags         collections
// @Produce      json
// @Security     BearerAuth
// @Param        key  path      string  true  "Collection id (numeric) or public_slug"
// @Success      200  {object}  handlers.CollectionResponse
// @Failure      400  {object}  restapi.ErrorResponse  "Invalid collection key"
// @Failure      401  {object}  restapi.ErrorResponse
// @Failure      403  {object}  restapi.ErrorResponse  "Token lacks the collections:read scope"
// @Failure      404  {object}  restapi.ErrorResponse  "Collection not found or not visible to caller"
// @Failure      500  {object}  restapi.ErrorResponse
// @Router       /collections/{key} [get]
func (h *CollectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	row, found := h.loadCollectionByKey(w, r)
	if !found {
		return
	}
	if !h.canViewCollection(user.ID, row) {
		// 404 not 403 — never leak that the key exists.
		h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Collection not found"))
		return
	}
	h.RespondOK(w, row.resp)
}

// GetItems handles GET /rest/api/v1/collections/{key}/items. Resolves the
// collection's QL query through the existing CQL evaluator (with the OAuth
// user threaded into currentUser()-style functions) and applies the caller's
// accessible-workspaces filter for per-row gating. Same {key} dispatch as
// Get — numeric → by id, otherwise → by slug.
//
// @Summary      List items in a collection
// @Description  Resolves the collection's saved QL query and returns matching items, gated by the caller's accessible workspaces. Permission failures return 404.
// @Tags         collections
// @Produce      json
// @Security     BearerAuth
// @Param        key    path      string  true   "Collection id (numeric) or public_slug"
// @Param        page   query     int     false  "Page number (1-based)"
// @Param        limit  query     int     false  "Items per page (max 100)"
// @Param        sort   query     string  false  "Sort field"
// @Param        order  query     string  false  "Sort order: asc or desc"
// @Success      200    {object}  restapi.PaginatedResponse{data=[]dto.ItemResponse}
// @Failure      400    {object}  restapi.ErrorResponse  "Invalid QL query stored on the collection"
// @Failure      401    {object}  restapi.ErrorResponse
// @Failure      403    {object}  restapi.ErrorResponse  "Token lacks collections:read or items:read scope"
// @Failure      404    {object}  restapi.ErrorResponse  "Collection not found or not visible to caller"
// @Failure      500    {object}  restapi.ErrorResponse
// @Router       /collections/{key}/items [get]
func (h *CollectionHandler) GetItems(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	row, found := h.loadCollectionByKey(w, r)
	if !found {
		return
	}
	if !h.canViewCollection(user.ID, row) {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Collection not found"))
		return
	}
	h.respondCollectionItems(w, r, row.resp.ID)
}

// loadCollectionByKey extracts {key} from the path and loads via numeric id
// or slug accordingly. Writes the appropriate 4xx response and returns
// (nil, false) when the caller should stop processing.
func (h *CollectionHandler) loadCollectionByKey(w http.ResponseWriter, r *http.Request) (*collectionRow, bool) {
	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid collection key"))
		return nil, false
	}
	var (
		row *collectionRow
		err error
	)
	if id, atoiErr := strconv.Atoi(key); atoiErr == nil {
		row, err = h.loadCollectionByID(id)
	} else {
		row, err = h.loadCollectionBySlug(key)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Collection not found"))
			return nil, false
		}
		h.RespondInternalError(w, r)
		return nil, false
	}
	return row, true
}

// respondCollectionItems shared between GetItems (slug) and GetItemsByID (id).
// Caller has already verified the user can view the collection.
func (h *CollectionHandler) respondCollectionItems(w http.ResponseWriter, r *http.Request, collectionID int) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	pagination := h.ParsePagination(r)

	accessibleWorkspaceIDs, err := h.Perms.GetAccessibleWorkspaceIDs(user.ID)
	if err != nil {
		h.RespondError(w, r, restapi.ErrInternalError.WithDetails(map[string]string{
			"message": "Failed to get accessible workspaces",
		}))
		return
	}
	if len(accessibleWorkspaceIDs) == 0 {
		h.RespondPaginated(w, []dto.ItemResponse{}, pagination, 0)
		return
	}

	items, total, err := h.itemCRUD.ListWithQL(services.ListWithQLParams{
		CollectionID: collectionID,
		WorkspaceIDs: accessibleWorkspaceIDs,
		UserID:       user.ID,
		Pagination: services.PaginationParams{
			Limit:  pagination.Limit,
			Offset: pagination.Offset,
		},
		SortBy:  pagination.SortBy,
		SortAsc: pagination.SortAsc,
	})
	if err != nil {
		if strings.Contains(err.Error(), "QL query error:") {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeValidationFailed, err.Error()))
			return
		}
		if strings.Contains(err.Error(), "collection not found") {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Collection not found"))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	baseURL := getBaseURL(r)
	itemResponses := dto.MapItemsToResponse(items, baseURL)
	h.RespondPaginated(w, itemResponses, pagination, total)
}

// List handles GET /rest/api/v1/collections?q=&limit=. Substring-matches the
// `q` against collection name (case-insensitive) and returns the rows the
// caller can view. Used by the docmost embed picker — slugs are rare on
// real-world collections, so a name-search picker is the discoverable UX.
//
// @Summary      Search collections by name
// @Description  Case-insensitive substring match on collection name. Results are filtered to collections the caller can view.
// @Tags         collections
// @Produce      json
// @Security     BearerAuth
// @Param        q      query     string  false  "Substring to match against collection name"
// @Param        limit  query     int     false  "Maximum results (default 20, max 100)"
// @Success      200    {object}  handlers.CollectionListResponse
// @Failure      401    {object}  restapi.ErrorResponse
// @Failure      403    {object}  restapi.ErrorResponse  "Token lacks the collections:read scope"
// @Failure      500    {object}  restapi.ErrorResponse
// @Router       /collections [get]
func (h *CollectionHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	pagination := h.ParsePagination(r)
	limit := pagination.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	// Pull a wider candidate set than `limit` so per-row visibility filtering
	// still has enough rows to return up to `limit` visible matches. Capped to
	// avoid pathological scans on huge collection tables.
	const candidateMultiplier = 5
	const candidateCap = 500
	candidateLimit := limit * candidateMultiplier
	if candidateLimit > candidateCap {
		candidateLimit = candidateCap
	}

	pattern := "%" + strings.ToLower(q) + "%"
	rows, err := h.DB.Query(`
		SELECT id, name, COALESCE(description, ''), workspace_id, is_public, created_by,
		       COALESCE(public_slug, ''), created_at, updated_at
		FROM collections
		WHERE LOWER(name) LIKE ?
		ORDER BY name ASC
		LIMIT ?
	`, pattern, candidateLimit)
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}
	defer func() { _ = rows.Close() }()

	results := make([]CollectionResponse, 0, limit)
	for rows.Next() {
		var (
			row         collectionRow
			description sql.NullString
			workspaceID sql.NullInt64
			createdBy   sql.NullInt64
			slug        string
		)
		if err := rows.Scan(
			&row.resp.ID, &row.resp.Name, &description,
			&workspaceID, &row.isPublic, &createdBy,
			&slug, &row.resp.CreatedAt, &row.resp.UpdatedAt,
		); err != nil {
			continue
		}
		if description.Valid {
			row.resp.Description = description.String
		}
		if workspaceID.Valid {
			id := int(workspaceID.Int64)
			row.resp.WorkspaceID = &id
		}
		if createdBy.Valid {
			id := int(createdBy.Int64)
			row.createdBy = &id
		}
		row.resp.Slug = slug

		if !h.canViewCollection(user.ID, &row) {
			continue
		}
		results = append(results, row.resp)
		if len(results) >= limit {
			break
		}
	}

	h.RespondOK(w, map[string]interface{}{
		"items": results,
		"total": len(results),
	})
}

// loadCollectionByID fetches a numeric-id-addressed collection. Mirrors
// loadCollectionBySlug but doesn't require public_slug to be set.
func (h *CollectionHandler) loadCollectionByID(id int) (*collectionRow, error) {
	var (
		row         collectionRow
		description sql.NullString
		workspaceID sql.NullInt64
		createdBy   sql.NullInt64
		slug        sql.NullString
	)
	err := h.DB.QueryRow(`
		SELECT id, name, COALESCE(description, ''), workspace_id, is_public, created_by,
		       public_slug, created_at, updated_at
		FROM collections
		WHERE id = ?
	`, id).Scan(
		&row.resp.ID, &row.resp.Name, &description,
		&workspaceID, &row.isPublic, &createdBy,
		&slug, &row.resp.CreatedAt, &row.resp.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if description.Valid {
		row.resp.Description = description.String
	}
	if workspaceID.Valid {
		id := int(workspaceID.Int64)
		row.resp.WorkspaceID = &id
	}
	if createdBy.Valid {
		id := int(createdBy.Int64)
		row.createdBy = &id
	}
	if slug.Valid {
		row.resp.Slug = slug.String
	}
	return &row, nil
}

// loadCollectionBySlug fetches a slug-addressed collection plus the columns
// needed for the visibility check. Returns sql.ErrNoRows when no such
// addressable collection exists.
func (h *CollectionHandler) loadCollectionBySlug(slug string) (*collectionRow, error) {
	var (
		row         collectionRow
		description sql.NullString
		workspaceID sql.NullInt64
		createdBy   sql.NullInt64
	)
	err := h.DB.QueryRow(`
		SELECT id, name, COALESCE(description, ''), workspace_id, is_public, created_by, created_at, updated_at
		FROM collections
		WHERE public_slug = ? AND public_slug IS NOT NULL
	`, slug).Scan(
		&row.resp.ID, &row.resp.Name, &description,
		&workspaceID, &row.isPublic, &createdBy,
		&row.resp.CreatedAt, &row.resp.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	row.resp.Slug = slug
	if description.Valid {
		row.resp.Description = description.String
	}
	if workspaceID.Valid {
		id := int(workspaceID.Int64)
		row.resp.WorkspaceID = &id
	}
	if createdBy.Valid {
		id := int(createdBy.Int64)
		row.createdBy = &id
	}
	return &row, nil
}

// canViewCollection decides whether the caller may see a slug-addressed
// collection's metadata + items. Workspace-scoped collections are visible to
// anyone with `item.view` on that workspace (matches the natural mental model
// for embedded reports). Global collections fall back to the legacy
// is_public-or-creator check from internal/handlers/collections.go.
func (h *CollectionHandler) canViewCollection(userID int, row *collectionRow) bool {
	if row.resp.WorkspaceID != nil {
		allowed, err := h.Perms.CanViewWorkspace(userID, *row.resp.WorkspaceID)
		return err == nil && allowed
	}
	if row.isPublic {
		return true
	}
	if row.createdBy != nil && *row.createdBy == userID {
		return true
	}
	return false
}
