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
