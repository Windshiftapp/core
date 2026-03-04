package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"windshift/internal/database"
	"windshift/internal/jira"
	"windshift/internal/logger"
	"windshift/internal/sso"
	"windshift/internal/utils"

	"github.com/google/uuid"
)

// JiraImportHandler handles Jira import endpoints
type JiraImportHandler struct {
	db         database.Database
	encryption *sso.SecretEncryption
}

// NewJiraImportHandler creates a new Jira import handler
func NewJiraImportHandler(db database.Database) *JiraImportHandler {
	// Get server secret for encryption (reuse SSO secret)
	serverSecret := os.Getenv("SSO_SECRET")
	if serverSecret == "" {
		serverSecret = os.Getenv("SESSION_SECRET")
	}
	if serverSecret == "" {
		slog.Error("SSO_SECRET or SESSION_SECRET environment variable must be set for Jira credential encryption", slog.String("component", "jira"))
		os.Exit(1)
	}

	return &JiraImportHandler{
		db:         db,
		encryption: sso.NewSecretEncryption(serverSecret),
	}
}

// Connect handles POST /api/admin/jira-import/connect
func (h *JiraImportHandler) Connect(w http.ResponseWriter, r *http.Request) {
	var req JiraConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if req.InstanceURL == "" || req.Email == "" || req.APIToken == "" {
		respondValidationError(w, r, "instance_url, email, and api_token are required")
		return
	}

	// Determine deployment type (default to cloud)
	deploymentType := jira.DeploymentCloud
	if req.DeploymentType == "datacenter" {
		deploymentType = jira.DeploymentDataCenter
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
		respondUnauthorized(w, r)
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

	_, err = h.db.ExecWrite(`
		INSERT INTO jira_import_connections (id, instance_url, email, encrypted_credentials, instance_name, deployment_type, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, connectionID, req.InstanceURL, req.Email, encryptedToken, instanceInfo.DisplayName, string(deploymentType), userID)
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
			Details: map[string]interface{}{
				"instance_url":    req.InstanceURL,
				"instance_name":   instanceInfo.DisplayName,
				"deployment_type": string(deploymentType),
			},
			Success: true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(JiraConnectResponse{
		ConnectionID: connectionID,
		InstanceInfo: instanceInfo,
	})
}

// GetConnections handles GET /api/admin/jira-import/connections
func (h *JiraImportHandler) GetConnections(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, instance_url, email, instance_name, deployment_type, created_at, last_used_at
		FROM jira_import_connections
		ORDER BY created_at DESC
	`)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to list connections: %w", err))
		return
	}
	defer func() { _ = rows.Close() }()

	connections := make([]ConnectionInfo, 0)
	for rows.Next() {
		var conn ConnectionInfo
		var instanceName sql.NullString
		var deploymentType sql.NullString
		var lastUsedAt sql.NullTime

		if err := rows.Scan(&conn.ID, &conn.InstanceURL, &conn.Email, &instanceName, &deploymentType, &conn.CreatedAt, &lastUsedAt); err != nil {
			slog.Warn("Failed to scan connection", slog.String("component", "jira"), slog.Any("error", err))
			continue
		}
		if instanceName.Valid {
			conn.InstanceName = instanceName.String
		}
		if deploymentType.Valid {
			conn.DeploymentType = deploymentType.String
		} else {
			conn.DeploymentType = "cloud" // Default for existing connections
		}
		if lastUsedAt.Valid {
			conn.LastUsedAt = &lastUsedAt.Time
		}
		connections = append(connections, conn)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(connections)
}

// DeleteConnection handles DELETE /api/admin/jira-import/connections/{connectionId}
func (h *JiraImportHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	connectionID := r.PathValue("connectionId")

	result, err := h.db.ExecWrite(`
		DELETE FROM jira_import_connections WHERE id = ?
	`, connectionID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to delete connection: %w", err))
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "connection")
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
	var instanceURL, email, encryptedCredentials string
	var deploymentTypeStr sql.NullString

	err := h.db.QueryRow(`
		SELECT instance_url, email, encrypted_credentials, deployment_type
		FROM jira_import_connections
		WHERE id = ?
	`, connectionID).Scan(&instanceURL, &email, &encryptedCredentials, &deploymentTypeStr)
	if err != nil {
		return nil, fmt.Errorf("connection not found: %w", err)
	}

	// Update last used timestamp
	if _, err = h.db.ExecWrite(`
		UPDATE jira_import_connections SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?
	`, connectionID); err != nil {
		slog.Warn("failed to update connection last_used_at", slog.String("component", "jira"), slog.String("connection_id", connectionID), slog.Any("error", err))
	}

	// Decrypt the API token
	apiToken, err := h.encryption.Decrypt(encryptedCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	// Determine deployment type (default to cloud for existing connections)
	deploymentType := jira.DeploymentCloud
	if deploymentTypeStr.Valid && deploymentTypeStr.String == "datacenter" {
		deploymentType = jira.DeploymentDataCenter
	}

	return jira.NewClient(jira.Config{
		InstanceURL:    instanceURL,
		Email:          email,
		APIToken:       apiToken,
		DeploymentType: deploymentType,
	})
}

// getUserIDFromContext extracts the user ID from request context
func getUserIDFromContext(r *http.Request) *int {
	if userID, ok := r.Context().Value("user_id").(int); ok {
		return &userID
	}
	return nil
}
