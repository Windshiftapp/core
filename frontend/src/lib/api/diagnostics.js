import { fetchAPI } from './core.js';

/**
 * Recent action execution logs across all workspaces (admin-only).
 * @param {Object} opts
 * @param {'failed'|'slowest'} [opts.mode='failed']
 * @param {string} [opts.since='24h'] - Go duration string ("24h", "1h", "15m")
 * @param {number} [opts.limit=25]
 */
export function getActionLogs(opts = {}) {
  const params = new URLSearchParams();
  if (opts.mode) params.set('mode', opts.mode);
  if (opts.since) params.set('since', opts.since);
  if (opts.limit != null) params.set('limit', String(opts.limit));
  const qs = params.toString();
  return fetchAPI(`/admin/diagnostics/action-logs${qs ? `?${qs}` : ''}`);
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
  const params = new URLSearchParams();
  if (opts.status) params.set('status', opts.status);
  if (opts.channelId) params.set('channel_id', String(opts.channelId));
  if (opts.since) params.set('since', opts.since);
  if (opts.limit != null) params.set('limit', String(opts.limit));
  const qs = params.toString();
  return fetchAPI(`/admin/diagnostics/webhook-deliveries${qs ? `?${qs}` : ''}`);
}

/**
 * Per-channel webhook delivery aggregates (admin-only).
 * @param {Object} opts
 * @param {string} [opts.since='24h']
 */
export function getWebhookStats(opts = {}) {
  const params = new URLSearchParams();
  if (opts.since) params.set('since', opts.since);
  const qs = params.toString();
  return fetchAPI(`/admin/diagnostics/webhook-stats${qs ? `?${qs}` : ''}`);
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
  const params = new URLSearchParams();
  if (opts.scheduler) params.set('scheduler', opts.scheduler);
  if (opts.status) params.set('status', opts.status);
  if (opts.since) params.set('since', opts.since);
  if (opts.limit != null) params.set('limit', String(opts.limit));
  const qs = params.toString();
  return fetchAPI(`/admin/diagnostics/scheduler-runs${qs ? `?${qs}` : ''}`);
}

/**
 * Per-scheduler aggregates (admin-only).
 * @param {Object} opts
 * @param {string} [opts.since='24h']
 */
export function getSchedulerStats(opts = {}) {
  const params = new URLSearchParams();
  if (opts.since) params.set('since', opts.since);
  const qs = params.toString();
  return fetchAPI(`/admin/diagnostics/scheduler-stats${qs ? `?${qs}` : ''}`);
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
 * Snapshot of the items.frac_index generator state.
 *
 * Returns:
 *   - cache: in-memory generator state (cached key, predicted next key, hit/miss counters)
 *   - db: persisted state (column collation, linguistic vs byte-wise max, top 10, predicted collision)
 *   - healthy: true when collation matches AND the predicted next key does not already exist
 *
 * @returns {Promise<{cache: object, db: object, healthy: boolean}>}
 */
export function getFracIndexState() {
  return fetchAPI('/admin/diagnostics/frac-index');
}
