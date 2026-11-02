package routes

import (
	"net/http"

	"windshift/internal/handlers"
)

// RegisterAIRoutes registers AI-powered feature routes.
func RegisterAIRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	admin := deps.PermissionMiddleware.RequireSystemAdmin()

	api.HandleH("GET /agent-studio/openapi.yaml", auth(http.HandlerFunc(handlers.AgentStudioOpenAPIYAML)))

	// AI feature endpoints (user) - rate limited to protect expensive LLM calls
	api.HandleH("GET /ai/status", auth(http.HandlerFunc(deps.AI.AI.Status)))
	api.HandleH("POST /ai/chat", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.Chat))))
	api.HandleH("GET /ai/sessions", auth(http.HandlerFunc(deps.AI.AI.ListAgentSessions)))
	api.HandleH("GET /ai/sessions/general", auth(http.HandlerFunc(deps.AI.AI.GetGeneralSession)))
	api.HandleH("GET /ai/sessions/{id}/messages", auth(http.HandlerFunc(deps.AI.AI.ListAgentMessages)))
	api.HandleH("POST /ai/sessions/{id}/archive", auth(http.HandlerFunc(deps.AI.AI.ArchiveAgentSession)))
	api.HandleH("GET /workspaces/{workspaceId}/available-standard-agents", auth(http.HandlerFunc(deps.AI.AI.ListAvailableStandardAgents)))
	api.HandleH("POST /workspaces/{workspaceId}/agent-sessions", auth(http.HandlerFunc(deps.AI.AI.CreateStandardSession)))
	api.HandleH("GET /ai/daily-briefing", auth(http.HandlerFunc(deps.AI.AI.GetDailyBriefing)))
	api.HandleH("GET /ai/plan-my-day", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.PlanMyDay))))
	api.HandleH("POST /ai/items/{id}/catch-me-up", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.CatchMeUp))))
	api.HandleH("POST /ai/items/{id}/find-similar", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.FindSimilarItems))))
	api.HandleH("POST /ai/items/{id}/decompose", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.DecomposeItem))))
	api.HandleH("POST /ai/milestones/{id}/generate-release-notes", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.GenerateReleaseNotes))))
	api.HandleH("POST /ai/iterations/{id}/analyze-dependencies", auth(deps.AIRateLimiter.Limit(http.HandlerFunc(deps.AI.AI.AnalyzeDependencies))))
	api.HandleH("POST /ai/iterations/{id}/accept-dependencies", auth(http.HandlerFunc(deps.AI.AI.AcceptDependencies)))

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
	api.HandleH("POST /admin/llm/providers/{type}/refresh-models", admin(http.HandlerFunc(deps.AI.LLMConnection.RefreshProviderModels)))
	api.HandleH("GET /admin/work-item-staleness", admin(http.HandlerFunc(deps.AI.WorkItemStaleness.Get)))
	api.HandleH("PUT /admin/work-item-staleness", admin(http.HandlerFunc(deps.AI.WorkItemStaleness.Update)))
}
