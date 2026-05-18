package restapi

import (
	"net/http"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/services"
)

// Deps carries the dependencies v1 (and future versions) need so we can add
// services without churning every call site. New fields go at the end with
// nil-safe defaults so unrelated callers compile unchanged.
type Deps struct {
	Mux               *http.ServeMux
	DB                database.Database
	TokenManager      *auth.TokenManager
	PermissionService *services.PermissionService
	// ActionService is the optional cache-invalidation hook for the actions
	// surface. v1 falls back to "next periodic refresh" when nil, which is
	// fine for cold-start tooling but worth wiring for production.
	ActionService *services.ActionService
}

// SetupRoutesFunc is a function type for setting up v1 routes
// This breaks the import cycle by allowing main.go to wire the dependency
type SetupRoutesFunc func(deps Deps)

// SetupRoutes registers all REST API routes under /rest/api
// The v1Setup function is called to register v1 routes on the provided mux
func SetupRoutes(deps Deps, v1Setup SetupRoutesFunc) {
	// Register v1 routes (they handle their own prefix /rest/api/v1)
	if v1Setup != nil {
		v1Setup(deps)
	}

	// Future: v2 routes
	// v2Setup(deps)
}
