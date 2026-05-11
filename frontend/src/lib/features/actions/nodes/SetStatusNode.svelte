<script>
  import { RefreshCw } from '@lucide/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import StatusBadge from '../../../components/StatusBadge.svelte';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();

  function getStatus(statusId) {
    if (!statusId || !data.statuses) return null;
    return data.statuses.find(s => s.id === statusId);
  }

  let status = $derived(data.config?.status_id ? getStatus(data.config.status_id) : null);
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={RefreshCw} title={t('actions.nodes.setStatus')} accentColor="teal">
  {#snippet body()}
    {#if status}
      <StatusBadge {status} />
    {:else if data.config?.status_id}
      <div class="status-id">ID: {data.config.status_id}</div>
    {:else}
      <div class="placeholder">{t('actions.config.selectStatus')}</div>
    {/if}
  {/snippet}
</GenericActionNode>

<style>
  .status-id {
    font-size: 12px;
    color: var(--ds-text-subtle);
    font-family: monospace;
  }
</style>
