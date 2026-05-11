<script>
  import { BasePicker } from '.';
  import { ShieldCheck } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    items = [],
    placeholder = '',
    workflowId = null,
    disabled = false,
    class: className = '',
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectConditionSet'));

  // Filter items to only show condition sets matching the current workflow
  const filteredItems = $derived(
    workflowId ? items.filter(cs => cs.workflow_id === workflowId) : items
  );
</script>

<BasePicker
  bind:value
  items={filteredItems}
  placeholder={resolvedPlaceholder}
  {disabled}
  class={className}
  showUnassigned={true}
  unassignedLabel={t('common.none')}
  searchFields={['name', 'description']}
  getValue={(cs) => cs?.id}
  getLabel={(cs) => cs?.name ?? ''}
  {onSelect}
  {onCancel}
>
  {#snippet itemSnippet({ item: cs, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <div class="flex-shrink-0">
        <div class="w-7 h-7 rounded flex items-center justify-center" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
          <ShieldCheck class="w-4 h-4" />
        </div>
      </div>
      <div class="flex flex-col min-w-0">
        <span class="font-medium truncate">{cs.name}</span>
        {#if cs.description}
          <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{cs.description}</span>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>
