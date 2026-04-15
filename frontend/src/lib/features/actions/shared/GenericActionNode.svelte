<script>
  import { Handle } from '@xyflow/svelte';
  import { getHandlePositions } from '../nodes/flowDirection.js';

  let {
    data = {},
    selected = false,
    flowStore,
    icon: Icon,
    title,
    accentColor = 'teal',
    colorVars = null,
    body,
  } = $props();

  let positions = $derived(getHandlePositions(flowStore.direction));

  let colors = $derived(colorVars || {
    accent: `var(--ds-accent-${accentColor})`,
    subtle: `var(--ds-accent-${accentColor}-subtle)`,
    subtler: `var(--ds-accent-${accentColor}-subtler)`,
  });
</script>

<div
  class="generic-action-node action-flow-node"
  class:selected
  style="--_accent: {colors.accent}; --_accent-subtle: {colors.subtle}; --_accent-subtler: {colors.subtler};"
>
  <Handle type="target" position={positions.input} id="input" />

  <div class="node-header">
    <Icon size={16} />
    <span class="node-title">{title}</span>
  </div>
  <div class="node-body">
    {@render body()}
  </div>

  <Handle type="source" position={positions.output} id="output" />
</div>

<style>
  .generic-action-node {
    background-color: var(--ds-surface-raised);
    border: 2px solid var(--_accent);
    border-radius: 8px;
    min-width: 180px;
    box-shadow: var(--shadow-md);
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 12px;
    background-color: var(--_accent-subtle);
    border-bottom: 1px solid var(--_accent-subtler);
    border-radius: 6px 6px 0 0;
  }

  .node-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--_accent);
  }

  .node-body {
    padding: 10px 12px;
  }

  .generic-action-node :global(.placeholder) {
    font-size: 12px;
    color: var(--ds-text-subtlest);
    font-style: italic;
  }

  :global(.generic-action-node .svelte-flow__handle) {
    width: 10px;
    height: 10px;
    background-color: var(--_accent);
    border: 2px solid var(--ds-surface-raised);
  }
</style>
