import { fetchAPI } from './core.js';

// Portal API (public endpoints, no authentication)
export const portal = {
  // Get portal configuration by slug
  get: async (slug) => {
    const response = await fetch(`/api/portal/${slug}`);
    if (!response.ok) {
      throw new Error(`Portal not found: ${response.statusText}`);
    }
    return response.json();
  },

  // Submit to portal (for future use)
  submit: async (slug, data) => {
    const response = await fetch(`/api/portal/${slug}/submit`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      credentials: 'same-origin', // Include session cookie for authenticated users
      body: JSON.stringify(data),
    });
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || 'Submission failed');
    }
    return response.json();
  },

  // Search knowledge base
  searchKnowledgeBase: async (slug, query) => {
    const response = await fetch(`/api/portal/${slug}/knowledge-base/search`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ query }),
    });
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || 'Knowledge base search failed');
    }
    return response.json();
  },

  // Get authenticated portal customer's requests
  getMyRequests: async (slug) => {
    const response = await fetch(`/api/portal/${slug}/my-requests`, {
      credentials: 'same-origin', // Include session cookie
    });
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || 'Failed to fetch requests');
    }
    return response.json();
  },

  // Get details for a specific request
  getRequestDetail: async (slug, itemId) => {
    const response = await fetch(`/api/portal/${slug}/requests/${itemId}`, {
      credentials: 'same-origin', // Include session cookie
    });
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || 'Failed to fetch request details');
    }
    return response.json();
  },

  // Get comments for a request
  getRequestComments: async (slug, itemId) => {
    const response = await fetch(`/api/portal/${slug}/requests/${itemId}/comments`, {
      credentials: 'same-origin', // Include session cookie
    });
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || 'Failed to fetch comments');
    }
    return response.json();
  },

  // Add a comment to a request
  addRequestComment: async (slug, itemId, content) => {
    const response = await fetch(`/api/portal/${slug}/requests/${itemId}/comments`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      credentials: 'same-origin', // Include session cookie
      body: JSON.stringify({ content }),
    });
    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(errorText || 'Failed to add comment');
    }
    return response.json();
  },
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
