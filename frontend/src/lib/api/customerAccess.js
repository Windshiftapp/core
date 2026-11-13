import { fetchAPIV2 } from './core.js';

const resource = (kind, key) =>
  `/customer-access/${encodeURIComponent(kind)}/${encodeURIComponent(key)}`;
export const customerAccess = {
  getPolicy: (kind, id) => fetchAPIV2(`${resource(kind, id)}/policy`),
  savePolicy: (kind, id, policy) =>
    fetchAPIV2(`${resource(kind, id)}/policy`, {
      method: 'PUT',
      body: JSON.stringify(policy),
    }),
  status: (kind, slug) => fetchAPIV2(`${resource(kind, slug)}/status`),
  login: (kind, slug, action, body = {}) =>
    fetchAPIV2(`${resource(kind, slug)}/auth/${action}`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
};
