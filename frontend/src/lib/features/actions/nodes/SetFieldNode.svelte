<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { Pencil } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import { getHandlePositions } from './flowDirection.js';

  export let data = {};
  export let selected = false;

  $: positions = getHandlePositions(actionFlowStore.direction);
</script>

<div class="set-field-node action-flow-node" class:selected>
  <Handle type="target" position={positions.input} id="input" />

  <div class="node-header">
    <Pencil size={16} class="node-icon" />
    <span class="node-title">{t('actions.nodes.setField')}</span>
  </div>
  <div class="node-body">
    {#if data.config?.field_name}
      <div class="field-info">
        <span class="field-name">{data.config.field_name}</span>
        <span class="field-arrow">→</span>
        <span class="field-value">{data.config.value || '...'}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.selectField')}</div>
    {/if}
  </div>

  <Handle type="source" position={positions.output} id="output" />
</div>

<style>
  .set-field-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--ds-accent-purple);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--ds-accent-purple-subtle);
    border-bottom: 1px solid var(--ds-accent-purple-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-icon {
    flex-shrink: 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-accent-purple);
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
    font-family: monospace;
    font-size: 11px;
    background-color: var(--ds-surface-sunken);
    padding: 2px 6px;
    border-radius: 4px;
    max-width: 100px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .placeholder {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }

  :global(.set-field-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-purple);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
