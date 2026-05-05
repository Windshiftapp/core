<script>
  import { ArrowRightCircle } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import StatusBadge from '../../../components/StatusBadge.svelte';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();

  function getStatus(statusId) {
    if (!statusId || !data.statuses) return null;
    return data.statuses.find(s => s.id === statusId);
  }

  let target = $derived(data.config?.target || { mode: 'matching_terminal' });
  let status = $derived(target.mode === 'explicit' ? getStatus(target.status_id) : null);
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={ArrowRightCircle} title={t('actions.nodes.transitionItem', 'Transition item')} accentColor="teal">
  {#snippet body()}
    {#if target.mode === 'explicit'}
      {#if status}
        <StatusBadge {status} />
      {:else}
        <div class="placeholder">Select a target status</div>
      {/if}
    {:else if target.mode === 'category_name'}
      <div class="target-summary">
        <div>To category: <strong>{target.category_name || '(unset)'}</strong></div>
        <div class="subtle">Resolved per item's own workflow</div>
      </div>
    {:else}
      <div class="target-summary">
        <div>Mirror trigger's terminal category</div>
        <div class="subtle">Picks the matching terminal in the current item's workflow</div>
      </div>
    {/if}
  {/snippet}
</GenericActionNode>

<style>
  .target-summary {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 12px;
    color: var(--ds-text);
  }
  .subtle {
    color: var(--ds-text-subtle);
    font-size: 11px;
  }
  .placeholder {
    color: var(--ds-text-subtle);
    font-style: italic;
    font-size: 12px;
  }
</style>
