import { api } from '../../../api.js';

/**
 * Flatten a tree of asset categories (each potentially carrying `children`)
 * into a single-level array, annotating each node with its `level` so a
 * select can render indentation. Pure — used by AssetBrowser and AssetManager.
 */
export function flattenCategories(categories, level = 0) {
  let result = [];
  for (const cat of categories) {
    result.push({ ...cat, level });
    if (cat.children?.length > 0) {
      result = result.concat(flattenCategories(cat.children, level + 1));
    }
  }
  return result;
}

/**
 * Fetch categories under an asset set, requesting the tree shape. Returns []
 * on failure (matches the inline behaviour both AssetBrowser and AssetManager
 * already have).
 */
export async function fetchAssetCategories(setId) {
  if (!setId) return [];
  try {
    return (await api.assetCategories.getAll(setId, true)) || [];
  } catch (error) {
    console.error('Failed to load asset categories:', error);
    return [];
  }
}

/**
 * Fetch statuses under an asset set. Same failure contract as
 * `fetchAssetCategories`.
 */
export async function fetchAssetStatuses(setId) {
  if (!setId) return [];
  try {
    return (await api.assetStatuses.getAll(setId)) || [];
  } catch (error) {
    console.error('Failed to load statuses:', error);
    return [];
  }
}
