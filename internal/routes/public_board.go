package routes

import "net/http"

// RegisterPublicBoardRoutes registers public (unauthenticated) board routes.
func RegisterPublicBoardRoutes(deps *Deps) {
	api := deps.API

	// Public board view - no auth required
	api.HandleH("GET /public/board/{slug}", http.HandlerFunc(deps.PublicBoard.GetPublicBoard))
	api.HandleH("GET /public/board/{slug}/items/{key}", http.HandlerFunc(deps.PublicBoard.GetPublicBoardItem))
	api.HandleH("GET /public/board/{slug}/attachments/{id}/download", http.HandlerFunc(deps.PublicBoard.DownloadAttachment))
}
