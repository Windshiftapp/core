<script>
  import { Users } from '@lucide/svelte';
  import { logbookActionFlowStore } from '../../../stores/logbookActionFlowStore.svelte.js';
  import GenericActionNode from '../../actions/shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<GenericActionNode {data} {selected} flowStore={logbookActionFlowStore} icon={Users} title="Associate Customer" accentColor="purple">
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
</GenericActionNode>

<style>
  .config-line {
    font-size: 12px;
    color: var(--ds-text-subtle);
  }
</style>
