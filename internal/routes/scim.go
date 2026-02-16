package routes

import "net/http"

// RegisterSCIMRoutes registers SCIM 2.0 protocol routes.
func RegisterSCIMRoutes(deps *Deps) {
	api := deps.API
	scim := deps.SCIMGroup
	admin := deps.PermissionMiddleware.RequireSystemAdmin()
	scimAuth := deps.SCIMAuthMiddleware.RequireSCIMAuth
	scimLimit := deps.SCIMRateLimiter.Limit // Rate limiting for SCIM endpoints
	h := deps.SCIM                          // SCIM handlers

	// SCIM token management (admin API)
	api.HandleH("GET /admin/scim-tokens", admin(http.HandlerFunc(h.SCIMToken.ListTokens)))
	api.HandleH("POST /admin/scim-tokens", admin(http.HandlerFunc(h.SCIMToken.CreateToken)))
	api.HandleH("GET /admin/scim-tokens/{id}", admin(http.HandlerFunc(h.SCIMToken.GetToken)))
	api.HandleH("DELETE /admin/scim-tokens/{id}", admin(http.HandlerFunc(h.SCIMToken.RevokeToken)))
	api.HandleH("GET /admin/scim-tokens/count", admin(http.HandlerFunc(h.SCIMToken.GetActiveTokenCount)))

	// SCIM 2.0 routes (separate from /api, uses SCIM token authentication)
	// Service provider endpoints (public per SCIM spec - needed for IdP discovery)
	// Rate limited to prevent discovery abuse
	scim.HandleH("GET /ServiceProviderConfig", scimLimit(http.HandlerFunc(h.SCIM.GetServiceProviderConfig)))
	scim.HandleH("GET /ResourceTypes", scimLimit(http.HandlerFunc(h.SCIM.GetResourceTypes)))
	scim.HandleH("GET /Schemas", scimLimit(http.HandlerFunc(h.SCIM.GetSchemas)))

	// User endpoints (SCIM token auth + rate limiting)
	scim.HandleH("GET /Users", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.ListUsers))))
	scim.HandleH("POST /Users", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.CreateUser))))
	scim.HandleH("GET /Users/{id}", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.GetUser))))
	scim.HandleH("PUT /Users/{id}", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.ReplaceUser))))
	scim.HandleH("PATCH /Users/{id}", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.PatchUser))))
	scim.HandleH("DELETE /Users/{id}", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.DeleteUser))))

	// Group endpoints (SCIM token auth + rate limiting)
	scim.HandleH("GET /Groups", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.ListGroups))))
	scim.HandleH("POST /Groups", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.CreateGroup))))
	scim.HandleH("GET /Groups/{id}", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.GetGroup))))
	scim.HandleH("PUT /Groups/{id}", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.ReplaceGroup))))
	scim.HandleH("PATCH /Groups/{id}", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.PatchGroup))))
	scim.HandleH("DELETE /Groups/{id}", scimLimit(scimAuth(http.HandlerFunc(h.SCIM.DeleteGroup))))
}
