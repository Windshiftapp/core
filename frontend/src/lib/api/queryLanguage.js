import { fetchAllV2Pages, fetchAPI, fetchV2Data } from './core.js';

export const queryLanguage = {
  getCatalog: () => fetchV2Data('/query-language/catalog'),

  async getValues(valueHelp) {
    if (!valueHelp?.endpoint) return [];

    if (valueHelp.api_version === 'v1') {
      return (await fetchAPI(valueHelp.endpoint)) || [];
    }
    if (valueHelp.paginated) {
      return fetchAllV2Pages(valueHelp.endpoint);
    }
    return (await fetchV2Data(valueHelp.endpoint)) || [];
  },
};
