<script>
  import { Database } from '@lucide/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={Database} title={t('actions.nodes.updateAsset')} accentColor="teal">
  {#snippet body()}
    {#if data.config?.source_field_id}
      <div class="field-info">
        <span class="field-name">{data.config.source_field_id}</span>
        <span class="field-arrow">&rarr;</span>
        <span class="field-value">{t('actions.config.fieldMappings', { count: data.config.field_mappings?.length || 0 })}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.configureAssetUpdate')}</div>
    {/if}
  {/snippet}
</GenericActionNode>

<style>
  .field-info {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
  }

  .field-name {
    color: var(--ds-text);
    font-weight: 500;
  }

  .field-arrow {
    color: var(--ds-text-subtlest);
  }

  .field-value {
    color: var(--ds-text-subtle);
    font-size: 11px;
    background-color: var(--ds-surface-sunken);
    padding: 2px 6px;
    border-radius: 4px;
  }
</style>
