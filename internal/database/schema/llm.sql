-- LLM connection management tables

CREATE TABLE IF NOT EXISTS llm_connections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    model TEXT NOT NULL,
    api_key_encrypted TEXT,
    base_url TEXT,
    provider_config TEXT,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS llm_provider_model_cache (
    provider_type     TEXT PRIMARY KEY,
    models_json       TEXT NOT NULL,
    last_refreshed_at DATETIME,
    last_error        TEXT,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Per-call LLM token usage + cost, metered at the broker (one row per
-- chat-completion). cost_usd is NULL when the provider catalog carries no
-- pricing; cost_source records how the cost was obtained.
CREATE TABLE IF NOT EXISTS llm_usage (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id            INTEGER NOT NULL,
    calls             INTEGER NOT NULL DEFAULT 1,
    model             TEXT NOT NULL,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens  INTEGER NOT NULL DEFAULT 0,
    cost_usd          REAL,
    cost_source       TEXT NOT NULL DEFAULT '',
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_llm_usage_run_id ON llm_usage(run_id);

-- migration: 20260617_llm_connections_provider_config
