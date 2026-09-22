<script>
  import { FileText } from '@lucide/svelte';
  import { logbookActionFlowStore } from '../../../stores/logbookActionFlowStore.svelte.js';
  import BaseActionNode from '../../actions/shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();

  const colorVars = {
    accent: 'var(--ds-success)',
    subtle: 'var(--ds-success-subtle)',
    subtler: 'var(--ds-success-subtler)',
  };
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || logbookActionFlowStore} icon={FileText} title="Create Item" {colorVars}>
  {#snippet body()}
    {#if data.config?.title}
      <div class="config-line">{data.config.title}</div>
    {:else}
      <div class="placeholder">Configure item creation</div>
    {/if}
  {/snippet}
</BaseActionNode>


