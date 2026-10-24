<script>
  import Tag from '@lucide/svelte/icons/tag';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import BaseActionNode from '../shared/BaseActionNode.svelte';

  let { data = {}, selected = false } = $props();

  let added = $derived(data.config?.add_label_ids?.length || 0);
  let removed = $derived(data.config?.remove_label_ids?.length || 0);
</script>

<BaseActionNode {data} {selected} flowStore={data.flowStore || actionFlowStore} icon={Tag} title={t('actions.nodes.adjustLabels')} accentColor="purple">
  {#snippet body()}
    {#if added || removed}
      <div class="labels-info">
        {#if added}<span class="label-add" data-testid="adjust-labels-added">+{added}</span>{/if}
        {#if removed}<span class="label-remove" data-testid="adjust-labels-removed">-{removed}</span>{/if}
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.selectLabels')}</div>
    {/if}
  {/snippet}
</BaseActionNode>

<style>
  .labels-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }

  .label-add {
    color: var(--ds-accent-green);
    font-weight: 600;
  }

  .label-remove {
    color: var(--ds-accent-red);
    font-weight: 600;
  }
</style>
