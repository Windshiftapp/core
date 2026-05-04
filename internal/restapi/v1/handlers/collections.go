package handlers

import (
	"database/sql"
	"net/http"
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

// collectionRow holds the slug-addressed collection's gating-relevant columns
// alongside the response fields. It's the result of a single SELECT so we
// don't issue a second query to check visibility.
type collectionRow struct {
	resp      CollectionResponse
	isPublic  bool
	createdBy *int
}

// Get handles GET /rest/api/v1/collections/{slug}.
func (h *CollectionHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid collection slug"))
		return
	}

	row, err := h.loadCollectionBySlug(slug)
	if err != nil {
		if err == sql.ErrNoRows {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Collection not found"))
			return
		}
		h.RespondInternalError(w, r)
		return
	}

	if !h.canViewCollection(user.ID, row) {
		// 404 not 403 — never leak that the slug exists.
		h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Collection not found"))
		return
	}

	h.RespondOK(w, row.resp)
}

// GetItems handles GET /rest/api/v1/collections/{slug}/items.
// Resolves the collection's QL query through the existing CQL evaluator
// (with the OAuth user threaded into currentUser()-style functions) and
// applies the caller's accessible-workspaces filter for per-row gating.
func (h *CollectionHandler) GetItems(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}

	slug := strings.TrimSpace(r.PathValue("slug"))
	if slug == "" {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid collection slug"))
		return
	}

	row, err := h.loadCollectionBySlug(slug)
	if err != nil {
		if err == sql.ErrNoRows {
			h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Collection not found"))
			return
		}
		h.RespondInternalError(w, r)
		return
	}
	if !h.canViewCollection(user.ID, row) {
		h.RespondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Collection not found"))
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
		CollectionID: row.resp.ID,
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
