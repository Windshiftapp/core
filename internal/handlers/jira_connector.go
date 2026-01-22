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
	"windshift/internal/sso"

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

// Connect handles POST /api/jira-import/connect
func (h *JiraImportHandler) Connect(w http.ResponseWriter, r *http.Request) {
	var req JiraConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.InstanceURL == "" || req.Email == "" || req.APIToken == "" {
		http.Error(w, "instance_url, email, and api_token are required", http.StatusBadRequest)
		return
	}

	// Create Jira client and test connection
	client, err := jira.NewClient(jira.Config{
		InstanceURL: req.InstanceURL,
		Email:       req.Email,
		APIToken:    req.APIToken,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create Jira client: %v", err), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	instanceInfo, err := client.TestConnection(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to connect to Jira: %v", err), http.StatusUnauthorized)
		return
	}

	// Encrypt the API token
	encryptedToken, err := h.encryption.Encrypt(req.APIToken)
	if err != nil {
		http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
		return
	}

	// Generate connection ID and store in database
	connectionID := uuid.New().String()

	// Get user ID from session
	userID := getUserIDFromContext(r)

	_, err = h.db.ExecWrite(`
		INSERT INTO jira_import_connections (id, instance_url, email, encrypted_credentials, instance_name, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, connectionID, req.InstanceURL, req.Email, encryptedToken, instanceInfo.DisplayName, userID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to store connection: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(JiraConnectResponse{
		ConnectionID: connectionID,
		InstanceInfo: instanceInfo,
	})
}

// GetConnections handles GET /api/jira-import/connections
func (h *JiraImportHandler) GetConnections(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, instance_url, email, instance_name, created_at, last_used_at
		FROM jira_import_connections
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list connections: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	connections := make([]ConnectionInfo, 0)
	for rows.Next() {
		var conn ConnectionInfo
		var instanceName sql.NullString
		var lastUsedAt sql.NullTime

		if err := rows.Scan(&conn.ID, &conn.InstanceURL, &conn.Email, &instanceName, &conn.CreatedAt, &lastUsedAt); err != nil {
			slog.Warn("Failed to scan connection", slog.String("component", "jira"), slog.Any("error", err))
			continue
		}
		if instanceName.Valid {
			conn.InstanceName = instanceName.String
		}
		if lastUsedAt.Valid {
			conn.LastUsedAt = &lastUsedAt.Time
		}
		connections = append(connections, conn)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(connections)
}

// DeleteConnection handles DELETE /api/jira-import/connections/{connectionId}
func (h *JiraImportHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	connectionID := r.PathValue("connectionId")

	result, err := h.db.ExecWrite(`
		DELETE FROM jira_import_connections WHERE id = ?
	`, connectionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete connection: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getClientForConnection retrieves stored credentials and creates a Jira client
func (h *JiraImportHandler) getClientForConnection(ctx context.Context, connectionID string) (jira.Client, error) {
	var instanceURL, email, encryptedCredentials string

	err := h.db.QueryRow(`
		SELECT instance_url, email, encrypted_credentials
		FROM jira_import_connections
		WHERE id = ?
	`, connectionID).Scan(&instanceURL, &email, &encryptedCredentials)
	if err != nil {
		return nil, fmt.Errorf("connection not found: %w", err)
	}

	// Update last used timestamp
	if _, err := h.db.ExecWrite(`
		UPDATE jira_import_connections SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?
	`, connectionID); err != nil {
		slog.Warn("failed to update connection last_used_at", slog.String("component", "jira"), slog.String("connection_id", connectionID), slog.Any("error", err))
	}

	// Decrypt the API token
	apiToken, err := h.encryption.Decrypt(encryptedCredentials)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	return jira.NewClient(jira.Config{
		InstanceURL: instanceURL,
		Email:       email,
		APIToken:    apiToken,
	})
}

// getUserIDFromContext extracts the user ID from request context
func getUserIDFromContext(r *http.Request) *int {
	if userID, ok := r.Context().Value("user_id").(int); ok {
		return &userID
	}
	return nil
}
