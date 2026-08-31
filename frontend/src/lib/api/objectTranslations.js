import { fetchAPI } from './core.js';

function objectPath(objectType, objectId) {
  return `/admin/object-translations/${encodeURIComponent(objectType)}/${objectId}`;
}

export const objectTranslations = {
  list: (objectType, objectId) => fetchAPI(objectPath(objectType, objectId)),
  resolve: (locale, targets) =>
    fetchAPI('/admin/object-translations/resolve', {
      method: 'POST',
      body: JSON.stringify({ locale, targets }),
    }),
  upsert: (objectType, objectId, field, locale, value) =>
    fetchAPI(
      `${objectPath(objectType, objectId)}/${encodeURIComponent(field)}/${encodeURIComponent(locale)}`,
      {
        method: 'PUT',
        body: JSON.stringify({ value }),
      }
    ),
  delete: (objectType, objectId, field, locale) =>
    fetchAPI(
      `${objectPath(objectType, objectId)}/${encodeURIComponent(field)}/${encodeURIComponent(locale)}`,
      { method: 'DELETE' }
    ),
};
