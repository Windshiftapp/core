import { fetchV2Data } from './core.js';

function actionMutationData(data) {
  const {
    name,
    description,
    trigger_type,
    trigger_config,
    is_enabled,
    actor_user_id,
    allowed_role_ids,
    nodes,
    edges,
  } = data;
  return {
    name,
    description,
    trigger_type,
    trigger_config,
    is_enabled,
    actor_user_id,
    allowed_role_ids,
    nodes,
    edges,
  };
}

export const actions = {
  // getCatalog returns the workspace-scoped action catalog: every available
  // trigger and node type with its JSON-Schema config, plus the action
  // capabilities reachable from this workspace. The visual palette is
  // built from this rather than a hardcoded list so adding a node type
  // server-side automatically surfaces it in the editor.
  getCatalog: (workspaceId) => fetchV2Data(`/workspaces/${workspaceId}/action-catalog`),
  getAll: (workspaceId, requestOptions = {}) =>
    fetchV2Data(`/workspaces/${workspaceId}/actions`, requestOptions),
  get: (workspaceId, id) => fetchV2Data(`/workspaces/${workspaceId}/actions/${id}`),
  create: (workspaceId, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/actions`, {
      method: 'POST',
      body: JSON.stringify(actionMutationData(data)),
    }),
  update: (workspaceId, id, data) =>
    fetchV2Data(`/workspaces/${workspaceId}/actions/${id}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/merge-patch+json' },
      body: JSON.stringify(actionMutationData(data)),
    }),
  delete: (workspaceId, id) =>
    fetchV2Data(`/workspaces/${workspaceId}/actions/${id}`, {
      method: 'DELETE',
    }),
  execute: (workspaceId, actionId, itemId) =>
    fetchV2Data(`/workspaces/${workspaceId}/actions/${actionId}/execute`, {
      method: 'POST',
      body: JSON.stringify({ item_id: itemId }),
    }),
  getLogs: (workspaceId, actionId) =>
    fetchV2Data(`/workspaces/${workspaceId}/actions/${actionId}/logs`),
};

// Action templates: read-only registry shipped with the binary, plus
// instantiation into a workspace via snapshot copy.
export const actionTemplates = {
  list: () => fetchV2Data('/action-templates'),
  apply: (workspaceId, templateKey) =>
    fetchV2Data(`/workspaces/${workspaceId}/action-templates/${templateKey}/apply`, {
      method: 'POST',
    }),
};
