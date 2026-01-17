<script>
  import { BasePicker } from '.';
  import { Layout } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    items = [],
    placeholder = '',
    defaultScreenId = null,
    unassignedLabel: customUnassignedLabel = null,
    disabled = false,
    class: className = '',
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectScreen'));

  // Get the default screen name for the "Default" option label
  const defaultScreenName = $derived(() => {
    if (!defaultScreenId) return '';
    const screen = items.find(s => s.id === defaultScreenId);
    return screen ? screen.name : '';
  });

  // Dynamic label for the unassigned/default option (custom label takes precedence)
  const unassignedLabel = $derived(
    customUnassignedLabel
      ? customUnassignedLabel
      : defaultScreenId && defaultScreenName()
        ? `${t('common.default')} (${defaultScreenName()})`
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
  getValue={(screen) => screen?.id}
  getLabel={(screen) => screen?.name ?? ''}
  {onSelect}
  {onCancel}
>
  {#snippet itemSnippet({ item: screen, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <!-- Screen Icon -->
      <div class="flex-shrink-0">
        <div class="w-7 h-7 rounded flex items-center justify-center" style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
          <Layout class="w-4 h-4" />
        </div>
      </div>

      <!-- Screen Info -->
      <div class="flex flex-col min-w-0">
        <span class="font-medium truncate">{screen.name}</span>
        {#if screen.description}
          <span class="text-xs truncate" style="color: var(--ds-text-subtle);">{screen.description}</span>
        {/if}
      </div>
    </div>
  {/snippet}
</BasePicker>
