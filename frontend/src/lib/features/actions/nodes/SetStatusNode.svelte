<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { RefreshCw } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';

  export let data = {};
  export let selected = false;
</script>

<div class="set-status-node">
  <Handle type="target" position={Position.Left} id="input" />

  <div class="node-header">
    <RefreshCw size={16} class="node-icon" />
    <span class="node-title">{t('actions.nodes.setStatus')}</span>
  </div>
  <div class="node-body">
    {#if data.config?.status_name}
      <div class="status-badge" style:background-color={data.config.status_color || 'var(--ds-accent-blue)'}>
        {data.config.status_name}
      </div>
    {:else if data.config?.status_id}
      <div class="status-id">ID: {data.config.status_id}</div>
    {:else}
      <div class="placeholder">{t('actions.config.selectStatus')}</div>
    {/if}
  </div>

  <Handle type="source" position={Position.Right} id="output" />
</div>

<style>
  .set-status-node {
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

  .status-badge {
    display: inline-block;
    padding: 4px 10px;
    border-radius: 12px;
    font-size: 12px;
    font-weight: 500;
    color: white;
  }

  .status-id {
    font-size: 12px;
    color: var(--ds-text-subtle);
    font-family: monospace;
  }

  .placeholder {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }

  :global(.set-status-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-teal);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
