import { items } from '../api/items.js';

// Resolve the transition-screen summary for a target status on an item, or
// null when the move has no screen. The available-transitions endpoint carries
// the summary so callers know whether to open the modal before moving (WI-1835).
export async function resolveTransitionScreen(itemId, targetStatusId) {
  try {
    const summary = await items.getAvailableStatusTransitions(itemId);
    const option = (summary?.available_transitions || []).find(
      (transition) => transition.id === targetStatusId
    );
    return option?.screen || null;
  } catch {
    // A failed lookup must not block the move; the server still enforces the
    // screen and returns screen_required.
    return null;
  }
}
