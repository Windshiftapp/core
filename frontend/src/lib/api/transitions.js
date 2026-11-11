import { fetchV2Data } from './core.js';

// Per-transition lookups beyond what's on workflows.js.
export const transitions = {
  // Returns { transition_id, conditions: [...], approval_drivers: [...],
  // screen: {id, name, field_count, required_field_count} | null }.
  // Used by FE to render override warnings and to bind a transition screen.
  governance: (transitionId) => fetchV2Data(`/transitions/${transitionId}/governance`),
  // Bind a screen to a transition (or clear it with null) and return the
  // refreshed governance. The binding saves immediately (WI-1835).
  setScreen: (transitionId, screenId) =>
    fetchV2Data(`/transitions/${transitionId}/screen`, {
      method: 'PUT',
      body: JSON.stringify({ screen_id: screenId ?? null }),
    }),
};
