package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	coreintegrations "windshift/internal/integrations"
	"windshift/internal/integrations/zammad"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sso"

	"uuid"
)

var zammadFieldNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)
var zammadSlugPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,79}$`)

var ErrZammadReauthorizationRequired = errors.New("zammad OAuth reauthorization is required")
var ErrZammadOAuthSuperseded = errors.New("zammad OAuth operation was superseded by a configuration change")
var ErrZammadOAuthRefreshInProgress = errors.New("zammad OAuth refresh is already in progress")
var ErrZammadLinkReservationConflict = errors.New("zammad connection or item changed while the ticket link was being prepared")
var ErrZammadTicketGroupPolicyChanged = errors.New("zammad connection no longer allows the ticket group")
var ErrZammadConnectionBusy = errors.New("zammad connection has a ticket operation in progress")

const zammadOAuthRefreshLeaseDuration = 2 * zammadHTTPTimeout
const zammadOAuthRefreshWaitDuration = 5 * time.Second
const zammadOAuthRefreshPollInterval = 50 * time.Millisecond
const zammadSyncLeaseDuration = 5 * time.Minute

type ZammadOAuthCallbackResult = coreintegrations.SystemOAuthCallbackResult

type ZammadSyncSummary struct {
	Selected  int `json:"selected"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

// zammadOAuthCredential is encrypted as the secret of the provider-managed
// action credential. It is deliberately not a value that can be sent as an
// Authorization header, including while OAuth is pending.
type zammadOAuthCredential struct {
	Version      int    `json:"version"`
	Status       string `json:"status"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

func pendingZammadOAuthCredential(status string) string {
	payload, _ := json.Marshal(zammadOAuthCredential{Version: 1, Status: status})
	return string(payload)
}

func activeZammadOAuthCredential(accessToken, refreshToken string) (string, error) {
	payload, err := json.Marshal(zammadOAuthCredential{Version: 1, Status: "active", AccessToken: accessToken, RefreshToken: refreshToken})
	return string(payload), err
}

func parseZammadOAuthCredential(raw string) (*zammadOAuthCredential, error) {
	var payload zammadOAuthCredential
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload.Version != 1 || payload.Status != "active" || strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" {
		return nil, ErrZammadReauthorizationRequired
	}
	return &payload, nil
}

type ZammadValidationError struct{ Message string }

func (e *ZammadValidationError) Error() string { return e.Message }

func zammadValidationError(message string) error {
	return &ZammadValidationError{Message: message}
}

type zammadWorkflowTransitioner interface {
	PerformTransition(context.Context, PerformTransitionRequest, *repository.ItemRepository, *ConditionService, transitionApprovalService) (*PerformTransitionResult, error)
}

type zammadPermissionChecker interface {
	HasWorkspacePermission(userID, workspaceID int, permission string) (bool, error)
}

type ZammadService struct {
	db                         database.Database
	repo                       *repository.ZammadRepository
	credentials                *ActionCredentialService
	permission                 zammadPermissionChecker
	workflow                   zammadWorkflowTransitioner
	condition                  *ConditionService
	approval                   *ApprovalService
	events                     *EventCoordinator
	transportOverride          zammad.Transport
	oauthTransportOverride     zammad.Transport
	encryption                 *sso.SecretEncryption
	oauthBeforeRefreshClaim    func()
	updateBeforeLock           func()
	persistBeforeLock          func()
	updateBeforeRemoteWrite    func()
	completionBeforeTransition func()
}

func NewZammadService(db database.Database, repo *repository.ZammadRepository, credentials *ActionCredentialService, permission zammadPermissionChecker, workflow zammadWorkflowTransitioner, condition *ConditionService, approval *ApprovalService) *ZammadService {
	return &ZammadService{
		db: db, repo: repo, credentials: credentials, permission: permission,
		workflow: workflow, condition: condition, approval: approval,
	}
}

func (s *ZammadService) SetEventCoordinator(events *EventCoordinator) {
	s.events = events
}

// SetTransportForTesting replaces the production SSRF-safe transport. Tests
// use it with httptest; production bootstrap never calls this method.
func (s *ZammadService) SetTransportForTesting(transport zammad.Transport) {
	s.transportOverride = transport
}

// SetOAuthEncryption supplies the system secret realm already used by
// integration_providers. It is set during server bootstrap, never exposed.
func (s *ZammadService) SetOAuthEncryption(encryption *sso.SecretEncryption) {
	s.encryption = encryption
}

func (s *ZammadService) SetOAuthTransportForTesting(transport zammad.Transport) {
	s.oauthTransportOverride = transport
}

func (s *ZammadService) SetOAuthBeforeRefreshClaimForTesting(hook func()) {
	s.oauthBeforeRefreshClaim = hook
}

func (s *ZammadService) SetUpdateBeforeConnectionLockForTesting(hook func()) {
	s.updateBeforeLock = hook
}

func (s *ZammadService) SetPersistBeforeConnectionLockForTesting(hook func()) {
	s.persistBeforeLock = hook
}

func (s *ZammadService) SetUpdateBeforeRemoteWriteForTesting(hook func()) {
	s.updateBeforeRemoteWrite = hook
}

func (s *ZammadService) SetCompletionBeforeTransitionForTesting(hook func()) {
	s.completionBeforeTransition = hook
}

func (s *ZammadService) ListConnections() ([]*models.ZammadConnection, error) {
	return s.repo.ListConnections()
}

func (s *ZammadService) ListConnectionsForWorkspace(workspaceID int) ([]*models.ZammadConnection, error) {
	return s.repo.ListConnectionsForWorkspace(workspaceID)
}

func (s *ZammadService) GetConnection(id string) (*models.ZammadConnection, error) {
	return s.repo.GetConnection(id)
}

func (s *ZammadService) CreateConnection(req models.CreateZammadConnectionRequest, actorID int) (*models.ZammadConnection, error) {
	connection, err := validateNewZammadConnection(req, actorID)
	if err != nil {
		return nil, err
	}
	if connection.AuthMethod == models.ZammadAuthMethodOAuth {
		if s.encryption == nil {
			return nil, errors.New("zammad OAuth encryption is not configured")
		}
		connection.OAuthClientSecretEncrypted, err = s.encryption.Encrypt(req.OAuthClientSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypt Zammad OAuth client secret: %w", err)
		}
		credential, err := s.credentials.CreateManaged(models.CreateActionCredentialRequest{
			Name: connection.Name + " Zammad OAuth credentials", CredentialType: models.CredentialCustomHeader,
			Secret: pendingZammadOAuthCredential("pending"), AppliesToAllWorkspaces: boolPointer(connection.AppliesToAllWorkspaces), WorkspaceIDs: connection.WorkspaceIDs,
		}, &actorID, string(models.IntegrationProviderZammad), connection.ProviderID)
		if err != nil {
			return nil, err
		}
		connection.CredentialID = credential.ID
		if err := s.repo.CreateConnection(connection); err != nil {
			_ = s.credentials.DeleteManaged(credential.ID, string(models.IntegrationProviderZammad), connection.ProviderID)
			return nil, err
		}
		return s.repo.GetConnection(connection.ProviderID)
	}
	credential, err := s.credentials.CreateManaged(models.CreateActionCredentialRequest{
		Name:                   connection.Name + " Zammad API token",
		CredentialType:         models.CredentialCustomHeader,
		Secret:                 req.APIToken,
		AppliesToAllWorkspaces: boolPointer(connection.AppliesToAllWorkspaces),
		WorkspaceIDs:           connection.WorkspaceIDs,
	}, &actorID, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		return nil, err
	}
	connection.CredentialID = credential.ID
	connection.HasAPIToken = true
	if err := s.repo.CreateConnection(connection); err != nil {
		_ = s.credentials.DeleteManaged(credential.ID, string(models.IntegrationProviderZammad), connection.ProviderID)
		return nil, err
	}
	return s.repo.GetConnection(connection.ProviderID)
}

func (s *ZammadService) UpdateConnection(id string, req models.UpdateZammadConnectionRequest) (*models.ZammadConnection, error) {
	connection, err := s.repo.GetConnection(id)
	if err != nil {
		return nil, err
	}
	originalBaseURL := connection.BaseURL
	originalOAuthClientID := connection.OAuthClientID
	oauthSecretChanged := req.OAuthClientSecret != nil && strings.TrimSpace(*req.OAuthClientSecret) != ""
	if req.Slug != nil {
		connection.Slug = strings.TrimSpace(*req.Slug)
	}
	if req.Name != nil {
		connection.Name = strings.TrimSpace(*req.Name)
	}
	if req.Enabled != nil {
		connection.Enabled = *req.Enabled
	}
	if req.AuthMethod != nil && *req.AuthMethod != connection.AuthMethod {
		return nil, zammadValidationError("auth_method cannot be changed after a connection is created")
	}
	if req.OAuthClientID != nil {
		connection.OAuthClientID = strings.TrimSpace(*req.OAuthClientID)
	}
	if oauthSecretChanged {
		if s.encryption == nil {
			return nil, errors.New("zammad OAuth encryption is not configured")
		}
		connection.OAuthClientSecretEncrypted, err = s.encryption.Encrypt(*req.OAuthClientSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypt Zammad OAuth client secret: %w", err)
		}
	}
	if req.BaseURL != nil {
		connection.BaseURL, err = NormalizeZammadBaseURL(*req.BaseURL)
		if err != nil {
			return nil, err
		}
	}
	if req.DefaultGroupID != nil {
		connection.DefaultGroupID = *req.DefaultGroupID
	}
	if req.DefaultGroupName != nil {
		connection.DefaultGroupName = strings.TrimSpace(*req.DefaultGroupName)
	}
	if req.AllowedGroups != nil {
		normalizedGroups := normalizeZammadGroupRefs(*req.AllowedGroups)
		if err := validateZammadGroupRefs(*req.AllowedGroups); err != nil && !slices.Equal(normalizedGroups, connection.AllowedGroups) {
			return nil, err
		}
		connection.AllowedGroups = normalizedGroups
	} else if req.AllowedGroupIDs != nil {
		if hasNonPositiveIDs(*req.AllowedGroupIDs) {
			return nil, zammadValidationError("allowed_group_ids must contain positive IDs")
		}
		connection.AllowedGroups = legacyZammadGroupRefs(*req.AllowedGroupIDs, connection.DefaultGroupID, connection.DefaultGroupName)
	}
	syncZammadDefaultGroupName(connection)
	if req.DefaultCustomer != nil {
		connection.DefaultCustomer = strings.TrimSpace(*req.DefaultCustomer)
	}
	if req.CorrelationField != nil {
		connection.CorrelationField = strings.TrimSpace(*req.CorrelationField)
	}
	if req.ClosedStateIDs != nil {
		if hasNonPositiveIDs(*req.ClosedStateIDs) {
			return nil, zammadValidationError("closed_state_ids must contain positive IDs")
		}
		connection.ClosedStateIDs = normalizePositiveIDs(*req.ClosedStateIDs)
	}
	if req.ClearCompletionStatus {
		connection.CompletionStatusID = nil
	} else if req.CompletionStatusID != nil {
		v := *req.CompletionStatusID
		connection.CompletionStatusID = &v
	}
	if req.AppliesToAllWorkspaces != nil {
		connection.AppliesToAllWorkspaces = *req.AppliesToAllWorkspaces
	}
	if req.WorkspaceIDs != nil {
		if hasNonPositiveIDs(*req.WorkspaceIDs) {
			return nil, zammadValidationError("workspace_ids must contain positive IDs")
		}
		connection.WorkspaceIDs = normalizePositiveIDs(*req.WorkspaceIDs)
	}
	if err := validateZammadConnection(connection); err != nil {
		return nil, err
	}
	if connection.AuthMethod == models.ZammadAuthMethodOAuth {
		if err := validateZammadOAuthConfiguration(connection); err != nil {
			return nil, err
		}
		credentialsChanged := connection.BaseURL != originalBaseURL || connection.OAuthClientID != originalOAuthClientID || oauthSecretChanged
		var replacementSecret *string
		if credentialsChanged {
			pending := pendingZammadOAuthCredential("pending")
			replacementSecret = &pending
		}
		credentialUpdate, err := s.credentials.PrepareManagedUpdate(connection.CredentialID,
			connection.Name+" Zammad OAuth credentials", connection.AppliesToAllWorkspaces, connection.WorkspaceIDs, replacementSecret,
			string(models.IntegrationProviderZammad), connection.ProviderID)
		if err != nil {
			return nil, err
		}
		if s.updateBeforeLock != nil {
			s.updateBeforeLock()
		}
		if err := database.WithTx(s.db, func(tx database.Tx) error {
			if err := s.repo.LockConnectionTx(tx, connection.ProviderID); err != nil {
				return err
			}
			if err := s.validateZammadConnectionLinksTx(tx, connection); err != nil {
				return err
			}
			if err := s.repo.UpdateConnectionTx(tx, connection); err != nil {
				return err
			}
			if err := credentialUpdate(tx); err != nil {
				return err
			}
			if replacementSecret != nil {
				return s.repo.ResetOAuthAuthorizationTx(tx, connection.ProviderID)
			}
			return nil
		}); err != nil {
			return nil, err
		}
		return s.repo.GetConnection(id)
	}
	if connection.CredentialID <= 0 {
		return nil, zammadValidationError("API token is not configured")
	}
	credentialUpdate, err := s.credentials.PrepareManagedUpdate(connection.CredentialID,
		connection.Name+" Zammad API token", connection.AppliesToAllWorkspaces,
		connection.WorkspaceIDs, req.APIToken, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		return nil, err
	}
	if s.updateBeforeLock != nil {
		s.updateBeforeLock()
	}
	if err := database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.LockConnectionTx(tx, connection.ProviderID); err != nil {
			return err
		}
		if err := s.validateZammadConnectionLinksTx(tx, connection); err != nil {
			return err
		}
		if err := s.repo.UpdateConnectionTx(tx, connection); err != nil {
			return err
		}
		return credentialUpdate(tx)
	}); err != nil {
		return nil, err
	}
	return s.repo.GetConnection(id)
}

func (s *ZammadService) DeleteConnection(id string) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.LockConnectionTx(tx, id); err != nil {
			return err
		}
		hasLinks, err := s.repo.HasTicketLinksForConnectionTx(tx, id)
		if err != nil {
			return err
		}
		if hasLinks {
			return zammadValidationError("unlink all Zammad tickets before deleting this connection")
		}
		return s.repo.DeleteConnectionTx(tx, id)
	})
}

func (s *ZammadService) validateZammadConnectionLinksTx(tx database.Tx, connection *models.ZammadConnection) error {
	current, err := s.repo.ConnectionMutationSnapshotTx(tx, connection.ProviderID)
	if err != nil {
		return err
	}
	links, err := s.repo.ListTicketLinkScopesForUpdateTx(tx, connection.ProviderID)
	if err != nil {
		return err
	}
	if len(links) > 0 && (connection.BaseURL != current.BaseURL || connection.CorrelationField != current.CorrelationField) {
		return zammadValidationError("base_url and correlation_field cannot change after ticket creation has started")
	}
	groupPolicyChanged := connection.DefaultGroupID != current.DefaultGroupID ||
		connection.DefaultGroupName != current.DefaultGroupName ||
		!slices.Equal(effectiveZammadGroupRefs(connection), effectiveZammadSnapshotGroupRefs(current))
	completionPolicyChanged := !slices.Equal(connection.ClosedStateIDs, current.ClosedStateIDs) ||
		!optionalIntEqual(connection.CompletionStatusID, current.CompletionStatusID)
	groupPolicyNarrows := groupPolicyChanged && zammadGroupPolicyNarrows(current, connection)
	for _, link := range links {
		if (groupPolicyChanged || completionPolicyChanged) && link.SyncLocked {
			return ErrZammadConnectionBusy
		}
		if groupPolicyNarrows && !link.SetupComplete {
			return ErrZammadConnectionBusy
		}
		if !zammadConnectionAllowsWorkspace(connection, link.WorkspaceID) {
			return zammadValidationError("workspace scope cannot exclude items with linked Zammad tickets")
		}
		if !zammadConnectionAllowsGroupSnapshot(connection, link.GroupID, link.GroupName) {
			return zammadValidationError("allowed groups cannot exclude linked Zammad tickets")
		}
	}
	return nil
}

func optionalIntEqual(left, right *int) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func zammadConnectionAllowsWorkspace(connection *models.ZammadConnection, workspaceID int) bool {
	return connection.AppliesToAllWorkspaces || slices.Contains(connection.WorkspaceIDs, workspaceID)
}

func zammadConnectionAllowsGroupSnapshot(connection *models.ZammadConnection, groupID int, groupName string) bool {
	allowedGroups := effectiveZammadGroupRefs(connection)
	if len(allowedGroups) > 0 {
		_, ok := zammadGroupRefByID(allowedGroups, groupID)
		return ok
	}
	if connection.DefaultGroupID > 0 {
		return connection.DefaultGroupID == groupID
	}
	return connection.DefaultGroupName != "" && connection.DefaultGroupName == groupName
}

// zammadGroupPolicyNarrows reports whether proposed can remove a group that an
// incomplete ticket reservation may still need. Numeric group IDs can be
// compared exactly. A name-only configuration has no stable ID to compare, so
// only retaining the identical name-only policy is provably safe.
func zammadGroupPolicyNarrows(current *repository.ZammadConnectionMutationSnapshot, proposed *models.ZammadConnection) bool {
	currentGroups := effectiveZammadSnapshotGroupRefs(current)
	if len(currentGroups) > 0 {
		for _, group := range currentGroups {
			if !zammadConnectionAllowsGroupSnapshot(proposed, group.ID, group.Name) {
				return true
			}
		}
		// The current default, including a name-only default, already resolves
		// to one of these validated IDs. Preserving the entire allowlist is the
		// complete safety condition and avoids treating a pure expansion as a
		// narrowing when the stored default has no numeric ID.
		return false
	}
	if current.DefaultGroupID > 0 {
		return !zammadConnectionAllowsGroupSnapshot(proposed, current.DefaultGroupID, current.DefaultGroupName)
	}
	return len(effectiveZammadGroupRefs(proposed)) > 0 || proposed.DefaultGroupID > 0 || proposed.DefaultGroupName != current.DefaultGroupName
}

func (s *ZammadService) reserveZammadTicketLink(connection *models.ZammadConnection, item *models.Item, link *models.ZammadTicketLink, existing bool) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		snapshot, err := s.repo.LockTicketLinkReservationTx(tx, connection.ProviderID, item.ID)
		if err != nil {
			return err
		}
		if !snapshot.ConnectionEnabled || !snapshot.WorkspaceAllowed ||
			snapshot.BaseURL != connection.BaseURL || snapshot.CorrelationField != connection.CorrelationField ||
			snapshot.DefaultCustomer != connection.DefaultCustomer ||
			snapshot.WorkspaceID != item.WorkspaceID || snapshot.WorkspaceItemNumber != item.WorkspaceItemNumber ||
			snapshot.WorkspaceKey != item.WorkspaceKey ||
			!zammadConnectionAllowsGroupSnapshot(&models.ZammadConnection{
				DefaultGroupID: snapshot.DefaultGroupID, DefaultGroupName: snapshot.DefaultGroupName,
				AllowedGroups: snapshot.AllowedGroups,
			}, link.GroupID, link.GroupName) {
			return ErrZammadLinkReservationConflict
		}
		if existing {
			return s.repo.ReserveExistingTicketLinkTx(tx, link)
		}
		return s.repo.CreatePendingTicketLinkTx(tx, link)
	})
}

// StartOAuth stores a short-lived state bound to this system connection and
// the initiating administrator. The callback URI is deliberately fixed.
func (s *ZammadService) StartOAuth(ctx context.Context, id string, actorID int, publicBaseURL string) (string, error) {
	connection, err := s.repo.GetConnection(id)
	if err != nil {
		return "", err
	}
	if !connection.Enabled {
		return "", zammadValidationError("connection is disabled")
	}
	if connection.AuthMethod != models.ZammadAuthMethodOAuth {
		return "", zammadValidationError("connection does not use OAuth")
	}
	if err := validateZammadOAuthConfiguration(connection); err != nil {
		return "", err
	}
	redirectURI, err := zammadOAuthRedirectURI(publicBaseURL)
	if err != nil {
		return "", err
	}
	state := uuid.New().String() + uuid.New().String()
	if err := s.repo.CreateOAuthState(state, connection.ProviderID, actorID, connection.OAuthGeneration, time.Now().Add(5*time.Minute)); err != nil {
		return "", err
	}
	authorizeURL := connection.BaseURL + "/oauth/authorize?" + url.Values{"response_type": {"code"}, "client_id": {connection.OAuthClientID}, "redirect_uri": {redirectURI}, "scope": {"full"}, "state": {state}}.Encode()
	return authorizeURL, nil
}

func (s *ZammadService) InvalidateOAuthState(state string) error {
	if strings.TrimSpace(state) == "" {
		return nil
	}
	return s.repo.InvalidateOAuthState(state)
}

func (s *ZammadService) ConsumeFailedOAuthCallback(state string) (*ZammadOAuthCallbackResult, error) {
	if strings.TrimSpace(state) == "" {
		return nil, repository.ErrNotFound
	}
	consumed, err := s.repo.ConsumeOAuthState(state)
	if err != nil {
		return nil, err
	}
	if err := s.repo.ClearOAuthAttempt(consumed.ProviderID, consumed.OAuthGeneration, state); err != nil {
		return s.oauthCallbackResult(consumed), err
	}
	return s.oauthCallbackResult(consumed), nil
}

func (s *ZammadService) CompleteOAuth(ctx context.Context, state, code, publicBaseURL string) (*ZammadOAuthCallbackResult, error) {
	redirectURI, err := zammadOAuthRedirectURI(publicBaseURL)
	if err != nil {
		return nil, err
	}
	consumed, err := s.repo.ConsumeOAuthState(state)
	if err != nil {
		return nil, err
	}
	attemptActive := true
	defer func() {
		if attemptActive {
			_ = s.repo.ClearOAuthAttempt(consumed.ProviderID, consumed.OAuthGeneration, state)
		}
	}()
	result := s.oauthCallbackResult(consumed)
	connection, err := s.repo.GetConnection(consumed.ProviderID)
	if err != nil {
		return result, err
	}
	result.ProviderName = connection.Name
	if connection.OAuthGeneration != consumed.OAuthGeneration {
		return result, ErrZammadOAuthSuperseded
	}
	if !connection.Enabled {
		return result, zammadValidationError("connection is disabled")
	}
	if connection.AuthMethod != models.ZammadAuthMethodOAuth {
		return result, zammadValidationError("connection OAuth configuration changed")
	}
	if err := validateZammadOAuthConfiguration(connection); err != nil {
		return result, err
	}
	if s.encryption == nil {
		return result, errors.New("zammad OAuth encryption is not configured")
	}
	secret, err := s.encryption.Decrypt(connection.OAuthClientSecretEncrypted)
	if err != nil {
		return result, errors.New("could not decrypt Zammad OAuth client secret")
	}
	tokens, err := zammad.ExchangeOAuthCode(ctx, s.oauthTransport(connection), connection.BaseURL+"/oauth/token", connection.OAuthClientID, secret, code, redirectURI)
	if err != nil {
		return result, err
	}
	bundle, err := activeZammadOAuthCredential(tokens.AccessToken, tokens.RefreshToken)
	if err != nil {
		return result, err
	}
	credentialUpdate, err := s.credentials.PrepareManagedSecretUpdate(connection.CredentialID, bundle,
		string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		return result, err
	}
	if err := database.WithTx(s.db, func(tx database.Tx) error {
		current, err := s.repo.GuardOAuthCallbackTx(tx, connection.ProviderID, consumed.OAuthGeneration, state)
		if err != nil {
			return err
		}
		if !current {
			return ErrZammadOAuthSuperseded
		}
		if err := credentialUpdate(tx); err != nil {
			return err
		}
		return s.repo.UpsertOAuthTokenTx(tx, repository.ZammadOAuthToken{ProviderID: connection.ProviderID, OAuthGeneration: consumed.OAuthGeneration, ExpiresAt: time.Now().Add(tokens.ExpiresIn)})
	}); err != nil {
		return result, err
	}
	attemptActive = false
	return result, nil
}

func (s *ZammadService) oauthCallbackResult(consumed *repository.ZammadOAuthState) *ZammadOAuthCallbackResult {
	result := &ZammadOAuthCallbackResult{ProviderID: consumed.ProviderID, Generation: consumed.OAuthGeneration}
	initiator, err := repository.NewUserRepository(s.db).GetByID(consumed.InitiatedBy)
	if err != nil {
		initiator = &models.User{ID: consumed.InitiatedBy, Username: "unknown"}
	}
	result.Initiator = initiator
	return result
}

func (s *ZammadService) oauthTransport(connection *models.ZammadConnection) zammad.Transport {
	if s.oauthTransportOverride != nil {
		return s.oauthTransportOverride
	}
	return newZammadSafeTransport(connection.BaseURL, "/oauth/token")
}

func (s *ZammadService) oauthAccessToken(ctx context.Context, connection *models.ZammadConnection, workspaceID int) (string, error) {
	if s.encryption == nil {
		return "", errors.New("zammad OAuth encryption is not configured")
	}
	token, err := s.repo.GetOAuthToken(connection.ProviderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", ErrZammadReauthorizationRequired
		}
		return "", err
	}
	if token.ReauthorizationRequired {
		return "", ErrZammadReauthorizationRequired
	}
	if token.OAuthGeneration != connection.OAuthGeneration {
		return "", ErrZammadOAuthSuperseded
	}
	if token.ExpiresAt.After(time.Now().Add(2 * time.Minute)) {
		return s.resolveStoredZammadOAuthAccessToken(ctx, connection, workspaceID)
	}
	if s.oauthBeforeRefreshClaim != nil {
		s.oauthBeforeRefreshClaim()
	}
	claimProviderID := connection.ProviderID
	claimGeneration := token.OAuthGeneration
	claimOwner := uuid.New().String()
	waitUntil := time.Now().Add(zammadOAuthRefreshWaitDuration)
	for {
		claimed, err := s.repo.ClaimOAuthRefresh(claimProviderID, claimGeneration, claimOwner, time.Now().Add(zammadOAuthRefreshLeaseDuration))
		if err != nil {
			return "", err
		}
		if claimed {
			break
		}

		// Another request owns the per-connection refresh lease. Briefly wait
		// for it to publish the rotated credential, then reuse that token rather
		// than issuing a competing refresh request.
		connection, err = s.repo.GetConnection(claimProviderID)
		if err != nil {
			return "", err
		}
		if connection.OAuthGeneration != claimGeneration {
			return "", ErrZammadOAuthSuperseded
		}
		token, err = s.repo.GetOAuthToken(claimProviderID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return "", ErrZammadReauthorizationRequired
			}
			return "", err
		}
		if token.ReauthorizationRequired {
			return "", ErrZammadReauthorizationRequired
		}
		if token.OAuthGeneration != claimGeneration {
			return "", ErrZammadOAuthSuperseded
		}
		if token.ExpiresAt.After(time.Now().Add(2 * time.Minute)) {
			return s.resolveStoredZammadOAuthAccessToken(ctx, connection, workspaceID)
		}
		if !time.Now().Before(waitUntil) {
			return "", ErrZammadOAuthRefreshInProgress
		}
		timer := time.NewTimer(zammadOAuthRefreshPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	claimActive := true
	defer func() {
		if claimActive {
			_ = s.repo.ReleaseOAuthRefreshClaim(claimProviderID, claimGeneration, claimOwner)
		}
	}()
	connection, err = s.repo.GetConnection(claimProviderID)
	if err != nil {
		return "", err
	}
	if connection.OAuthGeneration != claimGeneration {
		return "", ErrZammadOAuthSuperseded
	}
	token, err = s.repo.GetOAuthTokenForRefreshClaim(claimProviderID, claimGeneration, claimOwner)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return "", ErrZammadOAuthSuperseded
		}
		return "", err
	}
	if token.ReauthorizationRequired {
		return "", ErrZammadReauthorizationRequired
	}
	raw, _, err := s.credentials.ResolveManaged(ctx, connection.CredentialID, workspaceID, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		return "", err
	}
	bundle, err := parseZammadOAuthCredential(raw)
	if err != nil {
		return "", err
	}
	if token.ExpiresAt.After(time.Now().Add(2 * time.Minute)) {
		if err := s.repo.ReleaseOAuthRefreshClaim(claimProviderID, claimGeneration, claimOwner); err != nil {
			return "", err
		}
		claimActive = false
		return bundle.AccessToken, nil
	}
	clientSecret, err := s.encryption.Decrypt(connection.OAuthClientSecretEncrypted)
	if err != nil {
		return "", errors.New("could not decrypt Zammad OAuth client secret")
	}
	refreshed, err := zammad.RefreshOAuthToken(ctx, s.oauthTransport(connection), connection.BaseURL+"/oauth/token", connection.OAuthClientID, clientSecret, bundle.RefreshToken)
	if errors.Is(err, zammad.ErrInvalidGrant) {
		if persistErr := s.markOAuthReauthorizationRequired(connection, connection.OAuthGeneration, claimOwner); persistErr != nil {
			return "", persistErr
		}
		claimActive = false
		return "", ErrZammadReauthorizationRequired
	}
	if err != nil {
		return "", err
	}
	updatedBundle, err := activeZammadOAuthCredential(refreshed.AccessToken, refreshed.RefreshToken)
	if err != nil {
		return "", err
	}
	credentialUpdate, err := s.credentials.PrepareManagedSecretUpdate(connection.CredentialID, updatedBundle,
		string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		return "", err
	}
	if err := database.WithTx(s.db, func(tx database.Tx) error {
		current, err := s.repo.GuardOAuthGenerationTx(tx, connection.ProviderID, connection.OAuthGeneration)
		if err != nil {
			return err
		}
		if !current {
			return ErrZammadOAuthSuperseded
		}
		owned, err := s.repo.GuardOAuthRefreshClaimTx(tx, connection.ProviderID, connection.OAuthGeneration, claimOwner)
		if err != nil {
			return err
		}
		if !owned {
			return ErrZammadOAuthSuperseded
		}
		if err := credentialUpdate(tx); err != nil {
			return err
		}
		return s.repo.UpsertOAuthTokenTx(tx, repository.ZammadOAuthToken{ProviderID: connection.ProviderID, OAuthGeneration: connection.OAuthGeneration, ExpiresAt: time.Now().Add(refreshed.ExpiresIn)})
	}); err != nil {
		return "", err
	}
	claimActive = false
	return refreshed.AccessToken, nil
}

func (s *ZammadService) resolveStoredZammadOAuthAccessToken(ctx context.Context, connection *models.ZammadConnection, workspaceID int) (string, error) {
	raw, _, err := s.credentials.ResolveManaged(ctx, connection.CredentialID, workspaceID, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		return "", err
	}
	bundle, err := parseZammadOAuthCredential(raw)
	if err != nil {
		return "", err
	}
	return bundle.AccessToken, nil
}

func (s *ZammadService) markOAuthReauthorizationRequired(connection *models.ZammadConnection, generation int64, claimOwner string) error {
	pending := pendingZammadOAuthCredential("reauthorization_required")
	credentialUpdate, err := s.credentials.PrepareManagedSecretUpdate(connection.CredentialID, pending,
		string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		return err
	}
	return database.WithTx(s.db, func(tx database.Tx) error {
		current, err := s.repo.GuardOAuthGenerationTx(tx, connection.ProviderID, generation)
		if err != nil {
			return err
		}
		if !current {
			return ErrZammadOAuthSuperseded
		}
		owned, err := s.repo.GuardOAuthRefreshClaimTx(tx, connection.ProviderID, generation, claimOwner)
		if err != nil {
			return err
		}
		if !owned {
			return ErrZammadOAuthSuperseded
		}
		if err := credentialUpdate(tx); err != nil {
			return err
		}
		marked, err := s.repo.MarkOAuthReauthorizationRequiredTx(tx, connection.ProviderID, generation, claimOwner)
		if err != nil {
			return err
		}
		if !marked {
			return ErrZammadOAuthSuperseded
		}
		return nil
	})
}

func (s *ZammadService) TestConnection(ctx context.Context, id string) (*models.ZammadConnectionMetadata, error) {
	connection, err := s.repo.GetConnection(id)
	if err != nil {
		return nil, err
	}
	workspaceID := 0
	if !connection.AppliesToAllWorkspaces && len(connection.WorkspaceIDs) > 0 {
		workspaceID = connection.WorkspaceIDs[0]
	}
	client, err := s.clientForConnection(ctx, connection, workspaceID)
	if err != nil {
		return nil, err
	}
	metadata, err := zammadRuntimeMetadata(ctx, connection, client)
	if err == nil {
		// Least-privilege API-token and OAuth service accounts intentionally need
		// neither admin.object nor admin.group. The configured group catalog and
		// custom field therefore remain explicitly unverified.
		metadata.GroupCatalogVerified = false
		metadata.CorrelationFieldVerified = false
	}
	if err == nil {
		err = validateZammadStates(connection, metadata)
	}
	safeError := ""
	if err != nil {
		safeError = RedactString(err.Error())
	}
	_ = s.repo.SetConnectionTestResult(connection.ProviderID, time.Now(), safeError)
	return metadata, err
}

func (s *ZammadService) MetadataForWorkspace(ctx context.Context, id string, workspaceID int) (*models.ZammadConnectionMetadata, error) {
	connection, client, err := s.client(ctx, id, workspaceID)
	if err != nil {
		return nil, err
	}
	return zammadRuntimeMetadata(ctx, connection, client)
}

func (s *ZammadService) OwnersForWorkspace(ctx context.Context, id string, workspaceID, groupID int) ([]models.ZammadOwner, error) {
	if groupID <= 0 {
		return nil, zammadValidationError("group_id must be positive")
	}
	connection, client, err := s.client(ctx, id, workspaceID)
	if err != nil {
		return nil, err
	}
	if _, err := allowedZammadGroup(connection, groupID); err != nil {
		return nil, err
	}
	owners, err := client.Owners(ctx, groupID)
	if err != nil {
		return nil, err
	}
	result := make([]models.ZammadOwner, 0, len(owners)+1)
	result = append(result, models.ZammadOwner{ID: 1, Name: "Unassigned"})
	for _, owner := range owners {
		if owner.ID != 1 {
			result = append(result, models.ZammadOwner{ID: owner.ID, Name: owner.Name})
		}
	}
	return result, nil
}

func (s *ZammadService) TicketLinksForItem(itemID int) ([]*models.ZammadTicketLink, error) {
	return s.repo.GetTicketLinksForItem(itemID)
}

func (s *ZammadService) GetTicketLink(id string) (*models.ZammadTicketLink, error) {
	return s.repo.GetTicketLink(id)
}

func (s *ZammadService) ResolveTicketLink(correlationKey string) (itemID, workspaceID int, err error) {
	correlationKey = strings.TrimSpace(correlationKey)
	if correlationKey == "" || len(correlationKey) > 512 {
		return 0, 0, repository.ErrNotFound
	}
	providerAndItem, ok := strings.CutPrefix(correlationKey, "windshift:")
	if !ok {
		return 0, 0, repository.ErrNotFound
	}
	providerID, itemKey, ok := strings.Cut(providerAndItem, ":")
	if !ok || providerID == "" || itemKey == "" {
		return 0, 0, repository.ErrNotFound
	}
	return s.repo.GetItemDestinationByCorrelationKey(providerID, correlationKey)
}

// LinkExistingTicket attaches a remote ticket without creating a second one.
// The local reservation is written before the remote correlation field so a
// competing item cannot claim the same provider/ticket pair.
func (s *ZammadService) LinkExistingTicket(ctx context.Context, itemID, actorID int, req models.LinkZammadTicketRequest) (*models.ZammadTicketLink, error) {
	req.ConnectionID = strings.TrimSpace(req.ConnectionID)
	req.TicketNumber = strings.TrimSpace(req.TicketNumber)
	if req.ConnectionID == "" || req.TicketNumber == "" {
		return nil, zammadValidationError("connection_id and ticket_number are required")
	}
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(itemID)
	if err != nil {
		return nil, err
	}
	connection, client, err := s.client(ctx, req.ConnectionID, item.WorkspaceID)
	if err != nil {
		return nil, err
	}
	found, err := client.FindByNumber(ctx, req.TicketNumber)
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, zammadValidationError("Zammad ticket was not found")
	}
	ticket, err := client.GetTicket(ctx, found.ID)
	if err != nil {
		return nil, err
	}
	if ticket.Number != req.TicketNumber {
		return nil, zammadValidationError("Zammad ticket was not found")
	}
	metadata, err := zammadRuntimeMetadata(ctx, connection, client)
	if err != nil {
		return nil, err
	}
	group, err := allowedZammadGroup(connection, ticket.GroupID)
	if err != nil {
		return nil, err
	}
	correlation := fmt.Sprintf("windshift:%s:%s-%d", connection.ProviderID, item.WorkspaceKey, item.WorkspaceItemNumber)
	link, err := s.repo.GetTicketLinkForItem(itemID, connection.ProviderID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if link != nil {
		if link.TicketID != ticket.ID {
			return nil, zammadValidationError("this item already has another Zammad ticket for the selected connection")
		}
		correlation = link.CorrelationKey
		if link.SyncState == models.ZammadSyncLinked && link.ItemIntegrationLinkID != "" {
			return link, nil
		}
	}
	remoteCorrelation, err := zammadTicketAttributeString(ticket, connection.CorrelationField)
	if err != nil {
		return nil, err
	}
	if remoteCorrelation != "" && remoteCorrelation != correlation {
		return nil, zammadValidationError("Zammad ticket is already linked through another correlation key")
	}
	if link == nil {
		statusName := zammadStateName(metadata, ticket.StateID, ticket.StateName)
		groupName := ticket.GroupName
		if groupName == "" {
			groupName = group.Name
		}
		link = &models.ZammadTicketLink{
			ID: uuid.New().String(), ItemID: itemID, ProviderID: connection.ProviderID,
			TicketID: ticket.ID, TicketNumber: ticket.Number,
			TicketURL: connection.BaseURL + "/#ticket/zoom/" + strconv.Itoa(ticket.ID),
			GroupID:   ticket.GroupID, GroupName: groupName,
			OwnerID: ticket.OwnerID, OwnerName: ticket.OwnerName,
			CorrelationKey: correlation, SyncState: models.ZammadSyncCreating,
			LastStatusID: ticket.StateID, LastStatusName: statusName, CreatedBy: &actorID,
		}
		if err := s.reserveZammadTicketLink(connection, item, link, true); err != nil {
			if !errors.Is(err, repository.ErrDuplicateEntry) {
				return nil, err
			}
			existing, getErr := s.repo.GetTicketLinkForItem(itemID, connection.ProviderID)
			if getErr != nil || existing.TicketID != ticket.ID {
				return nil, err
			}
			link = existing
		}
	}
	syncOwner := uuid.New().String()
	claimed, err := s.repo.ClaimSync(link.ID, syncOwner, time.Now().Add(zammadSyncLeaseDuration))
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrZammadConnectionBusy
	}
	defer func() { _ = s.repo.ReleaseSyncClaim(link.ID, syncOwner) }()
	link, err = s.repo.GetTicketLink(link.ID)
	if err != nil {
		return nil, err
	}
	if link.SyncState == models.ZammadSyncLinked && link.ItemIntegrationLinkID != "" {
		return link, nil
	}
	if link.TicketID <= 0 || link.TicketID != ticket.ID {
		return nil, zammadValidationError("this item already has another Zammad ticket for the selected connection")
	}
	// Reservation and claim may have waited behind another request. Reload the
	// current connection and remote ticket before deciding whether to write the
	// correlation field.
	connection, client, err = s.client(ctx, link.ProviderID, item.WorkspaceID)
	if err != nil {
		return nil, err
	}
	ticket, err = client.GetTicket(ctx, link.TicketID)
	if err != nil {
		return nil, err
	}
	metadata, err = zammadRuntimeMetadata(ctx, connection, client)
	if err != nil {
		return nil, err
	}
	group, err = allowedZammadGroup(connection, ticket.GroupID)
	if err != nil {
		return nil, err
	}
	remoteCorrelation, err = zammadTicketAttributeString(ticket, connection.CorrelationField)
	if err != nil {
		return nil, err
	}
	correlation = link.CorrelationKey
	if remoteCorrelation != "" && remoteCorrelation != correlation {
		return nil, zammadValidationError("Zammad ticket is already linked through another correlation key")
	}

	if remoteCorrelation == "" {
		if err := s.renewZammadSyncClaim(link.ID, syncOwner); err != nil {
			return nil, err
		}
		ticket, err = client.UpdateTicket(ctx, ticket.ID, nil, nil, nil, connection.CorrelationField, correlation)
		if err != nil {
			_ = s.repo.MarkTicketLinkSetupError(link.ID, syncOwner, RedactString(err.Error()))
			return nil, err
		}
	}
	link.TicketID = ticket.ID
	link.TicketNumber = ticket.Number
	link.TicketURL = connection.BaseURL + "/#ticket/zoom/" + strconv.Itoa(ticket.ID)
	link.GroupID = ticket.GroupID
	link.GroupName = zammadGroupName(metadata, ticket.GroupID, ticket.GroupName)
	link.OwnerID = ticket.OwnerID
	link.OwnerName = resolveZammadOwnerName(ctx, client, ticket)
	link.LastStatusID = ticket.StateID
	link.LastStatusName = zammadStateName(metadata, ticket.StateID, ticket.StateName)
	if err := s.renewZammadSyncClaim(link.ID, syncOwner); err != nil {
		return nil, err
	}
	if err := s.completeExistingTicketLinkWithCurrentGroupPolicy(link, syncOwner, actorID); err != nil {
		_ = s.repo.MarkTicketLinkSetupError(link.ID, syncOwner, RedactString(err.Error()))
		return nil, s.mapZammadSyncClaimError(err)
	}
	return s.repo.GetTicketLink(link.ID)
}

func (s *ZammadService) CreateTicket(ctx context.Context, itemID, actorID int, req models.CreateZammadTicketRequest) (*models.ZammadTicketLink, error) {
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(itemID)
	if err != nil {
		return nil, err
	}
	connection, err := s.connection(req.ConnectionID, item.WorkspaceID)
	if err != nil {
		return nil, err
	}
	var client *zammad.Client
	groupID, groupName := req.GroupID, ""
	if groupID == 0 {
		groupID = connection.DefaultGroupID
	}
	group, err := allowedZammadGroup(connection, groupID)
	if err != nil {
		return nil, err
	}
	groupID, groupName = group.ID, group.Name
	if strings.TrimSpace(groupName) == "" {
		return nil, zammadValidationError("selected Zammad group needs a stored name before tickets can be created")
	}
	correlation := fmt.Sprintf("windshift:%s:%s-%d", connection.ProviderID, item.WorkspaceKey, item.WorkspaceItemNumber)
	link, err := s.repo.GetTicketLinkForItem(itemID, connection.ProviderID)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}
	if link == nil {
		link = &models.ZammadTicketLink{
			ID: uuid.New().String(), ItemID: itemID, ProviderID: connection.ProviderID,
			GroupID: groupID, GroupName: groupName, CorrelationKey: correlation,
			SyncState: models.ZammadSyncPending, CreatedBy: &actorID,
		}
		if err := s.reserveZammadTicketLink(connection, item, link, false); err != nil && !errors.Is(err, repository.ErrDuplicateEntry) {
			return nil, err
		}
		link, err = s.repo.GetTicketLinkForItem(itemID, connection.ProviderID)
		if err != nil {
			return nil, err
		}
	}
	if link.TicketID != 0 {
		return link, nil
	}
	syncOwner := uuid.New().String()
	now := time.Now()
	claimed, wasUncertain, err := s.repo.ClaimTicketCreation(link.ID, syncOwner, now, now.Add(zammadSyncLeaseDuration))
	if err != nil {
		return nil, err
	}
	if !claimed {
		return s.repo.GetTicketLinkForItem(itemID, connection.ProviderID)
	}
	defer func() { _ = s.repo.ReleaseSyncClaim(link.ID, syncOwner) }()
	link, err = s.repo.GetTicketLink(link.ID)
	if err != nil {
		return nil, err
	}
	if link.TicketID != 0 {
		return link, nil
	}
	// Once a durable creation attempt exists, retries keep its original
	// destination and correlation key. This also covers an item moved to a
	// different workspace between attempts.
	groupID = link.GroupID
	correlation = link.CorrelationKey
	connection, client, err = s.client(ctx, link.ProviderID, item.WorkspaceID)
	if err != nil {
		return nil, s.failZammadTicketCreationBeforePost(link.ID, syncOwner, wasUncertain, err)
	}
	if _, err := allowedZammadGroup(connection, groupID); err != nil {
		return nil, s.failZammadTicketCreationBeforePost(link.ID, syncOwner, wasUncertain, err)
	}

	ticket, requestErr := client.FindByCorrelation(ctx, connection.CorrelationField, correlation)
	postAttempted := false
	if requestErr == nil && ticket == nil && wasUncertain {
		if err := s.repo.MarkTicketLinkUncertain(link.ID, syncOwner, "Zammad ticket creation outcome is uncertain; retry only searches by correlation key"); err != nil {
			return nil, s.mapZammadSyncClaimError(err)
		}
		return s.repo.GetTicketLink(link.ID)
	}
	if requestErr == nil && ticket == nil {
		postAttempted = true
		title := truncateRunes(fmt.Sprintf("[%s-%d] %s", item.WorkspaceKey, item.WorkspaceItemNumber, item.Title), 200)
		body := truncateRunes(strings.TrimSpace(item.Description), 20000)
		if body == "" {
			body = title
		}
		if err := s.renewZammadSyncClaim(link.ID, syncOwner); err != nil {
			return nil, err
		}
		ticket, requestErr = client.CreateTicket(ctx, title, body, connection.DefaultCustomer,
			groupID, connection.CorrelationField, correlation)
	}
	if requestErr != nil {
		safeError := RedactString(requestErr.Error())
		var markErr error
		if wasUncertain || (postAttempted && zammadCreationOutcomeUncertain(requestErr)) {
			markErr = s.repo.MarkTicketLinkUncertain(link.ID, syncOwner, safeError)
		} else {
			markErr = s.repo.MarkTicketLinkFailed(link.ID, syncOwner, safeError)
		}
		if markErr != nil {
			return nil, s.mapZammadSyncClaimError(markErr)
		}
		return nil, requestErr
	}
	if ticket.GroupID == 0 {
		ticket, requestErr = client.GetTicket(ctx, ticket.ID)
		if requestErr != nil {
			if err := s.repo.MarkTicketLinkUncertain(link.ID, syncOwner, RedactString(requestErr.Error())); err != nil {
				return nil, s.mapZammadSyncClaimError(err)
			}
			return nil, requestErr
		}
	}
	ticketGroupID := ticket.GroupID
	allowedGroup, groupErr := s.requireAllowedTicketGroup(ctx, connection, client, ticket, nil)
	if groupErr != nil {
		if err := s.repo.MarkTicketLinkUncertain(link.ID, syncOwner, RedactString(groupErr.Error())); err != nil {
			return nil, s.mapZammadSyncClaimError(err)
		}
		return nil, groupErr
	}
	statusName := ticket.StateName
	if statusName == "" {
		statusName = s.resolveStateName(ctx, client, ticket.StateID)
	}
	ticketGroupName := ticket.GroupName
	if ticketGroupName == "" {
		ticketGroupName = allowedGroup.Name
	}
	ticketURL := connection.BaseURL + "/#ticket/zoom/" + fmt.Sprintf("%d", ticket.ID)
	if err := s.renewZammadSyncClaim(link.ID, syncOwner); err != nil {
		return nil, err
	}
	if err := s.completeTicketCreationWithCurrentGroupPolicy(link.ProviderID, link.ID, syncOwner, ticket.ID, ticket.Number, ticketURL,
		ticket.StateID, statusName, ticketGroupID, ticketGroupName, ticket.OwnerID, ticket.OwnerName, actorID); err != nil {
		// The remote ticket is known to exist. Keep retries search-only until
		// the durable local association has been completed.
		if markErr := s.repo.MarkTicketLinkUncertain(link.ID, syncOwner, RedactString(err.Error())); markErr != nil {
			return nil, s.mapZammadSyncClaimError(markErr)
		}
		return nil, s.mapZammadSyncClaimError(err)
	}
	return s.repo.GetTicketLink(link.ID)
}

func validateZammadStates(connection *models.ZammadConnection, metadata *models.ZammadConnectionMetadata) error {
	activeStates := make(map[int]struct{}, len(metadata.States))
	for _, state := range metadata.States {
		activeStates[state.ID] = struct{}{}
	}
	for _, stateID := range connection.ClosedStateIDs {
		if _, ok := activeStates[stateID]; !ok {
			return zammadValidationError(fmt.Sprintf("closed Zammad state %d is missing or inactive", stateID))
		}
	}
	return nil
}

func (s *ZammadService) UpdateTicketLink(ctx context.Context, linkID string, req models.UpdateZammadTicketLinkRequest) (*models.ZammadTicketLink, error) {
	if req.StateID == nil && req.GroupID == nil && req.OwnerID == nil {
		return nil, zammadValidationError("at least one of state_id, group_id, or owner_id is required")
	}
	if (req.StateID != nil && *req.StateID <= 0) || (req.GroupID != nil && *req.GroupID <= 0) || (req.OwnerID != nil && *req.OwnerID <= 0) {
		return nil, zammadValidationError("state_id, group_id, and owner_id must be positive")
	}
	link, err := s.repo.GetTicketLink(linkID)
	if err != nil {
		return nil, err
	}
	if link.TicketID <= 0 || link.ItemIntegrationLinkID == "" {
		return nil, zammadValidationError("Zammad ticket link setup is incomplete")
	}
	syncOwner := uuid.New().String()
	claimed, err := s.repo.ClaimSync(link.ID, syncOwner, time.Now().Add(zammadSyncLeaseDuration))
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrZammadConnectionBusy
	}
	defer func() { _ = s.repo.ReleaseSyncClaim(link.ID, syncOwner) }()
	// The caller's snapshot predates the claim. Reload after winning it so a
	// completed closed-state episode cannot be replayed from stale state.
	link, err = s.repo.GetTicketLink(link.ID)
	if err != nil {
		return nil, err
	}
	if link.TicketID <= 0 || link.ItemIntegrationLinkID == "" {
		return nil, zammadValidationError("Zammad ticket link setup is incomplete")
	}
	item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(link.ItemID)
	if err != nil {
		return nil, err
	}
	connection, client, err := s.client(ctx, link.ProviderID, item.WorkspaceID)
	if err != nil {
		return nil, err
	}
	current, err := client.GetTicket(ctx, link.TicketID)
	if err != nil {
		return nil, err
	}
	metadata, err := zammadRuntimeMetadata(ctx, connection, client)
	if err != nil {
		return nil, err
	}

	effectiveGroupID := current.GroupID
	if req.GroupID != nil {
		effectiveGroupID = *req.GroupID
	}
	if _, err := allowedZammadGroup(connection, effectiveGroupID); err != nil {
		return nil, err
	}
	if req.StateID != nil && !zammadStateExists(metadata, *req.StateID) {
		return nil, zammadValidationError("selected Zammad state is missing or inactive")
	}

	ownerID := req.OwnerID
	if req.GroupID != nil && req.OwnerID == nil && *req.GroupID != current.GroupID {
		// An owner may not have change access in the new group. Zammad's
		// unassigned system user is the deterministic safe default.
		unassigned := 1
		ownerID = &unassigned
	}
	if ownerID != nil && *ownerID != 1 {
		owners, err := client.Owners(ctx, effectiveGroupID)
		if err != nil {
			return nil, err
		}
		if !slices.ContainsFunc(owners, func(owner zammad.Owner) bool { return owner.ID == *ownerID }) {
			return nil, zammadValidationError("selected Zammad owner cannot be assigned tickets in this group")
		}
	}

	if s.updateBeforeRemoteWrite != nil {
		s.updateBeforeRemoteWrite()
	}
	if err := s.renewZammadSyncClaim(link.ID, syncOwner); err != nil {
		return nil, err
	}
	currentConnection, err := s.repo.GetConnection(link.ProviderID)
	if err != nil {
		return nil, err
	}
	effectiveGroupName := zammadGroupName(metadata, effectiveGroupID, current.GroupName)
	if !currentConnection.Enabled || !zammadConnectionAllowsGroupSnapshot(currentConnection, effectiveGroupID, effectiveGroupName) {
		return nil, ErrZammadTicketGroupPolicyChanged
	}
	updated, err := client.UpdateTicket(ctx, link.TicketID, req.StateID, req.GroupID, ownerID, "", "")
	if err != nil {
		return nil, err
	}
	if err := s.persistTicketSnapshot(ctx, link, client, updated, metadata, syncOwner); err != nil {
		return nil, err
	}
	return s.repo.GetTicketLink(link.ID)
}

// UnlinkTicket removes the Windshift association but never deletes the Zammad
// ticket. The remote correlation field is cleared only when it still contains
// this link's exact value. Ambiguous upstream failures keep the local link so
// the user can safely retry.
func (s *ZammadService) UnlinkTicket(ctx context.Context, linkID string) (*models.ZammadTicketLink, error) {
	link, err := s.repo.GetTicketLink(linkID)
	if err != nil {
		return nil, err
	}
	syncOwner := uuid.New().String()
	claimed, err := s.repo.ClaimSync(link.ID, syncOwner, time.Now().Add(zammadSyncLeaseDuration))
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrZammadConnectionBusy
	}
	defer func() { _ = s.repo.ReleaseSyncClaim(link.ID, syncOwner) }()
	link, err = s.repo.GetTicketLink(link.ID)
	if err != nil {
		return nil, err
	}
	needsCorrelationLookup := link.TicketID == 0 &&
		(link.SyncState == models.ZammadSyncCreating || link.SyncState == models.ZammadSyncUncertain)
	if link.TicketID > 0 || needsCorrelationLookup {
		item, err := repository.NewItemRepository(s.db).FindByIDWithDetails(link.ItemID)
		if err != nil {
			return nil, err
		}
		connection, client, err := s.clientForUnlink(ctx, link.ProviderID, item.WorkspaceID)
		if err != nil {
			return nil, err
		}
		var ticket *zammad.Ticket
		var getErr error
		if link.TicketID > 0 {
			ticket, getErr = client.GetTicket(ctx, link.TicketID)
		} else {
			ticket, getErr = client.FindByCorrelation(ctx, connection.CorrelationField, link.CorrelationKey)
		}
		switch {
		case getErr != nil:
			var apiErr *zammad.APIError
			if link.TicketID == 0 || !errors.As(getErr, &apiErr) || apiErr.StatusCode != 404 {
				return nil, getErr
			}
		case ticket == nil:
			// An empty search cannot prove that an ambiguously created ticket does
			// not exist. Keep the pinned correlation so a later search or explicit
			// administrator decision can recover it safely.
			return nil, zammadValidationError("cannot safely unlink an uncertain Zammad ticket while correlation search is empty")
		default:
			remoteCorrelation, err := zammadTicketAttributeString(ticket, connection.CorrelationField)
			if err != nil {
				return nil, err
			}
			if remoteCorrelation == link.CorrelationKey {
				if err := s.renewZammadSyncClaim(link.ID, syncOwner); err != nil {
					return nil, err
				}
				if _, err := client.UpdateTicket(ctx, ticket.ID, nil, nil, nil, connection.CorrelationField, ""); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := s.renewZammadSyncClaim(link.ID, syncOwner); err != nil {
		return nil, err
	}
	if err := s.repo.DeleteTicketLinkClaimed(link.ID, syncOwner); err != nil {
		return nil, s.mapZammadSyncClaimError(err)
	}
	return link, nil
}

// DetachTicketLinkLocally is an explicit administrator recovery path for
// upstream outages or permanently invalid credentials. It removes only the
// local typed and generic links. The remote correlation value may remain in
// Zammad and must be cleared there separately when the upstream system is
// available again.
func (s *ZammadService) DetachTicketLinkLocally(linkID string) (*models.ZammadTicketLink, error) {
	link, err := s.repo.GetTicketLink(linkID)
	if err != nil {
		return nil, err
	}
	syncOwner := uuid.New().String()
	claimed, err := s.repo.ClaimSync(link.ID, syncOwner, time.Now().Add(zammadSyncLeaseDuration))
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrZammadConnectionBusy
	}
	deleted := false
	defer func() {
		if !deleted {
			_ = s.repo.ReleaseSyncClaim(link.ID, syncOwner)
		}
	}()

	link, err = s.repo.GetTicketLink(link.ID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.DeleteTicketLinkClaimed(link.ID, syncOwner); err != nil {
		return nil, s.mapZammadSyncClaimError(err)
	}
	deleted = true
	return link, nil
}

func (s *ZammadService) SyncTicketLink(ctx context.Context, linkID string) (*models.ZammadTicketLink, error) {
	link, err := s.repo.GetTicketLink(linkID)
	if err != nil {
		return nil, err
	}
	if link.TicketID == 0 || link.ItemIntegrationLinkID == "" {
		return nil, zammadValidationError("Zammad ticket link setup is incomplete")
	}
	syncOwner := uuid.New().String()
	claimed, err := s.repo.ClaimSync(link.ID, syncOwner, time.Now().Add(zammadSyncLeaseDuration))
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrZammadConnectionBusy
	}
	return s.syncClaimedTicketLink(ctx, link, syncOwner)
}

// RetryUncertainTicketCreation is an explicit administrator override after
// the remote system has been checked and confirmed not to contain the ticket.
func (s *ZammadService) RetryUncertainTicketCreation(ctx context.Context, linkID string, actorID int) (*models.ZammadTicketLink, error) {
	link, err := s.repo.GetTicketLink(linkID)
	if err != nil {
		return nil, err
	}
	reset, err := s.repo.ResetUncertainTicketCreation(linkID)
	if err != nil {
		return nil, err
	}
	if !reset {
		return nil, zammadValidationError("ticket creation is not awaiting an administrator decision")
	}
	return s.CreateTicket(ctx, link.ItemID, actorID, models.CreateZammadTicketRequest{
		ConnectionID: link.ProviderID,
		GroupID:      link.GroupID,
	})
}

func (s *ZammadService) syncClaimedTicketLink(ctx context.Context, link *models.ZammadTicketLink, syncOwner string) (*models.ZammadTicketLink, error) {
	defer func() { _ = s.repo.ReleaseSyncClaim(link.ID, syncOwner) }()
	// The due-list/manual-refresh snapshot predates the sync claim. Reload after
	// winning the claim so completion_applied and other idempotency state cannot
	// be replayed from a stale worker snapshot.
	current, err := s.repo.GetTicketLink(link.ID)
	if err != nil {
		return nil, err
	}
	link = current
	item, itemErr := repository.NewItemRepository(s.db).FindByID(link.ItemID)
	if itemErr != nil {
		s.recordZammadSyncError(link, syncOwner, itemErr)
		return nil, itemErr
	}
	connection, client, err := s.client(ctx, link.ProviderID, item.WorkspaceID)
	if err != nil {
		if errors.Is(err, ErrZammadOAuthRefreshInProgress) {
			_ = s.repo.ReleaseSyncClaim(link.ID, syncOwner)
			return nil, err
		}
		s.recordZammadSyncError(link, syncOwner, err)
		return nil, err
	}
	if !connection.Enabled {
		if err := s.repo.UpdateTicketLinkSync(link.ID, syncOwner, link.LastStatusID, link.LastStatusName,
			link.GroupID, link.GroupName, link.OwnerID, link.OwnerName, "", time.Now(), false, false); err != nil {
			return nil, s.mapZammadSyncClaimError(err)
		}
		return s.repo.GetTicketLink(link.ID)
	}
	ticket, err := client.GetTicket(ctx, link.TicketID)
	if err != nil {
		s.recordZammadSyncError(link, syncOwner, err)
		return nil, err
	}
	allowedGroup, err := s.requireAllowedTicketGroup(ctx, connection, client, ticket, nil)
	if err != nil {
		s.recordZammadSyncError(link, syncOwner, err)
		return nil, err
	}
	if ticket.GroupName == "" {
		ticket.GroupName = allowedGroup.Name
	}
	var metadata *models.ZammadConnectionMetadata
	needsStateName := ticket.StateName == "" && (ticket.StateID != link.LastStatusID || link.LastStatusName == "")
	needsGroupName := ticket.GroupName == "" && (ticket.GroupID != link.GroupID || link.GroupName == "")
	if needsStateName || needsGroupName {
		metadata, err = zammadRuntimeMetadata(ctx, connection, client)
		if err != nil {
			s.recordZammadSyncError(link, syncOwner, err)
			return nil, err
		}
	}
	if err := s.persistTicketSnapshot(ctx, link, client, ticket, metadata, syncOwner); err != nil {
		return nil, err
	}
	updated, err := s.repo.GetTicketLink(link.ID)
	if err != nil {
		return nil, err
	}
	PublishItemChange(updated.ItemID, ItemChangeZammad)
	return updated, nil
}

func (s *ZammadService) recordZammadSyncError(link *models.ZammadTicketLink, syncOwner string, err error) {
	if updateErr := s.repo.UpdateTicketLinkSync(link.ID, syncOwner, link.LastStatusID, link.LastStatusName,
		link.GroupID, link.GroupName, link.OwnerID, link.OwnerName,
		RedactString(err.Error()), time.Now(), false, false); updateErr == nil {
		PublishItemChange(link.ItemID, ItemChangeZammad)
	}
}

func (s *ZammadService) persistTicketSnapshot(ctx context.Context, link *models.ZammadTicketLink, client *zammad.Client, ticket *zammad.Ticket, metadata *models.ZammadConnectionMetadata, syncOwner string) error {
	if err := s.renewZammadSyncClaim(link.ID, syncOwner); err != nil {
		return err
	}
	// A lease can expire without being taken over. Configuration may have
	// changed during that gap, so reload both the idempotency state and policy
	// only after renewal has made the operation exclusive again.
	currentLink, err := s.repo.GetTicketLink(link.ID)
	if err != nil {
		return err
	}
	link = currentLink
	connection, err := s.repo.GetConnection(link.ProviderID)
	if err != nil {
		return err
	}
	statusName := zammadStateName(metadata, ticket.StateID, ticket.StateName)
	if statusName == "" && ticket.StateID == link.LastStatusID {
		statusName = link.LastStatusName
	}
	groupName := zammadGroupName(metadata, ticket.GroupID, ticket.GroupName)
	if groupName == "" && ticket.GroupID == link.GroupID {
		groupName = link.GroupName
	}
	if !zammadConnectionAllowsGroupSnapshot(connection, ticket.GroupID, groupName) {
		s.recordZammadSyncError(link, syncOwner, ErrZammadTicketGroupPolicyChanged)
		return ErrZammadTicketGroupPolicyChanged
	}
	ownerName := resolveZammadOwnerName(ctx, client, ticket)
	isClosed := slices.Contains(connection.ClosedStateIDs, ticket.StateID)
	completionApplied := false
	if connection.CompletionStatusID != nil && isClosed && !link.CompletionApplied {
		if s.completionBeforeTransition != nil {
			s.completionBeforeTransition()
		}
		// Renew and reload unconditionally at the last boundary before the local
		// side effect. A worker may have been paused long enough for takeover or
		// for policy changes even without the test hook being installed.
		if err := s.renewZammadSyncClaim(link.ID, syncOwner); err != nil {
			return err
		}
		link, err = s.repo.GetTicketLink(link.ID)
		if err != nil {
			return err
		}
		connection, err = s.repo.GetConnection(link.ProviderID)
		if err != nil {
			return err
		}
		if !zammadConnectionAllowsGroupSnapshot(connection, ticket.GroupID, groupName) {
			s.recordZammadSyncError(link, syncOwner, ErrZammadTicketGroupPolicyChanged)
			return ErrZammadTicketGroupPolicyChanged
		}
		isClosed = slices.Contains(connection.ClosedStateIDs, ticket.StateID)
		if connection.CompletionStatusID != nil && isClosed && !link.CompletionApplied {
			completionCommitted, completionErr := s.completeWindshiftItem(ctx, link, connection)
			if completionErr != nil {
				safeError := RedactString(completionErr.Error())
				updateErr := s.updateTicketLinkSyncWithCurrentGroupPolicy(link.ProviderID, link.ID, syncOwner, ticket.StateID, statusName,
					ticket.GroupID, groupName, ticket.OwnerID, ownerName,
					safeError, time.Now(), completionCommitted, completionCommitted)
				if updateErr == nil {
					PublishItemChange(link.ItemID, ItemChangeZammad)
				}
				return completionErr
			}
			completionApplied = true
		}
	}
	setCompletionApplied := !isClosed || completionApplied
	err = s.updateTicketLinkSyncWithCurrentGroupPolicy(link.ProviderID, link.ID, syncOwner, ticket.StateID, statusName,
		ticket.GroupID, groupName, ticket.OwnerID, ownerName,
		"", time.Now(), setCompletionApplied, completionApplied)
	if errors.Is(err, ErrZammadTicketGroupPolicyChanged) {
		s.recordZammadSyncError(link, syncOwner, err)
	}
	return s.mapZammadSyncClaimError(err)
}

func (s *ZammadService) updateTicketLinkSyncWithCurrentGroupPolicy(providerID, linkID, syncOwner string, statusID int, statusName string, groupID int, groupName string, ownerID int, ownerName, safeError string, now time.Time, setCompletionApplied, completionApplied bool) error {
	if s.persistBeforeLock != nil {
		s.persistBeforeLock()
	}
	return database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.LockConnectionTx(tx, providerID); err != nil {
			return err
		}
		if err := s.requireCurrentZammadGroupTx(tx, providerID, groupID, groupName); err != nil {
			return err
		}
		return s.repo.UpdateTicketLinkSyncTx(tx, linkID, syncOwner, statusID, statusName, groupID, groupName,
			ownerID, ownerName, safeError, now, setCompletionApplied, completionApplied)
	})
}

func (s *ZammadService) renewZammadSyncClaim(linkID, syncOwner string) error {
	renewed, err := s.repo.RenewSyncClaim(linkID, syncOwner, time.Now().Add(zammadSyncLeaseDuration))
	if err != nil {
		return s.mapZammadSyncClaimError(err)
	}
	if !renewed {
		return ErrZammadConnectionBusy
	}
	return nil
}

func (s *ZammadService) mapZammadSyncClaimError(err error) error {
	if errors.Is(err, repository.ErrConcurrentUpdate) {
		return ErrZammadConnectionBusy
	}
	return err
}

func (s *ZammadService) failZammadTicketCreationBeforePost(linkID, syncOwner string, wasUncertain bool, cause error) error {
	var err error
	if wasUncertain {
		err = s.repo.MarkTicketLinkUncertain(linkID, syncOwner, RedactString(cause.Error()))
	} else {
		err = s.repo.MarkTicketLinkFailed(linkID, syncOwner, RedactString(cause.Error()))
	}
	if err != nil {
		return s.mapZammadSyncClaimError(err)
	}
	return cause
}

func (s *ZammadService) completeTicketCreationWithCurrentGroupPolicy(providerID, linkID, syncOwner string, ticketID int, number, ticketURL string, statusID int, statusName string, groupID int, groupName string, ownerID int, ownerName string, linkedBy int) error {
	if s.persistBeforeLock != nil {
		s.persistBeforeLock()
	}
	return database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.LockConnectionTx(tx, providerID); err != nil {
			return err
		}
		if err := s.requireCurrentZammadGroupTx(tx, providerID, groupID, groupName); err != nil {
			return err
		}
		return s.repo.CompleteTicketCreationTx(tx, linkID, syncOwner, ticketID, number, ticketURL, statusID, statusName,
			groupID, groupName, ownerID, ownerName, linkedBy)
	})
}

func (s *ZammadService) completeExistingTicketLinkWithCurrentGroupPolicy(link *models.ZammadTicketLink, syncOwner string, linkedBy int) error {
	if s.persistBeforeLock != nil {
		s.persistBeforeLock()
	}
	return database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.LockConnectionTx(tx, link.ProviderID); err != nil {
			return err
		}
		if err := s.requireCurrentZammadGroupTx(tx, link.ProviderID, link.GroupID, link.GroupName); err != nil {
			return err
		}
		return s.repo.CompleteExistingTicketLinkTx(tx, link.ID, syncOwner, link.ID+"-external", link, linkedBy)
	})
}

func (s *ZammadService) requireCurrentZammadGroupTx(tx database.Tx, providerID string, groupID int, groupName string) error {
	defaultGroupID, defaultGroupName, allowedGroups, err := s.repo.ConnectionGroupPolicyTx(tx, providerID)
	if err != nil {
		return err
	}
	if !zammadConnectionAllowsGroupSnapshot(&models.ZammadConnection{
		DefaultGroupID: defaultGroupID, DefaultGroupName: defaultGroupName, AllowedGroups: allowedGroups,
	}, groupID, groupName) {
		return ErrZammadTicketGroupPolicyChanged
	}
	return nil
}

func resolveZammadOwnerName(ctx context.Context, client *zammad.Client, ticket *zammad.Ticket) string {
	if ticket == nil || ticket.OwnerID <= 0 {
		return ""
	}
	if ticket.OwnerName != "" {
		return ticket.OwnerName
	}
	if ticket.OwnerID == 1 {
		return "Unassigned"
	}
	owners, err := client.Owners(ctx, ticket.GroupID)
	if err != nil {
		return ""
	}
	for _, owner := range owners {
		if owner.ID == ticket.OwnerID {
			return owner.Name
		}
	}
	return ""
}

func allowedZammadGroups(connection *models.ZammadConnection) []models.ZammadGroup {
	refs := effectiveZammadGroupRefs(connection)
	if len(refs) == 0 && connection.DefaultGroupID > 0 {
		refs = []models.ZammadGroupRef{{ID: connection.DefaultGroupID, Name: connection.DefaultGroupName}}
	}
	allowed := make([]models.ZammadGroup, 0, len(refs))
	for _, group := range refs {
		allowed = append(allowed, models.ZammadGroup{ID: group.ID, Name: group.Name, Active: true})
	}
	return allowed
}

func allowedZammadGroup(connection *models.ZammadConnection, groupID int) (models.ZammadGroup, error) {
	for _, group := range allowedZammadGroups(connection) {
		if group.ID == groupID {
			return group, nil
		}
	}
	return models.ZammadGroup{}, zammadValidationError("selected Zammad group is missing, inactive, or not allowed for this connection")
}

func (s *ZammadService) requireAllowedTicketGroup(_ context.Context, connection *models.ZammadConnection, _ *zammad.Client, ticket *zammad.Ticket, _ []models.ZammadGroup) (models.ZammadGroup, error) {
	if ticket == nil || ticket.GroupID <= 0 {
		return models.ZammadGroup{}, zammadValidationError("Zammad ticket group is missing or invalid")
	}
	return allowedZammadGroup(connection, ticket.GroupID)
}

func zammadRuntimeMetadata(ctx context.Context, connection *models.ZammadConnection, client *zammad.Client) (*models.ZammadConnectionMetadata, error) {
	states, err := client.States(ctx)
	if err != nil {
		return nil, err
	}
	return &models.ZammadConnectionMetadata{Groups: allowedZammadGroups(connection), States: states}, nil
}

func zammadCreationOutcomeUncertain(err error) bool {
	var upstreamErr *zammad.UpstreamError
	if errors.As(err, &upstreamErr) {
		return true
	}
	var apiErr *zammad.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusRequestTimeout || apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= http.StatusInternalServerError
}

func zammadStateExists(metadata *models.ZammadConnectionMetadata, stateID int) bool {
	return metadata != nil && slices.ContainsFunc(metadata.States, func(state models.ZammadState) bool {
		return state.ID == stateID && state.Active
	})
}

func zammadStateName(metadata *models.ZammadConnectionMetadata, stateID int, fallback string) string {
	if fallback != "" {
		return fallback
	}
	if metadata != nil {
		for _, state := range metadata.States {
			if state.ID == stateID {
				return state.Name
			}
		}
	}
	return ""
}

func zammadGroupName(metadata *models.ZammadConnectionMetadata, groupID int, fallback string) string {
	if fallback != "" {
		return fallback
	}
	if metadata != nil {
		for _, group := range metadata.Groups {
			if group.ID == groupID {
				return group.Name
			}
		}
	}
	return ""
}

func zammadTicketAttributeString(ticket *zammad.Ticket, field string) (string, error) {
	if ticket == nil || ticket.Attributes == nil {
		return "", nil
	}
	raw, ok := ticket.Attributes[field]
	if !ok || string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", &zammad.UpstreamError{Cause: errors.New("zammad correlation field is not a string")}
	}
	return strings.TrimSpace(value), nil
}

func (s *ZammadService) SyncDue(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 50
	}
	// The scheduler interval controls the polling cadence. Applying the same
	// age threshold here caused a freshly synchronized link to miss the next
	// tick and made the effective cadence roughly twice as long.
	links, err := s.repo.ListDueTicketLinks(time.Now(), limit)
	if err != nil {
		return err
	}
	var firstError error
	for _, link := range links {
		syncOwner := uuid.New().String()
		claimed, claimErr := s.repo.ClaimSync(link.ID, syncOwner, time.Now().Add(zammadSyncLeaseDuration))
		if claimErr != nil || !claimed {
			if claimErr != nil && firstError == nil {
				firstError = claimErr
			}
			continue
		}
		if _, syncErr := s.syncClaimedTicketLink(ctx, link, syncOwner); syncErr != nil && firstError == nil {
			firstError = syncErr
		}
	}
	return firstError
}

// SyncAllTicketLinks performs one explicit, system-wide refresh of every
// complete link covered by an enabled, authorized connection. Per-link
// failures are summarized so one broken upstream ticket does not prevent the
// remaining links from being refreshed.
func (s *ZammadService) SyncAllTicketLinks(ctx context.Context) (ZammadSyncSummary, error) {
	links, err := s.repo.ListSyncableTicketLinks()
	if err != nil {
		return ZammadSyncSummary{}, err
	}
	summary := ZammadSyncSummary{Selected: len(links)}
	for _, link := range links {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		syncOwner := uuid.New().String()
		claimed, claimErr := s.repo.ClaimSync(link.ID, syncOwner, time.Now().Add(zammadSyncLeaseDuration))
		if claimErr != nil {
			summary.Failed++
			continue
		}
		if !claimed {
			summary.Skipped++
			continue
		}
		if _, syncErr := s.syncClaimedTicketLink(ctx, link, syncOwner); syncErr != nil {
			summary.Failed++
			continue
		}
		summary.Succeeded++
	}
	return summary, nil
}

func (s *ZammadService) completeWindshiftItem(ctx context.Context, link *models.ZammadTicketLink, connection *models.ZammadConnection) (bool, error) {
	if connection.CompletionStatusID == nil {
		return false, nil
	}
	if connection.CreatedBy == nil {
		return false, errors.New("configured Zammad actor no longer exists")
	}
	itemRepo := repository.NewItemRepository(s.db)
	item, err := itemRepo.FindByIDWithDetails(link.ItemID)
	if err != nil {
		return false, err
	}
	allowed, err := s.permission.HasWorkspacePermission(*connection.CreatedBy, item.WorkspaceID, models.PermissionItemEdit)
	if err != nil {
		return false, err
	}
	if !allowed {
		return false, errors.New("configured Zammad actor no longer has item edit permission")
	}
	result, err := s.workflow.PerformTransition(ctx, PerformTransitionRequest{
		ItemID: link.ItemID, ToStatusID: *connection.CompletionStatusID,
		ActorUserID: *connection.CreatedBy,
		EventMetadata: func() itemevents.Metadata {
			metadata := itemevents.Integration(connection.ProviderID, "zammad_sync")
			metadata.SourceRef = fmt.Sprintf("ticket:%d/link:%s", link.TicketID, link.ID)
			metadata.CorrelationID = link.CorrelationKey
			return metadata
		}(),
	}, itemRepo, s.condition, s.approval)
	if err != nil && result == nil {
		return false, err
	}
	if !result.NoOp && s.events != nil {
		s.events.EmitStatusChanged(result.Item, result.OldStatusID, result.NewStatusID, *connection.CreatedBy, "Zammad")
	}
	return true, err
}

func (s *ZammadService) client(ctx context.Context, id string, workspaceID int) (*models.ZammadConnection, *zammad.Client, error) {
	connection, err := s.connection(id, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	client, err := s.clientForConnection(ctx, connection, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	return connection, client, nil
}

func (s *ZammadService) connection(id string, workspaceID int) (*models.ZammadConnection, error) {
	connection, err := s.repo.GetConnection(id)
	if err != nil {
		return nil, err
	}
	available, err := s.repo.IsConnectionAvailableToWorkspace(id, workspaceID)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, ErrCredentialScopeMismatch
	}
	return connection, nil
}

// clientForUnlink permits cleanup through a disabled connection while keeping
// its workspace and managed-credential boundaries intact.
func (s *ZammadService) clientForUnlink(ctx context.Context, id string, workspaceID int) (*models.ZammadConnection, *zammad.Client, error) {
	connection, err := s.repo.GetConnection(id)
	if err != nil {
		return nil, nil, err
	}
	scoped, err := s.repo.IsConnectionScopedToWorkspace(id, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	if !scoped {
		return nil, nil, ErrCredentialScopeMismatch
	}
	client, err := s.clientForConnection(ctx, connection, workspaceID)
	if err != nil {
		return nil, nil, err
	}
	return connection, client, nil
}

func (s *ZammadService) clientForConnection(ctx context.Context, connection *models.ZammadConnection, workspaceID int) (*zammad.Client, error) {
	transport := s.transportOverride
	if transport == nil {
		transport = newZammadSafeTransport(connection.BaseURL, "/api/v1/")
	}
	if connection.AuthMethod == models.ZammadAuthMethodOAuth {
		token, err := s.oauthAccessToken(ctx, connection, workspaceID)
		if err != nil {
			return nil, err
		}
		return zammad.NewOAuthClient(connection.BaseURL, token, transport), nil
	}
	if connection.CredentialID <= 0 {
		return nil, zammadValidationError("API token is not configured")
	}
	token, _, err := s.credentials.ResolveManaged(ctx, connection.CredentialID, workspaceID, string(models.IntegrationProviderZammad), connection.ProviderID)
	if err != nil {
		return nil, err
	}
	return zammad.NewClient(connection.BaseURL, token, transport), nil
}

func (s *ZammadService) resolveStateName(ctx context.Context, client *zammad.Client, stateID int) string {
	states, err := client.States(ctx)
	if err != nil {
		return ""
	}
	for _, state := range states {
		if state.ID == stateID {
			return state.Name
		}
	}
	return ""
}

func validateNewZammadConnection(req models.CreateZammadConnectionRequest, actorID int) (*models.ZammadConnection, error) {
	if hasNonPositiveIDs(req.ClosedStateIDs) {
		return nil, zammadValidationError("closed_state_ids must contain positive IDs")
	}
	if err := validateZammadGroupRefs(req.AllowedGroups); err != nil {
		return nil, err
	}
	if hasNonPositiveIDs(req.AllowedGroupIDs) {
		return nil, zammadValidationError("allowed_group_ids must contain positive IDs")
	}
	if hasNonPositiveIDs(req.WorkspaceIDs) {
		return nil, zammadValidationError("workspace_ids must contain positive IDs")
	}
	baseURL, err := NormalizeZammadBaseURL(req.BaseURL)
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	appliesAll := false
	if req.AppliesToAllWorkspaces != nil {
		appliesAll = *req.AppliesToAllWorkspaces
	}
	correlationField := strings.TrimSpace(req.CorrelationField)
	if correlationField == "" {
		correlationField = "windshift_item_key"
	}
	connection := &models.ZammadConnection{
		ProviderID: uuid.New().String(), Slug: strings.TrimSpace(req.Slug),
		Name: strings.TrimSpace(req.Name), Enabled: enabled, BaseURL: baseURL,
		DefaultGroupID: req.DefaultGroupID, DefaultGroupName: strings.TrimSpace(req.DefaultGroupName),
		AllowedGroups:   normalizeZammadGroupRefs(req.AllowedGroups),
		DefaultCustomer: strings.TrimSpace(req.DefaultCustomer), CorrelationField: correlationField,
		ClosedStateIDs: normalizePositiveIDs(req.ClosedStateIDs), CompletionStatusID: req.CompletionStatusID,
		AppliesToAllWorkspaces: appliesAll, WorkspaceIDs: normalizePositiveIDs(req.WorkspaceIDs),
		CreatedBy: &actorID,
	}
	if len(connection.AllowedGroups) == 0 && len(req.AllowedGroupIDs) > 0 {
		connection.AllowedGroups = legacyZammadGroupRefs(req.AllowedGroupIDs, connection.DefaultGroupID, connection.DefaultGroupName)
	}
	syncZammadDefaultGroupName(connection)
	connection.AuthMethod = req.AuthMethod
	if connection.AuthMethod == "" {
		connection.AuthMethod = models.ZammadAuthMethodAPIToken
	}
	switch connection.AuthMethod {
	case models.ZammadAuthMethodOAuth:
		connection.OAuthClientID = strings.TrimSpace(req.OAuthClientID)
		if connection.OAuthClientID == "" || strings.TrimSpace(req.OAuthClientSecret) == "" {
			return nil, zammadValidationError("oauth_client_id and oauth_client_secret are required for OAuth")
		}
	case models.ZammadAuthMethodAPIToken:
		if strings.TrimSpace(req.APIToken) == "" {
			return nil, zammadValidationError("Zammad API token is required")
		}
	default:
		return nil, zammadValidationError("auth_method must be api_token or oauth")
	}
	if err := validateZammadConnection(connection); err != nil {
		return nil, err
	}
	return connection, nil
}

func validateZammadConnection(connection *models.ZammadConnection) error {
	if connection.Name == "" || connection.Slug == "" {
		return zammadValidationError("name and slug are required")
	}
	if !zammadSlugPattern.MatchString(connection.Slug) {
		return zammadValidationError("slug must start with a letter and contain only lowercase letters, numbers, hyphens, and underscores")
	}
	if connection.DefaultCustomer == "" {
		return zammadValidationError("default_customer is required")
	}
	if connection.DefaultGroupID < 0 {
		return zammadValidationError("default_group_id must be positive")
	}
	if connection.DefaultGroupID == 0 && connection.DefaultGroupName == "" {
		return zammadValidationError("default_group_id or default_group_name is required")
	}
	allowedGroups := effectiveZammadGroupRefs(connection)
	if connection.DefaultGroupID > 0 && len(allowedGroups) > 0 {
		defaultGroup, ok := zammadGroupRefByID(allowedGroups, connection.DefaultGroupID)
		if !ok {
			return zammadValidationError("default Zammad group must be included in allowed_groups")
		}
		if connection.DefaultGroupName != "" && defaultGroup.Name != "" && connection.DefaultGroupName != defaultGroup.Name {
			return zammadValidationError("default Zammad group name must match allowed_groups")
		}
	}
	if connection.CompletionStatusID != nil && *connection.CompletionStatusID <= 0 {
		return zammadValidationError("completion_status_id must be positive")
	}
	if !zammadFieldNamePattern.MatchString(connection.CorrelationField) {
		return zammadValidationError("correlation_field is not a valid Zammad object field name")
	}
	if !connection.AppliesToAllWorkspaces && len(connection.WorkspaceIDs) == 0 {
		return zammadValidationError("at least one workspace is required")
	}
	return nil
}

func validateZammadOAuthConfiguration(connection *models.ZammadConnection) error {
	if connection.AuthMethod != models.ZammadAuthMethodOAuth {
		return nil
	}
	if strings.TrimSpace(connection.OAuthClientID) == "" || strings.TrimSpace(connection.OAuthClientSecretEncrypted) == "" {
		return zammadValidationError("OAuth client credentials are not configured")
	}
	return nil
}

func NormalizeZammadBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" {
		return "", zammadValidationError("invalid Zammad base URL")
	}
	if parsed.Scheme != "https" {
		return "", zammadValidationError("Zammad base URL must use HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", zammadValidationError("Zammad base URL must not contain credentials, a query, or a fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func zammadOAuthRedirectURI(publicBaseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(publicBaseURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Opaque != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", zammadValidationError("Zammad OAuth requires an absolute HTTPS public base URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/integrations/oauth/system/zammad/callback"
	parsed.RawPath = ""
	return parsed.String(), nil
}

func normalizePositiveIDs(ids []int) []int {
	seen := map[int]struct{}{}
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func validateZammadGroupRefs(groups []models.ZammadGroupRef) error {
	seen := make(map[int]struct{}, len(groups))
	for _, group := range groups {
		if group.ID <= 0 {
			return zammadValidationError("allowed_groups must contain positive IDs")
		}
		if strings.TrimSpace(group.Name) == "" {
			return zammadValidationError("allowed_groups must contain a name for every group")
		}
		if _, exists := seen[group.ID]; exists {
			return zammadValidationError("allowed_groups must not contain duplicate IDs")
		}
		seen[group.ID] = struct{}{}
	}
	return nil
}

func normalizeZammadGroupRefs(groups []models.ZammadGroupRef) []models.ZammadGroupRef {
	seen := make(map[int]struct{}, len(groups))
	out := make([]models.ZammadGroupRef, 0, len(groups))
	for _, group := range groups {
		if group.ID <= 0 {
			continue
		}
		if _, exists := seen[group.ID]; exists {
			continue
		}
		seen[group.ID] = struct{}{}
		out = append(out, models.ZammadGroupRef{ID: group.ID, Name: strings.TrimSpace(group.Name)})
	}
	return out
}

func legacyZammadGroupRefs(groupIDs []int, defaultGroupID int, defaultGroupName string) []models.ZammadGroupRef {
	groups := make([]models.ZammadGroupRef, 0, len(groupIDs))
	for _, groupID := range normalizePositiveIDs(groupIDs) {
		name := ""
		if groupID == defaultGroupID {
			name = strings.TrimSpace(defaultGroupName)
		}
		groups = append(groups, models.ZammadGroupRef{ID: groupID, Name: name})
	}
	return groups
}

func effectiveZammadGroupRefs(connection *models.ZammadConnection) []models.ZammadGroupRef {
	if connection == nil {
		return nil
	}
	return connection.AllowedGroups
}

func effectiveZammadSnapshotGroupRefs(snapshot *repository.ZammadConnectionMutationSnapshot) []models.ZammadGroupRef {
	if snapshot == nil {
		return nil
	}
	return snapshot.AllowedGroups
}

func syncZammadDefaultGroupName(connection *models.ZammadConnection) {
	if connection == nil || connection.DefaultGroupID <= 0 {
		return
	}
	if group, ok := zammadGroupRefByID(effectiveZammadGroupRefs(connection), connection.DefaultGroupID); ok && strings.TrimSpace(group.Name) != "" {
		connection.DefaultGroupName = strings.TrimSpace(group.Name)
	}
}

func zammadGroupRefByID(groups []models.ZammadGroupRef, groupID int) (models.ZammadGroupRef, bool) {
	for _, group := range groups {
		if group.ID == groupID {
			return group, true
		}
	}
	return models.ZammadGroupRef{}, false
}

func hasNonPositiveIDs(ids []int) bool {
	for _, id := range ids {
		if id <= 0 {
			return true
		}
	}
	return false
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func boolPointer(value bool) *bool { return &value }
