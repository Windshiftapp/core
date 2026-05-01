package routes

import "net/http"

// RegisterAIRoutes registers AI-powered feature routes.
func RegisterAIRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	// AI feature endpoints (user) - rate limited to protect expensive LLM calls
	api.HandleH("GET /ai/status", auth(http.HandlerFunc(deps.AI.AI.Status)))
	api.HandleH("POST /ai/chat", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.Chat))))
	api.HandleH("GET /ai/daily-briefing", auth(http.HandlerFunc(deps.AI.AI.GetDailyBriefing)))
	api.HandleH("GET /ai/plan-my-day", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.PlanMyDay))))
	api.HandleH("POST /ai/items/{id}/catch-me-up", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.CatchMeUp))))
	api.HandleH("POST /ai/items/{id}/find-similar", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.FindSimilarItems))))
	api.HandleH("POST /ai/items/{id}/decompose", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.DecomposeItem))))
	api.HandleH("POST /ai/milestones/{id}/generate-release-notes", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.GenerateReleaseNotes))))
	api.HandleH("POST /ai/iterations/{id}/analyze-dependencies", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.AnalyzeDependencies))))
	api.HandleH("POST /ai/iterations/{id}/accept-dependencies", auth(http.HandlerFunc(deps.AI.AI.AcceptDependencies)))
	api.HandleH("POST /ai/test-sets/{id}/summarize-description", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.SummarizeTestPlanDescription))))

	// LLM provider info (user)
	api.HandleH("GET /llm/providers", auth(http.HandlerFunc(deps.AI.LLMConnection.GetProviders)))
	api.HandleH("GET /llm/connections", auth(http.HandlerFunc(deps.AI.LLMConnection.GetEnabledConnections)))

	// LLM connection management (admin)
	api.HandleH("GET /admin/llm-connections", admin(http.HandlerFunc(deps.AI.LLMConnection.ListConnections)))
	api.HandleH("POST /admin/llm-connections", admin(http.HandlerFunc(deps.AI.LLMConnection.CreateConnection)))
	api.HandleH("GET /admin/llm-connections/{id}", admin(http.HandlerFunc(deps.AI.LLMConnection.GetConnection)))
	api.HandleH("PUT /admin/llm-connections/{id}", admin(http.HandlerFunc(deps.AI.LLMConnection.UpdateConnection)))
	api.HandleH("DELETE /admin/llm-connections/{id}", admin(http.HandlerFunc(deps.AI.LLMConnection.DeleteConnection)))
	api.HandleH("POST /admin/llm-connections/{id}/test", admin(http.HandlerFunc(deps.AI.LLMConnection.TestConnection)))
}
