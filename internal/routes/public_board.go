package routes

import "net/http"

// RegisterPublicBoardRoutes registers public (unauthenticated) board routes.
func RegisterPublicBoardRoutes(deps *Deps) {
	api := deps.API

	// Public board view - no auth required
	api.HandleH("GET /public/board/{slug}", http.HandlerFunc(deps.PublicBoard.GetPublicBoard))
}
