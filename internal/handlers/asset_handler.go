package handlers

import (
	"net/http"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// AssetHandler handles asset management operations.
// db is held on the struct because the asset domain spans several files
// (asset_crud_handlers.go, asset_link_handlers.go, asset_import.go, etc.)
// that still run their own SQL; they migrate together with this one when
// the rest of the asset surface is repo-shaped.
type AssetHandler struct {
	db                 database.Database
	repo               *repository.AssetRepository
	permissionService  *services.PermissionService
	attachmentPath     string
	assetActionService *services.AssetActionService
}

// NewAssetHandler creates a new asset handler
func NewAssetHandler(db database.Database, permissionService *services.PermissionService, attachmentPath string) *AssetHandler {
	return &AssetHandler{
		db:                db,
		repo:              repository.NewAssetRepository(db),
		permissionService: permissionService,
		attachmentPath:    attachmentPath,
	}
}

// SetAssetActionService sets the asset action service for emitting automation events
func (h *AssetHandler) SetAssetActionService(s *services.AssetActionService) {
	h.assetActionService = s
}

// Asset permission key constants
const (
	AssetPermissionKeyView   = "asset.view"
	AssetPermissionKeyCreate = "asset.create"
	AssetPermissionKeyEdit   = "asset.edit"
	AssetPermissionKeyDelete = "asset.delete"
	AssetPermissionKeyAdmin  = "asset.admin"
)

// Role name constants
const (
	AssetRoleViewer        = "Viewer"
	AssetRoleEditor        = "Editor"
	AssetRoleAdministrator = "Administrator"
)

// createDefaultStatuses creates default statuses for a new asset set.
func (h *AssetHandler) createDefaultStatuses(setID int) error {
	return h.repo.CreateDefaultStatuses(setID)
}

// getUserSetRole returns the role a user has for an asset set
// Priority: System Admin > Direct User Role > Group Role > Everyone Default
func (h *AssetHandler) getUserSetRole(userID, setID int) (*models.AssetRole, error) {
	isAdmin, err := h.permissionService.HasGlobalPermission(userID, "system.admin")
	if err != nil {
		return nil, err
	}
	if isAdmin {
		return &models.AssetRole{
			ID:   -1, // Virtual admin role
			Name: AssetRoleAdministrator,
		}, nil
	}
	return h.repo.GetUserSetRole(userID, setID)
}

// hasAssetPermission checks if a user has a specific asset permission for a set
func (h *AssetHandler) hasAssetPermission(userID, setID int, permissionKey string) (bool, error) {
	role, err := h.getUserSetRole(userID, setID)
	if err != nil {
		return false, err
	}
	if role == nil {
		return false, nil
	}
	return h.repo.RoleHasPermission(role.ID, permissionKey)
}

// HasAssetSetPermission is the exported form of hasAssetPermission, for use by
// other packages (e.g. the logbook node executor) that need to verify that a
// user is authorized to act on an asset set before performing a write.
func (h *AssetHandler) HasAssetSetPermission(userID, setID int, permissionKey string) (bool, error) {
	return h.hasAssetPermission(userID, setID, permissionKey)
}

// getUserSetRoleName returns the role name (for API responses)
func (h *AssetHandler) getUserSetRoleName(userID, setID int) (string, error) {
	role, err := h.getUserSetRole(userID, setID)
	if err != nil {
		return "", err
	}
	if role == nil {
		return "", nil
	}
	return role.Name, nil
}

// requireSetViewAccess checks auth, parses setId, and verifies view permission.
func (h *AssetHandler) requireSetViewAccess(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	return h.requireSetAccess(w, r, AssetPermissionKeyView)
}

// requireSetEditAccess checks auth, parses setId, and verifies edit permission.
func (h *AssetHandler) requireSetEditAccess(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	return h.requireSetAccess(w, r, AssetPermissionKeyEdit)
}

// requireSetAdminAccess checks auth, parses setId, and verifies admin permission.
func (h *AssetHandler) requireSetAdminAccess(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	return h.requireSetAccess(w, r, AssetPermissionKeyAdmin)
}

// requireSetAccess checks auth, parses setId, and verifies the given permission.
func (h *AssetHandler) requireSetAccess(w http.ResponseWriter, r *http.Request, permissionKey string) (*models.User, int, bool) {
	return h.requireSetAccessByParam(w, r, "setId", permissionKey)
}

// requireSetAdminByID checks auth, parses the "id" path param, and verifies admin permission.
// Use this for routes where the set ID param is named "id" (e.g. /asset-sets/{id}/roles).
func (h *AssetHandler) requireSetAdminByID(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	return h.requireSetAccessByParam(w, r, "id", AssetPermissionKeyAdmin)
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

// canViewSet checks if user can view a set
func (h *AssetHandler) canViewSet(userID, setID int) (bool, error) {
	return h.hasAssetPermission(userID, setID, AssetPermissionKeyView)
}

// canEditSet checks if user can edit assets in a set
func (h *AssetHandler) canEditSet(userID, setID int) (bool, error) {
	return h.hasAssetPermission(userID, setID, AssetPermissionKeyEdit)
}

// canAdminSet checks if user can administer a set
func (h *AssetHandler) canAdminSet(userID, setID int) (bool, error) {
	return h.hasAssetPermission(userID, setID, AssetPermissionKeyAdmin)
}
