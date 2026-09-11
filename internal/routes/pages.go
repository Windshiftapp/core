package routes

import "net/http"

// RegisterPageRoutes registers workspace knowledge-pages endpoints. All
// routes are workspace-scoped; permission failures inside the handlers
// return 404 (memory: workspace-resource access checks must not leak
// existence).
func RegisterPageRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth

	if deps.Pages.KnowledgeSearch != nil {
		api.HandleH("GET /workspaces/{workspaceId}/knowledge/search", auth(http.HandlerFunc(deps.Pages.KnowledgeSearch.Search)))
	}

}
