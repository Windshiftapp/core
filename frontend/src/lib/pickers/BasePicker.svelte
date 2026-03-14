<script>
  import { createCombobox, melt } from '@melt-ui/svelte';
  import { fly } from 'svelte/transition';
  import { Check, ChevronDown, X } from 'lucide-svelte';
  import Spinner from '../components/Spinner.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    // Core props
    value = $bindable(null),
    items = [],
    loading = false,
    error = null,
    disabled = false,

    // Display props
    id = undefined,
    placeholder = '',
    label = '',
    class: className = '',

    // Feature toggles
    allowClear = false,
    showUnassigned = false,
    unassignedLabel = '',
    multiple = false,
    showSelectedInTrigger = true,

    // Item configuration
    searchFields = ['name'],
    getValue = (item) => item?.id,
    getLabel = (item) => item?.name ?? '',

    // Snippets for customization
    itemSnippet = null,
    triggerSnippet = null,
    iconSnippet = null,
    chipSnippet = null,
    noResultsSnippet = null,
    createOptionSnippet = null,

    // Create functionality
    allowCreate = false,
    onCreate = null,

    // Event callbacks (Svelte 5 pattern)
    onSelect = () => {},
    onCancel = () => {},
    onChange = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.select'));
  const resolvedUnassignedLabel = $derived(unassignedLabel || t('pickers.unassigned'));

  // Expose input value for create functionality
  export function getInputValue() {
    return $inputValue;
  }

  // Create Melt combobox
  const {
    elements: { menu, input, option, label: labelEl },
    states: { open, inputValue, touchedInput, selected },
    helpers: { isSelected }
  } = createCombobox({
    forceVisible: true,
    preventScroll: false,
    multiple: false, // We handle multi-select manually for chip display
    positioning: {
      strategy: 'fixed',
      placement: 'bottom-start',
      sameWidth: false
    }
  });

  // Filter items based on search input
  const filteredItems = $derived.by(() => {
    if (!$touchedInput || !$inputValue) {
      return items;
    }

    const search = $inputValue.toLowerCase();
    return items.filter(item =>
      searchFields.some(field => {
        const fieldValue = typeof field === 'function' ? field(item) : item[field];
        return fieldValue?.toString().toLowerCase().includes(search);
      })
    );
  });

  // Create options for Melt combobox
  const options = $derived.by(() => {
    const opts = filteredItems.map(item => ({
      value: getValue(item),
      label: getLabel(item),
      item: item
    }));

    // Add unassigned option at the beginning if enabled (single-select only)
    if (showUnassigned && !multiple) {
      opts.unshift({
        value: null,
        label: resolvedUnassignedLabel,
        item: null,
        isUnassigned: true
      });
    }

    return opts;
  });

  // For multi-select: get array of selected items
  const selectedItems = $derived.by(() => {
    if (!multiple) return [];
    const valueArray = Array.isArray(value) ? value : [];
    return valueArray
      .map(v => items.find(item => getValue(item) === v))
      .filter(Boolean);
  });

  // For single-select: get selected item
  const selectedItem = $derived(
    !multiple ? (items.find(item => getValue(item) === value) || null) : null
  );

  // Track highlighted index for keyboard navigation
  let highlightedIndex = $state(0);

  // Check if an item is selected (for multi-select)
  function isItemSelected(itemValue) {
    if (!multiple) return value === itemValue;
    return Array.isArray(value) && value.includes(itemValue);
  }

  // Set display value when value changes externally (single-select only)
  $effect(() => {
    if (!multiple && !$touchedInput) {
      if (value != null && showSelectedInTrigger) {
        const item = items.find(i => getValue(i) === value);
        if (item) {
          $inputValue = getLabel(item);
        }
      } else {
        $inputValue = '';
      }
    }
  });


  // Handle keyboard navigation
  function handleKeydown(event) {
    if (event.key === 'Escape') {
      event.preventDefault();
      onCancel();
      return;
    }

    // Tab closes the dropdown without preventing default (allows focus to move)
    if (event.key === 'Tab') {
      $open = false;
      return;
    }

    // Only handle arrow keys, Enter, and Space when dropdown is open
    if (!$open) return;

    const totalItems = options.length;
    if (totalItems === 0) return;

    if (event.key === 'ArrowDown') {
      event.preventDefault();
      event.stopPropagation();
      highlightedIndex = (highlightedIndex + 1) % totalItems;
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      event.stopPropagation();
      highlightedIndex = highlightedIndex === 0 ? totalItems - 1 : highlightedIndex - 1;
    } else if (event.key === 'Enter' || (event.key === ' ' && event.target.tagName !== 'INPUT')) {
      event.preventDefault();
      event.stopPropagation();
      if (highlightedIndex >= 0 && highlightedIndex < totalItems) {
        const opt = options[highlightedIndex];
        // Same selection logic as onclick
        if (multiple) {
          const itemValue = opt.value;
          if (isItemSelected(itemValue)) {
            value = (value || []).filter(v => v !== itemValue);
          } else {
            value = [...(value || []), itemValue];
          }
          $inputValue = '';
          onChange(value);
        } else {
          value = opt.value;
          $inputValue = opt.isUnassigned ? '' : opt.label;
          onSelect(opt.item);
        }
        $open = false;
      }
    }
  }

  // Handle dropdown close without selection (single-select only)
  let wasOpen = $state(false);
  $effect(() => {
    if (wasOpen && !$open && !$selected && !multiple) {
      onCancel();
    }
    // Reset highlighted index when dropdown opens
    if (!wasOpen && $open) {
      highlightedIndex = 0;
    }
    wasOpen = $open;
  });

  // Reset highlighted index when filtered options change (e.g., when typing)
  $effect(() => {
    // Access options.length to track changes
    const len = options.length;
    // Ensure highlighted index is within bounds
    if (highlightedIndex >= len) {
      highlightedIndex = Math.max(0, len - 1);
    }
  });

  // Clear selection
  function handleClear(e) {
    e.stopPropagation();
    if (multiple) {
      value = [];
      onChange([]);
    } else {
      value = null;
      $inputValue = '';
      $selected = null;
      onSelect(null);
    }
  }

  // Remove a single item (multi-select)
  function removeItem(e, itemValue) {
    e.stopPropagation();
    value = (value || []).filter(v => v !== itemValue);
    onChange(value);
  }

  // Focus the input and open dropdown when clicking the container
  let inputRef = $state(null);
  function focusInput() {
    inputRef?.focus();
    $open = true;
  }

  // Reference to dropdown menu for scrolling
  let menuRef = $state(null);

  // Scroll highlighted item into view
  $effect(() => {
    if ($open && menuRef && options.length > 0) {
      const highlightedEl = menuRef.children[highlightedIndex];
      if (highlightedEl) {
        highlightedEl.scrollIntoView({ block: 'nearest' });
      }
    }
  });
</script>

<div class="relative {className}">
  {#if label}
    <label
      use:melt={$labelEl}
      class="block text-sm font-medium mb-1"
      style="color: var(--ds-text);"
    >
      {label}
    </label>
  {/if}

  <div class="relative">
    {#if multiple}
      <!-- Multi-select: Container with chips + input -->
      <div
        class="w-full min-h-[38px] px-2.5 py-1.5 pr-10 rounded border transition-all duration-200
               focus-within:outline-none focus-within:ring-2 focus-within:ring-blue-500 focus-within:ring-opacity-50
               disabled:opacity-50 disabled:cursor-not-allowed flex flex-wrap items-center gap-1.5"
        style="background-color: var(--ds-background-input);
               border-color: var(--ds-border);"
        onclick={focusInput}
        onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && focusInput()}
        role="button"
        tabindex="-1"
      >
        <!-- Selected items as chips -->
        {#each selectedItems as item (getValue(item))}
          <div
            class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-xs border"
            style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); color: var(--ds-text);"
          >
            {#if chipSnippet}
              {@render chipSnippet({ item })}
            {:else}
              <span class="font-medium truncate max-w-[150px]">{getLabel(item)}</span>
            {/if}
            <button
              type="button"
              onclick={(e) => removeItem(e, getValue(item))}
              class="rounded p-0.5 transition-colors"
              {disabled}
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
            >
              <X class="w-3 h-3" style="color: var(--ds-text-subtle);" />
            </button>
          </div>
        {/each}

        <!-- Search input -->
        <input
          bind:this={inputRef}
          use:melt={$input}
          type="text"
          placeholder={selectedItems.length === 0 ? resolvedPlaceholder : ''}
          {disabled}
          onkeydowncapture={handleKeydown}
          class="flex-1 min-w-[120px] px-1 py-0.5 bg-transparent border-0 outline-none text-sm"
          style="color: var(--ds-text);"
        />
      </div>

      <!-- Right side icons for multi-select -->
      <div class="absolute right-2 top-1/2 transform -translate-y-1/2 flex items-center gap-1 pointer-events-none">
        {#if loading}
          <Spinner size="sm" />
        {:else}
          <ChevronDown
            size={16}
            class="transition-transform duration-200 {$open ? 'rotate-180' : ''}"
            style="color: var(--ds-text-subtle);"
          />
        {/if}
      </div>
    {:else}
      <!-- Single-select: Input/Trigger -->
      <input
        use:melt={$input}
        {id}
        type="text"
        placeholder={resolvedPlaceholder}
        {disabled}
        onkeydowncapture={handleKeydown}
        class="w-full px-4 py-2 pr-16 rounded border transition-all duration-200
               focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50
               disabled:opacity-50 disabled:cursor-not-allowed text-sm"
        style="background-color: var(--ds-background-input);
               border-color: var(--ds-border);
               color: var(--ds-text);"
      />

      <!-- Right side icons for single-select -->
      <div class="absolute right-2 top-1/2 transform -translate-y-1/2 flex items-center gap-1">
        {#if allowClear && value != null && !disabled && showSelectedInTrigger}
          <button
            type="button"
            onclick={handleClear}
            class="p-0.5 rounded transition-colors"
            style="color: var(--ds-text-subtle);"
            aria-label={t('pickers.clearSelection')}
            onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
            onmouseleave={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
          >
            <X size={14} />
          </button>
        {/if}

        {#if loading}
          <Spinner size="sm" />
        {:else}
          <div class="pointer-events-none">
            <ChevronDown
              size={16}
              class="transition-transform duration-200 {$open ? 'rotate-180' : ''}"
              style="color: var(--ds-text-subtle);"
            />
          </div>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Dropdown Menu -->
  {#if $open && options.length > 0}
    <div
      bind:this={menuRef}
      use:melt={$menu}
      class="fixed z-50 min-w-[250px] rounded border shadow-lg max-h-60 overflow-y-auto"
      style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
      transition:fly={{ duration: 150, y: -5 }}
    >
      {#each options as opt, index (opt.value ?? 'unassigned')}
        {@const itemSelected = multiple ? isItemSelected(opt.value) : $isSelected(opt)}
        {@const isHighlighted = highlightedIndex === index}
        <div
          use:melt={$option(opt)}
          onclick={() => {
            if (multiple) {
              const itemValue = opt.value;
              if (isItemSelected(itemValue)) {
                value = (value || []).filter(v => v !== itemValue);
              } else {
                value = [...(value || []), itemValue];
              }
              $inputValue = '';
              onChange(value);
            } else {
              value = opt.value;
              $inputValue = opt.isUnassigned ? '' : opt.label;
              onSelect(opt.item);
            }
            $open = false;
          }}
          onmouseenter={() => highlightedIndex = index}
          class="px-4 py-3 cursor-pointer border-b last:border-b-0 transition-colors duration-150"
          style="border-color: var(--ds-border);
                 {itemSelected
                   ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
                   : isHighlighted
                     ? 'background-color: var(--ds-background-neutral-hovered); color: var(--ds-text);'
                     : 'color: var(--ds-text);'}"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-3 flex-1 min-w-0">
              {#if opt.isUnassigned}
                <!-- Unassigned option -->
                <span class="font-medium truncate" style="color: var(--ds-text-subtle);">{resolvedUnassignedLabel}</span>
              {:else if itemSnippet}
                <!-- Custom item rendering via snippet -->
                {@render itemSnippet({ item: opt.item, isSelected: itemSelected })}
              {:else}
                <!-- Default icon if provided -->
                {#if iconSnippet}
                  {@render iconSnippet({ item: opt.item })}
                {/if}

                <!-- Default item rendering -->
                <div class="flex flex-col min-w-0">
                  <span class="font-medium truncate">{opt.label}</span>
                </div>
              {/if}
            </div>

            {#if itemSelected}
              <Check class="w-4 h-4 text-blue-600 flex-shrink-0" />
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <!-- No Results -->
  {#if $open && $touchedInput && $inputValue.trim().length > 0 && options.length === 0 && !loading}
    <div
      class="absolute z-50 w-full mt-2 rounded border shadow-lg"
      style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
      transition:fly={{ duration: 150, y: -5 }}
    >
      {#if noResultsSnippet}
        {@render noResultsSnippet({ searchQuery: $inputValue })}
      {:else}
        <div class="px-4 py-4 text-center text-sm" style="color: var(--ds-text-subtle);">
          {t('pickers.noResultsFor', { query: $inputValue })}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Create option when search has results but doesn't match exactly -->
  {#if $open && allowCreate && $inputValue.trim().length > 0 && options.length > 0 && !options.some(opt => getLabel(opt.item)?.toLowerCase() === $inputValue.toLowerCase())}
    <div
      class="absolute z-50 w-full rounded border shadow-lg"
      style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); top: 100%; margin-top: calc(0.5rem + var(--dropdown-height, 15rem));"
    >
      {#if createOptionSnippet}
        {@render createOptionSnippet({ searchQuery: $inputValue, onCreate })}
      {:else if onCreate}
        <button
          type="button"
          class="w-full px-4 py-3 text-left hover:bg-gray-50 dark:hover:bg-gray-800 transition-colors flex items-center gap-2"
          style="color: var(--ds-interactive);"
          onclick={() => onCreate($inputValue)}
        >
          <span class="text-sm">{t('pickers.createItem', { name: $inputValue })}</span>
        </button>
      {/if}
    </div>
  {/if}

  <!-- Error State -->
  {#if error}
    <div
      class="absolute z-50 w-full mt-2 rounded border shadow-lg"
      style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);"
    >
      <div class="px-4 py-4 text-center text-sm text-red-600">
        {error}
      </div>
    </div>
  {/if}
</div>
