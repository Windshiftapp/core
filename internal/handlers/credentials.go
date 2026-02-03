package handlers

import (
    "windshift/internal/database"
    "windshift/internal/models"
    "windshift/internal/services"
    "database/sql"
    "encoding/json"
    "net/http"
    "sort"
    "strconv"
    "strings"
    "time"

)

type CredentialHandler struct {
    db                database.Database
    permissionService *services.PermissionService
}

func NewCredentialHandler(db database.Database, permissionService *services.PermissionService) *CredentialHandler {
	return &CredentialHandler{
		db:                db,
		permissionService: permissionService,
	}
}


// SSHKeyRequest represents the request to add an SSH key
type SSHKeyRequest struct {
	CredentialName string `json:"credential_name"`
	PublicKey      string `json:"public_key"`
}

// GetUserCredentials returns all credentials for a user (both legacy and WebAuthn)
func (h *CredentialHandler) GetUserCredentials(w http.ResponseWriter, r *http.Request) {
    userID, err := strconv.Atoi(r.PathValue("userId"))
    if err != nil {
        respondInvalidID(w, r, "userId")
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
	defer rows.Close()

	for rows.Next() {
		var cred models.UserCredential
		var lastUsedAt sql.NullTime
		var id int

		err := rows.Scan(&id, &cred.UserID, &cred.CredentialType, &cred.CredentialName,
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
	defer webauthnRows.Close()

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
			CredentialType: "fido",  // Mark as FIDO type for UI compatibility
			CredentialName: credentialName,
			IsActive:       true,    // WebAuthn credentials are always active
			CreatedAt:      createdAt,
			UpdatedAt:      createdAt, // Use created_at as updated_at for simplicity
		}

		if lastUsedAt.Valid {
			cred.LastUsedAt = &lastUsedAt.Time
		}

		credentials = append(credentials, cred)
	}

	// Sort all credentials by created_at DESC
	sort.Slice(credentials, func(i, j int) bool {
		return credentials[i].CreatedAt.After(credentials[j].CreatedAt)
	})

	if credentials == nil {
		credentials = []models.UserCredential{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(credentials)
}


// CreateSSHKey adds an SSH public key for a user
func (h *CredentialHandler) CreateSSHKey(w http.ResponseWriter, r *http.Request) {
	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	var req SSHKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
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

	// Return success response
	response := map[string]interface{}{
		"id":              credentialID,
		"credential_type": "ssh",
		"name":            req.CredentialName,
		"created_at":      time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// isValidSSHPublicKey performs basic validation of SSH public key format
func isValidSSHPublicKey(key string) bool {
	parts := strings.Fields(key)
	if len(parts) < 2 {
		return false
	}

	// Check for valid key types
	keyType := parts[0]
	validTypes := []string{
		"ssh-rsa", "ssh-dss", "ssh-ed25519",
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
	userID, err := strconv.Atoi(r.PathValue("userId"))
	if err != nil {
		respondInvalidID(w, r, "userId")
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	credentialIDStr := r.PathValue("credentialId")

	// Try to parse as integer for legacy credentials
	if credentialID, err := strconv.Atoi(credentialIDStr); err == nil {
		// Check if it's a legacy credential
		var exists bool
		err = h.db.QueryRow(`
			SELECT EXISTS(SELECT 1 FROM user_credentials WHERE id = ? AND user_id = ?)
		`, credentialID, userID).Scan(&exists)

		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if exists {
			// Delete from legacy table
			_, err = h.db.ExecWrite(`DELETE FROM user_credentials WHERE id = ? AND user_id = ?`, credentialID, userID)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}

	// Check if it's a WebAuthn credential
	var exists bool
	err = h.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM webauthn_credentials WHERE id = ? AND user_id = ?)
	`, credentialIDStr, userID).Scan(&exists)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if !exists {
		respondNotFound(w, r, "credential")
		return
	}

	// Delete from WebAuthn table
	_, err = h.db.ExecWrite(`DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, credentialIDStr, userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
