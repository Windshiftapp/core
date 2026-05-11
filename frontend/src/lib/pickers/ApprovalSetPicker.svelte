<script>
  import { BasePicker } from '.';
  import { Stamp } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    items = [],
    placeholder = '',
    workflowId = null,
    disabled = false,
    class: className = '',
    dataTestid = undefined,
    onSelect = () => {},
    onCancel = () => {}
  } = $props();
  void dataTestid;

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectApprovalSet'));

  // Approval sets are workflow-scoped — only offer ones matching the current workflow.
  const filteredItems = $derived(
    workflowId ? items.filter(s => s.workflow_id === workflowId) : items
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
  getValue={(s) => s?.id}
  getLabel={(s) => s?.name ?? ''}
  {onSelect}
  {onCancel}
>
  {#snippet itemSnippet({ item: s, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <div class="flex-shrink-0">
        <div class="w-7 h-7 rounded flex items-center justify-center" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
          <Stamp class="w-4 h-4" />
        </div>
      </div>
      <div class="flex flex-col min-w-0">
        <span class="font-medium truncate">{s.name}</span>
        {#if s.description}
          <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{s.description}</span>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>
