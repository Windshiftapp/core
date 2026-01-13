<script>
  import { createEventDispatcher, onMount } from 'svelte';
  import { ChevronDown, X } from 'lucide-svelte';
  import SearchInput from '../components/SearchInput.svelte';
  import { api } from '../api.js';

  const dispatch = createEventDispatcher();

  export let placeholder = 'Select field...';
  export let selectedField = null;
  export let disabled = false;

  let isOpen = false;
  let searchQuery = '';
  let filteredFields = [];
  let customFields = [];
  let dropdownElement;

  // Standard fields grouped by category
  const standardFields = [
    {
      category: 'Basic',
      fields: [
        { id: 'title', name: 'Title', type: 'text', description: 'Item title' },
        { id: 'description', name: 'Description', type: 'text', description: 'Item description' },
        { id: 'key', name: 'Key', type: 'identifier', description: 'Item key (e.g., "WK-123")' },
        { id: 'id', name: 'ID', type: 'identifier', description: 'Item ID' }
      ]
    },
    {
      category: 'Assignments',
      fields: [
        { id: 'assignee', name: 'Assignee', type: 'user', description: 'Assigned user ID' },
        { id: 'creator', name: 'Creator', type: 'user', description: 'Creator user ID' }
      ]
    },
    {
      category: 'Projects & Milestones',
      fields: [
        { id: 'milestone', name: 'Milestone', type: 'enum', description: 'Associated milestone' },
        { id: 'project', name: 'Project', type: 'enum', description: 'Associated project' },
        { id: 'itemType', name: 'Item Type', type: 'enum', description: 'Work item type' }
      ]
    },
    {
      category: 'Dates',
      fields: [
        { id: 'created', name: 'Created Date', type: 'date', description: 'Creation date' },
        { id: 'updated', name: 'Updated Date', type: 'date', description: 'Last update date' }
      ]
    },
    {
      category: 'Hierarchy',
      fields: [
        { id: 'parent', name: 'Parent ID', type: 'reference', description: 'Parent item ID' },
        { id: 'isTask', name: 'Is Task', type: 'boolean', description: 'Is a task item' }
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
        description: field.description || `Custom field: ${field.name}`,
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
        category: 'Custom Fields',
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
      text: 'Text',
      number: 'Number',
      date: 'Date',
      enum: 'Select',
      boolean: 'Boolean',
      user: 'User',
      reference: 'Reference',
      select: 'Select',
      textarea: 'Text Area'
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
          <span class="text-xs px-1.5 py-0.5 rounded bg-purple-100 text-purple-800">Custom</span>
        {/if}
      </div>
      <div class="flex items-center gap-1">
        <button
          type="button"
          onclick={(e) => { e.stopPropagation(); clearSelection(); }}
          class="p-1 hover:bg-gray-200 rounded transition-colors"
          title="Clear selection"
        >
          <X class="w-4 h-4 text-gray-500" />
        </button>
        <ChevronDown class="w-4 h-4 text-gray-500" />
      </div>
    {:else}
      <span class="text-gray-500">{placeholder}</span>
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
          placeholder="Search fields..."
          size="small"
          on_input={handleSearchInput}
        />
      </div>

      <!-- Field List -->
      <div class="max-h-96 overflow-y-auto">
        {#if filteredFields.length === 0}
          <div class="p-4 text-center text-gray-500 text-sm">
            No fields found matching "{searchQuery}"
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
                          <span class="text-xs px-1.5 py-0.5 rounded bg-purple-100 text-purple-800">Custom</span>
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
