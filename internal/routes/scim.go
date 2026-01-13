package routes

import "net/http"

// RegisterSCIMRoutes registers SCIM 2.0 protocol routes.
func RegisterSCIMRoutes(deps *Deps) {
	api := deps.API
	scim := deps.SCIMGroup
	admin := deps.PermissionMiddleware.RequireSystemAdmin()
	scimAuth := deps.SCIMAuthMiddleware.RequireSCIMAuth
	h := deps.SCIM // SCIM handlers

	// SCIM token management (admin API)
	api.HandleH("GET /scim-tokens", admin(http.HandlerFunc(h.SCIMToken.ListTokens)))
	api.HandleH("POST /scim-tokens", admin(http.HandlerFunc(h.SCIMToken.CreateToken)))
	api.HandleH("GET /scim-tokens/{id}", admin(http.HandlerFunc(h.SCIMToken.GetToken)))
	api.HandleH("DELETE /scim-tokens/{id}", admin(http.HandlerFunc(h.SCIMToken.RevokeToken)))
	api.HandleH("GET /scim-tokens/count", admin(http.HandlerFunc(h.SCIMToken.GetActiveTokenCount)))

	// SCIM 2.0 routes (separate from /api, uses SCIM token authentication)
	// Service provider endpoints (public per SCIM spec - needed for IdP discovery)
	scim.Handle("GET /ServiceProviderConfig", h.SCIM.GetServiceProviderConfig)
	scim.Handle("GET /ResourceTypes", h.SCIM.GetResourceTypes)
	scim.Handle("GET /Schemas", h.SCIM.GetSchemas)

	// User endpoints (SCIM token auth required)
	scim.HandleH("GET /Users", scimAuth(http.HandlerFunc(h.SCIM.ListUsers)))
	scim.HandleH("POST /Users", scimAuth(http.HandlerFunc(h.SCIM.CreateUser)))
	scim.HandleH("GET /Users/{id}", scimAuth(http.HandlerFunc(h.SCIM.GetUser)))
	scim.HandleH("PUT /Users/{id}", scimAuth(http.HandlerFunc(h.SCIM.ReplaceUser)))
	scim.HandleH("PATCH /Users/{id}", scimAuth(http.HandlerFunc(h.SCIM.PatchUser)))
	scim.HandleH("DELETE /Users/{id}", scimAuth(http.HandlerFunc(h.SCIM.DeleteUser)))

	// Group endpoints (SCIM token auth required)
	scim.HandleH("GET /Groups", scimAuth(http.HandlerFunc(h.SCIM.ListGroups)))
	scim.HandleH("POST /Groups", scimAuth(http.HandlerFunc(h.SCIM.CreateGroup)))
	scim.HandleH("GET /Groups/{id}", scimAuth(http.HandlerFunc(h.SCIM.GetGroup)))
	scim.HandleH("PUT /Groups/{id}", scimAuth(http.HandlerFunc(h.SCIM.ReplaceGroup)))
	scim.HandleH("PATCH /Groups/{id}", scimAuth(http.HandlerFunc(h.SCIM.PatchGroup)))
	scim.HandleH("DELETE /Groups/{id}", scimAuth(http.HandlerFunc(h.SCIM.DeleteGroup)))
}
