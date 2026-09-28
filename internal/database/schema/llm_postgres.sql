-- LLM connection management tables (PostgreSQL)

CREATE TABLE IF NOT EXISTS llm_connections (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    model TEXT NOT NULL,
    api_key_encrypted TEXT,
    base_url TEXT,
    provider_config JSONB,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS llm_provider_model_cache (
    provider_type     TEXT PRIMARY KEY,
    models_json       TEXT NOT NULL,
    last_refreshed_at TIMESTAMPTZ,
    last_error        TEXT,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Per-call LLM token usage + cost, metered at the broker (one row per
-- chat-completion). cost_usd is NULL when the provider catalog carries no
-- pricing; cost_source records how the cost was obtained.
CREATE TABLE IF NOT EXISTS llm_usage (
    id                SERIAL PRIMARY KEY,
    run_id            INTEGER NOT NULL,
    calls             INTEGER NOT NULL DEFAULT 1,
    model             TEXT NOT NULL,
    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
    completion_tokens INTEGER NOT NULL DEFAULT 0,
    total_tokens      INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens  INTEGER NOT NULL DEFAULT 0,
    cost_usd          DOUBLE PRECISION,
    cost_source       TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_llm_usage_run_id ON llm_usage(run_id);
