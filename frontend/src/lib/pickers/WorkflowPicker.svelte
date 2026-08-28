<script>
  import { BasePicker } from '.';
  import { GitBranch } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    items = [],
    placeholder = '',
    defaultWorkflowId = null,
    unassignedLabel: customUnassignedLabel = null,
    disabled = false,
    class: className = '',
    dataTestid = undefined,
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectWorkflow'));

  function isDefaultSystemWorkflow(workflow) {
    return workflow?.is_default === true && workflow.name === 'Default Workflow' &&
      workflow.description === 'Basic workflow for getting work done';
  }

  function getWorkflowDisplayValue(workflow, field) {
    return isDefaultSystemWorkflow(workflow)
      ? t(`workflows.defaults.default.${field}`)
      : (workflow?.[field] || '');
  }

  // Get the default workflow name for the "Default" option label
  const defaultWorkflowName = $derived(() => {
    if (!defaultWorkflowId) return '';
    const workflow = items.find(w => w.id === defaultWorkflowId);
    return workflow ? getWorkflowDisplayValue(workflow, 'name') : '';
  });

  // Dynamic label for the unassigned/default option (custom label takes precedence)
  const unassignedLabel = $derived(
    customUnassignedLabel
      ? customUnassignedLabel
      : defaultWorkflowId && defaultWorkflowName()
        ? `${t('common.default')} (${defaultWorkflowName()})`
        : t('common.default')
  );
</script>

<BasePicker
  bind:value
  {items}
  placeholder={resolvedPlaceholder}
  {disabled}
  class={className}
  showUnassigned={true}
  {unassignedLabel}
  searchFields={['name', 'description']}
  getValue={(workflow) => workflow?.id}
  getLabel={(workflow) => getWorkflowDisplayValue(workflow, 'name')}
  {onSelect}
  {onCancel}
>
  {#snippet itemSnippet({ item: workflow, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <!-- Workflow Icon -->
      <div class="flex-shrink-0">
        <div class="w-7 h-7 rounded flex items-center justify-center" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
          <GitBranch class="w-4 h-4" />
        </div>
      </div>

      <!-- Workflow Info -->
      <div class="flex flex-col min-w-0">
        <span class="font-medium truncate">{getWorkflowDisplayValue(workflow, 'name')}</span>
        {#if getWorkflowDisplayValue(workflow, 'description')}
          <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{getWorkflowDisplayValue(workflow, 'description')}</span>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>
