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

	"windshift/internal/database"
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

// ErrCredentialPurposeMismatch prevents provider-managed credentials from
// being reused by generic HTTP capabilities or another provider instance.
var ErrCredentialPurposeMismatch = errors.New("action credential purpose mismatch")

type managedCredentialMetadata struct {
	Marker    string `json:"_windshift_managed_credential"`
	ManagedBy string `json:"managed_by"`
	OwnerID   string `json:"owner_id"`
}

const managedCredentialMetadataMarker = "v1"

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
	if isManagedCredentialMetadata(req.SecretMetadata) {
		return nil, errors.New("managed credential metadata is reserved")
	}
	return s.create(req, createdBy)
}

// CreateManaged creates a credential that only its owning provider service can
// resolve or mutate. It is omitted from generic credential management APIs.
func (s *ActionCredentialService) CreateManaged(req models.CreateActionCredentialRequest, createdBy *int, purpose, ownerID string) (*models.ActionCredential, error) {
	if strings.TrimSpace(purpose) == "" || strings.TrimSpace(ownerID) == "" {
		return nil, errors.New("managed credential purpose and owner are required")
	}
	metadata, err := json.Marshal(managedCredentialMetadata{Marker: managedCredentialMetadataMarker, ManagedBy: purpose, OwnerID: ownerID})
	if err != nil {
		return nil, err
	}
	req.SecretMetadata = string(metadata)
	return s.create(req, createdBy)
}

func (s *ActionCredentialService) create(req models.CreateActionCredentialRequest, createdBy *int) (*models.ActionCredential, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("name is required")
	}
	if _, ok := validCredentialTypes[req.CredentialType]; !ok {
		return nil, fmt.Errorf("invalid credential_type: %q", req.CredentialType)
	}
	if strings.TrimSpace(req.Secret) == "" {
		return nil, errors.New("secret is required")
	}
	if err := validateCredentialSecret(req.CredentialType, req.Secret); err != nil {
		return nil, err
	}
	if err := validateSecretMetadata(req.SecretMetadata); err != nil {
		return nil, err
	}
	appliesAll := true
	if req.AppliesToAllWorkspaces != nil {
		appliesAll = *req.AppliesToAllWorkspaces
	}
	workspaceIDs, err := normalizeCredentialWorkspaceIDs(req.WorkspaceIDs)
	if err != nil {
		return nil, err
	}
	if !appliesAll && len(workspaceIDs) == 0 {
		return nil, errors.New("workspace_ids must contain at least one workspace when applies_to_all_workspaces is false")
	}
	if appliesAll {
		workspaceIDs = nil
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
		Name:                   req.Name,
		CredentialType:         req.CredentialType,
		AppliesToAllWorkspaces: appliesAll,
		CreatedBy:              createdBy,
		EncryptedSecret:        ciphertext,
		SecretPrefix:           models.SecretPrefixFor(req.Secret),
		SecretMetadata:         req.SecretMetadata,
		IsEnabled:              enabled,
	}
	if _, err := s.repo.CreateActionCredentialWithWorkspaces(c, workspaceIDs); err != nil {
		return nil, err
	}
	if !appliesAll {
		c.WorkspaceIDs = workspaceIDs
	}
	return c, nil
}

// UpdateMetadata applies metadata-only changes. The plaintext secret is never
// accepted on this path — callers must use Rotate. Scope fields are honored
// when present; passing AppliesToAllWorkspaces=true clears the workspace
// allowlist, passing false with WorkspaceIDs replaces it. Workspace-scoped
// callers should leave both nil so the credential's reach cannot be widened
// from a workspace permission.
func (s *ActionCredentialService) UpdateMetadata(id int, req models.UpdateActionCredentialRequest) (*models.ActionCredential, error) {
	c, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return nil, err
	}
	if isManagedCredential(c) || (req.SecretMetadata != nil && isManagedCredentialMetadata(*req.SecretMetadata)) {
		return nil, repository.ErrNotFound
	}
	return s.updateMetadata(c, req, false)
}

func (s *ActionCredentialService) UpdateManagedMetadata(id int, req models.UpdateActionCredentialRequest, purpose, ownerID string) (*models.ActionCredential, error) {
	c, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return nil, err
	}
	if !credentialMatchesPurpose(c, purpose, ownerID) {
		return nil, ErrCredentialPurposeMismatch
	}
	return s.updateMetadata(c, req, true)
}

// PrepareManagedUpdate applies provider-owned secret and metadata changes and
// returns a rollback closure for the caller's adjacent provider-table update.
// This keeps the credential restorable when the second persistence step fails.
func (s *ActionCredentialService) PrepareManagedUpdate(id int, name string, appliesToAll bool, workspaceIDs []int, secret *string, purpose, ownerID string) (func(database.Tx) error, error) {
	existing, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return nil, err
	}
	if !credentialMatchesPurpose(existing, purpose, ownerID) {
		return nil, ErrCredentialPurposeMismatch
	}
	normalizedWorkspaceIDs, err := normalizeCredentialWorkspaceIDs(workspaceIDs)
	if err != nil {
		return nil, err
	}
	if !appliesToAll && len(normalizedWorkspaceIDs) == 0 {
		return nil, errors.New("workspace_ids must contain at least one workspace when applies_to_all_workspaces is false")
	}
	if appliesToAll {
		normalizedWorkspaceIDs = nil
	}
	existing.Name = name
	existing.AppliesToAllWorkspaces = appliesToAll
	var encryptedSecret, prefix *string
	if secret != nil && strings.TrimSpace(*secret) != "" {
		if err := validateCredentialSecret(existing.CredentialType, *secret); err != nil {
			return nil, err
		}
		ciphertext, err := s.encryption.Encrypt(*secret)
		if err != nil {
			return nil, err
		}
		secretPrefix := models.SecretPrefixFor(*secret)
		encryptedSecret = &ciphertext
		prefix = &secretPrefix
	}
	return func(tx database.Tx) error {
		return s.repo.UpdateManagedCredentialTx(tx, existing, normalizedWorkspaceIDs, encryptedSecret, prefix)
	}, nil
}

// PrepareManagedSecretUpdate encrypts a replacement secret and returns a
// transaction closure that changes only secret material. Long-running OAuth
// operations use this so a stale connection snapshot cannot roll back a
// concurrent credential rename or workspace-scope update.
func (s *ActionCredentialService) PrepareManagedSecretUpdate(id int, secret, purpose, ownerID string) (func(database.Tx) error, error) {
	existing, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return nil, err
	}
	if !credentialMatchesPurpose(existing, purpose, ownerID) {
		return nil, ErrCredentialPurposeMismatch
	}
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("secret is required")
	}
	if err := validateCredentialSecret(existing.CredentialType, secret); err != nil {
		return nil, err
	}
	ciphertext, err := s.encryption.Encrypt(secret)
	if err != nil {
		return nil, fmt.Errorf("encrypt credential: %w", err)
	}
	prefix := models.SecretPrefixFor(secret)
	expectedMetadata := existing.SecretMetadata
	return func(tx database.Tx) error {
		updated, err := s.repo.UpdateManagedCredentialSecretTx(tx, id, expectedMetadata, ciphertext, prefix)
		if err != nil {
			return err
		}
		if !updated {
			return ErrCredentialPurposeMismatch
		}
		return nil
	}, nil
}

func (s *ActionCredentialService) updateMetadata(c *models.ActionCredential, req models.UpdateActionCredentialRequest, preserveManagedMetadata bool) (*models.ActionCredential, error) {
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return nil, errors.New("name cannot be empty")
		}
		c.Name = *req.Name
	}
	if req.SecretMetadata != nil {
		if preserveManagedMetadata {
			return nil, errors.New("managed credential metadata cannot be changed")
		}
		if err := validateSecretMetadata(*req.SecretMetadata); err != nil {
			return nil, err
		}
		c.SecretMetadata = *req.SecretMetadata
	}
	if req.IsEnabled != nil {
		c.IsEnabled = *req.IsEnabled
	}

	// Compute the next scope. If the request doesn't touch scope, keep the
	// existing values. If it does, validate the combo before persisting.
	nextAppliesAll := c.AppliesToAllWorkspaces
	if req.AppliesToAllWorkspaces != nil {
		nextAppliesAll = *req.AppliesToAllWorkspaces
	}
	var nextWorkspaceIDs []int
	var err error
	scopeTouched := req.AppliesToAllWorkspaces != nil || req.WorkspaceIDs != nil
	if scopeTouched {
		if req.WorkspaceIDs != nil {
			nextWorkspaceIDs, err = normalizeCredentialWorkspaceIDs(*req.WorkspaceIDs)
			if err != nil {
				return nil, err
			}
		} else {
			nextWorkspaceIDs = append([]int(nil), c.WorkspaceIDs...)
		}
		if !nextAppliesAll && len(nextWorkspaceIDs) == 0 {
			return nil, errors.New("workspace_ids must contain at least one workspace when applies_to_all_workspaces is false")
		}
		if nextAppliesAll {
			nextWorkspaceIDs = nil
		}
		c.AppliesToAllWorkspaces = nextAppliesAll
	}

	if scopeTouched {
		if err := s.repo.UpdateActionCredentialMetadataWithWorkspaces(c, nextWorkspaceIDs); err != nil {
			return nil, err
		}
		c.WorkspaceIDs = nextWorkspaceIDs
	} else if err := s.repo.UpdateActionCredentialMetadata(c); err != nil {
		return nil, err
	}
	return c, nil
}

// Rotate re-encrypts the secret for an existing credential.
func (s *ActionCredentialService) Rotate(id int, req models.RotateActionCredentialRequest) (*models.ActionCredential, error) {
	existing, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return nil, err
	}
	if isManagedCredential(existing) {
		return nil, repository.ErrNotFound
	}
	return s.rotate(existing, req)
}

func (s *ActionCredentialService) RotateManaged(id int, req models.RotateActionCredentialRequest, purpose, ownerID string) (*models.ActionCredential, error) {
	existing, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return nil, err
	}
	if !credentialMatchesPurpose(existing, purpose, ownerID) {
		return nil, ErrCredentialPurposeMismatch
	}
	return s.rotate(existing, req)
}

func (s *ActionCredentialService) rotate(existing *models.ActionCredential, req models.RotateActionCredentialRequest) (*models.ActionCredential, error) {
	if strings.TrimSpace(req.Secret) == "" {
		return nil, errors.New("secret is required")
	}
	if err := validateCredentialSecret(existing.CredentialType, req.Secret); err != nil {
		return nil, err
	}
	ciphertext, err := s.encryption.Encrypt(req.Secret)
	if err != nil {
		return nil, fmt.Errorf("encrypt credential: %w", err)
	}
	prefix := models.SecretPrefixFor(req.Secret)
	if err := s.repo.RotateActionCredential(existing.ID, ciphertext, prefix); err != nil {
		return nil, err
	}
	existing.EncryptedSecret = ciphertext
	existing.SecretPrefix = prefix
	return existing, nil
}

func validateCredentialSecret(credentialType models.ActionCredentialType, secret string) error {
	if credentialType == models.CredentialBasicAuth && !strings.Contains(secret, ":") {
		return errors.New("basic_auth secret must use username:password format")
	}
	return nil
}

// Delete removes a credential. Callers must enforce permission/scope first.
func (s *ActionCredentialService) Delete(id int) error {
	c, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return err
	}
	if isManagedCredential(c) {
		return repository.ErrNotFound
	}
	return s.repo.DeleteActionCredential(id)
}

func (s *ActionCredentialService) DeleteManaged(id int, purpose, ownerID string) error {
	c, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return err
	}
	if !credentialMatchesPurpose(c, purpose, ownerID) {
		return ErrCredentialPurposeMismatch
	}
	return s.repo.DeleteActionCredential(id)
}

// Get returns a credential record (ciphertext + metadata). The handler is
// responsible for Sanitize() before sending to clients.
func (s *ActionCredentialService) Get(id int) (*models.ActionCredential, error) {
	c, err := s.repo.GetActionCredentialByID(id)
	if err != nil {
		return nil, err
	}
	if isManagedCredential(c) {
		return nil, repository.ErrNotFound
	}
	return c, nil
}

// ListForWorkspace returns credentials usable in the given workspace: those
// that apply to all workspaces, plus those scoped to it via the join table.
// The execution engine uses this to validate that a credential reference is
// in-scope.
func (s *ActionCredentialService) ListForWorkspace(workspaceID int) ([]*models.ActionCredential, error) {
	credentials, err := s.repo.ListActionCredentialsForWorkspace(workspaceID)
	return filterManagedCredentials(credentials), err
}

// ListGlobal returns credentials that apply to all workspaces.
func (s *ActionCredentialService) ListGlobal() ([]*models.ActionCredential, error) {
	credentials, err := s.repo.ListActionCredentialsGlobal()
	return filterManagedCredentials(credentials), err
}

// ListAll returns every credential (system-admin view).
func (s *ActionCredentialService) ListAll() ([]*models.ActionCredential, error) {
	credentials, err := s.repo.ListAllActionCredentials()
	return filterManagedCredentials(credentials), err
}

// Resolve loads a credential and returns the plaintext secret, but only if
// the credential is enabled and in scope for the request:
//   - credentials that apply to all workspaces are usable everywhere
//   - otherwise, only workspaces in the credential's allowlist may resolve it
//
// The plaintext is returned in-band but must not be logged or returned in any
// response body. Resolve is the only path that decrypts.
func (s *ActionCredentialService) Resolve(_ context.Context, credentialID, workspaceID int) (string, *models.ActionCredential, error) {
	c, err := s.repo.GetActionCredentialByID(credentialID)
	if err != nil {
		return "", nil, err
	}
	if isManagedCredential(c) {
		return "", nil, ErrCredentialPurposeMismatch
	}
	return s.resolve(c, workspaceID)
}

func (s *ActionCredentialService) ResolveManaged(_ context.Context, credentialID, workspaceID int, purpose, ownerID string) (string, *models.ActionCredential, error) {
	c, err := s.repo.GetActionCredentialByID(credentialID)
	if err != nil {
		return "", nil, err
	}
	if !credentialMatchesPurpose(c, purpose, ownerID) {
		return "", nil, ErrCredentialPurposeMismatch
	}
	return s.resolve(c, workspaceID)
}

func (s *ActionCredentialService) resolve(c *models.ActionCredential, workspaceID int) (string, *models.ActionCredential, error) {
	if !c.IsEnabled {
		return "", c, ErrCredentialDisabled
	}
	if !c.AppliesToAllWorkspaces {
		ok, err := s.repo.IsCredentialScopedToWorkspace(c.ID, workspaceID)
		if err != nil {
			return "", c, err
		}
		if !ok {
			return "", c, ErrCredentialScopeMismatch
		}
	}
	plaintext, err := s.encryption.Decrypt(c.EncryptedSecret)
	if err != nil {
		return "", c, fmt.Errorf("decrypt credential: %w", err)
	}
	return plaintext, c, nil
}

func isManagedCredentialMetadata(raw string) bool {
	var metadata managedCredentialMetadata
	return json.Unmarshal([]byte(raw), &metadata) == nil &&
		metadata.Marker == managedCredentialMetadataMarker &&
		strings.TrimSpace(metadata.ManagedBy) != "" &&
		strings.TrimSpace(metadata.OwnerID) != ""
}

func isManagedCredential(c *models.ActionCredential) bool {
	return c != nil && isManagedCredentialMetadata(c.SecretMetadata)
}

func credentialMatchesPurpose(c *models.ActionCredential, purpose, ownerID string) bool {
	if c == nil {
		return false
	}
	var metadata managedCredentialMetadata
	if json.Unmarshal([]byte(c.SecretMetadata), &metadata) != nil {
		return false
	}
	return isManagedCredentialMetadata(c.SecretMetadata) &&
		metadata.ManagedBy == purpose && metadata.OwnerID == ownerID &&
		strings.TrimSpace(purpose) != "" && strings.TrimSpace(ownerID) != ""
}

func filterManagedCredentials(credentials []*models.ActionCredential) []*models.ActionCredential {
	if len(credentials) == 0 {
		return credentials
	}
	filtered := credentials[:0]
	for _, credential := range credentials {
		if !isManagedCredential(credential) {
			filtered = append(filtered, credential)
		}
	}
	return filtered
}

// CanCapabilityReference returns whether a given capability scope is allowed
// to reference a credential of the given scope. Used by capability validation.
//
//   - credential applies to all workspaces ⇒ always allowed.
//   - capability applies to all workspaces (capabilityWorkspaceIDs == nil) but
//     credential is restricted ⇒ disallowed: the capability would fail in any
//     workspace not in the credential's allowlist.
//   - both scoped ⇒ the credential's workspace set must be a superset of the
//     capability's, otherwise the capability would fail to resolve in some of
//     the workspaces it runs in.
func CanCapabilityReference(credential *models.ActionCredential, capabilityWorkspaceIDs []int) bool {
	if credential.AppliesToAllWorkspaces {
		return true
	}
	if len(capabilityWorkspaceIDs) == 0 {
		return false
	}
	allowed := make(map[int]struct{}, len(credential.WorkspaceIDs))
	for _, ws := range credential.WorkspaceIDs {
		allowed[ws] = struct{}{}
	}
	for _, ws := range capabilityWorkspaceIDs {
		if _, ok := allowed[ws]; !ok {
			return false
		}
	}
	return true
}

// normalizeCredentialWorkspaceIDs rejects invalid IDs and removes duplicates
// while preserving input order. Silently dropping a zero/negative value can
// hide a broken scope request when other IDs happen to be valid.
func normalizeCredentialWorkspaceIDs(ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, errors.New("workspace_ids must contain positive workspace IDs")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// validateSecretMetadata rejects metadata that's not parsable JSON or that
// contains keys that look like plaintext secrets.
func validateSecretMetadata(metadata string) error {
	return models.ValidateActionCredentialMetadata(metadata)
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
		var hc map[string]any
		if err := json.Unmarshal([]byte(cfg), &hc); err != nil {
			continue
		}
		raw, ok := hc["default_headers"].(map[string]any)
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
	Query(query string, args ...any) (*sql.Rows, error)
}
