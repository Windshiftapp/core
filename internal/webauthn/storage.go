package webauthn

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// CredentialStore handles storage of WebAuthn credentials
type CredentialStore struct {
	db Database
}

// NewCredentialStore creates a new credential store
func NewCredentialStore(db Database) *CredentialStore {
	return &CredentialStore{db: db}
}

// SaveCredential stores a new WebAuthn credential
func (cs *CredentialStore) SaveCredential(userID int, credentialName string, cred *webauthn.Credential) error {
	// Convert credential to database format
	dbCred := FromWebAuthnCredential(userID, credentialName, cred)

	// Convert transport array to JSON
	transportJSON, err := json.Marshal(dbCred.Transport)
	if err != nil {
		return fmt.Errorf("failed to marshal transport: %w", err)
	}

	// Insert into database
	_, err = cs.db.Exec(`
		INSERT INTO webauthn_credentials (
			id, user_id, credential_name, public_key, attestation_type,
			aaguid, sign_count, clone_warning, transport,
			flags_user_present, flags_user_verified,
			flags_backup_eligible, flags_backup_state,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		dbCred.ID, userID, credentialName, dbCred.PublicKey, dbCred.AttestationType,
		dbCred.AAGUID, dbCred.SignCount, dbCred.CloneWarning, transportJSON,
		dbCred.FlagsUserPresent, dbCred.FlagsUserVerified,
		dbCred.FlagsBackupEligible, dbCred.FlagsBackupState,
		time.Now(), time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to save credential: %w", err)
	}

	return nil
}

// GetUserCredentials retrieves all credentials for a user
func (cs *CredentialStore) GetUserCredentials(userID int) ([]webauthn.Credential, error) {
	rows, err := cs.db.Query(`
		SELECT id, public_key, attestation_type, aaguid, sign_count,
		       clone_warning, transport, flags_user_present, flags_user_verified,
		       flags_backup_eligible, flags_backup_state
		FROM webauthn_credentials
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var credentials []webauthn.Credential
	for rows.Next() {
		var credID string
		var publicKey []byte
		var attestationType string
		var aaguid []byte
		var signCount uint32
		var cloneWarning bool
		var transportJSON string
		var flagsUserPresent, flagsUserVerified bool
		var flagsBackupEligible, flagsBackupState bool

		err := rows.Scan(
			&credID, &publicKey, &attestationType, &aaguid, &signCount,
			&cloneWarning, &transportJSON, &flagsUserPresent, &flagsUserVerified,
			&flagsBackupEligible, &flagsBackupState,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan credential: %w", err)
		}

		// Decode credential ID from base64
		credIDBytes, err := base64.RawURLEncoding.DecodeString(credID)
		if err != nil {
			return nil, fmt.Errorf("failed to decode credential ID: %w", err)
		}

		// Parse transport JSON
		var transport []string
		if err := json.Unmarshal([]byte(transportJSON), &transport); err != nil {
			return nil, fmt.Errorf("failed to unmarshal transport: %w", err)
		}

		// Convert to protocol.AuthenticatorTransport
		transports := []protocol.AuthenticatorTransport{}
		for _, t := range transport {
			transports = append(transports, protocol.AuthenticatorTransport(t))
		}

		cred := webauthn.Credential{
			ID:              credIDBytes,
			PublicKey:       publicKey,
			AttestationType: attestationType,
			Transport:       transports,
			Flags: webauthn.CredentialFlags{
				UserPresent:    flagsUserPresent,
				UserVerified:   flagsUserVerified,
				BackupEligible: flagsBackupEligible,
				BackupState:    flagsBackupState,
			},
			Authenticator: webauthn.Authenticator{
				AAGUID:       aaguid,
				SignCount:    signCount,
				CloneWarning: cloneWarning,
			},
		}

		credentials = append(credentials, cred)
	}

	return credentials, nil
}

// GetCredentialByID retrieves a specific credential by its ID
func (cs *CredentialStore) GetCredentialByID(credentialID []byte) (*webauthn.Credential, error) {
	// Encode credential ID to base64 for query
	credIDStr := base64.RawURLEncoding.EncodeToString(credentialID)

	var publicKey []byte
	var attestationType string
	var aaguid []byte
	var signCount uint32
	var cloneWarning bool
	var transportJSON string
	var flagsUserPresent, flagsUserVerified bool
	var flagsBackupEligible, flagsBackupState bool

	err := cs.db.QueryRow(`
		SELECT public_key, attestation_type, aaguid, sign_count,
		       clone_warning, transport, flags_user_present, flags_user_verified,
		       flags_backup_eligible, flags_backup_state
		FROM webauthn_credentials
		WHERE id = ?
	`, credIDStr).Scan(
		&publicKey, &attestationType, &aaguid, &signCount,
		&cloneWarning, &transportJSON, &flagsUserPresent, &flagsUserVerified,
		&flagsBackupEligible, &flagsBackupState,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("credential not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get credential: %w", err)
	}

	// Parse transport JSON
	var transport []string
	if err := json.Unmarshal([]byte(transportJSON), &transport); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transport: %w", err)
	}

	// Convert to protocol.AuthenticatorTransport
	transports := []protocol.AuthenticatorTransport{}
	for _, t := range transport {
		transports = append(transports, protocol.AuthenticatorTransport(t))
	}

	cred := &webauthn.Credential{
		ID:              credentialID,
		PublicKey:       publicKey,
		AttestationType: attestationType,
		Transport:       transports,
		Flags: webauthn.CredentialFlags{
			UserPresent:    flagsUserPresent,
			UserVerified:   flagsUserVerified,
			BackupEligible: flagsBackupEligible,
			BackupState:    flagsBackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:       aaguid,
			SignCount:    signCount,
			CloneWarning: cloneWarning,
		},
	}

	return cred, nil
}

// UpdateCredentialCounter updates the sign count for a credential after successful authentication
func (cs *CredentialStore) UpdateCredentialCounter(credentialID []byte, signCount uint32, cloneWarning bool) error {
	// Encode credential ID to base64 for query
	credIDStr := base64.RawURLEncoding.EncodeToString(credentialID)

	_, err := cs.db.Exec(`
		UPDATE webauthn_credentials
		SET sign_count = ?, clone_warning = ?, last_used_at = ?, updated_at = ?
		WHERE id = ?
	`, signCount, cloneWarning, time.Now(), time.Now(), credIDStr)

	if err != nil {
		return fmt.Errorf("failed to update credential counter: %w", err)
	}

	return nil
}

// DeleteCredential removes a specific credential
func (cs *CredentialStore) DeleteCredential(credentialID string) error {
	_, err := cs.db.Exec(`
		DELETE FROM webauthn_credentials
		WHERE id = ?
	`, credentialID)

	if err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	return nil
}

// DeleteUserCredentials removes all credentials for a user
func (cs *CredentialStore) DeleteUserCredentials(userID int) error {
	_, err := cs.db.Exec(`
		DELETE FROM webauthn_credentials
		WHERE user_id = ?
	`, userID)

	if err != nil {
		return fmt.Errorf("failed to delete user credentials: %w", err)
	}

	return nil
}

// GetUserCredentialsList retrieves credential info for display (without sensitive data)
func (cs *CredentialStore) GetUserCredentialsList(userID int) ([]WebAuthnCredential, error) {
	rows, err := cs.db.Query(`
		SELECT id, user_id, credential_name, attestation_type,
		       aaguid, sign_count, clone_warning, transport,
		       flags_user_present, flags_user_verified,
		       flags_backup_eligible, flags_backup_state,
		       created_at, updated_at, last_used_at
		FROM webauthn_credentials
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var credentials []WebAuthnCredential
	for rows.Next() {
		var cred WebAuthnCredential
		var transportJSON string
		var lastUsedAt sql.NullTime

		err := rows.Scan(
			&cred.ID, &cred.UserID, &cred.CredentialName, &cred.AttestationType,
			&cred.AAGUID, &cred.SignCount, &cred.CloneWarning, &transportJSON,
			&cred.FlagsUserPresent, &cred.FlagsUserVerified,
			&cred.FlagsBackupEligible, &cred.FlagsBackupState,
			&cred.CreatedAt, &cred.UpdatedAt, &lastUsedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan credential: %w", err)
		}

		// Parse transport JSON
		if err := json.Unmarshal([]byte(transportJSON), &cred.Transport); err != nil {
			return nil, fmt.Errorf("failed to unmarshal transport: %w", err)
		}

		if lastUsedAt.Valid {
			lastUsedStr := lastUsedAt.Time.Format(time.RFC3339)
			cred.LastUsedAt = &lastUsedStr
		}

		credentials = append(credentials, cred)
	}

	return credentials, nil
}

// CheckCredentialExists verifies if a credential ID already exists
func (cs *CredentialStore) CheckCredentialExists(credentialID []byte) (bool, error) {
	credIDStr := base64.RawURLEncoding.EncodeToString(credentialID)

	var exists bool
	err := cs.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM webauthn_credentials WHERE id = ?)
	`, credIDStr).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("failed to check credential existence: %w", err)
	}

	return exists, nil
}