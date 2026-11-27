import { fetchV2Data } from './core.js';

const base = '/admin/jira-csv';
const post = (path, value) =>
  fetchV2Data(`${base}/${path}`, { method: 'POST', body: JSON.stringify(value) });

export const jiraCsv = {
  upload(file, delimiter) {
    const body = new FormData();
    body.set('file', file);
    body.set('delimiter', delimiter);
    return fetchV2Data(`${base}/upload`, { method: 'POST', body });
  },
  catalog: (id = 0) => fetchV2Data(`${base}/catalog${id ? `?workspace_id=${id}` : ''}`),
  available: (key) => fetchV2Data(`${base}/key?key=${encodeURIComponent(key)}`),
  preview: (config) => post('preview', config),
  start: (config) => post('start', config),
  history: () => fetchV2Data(`${base}/jobs`),
  job: (id) => fetchV2Data(`${base}/jobs/${encodeURIComponent(id)}`),
};
