package llm

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
)

// ModelCache persists per-provider model catalogs fetched from the provider's
// /models endpoint. It also records the outcome of the last refresh attempt so
// the UI can show "stale since…" or surface the network error verbatim. The
// cache is intentionally key-per-provider: the catalog from openrouter.ai is
// the same for every llm_connections row pointed at it.
type ModelCache struct {
	db database.Database
}

// NewModelCache constructs a ModelCache.
func NewModelCache(db database.Database) *ModelCache {
	return &ModelCache{db: db}
}

// CacheEntry is the persisted state for one provider's model catalog.
type CacheEntry struct {
	Models          []ModelInfo
	LastRefreshedAt *time.Time
	LastError       string
}

// Get returns the cache entry for a provider. A missing row returns an empty
// CacheEntry (zero models, nil timestamp, no error) and no error — the caller
// then knows to prompt the admin to refresh.
func (c *ModelCache) Get(providerType ProviderType) (CacheEntry, error) {
	var modelsJSON string
	var lastRefreshedAt sql.NullTime
	var lastError sql.NullString
	row := c.db.QueryRow(
		`SELECT models_json, last_refreshed_at, last_error
		 FROM llm_provider_model_cache WHERE provider_type = ?`,
		string(providerType),
	)
	if err := row.Scan(&modelsJSON, &lastRefreshedAt, &lastError); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return CacheEntry{}, nil
		}
		return CacheEntry{}, fmt.Errorf("read model cache for %q: %w", providerType, err)
	}

	var models []ModelInfo
	if modelsJSON != "" {
		if err := json.Unmarshal([]byte(modelsJSON), &models); err != nil {
			return CacheEntry{}, fmt.Errorf("decode cached models for %q: %w", providerType, err)
		}
	}
	// Re-apply the curated vision map on read so caches persisted before the
	// flag existed (or before a map entry was added) still resolve correctly.
	// Per-model protocols are re-applied the same way. Both are idempotent:
	// neither downgrades a value the catalog already carried.
	EnrichModelsVision(providerType, models)
	EnrichModelsAPIFormat(providerType, models)

	entry := CacheEntry{Models: models, LastError: lastError.String}
	if lastRefreshedAt.Valid {
		t := lastRefreshedAt.Time
		entry.LastRefreshedAt = &t
	}
	return entry, nil
}

// SaveSuccess records a successful refresh: overwrites models, sets
// last_refreshed_at to now, clears any prior last_error.
func (c *ModelCache) SaveSuccess(providerType ProviderType, models []ModelInfo, at time.Time) error {
	payload, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("encode models for %q: %w", providerType, err)
	}
	_, err = c.db.ExecWrite(`
		INSERT INTO llm_provider_model_cache (provider_type, models_json, last_refreshed_at, last_error, updated_at)
		VALUES (?, ?, ?, NULL, ?)
		ON CONFLICT(provider_type) DO UPDATE SET
			models_json       = excluded.models_json,
			last_refreshed_at = excluded.last_refreshed_at,
			last_error        = NULL,
			updated_at        = excluded.updated_at
	`, string(providerType), string(payload), at, at)
	if err != nil {
		return fmt.Errorf("save model cache for %q: %w", providerType, err)
	}
	return nil
}

// SaveFailure records a failed refresh attempt. Existing models_json is
// preserved (so airgapped admins keep the picker they had before the network
// went away); only last_error and updated_at change.
func (c *ModelCache) SaveFailure(providerType ProviderType, refreshErr error, at time.Time) error {
	errMsg := refreshErr.Error()
	// Try an UPDATE first — it preserves any prior cached models.
	res, err := c.db.ExecWrite(`
		UPDATE llm_provider_model_cache
		SET last_error = ?, updated_at = ?
		WHERE provider_type = ?
	`, errMsg, at, string(providerType))
	if err != nil {
		return fmt.Errorf("update model cache error for %q: %w", providerType, err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}
	// No prior row — insert an empty models entry just to remember the error.
	_, err = c.db.ExecWrite(`
		INSERT INTO llm_provider_model_cache (provider_type, models_json, last_refreshed_at, last_error, updated_at)
		VALUES (?, '[]', NULL, ?, ?)
	`, string(providerType), errMsg, at)
	if err != nil {
		return fmt.Errorf("insert model cache error for %q: %w", providerType, err)
	}
	return nil
}
