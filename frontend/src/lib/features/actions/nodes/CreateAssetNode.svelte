<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { PlusSquare } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';

  export let data = {};
  export let selected = false;
</script>

<div class="create-asset-node action-flow-node" class:selected>
  <Handle type="target" position={Position.Left} id="input" />

  <div class="node-header">
    <PlusSquare size={16} class="node-icon" />
    <span class="node-title">{t('actions.nodes.createAsset')}</span>
  </div>
  <div class="node-body">
    {#if data.config?.asset_type_id && data.config?.title}
      <div class="field-info">
        <span class="field-name">{data.config.title}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.configureAssetCreation')}</div>
    {/if}
  </div>

  <Handle type="source" position={Position.Right} id="output" />
</div>

<style>
  .create-asset-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--ds-accent-green);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--ds-accent-green-subtle);
    border-bottom: 1px solid var(--ds-accent-green-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-icon {
    flex-shrink: 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-accent-green);
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
    max-width: 160px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .placeholder {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }

  :global(.create-asset-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-green);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
