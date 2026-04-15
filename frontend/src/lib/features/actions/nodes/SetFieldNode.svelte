<script>
  import { Pencil } from 'lucide-svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={Pencil} title={t('actions.nodes.setField')} accentColor="purple">
  {#snippet body()}
    {#if data.config?.field_name}
      <div class="field-info">
        <span class="field-name">{data.config.field_display_name || data.config.field_name}</span>
        <span class="field-arrow">&rarr;</span>
        <span class="field-value">{data.config.value || '...'}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.selectField')}</div>
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
    font-family: monospace;
    font-size: 11px;
    background-color: var(--ds-surface-sunken);
    padding: 2px 6px;
    border-radius: 4px;
    max-width: 100px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
