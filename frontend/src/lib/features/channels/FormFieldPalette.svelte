<script>
  import { IconSearch, IconPlus, IconTextSize, IconForms, IconCheckbox, IconSelect, IconAlignBoxLeftTop } from '@tabler/icons-svelte-runes';
  import { t } from '../../stores/i18n.svelte.js';
  import { formBuilderStore } from '../../stores/formBuilderStore.svelte.js';
  import Input from '../../components/Input.svelte';

  function getFieldIcon(field) {
    if (field.type === 'virtual') {
      if (field.fieldType === 'textarea') return IconAlignBoxLeftTop;
      if (field.fieldType === 'select') return IconSelect;
      if (field.fieldType === 'checkbox') return IconCheckbox;
      return IconTextSize;
    }
    if (field.type === 'default') return IconForms;
    return IconTextSize;
  }

  // Group fields by category
  let groupedFields = $derived(() => {
    const groups = {};
    for (const field of formBuilderStore.searchFilteredFields) {
      const cat = field.category || 'Other';
      if (!groups[cat]) groups[cat] = [];
      groups[cat].push(field);
    }
    return groups;
  });
</script>

<div class="w-72 border-l overflow-y-auto" style="border-color: var(--ds-border); background-color: var(--ds-surface-raised);">
  <div class="p-4">
    <h4 class="text-sm font-semibold mb-3" style="color: var(--ds-text);">{t('forms.builder.availableFields')}</h4>

    <!-- Search -->
    <div class="relative mb-3">
      <IconSearch class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4" style="color: var(--ds-text-disabled);" />
      <input
        type="text"
        bind:value={formBuilderStore.fieldSearchQuery}
        placeholder={t('forms.builder.searchFields')}
        class="w-full pl-9 pr-3 py-2 text-sm rounded-lg border"
        style="border-color: var(--ds-border); background-color: var(--ds-surface); color: var(--ds-text);"
      />
    </div>

    <!-- Field Groups -->
    {#each Object.entries(groupedFields()) as [category, fields]}
      <div class="mb-4">
        <h5 class="text-xs font-semibold uppercase tracking-wider mb-2" style="color: var(--ds-text-subtle);">
          {category}
        </h5>
        <div class="space-y-1">
          {#each fields as field}
            {@const FieldIcon = getFieldIcon(field)}
            {@const isAdded = formBuilderStore.formFields.some(
              ff => ff.field_type === field.type && ff.field_identifier === field.identifier
            )}
            <div
              data-available-field={JSON.stringify(field)}
              class="flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-colors {isAdded ? 'opacity-40 cursor-not-allowed' : 'cursor-grab hover:bg-[var(--ds-background-neutral-hovered)]'}"
              style="color: var(--ds-text);"
            >
              <FieldIcon class="w-4 h-4 flex-shrink-0" style="color: var(--ds-text-subtle);" />
              <span class="flex-1 truncate">{field.name}</span>
              {#if !isAdded}
                <button
                  onclick={() => formBuilderStore.addField(field)}
                  class="p-0.5 rounded hover:bg-[var(--ds-background-neutral-hovered)]"
                  title={t('forms.builder.addField')}
                >
                  <IconPlus class="w-3.5 h-3.5" style="color: var(--ds-text-subtle);" />
                </button>
              {/if}
            </div>
          {/each}
        </div>
      </div>
    {/each}

    {#if formBuilderStore.searchFilteredFields.length === 0}
      <p class="text-xs text-center py-4" style="color: var(--ds-text-subtle);">
        {t('forms.builder.noFieldsAvailable')}
      </p>
    {/if}
  </div>
</div>
