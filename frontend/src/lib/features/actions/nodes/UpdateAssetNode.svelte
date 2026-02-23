<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { Database } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import { getHandlePositions } from './flowDirection.js';

  export let data = {};
  export let selected = false;

  $: positions = getHandlePositions(actionFlowStore.direction);
</script>

<div class="update-asset-node action-flow-node" class:selected>
  <Handle type="target" position={positions.input} id="input" />

  <div class="node-header">
    <Database size={16} class="node-icon" />
    <span class="node-title">{t('actions.nodes.updateAsset')}</span>
  </div>
  <div class="node-body">
    {#if data.config?.source_field_id}
      <div class="field-info">
        <span class="field-name">{data.config.source_field_id}</span>
        <span class="field-arrow">→</span>
        <span class="field-value">{t('actions.config.fieldMappings', { count: data.config.field_mappings?.length || 0 })}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.configureAssetUpdate')}</div>
    {/if}
  </div>

  <Handle type="source" position={positions.output} id="output" />
</div>

<style>
  .update-asset-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--ds-accent-teal);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--ds-accent-teal-subtle);
    border-bottom: 1px solid var(--ds-accent-teal-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-icon {
    flex-shrink: 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-accent-teal);
  }

  .node-body {
    padding: 10px 12px;
  }

  .field-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }

  .field-name {
    color: var(--ds-text);
    font-weight: 500;
  }

  .field-arrow {
    color: var(--ds-text-subtlest);
  }

  .field-value {
    color: var(--ds-text-subtle);
    font-size: 11px;
    background-color: var(--ds-surface-sunken);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .placeholder {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }

  :global(.update-asset-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-teal);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
