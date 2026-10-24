import { items } from '../api/items.js';

/**
 * Load the mobile above-the-fold item graph through the shared summary
 * contract. SCM bodies and agent logs remain owned by their lazy panels.
 *
 * @param {number|string} itemId
 * @param {RequestInit} [requestOptions]
 */
export function loadMobileItemDetailSummary(itemId, requestOptions = {}) {
  return items.getDetailSummary(itemId, { ...requestOptions, surface: 'mobile' });
}
