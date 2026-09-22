import { api } from '../api.js';

// One permission-profile request feeds both the global and workspace permission
// stores. Entries are scoped to an authenticated-shell generation so remounts
// and re-authentication get fresh data without duplicating the request within a
// single bootstrap.
const profiles = new Map();
let generation = 0;

function cacheKey(userId) {
  return `${generation}:${userId}`;
}

export function beginPermissionProfileGeneration() {
  generation += 1;
  profiles.clear();
  return generation;
}

export function loadPermissionProfile(userId) {
  if (!userId) {
    return Promise.resolve({ global_permissions: [], workspace_permissions: {} });
  }

  const key = cacheKey(userId);
  const cached = profiles.get(key);
  if (cached) return cached;

  const request = api.permissions.getUserPermissions(userId).catch((error) => {
    // Failed requests are retryable; only successful profiles stay cached for
    // the current authenticated-shell generation.
    profiles.delete(key);
    throw error;
  });
  profiles.set(key, request);
  return request;
}

export function clearPermissionProfiles() {
  generation += 1;
  profiles.clear();
}
