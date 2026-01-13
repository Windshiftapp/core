package restapi

import (
	"net/http"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/services"
)

// SetupRoutesFunc is a function type for setting up v1 routes
// This breaks the import cycle by allowing main.go to wire the dependency
type SetupRoutesFunc func(mux *http.ServeMux, db database.Database, tokenManager *auth.TokenManager, permissionService *services.PermissionService)

// SetupRoutes registers all REST API routes under /rest/api
// The v1Setup function is called to register v1 routes on the provided mux
func SetupRoutes(
	mux *http.ServeMux,
	db database.Database,
	tokenManager *auth.TokenManager,
	permissionService *services.PermissionService,
	v1Setup SetupRoutesFunc,
) {
	// Register v1 routes (they handle their own prefix /rest/api/v1)
	if v1Setup != nil {
		v1Setup(mux, db, tokenManager, permissionService)
	}

	// Future: v2 routes
	// v2Setup(mux, db, tokenManager, permissionService)
}
