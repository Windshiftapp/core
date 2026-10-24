import { API_BASE, fetchAPI } from './core.js';
import { createCrudClient } from './createCrudClient.js';

// Portal Auth API (magic link authentication for portal customers)
export const portalAuth = {
  // Request a magic link email. `next` is an optional same-portal path the
  // emailed verify link should return to after sign-in.
  requestMagicLink: (slug, email, next) =>
    fetchAPI(`/portal/${slug}/auth/request`, {
      method: 'POST',
      body: JSON.stringify(next ? { email, next } : { email }),
    }),

  // Verify a magic link token (returns session on success)
  verifyMagicLink: (slug, token) =>
    fetchAPI(`/portal/${slug}/auth/verify?token=${encodeURIComponent(token)}`),

  // Get current authenticated portal customer
  getCurrentCustomer: (slug) => fetchAPI(`/portal/${slug}/auth/me`),

  // Logout portal customer
  logout: (slug) =>
    fetchAPI(`/portal/${slug}/auth/logout`, {
      method: 'POST',
    }),
};

// Portal passkey (WebAuthn) API. Discoverable login: start returns a challenge
// and an opaque sessionId; complete sends the authenticator response back. The
// management endpoints require an active portal customer session.
export const portalPasskey = {
  startRegistration: (slug, credentialName) =>
    fetchAPI(`/portal/${slug}/credentials/webauthn/register/start`, {
      method: 'POST',
      body: JSON.stringify({ credential_name: credentialName }),
    }),
  completeRegistration: (slug, body) =>
    fetchAPI(`/portal/${slug}/credentials/webauthn/register/complete`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  list: (slug) => fetchAPI(`/portal/${slug}/credentials/webauthn`),
  remove: (slug, credentialId) =>
    fetchAPI(`/portal/${slug}/credentials/webauthn/${encodeURIComponent(credentialId)}`, {
      method: 'DELETE',
    }),
  startLogin: (slug) =>
    fetchAPI(`/portal/${slug}/auth/webauthn/login/start`, {
      method: 'POST',
    }),
  completeLogin: (slug, body) =>
    fetchAPI(`/portal/${slug}/auth/webauthn/login/complete`, {
      method: 'POST',
      body: JSON.stringify(body),
    }),
  dismissPrompt: (slug) =>
    fetchAPI(`/portal/${slug}/passkey-prompt/dismiss`, {
      method: 'POST',
    }),
};

// Portal API (uses fetchAPI for automatic CSRF handling)
export const portal = {
  get: (slug) => fetchAPI(`/portal/${slug}`),
  getBootstrap: (slug) => fetchAPI(`/portal/${slug}/bootstrap`),
  getUserBootstrap: (slug) => fetchAPI(`/portal/${slug}/user-bootstrap`),

  submit: (slug, data) =>
    fetchAPI(`/portal/${slug}/submit`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  searchKnowledgeBase: (slug, query) =>
    fetchAPI(`/portal/${slug}/knowledge-base/search`, {
      method: 'POST',
      body: JSON.stringify({ query }),
    }),

  /** Fetch one workspace page published through the portal knowledge base. */
  getKnowledgeBasePage: (slug, pageId, source = '') =>
    fetchAPI(
      `/portal/${slug}/knowledge-base/pages/${pageId}${source ? `?source=${encodeURIComponent(source)}` : ''}`
    ),

  /** Browse listing of published knowledge-base pages (titles + hierarchy). */
  listKnowledgeBasePages: (slug) => fetchAPI(`/portal/${slug}/knowledge-base/pages`),

  getMyRequests: (slug) => fetchAPI(`/portal/${slug}/my-requests`),

  getRequestDetail: (slug, itemId) => fetchAPI(`/portal/${slug}/requests/${itemId}`),

  getRequestComments: (slug, itemId) => fetchAPI(`/portal/${slug}/requests/${itemId}/comments`),

  addRequestComment: (slug, itemId, content) =>
    fetchAPI(`/portal/${slug}/requests/${itemId}/comments`, {
      method: 'POST',
      body: JSON.stringify({ content }),
    }),

  getRequestAttachments: (slug, itemId) =>
    fetchAPI(`/portal/${slug}/requests/${itemId}/attachments`),

  addRequestAttachment: (slug, itemId, file) => {
    const body = new FormData();
    body.append('file', file);
    return fetchAPI(`/portal/${slug}/requests/${itemId}/attachments`, {
      method: 'POST',
      body,
    });
  },

  // Session-authenticated download URL for one request attachment. Use as an
  // anchor href; the portal cookie authorizes the GET.
  requestAttachmentUrl: (slug, itemId, attachmentId) =>
    `${API_BASE}/portal/${slug}/requests/${itemId}/attachments/${attachmentId}/download`,

  // Get request type fields (portal-authenticated)
  getRequestTypeFields: (slug, requestTypeId) =>
    fetchAPI(`/portal/${slug}/request-types/${requestTypeId}/fields`),

  // Get custom fields used by this portal's request types (portal-authenticated)
  getCustomFields: (slug) => fetchAPI(`/portal/${slug}/custom-fields`),

  // Portal-side approvals — list pending approvals for the active customer (or
  // internal user with a customer link), open a single request, and decide.
  getMyApprovals: (slug, status = 'pending') =>
    fetchAPI(`/portal/${slug}/approvals/mine?status=${encodeURIComponent(status)}`),
  getApproval: (slug, id) => fetchAPI(`/portal/${slug}/approvals/${id}`),
  decideApproval: (slug, id, decision, comment = '') =>
    fetchAPI(`/portal/${slug}/approvals/${id}/decide`, {
      method: 'POST',
      body: JSON.stringify({ decision, comment }),
    }),

  // Portal request form drafts. One draft per (caller, request type); saving
  // upserts. getForRequestType returns null instead of throwing for 404 so the
  // form modal can use "no draft" as a normal control-flow signal.
  drafts: {
    list: (slug) => fetchAPI(`/portal/${slug}/drafts`),
    save: (slug, data) =>
      fetchAPI(`/portal/${slug}/drafts`, {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    getForRequestType: async (slug, requestTypeId) => {
      try {
        return await fetchAPI(`/portal/${slug}/drafts/${requestTypeId}`);
      } catch (err) {
        if (err?.status === 404) return null;
        throw err;
      }
    },
    delete: (slug, requestTypeId) =>
      fetchAPI(`/portal/${slug}/drafts/${requestTypeId}`, {
        method: 'DELETE',
      }),
  },
};

// Portal Customers Management (requires customers.manage permission)
export const portalCustomers = {
  ...createCrudClient('/portal-customers'),
  // Legacy alias retained for callers using `getById` instead of `get`.
  getById: (id) => fetchAPI(`/portal-customers/${id}`),
  getChannels: (id) => fetchAPI(`/portal-customers/${id}/channels`),
  getSubmissions: (id) => fetchAPI(`/portal-customers/${id}/submissions`),
  updateOrganisation: (id, customerOrganisationId) =>
    fetchAPI(`/portal-customers/${id}/organisation`, {
      method: 'PUT',
      body: JSON.stringify({ customer_organisation_id: customerOrganisationId }),
    }),
  deactivate: (id) =>
    fetchAPI(`/portal-customers/${id}/deactivate`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  activate: (id) =>
    fetchAPI(`/portal-customers/${id}/activate`, {
      method: 'POST',
      body: JSON.stringify({}),
    }),
};

// Contact Roles Management (requires customers.manage permission)
export const contactRoles = {
  ...createCrudClient('/contact-roles'),
  // Legacy alias retained for callers using `getById` instead of `get`.
  getById: (id) => fetchAPI(`/contact-roles/${id}`),
};

// Customer Organisations (requires customers.manage permission)
export const customerOrganisations = {
  ...createCrudClient('/customer-organisations'),
  getContacts: (id) => fetchAPI(`/customer-organisations/${id}/contacts`),
  getTickets: (id, requestOptions = {}) =>
    fetchAPI(`/customer-organisations/${id}/tickets`, requestOptions),
  getProjects: (id) => fetchAPI(`/customer-organisations/${id}/projects`),
  // Portal request-sharing settings (WI-1139).
  setRequestSharing: (id, data) =>
    fetchAPI(`/customer-organisations/${id}/request-sharing`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
};
