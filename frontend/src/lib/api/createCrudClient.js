import { fetchAllV2Pages, fetchAPI, fetchV2Data } from './core.js';
import { buildQueryString } from './utils.js';

/** Build CRUD clients with parent-scoped, flat-item, or admin-write paths. */
export function createCrudClient(basePath, options = {}) {
  const {
    parentPath,
    itemPath,
    adminBasePath,
    readV2 = false,
    v2 = false,
    allV2 = false,
  } = options;
  const detailRead = readV2 || v2 || allV2 ? fetchV2Data : fetchAPI;
  const listRead = allV2 ? fetchAllV2Pages : detailRead;
  const write = v2 ? fetchV2Data : fetchAPI;
  const updateMethod = v2 ? 'PATCH' : 'PUT';
  const updateHeaders = v2 ? { 'Content-Type': 'application/merge-patch+json' } : undefined;

  if (parentPath) {
    const collection = (parentId) => `${parentPath}/${parentId}${basePath}`;

    if (itemPath) {
      // List/create nest under parent; item operations stay flat.
      const item = (id) => `${itemPath}/${id}`;
      return {
        getAll: (parentId, filters = {}, requestOptions = {}) =>
          listRead(`${collection(parentId)}${buildQueryString(filters)}`, requestOptions),
        get: (id, requestOptions = {}) => detailRead(item(id), requestOptions),
        create: (parentId, data) =>
          write(collection(parentId), {
            method: 'POST',
            body: JSON.stringify(data),
          }),
        update: (id, data) =>
          write(item(id), {
            method: updateMethod,
            headers: updateHeaders,
            body: JSON.stringify(data),
          }),
        delete: (id) =>
          write(item(id), {
            method: 'DELETE',
          }),
      };
    }

    // Every operation nests under the parent.
    const item = (parentId, id) => `${collection(parentId)}/${id}`;
    return {
      getAll: (parentId, filters = {}, requestOptions = {}) =>
        listRead(`${collection(parentId)}${buildQueryString(filters)}`, requestOptions),
      get: (parentId, id, requestOptions = {}) => detailRead(item(parentId, id), requestOptions),
      create: (parentId, data) =>
        write(collection(parentId), {
          method: 'POST',
          body: JSON.stringify(data),
        }),
      update: (parentId, id, data) =>
        write(item(parentId, id), {
          method: updateMethod,
          headers: updateHeaders,
          body: JSON.stringify(data),
        }),
      delete: (parentId, id) =>
        write(item(parentId, id), {
          method: 'DELETE',
        }),
    };
  }

  const writePath = adminBasePath ?? basePath;
  return {
    getAll: (filters = {}, requestOptions = {}) =>
      listRead(`${basePath}${buildQueryString(filters)}`, requestOptions),
    get: (id, requestOptions = {}) => detailRead(`${basePath}/${id}`, requestOptions),
    create: (data) =>
      write(writePath, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    update: (id, data) =>
      write(`${writePath}/${id}`, {
        method: updateMethod,
        headers: updateHeaders,
        body: JSON.stringify(data),
      }),
    delete: (id) =>
      write(`${writePath}/${id}`, {
        method: 'DELETE',
      }),
  };
}
