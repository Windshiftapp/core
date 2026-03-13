<script>
  import { onMount } from 'svelte';
  import { ChevronDown, X } from 'lucide-svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  let {
    placeholder = '',
    selectedField = $bindable(null),
    disabled = false,
    fieldGroups = null,
    customFieldItems = null,
    onSelect = () => {},
    onClear = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectField'));

  let isOpen = $state(false);
  let searchQuery = $state('');
  let customFields = $state([]);
  let dropdownElement = $state();

  // Helper to get field translation (handles both object and string formats)
  function getFieldTranslation(fieldKey) {
    const field = t(`pickers.fields.${fieldKey}`);
    if (typeof field === 'object' && field !== null) {
      return { name: field.name || fieldKey, description: field.description || '' };
    }
    return { name: field || fieldKey, description: '' };
  }

  // Standard fields grouped by category
  const standardFields = $derived(fieldGroups || [
    {
      category: t('pickers.fieldCategories.basic'),
      fields: [
        { id: 'title', name: getFieldTranslation('title').name, type: 'text', description: getFieldTranslation('title').description },
        { id: 'description', name: getFieldTranslation('description').name, type: 'text', description: getFieldTranslation('description').description },
        { id: 'status', name: getFieldTranslation('status').name, type: 'enum', description: getFieldTranslation('status').description },
        { id: 'priority', name: getFieldTranslation('priority').name, type: 'enum', description: getFieldTranslation('priority').description },
        { id: 'type', name: getFieldTranslation('type').name, type: 'enum', description: getFieldTranslation('type').description }
      ]
    },
    {
      category: t('pickers.fieldCategories.people'),
      fields: [
        { id: 'assignee', name: getFieldTranslation('assignee').name, type: 'user', description: getFieldTranslation('assignee').description },
        { id: 'reporter', name: getFieldTranslation('reporter').name, type: 'user', description: getFieldTranslation('reporter').description }
      ]
    },
    {
      category: t('pickers.fieldCategories.dates'),
      fields: [
        { id: 'createdAt', name: getFieldTranslation('createdAt').name, type: 'date', description: getFieldTranslation('createdAt').description },
        { id: 'updatedAt', name: getFieldTranslation('updatedAt').name, type: 'date', description: getFieldTranslation('updatedAt').description },
        { id: 'dueDate', name: getFieldTranslation('dueDate').name, type: 'date', description: getFieldTranslation('dueDate').description }
      ]
    },
    {
      category: t('pickers.fieldCategories.workflow'),
      fields: [
        { id: 'milestone', name: getFieldTranslation('milestone').name, type: 'enum', description: getFieldTranslation('milestone').description },
        { id: 'sprint', name: getFieldTranslation('sprint').name, type: 'enum', description: getFieldTranslation('sprint').description },
        { id: 'labels', name: getFieldTranslation('labels').name, type: 'enum', description: getFieldTranslation('labels').description }
      ]
    },
  ]);

  // Filtered fields derived from search query
  const filteredFields = $derived.by(() => {
    const query = searchQuery.toLowerCase();

    const filteredStandard = standardFields.map(group => ({
      category: group.category,
      fields: group.fields.filter(field =>
        (field.name || '').toLowerCase().includes(query) ||
        (field.description || '').toLowerCase().includes(query)
      )
    })).filter(group => group.fields.length > 0);

    const filteredCustom = customFields.filter(field =>
      (field.name || '').toLowerCase().includes(query) ||
      (field.description || '').toLowerCase().includes(query)
    );

    if (filteredCustom.length > 0) {
      return [...filteredStandard, {
        category: t('pickers.customFields'),
        fields: filteredCustom
      }];
    }

    return filteredStandard;
  });

  onMount(async () => {
    await loadCustomFields();
  });

  async function loadCustomFields() {
    if (customFieldItems) {
      customFields = customFieldItems;
      return;
    }
    try {
      const fields = await api.customFields.getAll();
      customFields = (fields || []).map(field => ({
        id: `cf_${field.name}`,
        name: field.name,
        type: field.field_type,
        description: field.description || t('pickers.customFieldDesc', { name: field.name }),
        isCustom: true,
        options: field.options ? JSON.parse(field.options) : null
      }));
    } catch (error) {
      console.error('Failed to load custom fields:', error);
      customFields = [];
    }
  }

  function selectField(field) {
    selectedField = field;
    isOpen = false;
    searchQuery = '';
    onSelect(field);
  }

  function clearSelection() {
    selectedField = null;
    onClear();
  }

  function toggleDropdown() {
    if (!disabled) {
      isOpen = !isOpen;
      if (isOpen) {
        setTimeout(() => {
          const searchInput = dropdownElement?.querySelector('input[type="text"]');
          searchInput?.focus();
        }, 10);
      }
    }
  }

  function handleClickOutside(event) {
    if (dropdownElement && !dropdownElement.contains(event.target)) {
      isOpen = false;
      searchQuery = '';
    }
  }

  function handleSearchInput(event) {
    searchQuery = event.target.value;
  }

  function getFieldTypeLabel(type) {
    const labels = {
      text: t('pickers.fieldTypes.text'),
      number: t('pickers.fieldTypes.number'),
      date: t('pickers.fieldTypes.date'),
      enum: t('pickers.fieldTypes.select'),
      boolean: t('pickers.fieldTypes.boolean'),
      user: t('pickers.fieldTypes.user'),
      reference: t('pickers.fieldTypes.reference'),
      select: t('pickers.fieldTypes.select'),
      textarea: t('pickers.fieldTypes.textArea'),
      identifier: t('pickers.fieldTypes.identifier')
    };
    return labels[type] || type;
  }

  function getFieldTypeColor(type) {
    const colors = {
      text: 'bg-blue-100 text-blue-800',
      number: 'bg-green-100 text-green-800',
      date: 'bg-purple-100 text-purple-800',
      enum: 'bg-orange-100 text-orange-800',
      boolean: 'bg-neutral-100 dark:bg-neutral-700 text-neutral-800 dark:text-neutral-200',
      user: 'bg-indigo-100 text-indigo-800',
      reference: 'bg-pink-100 text-pink-800',
      select: 'bg-orange-100 text-orange-800',
      textarea: 'bg-blue-100 text-blue-800'
    };
    return colors[type] || 'bg-neutral-100 dark:bg-neutral-700 text-neutral-800 dark:text-neutral-200';
  }
</script>

<svelte:window onclick={handleClickOutside} />

<div class="relative w-full" bind:this={dropdownElement}>
  <!-- Selected Field Display / Trigger Button -->
  <button
    type="button"
    onclick={toggleDropdown}
    disabled={disabled}
    class="w-full flex items-center justify-between px-3 py-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
    style="border-color: var(--ds-border); background-color: {disabled ? 'var(--ds-background-neutral)' : 'var(--ds-surface)'};"
  >
    {#if selectedField}
      <div class="flex items-center gap-2 flex-1 min-w-0">
        <span class="font-medium truncate" style="color: var(--ds-text);">{selectedField.name}</span>
        <span class="text-xs px-1.5 py-0.5 rounded {getFieldTypeColor(selectedField.type)}">
          {getFieldTypeLabel(selectedField.type)}
        </span>
        {#if selectedField.isCustom}
          <span class="text-xs px-1.5 py-0.5 rounded bg-purple-100 text-purple-800">{t('pickers.custom')}</span>
        {/if}
      </div>
      <div class="flex items-center gap-1">
        <button
          type="button"
          onclick={(e) => { e.stopPropagation(); clearSelection(); }}
          class="p-1 rounded transition-colors hover-bg"
          title={t('pickers.clearSelection')}
        >
          <X class="w-4 h-4" style="color: var(--ds-text-subtle);" />
        </button>
        <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
      </div>
    {:else}
      <span style="color: var(--ds-text-subtle);">{resolvedPlaceholder}</span>
      <ChevronDown class="w-4 h-4" style="color: var(--ds-text-subtle);" />
    {/if}
  </button>

  <!-- Dropdown Menu -->
  {#if isOpen}
    <div class="absolute z-50 mt-1 w-full max-w-md border rounded shadow-lg overflow-hidden" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
      <!-- Search Input -->
      <div class="p-2" style="border-bottom: 1px solid var(--ds-border);">
        <SearchInput
          bind:value={searchQuery}
          placeholder={t('pickers.searchFields')}
          size="small"
          on_input={handleSearchInput}
        />
      </div>

      <!-- Field List -->
      <div class="max-h-96 overflow-y-auto">
        {#if filteredFields.length === 0}
          <div class="p-4 text-center text-sm" style="color: var(--ds-text-subtle);">
            {t('pickers.noFieldsFound', { query: searchQuery })}
          </div>
        {:else}
          {#each filteredFields as group}
            <div class="last:border-b-0" style="border-bottom: 1px solid var(--ds-border);">
              <!-- Category Header -->
              <div class="px-3 py-2 text-xs font-semibold uppercase tracking-wide" style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);">
                {group.category}
              </div>

              <!-- Fields in Category -->
              {#each group.fields as field}
                <button
                  type="button"
                  onclick={() => selectField(field)}
                  class="w-full px-3 py-2 text-left transition-colors focus:outline-none field-item-btn"
                >
                  <div class="flex items-center justify-between">
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        <span class="font-medium" style="color: var(--ds-text);">{field.name}</span>
                        <span class="text-xs px-1.5 py-0.5 rounded {getFieldTypeColor(field.type)}">
                          {getFieldTypeLabel(field.type)}
                        </span>
                        {#if field.isCustom}
                          <span class="text-xs px-1.5 py-0.5 rounded bg-purple-100 text-purple-800">{t('pickers.custom')}</span>
                        {/if}
                      </div>
                      {#if field.description}
                        <p class="text-xs mt-0.5" style="color: var(--ds-text-subtle);">{field.description}</p>
                      {/if}
                    </div>
                  </div>
                </button>
              {/each}
            </div>
          {/each}
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .hover-bg:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
  .field-item-btn:hover {
    background-color: var(--ds-background-neutral-hovered);
  }
  .field-item-btn:focus {
    background-color: var(--ds-background-neutral);
  }
</style>
