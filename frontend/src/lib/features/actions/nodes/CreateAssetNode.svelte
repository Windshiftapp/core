<script>
  import { PlusSquare } from '@lucide/svelte';
  import { t } from '../../../stores/i18n.svelte.js';
  import { actionFlowStore } from '../../../stores/actionFlowStore.svelte.js';
  import GenericActionNode from '../shared/GenericActionNode.svelte';

  let { data = {}, selected = false } = $props();
</script>

<GenericActionNode {data} {selected} flowStore={actionFlowStore} icon={PlusSquare} title={t('actions.nodes.createAsset')} accentColor="green">
  {#snippet body()}
    {#if data.config?.asset_type_id && data.config?.title}
      <div class="field-info">
        <span class="field-name">{data.config.title}</span>
      </div>
    {:else}
      <div class="placeholder">{t('actions.config.configureAssetCreation')}</div>
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
    max-width: 160px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
