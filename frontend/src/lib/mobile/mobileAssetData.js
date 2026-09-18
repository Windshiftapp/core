import { api } from '../api.js';

/**
 * @param {number} assetId
 */
export function loadMobileAssetSummary(assetId) {
  return api.assets.get(assetId);
}
