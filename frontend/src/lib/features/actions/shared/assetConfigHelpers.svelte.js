import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';

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
