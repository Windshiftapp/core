import { fetchAllV2Pages, fetchV2Data } from './core.js';

// Runtime approvals — request listing, decisions, cancel/delegate, admin actions.
// Approval-set CRUD lives in approvalSets.js.
export const approvals = {
  // Inbox: requests where the caller is in the active approver pool.
  mine: (status = 'pending') =>
    fetchAllV2Pages(`/approvals/mine?status=${encodeURIComponent(status)}`),

  // Full timeline (all requests with steps + decisions) for an item.
  forItem: (itemId) => fetchAllV2Pages(`/items/${itemId}/approvals`),

  // Single request with full audit log.
  get: (id) => fetchV2Data(`/approvals/${id}`),

  // Record a decision against the active step the actor is in.
  // decision ∈ { 'approve', 'reject', 'comment' }
  decide: (id, decision, comment = '') =>
    fetchV2Data(`/approvals/${id}/decisions`, {
      method: 'POST',
      body: JSON.stringify({ decision, comment }),
    }),

  // Manual cancel — requestor or item.edit-permitted user.
  cancel: (id, comment = '') =>
    fetchV2Data(`/approvals/${id}/cancellation`, {
      method: 'POST',
      body: JSON.stringify({ comment }),
    }),

  // Hand the actor's seat in the active step pool to another user.
  delegate: (id, toUserId, comment = '') =>
    fetchV2Data(`/approvals/${id}/delegations`, {
      method: 'POST',
      body: JSON.stringify({ to_user_id: toUserId, comment }),
    }),

  // Admin: re-run approver resolution for an active step (re-reads field
  // values, leave records, etc.). Writes a 'reassign' audit row.
  refreshApprovers: (id, stepId, comment = '') =>
    fetchV2Data(`/approvals/${id}/steps/${stepId}/approver-refreshes`, {
      method: 'POST',
      body: JSON.stringify({ comment }),
    }),

  // Admin: run the configured escalation policy now (ignores escalation_due_at).
  escalate: (id, stepId) =>
    fetchV2Data(`/approvals/${id}/steps/${stepId}/escalations`, {
      method: 'POST',
    }),
};
