<script>
  import { Handle } from '@xyflow/svelte';
  import { Zap } from 'lucide-svelte';
  import { getHandlePositions } from '../nodes/flowDirection.js';

  let {
    data = {},
    selected = false,
    flowStore,
    triggerLabels = {},
    title = 'Trigger',
    configSummaryFn = null,
  } = $props();

  let positions = $derived(getHandlePositions(flowStore.direction));

  function getConfigSummary() {
    if (configSummaryFn) return configSummaryFn(data);
    return '';
  }
</script>

<div class="trigger-node action-flow-node" class:selected>
  <div class="node-header">
    <Zap size={16} class="node-icon" />
    <span class="node-title">{title}</span>
  </div>
  <div class="node-body">
    <div class="trigger-type">{triggerLabels[data.triggerType] || data.triggerType || 'Select trigger'}</div>
    {#if getConfigSummary()}
      <div class="trigger-config">{getConfigSummary()}</div>
    {/if}
  </div>

  <Handle type="source" position={positions.output} id="output" />
</div>

<style>
  .trigger-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--ds-accent-blue);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--ds-accent-blue-subtle);
    border-bottom: 1px solid var(--ds-accent-blue-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-icon {
    flex-shrink: 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-accent-blue);
  }

  .node-body {
    padding: 10px 12px;
  }

  .trigger-type {
    font-size: 13px;
    font-weight: 500;
    color: var(--ds-text);
  }

  .trigger-config {
    margin-top: 6px;
    font-size: 11px;
    color: var(--ds-text-subtle);
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .config-label {
    color: var(--ds-text-subtlest);
    margin-right: 4px;
  }

  .config-value {
    color: var(--ds-text-subtle);
  }

  :global(.trigger-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-blue);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
