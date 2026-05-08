package portalwebauthn

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Database is the subset of *database.DB the credential store needs.
type Database interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// CredentialStore handles persistence of portal WebAuthn credentials.
type CredentialStore struct {
	db Database
}

// NewCredentialStore creates a new portal credential store.
func NewCredentialStore(db Database) *CredentialStore { return &CredentialStore{db: db} }

func unmarshalTransports(jsonStr string) ([]protocol.AuthenticatorTransport, error) {
	var transport []string
	if err := json.Unmarshal([]byte(jsonStr), &transport); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transport: %w", err)
	}
	out := make([]protocol.AuthenticatorTransport, len(transport))
	for i, t := range transport {
		out[i] = protocol.AuthenticatorTransport(t)
	}
	return out, nil
}

// SaveCredential persists a freshly-registered credential for a portal customer.
func (cs *CredentialStore) SaveCredential(portalCustomerID int, credentialName string, cred *webauthn.Credential) error {
	dbCred := FromWebAuthnCredential(portalCustomerID, credentialName, cred)
	transportJSON, err := json.Marshal(dbCred.Transport)
	if err != nil {
		return fmt.Errorf("failed to marshal transport: %w", err)
	}
	_, err = cs.db.Exec(`
		INSERT INTO portal_webauthn_credentials (
			id, portal_customer_id, credential_name, public_key, attestation_type,
			aaguid, sign_count, clone_warning, transport,
			flags_user_present, flags_user_verified,
			flags_backup_eligible, flags_backup_state,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		dbCred.ID, portalCustomerID, credentialName, dbCred.PublicKey, dbCred.AttestationType,
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

// GetCustomerCredentials loads every credential belonging to a customer in
// go-webauthn format, suitable for BeginLogin / FinishLogin.
func (cs *CredentialStore) GetCustomerCredentials(portalCustomerID int) ([]webauthn.Credential, error) {
	rows, err := cs.db.Query(`
		SELECT id, public_key, attestation_type, aaguid, sign_count,
		       clone_warning, transport, flags_user_present, flags_user_verified,
		       flags_backup_eligible, flags_backup_state
		FROM portal_webauthn_credentials
		WHERE portal_customer_id = ?
		ORDER BY created_at DESC
	`, portalCustomerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var credentials []webauthn.Credential
	for rows.Next() {
		var (
			credID                                string
			publicKey                             []byte
			attestationType                       string
			aaguid                                []byte
			signCount                             uint32
			cloneWarning                          bool
			transportJSON                         string
			flagsUserPresent, flagsUserVerified   bool
			flagsBackupEligible, flagsBackupState bool
		)
		if err := rows.Scan(
			&credID, &publicKey, &attestationType, &aaguid, &signCount,
			&cloneWarning, &transportJSON, &flagsUserPresent, &flagsUserVerified,
			&flagsBackupEligible, &flagsBackupState,
		); err != nil {
			return nil, fmt.Errorf("failed to scan credential: %w", err)
		}
		credIDBytes, err := base64.RawURLEncoding.DecodeString(credID)
		if err != nil {
			return nil, fmt.Errorf("failed to decode credential ID: %w", err)
		}
		transports, err := unmarshalTransports(transportJSON)
		if err != nil {
			return nil, err
		}
		credentials = append(credentials, webauthn.Credential{
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
		})
	}
	return credentials, nil
}

// LookupCustomerByCredentialID returns the customer who owns a given
// credential ID. Used by the discoverable-login resolver.
func (cs *CredentialStore) LookupCustomerByCredentialID(credentialID []byte) (int, error) {
	credIDStr := base64.RawURLEncoding.EncodeToString(credentialID)
	var customerID int
	err := cs.db.QueryRow(`
		SELECT portal_customer_id FROM portal_webauthn_credentials WHERE id = ?
	`, credIDStr).Scan(&customerID)
	if err != nil {
		return 0, err
	}
	return customerID, nil
}

// UpdateCredentialCounter persists the post-login sign count and clone flag.
func (cs *CredentialStore) UpdateCredentialCounter(credentialID []byte, signCount uint32, cloneWarning bool) error {
	credIDStr := base64.RawURLEncoding.EncodeToString(credentialID)
	_, err := cs.db.Exec(`
		UPDATE portal_webauthn_credentials
		SET sign_count = ?, clone_warning = ?, last_used_at = ?, updated_at = ?
		WHERE id = ?
	`, signCount, cloneWarning, time.Now(), time.Now(), credIDStr)
	if err != nil {
		return fmt.Errorf("failed to update credential counter: %w", err)
	}
	return nil
}

// DeleteCredential removes a credential by its ID.
func (cs *CredentialStore) DeleteCredential(credentialID string) error {
	_, err := cs.db.Exec(`DELETE FROM portal_webauthn_credentials WHERE id = ?`, credentialID)
	if err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}
	return nil
}

// GetCustomerCredentialsList returns display-friendly credential rows.
func (cs *CredentialStore) GetCustomerCredentialsList(portalCustomerID int) ([]Credential, error) {
	rows, err := cs.db.Query(`
		SELECT id, portal_customer_id, credential_name, attestation_type,
		       aaguid, sign_count, clone_warning, transport,
		       flags_user_present, flags_user_verified,
		       flags_backup_eligible, flags_backup_state,
		       created_at, updated_at, last_used_at
		FROM portal_webauthn_credentials
		WHERE portal_customer_id = ?
		ORDER BY created_at DESC
	`, portalCustomerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query credentials: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var credentials []Credential
	for rows.Next() {
		var c Credential
		var transportJSON string
		var lastUsedAt sql.NullTime
		if err := rows.Scan(
			&c.ID, &c.PortalCustomerID, &c.CredentialName, &c.AttestationType,
			&c.AAGUID, &c.SignCount, &c.CloneWarning, &transportJSON,
			&c.FlagsUserPresent, &c.FlagsUserVerified,
			&c.FlagsBackupEligible, &c.FlagsBackupState,
			&c.CreatedAt, &c.UpdatedAt, &lastUsedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan credential: %w", err)
		}
		if err := json.Unmarshal([]byte(transportJSON), &c.Transport); err != nil {
			return nil, fmt.Errorf("failed to unmarshal transport: %w", err)
		}
		if lastUsedAt.Valid {
			s := lastUsedAt.Time.Format(time.RFC3339)
			c.LastUsedAt = &s
		}
		credentials = append(credentials, c)
	}
	return credentials, nil
}

// CheckCredentialExists reports whether a given credential ID is already stored.
func (cs *CredentialStore) CheckCredentialExists(credentialID []byte) (bool, error) {
	credIDStr := base64.RawURLEncoding.EncodeToString(credentialID)
	var exists bool
	err := cs.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM portal_webauthn_credentials WHERE id = ?)
	`, credIDStr).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check credential existence: %w", err)
	}
	return exists, nil
}

// CountCustomerCredentials returns the number of passkeys a customer has
// registered. Used to enforce the per-customer cap and to drive the
// "set up a passkey" banner state.
func (cs *CredentialStore) CountCustomerCredentials(portalCustomerID int) (int, error) {
	var n int
	err := cs.db.QueryRow(`
		SELECT COUNT(*) FROM portal_webauthn_credentials WHERE portal_customer_id = ?
	`, portalCustomerID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("failed to count credentials: %w", err)
	}
	return n, nil
}
