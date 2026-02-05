-- LLM connection management tables (PostgreSQL)

CREATE TABLE IF NOT EXISTS llm_connections (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    model TEXT NOT NULL,
    api_key_encrypted TEXT,
    base_url TEXT,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS llm_connection_features (
    id SERIAL PRIMARY KEY,
    connection_id INTEGER NOT NULL REFERENCES llm_connections(id) ON DELETE CASCADE,
    feature TEXT NOT NULL,
    UNIQUE(connection_id, feature)
);
