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
	// Create auth middleware
	bearerAuth := middleware.NewBearerAuth(tokenManager)

	// Create rate limiter (1000 requests per minute)
	rateLimiter := middleware.NewRateLimiter(1000)

	// Initialize handlers
	itemHandler := handlers.NewItemHandler(db, permissionService)
	workspaceHandler := handlers.NewWorkspaceHandler(db, permissionService)
	statusHandler := handlers.NewStatusHandler(db)
	workflowHandler := handlers.NewWorkflowHandler(db)
	itemTypeHandler := handlers.NewItemTypeHandler(db)
	priorityHandler := handlers.NewPriorityHandler(db)
	customFieldHandler := handlers.NewCustomFieldHandler(db)
	userHandler := handlers.NewUserHandler(db, permissionService)
	commentHandler := handlers.NewCommentHandler(db, permissionService)
	milestoneHandler := handlers.NewMilestoneHandler(db, permissionService)
	iterationHandler := handlers.NewIterationHandler(db, permissionService)
	projectHandler := handlers.NewProjectHandler(db)

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
	v1.Handle("GET /items", itemHandler.List)
	v1.Handle("POST /items", itemHandler.Create)
	v1.HandleWithMiddleware("GET /items/{id}", itemHandler.Get, router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /items/{id}", itemHandler.Update, router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /items/{id}", itemHandler.Delete, router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/comments", itemHandler.GetComments, router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/comments", itemHandler.CreateComment, router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/history", itemHandler.GetHistory, router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/transitions", itemHandler.GetTransitions, router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/attachments", itemHandler.GetAttachments, router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/children", itemHandler.GetChildren, router.RequireNumericID)

	// ============================================
	// Workspaces
	// ============================================
	v1.Handle("GET /workspaces", workspaceHandler.List)
	v1.Handle("POST /workspaces", workspaceHandler.Create)
	v1.HandleWithMiddleware("GET /workspaces/{id}", workspaceHandler.Get, router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}", workspaceHandler.Update, router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}", workspaceHandler.Delete, router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/items", workspaceHandler.GetItems, router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/statuses", workspaceHandler.GetStatuses, router.RequireNumericID)

	// ============================================
	// Statuses & Status Categories
	// ============================================
	v1.Handle("GET /statuses", statusHandler.List)
	v1.HandleWithMiddleware("GET /statuses/{id}", statusHandler.Get, router.RequireNumericID)
	v1.Handle("GET /status-categories", statusHandler.ListCategories)
	v1.HandleWithMiddleware("GET /status-categories/{id}", statusHandler.GetCategory, router.RequireNumericID)

	// ============================================
	// Workflows
	// ============================================
	v1.Handle("GET /workflows", workflowHandler.List)
	v1.HandleWithMiddleware("GET /workflows/{id}", workflowHandler.Get, router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workflows/{id}/transitions", workflowHandler.GetTransitions, router.RequireNumericID)

	// ============================================
	// Item Types
	// ============================================
	v1.Handle("GET /item-types", itemTypeHandler.List)
	v1.HandleWithMiddleware("GET /item-types/{id}", itemTypeHandler.Get, router.RequireNumericID)

	// ============================================
	// Priorities
	// ============================================
	v1.Handle("GET /priorities", priorityHandler.List)
	v1.HandleWithMiddleware("GET /priorities/{id}", priorityHandler.Get, router.RequireNumericID)

	// ============================================
	// Custom Fields
	// ============================================
	v1.Handle("GET /custom-fields", customFieldHandler.List)
	v1.HandleWithMiddleware("GET /custom-fields/{id}", customFieldHandler.Get, router.RequireNumericID)

	// ============================================
	// Users
	// ============================================
	v1.Handle("GET /users", userHandler.List)
	v1.Handle("GET /users/me", userHandler.GetCurrent)
	v1.HandleWithMiddleware("GET /users/{id}", userHandler.Get, router.RequireNumericID)

	// ============================================
	// Comments (standalone)
	// ============================================
	v1.HandleWithMiddleware("GET /comments/{id}", commentHandler.Get, router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /comments/{id}", commentHandler.Update, router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /comments/{id}", commentHandler.Delete, router.RequireNumericID)

	// ============================================
	// Milestones
	// ============================================
	v1.Handle("GET /milestones", milestoneHandler.List)
	v1.Handle("POST /milestones", milestoneHandler.Create)
	v1.HandleWithMiddleware("GET /milestones/{id}", milestoneHandler.Get, router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /milestones/{id}", milestoneHandler.Update, router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /milestones/{id}", milestoneHandler.Delete, router.RequireNumericID)
	v1.HandleWithMiddleware("GET /milestones/{id}/items", milestoneHandler.GetItems, router.RequireNumericID)

	// ============================================
	// Iterations
	// ============================================
	v1.Handle("GET /iterations", iterationHandler.List)
	v1.Handle("POST /iterations", iterationHandler.Create)
	v1.HandleWithMiddleware("GET /iterations/{id}", iterationHandler.Get, router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /iterations/{id}", iterationHandler.Update, router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /iterations/{id}", iterationHandler.Delete, router.RequireNumericID)

	// ============================================
	// Projects
	// ============================================
	v1.Handle("GET /projects", projectHandler.List)
	v1.Handle("POST /projects", projectHandler.Create)
	v1.HandleWithMiddleware("GET /projects/{id}", projectHandler.Get, router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /projects/{id}", projectHandler.Update, router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /projects/{id}", projectHandler.Delete, router.RequireNumericID)

	// ============================================
	// Search
	// ============================================
	v1.Handle("GET /search/items", itemHandler.Search)
}
