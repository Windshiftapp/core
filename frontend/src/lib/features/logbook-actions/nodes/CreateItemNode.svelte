<script>
  import { FileText } from 'lucide-svelte';
  import { logbookActionFlowStore } from '../../../stores/logbookActionFlowStore.svelte.js';
  import GenericActionNode from '../../actions/shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();

  const colorVars = {
    accent: 'var(--ds-success)',
    subtle: 'var(--ds-success-subtle)',
    subtler: 'var(--ds-success-subtler)',
  };
</script>

<GenericActionNode {data} {selected} flowStore={logbookActionFlowStore} icon={FileText} title="Create Item" {colorVars}>
  {#snippet body()}
    {#if data.config?.title}
      <div class="config-line">{data.config.title}</div>
    {:else}
      <div class="placeholder">Configure item creation</div>
    {/if}
  {/snippet}
</GenericActionNode>

<style>
  .config-line {
    font-size: 12px;
    color: var(--ds-text-subtle);
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
