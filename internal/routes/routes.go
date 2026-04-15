package routes

// RegisterAll registers all API routes using the provided dependencies.
// This is the single entry point called from main.go.
func RegisterAll(deps *Deps) {
	RegisterAuthRoutes(deps)
	RegisterSCIMRoutes(deps)
	RegisterSCMRoutes(deps)
	RegisterItemRoutes(deps)
	RegisterWorkspaceRoutes(deps)
	RegisterUserRoutes(deps)
	RegisterAdminRoutes(deps)
	RegisterPlanningRoutes(deps)
	RegisterTimeTrackingRoutes(deps)
	RegisterTestManagementRoutes(deps)
	RegisterChannelRoutes(deps)
	RegisterPortalRoutes(deps)
	RegisterFormRoutes(deps)
	RegisterAssetRoutes(deps)
	RegisterCollectionRoutes(deps)
	RegisterAIRoutes(deps)
	RegisterAnalyticsRoutes(deps)
	RegisterMiscRoutes(deps)
	RegisterTeamRoutes(deps)
	RegisterPublicBoardRoutes(deps)
}
