package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/repository"
)

// GetAssetRoles returns all available asset roles
func (h *AssetHandler) GetAssetRoles(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	roles, err := h.repo.ListAllRoles()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, roles)
}

// GetAssetRole returns a single asset role with its permissions
func (h *AssetHandler) GetAssetRole(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	roleID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	role, err := h.repo.GetRoleByID(roleID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "Role")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	permissions, err := h.repo.GetRolePermissions(roleID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	role.Permissions = permissions

	respondJSONOK(w, role)
}
