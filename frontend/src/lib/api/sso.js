import { fetchAPI } from './core.js';

// SSO (Single Sign-On) endpoints
export const sso = {
  // Public endpoints (no auth required)
  getStatus: () => fetchAPI('/sso/status'),

  // Start SSO login (returns redirect URL)
  startLogin: (slug, providerType = 'oidc', rememberMe = false) => {
    const params = new URLSearchParams();
    if (rememberMe) params.append('remember_me', 'true');
    const query = params.toString();
    // SAML and OIDC have different login URL patterns
    const basePath = providerType === 'saml'
      ? `/api/sso/${slug}/saml/login`
      : `/api/sso/login/${slug}`;
    return `${basePath}${query ? `?${query}` : ''}`;
  },

  // Admin endpoints (require system.admin)
  listProviders: () => fetchAPI('/sso/providers'),
  getProvider: (id) => fetchAPI(`/sso/providers/${id}`),
  createProvider: (data) =>
    fetchAPI('/sso/providers', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateProvider: (id, data) =>
    fetchAPI(`/sso/providers/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteProvider: (id) =>
    fetchAPI(`/sso/providers/${id}`, {
      method: 'DELETE',
    }),
  testProvider: (id) =>
    fetchAPI(`/sso/providers/${id}/test`, {
      method: 'POST',
    }),

  // User external accounts (require auth)
  getExternalAccounts: () => fetchAPI('/sso/external-accounts'),
  unlinkExternalAccount: (id) =>
    fetchAPI(`/sso/external-accounts/${id}`, {
      method: 'DELETE',
    }),
};
