package routes

import "net/http"

// RegisterFormRoutes registers form channel public routes.
func RegisterFormRoutes(deps *Deps) {
	api := deps.API

	if deps.Portal.Form == nil {
		return
	}

	// Public form endpoints (no auth required)
	api.Handle("GET /forms/{slug}", deps.Portal.Form.GetFormChannel)
	api.Handle("GET /forms/{slug}/forms", deps.Portal.Form.GetForms)
	api.Handle("GET /forms/{slug}/forms/{id}/fields", deps.Portal.Form.GetFormFields)
	api.Handle("GET /forms/{slug}/custom-fields", deps.Portal.Form.GetCustomFields)

	// Form submission (rate-limited, auth handled inside handler based on per-form config).
	// OptionalPortalAuth populates session context when present so SubmitForm can
	// enforce RequireAuth forms and attribute submissions to an internal or portal user.
	submit := http.HandlerFunc(deps.Portal.Form.SubmitForm)
	rateLimited := deps.PortalSubmitLimiter.Limit(submit)
	if deps.PortalAuthMiddleware != nil {
		api.HandleH("POST /forms/{slug}/submit", deps.PortalAuthMiddleware.OptionalPortalAuth(rateLimited))
	} else {
		api.HandleH("POST /forms/{slug}/submit", rateLimited)
	}

	// Authenticated endpoint for updating per-form config (requires internal auth)
	auth := deps.AuthMiddleware.RequireAuth
	api.HandleH("PUT /request-types/{id}/config", auth(http.HandlerFunc(deps.Portal.Form.UpdateRequestTypeConfig)))
}
