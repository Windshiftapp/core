<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { controlSizeClasses } from '../utils/controlSizes.js';
  import { fieldControl } from '../utils/fieldContext.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import Check from '@lucide/svelte/icons/check';
  import Search from '@lucide/svelte/icons/search';
  import { tick, untrack } from 'svelte';
  import { derived } from 'svelte/store';
  import { createOwnedPopup } from '../utils/overlayScope.js';
  const ownedPopup = createOwnedPopup();

  /**
   * @type {{
   *   value?: any,
   *   options?: any[],
   *   disabled?: boolean,
   *   required?: boolean,
   *   size?: string,
   *   id?: string,
   *   class?: string,
   *   menuWidth?: string,
   *   portalOwner?: string,
   *   placeholder?: string,
    *   ariaLabel?: string,
    *   ariaDescribedby?: string,
    *   ariaInvalid?: boolean,
    *   dataTestid?: string,
    *   searchable?: boolean,
    *   searchPlaceholder?: string,
   *   onchange?: (e?: any) => void,
   *   onfocus?: (e?: any) => void,
   *   onblur?: (e?: any) => void,
   * }}
   */
  let {
    value = $bindable(''),
    options = [],
    disabled = false,
    required = false,
    size = 'medium',
    id = undefined,
    class: className = '',
    menuWidth = '',
    portalOwner = undefined,
    placeholder = '',
    ariaLabel = undefined,
    ariaDescribedby = undefined,
    ariaInvalid = undefined,
    dataTestid = undefined,
    onchange = undefined,
    onfocus = undefined,
    onblur = undefined,
    searchable = false,
    searchPlaceholder = t('placeholders.search')
  } = $props();
  const instanceId = $props.id();
  const listboxId = `${instanceId}-listbox`;
  const field = fieldControl(() => ({ id, ariaDescribedby, ariaInvalid, required }));

  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    disableFocusTrap: ownedPopup.withinModal,
    openFocus: null,
    closeFocus: ownedPopup.withinModal ? null : undefined,
    forceVisible: true,
    positioning: {
      strategy: 'fixed',
      fitViewport: true,
      placement: 'bottom-start',
      sameWidth: untrack(() => !menuWidth),
      gutter: 4
    },
    portal: 'body'
  });
  const fieldTrigger = derived(trigger, attributes => Object.defineProperty(
    { ...attributes, role: 'combobox', 'aria-haspopup': 'listbox' }, 'action', { value: attributes.action }
  ));

  let triggerElement = $state(null);
  let listboxElement = $state(null);
  let highlightedIndex = $state(-1);
  let query = $state('');
  let searchInput = $state(null);

  // A searchable select filters by the visible label plus optional keywords,
  // so a currency can show just its code while still matching "Swiss Franc".
  const filteredOptions = $derived.by(() => {
    if (!searchable) return options;
    const term = query.trim().toLowerCase();
    if (!term) return options;
    return options.filter((option) => {
      const keywords = Array.isArray(option.keywords) ? option.keywords.join(' ') : option.keywords || '';
      return `${option.label ?? ''} ${keywords}`.toLowerCase().includes(term);
    });
  });

  // Find the selected option label
  const selectedOption = $derived(options.find(o => String(o.value) === String(value)));
  const displayText = $derived(selectedOption?.label || placeholder || '');

  // Size variants
  const sizeClasses = $derived(controlSizeClasses(size));

  // Open dropdown and focus selected/first item
  function openDropdown() {
    if (disabled) return;
    query = '';
    open.set(true);
    tick().then(() => {
      const selectedIdx = filteredOptions.findIndex(o => String(o.value) === String(value));
      highlightedIndex = selectedIdx >= 0 ? selectedIdx : 0;
      scrollToHighlighted();
    });
  }

  function handleSearchInput(event) {
    query = event.currentTarget.value;
    highlightedIndex = 0;
    scrollToHighlighted();
  }

  function closeDropdown() {
    open.set(false);
    highlightedIndex = -1;
  }

  function selectOption(opt) {
    if (opt.disabled) return;
    value = opt.value;
    onchange?.(opt.value);
    closeDropdown();
    tick().then(() => triggerElement?.focus({ preventScroll: true }));
  }

  function optionId(opt, index) {
    if (id) return `${id}-option-${String(opt.value).replace(/[^A-Za-z0-9_-]/g, '-')}`;
    return `${field.id ?? instanceId}-option-${index}`;
  }

  function scrollToHighlighted() {
    tick().then(() => {
      if (!listboxElement) return;
      const items = listboxElement.querySelectorAll('[role="option"]');
      const item = items[highlightedIndex];
      if (!item) return;
      // Scroll only the dropdown's own list. scrollIntoView() walks every
      // scrollable ancestor, so opening the menu could scroll the surrounding
      // modal and leave the trigger (and the fixed popover) off-screen.
      const scroller = item.closest('[data-select-scroller]');
      if (!scroller) return;
      const scrollerRect = scroller.getBoundingClientRect();
      const itemRect = item.getBoundingClientRect();
      if (itemRect.top < scrollerRect.top) {
        scroller.scrollTop += itemRect.top - scrollerRect.top;
      } else if (itemRect.bottom > scrollerRect.bottom) {
        scroller.scrollTop += itemRect.bottom - scrollerRect.bottom;
      }
    });
  }

  function handleTriggerKeydown(e) {
    if (disabled) return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      if (!$open) {
        openDropdown();
      }
    } else if (e.key === 'Escape' && $open) {
      e.preventDefault();
      closeDropdown();
      triggerElement?.focus({ preventScroll: true });
    }
  }

  function handleListboxKeydown(e) {
    if (e.key === 'Tab') return;
    const total = filteredOptions.length;
    const fromSearch = e.target === searchInput;

    if (e.key === 'Escape') {
      e.preventDefault();
      closeDropdown();
      triggerElement?.focus({ preventScroll: true });
      return;
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (total === 0) return;
      highlightedIndex = (highlightedIndex + 1) % total;
      scrollToHighlighted();
      return;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (total === 0) return;
      highlightedIndex = highlightedIndex <= 0 ? total - 1 : highlightedIndex - 1;
      scrollToHighlighted();
      return;
    }
    if (e.key === 'Enter') {
      e.preventDefault();
      if (highlightedIndex >= 0 && highlightedIndex < total && !filteredOptions[highlightedIndex].disabled) {
        selectOption(filteredOptions[highlightedIndex]);
      }
      return;
    }
    // The search field owns printable keys (including Space) and caret moves.
    if (fromSearch) return;
    if (total === 0) return;
    if (e.key === ' ') {
      e.preventDefault();
      if (highlightedIndex >= 0 && highlightedIndex < total && !filteredOptions[highlightedIndex].disabled) {
        selectOption(filteredOptions[highlightedIndex]);
      }
    } else if (e.key === 'Home') {
      e.preventDefault();
      highlightedIndex = 0;
      scrollToHighlighted();
    } else if (e.key === 'End') {
      e.preventDefault();
      highlightedIndex = total - 1;
      scrollToHighlighted();
    }
  }

  // When open changes, manage focus
  let wasOpen = $state(false);
  $effect(() => {
    if ($open && !wasOpen) {
      tick().then(() => {
        if (!$open) return;
        if (searchable) searchInput?.focus({ preventScroll: true });
        else listboxElement?.focus({ preventScroll: true });
      });
    }
    wasOpen = $open;
  });
</script>

<div class="relative {className}">
  <!-- Trigger button -->
  <button
    bind:this={triggerElement}
    use:melt={$fieldTrigger}
    type="button"
    id={field.id}
    data-testid={dataTestid}
    aria-describedby={field.describedby}
    aria-invalid={field.invalid}
    {disabled}
    role="combobox"
    aria-required={field.ariaRequired || undefined}
    aria-haspopup="listbox"
    aria-expanded={$open}
    aria-label={ariaLabel}
    class="w-full rounded border transition-all duration-200 flex items-center justify-between gap-2
           focus:outline-none focus:ring-2 focus:ring-ds-border-focused
           disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer text-left {sizeClasses}"
    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
    onkeydown={handleTriggerKeydown}
    onfocus={onfocus}
    onblur={onblur}
  >
    <span class="truncate flex-1 {!selectedOption ? 'opacity-60' : ''}" dir="auto">
      {displayText || '\u00A0'}
    </span>
    <ChevronDown
      class="w-4 h-4 flex-shrink-0 transition-transform duration-200 {$open ? 'rotate-180' : ''}"
      style="color: var(--ds-text-subtle);"
    />
  </button>
</div>

{#if $open}
  <div
    use:melt={$content}
    use:ownedPopup={{ trigger: () => triggerElement, close: closeDropdown, keepTabStops: false }}
    data-popover-owner={portalOwner}
    onkeydowncapture={handleListboxKeydown}
    class="rounded border shadow-lg flex flex-col overflow-hidden z-[60] focus:outline-none"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); width: {menuWidth || undefined};
           box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.25), 0 10px 10px -5px rgba(0, 0, 0, 0.15);"
  >
    {#if searchable}
      <div class="shrink-0 border-b border-ds-border p-2">
        <div class="relative">
          <Search size={14} class="absolute left-2.5 top-1/2 -translate-y-1/2 text-ds-text-subtle" />
          <input
            bind:this={searchInput}
            type="text"
            value={query}
            oninput={handleSearchInput}
            placeholder={searchPlaceholder}
            aria-label={searchPlaceholder}
            aria-controls={listboxId}
            aria-autocomplete="list"
            autocomplete="off"
            class="w-full rounded pl-8 pr-3 py-2 text-sm outline-none focus:ring-2 focus:ring-ds-border-focused bg-ds-background-input border border-ds-border text-ds-text"
          />
        </div>
      </div>
    {/if}
    <div
      bind:this={listboxElement}
      id={listboxId}
      data-testid={id ? `${id}-listbox` : undefined}
      role="listbox"
      tabindex="-1"
      aria-activedescendant={highlightedIndex >= 0 && filteredOptions[highlightedIndex] ? optionId(filteredOptions[highlightedIndex], highlightedIndex) : undefined}
      data-select-scroller
      class="min-h-0 max-h-60 overflow-y-auto overscroll-contain focus:outline-none"
    >
    {#each filteredOptions as opt, index (opt.value ?? `opt-${index}`)}
      {@const isSelected = String(opt.value) === String(value)}
      {@const isHighlighted = highlightedIndex === index}
      <div
        id={optionId(opt, index)}
        data-testid={id ? `${id}-option` : undefined}
        data-option-id={opt.value ?? ''}
        role="option"
        tabindex="-1"
        aria-selected={isSelected}
        class="px-3 py-2 text-sm flex items-center justify-between gap-2 transition-colors duration-100
               {opt.disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}"
        style="{isSelected
          ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
          : isHighlighted && !opt.disabled
            ? 'background-color: var(--ds-background-neutral-hovered); color: var(--ds-text);'
            : 'color: var(--ds-text);'}"
        onclick={() => selectOption(opt)}
        onkeydown={(e) => { if ((e.key === 'Enter' || e.key === ' ') && !opt.disabled) { e.preventDefault(); selectOption(opt); } }}
        onmouseenter={() => { if (!opt.disabled) highlightedIndex = index; }}
      >
        <span class="truncate" dir="auto">{opt.label}</span>
        {#if isSelected}
          <Check class="w-4 h-4 flex-shrink-0" style="color: var(--ds-interactive);" />
        {/if}
      </div>
    {/each}
    </div>
  </div>
{/if}
