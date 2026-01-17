<script>
  import { X, ChevronDown } from 'lucide-svelte';
  import { onClickOutside } from 'runed';
  import Label from '../components/Label.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    categories = [],
    selectedIds = $bindable([]),
    placeholder = '',
    label = '',
    helperText = '',
    disabled = false,
    on_change = () => {},
    class: className = ''
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.selectCategories'));

  let showDropdown = $state(false);
  let searchInput = $state('');
  let dropdownElement = $state();

  // Close dropdown when clicking outside
  onClickOutside(
    () => dropdownElement,
    () => { showDropdown = false; }
  );

  function getHexFromColorName(colorName) {
    const colorMap = {
      'red': '#ef4444',
      'orange': '#f97316', 
      'yellow': '#eab308',
      'green': '#22c55e',
      'blue': '#3b82f6',
      'indigo': '#6366f1',
      'purple': '#a855f7',
      'pink': '#ec4899',
      'gray': '#6b7280',
      'slate': '#64748b'
    };
    return colorMap[colorName] || '#3b82f6';
  }

  function addCategory(categoryId) {
    if (!selectedIds.includes(categoryId)) {
      selectedIds = [...selectedIds, categoryId];
      on_change({ selectedIds, added: categoryId });
    }
  }

  function removeCategory(categoryId) {
    selectedIds = selectedIds.filter(id => id !== categoryId);
    on_change({ selectedIds, removed: categoryId });
  }

  function toggleDropdown() {
    if (!disabled) {
      showDropdown = !showDropdown;
      searchInput = '';
    }
  }

  const filteredCategories = $derived.by(() => {
    return categories.filter(category => {
      if (!searchInput) return true;
      return category.name.toLowerCase().includes(searchInput.toLowerCase());
    });
  });

  const selectedCount = $derived.by(() => selectedIds.length);
</script>

{#if label}
  <Label color="default" class="mb-2">{label}</Label>
{/if}

<div class="relative {className}" bind:this={dropdownElement}>
  <!-- Selected Tags Display -->
  {#if selectedCount > 0}
    <div class="flex flex-wrap gap-2 mb-3">
      {#each selectedIds as categoryId}
        {@const category = categories.find(c => c.id === categoryId)}
        {#if category}
          {@const hexColor = category.color?.startsWith('#') ? category.color : getHexFromColorName(category.color || 'blue')}
          <div class="flex items-center gap-1 px-3 py-1 rounded-full text-sm"
            style="background-color: {hexColor}20; color: {hexColor}; border: 1px solid {hexColor}40;">
            <span>{category.name}</span>
            <button
              onclick={() => removeCategory(categoryId)}
              class="ml-1 hover:bg-gray-200 rounded-full p-0.5 transition-colors"
              title={t('pickers.removeCategory')}
            >
              <X class="w-3 h-3" />
            </button>
          </div>
        {/if}
      {/each}
    </div>
  {/if}

  <!-- Dropdown Trigger -->
  <button
    type="button"
    onclick={toggleDropdown}
    {disabled}
    class="w-full flex items-center justify-between px-4 py-3 rounded border transition-all duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50 text-sm disabled:opacity-50 disabled:cursor-not-allowed"
    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
  >
    <span>
      {#if selectedCount === 0}
        {resolvedPlaceholder}
      {:else}
        {t('pickers.categoriesSelected', { count: selectedCount })}
      {/if}
    </span>
    <ChevronDown class="w-4 h-4 transition-transform {showDropdown ? 'rotate-180' : ''}" />
  </button>

  <!-- Dropdown Menu -->
  {#if showDropdown}
    <div class="absolute z-10 w-full mt-1 border rounded shadow-lg max-h-60 overflow-hidden"
      style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
      
      <!-- Search Input -->
      <div class="p-3 border-b" style="border-color: var(--ds-border);">
        <input
          type="text"
          bind:value={searchInput}
          placeholder={t('pickers.searchCategories')}
          class="w-full px-3 py-2 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-500"
          style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
        />
      </div>

      <!-- Categories List -->
      <div class="max-h-48 overflow-y-auto">
        {#each filteredCategories as category}
          {@const isSelected = selectedIds.includes(category.id)}
          {@const hexColor = category.color?.startsWith('#') ? category.color : getHexFromColorName(category.color || 'blue')}
          
          <button
            type="button"
            onclick={() => addCategory(category.id)}
            class="w-full px-3 py-2 text-left text-sm hover:bg-gray-50 flex items-center gap-3 transition-colors"
            style="hover:background-color: var(--ds-background-hover);"
          >
            <div class="w-3 h-3 rounded-full flex-shrink-0"
              style="background-color: {hexColor};"></div>
            <span class="flex-1" style="color: var(--ds-text);">{category.name}</span>
            {#if isSelected}
              <div class="w-4 h-4 rounded bg-blue-500 flex items-center justify-center">
                <svg class="w-3 h-3 text-white" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd"/>
                </svg>
              </div>
            {/if}
          </button>
        {/each}
        
        {#if filteredCategories.length === 0}
          <div class="px-3 py-4 text-center text-sm" style="color: var(--ds-text-subtle);">
            {t('pickers.noCategoriesFound')}
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>

{#if helperText}
  <p class="mt-2 text-sm" style="color: var(--ds-text-subtle);">
    {helperText}
  </p>
{/if}