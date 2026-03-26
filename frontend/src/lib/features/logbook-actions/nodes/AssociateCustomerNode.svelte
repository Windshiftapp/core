<script>
  import { Handle } from '@xyflow/svelte';
  import { Users } from 'lucide-svelte';
  import { logbookActionFlowStore } from '../../../stores/logbookActionFlowStore.svelte.js';
  import { getHandlePositions } from '../../actions/nodes/flowDirection.js';

  let { data = {}, selected = false } = $props();
  let positions = $derived(getHandlePositions(logbookActionFlowStore.direction));
</script>

<div class="customer-node action-flow-node" class:selected>
  <Handle type="target" position={positions.input} id="input" />
  <div class="node-header">
    <Users size={16} class="node-icon" />
    <span class="node-title">Associate Customer</span>
  </div>
  <div class="node-body">
    {#if data.config?.customer_organisation_id || data.config?.portal_customer_id}
      <div class="config-line">
        {#if data.config.customer_organisation_id}
          Org #{data.config.customer_organisation_id}
        {/if}
        {#if data.config.portal_customer_id}
          Customer #{data.config.portal_customer_id}
        {/if}
      </div>
    {:else}
      <div class="placeholder">Configure customer</div>
    {/if}
  </div>
  <Handle type="source" position={positions.output} id="output" />
</div>

<style>
  .customer-node {
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
  .node-icon { flex-shrink: 0; }
  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--ds-accent-purple);
  }
  .node-body { padding: 10px 12px; }
  .config-line {
    font-size: 12px;
    color: var(--ds-text-subtle);
  }
  .placeholder {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }
  :global(.customer-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--ds-accent-purple);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
