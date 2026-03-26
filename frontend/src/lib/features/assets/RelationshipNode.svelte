<script>
  import { Handle, Position } from '@xyflow/svelte';
  import { IconBox, IconFileText, IconTestPipe } from '@tabler/icons-svelte-runes';

  let { data } = $props();

  const typeConfig = {
    asset: { icon: IconBox, color: '#3b82f6', label: 'Asset' },
    item: { icon: IconFileText, color: '#22c55e', label: 'Item' },
    test_case: { icon: IconTestPipe, color: '#a855f7', label: 'Test Case' },
  };

  const config = $derived(typeConfig[data.type] || typeConfig.asset);
  const subtitle = $derived(
    data.metadata?.display_key || data.metadata?.asset_type || config.label
  );
  const status = $derived(data.metadata?.status || '');
</script>

<div
  class="relationship-node"
  style="border-left-color: {config.color}; {data.is_origin ? 'border-width: 2px;' : ''}"
>
  <Handle type="target" position={Position.Left} />
  <Handle type="source" position={Position.Right} />

  <div class="node-content">
    <div class="node-header">
      <svelte:component this={config.icon} size={14} style="color: {config.color}; flex-shrink: 0;" />
      <span class="node-title" title={data.title}>{data.title}</span>
    </div>
    <div class="node-subtitle">
      <span>{subtitle}</span>
      {#if status}
        <span class="node-status">{status}</span>
      {/if}
    </div>
  </div>
</div>

<style>
  .relationship-node {
    background: var(--ds-surface-raised, #fff);
    border: 1px solid var(--ds-border, #e5e7eb);
    border-left-width: 3px;
    border-left-style: solid;
    border-radius: 6px;
    padding: 8px 10px;
    min-width: 140px;
    max-width: 200px;
    font-size: 12px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.08);
    cursor: pointer;
    transition: background 0.15s ease;
  }

  .relationship-node:hover {
    background: var(--ds-surface-hovered, #f9fafb);
  }

  .node-content {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .node-header {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .node-title {
    font-weight: 600;
    color: var(--ds-text, #111);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .node-subtitle {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--ds-text-subtle, #6b7280);
    font-size: 10px;
  }

  .node-status {
    background: var(--ds-surface, #f3f4f6);
    padding: 0 4px;
    border-radius: 3px;
    font-size: 9px;
  }

  :global(.relationship-node .svelte-flow__handle) {
    width: 6px !important;
    height: 6px !important;
    background: var(--ds-border, #d1d5db) !important;
    border: none !important;
  }
</style>
