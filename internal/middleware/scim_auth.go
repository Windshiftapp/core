package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"windshift/internal/auth"
	"windshift/internal/models"
)

// SCIMAuthMiddleware handles SCIM token authentication
type SCIMAuthMiddleware struct {
	tokenManager *auth.SCIMTokenManager
}

// NewSCIMAuthMiddleware creates a new SCIM authentication middleware
func NewSCIMAuthMiddleware(tokenManager *auth.SCIMTokenManager) *SCIMAuthMiddleware {
	return &SCIMAuthMiddleware{
		tokenManager: tokenManager,
	}
}

// RequireSCIMAuth validates SCIM bearer token and adds token to context
func (m *SCIMAuthMiddleware) RequireSCIMAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")

		// Check for Bearer token
		if !strings.HasPrefix(authHeader, "Bearer ") {
			respondSCIMError(w, http.StatusUnauthorized, "Missing or invalid Authorization header", "")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Check for SCIM token prefix
		if !strings.HasPrefix(token, auth.SCIMTokenPrefix) {
			respondSCIMError(w, http.StatusUnauthorized, "Invalid token type. SCIM endpoints require a SCIM token.", "")
			return
		}

		// Validate token
		scimToken, err := m.tokenManager.ValidateToken(token)
		if err != nil {
			respondSCIMError(w, http.StatusUnauthorized, "Invalid or expired SCIM token", "")
			return
		}

		// Add SCIM token to context
		ctx := context.WithValue(r.Context(), ContextKeySCIMToken, scimToken)
		ctx = context.WithValue(ctx, ContextKeyAuthMethod, "scim")
		ctx = context.WithValue(ctx, ContextKeyCSRFExempt, true) // SCIM requests are CSRF exempt

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetSCIMToken retrieves the SCIM token from the request context
func GetSCIMToken(r *http.Request) *models.SCIMToken {
	if token, ok := r.Context().Value(ContextKeySCIMToken).(*models.SCIMToken); ok {
		return token
	}
	return nil
}

// respondSCIMError sends a SCIM-formatted error response
func respondSCIMError(w http.ResponseWriter, status int, detail, scimType string) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)

	scimError := models.SCIMError{
		Schemas:  []string{models.SCIMSchemaError},
		Detail:   detail,
		Status:   formatStatusCode(status),
		ScimType: scimType,
	}

	_ = json.NewEncoder(w).Encode(scimError)
}

// formatStatusCode converts an int status code to a string
func formatStatusCode(status int) string {
	return string(rune('0'+status/100)) + string(rune('0'+(status/10)%10)) + string(rune('0'+status%10))
}
