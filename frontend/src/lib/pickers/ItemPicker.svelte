<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { ChevronDown, X } from 'lucide-svelte';
  import { createEventDispatcher } from 'svelte';
  import { getVisibleColor } from '../utils/colorUtils.js';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  // Generate unique IDs for ARIA attributes
  const listboxId = `listbox-${Math.random().toString(36).slice(2, 9)}`;
  const getOptionId = (index) => `${listboxId}-option-${index}`;

  // Props
  let {
    value = $bindable(null),
    items = [],
    config = {},
    placeholder = '',
    showUnassigned = false,
    unassignedLabel = '',
    disabled = false,
    allowClear = true,
    loading = false,
    autoOpen = false,
    showSelectedInTrigger = true,
    class: className = '',
    children = null,  // Optional custom trigger snippet
    onSearchChange = null,  // Callback for async search: (searchTerm) => void
    searchDebounce = 300  // Debounce delay for onSearchChange in ms
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.select'));
  const resolvedUnassignedLabel = $derived(unassignedLabel || t('common.none'));

  // Default config values
  const defaultConfig = {
    icon: null,
    primary: { text: (item) => item.name || item.label || '' },
    secondary: null,
    badges: [],
    metadata: [],
    searchFields: ['name', 'label'],
    getValue: (item) => item.id,
    getLabel: (item) => item.name || item.label || ''
  };

  // Merge config with defaults
  let finalConfig = $derived({ ...defaultConfig, ...config });

  // State
  let searchTerm = $state('');
  let highlightedIndex = $state(0);
  let inputElement = $state(null);
  let searchDebounceTimeout = null;

  // Debounced search change callback for async search
  $effect(() => {
    if (onSearchChange) {
      if (searchDebounceTimeout) {
        clearTimeout(searchDebounceTimeout);
      }
      const term = searchTerm;
      searchDebounceTimeout = setTimeout(() => {
        onSearchChange(term);
      }, searchDebounce);
    }
  });

  // Create popover
  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    positioning: {
      placement: 'bottom-start',
      gutter: 8
    },
    portal: 'body',
    forceVisible: true,
    onOpenChange: ({ next }) => {
      if (next) {
        // Popover opening - reset search
        searchTerm = '';
        highlightedIndex = 0;
        // Focus input after a brief delay
        setTimeout(() => {
          inputElement?.focus();
        }, 50);
      } else {
        // Popover closing without selection - dispatch cancel
        if (!wasSelectionMade) {
          dispatch('cancel');
        }
        wasSelectionMade = false;
      }
      return next;
    }
  });

  let wasSelectionMade = false;

  // Derived state
  let selectedItem = $derived(
    items.find(item => {
      const itemValue = finalConfig.getValue(item);
      if (itemValue == null || value == null) {
        return itemValue === value;
      }
      // Compare as numbers for numeric IDs to handle type coercion
      return Number(itemValue) === Number(value);
    }) || null
  );

  let filteredItems = $derived.by(() => {
    // When onSearchChange is provided, skip client-side filtering (parent handles it via API)
    if (onSearchChange) {
      return items;
    }
    if (!searchTerm.trim()) {
      return items;
    }
    const term = searchTerm.toLowerCase();
    return items.filter(item => {
      return finalConfig.searchFields.some(field => {
        const fieldValue = item[field];
        return fieldValue && String(fieldValue).toLowerCase().includes(term);
      });
    });
  });

  // Auto-open popover when autoOpen is true
  $effect(() => {
    if (autoOpen) {
      $open = true;
    }
  });

  // Handle selection
  function handleSelect(item) {
    wasSelectionMade = true;
    value = item ? finalConfig.getValue(item) : null;
    $open = false;
    dispatch('select', item);
  }

  // Handle keyboard navigation
  function handleKeyDown(e) {
    const itemsList = filteredItems;
    const totalItems = (showUnassigned ? 1 : 0) + itemsList.length;

    // Tab closes the dropdown without preventing default (allows focus to move)
    if (e.key === 'Tab') {
      $open = false;
      return;
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      highlightedIndex = (highlightedIndex + 1) % totalItems;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      highlightedIndex = highlightedIndex === 0 ? totalItems - 1 : highlightedIndex - 1;
    } else if (e.key === 'Enter' || (e.key === ' ' && e.target.tagName !== 'INPUT')) {
      e.preventDefault();
      if (showUnassigned && highlightedIndex === 0) {
        handleSelect(null);
      } else {
        const adjustedIndex = showUnassigned ? highlightedIndex - 1 : highlightedIndex;
        if (adjustedIndex >= 0 && adjustedIndex < itemsList.length) {
          handleSelect(itemsList[adjustedIndex]);
        }
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      $open = false;
    }
  }

  // Clear selection
  function handleClear(e) {
    e.stopPropagation();
    handleSelect(null);
  }

  // Helper: Render icon
  function renderIcon(item, size = 16) {
    if (!finalConfig.icon) return null;

    const { type, source } = finalConfig.icon;

    if (type === 'component') {
      return source(item);
    } else if (type === 'color-dot') {
      const color = source(item);
      const dotSize = finalConfig.icon.size || 'w-2 h-2';
      return { type: 'color-dot', color, size: dotSize };
    }
    return null;
  }

  // Helper: Format date
  function formatDate(dateStr) {
    if (!dateStr) return '';
    const date = new Date(dateStr);
    return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }
</script>

<!-- Trigger Button -->
{#if children}
  <!-- Custom trigger provided via slot -->
  <button
    use:melt={$trigger}
    {disabled}
    class="cursor-pointer {className}"
    style:opacity={disabled ? 0.5 : 1}
    style:cursor={disabled ? 'not-allowed' : 'pointer'}
    role="combobox"
    aria-expanded={$open}
    aria-controls={listboxId}
    aria-haspopup="listbox"
  >
    {@render children()}
  </button>
{:else}
  <!-- Default trigger button -->
  <button
    use:melt={$trigger}
    {disabled}
    class="relative w-full flex items-center justify-between gap-2 px-3 py-2 rounded text-sm transition-colors {className}"
    style="
      background-color: var(--ds-background-input);
      border: 1px solid var(--ds-border);
      color: var(--ds-text);
    "
    style:opacity={disabled ? 0.5 : 1}
    style:cursor={disabled ? 'not-allowed' : 'pointer'}
    role="combobox"
    aria-expanded={$open}
    aria-controls={listboxId}
    aria-haspopup="listbox"
    onmouseover={(e) => {
      if (!disabled) {
        e.currentTarget.style.backgroundColor = 'var(--ds-background-input-hovered)';
      }
    }}
    onmouseout={(e) => {
      e.currentTarget.style.backgroundColor = 'var(--ds-background-input)';
    }}
    onfocus={(e) => {
      e.currentTarget.style.borderColor = 'var(--ds-border-focused)';
    }}
    onblur={(e) => {
      e.currentTarget.style.borderColor = 'var(--ds-border)';
    }}
  >
    <div class="flex items-center gap-2 flex-1 min-w-0">
      {#if selectedItem && showSelectedInTrigger}
        <!-- Show selected item -->
        {@const iconData = renderIcon(selectedItem, 16)}
        {#if iconData}
          {#if iconData.type === 'color-dot'}
            <div class="{iconData.size} rounded-full flex-shrink-0" style="background-color: {getVisibleColor(iconData.color)};"></div>
          {:else}
            <svelte:component this={iconData} size={16} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
          {/if}
        {/if}
        <span class="truncate">{finalConfig.getLabel(selectedItem)}</span>
      {:else}
        <!-- Show placeholder -->
        <span style="color: var(--ds-text-subtle);">{resolvedPlaceholder}</span>
      {/if}
    </div>

    <div class="flex items-center gap-1 flex-shrink-0">
      {#if allowClear && selectedItem && !disabled && showSelectedInTrigger}
        <button
          type="button"
          onclick={handleClear}
          class="p-0.5 rounded hover:bg-opacity-10"
          style="color: var(--ds-text-subtle);"
          aria-label={t('pickers.clearSelection')}
        >
          <X size={14} />
        </button>
      {/if}
      <ChevronDown size={16} style="color: var(--ds-text-subtle);" />
    </div>
  </button>
{/if}

<!-- Popover Content -->
{#if $open}
  <div
    use:melt={$content}
    class="z-[60] rounded shadow-lg overflow-hidden"
    style="
      background-color: var(--ds-surface-raised);
      border: 1px solid var(--ds-border);
      min-width: 320px;
      max-width: 400px;
    "
    onkeydown={(e) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        $open = false;
      }
    }}
  >
    <!-- Search Input -->
    <div class="p-2 border-b" style="border-color: var(--ds-border);">
      <input
        bind:this={inputElement}
        bind:value={searchTerm}
        onkeydown={handleKeyDown}
        type="text"
        placeholder={t('pickers.search')}
        class="w-full px-3 py-2 rounded text-sm outline-none"
        style="
          background-color: var(--ds-background-input);
          border: 1px solid var(--ds-border);
          color: var(--ds-text);
        "
        aria-controls={listboxId}
        aria-activedescendant={getOptionId(highlightedIndex)}
        aria-autocomplete="list"
        onfocus={(e) => {
          e.currentTarget.style.borderColor = 'var(--ds-border-focused)';
        }}
        onblur={(e) => {
          e.currentTarget.style.borderColor = 'var(--ds-border)';
        }}
      />
    </div>

    <!-- Items List -->
    <div class="max-h-80 overflow-y-auto" role="listbox" id={listboxId} aria-label={t('pickers.options')}>
      {#if loading}
        <div class="p-4 text-center" style="color: var(--ds-text-subtle);">
          {t('common.loading')}
        </div>
      {:else}
        <!-- Unassigned Option -->
        {#if showUnassigned}
          <button
            type="button"
            onclick={() => handleSelect(null)}
            class="w-full px-3 py-2 text-left text-sm transition-colors flex items-center gap-2"
            style="
              color: var(--ds-text);
              background-color: {highlightedIndex === 0 ? 'var(--ds-background-neutral-hovered)' : 'transparent'};
            "
            role="option"
            id={getOptionId(0)}
            aria-selected={value === null}
            onmouseover={(e) => {
              highlightedIndex = 0;
              e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)';
            }}
            onmouseout={(e) => {
              if (highlightedIndex !== 0) {
                e.currentTarget.style.backgroundColor = 'transparent';
              }
            }}
          >
            <span style="color: var(--ds-text-subtle);">{resolvedUnassignedLabel}</span>
          </button>
        {/if}

        <!-- Items -->
        {#each filteredItems as item, index}
          {@const itemIndex = showUnassigned ? index + 1 : index}
          {@const isHighlighted = highlightedIndex === itemIndex}
          {@const isSelected = value === finalConfig.getValue(item)}
          {@const iconData = renderIcon(item, 16)}

          <button
            type="button"
            onclick={() => handleSelect(item)}
            class="w-full px-3 py-2.5 text-left text-sm transition-colors"
            style="
              background-color: {isSelected ? 'var(--ds-background-selected)' : isHighlighted ? 'var(--ds-background-neutral-hovered)' : 'transparent'};
              border-bottom: 1px solid var(--ds-border);
            "
            role="option"
            id={getOptionId(itemIndex)}
            aria-selected={isSelected}
            onmouseover={(e) => {
              highlightedIndex = itemIndex;
              if (!isSelected) {
                e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)';
              }
            }}
            onmouseout={(e) => {
              if (!isSelected && highlightedIndex !== itemIndex) {
                e.currentTarget.style.backgroundColor = 'transparent';
              }
            }}
          >
            <!-- Item Header -->
            <div class="flex items-center gap-2 mb-1">
              <!-- Icon -->
              {#if iconData}
                {#if iconData.type === 'color-dot'}
                  <div class="{iconData.size} rounded-full flex-shrink-0" style="background-color: {getVisibleColor(iconData.color)};"></div>
                {:else}
                  <svelte:component this={iconData} size={16} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
                {/if}
              {/if}

              <!-- Primary text -->
              <span class="font-medium" style="color: var(--ds-text);">
                {finalConfig.primary.text(item)}
              </span>

              <!-- Badges -->
              {#each finalConfig.badges as badge}
                {@const badgeText = badge.text(item)}
                {#if badgeText}
                  <span
                    class="px-1.5 py-0.5 rounded text-xs"
                    style="
                      background-color: {badge.bgColor ? badge.bgColor(item) : 'var(--ds-background-neutral)'};
                      color: {badge.textColor ? badge.textColor(item) : 'var(--ds-text-subtle)'};
                    "
                  >
                    {badgeText}
                  </span>
                {/if}
              {/each}
            </div>

            <!-- Secondary text -->
            {#if finalConfig.secondary}
              {@const secondaryText = finalConfig.secondary.text(item)}
              {#if secondaryText}
                <div class="text-xs mb-1" style="color: var(--ds-text-subtle);">
                  {secondaryText}
                </div>
              {/if}
            {/if}

            <!-- Metadata -->
            {#each finalConfig.metadata as meta}
              {#if meta.type === 'date-range'}
                {@const startDate = meta.startDate(item)}
                {@const endDate = meta.endDate(item)}
                {#if startDate || endDate}
                  <div class="flex items-center gap-2 text-xs mb-1" style="color: var(--ds-text-subtle);">
                    {#if meta.icon}
                      <svelte:component this={meta.icon} size={12} />
                    {/if}
                    <span>
                      {formatDate(startDate)} → {formatDate(endDate)}
                    </span>
                  </div>
                {/if}
              {:else if meta.type === 'badge'}
                {@const badgeText = meta.text(item)}
                {#if badgeText}
                  <div class="flex items-center gap-2">
                    <span
                      class="inline-block px-2 py-0.5 rounded text-xs font-medium"
                      style="
                        background-color: {meta.bgColor ? meta.bgColor(item) : 'var(--ds-background-neutral)'};
                        color: {meta.textColor ? meta.textColor(item) : 'var(--ds-text)'};
                      "
                    >
                      {badgeText}
                    </span>
                  </div>
                {/if}
              {:else if meta.type === 'text'}
                {@const text = meta.text(item)}
                {#if text}
                  <div class="flex items-center gap-2 text-xs mb-1" style="color: var(--ds-text-subtle);">
                    {#if meta.icon}
                      <svelte:component this={meta.icon} size={12} />
                    {/if}
                    <span>{text}</span>
                  </div>
                {/if}
              {/if}
            {/each}
          </button>
        {/each}

        <!-- No Results -->
        {#if filteredItems.length === 0 && !showUnassigned}
          <div class="p-4 text-center" style="color: var(--ds-text-subtle);">
            {searchTerm ? t('pickers.noItemsFound') : t('pickers.noItemsAvailable')}
          </div>
        {/if}
      {/if}
    </div>
  </div>
{/if}

<style>
  button {
    font-family: inherit;
  }
</style>
