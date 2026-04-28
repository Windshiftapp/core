import { fetchAPI } from './core.js';

// Email Templates API — admin CRUD for built-in transactional email
// templates (magic_link, email_verification, invitation, notification_batch).
// Rows are seeded by the system; admins may edit but not create/delete.
export const emailTemplates = {
  getAll: () => fetchAPI('/email-templates'),

  get: (id) => fetchAPI(`/email-templates/${id}`),

  update: (id, data) =>
    fetchAPI(`/email-templates/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  preview: (data) =>
    fetchAPI('/email-templates/preview', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};
