import { api } from '../../../api.js';
import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';

/**
 * Reactive `assetTypeFields` for the asset config panels.
 *
 * Tracks `selectedNode.data.config.asset_type_id` via the supplied getter and
 * fetches the field list whenever it changes; returns an object whose `fields`
 * getter exposes the current array.
 *
 * Usage:
 *   const assetFields = useAssetTypeFields(() => selectedNode?.data?.config?.asset_type_id);
 *   // template: assetFields.fields
 */
export function useAssetTypeFields(getAssetTypeId) {
  let assetTypeFields = $state([]);

  $effect(() => {
    const assetTypeId = getAssetTypeId();
    if (!assetTypeId) {
      assetTypeFields = [];
      return;
    }
    api.assetTypes
      .getFields(assetTypeId)
      .then((result) => {
        assetTypeFields = result || [];
      })
      .catch((error) => {
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

/**
 * Apply an asset-type change to the action flow store, resetting field_mappings
 * (always) plus any caller-supplied keys (e.g. Create resets category/status).
 */
export function applyAssetTypeChange(nodeId, rawValue, extraReset = {}) {
  const value = parseInt(rawValue, 10) || 0;
  actionFlowStore.updateNodeConfig(nodeId, {
    asset_type_id: value,
    field_mappings: [],
    ...extraReset,
  });
}

/**
 * Persist a field-mappings change against the selected node.
 */
export function applyMappingsChange(nodeId, mappings) {
  actionFlowStore.updateNodeConfig(nodeId, { field_mappings: mappings });
}
