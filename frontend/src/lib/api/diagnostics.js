import { fetchAPI } from './core.js';

function diagnosticsQuery(endpoint, opts, overrides = {}) {
  const params = new URLSearchParams();
  if (opts.since) params.set('since', opts.since);
  if (opts.limit != null) params.set('limit', String(opts.limit));
  for (const [optKey, paramKey] of Object.entries(overrides)) {
    if (opts[optKey] != null) {
      params.set(paramKey, String(opts[optKey]));
    }
  }
  const qs = params.toString();
  return `/admin/diagnostics/${endpoint}${qs ? `?${qs}` : ''}`;
}

/**
 * Recent action execution logs across all workspaces (admin-only).
 * @param {Object} opts
 * @param {'failed'|'slowest'} [opts.mode='failed']
 * @param {string} [opts.since='24h'] - Go duration string ("24h", "1h", "15m")
 * @param {number} [opts.limit=25]
 */
export function getActionLogs(opts = {}) {
  return fetchAPI(diagnosticsQuery('action-logs', opts, { mode: 'mode' }));
}

/**
 * Recent outbound webhook delivery rows (admin-only).
 * @param {Object} opts
 * @param {''|'failed'|'success'} [opts.status]
 * @param {number} [opts.channelId]
 * @param {string} [opts.since='24h']
 * @param {number} [opts.limit=25]
 */
export function getWebhookDeliveries(opts = {}) {
  return fetchAPI(
    diagnosticsQuery('webhook-deliveries', opts, {
      status: 'status',
      channelId: 'channel_id',
    })
  );
}

/**
 * Per-channel webhook delivery aggregates (admin-only).
 * @param {Object} opts
 * @param {string} [opts.since='24h']
 */
export function getWebhookStats(opts = {}) {
  return fetchAPI(diagnosticsQuery('webhook-stats', opts));
}

/**
 * Process-local bounded webhook dispatch pipeline state (admin-only).
 * @returns {Promise<object>}
 */
export function getWebhookDispatchStats() {
  return fetchAPI('/admin/diagnostics/webhook-dispatch');
}

/**
 * Manually delete webhook delivery rows older than the given duration.
 * @param {string} olderThan - e.g. "30d", "168h"
 * @returns {Promise<{deleted: number}>}
 */
export function purgeWebhookDeliveries(olderThan) {
  return fetchAPI('/admin/diagnostics/webhook-deliveries/purge', {
    method: 'POST',
    body: JSON.stringify({ older_than: olderThan }),
  });
}

/**
 * Recent in-process scheduler tick history (admin-only).
 * @param {Object} opts
 * @param {''|'briefing'|'email'|'recurrence'|'notification'} [opts.scheduler]
 * @param {''|'success'|'failed'} [opts.status]
 * @param {string} [opts.since='24h']
 * @param {number} [opts.limit=25]
 */
export function getSchedulerRuns(opts = {}) {
  return fetchAPI(
    diagnosticsQuery('scheduler-runs', opts, {
      scheduler: 'scheduler',
      status: 'status',
    })
  );
}

/**
 * Per-scheduler aggregates (admin-only).
 * @param {Object} opts
 * @param {string} [opts.since='24h']
 */
export function getSchedulerStats(opts = {}) {
  return fetchAPI(diagnosticsQuery('scheduler-stats', opts));
}

/**
 * Manually delete scheduler run rows older than the given duration.
 * @param {string} olderThan
 * @returns {Promise<{deleted: number}>}
 */
export function purgeSchedulerRuns(olderThan) {
  return fetchAPI('/admin/diagnostics/scheduler-runs/purge', {
    method: 'POST',
    body: JSON.stringify({ older_than: olderThan }),
  });
}

/**
 * Snapshot of the items.frac_index persisted state.
 *
 * Returns:
 *   - db: persisted state (column collation, linguistic vs byte-wise max, top 10,
 *         not-null count, predicted next key, predicted collision)
 *   - healthy: true when collation matches AND the predicted next key does not already exist
 *
 * @returns {Promise<{db: object, healthy: boolean}>}
 */
export function getFracIndexState() {
  return fetchAPI('/admin/diagnostics/frac-index');
}

/**
 * Per-provider LLM model-catalog cache state plus enabled-connection drift
 * (configured model still in the cached catalog?).
 *
 * @returns {Promise<Array<{type: string, name: string, has_dynamic_models: boolean,
 *   last_refreshed_at?: string, last_error?: string, models_cached_count: number,
 *   connections: Array<{id: number, name: string, model: string, model_still_in_catalog?: boolean}>}>>}
 */
export function getLLMProviderStatus() {
  return fetchAPI('/admin/diagnostics/llm-providers');
}

/**
 * Recent daily_briefings rows where the LLM call failed, bucketed by error class.
 * @param {Object} opts
 * @param {string} [opts.since='24h']
 */
export function getBriefingFailures(opts = {}) {
  return fetchAPI(diagnosticsQuery('briefing-failures', opts));
}

/**
 * Daily-briefing generation state: feature resolution, the last scheduler tick,
 * per-user coverage, and the latest briefing row per user. Complements
 * getBriefingFailures, which only sees rows that recorded an error.
 * @param {Object} opts
 * @param {string} [opts.since='24h']
 * @param {number} [opts.limit=100]
 */
export function getBriefingHealth(opts = {}) {
  return fetchAPI(diagnosticsQuery('briefing-health', opts));
}

/**
 * Per-pool runner health: live/stale/revoked runners vs queued/running runs.
 * healthy=false means queued work with no live runner to claim it.
 *
 * @returns {Promise<Array<{id: number, name: string, enabled: boolean,
 *   max_concurrent_runs: number, live_runners: number, stale_runners: number,
 *   revoked_runners: number, last_heartbeat_at?: string, queued_runs: number,
 *   running_runs: number, oldest_queued_seconds?: number, healthy: boolean}>>}
 */
export function getRunnerPools() {
  return fetchAPI('/admin/diagnostics/runner-pools');
}

/**
 * Process-local SQL pool, PostgreSQL capacity-budget, and runtime state.
 * The legacy `pool` field is the main pool; new consumers should use `pools`.
 *
 * @returns {Promise<{instance: string, sampled_at: string, healthy: boolean,
 *   pools: Array<object>, capacity?: object, process: object}>}
 */
export function getDatabasePools() {
  return fetchAPI('/admin/diagnostics/database-pool');
}

/** Process memory budget and live cache allocation/eviction counters. */
export function getCacheMemory() {
  return fetchAPI('/admin/diagnostics/cache-memory');
}

/** Durable health for scheduled repository sync and pull-request refresh operations. */
export function getSCMConnectionHealth() {
  return fetchAPI('/admin/diagnostics/scm-connections');
}

/**
 * Machine-generated inbound mail (bounces and auto-replies) that intake
 * deliberately did not turn into work items, plus per-category counts.
 *
 * @param {Object} opts
 * @param {string} [opts.since='24h']
 * @param {number} [opts.limit=25]
 * @returns {Promise<{since: string, counts: Array<{category: string, count: number}>,
 *   messages: Array<object>}>}
 */
export function getEmailIntakeDiagnostics(opts = {}) {
  return fetchAPI(diagnosticsQuery('email-intake', opts));
}

/**
 * Global recurrence-rule cardinality and scheduler queue pressure.
 *
 * @returns {Promise<object>}
 */
export function getRecurrenceVolume() {
  return fetchAPI('/admin/diagnostics/recurrence-volume');
}

/**
 * Update the recurrence-volume warning diagnostic. The hard workspace quota
 * is intentionally not configurable.
 *
 * @param {{diagnostic_enabled: boolean, warning_threshold: number}} settings
 * @returns {Promise<object>}
 */
export function updateRecurrenceVolumeSettings(settings) {
  return fetchAPI('/admin/diagnostics/recurrence-volume', {
    method: 'PUT',
    body: JSON.stringify(settings),
  });
}

/** Durable domain-event delivery health and terminal failures. */
export function getDomainEventDiagnostics(opts = {}) {
  return fetchAPI(
    diagnosticsQuery('domain-events', opts, {
      consumerKey: 'consumer_key',
      workspaceId: 'workspace_id',
    })
  );
}

/** Schedule a terminal domain-event delivery for another attempt. */
export function replayDomainEvent(eventId, consumerKey, reason) {
  return changeDomainEventDelivery(eventId, consumerKey, 'replay', reason);
}

/** Explicitly skip a terminal delivery and unblock its aggregate. */
export function skipDomainEvent(eventId, consumerKey, reason) {
  return changeDomainEventDelivery(eventId, consumerKey, 'skip', reason);
}

function changeDomainEventDelivery(eventId, consumerKey, action, reason) {
  return fetchAPI(
    `/admin/diagnostics/domain-events/${eventId}/${encodeURIComponent(consumerKey)}/${action}`,
    {
      method: 'POST',
      body: JSON.stringify({ reason }),
    }
  );
}
