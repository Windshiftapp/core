-- Durable domain events and consumer delivery state.
-- This file is the immutable body of migration 20260827_domain_event_engine.
-- Evolve these tables with a new migration instead of editing this file.

CREATE TABLE IF NOT EXISTS domain_event_streams (
	aggregate_type TEXT NOT NULL,
	aggregate_id TEXT NOT NULL,
	last_sequence BIGINT NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (aggregate_type, aggregate_id)
);

CREATE TABLE IF NOT EXISTS domain_events (
	id BIGSERIAL PRIMARY KEY,
	event_key TEXT NOT NULL UNIQUE,
	workspace_id INTEGER,
	aggregate_type TEXT NOT NULL,
	aggregate_id TEXT NOT NULL,
	aggregate_sequence BIGINT NOT NULL CHECK (aggregate_sequence > 0),
	event_type TEXT NOT NULL,
	payload_version INTEGER NOT NULL CHECK (payload_version > 0),
	occurred_at TIMESTAMPTZ NOT NULL,
	recorded_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	actor_kind TEXT NOT NULL,
	actor_ref TEXT,
	source_kind TEXT NOT NULL,
	source_ref TEXT,
	correlation_id TEXT,
	causation_event_key TEXT,
	payload TEXT NOT NULL,
	UNIQUE (aggregate_type, aggregate_id, aggregate_sequence)
);

CREATE INDEX IF NOT EXISTS idx_domain_events_type_id
	ON domain_events(event_type, id);
CREATE INDEX IF NOT EXISTS idx_domain_events_workspace_id
	ON domain_events(workspace_id, id);
CREATE INDEX IF NOT EXISTS idx_domain_events_recorded_at
	ON domain_events(recorded_at, id);

CREATE TABLE IF NOT EXISTS domain_event_consumers (
	consumer_key TEXT PRIMARY KEY,
	handler_version INTEGER NOT NULL DEFAULT 1 CHECK (handler_version > 0),
	is_active BOOLEAN NOT NULL DEFAULT false,
	start_event_id BIGINT NOT NULL DEFAULT 1 CHECK (start_event_id > 0),
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS domain_event_subscriptions (
	consumer_key TEXT NOT NULL,
	event_type TEXT NOT NULL,
	PRIMARY KEY (consumer_key, event_type),
	FOREIGN KEY (consumer_key) REFERENCES domain_event_consumers(consumer_key) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_domain_event_subscriptions_type
	ON domain_event_subscriptions(event_type, consumer_key);

CREATE TABLE IF NOT EXISTS domain_event_consumer_streams (
	consumer_key TEXT NOT NULL,
	aggregate_type TEXT NOT NULL,
	aggregate_id TEXT NOT NULL,
	completed_sequence BIGINT NOT NULL DEFAULT 0 CHECK (completed_sequence >= 0),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (consumer_key, aggregate_type, aggregate_id),
	FOREIGN KEY (consumer_key) REFERENCES domain_event_consumers(consumer_key) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS domain_event_deliveries (
	event_id BIGINT NOT NULL,
	consumer_key TEXT NOT NULL,
	state TEXT NOT NULL DEFAULT 'pending'
		CHECK (state IN ('pending', 'leased', 'retry', 'failed', 'completed', 'skipped')),
	attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
	next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, -- earliest eligible time; lease expiry while leased
	lease_owner TEXT,
	lease_token TEXT,
	lease_expires_at TIMESTAMPTZ,
	last_error TEXT,
	completed_at TIMESTAMPTZ,
	skipped_by_kind TEXT,
	skipped_by_ref TEXT,
	skip_reason TEXT,
	skipped_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (event_id, consumer_key),
	FOREIGN KEY (event_id) REFERENCES domain_events(id) ON DELETE CASCADE,
	FOREIGN KEY (consumer_key) REFERENCES domain_event_consumers(consumer_key) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_domain_event_deliveries_runnable
	ON domain_event_deliveries(consumer_key, state, next_attempt_at, event_id);
-- Claimable deliveries are ordered by next_attempt_at across pending, retry,
-- and leased states, so a partial index serves the claim without a temp sort.
CREATE INDEX IF NOT EXISTS idx_domain_event_deliveries_claimable
	ON domain_event_deliveries(consumer_key, next_attempt_at, event_id)
	WHERE state IN ('pending', 'retry', 'leased');
CREATE INDEX IF NOT EXISTS idx_domain_event_deliveries_lease
	ON domain_event_deliveries(state, lease_expires_at);
CREATE INDEX IF NOT EXISTS idx_domain_event_deliveries_state
	ON domain_event_deliveries(state, updated_at);

CREATE TABLE IF NOT EXISTS domain_event_delivery_actions (
	id BIGSERIAL PRIMARY KEY,
	event_id BIGINT NOT NULL,
	consumer_key TEXT NOT NULL,
	action TEXT NOT NULL CHECK (action IN ('replay', 'skip')),
	operator_kind TEXT NOT NULL,
	operator_ref TEXT,
	reason TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (event_id, consumer_key)
		REFERENCES domain_event_deliveries(event_id, consumer_key) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_domain_event_delivery_actions_delivery
	ON domain_event_delivery_actions(event_id, consumer_key, created_at);
