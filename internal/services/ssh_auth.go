package services

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/database"
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

// SSHUserCredential extends UserCredential with user details
type SSHUserCredential struct {
	models.UserCredential
	Email     string
	Username  string
	FirstName string
	LastName  string
}

// FindUserBySSHKeyWithDetails finds a user by their SSH public key and returns user details
func (s *SSHAuthService) FindUserBySSHKeyWithDetails(publicKeyStr string) (*SSHUserCredential, error) {
	// Normalize the input key (remove extra whitespace and comments)
	normalizedKey := normalizeSSHPublicKey(publicKeyStr)

	// Compute fingerprint for indexed lookup
	fingerprint := ComputeSSHFingerprint(publicKeyStr)

	// Try fingerprint-based lookup first (O(1) with index)
	if fingerprint != "" {
		cred, err := s.findByFingerprint(fingerprint, normalizedKey)
		if err == nil && cred != nil {
			return cred, nil
		}
		// If not found by fingerprint, fall through to full scan for migration period
	}

	// Fallback: full scan for credentials without fingerprint (migration period)
	return s.findByFullScan(normalizedKey)
}

// findByFingerprint attempts to find a credential using the indexed fingerprint column
func (s *SSHAuthService) findByFingerprint(fingerprint, normalizedKey string) (*SSHUserCredential, error) {
	query := `
		SELECT uc.id, uc.user_id, uc.credential_type, uc.credential_name,
		       uc.credential_data, uc.is_active, uc.created_at, uc.updated_at, uc.last_used_at,
		       u.email, u.username, u.first_name, u.last_name
		FROM user_credentials uc
		JOIN users u ON uc.user_id = u.id
		WHERE uc.public_key_fingerprint = ? AND uc.credential_type = 'ssh' AND uc.is_active = true AND u.is_active = true
	`

	var cred SSHUserCredential
	var lastUsedAt sql.NullTime

	err := s.db.QueryRow(query, fingerprint).Scan(
		&cred.ID, &cred.UserID, &cred.CredentialType, &cred.CredentialName,
		&cred.CredentialData, &cred.IsActive, &cred.CreatedAt, &cred.UpdatedAt, &lastUsedAt,
		&cred.Email, &cred.Username, &cred.FirstName, &cred.LastName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query SSH credential by fingerprint: %w", err)
	}

	if lastUsedAt.Valid {
		cred.LastUsedAt = &lastUsedAt.Time
	}

	// Verify the full key matches (fingerprint collision check)
	var credData SSHCredentialData
	if err := json.Unmarshal([]byte(cred.CredentialData), &credData); err != nil {
		return nil, err
	}
	storedKey := normalizeSSHPublicKey(credData.PublicKey)
	if storedKey != normalizedKey {
		return nil, nil
	}

	return &cred, nil
}

// findByFullScan performs a full table scan for credentials without fingerprints (migration period)
func (s *SSHAuthService) findByFullScan(normalizedKey string) (*SSHUserCredential, error) {
	query := `
		SELECT uc.id, uc.user_id, uc.credential_type, uc.credential_name,
		       uc.credential_data, uc.is_active, uc.created_at, uc.updated_at, uc.last_used_at,
		       u.email, u.username, u.first_name, u.last_name
		FROM user_credentials uc
		JOIN users u ON uc.user_id = u.id
		WHERE uc.credential_type = 'ssh' AND uc.is_active = true AND u.is_active = true
		  AND uc.public_key_fingerprint IS NULL
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

		err = rows.Scan(
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
		if err = json.Unmarshal([]byte(cred.CredentialData), &credData); err != nil {
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

	// No match found. Return (nil, nil) so callers can emit the
	// "unauthorized key attempt" audit log instead of treating it as an error.
	return nil, nil
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

// ComputeSSHFingerprint computes the SHA256 fingerprint of an SSH public key
// in standard OpenSSH format: "SHA256:<unpadded-base64>". This matches the
// output of ssh-keygen -lf and golang.org/x/crypto/ssh.FingerprintSHA256.
func ComputeSSHFingerprint(publicKey string) string {
	normalized := normalizeSSHPublicKey(publicKey)
	parts := strings.Fields(normalized)
	if len(parts) < 2 {
		return ""
	}
	keyData, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	hash := sha256.Sum256(keyData)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(hash[:])
}
