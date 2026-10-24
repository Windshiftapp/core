import { assetTypes } from '../../../api/assets.js';
import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';

/** Fetch reactive asset-type fields from the supplied ID getter. Late requests
 * cannot overwrite newer selections. */
export function useAssetTypeFields(getAssetTypeId) {
  let assetTypeFields = $state([]);
  let requestToken = 0;

  $effect(() => {
    const assetTypeId = getAssetTypeId();
    const token = ++requestToken;
    if (!assetTypeId) {
      assetTypeFields = [];
      return;
    }
    assetTypes
      .getFields(assetTypeId)
      .then((result) => {
        if (token === requestToken) assetTypeFields = result || [];
      })
      .catch((error) => {
        if (token !== requestToken) return;
        console.error('Failed to load asset type fields:', error);
        assetTypeFields = [];
      });
  });

  return {
    get fields() {
      return assetTypeFields;
    },
  };
}

/** Set an asset type and reset mappings plus caller-selected fields. */
export function applyAssetTypeChange(
  nodeId,
  rawValue,
  extraReset = {},
  flowStore = actionFlowStore
) {
  const value = parseInt(rawValue, 10) || 0;
  flowStore.updateNodeConfig(nodeId, {
    asset_type_id: value,
    field_mappings: [],
    ...extraReset,
  });
}

/** Persist mappings for a selected node. */
export function applyMappingsChange(nodeId, mappings, flowStore = actionFlowStore) {
  flowStore.updateNodeConfig(nodeId, { field_mappings: mappings });
}
