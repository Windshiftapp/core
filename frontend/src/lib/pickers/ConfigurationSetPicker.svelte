<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { ChevronDown, X, Settings } from 'lucide-svelte';
  import { createEventDispatcher } from 'svelte';

  const dispatch = createEventDispatcher();

  // Generate unique IDs for ARIA attributes
  const listboxId = `listbox-${Math.random().toString(36).slice(2, 9)}`;
  const getOptionId = (index) => `${listboxId}-option-${index}`;

  // Props
  let {
    value = $bindable(null),
    items = [],
    placeholder = 'Select configuration set...',
    disabled = false,
    class: className = '',
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  // State
  let searchTerm = $state('');
  let highlightedIndex = $state(0);
  let inputElement = $state(null);

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
          onCancel();
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
      if (item?.id == null || value == null) {
        return item?.id === value;
      }
      return Number(item.id) === Number(value);
    }) || null
  );

  let filteredItems = $derived.by(() => {
    if (!searchTerm.trim()) {
      return items;
    }
    const term = searchTerm.toLowerCase();
    return items.filter(item => {
      const name = item.name || '';
      const description = item.description || '';
      return name.toLowerCase().includes(term) || description.toLowerCase().includes(term);
    });
  });

  // Handle selection
  function handleSelect(item) {
    wasSelectionMade = true;
    value = item ? item.id : null;
    $open = false;
    onSelect(item);
    dispatch('select', item);
  }

  // Handle keyboard navigation
  function handleKeyDown(e) {
    const totalItems = 1 + filteredItems.length; // +1 for "Default Configuration"

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
      if (highlightedIndex === 0) {
        handleSelect(null);
      } else {
        const adjustedIndex = highlightedIndex - 1;
        if (adjustedIndex >= 0 && adjustedIndex < filteredItems.length) {
          handleSelect(filteredItems[adjustedIndex]);
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
</script>

<!-- Trigger Button -->
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
    {#if selectedItem}
      <!-- Show selected item -->
      <Settings size={16} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      <span class="truncate">{selectedItem.name}</span>
    {:else if value === null}
      <!-- Default Configuration selected -->
      <Settings size={16} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      <span class="truncate">Default Configuration</span>
    {:else}
      <!-- Show placeholder -->
      <span style="color: var(--ds-text-subtle);">{placeholder}</span>
    {/if}
  </div>

  <div class="flex items-center gap-1 flex-shrink-0">
    {#if selectedItem && !disabled}
      <button
        type="button"
        onclick={handleClear}
        class="p-0.5 rounded hover:bg-opacity-10"
        style="color: var(--ds-text-subtle);"
        aria-label="Clear selection"
      >
        <X size={14} />
      </button>
    {/if}
    <ChevronDown size={16} style="color: var(--ds-text-subtle);" />
  </div>
</button>

<!-- Popover Content -->
{#if $open}
  <div
    use:melt={$content}
    class="z-50 rounded shadow-lg overflow-hidden"
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
        placeholder="Search configuration sets..."
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
    <div class="max-h-80 overflow-y-auto" role="listbox" id={listboxId} aria-label="Configuration sets">
      <!-- Default Configuration Option -->
      <button
        type="button"
        onclick={() => handleSelect(null)}
        class="w-full px-3 py-2.5 text-left text-sm transition-colors"
        style="
          background-color: {highlightedIndex === 0 ? 'var(--ds-background-neutral-hovered)' : value === null ? 'var(--ds-background-selected)' : 'transparent'};
          border-bottom: 1px solid var(--ds-border);
        "
        role="option"
        id={getOptionId(0)}
        aria-selected={value === null}
        onmouseover={(e) => {
          highlightedIndex = 0;
          if (value !== null) {
            e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)';
          }
        }}
        onmouseout={(e) => {
          if (value !== null && highlightedIndex !== 0) {
            e.currentTarget.style.backgroundColor = 'transparent';
          }
        }}
      >
        <div class="flex items-center gap-3">
          <div class="w-7 h-7 rounded flex items-center justify-center flex-shrink-0"
               style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
            <Settings class="w-4 h-4" />
          </div>
          <div class="flex flex-col min-w-0">
            <span class="font-medium" style="color: var(--ds-text);">Default Configuration</span>
            <span class="text-xs" style="color: var(--ds-text-subtle);">Use the system default configuration set</span>
          </div>
        </div>
      </button>

      <!-- Configuration Set Items -->
      {#each filteredItems as configSet, index}
        {@const itemIndex = index + 1}
        {@const isHighlighted = highlightedIndex === itemIndex}
        {@const isSelected = value === configSet.id}

        <button
          type="button"
          onclick={() => handleSelect(configSet)}
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
          <div class="flex items-center gap-3">
            <div class="w-7 h-7 rounded flex items-center justify-center flex-shrink-0"
                 style="background: var(--ds-background-neutral); color: var(--ds-text-subtle);">
              <Settings class="w-4 h-4" />
            </div>
            <div class="flex flex-col min-w-0">
              <span class="font-medium truncate" style="color: var(--ds-text);">{configSet.name}</span>
              {#if configSet.description}
                <span class="text-xs truncate" style="color: var(--ds-text-subtle);">
                  {configSet.description}
                </span>
              {/if}
            </div>
          </div>
        </button>
      {/each}

      <!-- No Results -->
      {#if filteredItems.length === 0 && searchTerm}
        <div class="p-4 text-center" style="color: var(--ds-text-subtle);">
          No configuration sets found
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  button {
    font-family: inherit;
  }
</style>
