<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { RefreshCw } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import { getHandlePositions } from './flowDirection.js';
  import StatusBadge from '../../../components/StatusBadge.svelte';

  let { data = {}, selected = false } = $props();

  let positions = $derived(getHandlePositions(actionFlowStore.direction));

  function getStatus(statusId) {
    if (!statusId || !data.statuses) return null;
    return data.statuses.find(s => s.id === statusId);
  }

  let status = $derived(data.config?.status_id ? getStatus(data.config.status_id) : null);
</script>

<div class="set-status-node action-flow-node" class:selected>
  <Handle type="target" position={positions.input} id="input" />

  <div class="node-header">
    <RefreshCw size={16} class="node-icon" />
    <span class="node-title">{t('actions.nodes.setStatus')}</span>
  </div>
  <div class="node-body">
    {#if status}
      <StatusBadge {status} />
    {:else if data.config?.status_id}
      <div class="status-id">ID: {data.config.status_id}</div>
    {:else}
      <div class="placeholder">{t('actions.config.selectStatus')}</div>
    {/if}
  </div>

  <Handle type="source" position={positions.output} id="output" />
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
