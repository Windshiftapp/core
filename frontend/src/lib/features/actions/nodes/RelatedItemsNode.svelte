<script>
  import { GitFork } from '@lucide/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();

  let relation = $derived(data.config?.relation || 'descendants');
  let crossWorkspace = $derived(data.config?.cross_workspace !== false);
  let linkTypeId = $derived(data.config?.link_type_id ?? null);
  let linkDirection = $derived(data.config?.link_direction || 'both');

  const relationLabels = {
    descendants: 'All descendants',
    direct_children: 'Direct children only',
    ancestors: 'Ancestors',
    linked: 'Linked items',
  };

  const directionLabels = {
    outgoing: 'outgoing',
    incoming: 'incoming',
    both: 'both directions',
  };

  function linkTypeName(id) {
    if (id == null) return 'any link type';
    const lt = data.linkTypes?.find?.(t => t.id === id);
    return lt ? `"${lt.name}"` : `link type #${id}`;
  }
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={GitFork} title={t('actions.nodes.relatedItems', 'For each related item')} accentColor="indigo">
  {#snippet body()}
    <div class="related-summary">
      <div class="row"><span class="label">Relation:</span> {relationLabels[relation] || relation}</div>
      {#if relation === 'linked'}
        <div class="row subtle">
          {linkTypeName(linkTypeId)} ({directionLabels[linkDirection] || linkDirection})
        </div>
      {:else if relation === 'descendants' || relation === 'direct_children'}
        <div class="row subtle">
          {crossWorkspace ? 'Crosses workspace boundaries' : 'Same workspace only'}
        </div>
      {/if}
    </div>
  {/snippet}
</GenericActionNode>

<style>
  .related-summary {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 12px;
  }
  .row {
    color: var(--ds-text);
  }
  .label {
    color: var(--ds-text-subtle);
    margin-right: 4px;
  }
  .subtle {
    color: var(--ds-text-subtle);
    font-size: 11px;
  }
</style>
