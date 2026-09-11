// API client for the coding-agent harness runs surface (WI-91 / WI-83).
// The runs endpoints back the workspace-admin "Agent runs" panel + the
// item-detail runs list. Event polling is the cheap shape — call
// listEventsAfter(runId, afterId) every few seconds and append to a
// local store, trimming what you don't need to render.

import { fetchV2Data } from './core.js';

/**
 * Append run-list pagination options to the query string.
 * @param {{ limit?: number, cursor?: string }} opts
 * @returns {string} query string with leading '?' when non-empty
 */
function runListQuery(opts) {
  const params = new URLSearchParams();
  if (opts.limit) params.set('page_size', String(opts.limit));
  if (opts.cursor) params.set('cursor', opts.cursor);
  const qs = params.toString();
  return qs ? `?${qs}` : '';
}

const eventCursors = new Map();

async function listEventsAfter(runId, afterId = 0, limit = 200) {
  const cached = eventCursors.get(runId);
  const params = new URLSearchParams({ page_size: String(limit) });
  if (cached?.lastId === afterId && cached.cursor) params.set('cursor', cached.cursor);
  const response = await fetchV2Data(`/agent-runs/${runId}/events?${params}`);
  const events = Array.isArray(response?.events) ? response.events : [];
  if (events.length > 0 && response?.next_cursor) {
    eventCursors.set(runId, {
      lastId: events[events.length - 1].id,
      cursor: response.next_cursor,
    });
  }
  return events;
}

export const agentRuns = {
  /**
   * List the workspace's recent agent runs.
   * @param {number} workspaceId
   * @param {{ limit?: number, cursor?: string }} [opts]
   */
  listForWorkspace: async (workspaceId, opts = {}) => {
    const response = await fetchV2Data(
      `/workspaces/${workspaceId}/agent-runs${runListQuery(opts)}`
    );
    return response?.runs || [];
  },

  /**
   * List the runs triggered against one work item (newest first) — backs
   * the item-detail "Agent log" tab (WI-260).
   * @param {number} itemId
   * @param {{ limit?: number, cursor?: string }} [opts]
   */
  listForItem: async (itemId, opts = {}) => {
    const response = await fetchV2Data(`/items/${itemId}/agent-runs${runListQuery(opts)}`);
    return response?.runs || [];
  },

  /** Get a single run by id. */
  get: (runId) => fetchV2Data(`/agent-runs/${runId}`),

  /**
   * Get the run's metered LLM usage totals: prompt/completion/total tokens,
   * cost_usd (null when rates are unknown), and call count (WI-494).
   */
  usage: (runId) => fetchV2Data(`/agent-runs/${runId}/usage`),

  /**
   * Poll the run's event stream. `afterId` is the highest event id the
   * caller has already rendered; pass 0 on the first call to get the
   * full backlog.
   */
  listEventsAfter,

  /**
   * Cancel an in-flight run (workspace admin). Idempotent. With { force: true }
   * the run's row is transitioned to canceled directly even if the cooperative
   * cancel can't reach the worker — the escape hatch for a phantom run (a runner
   * that lost its terminal report and keeps the run 'running'). WI-512.
   */
  cancel: (runId, { force = false } = {}) =>
    fetchV2Data(`/agent-runs/${runId}/cancel${force ? '?force=true' : ''}`, {
      method: 'POST',
    }),

  /**
   * Manually re-run the agent that last worked an item (the item agent-log
   * "Re-run" button). Enqueues a fresh run reusing the last run's binding.
   * Returns { started }: false means a run was already queued/running, so the
   * call was a no-op. Requires item.edit.
   */
  rerun: (itemId) =>
    fetchV2Data(`/items/${itemId}/agent-runs`, {
      method: 'POST',
    }),
};
