import { fetchAPI } from './core.js';

export const forms = {
  getChannel: (slug) => fetchAPI(`/forms/${slug}`),
  getForms: (slug) => fetchAPI(`/forms/${slug}/forms`),
  getFormFields: (slug, formId) => fetchAPI(`/forms/${slug}/forms/${formId}/fields`),
  getCustomFields: (slug) => fetchAPI(`/forms/${slug}/custom-fields`),
  submit: (slug, data) =>
    fetchAPI(`/forms/${slug}/submit`, {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};
