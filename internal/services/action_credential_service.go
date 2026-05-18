// Package services provides business-logic services.
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sso"
)

// ActionCredentialService encapsulates encryption + scope-checking around the
// action credential repository. Plaintext secrets exist only inside this
// service's call frames; no other layer should see them.
type ActionCredentialService struct {
	repo       *repository.ActionCredentialRepository
	encryption *sso.SecretEncryption
}

// NewActionCredentialService builds a service bound to the action-credentials
// HKDF realm so ciphertext written here cannot be decrypted by the generic
// SSO encryption (and vice versa).
func NewActionCredentialService(repo *repository.ActionCredentialRepository, serverSecret string) *ActionCredentialService {
	return &ActionCredentialService{
		repo:       repo,
		encryption: sso.NewSecretEncryptionWithInfo(serverSecret, models.ActionCredentialEncryptionInfo),
	}
}

// ErrCredentialScopeMismatch is returned when a credential cannot be used in
// the requested workspace (e.g. workspace-scoped credential referenced from a
// different workspace, or from a global capability).
var ErrCredentialScopeMismatch = errors.New("action credential not in scope")

// ErrCredentialDisabled is returned when a credential row exists but
// is_enabled = false.
var ErrCredentialDisabled = errors.New("action credential disabled")

// validCredentialTypes enumerates the credential_type values the API accepts.
var validCredentialTypes = map[models.ActionCredentialType]struct{}{
	models.CredentialBearerToken:  {},
	models.CredentialAPIKey:       {},
	models.CredentialBasicAuth:    {},
	models.CredentialCustomHeader: {},
}

// Create encrypts the plaintext secret and inserts a new credential row. The
// returned model has EncryptedSecret populated but never plaintext; callers
// should immediately Sanitize() before returning to clients.
func (s *ActionCredentialService) Create(req models.CreateActionCredentialRequest, createdBy *int) (*models.ActionCredential, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("name is required")
	}
	if _, ok := validCredentialTypes[req.CredentialType]; !ok {
		return nil, fmt.Errorf("invalid credential_type: %q", req.CredentialType)
	}
	if strings.TrimSpace(req.Secret) == "" {
		return nil, errors.New("secret is required")
	}
	if err := validateSecretMetadata(req.SecretMetadata); err != nil {
		return nil, err
	}

	ciphertext, err := s.encryption.Encrypt(req.Secret)
	if err != nil {
		return nil, fmt.Errorf("encrypt credential: %w", err)
	}

	enabled := true
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}

	c := &models.ActionCredential{
		Name:            req.Name,
		CredentialType:  req.CredentialType,
		WorkspaceID:     req.WorkspaceID,
		CreatedBy:       createdBy,
		EncryptedSecret: ciphertext,
		SecretPrefix:    models.SecretPrefixFor(req.Secret),
		SecretMetadata:  req.SecretMetadata,
		IsEnabled:       enabled,
	}
	if _, err := s.repo.CreateActionCredential(c); err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateMetadata applies metadata-only changes. The plaintext secret is never
// accepted on this path — callers must use Rotate.
func (s *ActionCredentialService) UpdateMetadata(id int, req models.UpdateActionCredentialRequest) (*models.ActionCredential, error) {
	c, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return nil, errors.New("name cannot be empty")
		}
		c.Name = *req.Name
	}
	if req.SecretMetadata != nil {
		if err := validateSecretMetadata(*req.SecretMetadata); err != nil {
			return nil, err
		}
		c.SecretMetadata = *req.SecretMetadata
	}
	if req.IsEnabled != nil {
		c.IsEnabled = *req.IsEnabled
	}
	if err := s.repo.UpdateActionCredentialMetadata(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Rotate re-encrypts the secret for an existing credential.
func (s *ActionCredentialService) Rotate(id int, req models.RotateActionCredentialRequest) (*models.ActionCredential, error) {
	if strings.TrimSpace(req.Secret) == "" {
		return nil, errors.New("secret is required")
	}
	existing, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return nil, err
	}
	ciphertext, err := s.encryption.Encrypt(req.Secret)
	if err != nil {
		return nil, fmt.Errorf("encrypt credential: %w", err)
	}
	prefix := models.SecretPrefixFor(req.Secret)
	if err := s.repo.RotateActionCredential(id, ciphertext, prefix); err != nil {
		return nil, err
	}
	existing.EncryptedSecret = ciphertext
	existing.SecretPrefix = prefix
	return existing, nil
}

// Delete removes a credential. Callers must enforce permission/scope first.
func (s *ActionCredentialService) Delete(id int) error {
	return s.repo.DeleteActionCredential(id)
}

// Get returns a credential record (ciphertext + metadata). The handler is
// responsible for Sanitize() before sending to clients.
func (s *ActionCredentialService) Get(id int) (*models.ActionCredential, error) {
	return s.repo.GetActionCredentialByID(id)
}

// ListForWorkspace returns credentials available to the given workspace,
// always including globals. The execution engine uses this to validate that
// a credential reference is in-scope.
func (s *ActionCredentialService) ListForWorkspace(workspaceID int) ([]*models.ActionCredential, error) {
	return s.repo.ListActionCredentialsForWorkspace(workspaceID, true)
}

// ListGlobal returns global (workspace_id IS NULL) credentials only.
func (s *ActionCredentialService) ListGlobal() ([]*models.ActionCredential, error) {
	return s.repo.ListActionCredentialsGlobal()
}

// ListAll returns every credential (system-admin view).
func (s *ActionCredentialService) ListAll() ([]*models.ActionCredential, error) {
	return s.repo.ListAllActionCredentials()
}

// Resolve loads a credential and returns the plaintext secret, but only if
// the credential is enabled and in scope for the request:
//   - global credentials (workspace_id IS NULL) are usable everywhere
//   - workspace-scoped credentials may only be resolved from the same workspace
//
// The plaintext is returned in-band but must not be logged or returned in any
// response body. Resolve is the only path that decrypts.
func (s *ActionCredentialService) Resolve(_ context.Context, credentialID, workspaceID int) (string, *models.ActionCredential, error) {
	c, err := s.repo.GetActionCredentialByID(credentialID)
	if err != nil {
		return "", nil, err
	}
	if !c.IsEnabled {
		return "", c, ErrCredentialDisabled
	}
	if c.WorkspaceID != nil && *c.WorkspaceID != workspaceID {
		return "", c, ErrCredentialScopeMismatch
	}
	plaintext, err := s.encryption.Decrypt(c.EncryptedSecret)
	if err != nil {
		return "", c, fmt.Errorf("decrypt credential: %w", err)
	}
	return plaintext, c, nil
}

// CanCapabilityReference returns whether a given capability scope is allowed
// to reference a credential of the given scope. Used by capability validation.
//
//   - capabilityWorkspaceIDs == nil  ⇒ capability applies to all workspaces;
//     only global credentials are referenceable.
//   - capabilityWorkspaceIDs set     ⇒ capability is scoped to those workspaces;
//     global OR a credential scoped to one of those workspaces is allowed.
func CanCapabilityReference(credential *models.ActionCredential, capabilityWorkspaceIDs []int) bool {
	if credential.WorkspaceID == nil {
		return true
	}
	for _, ws := range capabilityWorkspaceIDs {
		if ws == *credential.WorkspaceID {
			return true
		}
	}
	return false
}

// validateSecretMetadata rejects metadata that's not parsable JSON or that
// contains keys that look like plaintext secrets.
func validateSecretMetadata(metadata string) error {
	if metadata == "" {
		return nil
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &parsed); err != nil {
		return fmt.Errorf("secret_metadata must be a JSON object: %w", err)
	}
	for k := range parsed {
		if isSensitiveMetadataKey(k) {
			return fmt.Errorf("secret_metadata must not contain sensitive key %q (use the secret field)", k)
		}
	}
	return nil
}

// ScanLegacyInlineSecrets walks every http_client capability and emits a
// structured warning for any default_headers key whose name is sensitive.
// We never log the value — only the capability ID and header name — so an
// operator gets a clear signal that legacy inline tokens still exist and
// should be migrated to the credential store, without the scanner itself
// becoming a leak vector.
//
// Returns the number of (capability, header) pairs that triggered a
// warning, which is convenient for tests and for the server bootstrap log.
func ScanLegacyInlineSecrets(db scanLegacyDB) int {
	rows, err := db.Query(`
		SELECT id, name, config
		FROM action_capabilities
		WHERE capability_type = 'http_client'
	`)
	if err != nil {
		slog.Warn("action_credentials_migration.scan_failed",
			slog.String("component", "actions"),
			slog.Any("error", err))
		return 0
	}
	defer func() { _ = rows.Close() }()

	hits := 0
	for rows.Next() {
		var id int
		var name, cfg string
		if err := rows.Scan(&id, &name, &cfg); err != nil {
			continue
		}
		var hc map[string]interface{}
		if err := json.Unmarshal([]byte(cfg), &hc); err != nil {
			continue
		}
		raw, ok := hc["default_headers"].(map[string]interface{})
		if !ok {
			continue
		}
		for header := range raw {
			if !models.IsSensitiveHeaderName(header) {
				continue
			}
			hits++
			slog.Warn("action_credentials_migration.legacy_inline_secret",
				slog.String("component", "actions"),
				slog.Int("capability_id", id),
				slog.String("capability_name", name),
				slog.String("header_name", header),
				slog.String("hint", "move to auth.credential_id or secret_header_refs in the capability config"))
		}
	}
	if err := rows.Err(); err != nil {
		slog.Warn("action_credentials_migration.iter_failed",
			slog.String("component", "actions"),
			slog.Any("error", err))
	}
	if hits > 0 {
		slog.Warn("action_credentials_migration.summary",
			slog.String("component", "actions"),
			slog.Int("legacy_inline_secret_count", hits))
	}
	return hits
}

// scanLegacyDB narrows the database.Database interface to the one method
// the scanner needs, so tests can pass a stub.
type scanLegacyDB interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// isSensitiveMetadataKey rejects keys that look like they hold plaintext
// secret material. The credential's secret belongs in encrypted_secret only.
func isSensitiveMetadataKey(k string) bool {
	lk := strings.ToLower(strings.TrimSpace(k))
	if lk == "" {
		return false
	}
	switch lk {
	case "secret", "token", "password", "api_key", "apikey", "authorization", "client_secret", "private_key":
		return true
	}
	if strings.HasSuffix(lk, "_token") || strings.HasSuffix(lk, "_secret") || strings.HasSuffix(lk, "_password") || strings.HasSuffix(lk, "_key") {
		return true
	}
	return false
}
