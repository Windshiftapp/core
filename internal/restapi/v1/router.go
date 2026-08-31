// Package v1 provides scoped bearer-token REST endpoints.
//
// Only /rest/api/v1 accepts crw_* bearer tokens; /api uses session auth and
// /api/internal uses the sidecar secret. Routes must enforce token scope and
// resource access. System admins never bypass token scopes.
package v1

import (
	coremiddleware "windshift/internal/middleware"
	"windshift/internal/objecttranslation"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/handlers"
	v1middleware "windshift/internal/restapi/v1/middleware"
	"windshift/internal/router"
	"windshift/internal/services"
)

// RegisterRoutes registers all v1 API routes on the given ServeMux
func RegisterRoutes(deps restapi.Deps) {
	mux := deps.Mux
	db := deps.DB
	tokenManager := deps.TokenManager
	permissionService := deps.PermissionService
	objectTranslationService := deps.ObjectTranslationService
	if objectTranslationService == nil {
		objectTranslationService = objecttranslation.NewService(db)
	}

	bearerAuth := v1middleware.NewBearerAuthWithPermissions(tokenManager, permissionService)

	rateLimiter := v1middleware.NewRateLimiter(1000)

	// Reuse the fully wired comment service so v1 comments retain side effects.

	itemHandler := handlers.NewItemHandler(db, permissionService, deps.CommentService, deps.ItemCreationService)
	itemHandler.SetItemUpdateApplicationService(deps.ItemUpdateApplicationService)
	itemHandler.SetItemDeletionApplicationService(deps.ItemDeletionApplicationService)
	workspaceHandler := handlers.NewWorkspaceHandler(db, permissionService)
	statusHandler := handlers.NewStatusHandler(db, permissionService, objectTranslationService)
	workflowHandler := handlers.NewWorkflowHandler(db, permissionService)
	itemTypeHandler := handlers.NewItemTypeHandler(db, permissionService, objectTranslationService)
	priorityHandler := handlers.NewPriorityHandler(db, permissionService, objectTranslationService)
	customFieldHandler := handlers.NewCustomFieldHandler(db, permissionService)
	userHandler := handlers.NewUserHandler(db, permissionService)
	userPreferencesHandler := handlers.NewUserPreferencesHandler(db, permissionService)
	agentRunHandler := handlers.NewAgentRunHandler(db, permissionService)
	commentHandler := handlers.NewCommentHandler(db, permissionService, deps.CommentService)
	milestoneHandler := handlers.NewMilestoneHandler(db, permissionService)
	iterationHandler := handlers.NewIterationHandler(db, permissionService)
	collectionHandler := handlers.NewCollectionHandler(db, permissionService)
	actionHandler := handlers.NewActionHandler(db, permissionService, deps.ActionService)
	attachmentHandler := handlers.NewAttachmentHandler(db, permissionService, deps.AttachmentPath)
	pagePermissionService := services.NewPagePermissionService(db, permissionService)
	pageApplicationService := deps.PageApplicationService
	if pageApplicationService == nil {
		pageApplicationService = services.NewPageApplicationService(
			services.NewPageService(db),
			pagePermissionService,
		)
	}
	pageHandler := handlers.NewPageHandler(db, permissionService)
	pageHandler.SetPageApplicationService(pageApplicationService)
	pageDiagramService := deps.PageDiagramService
	if pageDiagramService == nil {
		pageDiagramService = services.NewPageDiagramService(
			db,
			deps.AttachmentPath,
			pageApplicationService,
			pagePermissionService,
			permissionService,
		)
	}
	pageDiagramHandler := handlers.NewPageDiagramHandler(
		handlers.NewBaseHandler(db, permissionService),
		pageDiagramService,
		pageApplicationService.PageService(),
	)
	pageLabelHandler := handlers.NewPageLabelHandler(db, permissionService)
	agentSkillHandler := handlers.NewAgentSkillHandler(db, permissionService)
	diagramHandler := handlers.NewDiagramHandler(db, permissionService)
	labelHandler := handlers.NewLabelHandler(db, permissionService)
	templateHandler := handlers.NewTemplateHandler(db, permissionService)
	testMgmtHandler := handlers.NewTestManagementHandler(db, permissionService)
	recurrenceHandler := handlers.NewRecurrenceHandler(db, permissionService)

	pageAttachmentUploadHandler := handlers.NewPageAttachmentUploadHandler(
		handlers.NewBaseHandler(db, permissionService),
		services.NewPageAttachmentUploadService(db, deps.AttachmentPath, permissionService, pagePermissionService),
	)
	itemAttachmentHandler := handlers.NewItemAttachmentHandler(
		handlers.NewBaseHandler(db, permissionService),
		services.NewItemAttachmentService(db, deps.AttachmentPath, permissionService),
	)

	timePermService := services.NewTimePermissionService(db, permissionService)
	timeProjectHandler := handlers.NewTimeProjectHandler(handlers.NewBaseHandler(db, permissionService), timePermService)
	timeWorklogHandler := handlers.NewTimeWorklogHandler(handlers.NewBaseHandler(db, permissionService), timePermService)
	timerRepo := repository.NewActiveTimerRepository(db)
	itemRepo := repository.NewItemRepository(db)
	timerService := services.NewTimerService(timerRepo, itemRepo, timePermService, permissionService)
	activeTimerHandler := handlers.NewActiveTimerHandler(handlers.NewBaseHandler(db, permissionService), timerRepo, timerService)

	// Public discovery routes (no bearer auth). Mounted on a sibling group
	// that shares the /rest/api/v1 prefix and rate limiter but skips
	// RequireAuth — the OpenAPI document describes the public surface and
	// has to be fetchable by clients that don't yet have a token.
	publicV1 := router.NewRouteGroup(mux, "/rest/api/v1",
		v1middleware.RequestID,
		coremiddleware.LimitJSONRequestBody(restapi.DefaultJSONRequestBodyLimit),
		rateLimiter.Middleware,
	)
	publicV1.Handle("GET /openapi.json", handlers.OpenAPISpecJSON)
	publicV1.Handle("GET /openapi.yaml", handlers.OpenAPISpecYAML)

	// Create authenticated route group with middleware chain:
	// RequestID -> RequireAuth -> RateLimiter
	v1 := router.NewRouteGroup(mux, "/rest/api/v1",
		v1middleware.RequestID,
		coremiddleware.LimitJSONRequestBody(restapi.DefaultJSONRequestBodyLimit),
		bearerAuth.RequireAuth,
		rateLimiter.Middleware,
	)

	v1.HandleWithMiddleware("GET /items", itemHandler.List, bearerAuth.RequirePermission("items:read"))
	v1.HandleWithMiddleware("GET /items/changes", itemHandler.ListChanges, bearerAuth.RequirePermission("items:read"))
	// Bulk fetch by id set. Literal segment, no RequireNumericID — registered
	// before /items/{id} so it isn't swallowed by the wildcard.
	v1.HandleWithMiddleware("GET /items/batch", itemHandler.GetBatch, bearerAuth.RequirePermission("items:read"))
	v1.HandleWithMiddleware("POST /items", itemHandler.Create, bearerAuth.RequirePermission("items:write"))
	v1.HandleWithMiddleware("GET /items/{id}", itemHandler.Get, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /items/{id}", itemHandler.Update, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /items/{id}", itemHandler.Delete, bearerAuth.RequirePermission("items:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/comments", itemHandler.GetComments, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/comments", itemHandler.CreateComment, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/history", itemHandler.GetHistory, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/agent-runs", agentRunHandler.ListForItem, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/transitions", itemHandler.GetTransitions, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/transition", itemHandler.Transition, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/change-type", itemHandler.ChangeType, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/attachments", itemHandler.GetAttachments, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/attachments", itemAttachmentHandler.Upload, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /attachments/{id}/download", attachmentHandler.Download, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /attachments/{id}/thumbnail", attachmentHandler.Thumbnail, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /attachments/{id}", itemAttachmentHandler.Delete, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/children", itemHandler.GetChildren, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /workspaces", workspaceHandler.List, bearerAuth.RequirePermission("workspaces:read"))
	v1.HandleWithMiddleware("POST /workspaces", workspaceHandler.Create, bearerAuth.RequirePermission("workspaces:write"))
	v1.HandleWithMiddleware("GET /workspace-templates", workspaceHandler.ListTemplates, bearerAuth.RequirePermission("workspaces:read"))
	v1.HandleWithMiddleware("GET /workspaces/{id}", workspaceHandler.Get, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}", workspaceHandler.Update, bearerAuth.RequirePermission("workspaces:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}", workspaceHandler.Delete, bearerAuth.RequirePermission("workspaces:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/items", workspaceHandler.GetItems, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/statuses", workspaceHandler.GetStatuses, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/statuses/completed", workspaceHandler.ListCompletedStatuses, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/item-types", workspaceHandler.GetItemTypes, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/workflows", workspaceHandler.GetWorkflows, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/priorities", workspaceHandler.GetPriorities, bearerAuth.RequirePermission("workspaces:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/assignable-users", userHandler.GetAssignableForWorkspace, bearerAuth.RequirePermission("users:read"), router.RequireNumericID)

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
	v1.HandleWithMiddleware("POST /workspaces/{id}/milestones/reorder", milestoneHandler.ReorderInWorkspace, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/milestones/{milestoneId}", milestoneHandler.GetInWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/milestones/{milestoneId}", milestoneHandler.UpdateInWorkspace, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}/milestones/{milestoneId}", milestoneHandler.DeleteInWorkspace, bearerAuth.RequirePermission("items:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/milestones/{milestoneId}/items", milestoneHandler.GetItemsInWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/milestones/{milestoneId}/progress", milestoneHandler.GetProgressInWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)

	// Workspace-scoped iterations. Same convention as workspace-scoped
	// milestones — gated by items:* token scopes plus in-handler workspace
	// permission checks. Global iterations remain reachable via /iterations
	// for cross-workspace use cases.
	v1.HandleWithMiddleware("GET /workspaces/{id}/iterations", iterationHandler.ListForWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/iterations", iterationHandler.CreateInWorkspace, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/iterations/{iterationId}", iterationHandler.GetInWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/iterations/{iterationId}", iterationHandler.UpdateInWorkspace, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}/iterations/{iterationId}", iterationHandler.DeleteInWorkspace, bearerAuth.RequirePermission("items:delete"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /statuses", statusHandler.List, bearerAuth.RequirePermission("statuses:read"))
	v1.HandleWithMiddleware("GET /statuses/{id}", statusHandler.Get, bearerAuth.RequirePermission("statuses:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /status-categories", statusHandler.ListCategories, bearerAuth.RequirePermission("statuses:read"))
	v1.HandleWithMiddleware("GET /status-categories/{id}", statusHandler.GetCategory, bearerAuth.RequirePermission("statuses:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /workflows", workflowHandler.List, bearerAuth.RequirePermission("workflows:read"))
	v1.HandleWithMiddleware("GET /workflows/{id}", workflowHandler.Get, bearerAuth.RequirePermission("workflows:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workflows/{id}/transitions", workflowHandler.GetTransitions, bearerAuth.RequirePermission("workflows:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /item-types", itemTypeHandler.List, bearerAuth.RequirePermission("item-types:read"))
	v1.HandleWithMiddleware("GET /item-types/{id}", itemTypeHandler.Get, bearerAuth.RequirePermission("item-types:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /priorities", priorityHandler.List, bearerAuth.RequirePermission("priorities:read"))
	v1.HandleWithMiddleware("GET /priorities/{id}", priorityHandler.Get, bearerAuth.RequirePermission("priorities:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /custom-fields", customFieldHandler.List, bearerAuth.RequirePermission("custom-fields:read"))
	v1.HandleWithMiddleware("GET /custom-fields/{id}", customFieldHandler.Get, bearerAuth.RequirePermission("custom-fields:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /users", userHandler.List, bearerAuth.RequirePermission("users:read"))
	v1.HandleWithMiddleware("GET /users/me", userHandler.GetCurrent, bearerAuth.RequirePermission("users:read"))
	v1.HandleWithMiddleware("GET /users/me/tui-preferences", userPreferencesHandler.GetTUI, bearerAuth.RequirePermission("user-preferences:read"))
	v1.HandleWithMiddleware("PUT /users/me/tui-preferences", userPreferencesHandler.UpdateTUI, bearerAuth.RequirePermission("user-preferences:write"))
	v1.HandleWithMiddleware("GET /users/{id}", userHandler.Get, bearerAuth.RequirePermission("users:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /comments/{id}", commentHandler.Get, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /comments/{id}", commentHandler.Update, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /comments/{id}", commentHandler.Delete, bearerAuth.RequirePermission("items:delete"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /milestones", milestoneHandler.List, bearerAuth.RequirePermission("milestones:read"))
	v1.HandleWithMiddleware("POST /milestones", milestoneHandler.Create, bearerAuth.RequirePermission("milestones:write"))
	v1.HandleWithMiddleware("POST /milestones/reorder", milestoneHandler.ReorderGlobal, bearerAuth.RequirePermission("milestones:write"))
	v1.HandleWithMiddleware("GET /milestones/{id}", milestoneHandler.Get, bearerAuth.RequirePermission("milestones:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /milestones/{id}", milestoneHandler.Update, bearerAuth.RequirePermission("milestones:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /milestones/{id}", milestoneHandler.Delete, bearerAuth.RequirePermission("milestones:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /milestones/{id}/items", milestoneHandler.GetItems, bearerAuth.RequirePermission("milestones:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /milestones/{id}/progress", milestoneHandler.GetProgress, bearerAuth.RequirePermission("milestones:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /iterations", iterationHandler.List, bearerAuth.RequirePermission("iterations:read"))
	v1.HandleWithMiddleware("POST /iterations", iterationHandler.Create, bearerAuth.RequirePermission("iterations:write"))
	v1.HandleWithMiddleware("GET /iterations/{id}", iterationHandler.Get, bearerAuth.RequirePermission("iterations:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /iterations/{id}", iterationHandler.Update, bearerAuth.RequirePermission("iterations:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /iterations/{id}", iterationHandler.Delete, bearerAuth.RequirePermission("iterations:delete"), router.RequireNumericID)

	// ============================================
	// Collections — addressable by either numeric id or public_slug.
	// The handler picks the lookup based on whether {key} is all digits.
	// ============================================
	v1.HandleWithMiddleware("GET /collections", collectionHandler.List, bearerAuth.RequirePermission("collections:read"))
	v1.HandleWithMiddleware("GET /collections/{key}", collectionHandler.Get, bearerAuth.RequirePermission("collections:read"))
	v1.HandleWithMiddleware("GET /collections/{key}/items", collectionHandler.GetItems, bearerAuth.RequirePermission("collections:read", "items:read"))

	// ============================================
	// Actions (workspace-scoped automation graphs)
	// ============================================
	v1.HandleWithMiddleware("GET /workspaces/{id}/action-catalog", actionHandler.GetCatalog, bearerAuth.RequirePermission("actions:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/actions", actionHandler.ListActions, bearerAuth.RequirePermission("actions:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/actions", actionHandler.CreateAction, bearerAuth.RequirePermission("actions:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/actions/validate", actionHandler.ValidateAction, bearerAuth.RequirePermission("actions:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/actions/{actionId}", actionHandler.GetAction, bearerAuth.RequirePermission("actions:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/actions/{actionId}", actionHandler.UpdateAction, bearerAuth.RequirePermission("actions:write"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /workspaces/{id}/pages", pageHandler.List, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	// Literal "search" segment; the ServeMux prefers it over the {pageId}
	// wildcard route below, so order is not load-bearing.
	v1.HandleWithMiddleware("GET /workspaces/{id}/pages/search", pageHandler.Search, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/pages", pageHandler.Create, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/pages/{pageId}", pageHandler.Get, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/pages/{pageId}", pageHandler.Update, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}/pages/{pageId}", pageHandler.Archive, bearerAuth.RequirePermission("pages:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/pages/{pageId}/move", pageHandler.Move, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/pages/{pageId}/history", pageHandler.GetHistory, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/pages/{pageId}/history/{revisionId}", pageHandler.GetRevision, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/pages/{pageId}/history/{revisionId}/restore", pageHandler.RestoreRevision, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/pages/{pageId}/permissions", pageHandler.GetPermissions, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/pages/{pageId}/permissions", pageHandler.GrantPermission, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}/pages/{pageId}/permissions/{permissionId}", pageHandler.RevokePermission, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("PATCH /workspaces/{id}/pages/{pageId}/inheritance", pageHandler.SetInheritance, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/pages/{pageId}/diagrams", pageDiagramHandler.List, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/pages/{pageId}/diagrams", pageDiagramHandler.Create, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/pages/{pageId}/diagrams/{attachmentId}", pageDiagramHandler.Get, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/pages/{pageId}/diagrams/{attachmentId}", pageDiagramHandler.Update, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	// Bearer-authenticated page-attachment upload (the cookie-auth
	// /api/attachments/upload route rejects crw_ tokens). Uses the shared
	// upload service so validation/storage/audit stay in one place.
	v1.HandleWithMiddleware("POST /workspaces/{id}/pages/{pageId}/attachments", pageAttachmentUploadHandler.Upload, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /workspaces/{id}/agent-skills", agentSkillHandler.List, bearerAuth.RequirePermission("agent-skills:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/agent-skills/{skillId}", agentSkillHandler.Get, bearerAuth.RequirePermission("agent-skills:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/page-labels", pageLabelHandler.ListLabels, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/page-labels", pageLabelHandler.CreateLabel, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/page-labels/{labelId}", pageLabelHandler.GetLabel, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/page-labels/{labelId}", pageLabelHandler.UpdateLabel, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}/page-labels/{labelId}", pageLabelHandler.DeleteLabel, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /workspaces/{id}/pages/{pageId}/labels", pageLabelHandler.ListForPage, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/pages/{pageId}/labels", pageLabelHandler.SetForPage, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/pages/{pageId}/labels", pageLabelHandler.AddToPage, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}/pages/{pageId}/labels/{labelId}", pageLabelHandler.RemoveFromPage, bearerAuth.RequirePermission("pages:write"), router.RequireNumericID)

	// ============================================
	// Links (item ↔ item / item ↔ page / item ↔ test_case)
	// Shares the fully-wired *services.ItemLinkService that the cookie-
	// auth handler built (asset checker, page checker, notification +
	// action emitters) so both surfaces behave identically. deps.ItemLinkService
	// is nil during early-boot or in tests that don't construct the
	// cookie path; fall back to a bare service so the rest of v1 still
	// boots — link endpoints in that case fail closed (404) because the
	// permission checkers are absent.
	// ============================================
	linkSvc := deps.ItemLinkService
	if linkSvc == nil {
		linkSvc = services.NewItemLinkService(db).WithPermissionService(permissionService)
	}
	linkHandler := handlers.NewLinkHandler(handlers.NewBaseHandler(db, permissionService), linkSvc)

	v1.HandleWithMiddleware("GET /link-types", linkHandler.ListLinkTypes, bearerAuth.RequirePermission("items:read"))
	v1.HandleWithMiddleware("GET /links/batch", linkHandler.GetLinksBatch, bearerAuth.RequirePermission("items:read"))
	v1.HandleWithMiddleware("POST /links", linkHandler.CreateLink, bearerAuth.RequirePermission("items:write"))
	v1.HandleWithMiddleware("DELETE /links/{id}", linkHandler.DeleteLink, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/links", linkHandler.GetLinksForEntity, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /pages/{id}/links", linkHandler.GetLinksForEntity, bearerAuth.RequirePermission("pages:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /test-cases/{id}/links", linkHandler.GetLinksForEntity, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)

	// ============================================
	// Item diagrams (Mermaid / Excalidraw payloads attached to items).
	// Gated by items:* because diagrams are item-scoped content; the
	// handler still applies the workspace view/edit check on the owning
	// item so a token cannot probe diagrams it isn't authorized to see.
	// ============================================
	v1.HandleWithMiddleware("GET /items/{id}/diagrams", diagramHandler.ListForItem, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/diagrams", diagramHandler.CreateForItem, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /diagrams/{id}", diagramHandler.Get, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /diagrams/{id}", diagramHandler.Update, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /diagrams/{id}", diagramHandler.Delete, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)

	// ============================================
	// Item recurrence (RRULE rules + generated instances + RRULE preview).
	// Mirrors the cookie surface at /api/items/{id}/recurrence[*] and
	// /api/recurrence-rules/preview, gated by items:* because recurrence is
	// item-scoped content. The handler still enforces workspace view/edit on
	// the owning item (404 on failure — existence is never leaked). A GET
	// returns JSON null (not 404) when no rule is configured, matching the
	// cookie handler so absence stays a normal state.
	// ============================================
	v1.HandleWithMiddleware("GET /items/{id}/recurrence", recurrenceHandler.GetRecurrence, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/recurrence", recurrenceHandler.CreateRecurrence, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /items/{id}/recurrence", recurrenceHandler.UpdateRecurrence, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /items/{id}/recurrence", recurrenceHandler.DeleteRecurrence, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /items/{id}/recurrence/instances", recurrenceHandler.ListInstances, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/recurrence/generate", recurrenceHandler.ForceGenerate, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /recurrence-rules/preview", recurrenceHandler.PreviewRRule, bearerAuth.RequirePermission("items:read"))

	// ============================================
	// Global item-label catalog + per-item attach/detach. Catalog routes keep
	// the workspace path as their authorization context.
	// ============================================
	v1.HandleWithMiddleware("GET /workspaces/{id}/labels", labelHandler.ListForWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/labels", labelHandler.CreateInWorkspace, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/labels/{labelId}", labelHandler.GetInWorkspace, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/labels/{labelId}", labelHandler.UpdateInWorkspace, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}/labels/{labelId}", labelHandler.DeleteInWorkspace, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)

	// ============================================
	// Work item templates (WI-438): workspace-scoped catalog under
	// /workspaces/{id}/templates. Gated by the dedicated item-templates:*
	// scopes; the handler enforces workspace view/edit on top.
	// ============================================
	v1.HandleWithMiddleware("GET /workspaces/{id}/templates", templateHandler.ListForWorkspace, bearerAuth.RequirePermission("item-templates:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /workspaces/{id}/templates", templateHandler.CreateInWorkspace, bearerAuth.RequirePermission("item-templates:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /workspaces/{id}/templates/{templateId}", templateHandler.GetInWorkspace, bearerAuth.RequirePermission("item-templates:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /workspaces/{id}/templates/{templateId}", templateHandler.UpdateInWorkspace, bearerAuth.RequirePermission("item-templates:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /workspaces/{id}/templates/{templateId}", templateHandler.DeleteInWorkspace, bearerAuth.RequirePermission("item-templates:write"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /items/{id}/labels", labelHandler.ListForItem, bearerAuth.RequirePermission("items:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /items/{id}/labels", labelHandler.SetForItem, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /items/{id}/labels", labelHandler.AddToItem, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /items/{id}/labels/{labelId}", labelHandler.RemoveFromItem, bearerAuth.RequirePermission("items:write"), router.RequireNumericID)

	// ============================================
	// Test management (WI-68 phase 1 + WI-81 phase 2).
	// Gated by tests:* token scope at the route layer; in-handler
	// workspace permission checks enforce test.view / test.manage /
	// test.execute so token scope alone never grants workspace access.
	// ============================================
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-folders", testMgmtHandler.ListTestFolders, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-folders", testMgmtHandler.CreateTestFolder, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-folders/{id}", testMgmtHandler.GetTestFolder, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-folders/{id}", testMgmtHandler.UpdateTestFolder, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-folders/{id}", testMgmtHandler.DeleteTestFolder, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-folders/reorder", testMgmtHandler.ReorderTestFolders, bearerAuth.RequirePermission("tests:write"))

	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-cases", testMgmtHandler.ListTestCases, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-cases/count", testMgmtHandler.GetTestCaseCount, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-cases", testMgmtHandler.CreateTestCase, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-cases/{id}", testMgmtHandler.GetTestCase, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-cases/{id}", testMgmtHandler.UpdateTestCase, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-cases/{id}", testMgmtHandler.DeleteTestCase, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-cases/{id}/move", testMgmtHandler.MoveTestCase, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-cases/reorder", testMgmtHandler.ReorderTestCases, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-cases/{id}/connections", testMgmtHandler.GetTestCaseConnections, bearerAuth.RequirePermission("tests:read"))

	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-cases/{testCaseId}/steps", testMgmtHandler.GetTestCaseSteps, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-cases/{testCaseId}/steps", testMgmtHandler.CreateTestCaseStep, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-cases/{testCaseId}/steps/{stepId}", testMgmtHandler.UpdateTestCaseStep, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-cases/{testCaseId}/steps/{stepId}", testMgmtHandler.DeleteTestCaseStep, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-cases/{testCaseId}/steps/reorder", testMgmtHandler.ReorderTestCaseSteps, bearerAuth.RequirePermission("tests:write"))

	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-labels", testMgmtHandler.ListTestLabels, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-labels", testMgmtHandler.CreateTestLabel, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-labels/{labelId}", testMgmtHandler.UpdateTestLabel, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-labels/{labelId}", testMgmtHandler.DeleteTestLabel, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-cases/{testCaseId}/labels", testMgmtHandler.ListTestCaseLabels, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-cases/{testCaseId}/labels", testMgmtHandler.AddTestCaseLabel, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-cases/{testCaseId}/labels/{labelId}", testMgmtHandler.RemoveTestCaseLabel, bearerAuth.RequirePermission("tests:write"))

	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-sets", testMgmtHandler.ListTestSets, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-sets", testMgmtHandler.CreateTestSet, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-sets/{id}", testMgmtHandler.GetTestSet, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-sets/{id}", testMgmtHandler.UpdateTestSet, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-sets/{id}", testMgmtHandler.DeleteTestSet, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-sets/{id}/test-cases", testMgmtHandler.GetTestSetCases, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-sets/{id}/test-cases", testMgmtHandler.AddTestSetCase, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-sets/{id}/test-cases/{testCaseId}", testMgmtHandler.RemoveTestSetCase, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-sets/{id}/runs", testMgmtHandler.ListTestSetRuns, bearerAuth.RequirePermission("tests:read"))

	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-plans", testMgmtHandler.ListTestPlans, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-plans", testMgmtHandler.CreateTestPlan, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-plans/{id}", testMgmtHandler.GetTestPlan, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-plans/{id}", testMgmtHandler.UpdateTestPlan, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-plans/{id}", testMgmtHandler.DeleteTestPlan, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-plans/{id}/test-cases", testMgmtHandler.ListTestPlanCases, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-plans/{id}/test-cases", testMgmtHandler.AddTestPlanCase, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-plans/{id}/test-cases/{testCaseId}", testMgmtHandler.RemoveTestPlanCase, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-plans/{id}/runs", testMgmtHandler.ListTestPlanRuns, bearerAuth.RequirePermission("tests:read"))

	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-run-templates", testMgmtHandler.ListTestRunTemplates, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-run-templates", testMgmtHandler.CreateTestRunTemplate, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-run-templates/{id}", testMgmtHandler.GetTestRunTemplate, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-run-templates/{id}", testMgmtHandler.UpdateTestRunTemplate, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-run-templates/{id}", testMgmtHandler.DeleteTestRunTemplate, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-run-templates/{id}/executions", testMgmtHandler.ListTestRunTemplateExecutions, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-run-templates/{id}/execute", testMgmtHandler.ExecuteTestRunTemplate, bearerAuth.RequirePermission("tests:write"))

	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-runs", testMgmtHandler.ListTestRuns, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-runs", testMgmtHandler.CreateTestRun, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-runs/{id}", testMgmtHandler.GetTestRun, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-runs/{id}/detail", testMgmtHandler.GetTestRunDetail, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-runs/{id}", testMgmtHandler.UpdateTestRun, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-runs/{id}", testMgmtHandler.DeleteTestRun, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-runs/{id}/end", testMgmtHandler.EndTestRun, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-runs/{id}/results", testMgmtHandler.GetTestRunResults, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-runs/{id}/results/{resultId}", testMgmtHandler.UpdateTestRunResult, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-runs/{id}/steps", testMgmtHandler.GetTestRunStepResults, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("PUT /workspaces/{workspaceId}/test-runs/{id}/steps/{stepId}", testMgmtHandler.UpdateTestRunStepResult, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-runs/{id}/summary", testMgmtHandler.GetTestRunSummary, bearerAuth.RequirePermission("tests:read"))

	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-reports/summary", testMgmtHandler.GetTestReportsSummary, bearerAuth.RequirePermission("tests:read"))
	v1.HandleWithMiddleware("POST /workspaces/{workspaceId}/test-results/{resultId}/items", testMgmtHandler.LinkTestResultItem, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("DELETE /workspaces/{workspaceId}/test-results/{resultId}/items/{itemId}", testMgmtHandler.UnlinkTestResultItem, bearerAuth.RequirePermission("tests:write"))
	v1.HandleWithMiddleware("GET /workspaces/{workspaceId}/test-results/{resultId}/items", testMgmtHandler.ListTestResultItems, bearerAuth.RequirePermission("tests:read"))

	// ============================================
	// Assets. Gated by assets:* at the route layer; the handler still asks
	// the per-set asset role (Viewer / Editor / Administrator with the
	// asset.view/create/edit/delete/admin keys) via AssetPermissionService
	// so a token can't reach a set the user can't see. 404 (not 403) on
	// any permission failure mirrors the items convention — set / asset
	// existence is never leaked.
	//
	// Mutating sets / types / categories / statuses / role assignments and
	// the asset-actions automation graphs stay admin-UI-only in this slice;
	// follow-ups can promote subsets behind explicit asset-sets:write etc.
	// ============================================
	assetRepo := repository.NewAssetRepository(db)
	assetPermSvc := deps.AssetPermissionService
	if assetPermSvc == nil {
		// Nil-safe fallback for embedders that haven't wired the shared
		// service yet — construct a fresh one so asset routes still serve.
		assetPermSvc = services.NewAssetPermissionService(assetRepo, permissionService)
	}
	assetSvc := deps.AssetService
	if assetSvc == nil {
		assetSvc = services.NewAssetService(db, assetRepo)
	}
	assetHandler := handlers.NewAssetHandler(db, permissionService, assetPermSvc, assetSvc)

	v1.HandleWithMiddleware("GET /asset-sets/{setId}/assets", assetHandler.List, bearerAuth.RequirePermission("assets:read"))
	v1.HandleWithMiddleware("POST /asset-sets/{setId}/assets", assetHandler.Create, bearerAuth.RequirePermission("assets:write"))
	v1.HandleWithMiddleware("POST /asset-sets/{setId}/assets/import", assetHandler.ImportCSV, bearerAuth.RequirePermission("assets:write"))
	v1.HandleWithMiddleware("GET /assets/{id}", assetHandler.Get, bearerAuth.RequirePermission("assets:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("PUT /assets/{id}", assetHandler.Update, bearerAuth.RequirePermission("assets:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /assets/{id}", assetHandler.Delete, bearerAuth.RequirePermission("assets:delete"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /asset-sets", assetHandler.ListSets, bearerAuth.RequirePermission("assets:read"))
	v1.HandleWithMiddleware("GET /asset-sets/{setId}", assetHandler.GetSet, bearerAuth.RequirePermission("assets:read"))

	v1.HandleWithMiddleware("GET /asset-sets/{setId}/types", assetHandler.ListTypes, bearerAuth.RequirePermission("assets:read"))
	v1.HandleWithMiddleware("GET /asset-types/{id}", assetHandler.GetType, bearerAuth.RequirePermission("assets:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /asset-sets/{setId}/categories", assetHandler.ListCategories, bearerAuth.RequirePermission("assets:read"))
	v1.HandleWithMiddleware("GET /asset-categories/{id}", assetHandler.GetCategory, bearerAuth.RequirePermission("assets:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /asset-sets/{setId}/statuses", assetHandler.ListStatuses, bearerAuth.RequirePermission("assets:read"))
	v1.HandleWithMiddleware("GET /asset-statuses/{id}", assetHandler.GetStatus, bearerAuth.RequirePermission("assets:read"), router.RequireNumericID)

	v1.HandleWithMiddleware("GET /search/items", itemHandler.Search, bearerAuth.RequirePermission("items:read"))

	v1.HandleWithMiddleware("GET /time/projects", timeProjectHandler.List, bearerAuth.RequirePermission("time:read"))
	v1.HandleWithMiddleware("GET /time/projects/{id}", timeProjectHandler.Get, bearerAuth.RequirePermission("time:read"), router.RequireNumericID)
	v1.HandleWithMiddleware("GET /time/worklogs", timeWorklogHandler.ListMine, bearerAuth.RequirePermission("time:read"))
	v1.HandleWithMiddleware("POST /time/worklogs", timeWorklogHandler.Create, bearerAuth.RequirePermission("time:write"))
	v1.HandleWithMiddleware("PUT /time/worklogs/{id}", timeWorklogHandler.Update, bearerAuth.RequirePermission("time:write"), router.RequireNumericID)
	v1.HandleWithMiddleware("DELETE /time/worklogs/{id}", timeWorklogHandler.Delete, bearerAuth.RequirePermission("time:delete"), router.RequireNumericID)
	v1.HandleWithMiddleware("POST /timer/start", activeTimerHandler.StartTimer, bearerAuth.RequirePermission("time:write"))
	v1.HandleWithMiddleware("GET /timer/active", activeTimerHandler.GetActiveTimer, bearerAuth.RequirePermission("time:read"))
	v1.HandleWithMiddleware("DELETE /timer/stop", activeTimerHandler.StopTimer, bearerAuth.RequirePermission("time:write"))

	adminUserHandler := handlers.NewAdminUserHandler(db, permissionService)
	adminGroupHandler := handlers.NewAdminGroupHandler(db, permissionService)
	adminAuditLogHandler := handlers.NewAdminAuditLogHandler(db, permissionService)
	adminAPITokenHandler := handlers.NewAdminAPITokenHandler(db, tokenManager, permissionService)
	objectTranslationHandler := handlers.NewObjectTranslationHandler(
		handlers.NewBaseHandler(db, permissionService), objectTranslationService,
	)

	adminV1 := v1.Group("", bearerAuth.RequireSystemAdmin)

	adminV1.HandleWithMiddleware("GET /admin/users", adminUserHandler.List, bearerAuth.RequirePermission("admin:users:read"))
	adminV1.HandleWithMiddleware("PUT /admin/users/{id}", adminUserHandler.Update, bearerAuth.RequirePermission("admin:users:write"), router.RequireNumericID)

	adminV1.HandleWithMiddleware("GET /admin/groups", adminGroupHandler.List, bearerAuth.RequirePermission("admin:groups:read"))
	adminV1.HandleWithMiddleware("POST /admin/groups", adminGroupHandler.Create, bearerAuth.RequirePermission("admin:groups:write"))
	adminV1.HandleWithMiddleware("PUT /admin/groups/{id}", adminGroupHandler.Update, bearerAuth.RequirePermission("admin:groups:write"), router.RequireNumericID)
	adminV1.HandleWithMiddleware("DELETE /admin/groups/{id}", adminGroupHandler.Delete, bearerAuth.RequirePermission("admin:groups:write"), router.RequireNumericID)

	adminV1.HandleWithMiddleware("GET /admin/audit-logs", adminAuditLogHandler.List, bearerAuth.RequirePermission("admin:audit-logs:read"))
	adminV1.HandleWithMiddleware("GET /admin/audit-logs/since", adminAuditLogHandler.ListSince, bearerAuth.RequirePermission("admin:audit-logs:read"))

	adminV1.HandleWithMiddleware("GET /admin/api-tokens", adminAPITokenHandler.ListAll, bearerAuth.RequirePermission("admin:api-tokens:read"))
	adminV1.HandleWithMiddleware("DELETE /admin/api-tokens/{id}", adminAPITokenHandler.Revoke, bearerAuth.RequirePermission("admin:api-tokens:write"), router.RequireNumericID)

	adminV1.HandleWithMiddleware("GET /admin/object-translations/definitions", objectTranslationHandler.ListDefinitions, bearerAuth.RequirePermission("admin:object-translations:read"))
	adminV1.HandleWithMiddleware("POST /admin/object-translations/resolve", objectTranslationHandler.Resolve, bearerAuth.RequirePermission("admin:object-translations:read"))
	adminV1.HandleWithMiddleware("GET /admin/object-translations/orphans", objectTranslationHandler.FindOrphans, bearerAuth.RequirePermission("admin:object-translations:read"))
	adminV1.HandleWithMiddleware("GET /admin/object-translations/canonical-differences", objectTranslationHandler.FindCanonicalDifferences, bearerAuth.RequirePermission("admin:object-translations:read"))
	adminV1.HandleWithMiddleware("GET /admin/object-translations/{objectType}/{objectId}", objectTranslationHandler.List, bearerAuth.RequirePermission("admin:object-translations:read"))
	adminV1.HandleWithMiddleware("PUT /admin/object-translations/{objectType}/{objectId}/{field}/{locale}", objectTranslationHandler.Upsert, bearerAuth.RequirePermission("admin:object-translations:write"))
	adminV1.HandleWithMiddleware("DELETE /admin/object-translations/{objectType}/{objectId}/{field}/{locale}", objectTranslationHandler.Delete, bearerAuth.RequirePermission("admin:object-translations:write"))
}
