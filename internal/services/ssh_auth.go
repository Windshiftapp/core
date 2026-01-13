package services

import (
	"windshift/internal/database"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"windshift/internal/models"
)

// SSHAuthService handles SSH public key authentication
type SSHAuthService struct {
	db database.Database
}

// NewSSHAuthService creates a new SSH authentication service
func NewSSHAuthService(db database.Database) *SSHAuthService {
	return &SSHAuthService{db: db}
}

// SSHCredentialData represents the structure of SSH credential data stored in the database
type SSHCredentialData struct {
	PublicKey string `json:"public_key"`
	KeyType   string `json:"key_type"`
}

// GetAllActiveSSHCredentials retrieves all active SSH public keys from the database
func (s *SSHAuthService) GetAllActiveSSHCredentials() ([]models.UserCredential, error) {
	query := `
		SELECT uc.id, uc.user_id, uc.credential_type, uc.credential_name, 
		       uc.credential_data, uc.is_active, uc.created_at, uc.updated_at, uc.last_used_at,
		       u.email, u.username, u.first_name, u.last_name
		FROM user_credentials uc
		JOIN users u ON uc.user_id = u.id
		WHERE uc.credential_type = 'ssh' AND uc.is_active = true AND u.is_active = true
		ORDER BY uc.created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query SSH credentials: %w", err)
	}
	defer rows.Close()

	var credentials []models.UserCredential
	for rows.Next() {
		var cred models.UserCredential
		var lastUsedAt sql.NullTime
		var userEmail, username, firstName, lastName string

		err := rows.Scan(
			&cred.ID, &cred.UserID, &cred.CredentialType, &cred.CredentialName,
			&cred.CredentialData, &cred.IsActive, &cred.CreatedAt, &cred.UpdatedAt, &lastUsedAt,
			&userEmail, &username, &firstName, &lastName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan SSH credential: %w", err)
		}

		if lastUsedAt.Valid {
			cred.LastUsedAt = &lastUsedAt.Time
		}

		credentials = append(credentials, cred)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating SSH credentials: %w", err)
	}

	return credentials, nil
}

// SSHUserCredential extends UserCredential with user details
type SSHUserCredential struct {
	models.UserCredential
	Email     string
	Username  string
	FirstName string
	LastName  string
}

// FindUserBySSHKey finds a user by their SSH public key
func (s *SSHAuthService) FindUserBySSHKey(publicKeyStr string) (*models.UserCredential, error) {
	credential, err := s.FindUserBySSHKeyWithDetails(publicKeyStr)
	if err != nil {
		return nil, err
	}
	return &credential.UserCredential, nil
}

// FindUserBySSHKeyWithDetails finds a user by their SSH public key and returns user details
func (s *SSHAuthService) FindUserBySSHKeyWithDetails(publicKeyStr string) (*SSHUserCredential, error) {
	// Normalize the input key (remove extra whitespace and comments)
	normalizedKey := normalizeSSHPublicKey(publicKeyStr)

	query := `
		SELECT uc.id, uc.user_id, uc.credential_type, uc.credential_name, 
		       uc.credential_data, uc.is_active, uc.created_at, uc.updated_at, uc.last_used_at,
		       u.email, u.username, u.first_name, u.last_name
		FROM user_credentials uc
		JOIN users u ON uc.user_id = u.id
		WHERE uc.credential_type = 'ssh' AND uc.is_active = true AND u.is_active = true
		ORDER BY uc.created_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query SSH credentials: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cred SSHUserCredential
		var lastUsedAt sql.NullTime

		err := rows.Scan(
			&cred.ID, &cred.UserID, &cred.CredentialType, &cred.CredentialName,
			&cred.CredentialData, &cred.IsActive, &cred.CreatedAt, &cred.UpdatedAt, &lastUsedAt,
			&cred.Email, &cred.Username, &cred.FirstName, &cred.LastName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan SSH credential: %w", err)
		}

		if lastUsedAt.Valid {
			cred.LastUsedAt = &lastUsedAt.Time
		}

		// Parse the stored credential data
		var credData SSHCredentialData
		if err := json.Unmarshal([]byte(cred.CredentialData), &credData); err != nil {
			// Log credential parsing errors but continue checking other credentials
			continue
		}

		// Normalize the stored key
		storedKey := normalizeSSHPublicKey(credData.PublicKey)

		// Compare normalized keys
		if storedKey == normalizedKey {
			return &cred, nil
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating SSH credentials: %w", err)
	}

	return nil, fmt.Errorf("SSH key not found or not authorized")
}

// IsSSHKeyAuthorized checks if an SSH public key is authorized for any user
func (s *SSHAuthService) IsSSHKeyAuthorized(publicKeyStr string) (bool, *models.UserCredential, error) {
	credential, err := s.FindUserBySSHKey(publicKeyStr)
	if err != nil {
		return false, nil, err
	}

	return credential != nil, credential, nil
}

// UpdateLastUsed updates the last_used_at timestamp for a credential
func (s *SSHAuthService) UpdateLastUsed(credentialID int) error {
	query := `UPDATE user_credentials SET last_used_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.ExecWrite(query, credentialID)
	if err != nil {
		return fmt.Errorf("failed to update last_used_at for credential %d: %w", credentialID, err)
	}
	return nil
}

// normalizeSSHPublicKey normalizes an SSH public key for comparison
func normalizeSSHPublicKey(key string) string {
	// Remove leading/trailing whitespace
	key = strings.TrimSpace(key)
	
	// Split the key into parts (key-type, key-data, optional-comment)
	parts := strings.Fields(key)
	if len(parts) < 2 {
		return ""
	}
	
	// Return just the key type and key data (remove comment)
	return parts[0] + " " + parts[1]
}

// ParseSSHPublicKey parses SSH credential data from JSON
func (s *SSHAuthService) ParseSSHPublicKey(credentialData string) (*SSHCredentialData, error) {
	var data SSHCredentialData
	if err := json.Unmarshal([]byte(credentialData), &data); err != nil {
		return nil, fmt.Errorf("failed to parse SSH credential data: %w", err)
	}
	return &data, nil
}