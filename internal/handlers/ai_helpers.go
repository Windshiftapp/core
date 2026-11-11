package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/llm"
	"windshift/internal/redact"
	"windshift/internal/restapi"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// parseConnectionIDParam extracts connection_id from the query string.
// Returns 0 if absent or unparseable (zero triggers default resolution).
func parseConnectionIDParam(r *http.Request) int {
	var connectionID int
	if cidStr := r.URL.Query().Get("connection_id"); cidStr != "" {
		fmt.Sscan(cidStr, &connectionID) //nolint:errcheck,gosec // best-effort parse, zero-value fallback is fine
	}
	return connectionID
}

// requireLLMClientForFeature resolves an LLM client respecting per-feature admin
// llmFeatureResolver resolves an LLM client for one AI feature. Implemented by
// *llm.ConnectionManager; the narrow interface keeps AI handlers testable.
type llmFeatureResolver interface {
	ResolveForFeatureWithOverride(featureKey string, userOverrideConnectionID int) (llm.Client, error)
}

// configuration. The user-supplied override (Chat UI's connection selector) is
// only honored when the feature is in default mode — a feature that's pinned
// to a specific connection or fully disabled cannot be bypassed by sending a
// different connection_id.
func requireLLMClientForFeature(w http.ResponseWriter, r *http.Request, manager llmFeatureResolver, featureKey string, userOverrideConnectionID int) llm.Client {
	client, err := manager.ResolveForFeatureWithOverride(featureKey, userOverrideConnectionID)
	if err != nil {
		if errors.Is(err, llm.ErrFeatureDisabled) {
			restapi.RespondErrorWithMessage(w, r, http.StatusForbidden, "feature_disabled", err.Error())
			return nil
		}
		respondInternalError(w, r, fmt.Errorf("failed to resolve LLM connection for feature %s: %w", featureKey, err))
		return nil
	}
	if !client.Available() {
		respondServiceUnavailable(w, r, "AI features are not available. LLM service is not configured.")
		return nil
	}
	return client
}

// aiWriteDeadline is the per-request write deadline for AI handlers. It must
// stay strictly above llm.DefaultRequestTimeout (the bound on the handler's
// actual work): otherwise the server's 30s WriteTimeout — which this replaces —
// would sever the response before the handler finishes. The 30s margin covers
// response serialization and flush after the LLM call returns.
const aiWriteDeadline = llm.DefaultRequestTimeout + 30*time.Second

// extendWriteDeadline pushes the HTTP server's per-request write deadline
// forward so that long-running AI calls aren't killed by WriteTimeout.
func extendWriteDeadline(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(aiWriteDeadline))
}

// requestIsSystemAdmin reports whether the authenticated caller is a system
// admin. It consults the permission service because the user attached to a
// session is reloaded without the cached IsSystemAdmin flag.
func requestIsSystemAdmin(r *http.Request, permService *services.PermissionService) bool {
	user := utils.GetCurrentUser(r)
	if user == nil || permService == nil {
		return false
	}
	isAdmin, err := permService.IsSystemAdmin(user.ID)
	return err == nil && isAdmin
}

// respondLLMError logs an LLM call failure and writes a structured 503 response.
// The provider's own message is logged in full (redacted) and attached to the
// response for system admins so a rejected request is diagnosable without a
// proxy capture. Non-admins get the generic message.
func respondLLMError(w http.ResponseWriter, r *http.Request, err error, permService *services.PermissionService) {
	detail := redact.String(err.Error())
	slog.ErrorContext(r.Context(), "LLM request failed",
		slog.String("error_type", fmt.Sprintf("%T", err)),
		slog.String("error", detail),
	)
	apiErr := restapi.NewAPIError(http.StatusServiceUnavailable, restapi.ErrCodeServiceUnavailable,
		"The AI service could not complete this request.")
	if requestIsSystemAdmin(r, permService) {
		apiErr.Details = map[string]string{"provider_error": detail}
	}
	respondError(w, r, apiErr)
}
