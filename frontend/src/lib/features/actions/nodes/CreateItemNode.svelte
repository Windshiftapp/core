<script>
  import FileText from '@lucide/svelte/icons/file-text';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();

  const colorVars = {
    accent: 'var(--ds-success)',
    subtle: 'var(--ds-success-subtle)',
    subtler: 'var(--ds-success-subtler)',
  };
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={FileText} title="Create Item" {colorVars}>
  {#snippet body()}
    {#if data.config?.title}
      <div class="config-line">{data.config.title}</div>
    {:else}
      <div class="placeholder">Configure item creation</div>
    {/if}
  {/snippet}
</BaseActionNode>


