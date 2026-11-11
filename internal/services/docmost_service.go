package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"windshift/internal/integrations/docmost"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sso"
)

// Errors the Docmost service surfaces. Handlers map them to reconnect vs
// unavailable; none carry token material.
var (
	ErrDocmostNotConfigured = errors.New("docmost: not configured")
	ErrDocmostReconnect     = errors.New("docmost: reconnect required")
)

const (
	docmostCredentialEncryptionInfo = "windshift-docmost-credentials-encryption-v1" //nolint:gosec // G101: HKDF domain-separation label, not a credential
	docmostOAuthScopes              = "read write"
	docmostRefreshSkew              = 60 * time.Second
	docmostRefreshLease             = 30 * time.Second
)

// DocmostClientAPI is the client surface the service needs. *docmost.Client
// satisfies it; tests substitute a fake.
type DocmostClientAPI interface {
	RegisterClient(ctx context.Context, baseURL, clientName, redirectURI string) (string, error)
	ExchangeCode(ctx context.Context, baseURL, clientID, redirectURI, code, codeVerifier string) (docmost.Token, error)
	RefreshToken(ctx context.Context, baseURL, clientID, refreshToken string) (docmost.Token, error)
	RevokeToken(ctx context.Context, baseURL, clientID, token string) error
}

// DocmostService owns the Docmost provider's admin base connection and each
// user's token. Docmost is a global integration (one instance per deployment),
// stored in the shared integration tables like Notion/Todoist.
type DocmostService struct {
	providers  *repository.IntegrationProviderRepository
	tokens     *repository.IntegrationTokenRepository
	encryption *sso.SecretEncryption
	client     DocmostClientAPI
	// registerMu serializes first-connect client registration so concurrent
	// users do not register two public clients and race the stored client id.
	registerMu sync.Mutex
}

// NewDocmostService wires the shared provider/token repositories, the
// domain-separated encryption helper, and the Docmost client.
func NewDocmostService(providers *repository.IntegrationProviderRepository, tokens *repository.IntegrationTokenRepository, encryption *sso.SecretEncryption, client DocmostClientAPI) *DocmostService {
	return &DocmostService{providers: providers, tokens: tokens, encryption: encryption, client: client}
}

// NewDocmostCredentialEncryption derives the credential encryption helper from
// the server secret.
func NewDocmostCredentialEncryption(serverSecret string) *sso.SecretEncryption {
	return sso.NewSecretEncryptionWithInfo(serverSecret, docmostCredentialEncryptionInfo)
}

type docmostProviderConfig struct {
	BaseURL string `json:"base_url"`
}

func docmostBaseURL(config string) string {
	var cfg docmostProviderConfig
	if config != "" {
		_ = json.Unmarshal([]byte(config), &cfg)
	}
	return strings.TrimRight(cfg.BaseURL, "/")
}

// Provider resolves the enabled Docmost provider and its base URL.
func (s *DocmostService) Provider() (*repository.IntegrationProvider, string, error) {
	provider, err := s.providers.GetEnabledByType(models.IntegrationProviderDocmost)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, "", ErrDocmostNotConfigured
	}
	if err != nil {
		return nil, "", err
	}
	baseURL := docmostBaseURL(provider.ProviderConfig)
	if baseURL == "" {
		return nil, "", ErrDocmostNotConfigured
	}
	return provider, baseURL, nil
}

// providerByID resolves one provider row and its base URL.
func (s *DocmostService) providerByID(providerID string) (*repository.IntegrationProvider, string, error) {
	provider, err := s.providers.GetByID(providerID)
	if err != nil {
		return nil, "", err
	}
	baseURL := docmostBaseURL(provider.ProviderConfig)
	if baseURL == "" {
		return nil, "", ErrDocmostNotConfigured
	}
	return provider, baseURL, nil
}

// StartOAuth ensures a public client is registered and returns the browser
// consent URL plus the PKCE verifier for the caller to store with the state.
func (s *DocmostService) StartOAuth(ctx context.Context, providerID, state, redirectURI string) (authorizeURL, codeVerifier string, err error) {
	provider, baseURL, err := s.providerByID(providerID)
	if err != nil {
		return "", "", err
	}

	clientID := provider.OAuthClientID
	if clientID == "" {
		clientID, err = s.ensureClientID(ctx, providerID, redirectURI)
		if err != nil {
			return "", "", err
		}
	}

	codeVerifier, err = randomToken(48)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(codeVerifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {docmostOAuthScopes},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return baseURL + "/oauth/consent?" + params.Encode(), codeVerifier, nil
}

// ensureClientID returns the provider's registered public client id, registering
// one on first use. registerMu serializes the registration so concurrent users
// do not create two clients and race the stored client id.
func (s *DocmostService) ensureClientID(ctx context.Context, providerID, redirectURI string) (string, error) {
	s.registerMu.Lock()
	defer s.registerMu.Unlock()

	// Re-read under the lock: another request may have registered the client
	// between StartOAuth's first read and acquiring the lock.
	provider, baseURL, err := s.providerByID(providerID)
	if err != nil {
		return "", err
	}
	if provider.OAuthClientID != "" {
		return provider.OAuthClientID, nil
	}
	clientID, err := s.client.RegisterClient(ctx, baseURL, "Windshift", redirectURI)
	if err != nil {
		return "", err
	}
	if err := s.providers.Update(provider.ID, repository.IntegrationProviderUpdate{OAuthClientID: &clientID}); err != nil {
		return "", err
	}
	return clientID, nil
}

// CompleteOAuth exchanges the authorization code and stores the encrypted
// token pair.
func (s *DocmostService) CompleteOAuth(ctx context.Context, providerID string, userID int, codeVerifier, code, redirectURI string) error {
	provider, baseURL, err := s.providerByID(providerID)
	if err != nil {
		return err
	}
	token, err := s.client.ExchangeCode(ctx, baseURL, provider.OAuthClientID, redirectURI, code, codeVerifier)
	if err != nil {
		return err
	}
	return s.storeToken(provider, userID, token)
}

// Credential returns the acting user's Docmost access token, refreshing it when
// it is close to expiry. Fails closed: a missing credential returns
// repository.ErrNotFound and a rejected one returns ErrDocmostReconnect.
func (s *DocmostService) Credential(ctx context.Context, userID int) (baseURL, token string, err error) {
	provider, baseURL, err := s.Provider()
	if err != nil {
		return "", "", err
	}
	row, err := s.tokens.GetUserToken(provider.ID, userID)
	if err != nil {
		return "", "", err
	}
	if row.InvalidatedAt != nil {
		return "", "", ErrDocmostReconnect
	}

	if row.ExpiresAt != nil {
		now := time.Now()
		if now.Add(docmostRefreshSkew).After(*row.ExpiresAt) {
			expired := now.After(*row.ExpiresAt)
			refreshed, refreshErr := s.refresh(ctx, provider, baseURL, userID)
			if refreshErr != nil {
				return "", "", refreshErr
			}
			if refreshed {
				row, err = s.tokens.GetUserToken(provider.ID, userID)
				if err != nil {
					return "", "", err
				}
			} else if expired {
				return "", "", docmost.ErrUnavailable
			}
		}
	}

	decrypted, err := s.encryption.Decrypt(row.EncryptedAccessToken)
	if err != nil {
		return "", "", fmt.Errorf("decrypt docmost token: %w", err)
	}
	return baseURL, decrypted, nil
}

func (s *DocmostService) refresh(ctx context.Context, provider *repository.IntegrationProvider, baseURL string, userID int) (bool, error) {
	owner, err := randomToken(16)
	if err != nil {
		return false, err
	}
	claimed, err := s.tokens.ClaimRefresh(provider.ID, userID, owner, time.Now().Add(docmostRefreshLease))
	if err != nil {
		return false, err
	}
	if !claimed {
		return false, nil
	}
	defer func() { _ = s.tokens.ReleaseRefreshClaim(provider.ID, userID, owner) }()

	row, err := s.tokens.GetUserToken(provider.ID, userID)
	if err != nil {
		return false, err
	}
	if row.EncryptedRefreshToken == "" {
		s.MarkInvalid(userID)
		return false, ErrDocmostReconnect
	}
	refreshToken, err := s.encryption.Decrypt(row.EncryptedRefreshToken)
	if err != nil {
		return false, fmt.Errorf("decrypt docmost refresh token: %w", err)
	}
	token, err := s.client.RefreshToken(ctx, baseURL, provider.OAuthClientID, refreshToken)
	if err != nil {
		if errors.Is(err, docmost.ErrUnauthorized) {
			s.MarkInvalid(userID)
			return false, ErrDocmostReconnect
		}
		return false, err
	}
	if err := s.storeToken(provider, userID, token); err != nil {
		return false, err
	}
	return true, nil
}

func (s *DocmostService) storeToken(provider *repository.IntegrationProvider, userID int, token docmost.Token) error {
	encAccess, err := s.encryption.Encrypt(token.AccessToken)
	if err != nil {
		return fmt.Errorf("encrypt docmost access token: %w", err)
	}
	encRefresh := ""
	if token.RefreshToken != "" {
		encRefresh, err = s.encryption.Encrypt(token.RefreshToken)
		if err != nil {
			return fmt.Errorf("encrypt docmost refresh token: %w", err)
		}
	}
	var expiresAt *time.Time
	if !token.ExpiresAt.IsZero() {
		expiresAt = &token.ExpiresAt
	}
	return s.tokens.UpsertOAuthToken(provider.ID, userID, encAccess, encRefresh, token.Scope, expiresAt)
}

// Revoke revokes the user's Docmost token at the provider, best-effort.
func (s *DocmostService) Revoke(ctx context.Context, providerID string, userID int) {
	provider, baseURL, err := s.providerByID(providerID)
	if err != nil {
		return
	}
	row, err := s.tokens.GetUserToken(provider.ID, userID)
	if err != nil || row.EncryptedRefreshToken == "" {
		return
	}
	refreshToken, err := s.encryption.Decrypt(row.EncryptedRefreshToken)
	if err != nil {
		return
	}
	_ = s.client.RevokeToken(ctx, baseURL, provider.OAuthClientID, refreshToken)
}

// MarkInvalid flags the user's token so the UI prompts a reconnect.
func (s *DocmostService) MarkInvalid(userID int) {
	if provider, _, err := s.Provider(); err == nil {
		_ = s.tokens.MarkInvalid(provider.ID, userID)
	}
}

// MarkVerified clears a prior invalidated state.
func (s *DocmostService) MarkVerified(userID int) {
	if provider, _, err := s.Provider(); err == nil {
		_ = s.tokens.MarkVerified(provider.ID, userID)
	}
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
