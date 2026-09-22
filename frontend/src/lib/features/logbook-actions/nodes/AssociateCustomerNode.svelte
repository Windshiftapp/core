<script>
  import { Users } from '@lucide/svelte';
  import { logbookActionFlowStore } from '../../../stores/logbookActionFlowStore.svelte.js';
  import BaseActionNode from '../../actions/shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || logbookActionFlowStore} icon={Users} title="Associate Customer" accentColor="purple">
  {#snippet body()}
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
  {/snippet}
</BaseActionNode>


