import { fetchV2Data } from './core.js';
import { createCrudClient } from './createCrudClient.js';

export const statusCategories = createCrudClient('/status-categories', { v2: true });

const statusCRUD = createCrudClient('/statuses', { v2: true });

export function normalizeStatus(status) {
  if (!status?.category) return status;

  const { category } = status;
  return {
    ...status,
    category_id: status.category_id ?? category.id,
    category_name: status.category_name ?? category.name,
    category_display_name: status.category_display_name ?? category.display_name,
    category_builtin_key: status.category_builtin_key ?? category.builtin_key,
    category_color: status.category_color ?? category.color,
    is_completed: status.is_completed ?? category.is_completed,
  };
}

export function normalizeStatuses(items) {
  return (items ?? []).map(normalizeStatus);
}

export const statuses = {
  ...statusCRUD,
  getAll: async (...args) => normalizeStatuses(await statusCRUD.getAll(...args)),
  get: async (...args) => normalizeStatus(await statusCRUD.get(...args)),
  create: async (...args) => normalizeStatus(await statusCRUD.create(...args)),
  update: async (...args) => normalizeStatus(await statusCRUD.update(...args)),
};

const workflowCRUD = createCrudClient('/workflows', { v2: true });

function editableTransition(transition) {
  return {
    ...transition,
    from_status_id: transition.from?.id ?? null,
    to_status_id: transition.to.id,
  };
}

async function getTransitions(id) {
  const transitions = (await fetchV2Data(`/workflows/${id}/transitions`)) ?? [];
  return transitions.map(editableTransition);
}

async function getWorkflow(id, requestOptions = {}) {
  const [workflow, transitions] = await Promise.all([
    fetchV2Data(`/workflows/${id}`, requestOptions),
    getTransitions(id),
  ]);
  return { ...workflow, transitions };
}

async function getAllWithTransitions() {
  const items = (await workflowCRUD.getAll()) ?? [];
  return Promise.all(
    items.map(async (workflow) => ({ ...workflow, transitions: await getTransitions(workflow.id) }))
  );
}

export const workflows = {
  ...workflowCRUD,
  get: getWorkflow,
  getAllWithTransitions,
  getTransitions,
  updateTransitions: (id, data) =>
    fetchV2Data(`/workflows/${id}/transitions`, {
      method: 'PUT',
      body: JSON.stringify({ transitions: data }),
    }),
};
