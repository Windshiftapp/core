package middleware

import (
	"context"
	"net/http"
	"strings"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/restapi"
)

// BearerAuth middleware requires bearer token authentication for the public API
// It only accepts Authorization: Bearer crw_xxx tokens, not session cookies
type BearerAuth struct {
	tokenManager *auth.TokenManager
}

// NewBearerAuth creates a new bearer token auth middleware
func NewBearerAuth(tokenManager *auth.TokenManager) *BearerAuth {
	return &BearerAuth{
		tokenManager: tokenManager,
	}
}

// RequireAuth returns middleware that requires valid bearer token authentication
func (ba *BearerAuth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			restapi.RespondError(w, r, restapi.ErrUnauthorized)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			restapi.RespondError(w, r, restapi.NewAPIError(
				http.StatusUnauthorized,
				restapi.ErrCodeInvalidToken,
				"Authorization header must use Bearer scheme",
			))
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			restapi.RespondError(w, r, restapi.NewAPIError(
				http.StatusUnauthorized,
				restapi.ErrCodeInvalidToken,
				"Bearer token is empty",
			))
			return
		}

		user, apiToken, err := ba.tokenManager.ValidateToken(token)
		if err != nil {
			// Check for specific error types
			errMsg := err.Error()
			if strings.Contains(errMsg, "expired") {
				restapi.RespondError(w, r, restapi.ErrTokenExpired)
				return
			}
			if strings.Contains(errMsg, "disabled") {
				restapi.RespondError(w, r, restapi.NewAPIError(
					http.StatusUnauthorized,
					restapi.ErrCodeUnauthorized,
					"User account is disabled",
				))
				return
			}
			restapi.RespondError(w, r, restapi.ErrInvalidToken)
			return
		}

		// Add user and token to context
		ctx := r.Context()
		ctx = context.WithValue(ctx, restapi.ContextKeyUser, user)
		ctx = context.WithValue(ctx, restapi.ContextKeyAPIToken, apiToken)
		ctx = context.WithValue(ctx, restapi.ContextKeyAuthMethod, "bearer")

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequirePermission returns middleware that checks if the token has required permissions
func (ba *BearerAuth) RequirePermission(permissions ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiToken, ok := r.Context().Value(restapi.ContextKeyAPIToken).(*models.ApiToken)
			if !ok || apiToken == nil {
				restapi.RespondError(w, r, restapi.ErrUnauthorized)
				return
			}

			if !ba.tokenManager.CheckTokenPermissions(apiToken, permissions) {
				restapi.RespondError(w, r, restapi.NewAPIError(
					http.StatusForbidden,
					restapi.ErrCodeInsufficientPermission,
					"Token does not have required permissions",
				).WithDetails(map[string]interface{}{
					"required": permissions,
				}))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetUser retrieves the authenticated user from context
func GetUser(ctx context.Context) *models.User {
	if user, ok := ctx.Value(restapi.ContextKeyUser).(*models.User); ok {
		return user
	}
	return nil
}

// GetAPIToken retrieves the API token from context
func GetAPIToken(ctx context.Context) *models.ApiToken {
	if token, ok := ctx.Value(restapi.ContextKeyAPIToken).(*models.ApiToken); ok {
		return token
	}
	return nil
}
