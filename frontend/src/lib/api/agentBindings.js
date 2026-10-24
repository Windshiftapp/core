// API client for the workspace-admin agent-binding surface (WI-88).
// Bindings tell the orchestrator which acting identity to use, which
// repo to mount, and which scopes the per-run ws token gets.

import { fetchAPI } from './core.js';

export const agentBindings = {
  /** List the member-safe Agent Studio catalog projection. */
  listCatalog: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/agent-profiles`),

  /** List the approved immutable templates available to workspace admins. */
  listTemplates: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/agent-templates`),

  /** Create an Agent Studio profile as a Draft. */
  createProfile: (workspaceId, body) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-profiles`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  /** Edit mutable overview fields with optimistic profile-version checking. */
  updateProfile: (workspaceId, id, body) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-profiles/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    }),

  /** Move a grandfathered Legacy local profile to an authorized runner. */
  migrateLegacyProfile: (workspaceId, id, targetPoolId) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-profiles/${id}/migrate-runner`, {
      method: 'POST',
      body: JSON.stringify({ target_pool_id: Number(targetPoolId) }),
    }),

  /** Authorize the first runner pool for an incomplete Coding Draft. */
  connectCodingRunner: (workspaceId, id, targetPoolId) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-profiles/${id}/connect-runner`, {
      method: 'POST',
      body: JSON.stringify({ target_pool_id: Number(targetPoolId) }),
    }),

  /** Run a private Standard test or queue a bounded Coding verification. */
  testProfile: (workspaceId, id, prompt = '') =>
    fetchAPI(`/workspaces/${workspaceId}/agent-profiles/${id}/test`, {
      method: 'POST',
      body: JSON.stringify({ prompt }),
    }),

  /** Check current dependencies without changing the profile lifecycle. */
  validateProfile: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-profiles/${id}/validation`),

  /** Promote a valid Draft or Paused profile to Ready. */
  activateProfile: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-profiles/${id}/ready`, {
      method: 'POST',
    }),

  /** List bindings configured in a workspace. */
  listForWorkspace: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/agent-bindings`),

  /** Fetch the effective server-managed prompt, including runtime overrides. */
  getStandardPrompt: (workspaceId) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-bindings/standard-prompt`),

  /**
   * Create a binding. The acting identity is validated server-side by
   * the WI-87 chokepoint; a 403 means the acting user isn't usable in
   * this workspace (owned by someone else, gated as a centralized
   * service user, etc.).
   *
   * @param {number} workspaceId
   * @param {{
   *   acting_user_id: number,
   *   repo_slug?: string,
   *   repo_remote_url?: string,
   *   repo_base_ref?: string,
   *   llm_connection_id?: number,
   *   scm_connection_id?: number,
   *   token_scopes?: string[],
   *   token_ttl_minutes?: number,
   *   max_runs_per_day?: number,
   * }} body
   */
  create: (workspaceId, body) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-bindings`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),

  /** Delete a binding by id. */
  remove: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-bindings/${id}`, {
      method: 'DELETE',
    }),

  /** Restore an archived profile as Draft with its stable identity/history. */
  restore: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-bindings/${id}/restore`, {
      method: 'POST',
    }),

  /**
   * Rewrite the binding's prompt-shaping config: custom instructions (the
   * persona appended to the run's initial prompt) + attached skill ids
   * (WI-258), plus the optional custom runner image for pool bindings (WI-450)
   * and the model connection (presence-aware). runner_image and
   * llm_connection_id are presence-aware: omit to leave the current value
   * untouched (pass "" for runner_image to clear it).
   * @param {number} workspaceId
   * @param {number} id
   * @param {{ instructions: string, skill_ids: number[], runner_image?: string, llm_connection_id?: number }} body
   */
  updateAgentConfig: (workspaceId, id, body) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-bindings/${id}/agent-config`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),

  /**
   * Edit an existing binding's mutable configuration (WI-450): LLM connection,
   * repositories, token scopes/TTL, daily budget, instructions, runner image
   * (pool bindings), and skills. The acting service user, workspace, and target
   * pool are fixed at create and not accepted here. runner_image is
   * presence-aware (omit to leave untouched, "" to clear).
   * @param {number} workspaceId
   * @param {number} id
   * @param {{ repos?: object[], llm_connection_id?: number, token_scopes?: string[],
   *   token_ttl_minutes?: number, max_runs_per_day?: number, instructions?: string,
   *   capability_groups?: string[], runner_image?: string, skill_ids?: number[] }} body
   */
  update: (workspaceId, id, body) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-bindings/${id}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    }),

  /** Test a binding's LLM, optionally including its cloned-root snapshot. A
   * repo error does not discard a working reply; blank prompts use a default. */
  testLLM: (workspaceId, id, prompt) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-bindings/${id}/test-llm`, {
      method: 'POST',
      body: JSON.stringify(prompt ? { prompt } : {}),
    }),

  /** Run an ephemeral read-only binding check: no work item, branch, or PR. */
  testRun: (workspaceId, id) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-bindings/${id}/test-run`, {
      method: 'POST',
    }),

  /** List candidate identities; creation revalidates this advisory picker. */
  getCandidates: (workspaceId) => fetchAPI(`/workspaces/${workspaceId}/agent-binding-candidates`),

  /** List the canonical capability groups exposed by the Standard runtime. */
  listToolCapabilities: (workspaceId) =>
    fetchAPI(`/workspaces/${workspaceId}/agent-tool-capabilities`),
};
