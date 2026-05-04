package mcp

import (
	"context"
	"net/http"
	"strings"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/restapi"
)

// bearerAuthMiddleware validates Bearer tokens and injects the user into context.
func bearerAuthMiddleware(tokenManager *auth.TokenManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow DELETE without auth (MCP session termination)
		if r.Method == http.MethodDelete {
			next.ServeHTTP(w, r)
			return
		}

		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(header, "Bearer ")
		user, apiToken, err := tokenManager.ValidateToken(token)
		if err != nil {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		if !tokenManager.CheckTokenPermissions(apiToken, []string{auth.ScopeMCPAccess}) {
			http.Error(w, `{"error":"token missing required scope: mcp:access"}`, http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), restapi.ContextKeyUser, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// userFromContext extracts the authenticated user from context.
func userFromContext(ctx context.Context) *models.User {
	if user, ok := ctx.Value(restapi.ContextKeyUser).(*models.User); ok {
		return user
	}
	return nil
}
