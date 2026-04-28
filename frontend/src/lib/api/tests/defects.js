import { fetchAPI } from '../core.js';
import { createCrudClient } from '../createCrudClient.js';

export const defects = {
  ...createCrudClient('/defects'),
  linkToStep: (data) =>
    fetchAPI('/defects/link-to-step', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
};
