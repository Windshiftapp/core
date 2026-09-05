import { fetchV2Data } from './core.js';

export const queryLanguage = {
  getCatalog: () => fetchV2Data('/query-language/catalog'),

  async getValues(valueHelp, query = '') {
    if (!valueHelp?.source || !valueHelp?.value_field) return [];

    const params = new URLSearchParams({
      source: valueHelp.source,
      value_field: valueHelp.value_field,
    });
    if (query) params.set('q', query);
    return (await fetchV2Data(`/query-language/values?${params.toString()}`)) || [];
  },
};
