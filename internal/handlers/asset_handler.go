package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// AssetHandler handles asset management operations on the cookie-auth
// surface. Per-set role check lives on services.AssetPermissionService;
// asset mutations (create / update / delete / CSV import) plus their
// audit + automation event emission live on services.AssetService. Both
// the cookie-auth and the bearer-auth v1 handler share one instance of
// each so the two surfaces produce identical audit rows.
type AssetHandler struct {
	db                database.Database
	repo              *repository.AssetRepository
	permissionService *services.PermissionService
	assetPerm         *services.AssetPermissionService
	assetService      *services.AssetService
	attachmentPath    string
}

// NewAssetHandler creates a new asset handler
func NewAssetHandler(db database.Database, permissionService *services.PermissionService, attachmentPath string) *AssetHandler {
	repo := repository.NewAssetRepository(db)
	return &AssetHandler{
		db:                db,
		repo:              repo,
		permissionService: permissionService,
		assetPerm:         services.NewAssetPermissionService(repo, permissionService),
		assetService:      services.NewAssetService(db, repo),
		attachmentPath:    attachmentPath,
	}
}

// AssetPermissionService returns the per-set permission service this handler
// delegates to, so callers wiring up the v1 surface can share the same
// instance instead of constructing a parallel one.
func (h *AssetHandler) AssetPermissionService() *services.AssetPermissionService {
	return h.assetPerm
}

// AssetService returns the shared mutation, audit, and durable-event service.
func (h *AssetHandler) AssetService() *services.AssetService {
	return h.assetService
}

// Role name constants — these are response-shape strings, not used by the
// permission service. Kept here next to the only callers that need them.
const (
	AssetRoleAdministrator = "Administrator"
)

// hasAssetPermission delegates to AssetPermissionService.
func (h *AssetHandler) hasAssetPermission(userID, setID int, permissionKey string) (bool, error) {
	return h.assetPerm.HasAssetSetPermission(userID, setID, permissionKey)
}

// HasAssetSetPermission satisfies services.AssetSetPermissionChecker so the
// action service and item-link orchestration can keep accepting this handler
// as a permission source — they now transparently hit the shared service.
func (h *AssetHandler) HasAssetSetPermission(userID, setID int, permissionKey string) (bool, error) {
	return h.assetPerm.HasAssetSetPermission(userID, setID, permissionKey)
}

// requireSetAdminAccess checks auth, parses setId, and verifies admin permission.
func (h *AssetHandler) requireSetAdminAccess(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	return h.requireSetAccess(w, r, services.AssetPermissionKeyAdmin)
}

// requireSetAccess checks auth, parses setId, and verifies the given permission.
func (h *AssetHandler) requireSetAccess(w http.ResponseWriter, r *http.Request, permissionKey string) (*models.User, int, bool) {
	return h.requireSetAccessByParam(w, r, "setId", permissionKey)
}

// requireSetAdminByID checks auth, parses the "id" path param, and verifies admin permission.
// Use this for routes where the set ID param is named "id" (e.g. /asset-sets/{id}/roles).
func (h *AssetHandler) requireSetAdminByID(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	return h.requireSetAccessByParam(w, r, "id", services.AssetPermissionKeyAdmin)
}

// requireSetAccessByParam checks auth, parses the given path param as a set ID, and verifies the given permission.
func (h *AssetHandler) requireSetAccessByParam(w http.ResponseWriter, r *http.Request, paramName, permissionKey string) (*models.User, int, bool) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return nil, 0, false
	}
	setID, ok := requireIDParam(w, r, paramName)
	if !ok {
		return nil, 0, false
	}
	hasPerm, err := h.hasAssetPermission(currentUser.ID, setID, permissionKey)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, 0, false
	}
	if !hasPerm {
		respondNotFound(w, r, "asset set")
		return nil, 0, false
	}
	return currentUser, setID, true
}
