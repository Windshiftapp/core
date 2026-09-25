package llm

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/sso"
)

// ErrFeatureDisabled is returned when an AI feature has been disabled by admin.
var ErrFeatureDisabled = errors.New("this AI feature is disabled by your administrator")

// ConnectionInfo represents an LLM connection without sensitive fields.
type ConnectionInfo struct {
	ID             int          `json:"id"`
	Name           string       `json:"name"`
	ProviderType   ProviderType `json:"provider_type"`
	Model          string       `json:"model"`
	HasAPIKey      bool         `json:"has_api_key"`
	BaseURL        string       `json:"base_url,omitempty"`
	ProviderConfig string       `json:"provider_config,omitempty"`
	IsDefault      bool         `json:"is_default"`
	IsEnabled      bool         `json:"is_enabled"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// ConnectionManager bridges the database and the LLM client layer.
type ConnectionManager struct {
	db         database.Database
	encryption *sso.SecretEncryption
	fallback   Client
	modelCache *ModelCache // optional; enables freshest vision-capability resolution
	// warnedModelLimits keys the models already reported as missing catalog
	// limits, so the warning stays one per model rather than one per resolve.
	warnedModelLimits sync.Map
}

// NewConnectionManager creates a new connection manager.
func NewConnectionManager(db database.Database, encryption *sso.SecretEncryption, fallback Client) *ConnectionManager {
	return &ConnectionManager{
		db:         db,
		encryption: encryption,
		fallback:   fallback,
	}
}

// SetModelCache attaches the refreshed-model cache so vision capability resolves
// from the freshest catalog data. Optional and nil-safe: without it, resolution
// falls back to static seeds + the curated map.
func (m *ConnectionManager) SetModelCache(cache *ModelCache) {
	m.modelCache = cache
}

// resolveModelVision returns whether the named model on the provider supports
// vision, consulting (in order) the refreshed cache, the static seed list, and
// finally the curated capability map for ids none of the catalogs recognize.
func (m *ConnectionManager) resolveModelVision(providerType ProviderType, modelID string) bool {
	if m.modelCache != nil {
		if entry, err := m.modelCache.Get(providerType); err == nil {
			for _, mi := range entry.Models {
				if mi.ID == modelID {
					return mi.SupportsVision
				}
			}
		}
	}
	if p := GetProvider(providerType); p != nil {
		for _, mi := range p.Models {
			if mi.ID == modelID {
				return mi.SupportsVision
			}
		}
	}
	return curatedVisionCapable(modelID)
}

// ModelPricing returns the advertised USD rates for the named model, or nil if
// the catalog doesn't carry pricing (cost is then left unknown, not guessed).
// Consults the refreshed cache first, then the static seed list.
func (m *ConnectionManager) ModelPricing(providerType ProviderType, modelID string) *Pricing {
	if m.modelCache != nil {
		if entry, err := m.modelCache.Get(providerType); err == nil {
			for _, mi := range entry.Models {
				if mi.ID == modelID {
					return mi.Pricing
				}
			}
		}
	}
	if p := GetProvider(providerType); p != nil {
		for _, mi := range p.Models {
			if mi.ID == modelID {
				return mi.Pricing
			}
		}
	}
	return nil
}

// Resolve returns a Client for the given connection ID.
// If connectionID > 0, uses that specific enabled connection.
// Otherwise, picks the default enabled connection (or the first enabled one).
// Falls back to the env-var-based client if no DB connections exist.
func (m *ConnectionManager) Resolve(connectionID int) (Client, error) {
	rc, err := m.resolve(connectionID)
	if err != nil {
		return nil, err
	}
	return rc.client, nil
}

// resolvedConnection bundles a ready Client with the non-secret connection
// metadata (provider, model, effective endpoint) so callers that want to log
// or report what they're talking to don't have to re-query. The fallback
// env-var client has no DB row, so usedFallback flags that provider/model are
// unknown.
type resolvedConnection struct {
	client       Client
	connectionID int
	providerType ProviderType
	model        string
	baseURL      string // effective endpoint (provider default when none stored)
	usedFallback bool
}

// resolve is the metadata-returning core behind Resolve. Keeping Resolve's
// signature narrow (just a Client) avoids churning its many callers while
// still letting PromptConnection name the provider/model in its logs.
func (m *ConnectionManager) resolve(connectionID int) (*resolvedConnection, error) {
	var row *sql.Row
	if connectionID > 0 {
		row = m.db.QueryRow(
			`SELECT id, provider_type, model, api_key_encrypted, base_url, provider_config
			 FROM llm_connections
			 WHERE id = ? AND is_enabled = true`,
			connectionID,
		)
	} else {
		row = m.db.QueryRow(
			`SELECT id, provider_type, model, api_key_encrypted, base_url, provider_config
			 FROM llm_connections
			 WHERE is_enabled = true
			 ORDER BY is_default DESC, id ASC
			 LIMIT 1`,
		)
	}

	var id int
	var providerType, model string
	var apiKeyEncrypted, baseURL, providerConfig sql.NullString
	err := row.Scan(&id, &providerType, &model, &apiKeyEncrypted, &baseURL, &providerConfig)
	if errors.Is(err, sql.ErrNoRows) {
		if connectionID > 0 {
			return nil, fmt.Errorf("LLM connection %d not found or disabled", connectionID)
		}
		// No DB connections configured — fall back to the env-var client
		return &resolvedConnection{client: m.fallback, usedFallback: true}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query connection: %w", err)
	}

	var apiKey string
	if apiKeyEncrypted.Valid && apiKeyEncrypted.String != "" {
		apiKey, err = m.encryption.Decrypt(apiKeyEncrypted.String)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt API key: %w", err)
		}
	}

	effectiveBaseURL := baseURL.String
	if effectiveBaseURL == "" {
		if p := GetProvider(ProviderType(providerType)); p != nil {
			effectiveBaseURL = p.BaseURL
		}
	}

	return &resolvedConnection{
		client: NewProviderClient(ConnectionConfig{
			ProviderType:   ProviderType(providerType),
			Model:          model,
			APIKey:         apiKey,
			BaseURL:        baseURL.String,
			ProviderConfig: providerConfig.String,
		}),
		connectionID: id,
		providerType: ProviderType(providerType),
		model:        model,
		baseURL:      effectiveBaseURL,
	}, nil
}

// ConnectionDescriptor is the non-secret identity of a resolved LLM connection:
// enough for a caller to report which model answered a request and to price the
// usage it produced, without holding a client or touching the API key.
type ConnectionDescriptor struct {
	ConnectionID int
	ProviderType ProviderType
	Model        string
	// UsedFallback marks the env-var client, which has no database row — its
	// provider and model are genuinely unknown, so callers must not guess them.
	UsedFallback bool
	// Pricing is the catalog's USD rates for Model, or nil when the catalog
	// carries none. A nil Pricing means cost stays unknown, never estimated.
	Pricing *Pricing
}

// ResolveWithIdentity returns a Client for the given connection ID together
// with the descriptor for the same resolution. Callers that need to report or
// price a call use this rather than resolving a second time: a second resolve
// could land on a different connection if the admin changed the default
// mid-request, which would attribute one model's cost to another.
func (m *ConnectionManager) ResolveWithIdentity(connectionID int) (Client, ConnectionDescriptor, error) {
	rc, err := m.resolve(connectionID)
	if err != nil {
		return nil, ConnectionDescriptor{}, err
	}
	return rc.client, m.descriptorFor(rc), nil
}

// descriptorFor projects a resolved connection onto its public identity,
// attaching catalog pricing when one exists for the model.
func (m *ConnectionManager) descriptorFor(rc *resolvedConnection) ConnectionDescriptor {
	desc := ConnectionDescriptor{
		ConnectionID: rc.connectionID,
		ProviderType: rc.providerType,
		Model:        rc.model,
		UsedFallback: rc.usedFallback,
	}
	if desc.Model != "" {
		desc.Pricing = m.ModelPricing(rc.providerType, rc.model)
	}
	return desc
}

// ConnectionRuntimeConfig contains the decrypted runtime fields needed to
// resolve the admin-selected provider for a coding-agent run (the model id for
// the agent container; the key + base URL stay server-side in the llm-proxy).
type ConnectionRuntimeConfig struct {
	ProviderType   string
	APIFormat      string
	Model          string
	APIKey         string
	BaseURL        string
	ProviderConfig string
	Protocol       string
	// VisionMode is the resolved per-connection override (auto/on/off).
	VisionMode string
	// SupportsVision is the effective vision capability for this connection's
	// model after applying the override — the value injected into the agent env.
	SupportsVision  bool
	ContextWindow   int
	MaxOutputTokens int
}

// ConnectionRuntime returns the runtime config for one enabled connection. It
// is intentionally narrower than GetConnection and is only used after callers
// have already authorized access to the selected connection.
func (m *ConnectionManager) ConnectionRuntime(ctx context.Context, connectionID int) (*ConnectionRuntimeConfig, error) {
	cfg := &ConnectionRuntimeConfig{}
	var apiKeyEncrypted, baseURLNull, providerConfig sql.NullString
	err := m.db.QueryRowContext(ctx,
		`SELECT provider_type, model, api_key_encrypted, base_url, provider_config
		 FROM llm_connections
		 WHERE id = ? AND is_enabled = true`,
		connectionID,
	).Scan(&cfg.ProviderType, &cfg.Model, &apiKeyEncrypted, &baseURLNull, &providerConfig)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("LLM connection %d not found or disabled", connectionID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query connection runtime: %w", err)
	}
	provider := GetProvider(ProviderType(cfg.ProviderType))
	if provider == nil {
		return nil, fmt.Errorf("unknown LLM provider type %q", cfg.ProviderType)
	}
	cfg.APIFormat = provider.APIFormat
	if apiKeyEncrypted.Valid && apiKeyEncrypted.String != "" {
		cfg.APIKey, err = m.encryption.Decrypt(apiKeyEncrypted.String)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt API key: %w", err)
		}
	}
	if baseURLNull.Valid {
		cfg.BaseURL = baseURLNull.String
	}
	if providerConfig.Valid {
		cfg.ProviderConfig = providerConfig.String
	}
	cfg.Protocol = ResolveGenerationProtocol(ProviderType(cfg.ProviderType), cfg.BaseURL, cfg.ProviderConfig)
	cfg.VisionMode = ProviderConfigVisionMode(cfg.ProviderConfig)
	cfg.SupportsVision = EffectiveVision(cfg.VisionMode, m.resolveModelVision(ProviderType(cfg.ProviderType), cfg.Model))
	resolvedContext, resolvedOutput := m.resolveModelLimits(ProviderType(cfg.ProviderType), cfg.Model)
	cfg.ContextWindow, cfg.MaxOutputTokens = resolvedContext, resolvedOutput
	if applyFallbackModelLimits(cfg) {
		// ConnectionRuntime runs per profile render and per run start, so warn
		// once per model rather than on every resolve.
		if _, warned := m.warnedModelLimits.LoadOrStore(cfg.ProviderType+"\x00"+cfg.Model, true); !warned {
			slog.Warn("LLM model limits are not advertised by the provider catalog; using conservative defaults",
				slog.String("provider_type", cfg.ProviderType),
				slog.String("model", cfg.Model),
				slog.Int("resolved_context_window", resolvedContext),
				slog.Int("resolved_max_output_tokens", resolvedOutput),
				slog.Int("context_window", cfg.ContextWindow),
				slog.Int("max_output_tokens", cfg.MaxOutputTokens),
			)
		}
	}
	return cfg, nil
}

// Conservative limits for a model no catalog advertises. Several providers
// publish no limits at all — Anthropic's /v1/models carries none, OpenAI's
// carries neither a context length nor an output cap, and the local provider
// has no static model list — so refusing to resolve such a connection would
// take out binding validation, profile creation, and run start along with the
// inference call. Erring low is deliberate in both directions: an oversized
// max_tokens is a hard 400 on Anthropic and OpenAI while an undersized one only
// truncates, and an oversized context window overflows the upstream prompt
// while an undersized one merely packs less history.
const (
	fallbackContextWindow   = 128_000
	fallbackMaxOutputTokens = 4096
)

// applyFallbackModelLimits fills limits the catalog left unresolved, reporting
// whether it had to. A caller that resolves repeatedly uses that to warn about
// each unknown model once instead of on every resolve.
func applyFallbackModelLimits(cfg *ConnectionRuntimeConfig) bool {
	if cfg.ContextWindow > 0 && cfg.MaxOutputTokens > 0 {
		return false
	}
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = fallbackContextWindow
	}
	if cfg.MaxOutputTokens <= 0 {
		cfg.MaxOutputTokens = fallbackMaxOutputTokens
		// A model may advertise a context window smaller than the output floor.
		if cfg.ContextWindow < cfg.MaxOutputTokens {
			cfg.MaxOutputTokens = cfg.ContextWindow
		}
	}
	return true
}

func (m *ConnectionManager) resolveModelLimits(providerType ProviderType, modelID string) (contextWindow, maxOutput int) {
	if m.modelCache != nil {
		if entry, err := m.modelCache.Get(providerType); err == nil {
			for _, mi := range entry.Models {
				if mi.ID == modelID {
					if mi.ContextWindow > 0 {
						contextWindow = mi.ContextWindow
						maxOutput = mi.MaxTokens
					} else {
						contextWindow = mi.MaxTokens
					}
					break
				}
			}
		}
	}
	if p := GetProvider(providerType); p != nil {
		for _, mi := range p.Models {
			if mi.ID == modelID {
				if contextWindow == 0 {
					contextWindow = mi.ContextWindow
				}
				if maxOutput == 0 {
					maxOutput = mi.MaxTokens
				}
				break
			}
		}
	}
	return contextWindow, maxOutput
}

// ListConnections returns all connections (without secrets) for admin listing.
func (m *ConnectionManager) ListConnections() ([]ConnectionInfo, error) {
	rows, err := m.db.Query(
		`SELECT id, name, provider_type, model, api_key_encrypted, base_url, provider_config, is_default, is_enabled, created_at, updated_at
		 FROM llm_connections ORDER BY is_default DESC, name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list connections: %w", err)
	}
	defer rows.Close()

	return scanConnections(rows)
}

// PublicConnectionInfo is the slim, user-facing view of an LLM connection.
// It deliberately omits admin-only fields (BaseURL, HasAPIKey, timestamps,
// IsEnabled) so the user dropdown endpoint can't leak infrastructure URLs
// — see bughunt8 finding 4.
type PublicConnectionInfo struct {
	ID           int          `json:"id"`
	Name         string       `json:"name"`
	ProviderType ProviderType `json:"provider_type"`
	Model        string       `json:"model"`
	IsDefault    bool         `json:"is_default"`
	// SupportsVision is the connection's effective vision capability (model
	// capability + per-connection override), so the binding picker can warn when
	// a bound model can't see images on work items.
	SupportsVision bool `json:"supports_vision"`
}

// ListEnabledPublic returns the slim, user-facing view of all enabled
// connections. It's the user-facing counterpart of ListConnections
// (which is admin-only and returns the full ConnectionInfo).
func (m *ConnectionManager) ListEnabledPublic() ([]PublicConnectionInfo, error) {
	rows, err := m.db.Query(
		`SELECT id, name, provider_type, model, is_default, provider_config
		 FROM llm_connections
		 WHERE is_enabled = true
		 ORDER BY is_default DESC, name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled connections: %w", err)
	}
	defer rows.Close()

	out := make([]PublicConnectionInfo, 0)
	for rows.Next() {
		var c PublicConnectionInfo
		var providerType string
		var providerConfig sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &providerType, &c.Model, &c.IsDefault, &providerConfig); err != nil {
			return nil, fmt.Errorf("failed to scan connection: %w", err)
		}
		c.ProviderType = ProviderType(providerType)
		c.SupportsVision = EffectiveVision(ProviderConfigVisionMode(providerConfig.String), m.resolveModelVision(c.ProviderType, c.Model))
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate connections: %w", err)
	}
	return out, nil
}

// GetConnection returns a single connection by ID.
func (m *ConnectionManager) GetConnection(id int) (*ConnectionInfo, error) {
	var c ConnectionInfo
	var apiKeyEncrypted, baseURL, providerConfig sql.NullString
	err := m.db.QueryRow(
		`SELECT id, name, provider_type, model, api_key_encrypted, base_url, provider_config, is_default, is_enabled, created_at, updated_at
		 FROM llm_connections WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.ProviderType, &c.Model, &apiKeyEncrypted, &baseURL, &providerConfig, &c.IsDefault, &c.IsEnabled, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}
	c.HasAPIKey = apiKeyEncrypted.Valid && apiKeyEncrypted.String != ""
	if baseURL.Valid {
		c.BaseURL = baseURL.String
	}
	if providerConfig.Valid {
		c.ProviderConfig = providerConfig.String
	}
	return &c, nil
}

// CreateConnectionRequest is the input for creating a connection.
type CreateConnectionRequest struct {
	Name           string       `json:"name"`
	ProviderType   ProviderType `json:"provider_type"`
	Model          string       `json:"model"`
	APIKey         string       `json:"api_key,omitempty"`
	BaseURL        string       `json:"base_url,omitempty"`
	ProviderConfig string       `json:"provider_config,omitempty"`
	IsDefault      bool         `json:"is_default"`
	IsEnabled      bool         `json:"is_enabled"`
}

// CreateConnection creates a new LLM connection.
func (m *ConnectionManager) CreateConnection(req CreateConnectionRequest) (*ConnectionInfo, error) {
	var encryptedKey sql.NullString
	if req.APIKey != "" {
		encrypted, err := m.encryption.Encrypt(req.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
		encryptedKey = sql.NullString{String: encrypted, Valid: true}
	}

	var baseURL sql.NullString
	if req.BaseURL != "" {
		baseURL = sql.NullString{String: req.BaseURL, Valid: true}
	}
	var providerConfig sql.NullString
	if req.ProviderConfig != "" {
		providerConfig = sql.NullString{String: req.ProviderConfig, Valid: true}
	}

	// If setting as default, clear existing defaults
	if req.IsDefault {
		if _, err := m.db.ExecWrite("UPDATE llm_connections SET is_default = false WHERE is_default = true"); err != nil {
			return nil, fmt.Errorf("failed to clear existing defaults: %w", err)
		}
	}

	var id int64
	err := m.db.QueryRow(
		`INSERT INTO llm_connections (name, provider_type, model, api_key_encrypted, base_url, provider_config, is_default, is_enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		req.Name, string(req.ProviderType), req.Model, encryptedKey, baseURL, providerConfig, req.IsDefault, req.IsEnabled,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection: %w", err)
	}

	return m.GetConnection(int(id))
}

// UpdateConnectionRequest is the input for updating a connection.
type UpdateConnectionRequest struct {
	Name           string       `json:"name"`
	ProviderType   ProviderType `json:"provider_type"`
	Model          string       `json:"model"`
	APIKey         string       `json:"api_key,omitempty"`
	BaseURL        string       `json:"base_url,omitempty"`
	ProviderConfig string       `json:"provider_config,omitempty"`
	IsDefault      bool         `json:"is_default"`
	IsEnabled      bool         `json:"is_enabled"`
}

// UpdateConnection updates an existing LLM connection.
func (m *ConnectionManager) UpdateConnection(id int, req UpdateConnectionRequest) (*ConnectionInfo, error) {
	// If setting as default, clear existing defaults
	if req.IsDefault {
		if _, err := m.db.ExecWrite("UPDATE llm_connections SET is_default = false WHERE is_default = true AND id != ?", id); err != nil {
			return nil, fmt.Errorf("failed to clear existing defaults: %w", err)
		}
	}
	var providerConfig sql.NullString
	if req.ProviderConfig != "" {
		providerConfig = sql.NullString{String: req.ProviderConfig, Valid: true}
	}

	if req.APIKey != "" {
		encrypted, err := m.encryption.Encrypt(req.APIKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt API key: %w", err)
		}
		_, err = m.db.ExecWrite(
			`UPDATE llm_connections SET name = ?, provider_type = ?, model = ?, api_key_encrypted = ?, base_url = ?, provider_config = ?, is_default = ?, is_enabled = ?, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			req.Name, string(req.ProviderType), req.Model, encrypted, req.BaseURL, providerConfig, req.IsDefault, req.IsEnabled, id,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update connection: %w", err)
		}
	} else {
		// Don't overwrite API key if not provided
		_, err := m.db.ExecWrite(
			`UPDATE llm_connections SET name = ?, provider_type = ?, model = ?, base_url = ?, provider_config = ?, is_default = ?, is_enabled = ?, updated_at = CURRENT_TIMESTAMP
			 WHERE id = ?`,
			req.Name, string(req.ProviderType), req.Model, req.BaseURL, providerConfig, req.IsDefault, req.IsEnabled, id,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update connection: %w", err)
		}
	}

	return m.GetConnection(id)
}

// DeleteConnection deletes an LLM connection.
func (m *ConnectionManager) DeleteConnection(id int) error {
	_, err := m.db.ExecWrite("DELETE FROM llm_connections WHERE id = ?", id)
	return err
}

// GetAnyAPIKeyForProvider returns a decrypted API key from any enabled
// connection of the given provider type, or "" when none is configured.
//
// Used by the catalog-refresh handler: most provider /models endpoints require
// auth, so we borrow the key from an existing connection rather than asking
// admins to enter it twice. Returns ("", nil) when no enabled connection
// exists — the handler then decides whether the provider requires a key
// (everything except OpenRouter does today).
func (m *ConnectionManager) GetAnyAPIKeyForProvider(providerType ProviderType) (string, error) {
	runtime, err := m.GetCatalogRuntimeForProvider(providerType)
	if err != nil {
		return "", err
	}
	if runtime == nil {
		return "", nil
	}
	return runtime.APIKey, nil
}

// CatalogRuntime contains endpoint/auth material used to refresh a provider's
// model catalog. It intentionally excludes model names and other unrelated
// connection fields.
type CatalogRuntime struct {
	ConnectionID int
	APIKey       string
	BaseURL      string
}

// GetCatalogRuntimeForProvider returns auth/base URL from the preferred enabled
// connection for a provider. It returns nil when no enabled connection exists.
func (m *ConnectionManager) GetCatalogRuntimeForProvider(providerType ProviderType) (*CatalogRuntime, error) {
	return m.catalogRuntimeFromRow(m.db.QueryRow(
		`SELECT id, api_key_encrypted, base_url FROM llm_connections
		 WHERE provider_type = ? AND is_enabled = true
		 ORDER BY CASE WHEN api_key_encrypted IS NOT NULL AND api_key_encrypted <> '' THEN 0 ELSE 1 END,
		          is_default DESC, id ASC LIMIT 1`,
		string(providerType),
	), fmt.Sprintf("%q", providerType))
}

// GetCatalogRuntimeForConnection returns auth/base URL for one enabled
// connection, and the connection's provider type so callers can validate it
// against route parameters.
func (m *ConnectionManager) GetCatalogRuntimeForConnection(connectionID int) (ProviderType, *CatalogRuntime, error) {
	var providerType string
	row := m.db.QueryRow(
		`SELECT provider_type, id, api_key_encrypted, base_url FROM llm_connections
		 WHERE id = ? AND is_enabled = true`,
		connectionID,
	)
	var id int
	var apiKeyEncrypted, baseURL sql.NullString
	if err := row.Scan(&providerType, &id, &apiKeyEncrypted, &baseURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("lookup catalog runtime for connection %d: %w", connectionID, err)
	}
	runtime, err := m.decryptCatalogRuntime(id, apiKeyEncrypted, baseURL, fmt.Sprintf("connection %d", connectionID))
	return ProviderType(providerType), runtime, err
}

func (m *ConnectionManager) catalogRuntimeFromRow(row *sql.Row, label string) (*CatalogRuntime, error) {
	var id int
	var apiKeyEncrypted, baseURL sql.NullString
	if err := row.Scan(&id, &apiKeyEncrypted, &baseURL); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup catalog runtime for %s: %w", label, err)
	}
	return m.decryptCatalogRuntime(id, apiKeyEncrypted, baseURL, label)
}

func (m *ConnectionManager) decryptCatalogRuntime(id int, apiKeyEncrypted, baseURL sql.NullString, label string) (*CatalogRuntime, error) {
	runtime := &CatalogRuntime{ConnectionID: id}
	if apiKeyEncrypted.Valid && apiKeyEncrypted.String != "" {
		apiKey, err := m.encryption.Decrypt(apiKeyEncrypted.String)
		if err != nil {
			return nil, fmt.Errorf("decrypt api key for %s: %w", label, err)
		}
		runtime.APIKey = apiKey
	}
	if baseURL.Valid {
		runtime.BaseURL = baseURL.String
	}
	return runtime, nil
}

// TestConnection tests a connection by creating a client and calling Health.
func (m *ConnectionManager) TestConnection(id int) error {
	var providerType, model string
	var apiKeyEncrypted, baseURL, providerConfig sql.NullString
	err := m.db.QueryRow(
		"SELECT provider_type, model, api_key_encrypted, base_url, provider_config FROM llm_connections WHERE id = ?", id,
	).Scan(&providerType, &model, &apiKeyEncrypted, &baseURL, &providerConfig)
	if err != nil {
		return fmt.Errorf("connection not found: %w", err)
	}

	var apiKey string
	if apiKeyEncrypted.Valid && apiKeyEncrypted.String != "" {
		apiKey, err = m.encryption.Decrypt(apiKeyEncrypted.String)
		if err != nil {
			return fmt.Errorf("failed to decrypt API key: %w", err)
		}
	}

	client := NewProviderClient(ConnectionConfig{
		ProviderType:   ProviderType(providerType),
		Model:          model,
		APIKey:         apiKey,
		BaseURL:        baseURL.String,
		ProviderConfig: providerConfig.String,
		Timeout:        30 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return client.Health(ctx)
}

// PromptConnection runs a one-shot chat completion against an enabled
// connection and returns the model's reply text. Unlike TestConnection's
// Health ping, this exercises the full inference path — provider, key, and
// model — which is what a "test this agent's LLM" button needs. The connection
// must be enabled (Resolve only returns enabled rows); a disabled or missing
// id errors. The resolved client carries the manager's private-CIDR allowlist
// so a self-hosted base URL can't be turned into an SSRF probe.
func (m *ConnectionManager) PromptConnection(ctx context.Context, connectionID int, prompt string) (string, error) {
	if connectionID <= 0 {
		return "", fmt.Errorf("a connection id is required")
	}
	rc, err := m.resolve(connectionID)
	if err != nil {
		slog.Warn("llm test prompt: resolve failed",
			slog.Int("connection_id", connectionID),
			slog.String("error", err.Error()),
		)
		return "", err
	}

	log := slog.With(
		slog.Int("connection_id", connectionID),
		slog.String("provider", string(rc.providerType)),
		slog.String("model", rc.model),
		slog.String("base_url", rc.baseURL),
		slog.Bool("fallback_client", rc.usedFallback),
		slog.Int("prompt_chars", len(prompt)),
	)
	log.Info("llm test prompt: sending to provider")

	start := time.Now()
	resp, err := rc.client.Complete(ctx, CompletionRequest{
		Messages:  []Message{{Role: "user", Content: prompt}},
		MaxTokens: 256,
	})
	if err != nil {
		log.Warn("llm test prompt: provider call failed",
			slog.Duration("duration", time.Since(start)),
			slog.String("error", err.Error()),
		)
		return "", err
	}
	if len(resp.Choices) == 0 {
		log.Warn("llm test prompt: provider returned no choices",
			slog.Duration("duration", time.Since(start)),
		)
		return "", fmt.Errorf("model returned no choices")
	}

	answer := strings.TrimSpace(resp.Choices[0].Message.Content)
	log.Info("llm test prompt: reply received",
		slog.Duration("duration", time.Since(start)),
		slog.Int("answer_chars", len(answer)),
		slog.String("finish_reason", resp.Choices[0].FinishReason),
		slog.Int("prompt_tokens", resp.Usage.PromptTokens),
		slog.Int("completion_tokens", resp.Usage.CompletionTokens),
		slog.Int("cache_read_tokens", resp.Usage.CacheReadTokens),
		slog.Int("cache_write_tokens", resp.Usage.CacheWriteTokens),
		slog.Int("reasoning_tokens", resp.Usage.ReasoningTokens),
		slog.Int("total_tokens", resp.Usage.TotalTokens),
	)
	return answer, nil
}

// LoadAIFeaturesConfig reads the per-feature AI configuration from system_settings.
func LoadAIFeaturesConfig(db database.Database) (models.AIFeaturesConfig, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM system_settings WHERE key = 'ai_feature_config'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return models.AIFeaturesConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load AI features config: %w", err)
	}
	var cfg models.AIFeaturesConfig
	if err := json.Unmarshal([]byte(value), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse AI features config: %w", err)
	}
	return cfg, nil
}

// SaveAIFeaturesConfig persists the per-feature AI configuration to system_settings.
func SaveAIFeaturesConfig(db database.Database, cfg models.AIFeaturesConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal AI features config: %w", err)
	}
	_, err = db.ExecWrite(
		`UPDATE system_settings SET value = ?, updated_at = CURRENT_TIMESTAMP WHERE key = 'ai_feature_config'`,
		string(data),
	)
	return err
}

// ResolveForFeature resolves an LLM client respecting per-feature configuration.
func (m *ConnectionManager) ResolveForFeature(featureKey string) (Client, error) {
	return m.ResolveForFeatureWithOverride(featureKey, 0)
}

// ResolveForFeatureWithOverride resolves an LLM client respecting per-feature
// admin configuration, optionally honoring a user-supplied connection override.
//
// Policy:
//   - Mode == Disabled → returns ErrFeatureDisabled regardless of override.
//   - Mode == Specific → ignores override, returns the pinned connection.
//     This is the security-critical case: a user who supplies a different
//     connection_id MUST NOT be able to escape the admin's pin.
//   - Mode == Default (or no entry) → uses override if > 0, else the default
//     enabled connection.
func (m *ConnectionManager) ResolveForFeatureWithOverride(featureKey string, userOverrideConnectionID int) (Client, error) {
	cfg, err := LoadAIFeaturesConfig(m.db)
	if err != nil {
		return nil, err
	}
	decision := decideFeatureResolution(cfg[featureKey], userOverrideConnectionID)
	if decision.disabled {
		return nil, ErrFeatureDisabled
	}
	return m.Resolve(decision.connectionID)
}

// ResolveForFeatureWithIdentity applies the same per-feature policy as
// ResolveForFeatureWithOverride and returns the descriptor for the connection
// that policy selected. Both must stay on decideFeatureResolution: a chat that
// reports the wrong model because it re-derived the policy is worse than one
// that reports nothing.
func (m *ConnectionManager) ResolveForFeatureWithIdentity(featureKey string, userOverrideConnectionID int) (Client, ConnectionDescriptor, error) {
	cfg, err := LoadAIFeaturesConfig(m.db)
	if err != nil {
		return nil, ConnectionDescriptor{}, err
	}
	decision := decideFeatureResolution(cfg[featureKey], userOverrideConnectionID)
	if decision.disabled {
		return nil, ConnectionDescriptor{}, ErrFeatureDisabled
	}
	return m.ResolveWithIdentity(decision.connectionID)
}

// featureResolution is the outcome of applying the feature policy: either the
// feature is disabled, or we know which connection_id to pass to Resolve.
type featureResolution struct {
	disabled     bool
	connectionID int // 0 means "use the default enabled connection"
}

// decideFeatureResolution is the pure policy function. Extracted so the rules
// can be unit-tested without spinning up a database or a manager.
func decideFeatureResolution(fc models.AIFeatureConfig, userOverrideConnectionID int) featureResolution {
	switch fc.Mode {
	case models.AIFeatureModeDisabled:
		return featureResolution{disabled: true}
	case models.AIFeatureModeSpecific:
		// Admin pinned this feature to a specific connection — ignore the
		// caller's override so it cannot escape the pin.
		return featureResolution{connectionID: fc.ConnectionID}
	default: // AIFeatureModeDefault (or unrecognized — treat as default)
		if userOverrideConnectionID > 0 {
			return featureResolution{connectionID: userOverrideConnectionID}
		}
		return featureResolution{connectionID: 0}
	}
}
