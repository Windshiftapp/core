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
 * Parent-scoped — `parentPath` set, no `itemPath`. Reads and writes nest
 * under the parent (e.g. parentPath='/workspaces', basePath='/test-folders'):
 *   getAll(parentId, filters?)         → GET    /workspaces/{parentId}/test-folders{?filters}
 *   get(parentId, id)                  → GET    /workspaces/{parentId}/test-folders/{id}
 *   create(parentId, data)             → POST   /workspaces/{parentId}/test-folders
 *   update(parentId, id, data)         → PUT    /workspaces/{parentId}/test-folders/{id}
 *   delete(parentId, id)               → DELETE /workspaces/{parentId}/test-folders/{id}
 *
 * Nested-list / flat-item — `parentPath` AND `itemPath` set. Collection ops
 * nest under the parent; item ops live at a flat path. Used by the asset
 * hierarchy, where e.g. types are listed under `/asset-sets/{id}/types` but
 * mutated/read individually at `/asset-types/{id}`:
 *   getAll(parentId, filters?)         → GET    /asset-sets/{parentId}/types{?filters}
 *   get(id)                            → GET    /asset-types/{id}
 *   create(parentId, data)             → POST   /asset-sets/{parentId}/types
 *   update(id, data)                   → PUT    /asset-types/{id}
 *   delete(id)                         → DELETE /asset-types/{id}
 *
 * Options:
 *   - parentPath:    parent collection prefix; switches list/create to
 *                    nest under `${parentPath}/{parentId}${basePath}`.
 *   - itemPath:      when combined with parentPath, item ops (get/update/
 *                    delete) live at `${itemPath}/{id}` instead of nesting
 *                    under the parent.
 *   - adminBasePath: when set (plain mode only), write methods target this
 *                    path instead of basePath. Reads stay on basePath.
 *
 * Bespoke methods (e.g. `channels.toggle`) sit alongside the factory output
 * via spread:
 *   export const channels = {
 *     ...createCrudClient('/channels'),
 *     toggle: (id) => fetchAPI(`/channels/${id}/toggle`, { method: 'PUT' }),
 *   };
 */
export function createCrudClient(basePath, options = {}) {
  const { parentPath, itemPath, adminBasePath } = options;

  if (parentPath) {
    const collection = (parentId) => `${parentPath}/${parentId}${basePath}`;

    if (itemPath) {
      // Nested-list / flat-item: list+create under parent, item ops flat.
      const item = (id) => `${itemPath}/${id}`;
      return {
        getAll: (parentId, filters = {}) =>
          fetchAPI(`${collection(parentId)}${buildQueryString(filters)}`),
        get: (id) => fetchAPI(item(id)),
        create: (parentId, data) =>
          fetchAPI(collection(parentId), {
            method: 'POST',
            body: JSON.stringify(data),
          }),
        update: (id, data) =>
          fetchAPI(item(id), {
            method: 'PUT',
            body: JSON.stringify(data),
          }),
        delete: (id) =>
          fetchAPI(item(id), {
            method: 'DELETE',
          }),
      };
    }

    // Fully parent-scoped: every op nests under the parent.
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
