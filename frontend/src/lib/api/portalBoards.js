import { fetchAPIV2, fetchV2Data } from './core.js';

export const portalBoards = {
  experience: (slug, options = {}) =>
    fetchAPIV2(`/portal/${encodeURIComponent(slug)}/experience`, options),
  list: (channelId) => fetchV2Data(`/channels/${channelId}/board-placements`),
  save: (channelId, placement) =>
    fetchV2Data(
      `/channels/${channelId}/board-placements${placement.id ? `/${encodeURIComponent(placement.id)}` : ''}`,
      {
        method: placement.id ? 'PUT' : 'POST',
        body: JSON.stringify(placement),
      }
    ),
  remove: (channelId, placement) =>
    fetchV2Data(
      `/channels/${channelId}/board-placements/${encodeURIComponent(placement.id)}?revision=${placement.revision}`,
      { method: 'DELETE' }
    ),
  client(slug, placementId) {
    const path = `/portal/${encodeURIComponent(slug)}/boards/${encodeURIComponent(placementId)}`;
    return {
      get: (_slug, options = {}) => fetchAPIV2(path, options),
      getItem: (_slug, key, options = {}) =>
        fetchAPIV2(`${path}/items/${encodeURIComponent(key)}`, options),
    };
  },
};
