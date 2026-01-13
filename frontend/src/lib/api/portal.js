import { fetchAPI } from './core.js';

// Portal API (uses fetchAPI for automatic CSRF handling)
export const portal = {
  get: (slug) => fetchAPI(`/portal/${slug}`),

  submit: (slug, data) => fetchAPI(`/portal/${slug}/submit`, {
    method: 'POST',
    body: JSON.stringify(data),
  }),

  searchKnowledgeBase: (slug, query) => fetchAPI(`/portal/${slug}/knowledge-base/search`, {
    method: 'POST',
    body: JSON.stringify({ query }),
  }),

  getMyRequests: (slug) => fetchAPI(`/portal/${slug}/my-requests`),

  getRequestDetail: (slug, itemId) => fetchAPI(`/portal/${slug}/requests/${itemId}`),

  getRequestComments: (slug, itemId) => fetchAPI(`/portal/${slug}/requests/${itemId}/comments`),

  addRequestComment: (slug, itemId, content) => fetchAPI(`/portal/${slug}/requests/${itemId}/comments`, {
    method: 'POST',
    body: JSON.stringify({ content }),
  }),
};

// Portal Customers Management (requires customers.manage permission)
export const portalCustomers = {
  getAll: () => fetchAPI('/portal-customers'),
  create: (data) => fetchAPI('/portal-customers', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  }),
  getById: (id) => fetchAPI(`/portal-customers/${id}`),
  update: (id, data) => fetchAPI(`/portal-customers/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  }),
  delete: (id) => fetchAPI(`/portal-customers/${id}`, {
    method: 'DELETE'
  }),
  getChannels: (id) => fetchAPI(`/portal-customers/${id}/channels`),
  getSubmissions: (id) => fetchAPI(`/portal-customers/${id}/submissions`),
  updateOrganisation: (id, customerOrganisationId) => fetchAPI(`/portal-customers/${id}/organisation`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ customer_organisation_id: customerOrganisationId })
  }),
};

// Contact Roles Management (requires customers.manage permission)
export const contactRoles = {
  getAll: () => fetchAPI('/contact-roles'),
  getById: (id) => fetchAPI(`/contact-roles/${id}`),
  create: (data) => fetchAPI('/contact-roles', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  }),
  update: (id, data) => fetchAPI(`/contact-roles/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data)
  }),
  delete: (id) => fetchAPI(`/contact-roles/${id}`, {
    method: 'DELETE'
  }),
};

// Customer Organisations (requires customers.manage permission)
export const customerOrganisations = {
  getContacts: (id) => fetchAPI(`/customer-organisations/${id}/contacts`),
};
