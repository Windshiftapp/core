import { fetchAPI } from './core.js';

// Per-transition lookups beyond what's on workflows.js.
export const transitions = {
  // Returns { transition_id, conditions: [...], approval_drivers: [...] }
  // Used by FE to render override warnings when conditions and approvals
  // both target the same workflow_transitions row.
  governance: (transitionId) => fetchAPI(`/transitions/${transitionId}/governance`),
};
