/** Load comments whose agent-owner attribution is already permission-filtered server-side. */
export function loadAttributedComments(apiClient, itemId, params = {}) {
  return apiClient.getComments(itemId, params);
}

/** Load item history whose agent-owner attribution is already permission-filtered server-side. */
export function loadAttributedItemHistory(apiClient, itemId) {
  return apiClient.items.getHistory(itemId);
}

export function agentOwnerName(entry) {
  return typeof entry?.agent_owner_name === 'string' ? entry.agent_owner_name : '';
}

/**
 * The source an agent acted through, as stamped on item_history.source. Only
 * the AI Chat is surfaced today; other agent surfaces (mcp, standard_agent)
 * either act as a real agent user — already covered by the is_agent marker —
 * or have no in-product history feed to annotate.
 */
const ANNOTATED_HISTORY_SOURCES = new Set(['ai_chat']);

/**
 * Whether a history row was written by the AI Chat on the acting user's
 * behalf. Distinct from `is_agent`, which means the author *is* an agent
 * account; this means a human's name is on a change the model made.
 */
export function isAIChatAttributed(entry) {
  return ANNOTATED_HISTORY_SOURCES.has(entry?.source);
}

/**
 * Most agent turns a single history view will fetch telemetry for. Hover is
 * lazy, so this is a fan-out ceiling rather than a prefetch budget: an item
 * with hundreds of chat-written rows must not be able to turn a scroll into
 * hundreds of usage requests. Newest runs win, matching the feed order.
 */
export const MAX_HISTORY_TELEMETRY_RUNS = 20;

/**
 * The run ids whose telemetry this history view may load, newest first, capped
 * at max. Only AI-chat groups qualify: the other agent surfaces have no
 * per-turn telemetry the history feed can attribute.
 */
export function historyTelemetryRunIDs(groups, max = MAX_HISTORY_TELEMETRY_RUNS) {
  const ids = [];
  for (const group of groups || []) {
    const runId = group?.agent_run_id;
    if (!runId || !isAIChatAttributed(group)) continue;
    if (ids.includes(runId)) continue;
    ids.push(runId);
    if (ids.length >= max) break;
  }
  return ids;
}
