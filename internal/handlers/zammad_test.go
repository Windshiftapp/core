package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"windshift/internal/database"
	"windshift/internal/integrations/zammad"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/sso"
)

func newZammadHandlerTest(t *testing.T) (*ZammadHandler, database.Database, *models.User, int) {
	t.Helper()
	db, err := database.NewSQLiteDB(t.TempDir() + "/windshift.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Initialize(); err != nil {
		t.Fatal(err)
	}
	result, err := db.ExecWrite(`INSERT INTO users (email, username, first_name, last_name) VALUES (?, ?, ?, ?)`, "admin@example.test", "synthetic-admin", "Synthetic", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	userID, _ := result.LastInsertId()
	result, err = db.ExecWrite(`INSERT INTO workspaces (name, key) VALUES (?, ?)`, "Primary", "PRI")
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, _ := result.LastInsertId()
	credentialService := services.NewActionCredentialService(repository.NewActionCredentialRepository(db), "synthetic-handler-test-secret")
	service := services.NewZammadService(db, repository.NewZammadRepository(db), credentialService, nil, nil, nil, nil)
	return NewZammadHandler(repository.NewItemRepository(db), service, nil, logger.NewAuditor(db)), db, &models.User{ID: int(userID), Username: "synthetic-admin"}, int(workspaceID)
}

func authenticatedZammadRequest(method, target string, body []byte, user *models.User) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	return req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUser, user))
}

func TestZammadHandlerCreateConnectionResponseOmitsSecret(t *testing.T) {
	handler, db, user, workspaceID := newZammadHandlerTest(t)
	body, _ := json.Marshal(models.CreateZammadConnectionRequest{
		Slug: "helpdesk", Name: "Synthetic helpdesk", BaseURL: "https://zammad.example.test",
		APIToken: "handler-secret-token", DefaultGroupID: 7, DefaultGroupName: "Support",
		DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{workspaceID},
	})
	recorder := httptest.NewRecorder()
	handler.CreateConnection(recorder, authenticatedZammadRequest(http.MethodPost, "/api/admin/zammad-connections", body, user))
	if recorder.Code != http.StatusCreated || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected response status or type: %d %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["has_api_token"] != true || response["id"] == "" || response["base_url"] != "https://zammad.example.test" {
		t.Fatalf("unexpected response shape: %#v", response)
	}
	if _, exists := response["api_token"]; exists {
		t.Fatalf("response disclosed api_token field: %s", recorder.Body.String())
	}
	if _, exists := response["credential_id"]; exists {
		t.Fatalf("response disclosed credential_id field: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "handler-secret-token") {
		t.Fatalf("response disclosed token: %s", recorder.Body.String())
	}
	var encrypted string
	if err := db.QueryRow("SELECT encrypted_secret FROM action_credentials").Scan(&encrypted); err != nil || encrypted == "handler-secret-token" {
		t.Fatalf("credential was not encrypted: err=%v", err)
	}
	genericRepo := repository.NewIntegrationProviderRepository(db)
	providers, err := genericRepo.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 0 {
		t.Fatalf("provider-specific Zammad connection leaked into generic OAuth CRUD: %#v", providers)
	}
	if err := genericRepo.Delete(response["id"].(string)); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("generic provider delete was not blocked: %v", err)
	}
}

func TestZammadHandlerReturnsStructuredValidationAndNotFoundErrors(t *testing.T) {
	handler, _, user, workspaceID := newZammadHandlerTest(t)
	body, _ := json.Marshal(models.CreateZammadConnectionRequest{
		Slug: "helpdesk", Name: "Synthetic helpdesk", BaseURL: "http://zammad.example.test",
		APIToken: "handler-secret-token", DefaultCustomer: "robot@example.test",
		WorkspaceIDs: []int{workspaceID},
	})
	recorder := httptest.NewRecorder()
	handler.CreateConnection(recorder, authenticatedZammadRequest(http.MethodPost, "/api/admin/zammad-connections", body, user))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected validation status 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var validation map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &validation); err != nil {
		t.Fatal(err)
	}
	if validation["code"] != "VALIDATION_FAILED" {
		t.Fatalf("unexpected validation response: %#v", validation)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/admin/zammad-connections/missing", nil)
	request.SetPathValue("id", "missing")
	recorder = httptest.NewRecorder()
	handler.GetConnection(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected not-found status 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestZammadHandlerRefreshAllTicketsQueuesBackgroundRun(t *testing.T) {
	handler, _, user, _ := newZammadHandlerTest(t)
	handler.SetSyncAllTrigger(func() bool { return true })
	recorder := httptest.NewRecorder()
	request := authenticatedZammadRequest(http.MethodPost, "/api/admin/zammad-ticket-links/refresh", nil, user)
	handler.RefreshAllTickets(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d %s", recorder.Code, recorder.Body.String())
	}
	var result map[string]bool
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result["started"] {
		t.Fatalf("unexpected refresh result: %#v", result)
	}
}

func TestZammadWorkspaceOwnersRequireItemEditPermission(t *testing.T) {
	handler, db, user, workspaceID := newZammadHandlerTest(t)
	if _, err := db.ExecWrite(`INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by)
		SELECT ?, ?, id, ? FROM workspace_roles WHERE name = 'Viewer'`, user.ID, workspaceID, user.ID); err != nil {
		t.Fatal(err)
	}
	config := services.DefaultPermissionCacheConfig()
	config.WarmupOnStartup = false
	config.PreWarmActive = false
	permissionService, err := services.NewPermissionService(db, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = permissionService.Close() })
	handler.permissionService = permissionService

	workspacePathID := strconv.Itoa(workspaceID)
	request := authenticatedZammadRequest(http.MethodGet, "/api/workspaces/"+workspacePathID+"/zammad-connections/helpdesk/owners?group_id=7", nil, user)
	request.SetPathValue("workspaceId", workspacePathID)
	request.SetPathValue("id", "helpdesk")
	recorder := httptest.NewRecorder()
	handler.GetWorkspaceOwners(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("viewer could enumerate Zammad owners: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestZammadTicketLinkResolverRequiresViewPermission(t *testing.T) {
	handler, db, user, workspaceID := newZammadHandlerTest(t)
	var statusID int
	if err := db.QueryRow(`SELECT id FROM statuses ORDER BY id LIMIT 1`).Scan(&statusID); err != nil {
		t.Fatal(err)
	}
	itemResult, err := db.ExecWrite(`INSERT INTO items
		(workspace_id, workspace_item_number, title, description, frac_index, status_id, creator_id, last_active_at)
		VALUES (?, 1, 'Resolver target', '', 'a0', ?, ?, CURRENT_TIMESTAMP)`, workspaceID, statusID, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	itemID, _ := itemResult.LastInsertId()
	if _, err := db.ExecWrite(`INSERT INTO action_credentials
		(id, name, credential_type, applies_to_all_workspaces, encrypted_secret, is_enabled)
		VALUES (1, 'Resolver credential', 'custom_header', true, 'test-ciphertext', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO integration_providers
		(id, slug, name, provider_type, enabled)
		VALUES ('zammad-resolver', 'zammad-resolver', 'Zammad resolver', 'zammad', true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO zammad_connections
		(provider_id, credential_id, base_url, default_customer)
		VALUES ('zammad-resolver', 1, 'https://zammad.example.test', 'robot@example.test')`); err != nil {
		t.Fatal(err)
	}
	correlationKey := "windshift:zammad-resolver:PRI-1"
	if _, err := db.ExecWrite(`INSERT INTO zammad_ticket_links
		(id, item_id, provider_id, correlation_key, sync_state, created_by)
		VALUES ('resolver-link', ?, 'zammad-resolver', ?, ?, ?)`, itemID, correlationKey, models.ZammadSyncLinked, user.ID); err != nil {
		t.Fatal(err)
	}
	movedWorkspaceResult, err := db.ExecWrite(`INSERT INTO workspaces (name, key) VALUES ('Moved target', 'MOV')`)
	if err != nil {
		t.Fatal(err)
	}
	movedWorkspaceID, _ := movedWorkspaceResult.LastInsertId()
	if _, err := db.ExecWrite(`UPDATE items SET workspace_id = ? WHERE id = ?`, movedWorkspaceID, itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecWrite(`INSERT INTO user_workspace_roles (user_id, workspace_id, role_id, granted_by)
		SELECT ?, ?, id, ? FROM workspace_roles WHERE name = 'Viewer'`, user.ID, movedWorkspaceID, user.ID); err != nil {
		t.Fatal(err)
	}
	config := services.DefaultPermissionCacheConfig()
	config.WarmupOnStartup = false
	config.PreWarmActive = false
	permissionService, err := services.NewPermissionService(db, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = permissionService.Close() })
	handler.permissionService = permissionService

	request := authenticatedZammadRequest(http.MethodGet, "/api/zammad-ticket-links/resolve/"+correlationKey, nil, user)
	request.SetPathValue("correlationKey", correlationKey)
	recorder := httptest.NewRecorder()
	handler.ResolveTicketLink(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("viewer could not resolve linked item: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var destination struct {
		WorkspaceID int `json:"workspace_id"`
		ItemID      int `json:"item_id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &destination); err != nil {
		t.Fatal(err)
	}
	if destination.WorkspaceID != int(movedWorkspaceID) || destination.ItemID != int(itemID) {
		t.Fatalf("unexpected resolver destination: %#v", destination)
	}

	deniedResult, err := db.ExecWrite(`INSERT INTO users (email, username, first_name, last_name)
		VALUES ('denied@example.test', 'denied', 'Denied', 'User')`)
	if err != nil {
		t.Fatal(err)
	}
	deniedID, _ := deniedResult.LastInsertId()
	deniedRequest := authenticatedZammadRequest(http.MethodGet, "/api/zammad-ticket-links/resolve/"+correlationKey, nil, &models.User{ID: int(deniedID)})
	deniedRequest.SetPathValue("correlationKey", correlationKey)
	deniedRecorder := httptest.NewRecorder()
	handler.ResolveTicketLink(deniedRecorder, deniedRequest)
	if deniedRecorder.Code != http.StatusNotFound {
		t.Fatalf("resolver disclosed an item without view permission: status=%d body=%s", deniedRecorder.Code, deniedRecorder.Body.String())
	}

	missingKey := "windshift:zammad-resolver:missing"
	missingRequest := authenticatedZammadRequest(http.MethodGet, "/api/zammad-ticket-links/resolve/"+missingKey, nil, user)
	missingRequest.SetPathValue("correlationKey", missingKey)
	missingRecorder := httptest.NewRecorder()
	handler.ResolveTicketLink(missingRecorder, missingRequest)
	if missingRecorder.Code != deniedRecorder.Code || missingRecorder.Body.String() != deniedRecorder.Body.String() {
		t.Fatalf("resolver distinguished an unauthorized target from a missing one: denied=%d %s missing=%d %s",
			deniedRecorder.Code, deniedRecorder.Body.String(), missingRecorder.Code, missingRecorder.Body.String())
	}
}

func TestZammadHandlerMapsRemoteAuthenticationFailureToBadGateway(t *testing.T) {
	handler, _, _, _ := newZammadHandlerTest(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/zammad-connections/example/test", nil)
	if handler.respondServiceError(recorder, request, &zammad.APIError{StatusCode: http.StatusUnauthorized}) {
		t.Fatal("expected handler to write an error response")
	}
	if recorder.Code != http.StatusBadGateway || strings.Contains(recorder.Body.String(), "401") {
		t.Fatalf("unexpected upstream response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestZammadHandlerMapsOAuthRefreshContentionToRetryableResponse(t *testing.T) {
	handler, _, _, _ := newZammadHandlerTest(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/zammad-connections/example/test", nil)
	if handler.respondServiceError(recorder, request, services.ErrZammadOAuthRefreshInProgress) {
		t.Fatal("expected handler to write an error response")
	}
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("unexpected refresh contention response: status=%d retry-after=%q body=%s", recorder.Code, recorder.Header().Get("Retry-After"), recorder.Body.String())
	}
}

func TestZammadHandlerMapsSupersededOAuthOperationToRetryableConflict(t *testing.T) {
	handler, _, _, _ := newZammadHandlerTest(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/zammad-connections/example/test", nil)
	if handler.respondServiceError(recorder, request, services.ErrZammadOAuthSuperseded) {
		t.Fatal("expected handler to write an error response")
	}
	if recorder.Code != http.StatusConflict || recorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("unexpected superseded OAuth response: status=%d retry-after=%q body=%s", recorder.Code, recorder.Header().Get("Retry-After"), recorder.Body.String())
	}
}

func TestZammadHandlerMapsReservationRaceToConflict(t *testing.T) {
	handler, _, _, _ := newZammadHandlerTest(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/items/1/zammad-tickets", nil)
	if handler.respondServiceError(recorder, request, services.ErrZammadLinkReservationConflict) {
		t.Fatal("expected handler to write an error response")
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("unexpected reservation conflict response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestZammadHandlerMapsConcurrentConnectionUpdateToConflict(t *testing.T) {
	handler, _, _, _ := newZammadHandlerTest(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/zammad-connections/example", nil)
	if handler.respondServiceError(recorder, request, repository.ErrConcurrentUpdate) {
		t.Fatal("expected handler to write an error response")
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("unexpected connection conflict response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestZammadHandlerMapsTicketGroupPolicyRaceToConflict(t *testing.T) {
	handler, _, _, _ := newZammadHandlerTest(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/integrations/zammad/ticket-links/example", nil)
	if handler.respondServiceError(recorder, request, services.ErrZammadTicketGroupPolicyChanged) {
		t.Fatal("expected handler to write an error response")
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("unexpected group policy conflict response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestZammadHandlerMapsBusyConnectionToRetryableConflict(t *testing.T) {
	handler, _, _, _ := newZammadHandlerTest(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/admin/zammad-connections/example", nil)
	if handler.respondServiceError(recorder, request, services.ErrZammadConnectionBusy) {
		t.Fatal("expected handler to write an error response")
	}
	if recorder.Code != http.StatusConflict || recorder.Header().Get("Retry-After") != "1" {
		t.Fatalf("unexpected busy connection response: status=%d retry-after=%q body=%s", recorder.Code, recorder.Header().Get("Retry-After"), recorder.Body.String())
	}
}

func TestZammadConnectionReadyRequiresLocallyUsableDefaultGroup(t *testing.T) {
	tests := []struct {
		name       string
		connection models.ZammadConnection
		want       bool
	}{
		{name: "numeric default without allowlist", connection: models.ZammadConnection{DefaultGroupID: 7}, want: true},
		{name: "numeric default inside allowlist", connection: models.ZammadConnection{DefaultGroupID: 7, AllowedGroups: []models.ZammadGroupRef{{ID: 7, Name: "Support"}, {ID: 8, Name: "Escalations"}}}, want: true},
		{name: "numeric default outside allowlist", connection: models.ZammadConnection{DefaultGroupID: 7, AllowedGroups: []models.ZammadGroupRef{{ID: 8, Name: "Escalations"}}}, want: false},
		{name: "name default without allowlist", connection: models.ZammadConnection{DefaultGroupName: "Support"}, want: true},
		{name: "name default with allowlist is unresolved", connection: models.ZammadConnection{DefaultGroupName: "Support", AllowedGroups: []models.ZammadGroupRef{{ID: 8, Name: "Escalations"}}}, want: false},
		{name: "missing default", connection: models.ZammadConnection{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := zammadConnectionHasUsableDefaultGroup(&tt.connection); got != tt.want {
				t.Fatalf("zammadConnectionHasUsableDefaultGroup(%#v) = %v, want %v", tt.connection, got, tt.want)
			}
		})
	}
}

func TestZammadOAuthCallbackRedirectsToIntegrationsAdminTab(t *testing.T) {
	handler, db, _, _ := newZammadHandlerTest(t)
	oauthHandler := NewIntegrationOAuthHandler(db, sso.NewSecretEncryption("synthetic-handler-test-secret"), "https://windshift.example.test")
	oauthHandler.RegisterSystemOAuthFlow(models.IntegrationProviderZammad, handler.service, logger.NewAuditor(db))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/integrations/oauth/system/zammad/callback?error=access_denied&state=missing", nil)
	request.SetPathValue("providerType", string(models.IntegrationProviderZammad))
	request.SetPathValue("slug", "zammad")
	oauthHandler.SystemOAuthCallback(recorder, request)
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/admin/integration-providers?tab=zammad&oauth=error" {
		t.Fatalf("unexpected OAuth callback redirect: status=%d location=%q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestUserOAuthSlugCannotCollideWithSystemCallback(t *testing.T) {
	_, db, _, _ := newZammadHandlerTest(t)
	oauthHandler := NewIntegrationOAuthHandler(db, sso.NewSecretEncryption("synthetic-handler-test-secret"), "https://windshift.example.test")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/integrations/oauth/zammad/callback?error=access_denied", nil)
	request.SetPathValue("slug", "zammad")
	oauthHandler.OAuthCallback(recorder, request)
	if recorder.Code != http.StatusFound || !strings.HasPrefix(recorder.Header().Get("Location"), "/profile?tab=connected-accounts&oauth=error") {
		t.Fatalf("user callback was dispatched as a system callback: status=%d location=%q", recorder.Code, recorder.Header().Get("Location"))
	}
}

func TestZammadOAuthCallbackAuditsInitiatorSuccessAndFailureWithoutSecrets(t *testing.T) {
	handler, db, user, workspaceID := newZammadHandlerTest(t)
	encryption := sso.NewSecretEncryption("synthetic-handler-test-secret")
	handler.service.SetOAuthEncryption(encryption)
	oauthHandler := NewIntegrationOAuthHandler(db, encryption, "https://windshift.example.test")
	oauthHandler.RegisterSystemOAuthFlow(models.IntegrationProviderZammad, handler.service, logger.NewAuditor(db))
	handler.service.SetOAuthTransportForTesting(zammad.TransportFunc(func(_ context.Context, _ string, _ string, body []byte, _ map[string]string) (*zammad.Response, error) {
		if strings.Contains(string(body), "failure-code") {
			return &zammad.Response{StatusCode: http.StatusBadGateway, Body: []byte(`{"error":"upstream_failure"}`)}, nil
		}
		return &zammad.Response{StatusCode: http.StatusOK, Body: []byte(`{"access_token":"audit-access-secret","refresh_token":"audit-refresh-secret","expires_in":3600}`)}, nil
	}))
	connection, err := handler.service.CreateConnection(models.CreateZammadConnectionRequest{
		Slug: "audit-oauth", Name: "Audit OAuth", BaseURL: "https://audit-oauth.example.test",
		AuthMethod: models.ZammadAuthMethodOAuth, OAuthClientID: "audit-client", OAuthClientSecret: "audit-client-secret",
		DefaultGroupID: 7, DefaultGroupName: "Support", AllowedGroups: []models.ZammadGroupRef{{ID: 7, Name: "Support"}},
		DefaultCustomer: "robot@example.test", WorkspaceIDs: []int{workspaceID},
	}, user.ID)
	if err != nil {
		t.Fatal(err)
	}

	callback := func(code string) {
		t.Helper()
		authURL, err := handler.service.StartOAuth(context.Background(), connection.ProviderID, user.ID, "https://windshift.example.test")
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(authURL)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodGet, "/api/integrations/oauth/system/zammad/callback?state="+url.QueryEscape(parsed.Query().Get("state"))+"&code="+url.QueryEscape(code), nil)
		request.SetPathValue("providerType", string(models.IntegrationProviderZammad))
		request.SetPathValue("slug", "zammad")
		recorder := httptest.NewRecorder()
		oauthHandler.SystemOAuthCallback(recorder, request)
		if recorder.Code != http.StatusFound {
			t.Fatalf("callback status = %d", recorder.Code)
		}
	}
	callback("success-code")
	callback("failure-code")

	rows, err := db.Query(`SELECT user_id, username, success, COALESCE(error_message, ''), COALESCE(details, '')
		FROM audit_logs WHERE action_type = ? ORDER BY id`, logger.ActionIntegrationProviderOAuthCredentialSet)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	type auditRow struct {
		userID                int
		username, error, data string
		success               bool
	}
	var audits []auditRow
	for rows.Next() {
		var row auditRow
		if err := rows.Scan(&row.userID, &row.username, &row.success, &row.error, &row.data); err != nil {
			t.Fatal(err)
		}
		audits = append(audits, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(audits) != 2 || !audits[0].success || audits[1].success || audits[0].userID != user.ID || audits[1].userID != user.ID || audits[0].username != user.Username || audits[1].error != "oauth_callback_failed" {
		t.Fatalf("unexpected callback audits: %#v", audits)
	}
	for _, audit := range audits {
		if strings.Contains(audit.data, "success-code") || strings.Contains(audit.data, "failure-code") || strings.Contains(audit.data, "audit-access-secret") || strings.Contains(audit.data, "audit-refresh-secret") || strings.Contains(audit.data, "audit-client-secret") {
			t.Fatalf("callback audit disclosed OAuth material: %s", audit.data)
		}
	}
}
