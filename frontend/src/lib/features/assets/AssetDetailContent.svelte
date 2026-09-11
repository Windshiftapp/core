<script>
  import { IconFolder } from '@tabler/icons-svelte-runes';
  import ColorDot from '../../components/ColorDot.svelte';
  import { t } from '../../stores/i18n.svelte.js';
  import { formatDateSimple } from '../../utils/dateFormatter.js';
  import CustomFieldRenderer from '../items/CustomFieldRenderer.svelte';

  let {
    asset,
    fieldDefinitions = [],
    layout = 'stacked',
  } = $props();

  const fieldsClass = $derived(layout === 'grid' ? 'grid grid-cols-2 gap-4' : 'space-y-3');
  const descriptionClass = $derived(layout === 'grid' ? 'mb-6' : 'mb-4');
</script>

{#if asset.description}
  <div class={descriptionClass}>
    <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">{t('common.description')}</h4>
    <p class="text-sm" style="color: var(--ds-text);">{asset.description}</p>
  </div>
{/if}

<div class={fieldsClass}>
  {#if asset.asset_type_name}
    <div>
      <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">{t('common.type')}</h4>
      <span class="inline-flex items-center gap-1" style="color: var(--ds-text);">
        <ColorDot color={asset.asset_type_color || '#6b7280'} />
        {asset.asset_type_name}
      </span>
    </div>
  {/if}
  {#if asset.category_name}
    <div>
      <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">{t('common.category')}</h4>
      <span class="inline-flex items-center gap-1" style="color: var(--ds-text);">
        <IconFolder class="w-4 h-4 text-yellow-500" />
        {asset.category_name}
      </span>
    </div>
  {/if}
  {#if asset.status_name}
    <div>
      <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">{t('common.status')}</h4>
      <span class="inline-flex items-center gap-1.5" style="color: var(--ds-text);">
        <span class="w-2 h-2 rounded-full" style="background-color: {asset.status_color || '#6b7280'};"></span>
        {asset.status_name}
      </span>
    </div>
  {/if}
  {#if asset.asset_tag}
    <div>
      <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Asset Tag</h4>
      <span class="text-sm font-mono" style="color: var(--ds-text);">{asset.asset_tag}</span>
    </div>
  {/if}
  {#if asset.creator_name}
    <div>
      <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">{t('common.createdBy')}</h4>
      <span class="text-sm" style="color: var(--ds-text);">{asset.creator_name}</span>
    </div>
  {/if}
  <div>
    <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">{t('common.created')}</h4>
    <span class="text-sm" style="color: var(--ds-text);">{formatDateSimple(asset.created_at)}</span>
  </div>
  <div>
    <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">{t('common.updated')}</h4>
    <span class="text-sm" style="color: var(--ds-text);">{formatDateSimple(asset.updated_at)}</span>
  </div>
  {#if asset.linked_item_count > 0}
    <div>
      <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">Linked Items</h4>
      <span class="text-sm" style="color: var(--ds-text);">{asset.linked_item_count}</span>
    </div>
  {/if}
</div>

{#if asset.custom_field_values && Object.keys(asset.custom_field_values).length > 0}
  <div class="border-t pt-4 mt-4" style="border-color: var(--ds-border);">
    <h4 class="text-xs font-medium uppercase mb-3" style="color: var(--ds-text-subtlest);">Custom Fields</h4>
    {#each Object.entries(asset.custom_field_values) as [fieldId, value]}
      {@const field = fieldDefinitions.find((definition) => String(definition.custom_field_id) === String(fieldId))}
      {#if field && value !== null && value !== ''}
        <div class="mb-3">
          <h4 class="text-xs font-medium uppercase mb-1" style="color: var(--ds-text-subtlest);">{field.field_name}</h4>
          <CustomFieldRenderer
            field={{
              id: field.custom_field_id,
              name: field.field_name,
              field_type: field.field_type,
              options: field.options,
            }}
            {value}
            readonly
            noPadding
          />
        </div>
      {/if}
    {/each}
  </div>
{/if}
