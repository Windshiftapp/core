package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/restapi"
	"windshift/internal/restapi/v1/middleware"
)

func TestRequirePermission(t *testing.T) {
	ba := middleware.NewBearerAuthWithPermissions(&auth.TokenManager{}, nil)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	withToken := func(req *http.Request, token *models.APIToken) *http.Request {
		ctx := context.WithValue(req.Context(), restapi.ContextKeyAPIToken, token)
		return req.WithContext(ctx)
	}

	t.Run("token has required scope passes through", func(t *testing.T) {
		handler := ba.RequirePermission("items:write")(next)
		req := withToken(
			httptest.NewRequest(http.MethodPost, "/items", nil),
			&models.APIToken{Permissions: `["items:write"]`},
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("token lacks required scope returns 403 with details", func(t *testing.T) {
		handler := ba.RequirePermission("items:write")(next)
		req := withToken(
			httptest.NewRequest(http.MethodPost, "/items", nil),
			&models.APIToken{Permissions: `["items:read"]`},
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusForbidden)
		}

		var body restapi.ErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response body: %v", err)
		}
		if body.Code != restapi.ErrCodeInsufficientPermission {
			t.Errorf("got code %q, want %q", body.Code, restapi.ErrCodeInsufficientPermission)
		}
		details, ok := body.Details.(map[string]interface{})
		if !ok {
			t.Fatalf("details is %T, want map[string]interface{}", body.Details)
		}
		required, ok := details["required"].([]interface{})
		if !ok {
			t.Fatalf("details.required is %T, want []interface{}", details["required"])
		}
		if len(required) != 1 || required[0] != "items:write" {
			t.Errorf("got details.required = %v, want [items:write]", required)
		}
	})

	t.Run("multiple required scopes, token missing one returns 403", func(t *testing.T) {
		handler := ba.RequirePermission("items:read", "workspaces:write")(next)
		req := withToken(
			httptest.NewRequest(http.MethodPost, "/anything", nil),
			&models.APIToken{Permissions: `["items:read"]`},
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("write scope satisfies read requirement", func(t *testing.T) {
		handler := ba.RequirePermission("items:read")(next)
		req := withToken(
			httptest.NewRequest(http.MethodGet, "/items", nil),
			&models.APIToken{Permissions: `["items:write"]`},
		)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("no token in context returns 401", func(t *testing.T) {
		handler := ba.RequirePermission("items:write")(next)
		req := httptest.NewRequest(http.MethodPost, "/items", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got status %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		var body restapi.ErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response body: %v", err)
		}
		if body.Code != restapi.ErrCodeUnauthorized {
			t.Errorf("got code %q, want %q", body.Code, restapi.ErrCodeUnauthorized)
		}
	})
}
