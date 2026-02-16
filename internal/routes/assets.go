package routes

import "net/http"

// RegisterAssetRoutes registers asset management routes.
func RegisterAssetRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth

	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	// Asset Sets
	api.HandleH("GET /asset-sets", auth(http.HandlerFunc(deps.Assets.Asset.GetAssetSets)))
	api.HandleH("GET /asset-sets/{id}", auth(http.HandlerFunc(deps.Assets.Asset.GetAssetSet)))
	api.HandleH("POST /admin/asset-sets", admin(http.HandlerFunc(deps.Assets.Asset.CreateAssetSet)))
	api.HandleH("PUT /admin/asset-sets/{id}", admin(http.HandlerFunc(deps.Assets.Asset.UpdateAssetSet)))
	api.HandleH("DELETE /admin/asset-sets/{id}", admin(http.HandlerFunc(deps.Assets.Asset.DeleteAssetSet)))

	// Asset Roles
	api.HandleH("GET /asset-roles", auth(http.HandlerFunc(deps.Assets.Asset.GetAssetRoles)))
	api.HandleH("GET /asset-roles/{id}", auth(http.HandlerFunc(deps.Assets.Asset.GetAssetRole)))

	// Asset Set Role Assignments
	api.HandleH("GET /asset-sets/{id}/roles", auth(http.HandlerFunc(deps.Assets.Asset.GetSetRoles)))
	api.HandleH("POST /asset-sets/{id}/roles", auth(http.HandlerFunc(deps.Assets.Asset.AssignSetRole)))
	api.HandleH("DELETE /asset-sets/{id}/roles/{assignmentId}", auth(http.HandlerFunc(deps.Assets.Asset.RevokeSetRole)))

	// Asset Set Everyone Default Role
	api.HandleH("GET /asset-sets/{id}/everyone-role", auth(http.HandlerFunc(deps.Assets.Asset.GetEveryoneRole)))
	api.HandleH("PUT /asset-sets/{id}/everyone-role", auth(http.HandlerFunc(deps.Assets.Asset.SetEveryoneRole)))

	// Assets
	api.HandleH("GET /asset-sets/{setId}/assets", auth(http.HandlerFunc(deps.Assets.Asset.GetAssets)))
	api.HandleH("POST /asset-sets/{setId}/assets", auth(http.HandlerFunc(deps.Assets.Asset.CreateAsset)))
	api.HandleH("GET /assets/{id}", auth(http.HandlerFunc(deps.Assets.Asset.GetAsset)))
	api.HandleH("PUT /assets/{id}", auth(http.HandlerFunc(deps.Assets.Asset.UpdateAsset)))
	api.HandleH("DELETE /assets/{id}", auth(http.HandlerFunc(deps.Assets.Asset.DeleteAsset)))

	// Asset Links
	api.HandleH("GET /assets/{id}/links", auth(http.HandlerFunc(deps.Assets.Asset.GetAssetLinks)))
	api.HandleH("POST /assets/{id}/links", auth(http.HandlerFunc(deps.Assets.Asset.CreateAssetLink)))

	// Asset Types
	api.HandleH("GET /asset-sets/{setId}/types", auth(http.HandlerFunc(deps.Assets.Type.GetAssetTypes)))
	api.HandleH("POST /asset-sets/{setId}/types", auth(http.HandlerFunc(deps.Assets.Type.CreateAssetType)))
	api.HandleH("GET /asset-types/{id}", auth(http.HandlerFunc(deps.Assets.Type.GetAssetType)))
	api.HandleH("PUT /asset-types/{id}", auth(http.HandlerFunc(deps.Assets.Type.UpdateAssetType)))
	api.HandleH("DELETE /asset-types/{id}", auth(http.HandlerFunc(deps.Assets.Type.DeleteAssetType)))
	api.HandleH("GET /asset-types/{id}/fields", auth(http.HandlerFunc(deps.Assets.Type.GetTypeFields)))
	api.HandleH("PUT /asset-types/{id}/fields", auth(http.HandlerFunc(deps.Assets.Type.UpdateTypeFields)))

	// Asset Categories
	api.HandleH("GET /asset-sets/{setId}/categories", auth(http.HandlerFunc(deps.Assets.Category.GetCategories)))
	api.HandleH("POST /asset-sets/{setId}/categories", auth(http.HandlerFunc(deps.Assets.Category.CreateCategory)))
	api.HandleH("GET /asset-categories/{id}", auth(http.HandlerFunc(deps.Assets.Category.GetCategory)))
	api.HandleH("PUT /asset-categories/{id}", auth(http.HandlerFunc(deps.Assets.Category.UpdateCategory)))
	api.HandleH("DELETE /asset-categories/{id}", auth(http.HandlerFunc(deps.Assets.Category.DeleteCategory)))
	api.HandleH("PUT /asset-categories/{id}/move", auth(http.HandlerFunc(deps.Assets.Category.MoveCategory)))

	// Asset Statuses
	api.HandleH("GET /asset-sets/{setId}/statuses", auth(http.HandlerFunc(deps.Assets.Status.GetAssetStatuses)))
	api.HandleH("POST /asset-sets/{setId}/statuses", auth(http.HandlerFunc(deps.Assets.Status.CreateAssetStatus)))
	api.HandleH("GET /asset-statuses/{id}", auth(http.HandlerFunc(deps.Assets.Status.GetAssetStatus)))
	api.HandleH("PUT /asset-statuses/{id}", auth(http.HandlerFunc(deps.Assets.Status.UpdateAssetStatus)))
	api.HandleH("DELETE /asset-statuses/{id}", auth(http.HandlerFunc(deps.Assets.Status.DeleteAssetStatus)))
}
