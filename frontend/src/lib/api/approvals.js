import { fetchAPI } from './core.js';

// Runtime approvals — request listing, decisions, cancel/delegate, admin actions.
// Approval-set CRUD lives in approvalSets.js.
export const approvals = {
  // Inbox: requests where the caller is in the active approver pool.
  mine: (status = 'pending') => fetchAPI(`/approvals/mine?status=${encodeURIComponent(status)}`),

  // Full timeline (all requests with steps + decisions) for an item.
  forItem: (itemId) => fetchAPI(`/items/${itemId}/approvals`),

  // Single request with full audit log.
  get: (id) => fetchAPI(`/approvals/${id}`),

  // Record a decision against the active step the actor is in.
  // decision ∈ { 'approve', 'reject', 'comment' }
  decide: (id, decision, comment = '') =>
    fetchAPI(`/approvals/${id}/decide`, {
      method: 'POST',
      body: JSON.stringify({ decision, comment }),
    }),

  // Manual cancel — requestor or item.edit-permitted user.
  cancel: (id, comment = '') =>
    fetchAPI(`/approvals/${id}/cancel`, {
      method: 'POST',
      body: JSON.stringify({ comment }),
    }),

  // Hand the actor's seat in the active step pool to another user.
  delegate: (id, toUserId, comment = '') =>
    fetchAPI(`/approvals/${id}/delegate`, {
      method: 'POST',
      body: JSON.stringify({ to_user_id: toUserId, comment }),
    }),

  // Admin: re-run approver resolution for an active step (re-reads field
  // values, leave records, etc.). Writes a 'reassign' audit row.
  refreshApprovers: (id, stepId, comment = '') =>
    fetchAPI(`/approvals/${id}/steps/${stepId}/refresh-approvers`, {
      method: 'POST',
      body: JSON.stringify({ comment }),
    }),

  // Admin: run the configured escalation policy now (ignores escalation_due_at).
  escalate: (id, stepId) =>
    fetchAPI(`/approvals/${id}/steps/${stepId}/escalate`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
};
