<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { ChevronDown, Check } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { getVisibleColor } from '../utils/colorUtils.js';
  import Input from '../components/Input.svelte';

  let {
    value = $bindable(null),
    items = [],
    getValue = (item) => item?.id,
    getLabel = (item) => item?.name ?? '',
    icon: Icon = null,
    colorDot = null,
    placeholder = '',
    disabled = false,
    required = false,
    searchable = false,
    searchFields = ['name'],
    triggerSnippet = null,
    itemSnippet = null,
    onSelect = () => {},
    testId = null
  } = $props();

  const listboxId = `chiplist-${Math.random().toString(36).slice(2, 9)}`;

  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    positioning: {
      strategy: 'fixed',
      fitViewport: true,
      placement: 'bottom-start',
      gutter: 4,
      flip: true
    },
    portal: 'body',
    forceVisible: true
  });

  let searchTerm = $state('');
  let highlightedIndex = $state(0);
  let inputElement = $state(null);
  let listRef = $state(null);

  // Derive display value from current value
  let selectedItem = $derived(
    items.find(item => getValue(item) === value) || null
  );

  let displayValue = $derived(selectedItem ? getLabel(selectedItem) : '');
  let showValue = $derived(displayValue || (value !== null && value !== undefined));

  // Filter items when searchable
  let filteredItems = $derived.by(() => {
    if (!searchable || !searchTerm.trim()) return items;
    const term = searchTerm.toLowerCase();
    return items.filter(item =>
      searchFields.some(field => {
        const fieldValue = typeof field === 'function' ? field(item) : item[field];
        return fieldValue?.toString().toLowerCase().includes(term);
      })
    );
  });

  // Focus input when popover opens, reset state
  $effect(() => {
    if ($open) {
      searchTerm = '';
      highlightedIndex = 0;
      if (searchable) {
        setTimeout(() => inputElement?.focus(), 50);
      } else {
        setTimeout(() => listRef?.focus(), 50);
      }
    }
  });

  // Reset highlighted index when filtered items change
  $effect(() => {
    const len = filteredItems.length;
    if (highlightedIndex >= len) {
      highlightedIndex = Math.max(0, len - 1);
    }
  });

  // Scroll highlighted item into view
  $effect(() => {
    if ($open && listRef && filteredItems.length > 0) {
      const highlightedEl = listRef.children[highlightedIndex];
      if (highlightedEl) {
        highlightedEl.scrollIntoView({ block: 'nearest' });
      }
    }
  });

  // Melt restores focus to the trigger after closing. Let that single delayed
  // focus operation own the hand-off: focusing here as well creates a race
  // where a fast Tab reaches the next control before Melt moves focus back.
  function closePicker() {
    $open = false;
  }

  function handleSelect(item) {
    value = getValue(item);
    closePicker();
    onSelect(item);
  }

  function handleKeyDown(e) {
    const total = filteredItems.length;

    // Tab is intentionally left to native handling: when closed it moves to the
    // next field (no trap, WI-445); when open the user selects with Enter rather
    // than tabbing out. Melt restores focus to the trigger on select/Escape;
    // Tab itself is not intercepted.
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      closePicker();
      return;
    }

    if (!$open || total === 0) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      e.stopPropagation();
      highlightedIndex = (highlightedIndex + 1) % total;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      e.stopPropagation();
      highlightedIndex = highlightedIndex === 0 ? total - 1 : highlightedIndex - 1;
    } else if (e.key === 'Enter' || (e.key === ' ' && e.target.tagName !== 'INPUT')) {
      e.preventDefault();
      e.stopPropagation();
      if (highlightedIndex >= 0 && highlightedIndex < total) {
        handleSelect(filteredItems[highlightedIndex]);
      }
    }
  }
</script>

<!-- Chip Trigger Button -->
<button
  use:melt={$trigger}
  {disabled}
  data-testid={testId}
  onkeydown={handleKeyDown}
  class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-sm transition-colors"
  style="
    background-color: var(--ds-surface);
    border: 1px solid {required && !showValue ? 'var(--ds-border-danger, #ef4444)' : 'var(--ds-border)'};
    color: {showValue ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};
    opacity: {disabled ? 0.5 : 1};
    cursor: {disabled ? 'not-allowed' : 'pointer'};
  "
  onmouseover={(e) => {
    if (!disabled) {
      e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)';
    }
  }}
  onmouseout={(e) => {
    e.currentTarget.style.backgroundColor = 'var(--ds-surface)';
  }}
>
  {#if triggerSnippet && selectedItem}
    {@render triggerSnippet({ item: selectedItem })}
  {:else if Icon}
    <Icon size={14} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
  {/if}
  {#if colorDot}
    <div class="w-2 h-2 rounded-full flex-shrink-0" style="background-color: {getVisibleColor(colorDot)};"></div>
  {/if}
  <span class="truncate max-w-[120px]">
    {displayValue || placeholder}
  </span>
  <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
</button>

<!-- Popover Content -->
{#if $open}
  <div
    use:melt={$content}
    data-testid={testId ? `${testId}-dropdown` : undefined}
    class="z-[70] rounded-lg shadow-lg flex flex-col overflow-hidden"
    style="
      background-color: var(--ds-surface-raised);
      border: 1px solid var(--ds-border);
      min-width: 200px;
      max-width: 320px;
    "
  >
    <!-- Search Input (optional) -->
    {#if searchable}
      <div class="p-2 border-b shrink-0" style="border-color: var(--ds-border);">
        <Input
          bind:inputRef={inputElement}
          bind:value={searchTerm}
          dataTestid={testId ? `${testId}-search` : undefined}
          onkeydown={handleKeyDown}
          type="text"
          placeholder={t('pickers.search')}
          size="small"
        />
      </div>
    {/if}

    <!-- Items List -->
    <div
      bind:this={listRef}
      data-testid={testId ? `${testId}-listbox` : undefined}
      class="min-h-0 max-h-48 overflow-y-auto overscroll-contain"
      role="listbox"
      id={listboxId}
      tabindex="0"
      onkeydown={handleKeyDown}
      style="outline: none;"
    >
      {#if filteredItems.length === 0}
        <div class="p-4 text-center text-sm" style="color: var(--ds-text-subtle);">
          {t('pickers.noItemsFound')}
        </div>
      {:else}
        {#each filteredItems as item, index}
          {@const itemValue = getValue(item)}
          {@const isSelected = itemValue === value}
          {@const isHighlighted = highlightedIndex === index}
          <button
            type="button"
            data-testid={testId ? `${testId}-option` : undefined}
            class="w-full flex items-center gap-2 px-3 py-2.5 text-left text-sm transition-colors"
            style="
              background-color: {isSelected ? 'var(--ds-background-selected)' : isHighlighted ? 'var(--ds-background-neutral-hovered)' : 'transparent'};
              color: var(--ds-text);
            "
            role="option"
            aria-selected={isSelected}
            onmouseenter={() => highlightedIndex = index}
            onclick={() => handleSelect(item)}
          >
            <div class="flex items-center gap-2 flex-1 min-w-0">
              {#if itemSnippet}
                {@render itemSnippet({ item, isSelected })}
              {:else}
                <span class="truncate">{getLabel(item)}</span>
              {/if}
            </div>
            {#if isSelected}
              <Check size={14} class="flex-shrink-0" style="color: var(--ds-interactive);" />
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  </div>
{/if}
