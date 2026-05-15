package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type CredentialHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	sshEnabled        bool
}

func NewCredentialHandler(db database.Database, permissionService *services.PermissionService, sshEnabled bool) *CredentialHandler {
	return &CredentialHandler{
		db:                db,
		permissionService: permissionService,
		sshEnabled:        sshEnabled,
	}
}

// SSHKeyRequest represents the request to add an SSH key
type SSHKeyRequest struct {
	CredentialName string `json:"credential_name"`
	PublicKey      string `json:"public_key"`
}

// GetUserCredentials returns all credentials for a user (both legacy and WebAuthn)
func (h *CredentialHandler) GetUserCredentials(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	var credentials []models.UserCredential

	// Query legacy credentials from user_credentials table
	legacyQuery := `
		SELECT id, user_id, credential_type, credential_name, is_active, created_at, updated_at, last_used_at
		FROM user_credentials
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	rows, err := h.db.Query(legacyQuery, userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cred models.UserCredential
		var lastUsedAt sql.NullTime
		var id int

		err = rows.Scan(&id, &cred.UserID, &cred.CredentialType, &cred.CredentialName,
			&cred.IsActive, &cred.CreatedAt, &cred.UpdatedAt, &lastUsedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		cred.ID = strconv.Itoa(id) // Convert int ID to string

		if lastUsedAt.Valid {
			cred.LastUsedAt = &lastUsedAt.Time
		}

		credentials = append(credentials, cred)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Query WebAuthn credentials from webauthn_credentials table
	webauthnQuery := `
		SELECT id, credential_name, created_at, last_used_at
		FROM webauthn_credentials
		WHERE user_id = ?
		ORDER BY created_at DESC
	`

	webauthnRows, err := h.db.Query(webauthnQuery, userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = webauthnRows.Close() }()

	for webauthnRows.Next() {
		var id string
		var credentialName string
		var createdAt time.Time
		var lastUsedAt sql.NullTime

		err := webauthnRows.Scan(&id, &credentialName, &createdAt, &lastUsedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Map WebAuthn credential to UserCredential structure
		cred := models.UserCredential{
			ID:             id,
			UserID:         userID,
			CredentialType: "fido", // Mark as FIDO type for UI compatibility
			CredentialName: credentialName,
			IsActive:       true, // WebAuthn credentials are always active
			CreatedAt:      createdAt,
			UpdatedAt:      createdAt, // Use created_at as updated_at for simplicity
		}

		if lastUsedAt.Valid {
			cred.LastUsedAt = &lastUsedAt.Time
		}

		credentials = append(credentials, cred)
	}
	if err := webauthnRows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Sort all credentials by created_at DESC
	sort.Slice(credentials, func(i, j int) bool {
		return credentials[i].CreatedAt.After(credentials[j].CreatedAt)
	})

	if credentials == nil {
		credentials = []models.UserCredential{}
	}

	respondJSONOK(w, credentials)
}

// CreateSSHKey adds an SSH public key for a user
func (h *CredentialHandler) CreateSSHKey(w http.ResponseWriter, r *http.Request) {
	if !h.sshEnabled {
		respondBadRequest(w, r, "SSH is not enabled on this server")
		return
	}

	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	var err error

	currentUser := AuthorizeUserRequest(w, r, userID, h.permissionService)
	if currentUser == nil {
		return
	}

	req, ok := decodeJSON[SSHKeyRequest](w, r)
	if !ok {
		return
	}

	// Validate input
	if req.CredentialName == "" {
		respondValidationError(w, r, "Credential name is required")
		return
	}

	if req.PublicKey == "" {
		respondValidationError(w, r, "Public key is required")
		return
	}

	// Basic SSH public key validation
	req.PublicKey = strings.TrimSpace(req.PublicKey)
	if !isValidSSHPublicKey(req.PublicKey) {
		respondValidationError(w, r, "Invalid SSH public key format")
		return
	}

	// Create credential data
	credentialData := map[string]interface{}{
		"public_key": req.PublicKey,
		"key_type":   getSSHKeyType(req.PublicKey),
	}

	credentialJSON, err := json.Marshal(credentialData)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Compute fingerprint for indexed lookup
	fingerprint := services.ComputeSSHFingerprint(req.PublicKey)

	// Insert into database
	var credentialID int64
	err = h.db.QueryRow(`
		INSERT INTO user_credentials (user_id, credential_type, credential_name, credential_data, public_key_fingerprint)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, userID, "ssh", req.CredentialName, string(credentialJSON), fingerprint).Scan(&credentialID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	credID := int(credentialID)
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionCredentialCreate,
		ResourceType: logger.ResourceCredential,
		ResourceID:   &credID,
		ResourceName: req.CredentialName,
		Details: map[string]interface{}{
			"credential_type": "ssh",
			"target_user_id":  userID,
			"key_type":        getSSHKeyType(req.PublicKey),
		},
		Success: true,
	})

	// Return success response
	response := map[string]interface{}{
		"id":              credentialID,
		"credential_type": "ssh",
		"name":            req.CredentialName,
		"created_at":      time.Now().Format(time.RFC3339),
	}

	respondJSONCreated(w, response)
}

// isValidSSHPublicKey performs basic validation of SSH public key format
func isValidSSHPublicKey(key string) bool {
	parts := strings.Fields(key)
	if len(parts) < 2 {
		return false
	}

	// Check for valid key types. ssh-dss (DSA) is intentionally excluded:
	// OpenSSH deprecated DSA in 7.0 and removed it from defaults in 9.8.
	keyType := parts[0]
	validTypes := []string{
		"ssh-rsa", "ssh-ed25519",
		"ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521",
	}

	for _, validType := range validTypes {
		if keyType == validType {
			return true
		}
	}

	return false
}

// getSSHKeyType extracts the key type from an SSH public key
func getSSHKeyType(key string) string {
	parts := strings.Fields(key)
	if len(parts) > 0 {
		return parts[0]
	}
	return "unknown"
}

// RemoveCredential removes a user credential (handles both legacy and WebAuthn credentials)
func (h *CredentialHandler) RemoveCredential(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	var err error

	currentUser := AuthorizeUserRequest(w, r, userID, h.permissionService)
	if currentUser == nil {
		return
	}

	credentialIDStr := r.PathValue("credentialId")

	// Try to parse as integer for legacy credentials
	var credentialID int
	if credentialID, err = strconv.Atoi(credentialIDStr); err == nil {
		var credType, credName string
		err = h.db.QueryRow(`
			SELECT credential_type, credential_name FROM user_credentials WHERE id = ? AND user_id = ?
		`, credentialID, userID).Scan(&credType, &credName)

		if err == nil {
			// Delete from legacy table
			_, err = h.db.ExecWrite(`DELETE FROM user_credentials WHERE id = ? AND user_id = ?`, credentialID, userID)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}

			_ = logger.LogAudit(h.db, logger.AuditEvent{
				UserID:       currentUser.ID,
				Username:     currentUser.Username,
				IPAddress:    utils.GetClientIP(r),
				UserAgent:    r.UserAgent(),
				ActionType:   logger.ActionCredentialRemove,
				ResourceType: logger.ResourceCredential,
				ResourceID:   &credentialID,
				ResourceName: credName,
				Details: map[string]interface{}{
					"credential_type": credType,
					"target_user_id":  userID,
				},
				Success: true,
			})

			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			respondInternalError(w, r, err)
			return
		}
	}

	// Check if it's a WebAuthn credential
	var credName string
	err = h.db.QueryRow(`
		SELECT credential_name FROM webauthn_credentials WHERE id = ? AND user_id = ?
	`, credentialIDStr, userID).Scan(&credName)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "credential")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete from WebAuthn table
	_, err = h.db.ExecWrite(`DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, credentialIDStr, userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionWebAuthnRemove,
		ResourceType: logger.ResourceWebAuthn,
		ResourceName: credName,
		Details: map[string]interface{}{
			"credential_id":  credentialIDStr,
			"target_user_id": userID,
		},
		Success: true,
	})

	w.WriteHeader(http.StatusNoContent)
}
