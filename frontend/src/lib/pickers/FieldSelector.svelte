<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { ChevronDown, X } from 'lucide-svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import { api } from '../api.js';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  export let placeholder = '';
  export let selectedField = null;
  export let disabled = false;

  $: resolvedPlaceholder = placeholder || t('pickers.selectField');

  let isOpen = false;
  let searchQuery = '';
  let filteredFields = [];
  let customFields = [];
  let dropdownElement;

  // Standard fields grouped by category - using reactive to support i18n
  $: standardFields = [
    {
      category: t('pickers.fieldCategories.basic'),
      fields: [
        { id: 'title', name: t('pickers.fields.title'), type: 'text', description: t('pickers.fields.titleDesc') },
        { id: 'description', name: t('pickers.fields.description'), type: 'text', description: t('pickers.fields.descriptionDesc') },
        { id: 'key', name: t('pickers.fields.key'), type: 'identifier', description: t('pickers.fields.keyDesc') },
        { id: 'id', name: t('pickers.fields.id'), type: 'identifier', description: t('pickers.fields.idDesc') }
      ]
    },
    {
      category: t('pickers.fieldCategories.assignments'),
      fields: [
        { id: 'assignee', name: t('pickers.fields.assignee'), type: 'user', description: t('pickers.fields.assigneeDesc') },
        { id: 'creator', name: t('pickers.fields.creator'), type: 'user', description: t('pickers.fields.creatorDesc') }
      ]
    },
    {
      category: t('pickers.fieldCategories.projectsMilestones'),
      fields: [
        { id: 'milestone', name: t('pickers.fields.milestone'), type: 'enum', description: t('pickers.fields.milestoneDesc') },
        { id: 'project', name: t('pickers.fields.project'), type: 'enum', description: t('pickers.fields.projectDesc') },
        { id: 'itemType', name: t('pickers.fields.itemType'), type: 'enum', description: t('pickers.fields.itemTypeDesc') }
      ]
    },
    {
      category: t('pickers.fieldCategories.dates'),
      fields: [
        { id: 'created', name: t('pickers.fields.createdDate'), type: 'date', description: t('pickers.fields.createdDateDesc') },
        { id: 'updated', name: t('pickers.fields.updatedDate'), type: 'date', description: t('pickers.fields.updatedDateDesc') }
      ]
    },
    {
      category: t('pickers.fieldCategories.hierarchy'),
      fields: [
        { id: 'parent', name: t('pickers.fields.parentId'), type: 'reference', description: t('pickers.fields.parentIdDesc') },
        { id: 'isTask', name: t('pickers.fields.isTask'), type: 'boolean', description: t('pickers.fields.isTaskDesc') }
      ]
    },
  ];

  onMount(async () => {
    await loadCustomFields();
    updateFilteredFields();
  });

  async function loadCustomFields() {
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
      updateFilteredFields();
    } catch (error) {
      console.error('Failed to load custom fields:', error);
      customFields = [];
    }
  }

  function updateFilteredFields() {
    const query = searchQuery.toLowerCase();

    // Filter standard fields
    const filteredStandard = standardFields.map(group => ({
      category: group.category,
      fields: group.fields.filter(field =>
        field.name.toLowerCase().includes(query) ||
        field.description.toLowerCase().includes(query)
      )
    })).filter(group => group.fields.length > 0);

    // Filter custom fields
    const filteredCustom = customFields.filter(field =>
      field.name.toLowerCase().includes(query) ||
      field.description.toLowerCase().includes(query)
    );

    filteredFields = filteredStandard;

    // Add custom fields as a separate category if any match
    if (filteredCustom.length > 0) {
      filteredFields.push({
        category: t('pickers.customFields'),
        fields: filteredCustom
      });
    }
  }

  function selectField(field) {
    selectedField = field;
    isOpen = false;
    searchQuery = '';
    updateFilteredFields();
    dispatch('select', field);
  }

  function clearSelection() {
    selectedField = null;
    dispatch('clear');
  }

  function toggleDropdown() {
    if (!disabled) {
      isOpen = !isOpen;
      if (isOpen) {
        // Focus search input when dropdown opens
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
      updateFilteredFields();
    }
  }

  function handleSearchInput(event) {
    searchQuery = event.target.value;
    updateFilteredFields();
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
      boolean: 'bg-gray-100 text-gray-800',
      user: 'bg-indigo-100 text-indigo-800',
      reference: 'bg-pink-100 text-pink-800',
      select: 'bg-orange-100 text-orange-800',
      textarea: 'bg-blue-100 text-blue-800'
    };
    return colors[type] || 'bg-gray-100 text-gray-800';
  }

  $: if (searchQuery !== undefined) {
    updateFilteredFields();
  }
</script>

<svelte:window onclick={handleClickOutside} />

<div class="relative w-full" bind:this={dropdownElement}>
  <!-- Selected Field Display / Trigger Button -->
  <button
    type="button"
    onclick={toggleDropdown}
    disabled={disabled}
    class="w-full flex items-center justify-between px-3 py-2 border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
    class:bg-gray-50={disabled}
    class:hover:bg-gray-50={!disabled}
  >
    {#if selectedField}
      <div class="flex items-center gap-2 flex-1 min-w-0">
        <span class="font-medium text-gray-900 truncate">{selectedField.name}</span>
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
          class="p-1 hover:bg-gray-200 rounded transition-colors"
          title={t('pickers.clearSelection')}
        >
          <X class="w-4 h-4 text-gray-500" />
        </button>
        <ChevronDown class="w-4 h-4 text-gray-500" />
      </div>
    {:else}
      <span class="text-gray-500">{resolvedPlaceholder}</span>
      <ChevronDown class="w-4 h-4 text-gray-500" />
    {/if}
  </button>

  <!-- Dropdown Menu -->
  {#if isOpen}
    <div class="absolute z-50 mt-1 w-full max-w-md bg-white border border-gray-300 rounded shadow-lg overflow-hidden">
      <!-- Search Input -->
      <div class="p-2 border-b border-gray-200">
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
          <div class="p-4 text-center text-gray-500 text-sm">
            {t('pickers.noFieldsFound', { query: searchQuery })}
          </div>
        {:else}
          {#each filteredFields as group}
            <div class="border-b border-gray-100 last:border-b-0">
              <!-- Category Header -->
              <div class="px-3 py-2 bg-gray-50 text-xs font-semibold text-gray-700 uppercase tracking-wide">
                {group.category}
              </div>

              <!-- Fields in Category -->
              {#each group.fields as field}
                <button
                  type="button"
                  onclick={() => selectField(field)}
                  class="w-full px-3 py-2 text-left hover:bg-gray-50 transition-colors focus:outline-none focus:bg-gray-100"
                >
                  <div class="flex items-center justify-between">
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        <span class="font-medium text-gray-900">{field.name}</span>
                        <span class="text-xs px-1.5 py-0.5 rounded {getFieldTypeColor(field.type)}">
                          {getFieldTypeLabel(field.type)}
                        </span>
                        {#if field.isCustom}
                          <span class="text-xs px-1.5 py-0.5 rounded bg-purple-100 text-purple-800">{t('pickers.custom')}</span>
                        {/if}
                      </div>
                      {#if field.description}
                        <p class="text-xs text-gray-500 mt-0.5">{field.description}</p>
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
