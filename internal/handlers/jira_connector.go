package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

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
	encryption         *sso.SecretEncryption
	capturePayloadsDir string // JIRA_CAPTURE_PAYLOADS (empty disables capture)
	mappingFailuresMu  sync.Mutex
	mappingFailures    map[string]error
	cacheInvalidator   *services.AuthorizationCacheInvalidator
}

func (h *JiraImportHandler) WithAuthorizationCacheInvalidator(invalidator *services.AuthorizationCacheInvalidator) *JiraImportHandler {
	h.cacheInvalidator = invalidator
	return h
}

// NewJiraImportHandler creates a new Jira import handler.
// sessionSecret: session-signing secret (resolved upstream by config.Load).
// capturePayloadsDir: optional directory for request/response capture (empty = disabled).
func NewJiraImportHandler(db database.Database, sessionSecret, capturePayloadsDir string) *JiraImportHandler {
	if sessionSecret == "" {
		slog.Error("NewJiraImportHandler received empty session secret (config wiring bug)", slog.String("component", "jira"))
		panic("config: empty session secret passed to NewJiraImportHandler")
	}
	return &JiraImportHandler{
		db:                 db,
		imports:            jiraimport.New(db),
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
	// InstanceURL + Email render in the connections list, audit log, and
	// warn-level logs; APIToken is a secret and DeploymentType a strict
	// enum — both stay untouched.
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.InstanceURL, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.Email, Policy: sanitize.ShortIdentifier},
	)

	// Determine deployment type (default to cloud)
	deploymentType := jira.DeploymentCloud
	if req.DeploymentType == "datacenter" {
		deploymentType = jira.DeploymentDataCenter
	}

	// Data Center PATs authenticate as Bearer tokens and do not need a
	// username. Jira Cloud still requires the account email for Basic auth.
	if req.InstanceURL == "" || req.APIToken == "" {
		respondValidationError(w, r, "instance_url and api_token are required")
		return
	}
	if deploymentType == jira.DeploymentCloud && req.Email == "" {
		respondValidationError(w, r, "email is required for Jira Cloud")
		return
	}

	// Create Jira client and test connection
	client, err := jira.NewClient(jira.Config{
		InstanceURL:    req.InstanceURL,
		Email:          req.Email,
		APIToken:       req.APIToken,
		DeploymentType: deploymentType,
	})
	if err != nil {
		respondBadRequest(w, r, fmt.Sprintf("Failed to create Jira client: %v", err))
		return
	}

	ctx := r.Context()
	instanceInfo, err := client.TestConnection(ctx)
	if err != nil {
		// Map upstream Jira failures (bad credentials, unreachable host, wrong URL)
		// to 400 — never 401. A Windshift 401 here would trigger the frontend's
		// session-expired interceptor and log the user out for a third-party error.
		//
		// Log the upstream message at warn so operators can see *why* Jira rejected
		// the request (rejected auth scheme, SSO required, invalid token).
		// instance_url + email + deployment go to the log; the API token never does.
		slog.Warn("Jira connection test failed",
			slog.String("component", "jira"),
			slog.String("instance_url", req.InstanceURL),
			slog.String("email", req.Email),
			slog.String("deployment", string(deploymentType)),
			slog.Any("error", err),
		)
		respondBadRequest(w, r, fmt.Sprintf("Jira connection test failed: %v", err))
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
			ResourceType: logger.ResourceJiraImport,
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
	if errors.Is(err, jiraimport.ErrConnectionHasHistory) {
		respondConflict(w, r, "Cannot delete a Jira connection while import jobs reference it. Delete imported data and retain the connection to preserve import provenance.")
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
			ResourceType: logger.ResourceJiraImport,
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
