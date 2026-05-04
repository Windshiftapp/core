// Package v1 provides REST API version 1 endpoints and routing.
package v1

import (
	"net/http"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/restapi/v1/handlers"
	"windshift/internal/restapi/v1/middleware"
	"windshift/internal/router"
	"windshift/internal/services"
)

// RegisterRoutes registers all v1 API routes on the given ServeMux
func RegisterRoutes(
	mux *http.ServeMux,
	db database.Database,
	tokenManager *auth.TokenManager,
	permissionService *services.PermissionService,
) {
	// Create auth middleware (with permission service for admin checks)
	bearerAuth := middleware.NewBearerAuthWithPermissions(tokenManager, permissionService)

	// Create rate limiter (1000 requests per minute)
	rateLimiter := middleware.NewRateLimiter(1000)

	// Initialize handlers
	itemHandler := handlers.NewItemHandler(db, permissionService)
	workspaceHandler := handlers.NewWorkspaceHandler(db, permissionService)
	statusHandler := handlers.NewStatusHandler(db, permissionService)
	workflowHandler := handlers.NewWorkflowHandler(db, permissionService)
	itemTypeHandler := handlers.NewItemTypeHandler(db, permissionService)
	priorityHandler := handlers.NewPriorityHandler(db, permissionService)
	customFieldHandler := handlers.NewCustomFieldHandler(db, permissionService)
	userHandler := handlers.NewUserHandler(db, permissionService)
	commentHandler := handlers.NewCommentHandler(db, permissionService)
	milestoneHandler := handlers.NewMilestoneHandler(db, permissionService)
	iterationHandler := handlers.NewIterationHandler(db, permissionService)
	projectHandler := handlers.NewProjectHandler(db, permissionService)
	collectionHandler := handlers.NewCollectionHandler(db, permissionService)

	// Create authenticated route group with middleware chain:
	// RequestID -> RequireAuth -> RateLimiter
	v1 := router.NewRouteGroup(mux, "/rest/api/v1",
		middleware.RequestID,
		bearerAuth.RequireAuth,
		rateLimiter.Middleware,
	)

	// ============================================
	// Items
	// ============================================
	v1.HandleWithMiddleware("GET /items", itemHandler.List, bearerAuth.RequirePermission("items:read"))
	v1.HandleWithMiddleware("POST /items", itemHandler.Create, bearerAuth.RequirePermission("items:write"))
	v1.HandleWithMiddleware("GET /items/{id}", itemHandler.Get, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /items/{id}", itemHandler.Update, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /items/{id}", itemHandler.Delete, bearerAuth.RequirePermission("items:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/comments", itemHandler.GetComments, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/comments", itemHandler.CreateComment, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/history", itemHandler.GetHistory, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/transitions", itemHandler.GetTransitions, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/transition", itemHandler.Transition, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/attachments", itemHandler.GetAttachments, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/children", itemHandler.GetChildren, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)

	// ============================================
	// Workspaces
	// ============================================
	v1.HandleWithMiddleware("GET /workspaces", workspaceHandler.List, bearerAuth.RequirePermission("workspaces:read"))
	v1.HandleWithMiddleware("POST /workspaces", workspaceHandler.Create, bearerAuth.RequirePermission("workspaces:write"))
	v1.HandleWithMiddleware("GET /workspaces/{id}", workspaceHandler.Get, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}", workspaceHandler.Update, bearerAuth.RequirePermission("workspaces:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}", workspaceHandler.Delete, bearerAuth.RequirePermission("workspaces:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/items", workspaceHandler.GetItems, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/statuses", workspaceHandler.GetStatuses, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/statuses/completed", workspaceHandler.ListCompletedStatuses, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/item-types", workspaceHandler.GetItemTypes, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)

	// Item lookup by stable (workspace_key, item_number) pair — for embed clients
	// (e.g. docmost) that store stable references rather than volatile numeric ids.
	v1.HandleWithMiddleware("GET /workspaces/{ws_key}/items/{number}", itemHandler.GetByKeyAndNumber, bearerAuth.RequirePermission("items:read"))

	// Workspace-scoped milestones. These mirror the global /milestones surface
	// but constrain every request to milestones owned by the workspace in the
	// URL. They are gated by items:* token scopes (matching the convention used
	// by /workspaces/{id}/items) rather than the global milestones:* scopes,
	// because a token authorized to read or edit items in a workspace should be
	// able to read or edit that workspace's milestones too — milestones here
	// are workspace content, not a global resource.
	v1.HandleWithMiddleware("GET /workspaces/{id}/milestones", milestoneHandler.ListForWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/milestones", milestoneHandler.CreateInWorkspace, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/milestones/{milestoneId}", milestoneHandler.GetInWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/milestones/{milestoneId}", milestoneHandler.UpdateInWorkspace, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}/milestones/{milestoneId}", milestoneHandler.DeleteInWorkspace, bearerAuth.RequirePermission("items:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/milestones/{milestoneId}/items", milestoneHandler.GetItemsInWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/milestones/{milestoneId}/progress", milestoneHandler.GetProgressInWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)

	// ============================================
	// Statuses & Status Categories
	// ============================================
	v1.HandleWithMiddleware("GET /statuses", statusHandler.List, bearerAuth.RequirePermission("statuses:read"))
	v1.HandleWithMiddleware("GET /statuses/{id}", statusHandler.Get, bearerAuth.RequirePermission("statuses:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /status-categories", statusHandler.ListCategories, bearerAuth.RequirePermission("statuses:read"))
	v1.HandleWithMiddleware("GET /status-categories/{id}", statusHandler.GetCategory, bearerAuth.RequirePermission("statuses:read"), router.RequireNumericID)

	// ============================================
	// Workflows
	// ============================================
	v1.HandleWithMiddleware("GET /workflows", workflowHandler.List, bearerAuth.RequirePermission("workflows:read"))
	v1.HandleWithMiddleware("GET /workflows/{id}", workflowHandler.Get, bearerAuth.RequirePermission("workflows:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workflows/{id}/transitions", workflowHandler.GetTransitions, bearerAuth.RequirePermission("workflows:read"), router.RequireNumericID)

	// ============================================
	// Item Types
	// ============================================
	v1.HandleWithMiddleware("GET /item-types", itemTypeHandler.List, bearerAuth.RequirePermission("item-types:read"))
	v1.HandleWithMiddleware("GET /item-types/{id}", itemTypeHandler.Get, bearerAuth.RequirePermission("item-types:read"), router.RequireNumericID)

	// ============================================
	// Priorities
	// ============================================
	v1.HandleWithMiddleware("GET /priorities", priorityHandler.List, bearerAuth.RequirePermission("priorities:read"))
	v1.HandleWithMiddleware("GET /priorities/{id}", priorityHandler.Get, bearerAuth.RequirePermission("priorities:read"), router.RequireNumericID)

	// ============================================
	// Custom Fields
	// ============================================
	v1.HandleWithMiddleware("GET /custom-fields", customFieldHandler.List, bearerAuth.RequirePermission("custom-fields:read"))
	v1.HandleWithMiddleware("GET /custom-fields/{id}", customFieldHandler.Get, bearerAuth.RequirePermission("custom-fields:read"), router.RequireNumericID)

	// ============================================
	// Users
	// ============================================
	v1.HandleWithMiddleware("GET /users", userHandler.List, bearerAuth.RequirePermission("users:read"))
	v1.HandleWithMiddleware("GET /users/me", userHandler.GetCurrent, bearerAuth.RequirePermission("users:read"))
	v1.HandleWithMiddleware("GET /users/{id}", userHandler.Get, bearerAuth.RequirePermission("users:read"), router.RequireNumericID)

	// ============================================
	// Comments (standalone)
	// ============================================
	v1.HandleWithMiddleware("GET /comments/{id}", commentHandler.Get, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /comments/{id}", commentHandler.Update, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /comments/{id}", commentHandler.Delete, bearerAuth.RequirePermission("items:delete"), router.RequireNumericID)

	// ============================================
	// Milestones
	// ============================================
	v1.HandleWithMiddleware("GET /milestones", milestoneHandler.List, bearerAuth.RequirePermission("milestones:read"))
	v1.HandleWithMiddleware("POST /milestones", milestoneHandler.Create, bearerAuth.RequirePermission("milestones:write"))
	v1.HandleWithMiddleware("GET /milestones/{id}", milestoneHandler.Get, bearerAuth.RequirePermission("milestones:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /milestones/{id}", milestoneHandler.Update, bearerAuth.RequirePermission("milestones:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /milestones/{id}", milestoneHandler.Delete, bearerAuth.RequirePermission("milestones:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /milestones/{id}/items", milestoneHandler.GetItems, bearerAuth.RequirePermission("milestones:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /milestones/{id}/progress", milestoneHandler.GetProgress, bearerAuth.RequirePermission("milestones:read"), router.RequireNumericID)

	// ============================================
	// Iterations
	// ============================================
	v1.HandleWithMiddleware("GET /iterations", iterationHandler.List, bearerAuth.RequirePermission("iterations:read"))
	v1.HandleWithMiddleware("POST /iterations", iterationHandler.Create, bearerAuth.RequirePermission("iterations:write"))
	v1.HandleWithMiddleware("GET /iterations/{id}", iterationHandler.Get, bearerAuth.RequirePermission("iterations:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /iterations/{id}", iterationHandler.Update, bearerAuth.RequirePermission("iterations:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /iterations/{id}", iterationHandler.Delete, bearerAuth.RequirePermission("iterations:delete"), router.RequireNumericID)

	// ============================================
	// Projects
	// ============================================
	v1.HandleWithMiddleware("GET /projects", projectHandler.List, bearerAuth.RequirePermission("projects:read"))
	v1.HandleWithMiddleware("POST /projects", projectHandler.Create, bearerAuth.RequirePermission("projects:write"))
	v1.HandleWithMiddleware("GET /projects/{id}", projectHandler.Get, bearerAuth.RequirePermission("projects:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /projects/{id}", projectHandler.Update, bearerAuth.RequirePermission("projects:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /projects/{id}", projectHandler.Delete, bearerAuth.RequirePermission("projects:delete"), router.RequireNumericID)

	// ============================================
	// Collections (slug-addressed; for embed clients)
	// ============================================
	v1.HandleWithMiddleware("GET /collections/{slug}", collectionHandler.Get, bearerAuth.RequirePermission("collections:read"))
	v1.HandleWithMiddleware("GET /collections/{slug}/items", collectionHandler.GetItems, bearerAuth.RequirePermission("collections:read", "items:read"))

	// ============================================
	// Search
	// ============================================
	v1.HandleWithMiddleware("GET /search/items", itemHandler.Search, bearerAuth.RequirePermission("items:read"))

	// ============================================
	// Admin endpoints (require system admin + scope)
	// ============================================
	adminUserHandler := handlers.NewAdminUserHandler(db, permissionService)
	adminGroupHandler := handlers.NewAdminGroupHandler(db, permissionService)
	adminAuditLogHandler := handlers.NewAdminAuditLogHandler(db, permissionService)
	adminAPITokenHandler := handlers.NewAdminAPITokenHandler(db, tokenManager, permissionService)

	// Admin sub-group: inherits auth + rate limit, adds RequireSystemAdmin
	adminV1 := v1.Group("", bearerAuth.RequireSystemAdmin)

	// Admin: Users
	adminV1.HandleWithMiddleware("GET /admin/users", adminUserHandler.List, bearerAuth.RequirePermission("admin:users:read"))
	adminV1.HandleWithMiddleware("PUT /admin/users/{id}", adminUserHandler.Update, bearerAuth.RequirePermission("admin:users:write"), router.RequireNumericID)

	// Admin: Groups
	adminV1.HandleWithMiddleware("GET /admin/groups", adminGroupHandler.List, bearerAuth.RequirePermission("admin:groups:read"))
	adminV1.HandleWithMiddleware("POST /admin/groups", adminGroupHandler.Create, bearerAuth.RequirePermission("admin:groups:write"))
	adminV1.HandleWithMiddleware("PUT /admin/groups/{id}", adminGroupHandler.Update, bearerAuth.RequirePermission("admin:groups:write"), router.RequireNumericID)
	adminV1.HandleWithMiddleware("DELETE /admin/groups/{id}", adminGroupHandler.Delete, bearerAuth.RequirePermission("admin:groups:write"), router.RequireNumericID)

	// Admin: Audit Logs
	adminV1.HandleWithMiddleware("GET /admin/audit-logs", adminAuditLogHandler.List, bearerAuth.RequirePermission("admin:audit-logs:read"))

	// Admin: API Tokens
	adminV1.HandleWithMiddleware("GET /admin/api-tokens", adminAPITokenHandler.ListAll, bearerAuth.RequirePermission("admin:api-tokens:read"))
	adminV1.HandleWithMiddleware("DELETE /admin/api-tokens/{id}", adminAPITokenHandler.Revoke, bearerAuth.RequirePermission("admin:api-tokens:write"), router.RequireNumericID)
}
