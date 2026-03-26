<script>
  import { Handle } from '@xyflow/svelte';
  import { FileText } from 'lucide-svelte';
  import { logbookActionFlowStore } from '../../../stores/logbookActionFlowStore.svelte.js';
  import { getHandlePositions } from '../../actions/nodes/flowDirection.js';

  let { data = {}, selected = false } = $props();
  let positions = $derived(getHandlePositions(logbookActionFlowStore.direction));
</script>

<div class="create-item-node action-flow-node" class:selected>
  <Handle type="target" position={positions.input} id="input" />
  <div class="node-header">
    <FileText size={16} class="node-icon" />
    <span class="node-title">Create Item</span>
  </div>
  <div class="node-body">
    {#if data.config?.title}
      <div class="config-line">{data.config.title}</div>
    {:else}
      <div class="placeholder">Configure item creation</div>
    {/if}
  </div>
  <Handle type="source" position={positions.output} id="output" />
</div>

<style>
  .create-item-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--ds-success);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }
  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--ds-success-subtle);
    border-bottom: 1px solid var(--ds-success-subtler);
    border-radius: 6px 6px 0 0;
  }
  .node-icon { flex-shrink: 0; }
  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-success);
  }
  .node-body { padding: 10px 12px; }
  .config-line {
    font-size: 12px;
    color: var(--ds-text-subtle);
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .placeholder {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }
  :global(.create-item-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-success);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
