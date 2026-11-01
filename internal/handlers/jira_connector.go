package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"windshift/internal/blobstore"
	"windshift/internal/database"
	"windshift/internal/jira"
	"windshift/internal/jiraimport"
	"windshift/internal/logger"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/sso"
	"windshift/internal/utils"

	"uuid"
)

// JiraImportHandler handles Jira import endpoints
type JiraImportHandler struct {
	db                 database.Database
	imports            *jiraimport.Service
	storage            *blobstore.Store
	encryption         *sso.SecretEncryption
	capturePayloadsDir string // JIRA_CAPTURE_PAYLOADS (empty disables capture)
	mappingFailuresMu  sync.Mutex
	mappingFailures    map[string]error
	// jobTokens holds the per-run ownership token for each active import, keyed
	// by job id, so job-state writes can be fenced without threading the token
	// through every import helper.
	jobTokens        map[string]string
	cacheInvalidator *services.AuthorizationCacheInvalidator
}

func (h *JiraImportHandler) WithAuthorizationCacheInvalidator(invalidator *services.AuthorizationCacheInvalidator) *JiraImportHandler {
	h.cacheInvalidator = invalidator
	return h
}

// NewJiraImportHandler creates a new Jira import handler.
// sessionSecret: session-signing secret (resolved upstream by config.Load).
// capturePayloadsDir: optional directory for request/response capture (empty = disabled).
func NewJiraImportHandler(db database.Database, sessionSecret, capturePayloadsDir string, storage *blobstore.Store) *JiraImportHandler {
	if sessionSecret == "" {
		slog.Error("NewJiraImportHandler received empty session secret (config wiring bug)", slog.String("component", "jira"))
		panic("config: empty session secret passed to NewJiraImportHandler")
	}
	return &JiraImportHandler{
		db:                 db,
		imports:            jiraimport.New(db, storage),
		storage:            storage,
		encryption:         sso.NewSecretEncryption(sessionSecret),
		capturePayloadsDir: capturePayloadsDir,
		mappingFailures:    make(map[string]error),
	}
}

// Connect handles POST /api/admin/jira-import/connect
func (h *JiraImportHandler) Connect(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[JiraConnectRequest](w, r)
	if !ok {
		return
	}
	sanitizeJiraConnectRequest(&req)

	deploymentType := resolveJiraDeploymentType(req.DeploymentType)
	if message := validateJiraConnectRequest(req, deploymentType); message != "" {
		respondValidationError(w, r, message)
		return
	}

	instanceInfo, message := testJiraConnection(r.Context(), req, deploymentType)
	if message != "" {
		respondBadRequest(w, r, message)
		return
	}

	// Encrypt the API token
	encryptedToken, err := h.encryption.Encrypt(req.APIToken)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to encrypt credentials: %w", err))
		return
	}

	// Generate connection ID and store in database
	connectionID := uuid.New().String()

	// Get user ID from session
	userID := getUserIDFromContext(r)

	err = h.imports.CreateConnection(jiraimport.NewConnection{
		ID: connectionID, InstanceURL: req.InstanceURL, Email: req.Email,
		EncryptedCredentials: encryptedToken, InstanceName: instanceInfo.DisplayName,
		DeploymentType: string(deploymentType), CreatedBy: userID,
	})
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to store connection: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionJiraConnect,
			ResourceType: logger.ResourceJiraConnection,
			ResourceName: connectionID,
			Details: map[string]any{
				"instance_url":    req.InstanceURL,
				"instance_name":   instanceInfo.DisplayName,
				"deployment_type": string(deploymentType),
			},
			Success: true,
		})
	}

	respondJSONOK(w, JiraConnectResponse{
		ConnectionID: connectionID,
		InstanceInfo: instanceInfo,
	})
}

// UpdateConnection handles PUT /api/admin/jira-import/connections/{connectionId}.
// It replaces the stored credentials after verifying them with Jira, so an
// expired or revoked token is recoverable without deleting the connection and
// its import provenance.
func (h *JiraImportHandler) UpdateConnection(w http.ResponseWriter, r *http.Request) {
	connectionID := r.PathValue("connectionId")
	req, ok := decodeJSON[JiraConnectionUpdateRequest](w, r)
	if !ok {
		return
	}
	sanitize.ApplyAll(sanitize.Pair{Target: &req.Email, Policy: sanitize.ShortIdentifier})

	stored, err := h.imports.LoadConnectionCredentials(connectionID)
	if errors.Is(err, jiraimport.ErrConnectionNotFound) {
		respondNotFound(w, r, "connection")
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to load connection: %w", err))
		return
	}

	deploymentType := resolveJiraDeploymentType(stored.DeploymentType)
	if req.APIToken == "" {
		respondValidationError(w, r, "api_token is required")
		return
	}
	if deploymentType == jira.DeploymentCloud && req.Email == "" {
		respondValidationError(w, r, "email is required for Jira Cloud")
		return
	}

	connectRequest := JiraConnectRequest{
		InstanceURL:    stored.InstanceURL,
		Email:          req.Email,
		APIToken:       req.APIToken,
		DeploymentType: string(deploymentType),
	}
	instanceInfo, message := testJiraConnection(r.Context(), connectRequest, deploymentType)
	if message != "" {
		respondBadRequest(w, r, message)
		return
	}

	encryptedToken, err := h.encryption.Encrypt(req.APIToken)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to encrypt credentials: %w", err))
		return
	}

	if err := h.imports.UpdateConnection(connectionID, jiraimport.ConnectionUpdate{
		Email:                req.Email,
		InstanceName:         instanceInfo.DisplayName,
		EncryptedCredentials: encryptedToken,
	}); err != nil {
		if errors.Is(err, jiraimport.ErrConnectionNotFound) {
			respondNotFound(w, r, "connection")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to update connection: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionJiraConnectionUpdate,
			ResourceType: logger.ResourceJiraConnection,
			ResourceName: connectionID,
			Details: map[string]any{
				"instance_url":    stored.InstanceURL,
				"instance_name":   instanceInfo.DisplayName,
				"deployment_type": string(deploymentType),
			},
			Success: true,
		})
	}

	respondJSONOK(w, jiraimport.Connection{
		ID:             connectionID,
		InstanceURL:    stored.InstanceURL,
		Email:          req.Email,
		InstanceName:   instanceInfo.DisplayName,
		DeploymentType: string(deploymentType),
	})
}

// sanitizeJiraConnectRequest bounds the fields that are persisted or rendered.
// APIToken is a secret and DeploymentType a strict enum, so both stay untouched.
func sanitizeJiraConnectRequest(req *JiraConnectRequest) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.InstanceURL, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Email, Policy: sanitize.ShortIdentifier},
	)
}

func resolveJiraDeploymentType(raw string) jira.DeploymentType {
	if raw == "datacenter" {
		return jira.DeploymentDataCenter
	}
	return jira.DeploymentCloud
}

// validateJiraConnectRequest reports a user-facing message for missing fields.
// Data Center PATs authenticate as Bearer tokens and need no username; Jira
// Cloud still requires the account email for Basic auth.
func validateJiraConnectRequest(req JiraConnectRequest, deploymentType jira.DeploymentType) string {
	if req.InstanceURL == "" || req.APIToken == "" {
		return "instance_url and api_token are required"
	}
	if deploymentType == jira.DeploymentCloud && req.Email == "" {
		return "email is required for Jira Cloud"
	}
	return ""
}

// testJiraConnection verifies the supplied credentials against Jira and returns
// a user-facing message on failure. Upstream failures map to a client error,
// never a Windshift 401, so the frontend's session-expired interceptor does not
// log the user out for a third-party error. The upstream reason is logged at
// warn; the API token never is.
func testJiraConnection(ctx context.Context, req JiraConnectRequest, deploymentType jira.DeploymentType) (instanceInfo *jira.JiraInstanceInfo, userMessage string) {
	client, err := jira.NewClient(jira.Config{
		InstanceURL:    req.InstanceURL,
		Email:          req.Email,
		APIToken:       req.APIToken,
		DeploymentType: deploymentType,
	})
	if err != nil {
		return nil, fmt.Sprintf("Failed to create Jira client: %v", err)
	}
	instanceInfo, err = client.TestConnection(ctx)
	if err != nil {
		slog.Warn("Jira connection test failed",
			slog.String("component", "jira"),
			slog.String("instance_url", req.InstanceURL),
			slog.String("email", req.Email),
			slog.String("deployment", string(deploymentType)),
			slog.Any("error", err),
		)
		return nil, fmt.Sprintf("Jira connection test failed: %v", err)
	}
	return instanceInfo, ""
}

// GetConnections handles GET /api/admin/jira-import/connections
func (h *JiraImportHandler) GetConnections(w http.ResponseWriter, r *http.Request) {
	connections, err := h.imports.ListConnections()
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to list connections: %w", err))
		return
	}
	respondJSONOK(w, connections)
}

// DeleteConnection handles DELETE /api/admin/jira-import/connections/{connectionId}
func (h *JiraImportHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	connectionID := r.PathValue("connectionId")

	err := h.imports.DeleteConnection(connectionID)
	if errors.Is(err, jiraimport.ErrConnectionActive) {
		respondConflict(w, r, "This Jira connection has an import queued or running. Wait for it to finish before removing the connection.")
		return
	}
	if errors.Is(err, jiraimport.ErrConnectionNotFound) {
		respondNotFound(w, r, "connection")
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to delete connection: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionJiraDisconnect,
			ResourceType: logger.ResourceJiraConnection,
			ResourceName: connectionID,
			Success:      true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// getClientForConnection retrieves stored credentials and creates a Jira client
func (h *JiraImportHandler) getClientForConnection(_ context.Context, connectionID string) (jira.Client, error) {
	connection, err := h.imports.UseConnection(connectionID)
	if err != nil {
		return nil, fmt.Errorf("connection not found: %w", err)
	}

	// Decrypt the API token
	apiToken, err := h.encryption.Decrypt(connection.EncryptedCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	// Determine deployment type (default to cloud for existing connections)
	deploymentType := jira.DeploymentCloud
	if connection.DeploymentType == "datacenter" {
		deploymentType = jira.DeploymentDataCenter
	}

	return jira.NewClient(jira.Config{
		InstanceURL:    connection.InstanceURL,
		Email:          connection.Email,
		APIToken:       apiToken,
		DeploymentType: deploymentType,
	})
}

// getUserIDFromContext extracts the user ID from request context
func getUserIDFromContext(r *http.Request) *int {
	if user := utils.GetCurrentUser(r); user != nil {
		userID := user.ID
		return &userID
	}
	return nil
}
