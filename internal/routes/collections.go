package routes

import "net/http"

// RegisterCollectionRoutes registers collection-related routes.
func RegisterCollectionRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth

	// Collection Category endpoints
	api.HandleH("GET /collection-categories", auth(http.HandlerFunc(deps.Collections.Category.GetAll)))
	api.HandleH("POST /collection-categories", auth(http.HandlerFunc(deps.Collections.Category.Create)))
	api.HandleH("GET /collection-categories/{id}", auth(http.HandlerFunc(deps.Collections.Category.Get)))
	api.HandleH("PUT /collection-categories/{id}", auth(http.HandlerFunc(deps.Collections.Category.Update)))
	api.HandleH("DELETE /collection-categories/{id}", auth(http.HandlerFunc(deps.Collections.Category.Delete)))

	// Collection endpoints
	api.HandleH("GET /collections", auth(http.HandlerFunc(deps.Collections.Collection.GetAll)))
	api.HandleH("POST /collections", auth(http.HandlerFunc(deps.Collections.Collection.Create)))
	api.HandleH("GET /collections/{id}", auth(http.HandlerFunc(deps.Collections.Collection.Get)))
	api.HandleH("PUT /collections/{id}", auth(http.HandlerFunc(deps.Collections.Collection.Update)))
	api.HandleH("DELETE /collections/{id}", auth(http.HandlerFunc(deps.Collections.Collection.Delete)))

	// Board configuration endpoints
	api.HandleH("GET /collections/{id}/board-configuration", auth(http.HandlerFunc(deps.Collections.BoardConfig.GetByCollection)))
	api.HandleH("POST /collections/{id}/board-configuration", auth(http.HandlerFunc(deps.Collections.BoardConfig.CreateForCollection)))
	api.HandleH("PUT /collections/{collectionId}/board-configuration/{configId}", auth(http.HandlerFunc(deps.Collections.BoardConfig.UpdateForCollection)))
	api.HandleH("DELETE /collections/{collectionId}/board-configuration/{configId}", auth(http.HandlerFunc(deps.Collections.BoardConfig.DeleteForCollection)))
}
