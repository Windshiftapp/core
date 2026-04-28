import { fetchAPI } from './core.js';
import { buildQueryString } from './utils.js';

/**
 * Build the standard CRUD shape (`getAll`, `get`, `create`, `update`, `delete`)
 * that ~25 API resource modules share.
 *
 * Plain (no `parentPath`):
 *   getAll(filters?)            → GET    {basePath}{?filters}
 *   get(id)                     → GET    {basePath}/{id}
 *   create(data)                → POST   {basePath}
 *   update(id, data)            → PUT    {basePath}/{id}
 *   delete(id)                  → DELETE {basePath}/{id}
 *
 * With `parentPath` (e.g. parentPath='/workspaces', basePath='/test-folders'):
 *   getAll(parentId, filters?)         → GET    /workspaces/{parentId}/test-folders{?filters}
 *   get(parentId, id)                  → GET    /workspaces/{parentId}/test-folders/{id}
 *   create(parentId, data)             → POST   /workspaces/{parentId}/test-folders
 *   update(parentId, id, data)         → PUT    /workspaces/{parentId}/test-folders/{id}
 *   delete(parentId, id)               → DELETE /workspaces/{parentId}/test-folders/{id}
 *
 * Options:
 *   - parentPath:    parent collection prefix; switches the methods to the
 *                    parent-scoped signatures shown above.
 *   - adminBasePath: when set, write methods (create/update/delete) target
 *                    this path instead of basePath. Reads stay on basePath.
 *                    Used for resources where mutations live under /admin.
 *
 * Bespoke methods (e.g. `channels.toggle`) sit alongside the factory output
 * via spread:
 *   export const channels = {
 *     ...createCrudClient('/channels'),
 *     toggle: (id) => fetchAPI(`/channels/${id}/toggle`, { method: 'PUT' }),
 *   };
 */
export function createCrudClient(basePath, options = {}) {
  const { parentPath, adminBasePath } = options;

  if (parentPath) {
    const collection = (parentId) => `${parentPath}/${parentId}${basePath}`;
    const item = (parentId, id) => `${collection(parentId)}/${id}`;
    return {
      getAll: (parentId, filters = {}) =>
        fetchAPI(`${collection(parentId)}${buildQueryString(filters)}`),
      get: (parentId, id) => fetchAPI(item(parentId, id)),
      create: (parentId, data) =>
        fetchAPI(collection(parentId), {
          method: 'POST',
          body: JSON.stringify(data),
        }),
      update: (parentId, id, data) =>
        fetchAPI(item(parentId, id), {
          method: 'PUT',
          body: JSON.stringify(data),
        }),
      delete: (parentId, id) =>
        fetchAPI(item(parentId, id), {
          method: 'DELETE',
        }),
    };
  }

  const writePath = adminBasePath ?? basePath;
  return {
    getAll: (filters = {}) => fetchAPI(`${basePath}${buildQueryString(filters)}`),
    get: (id) => fetchAPI(`${basePath}/${id}`),
    create: (data) =>
      fetchAPI(writePath, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id, data) =>
      fetchAPI(`${writePath}/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
      }),
    delete: (id) =>
      fetchAPI(`${writePath}/${id}`, {
        method: 'DELETE',
      }),
  };
}
