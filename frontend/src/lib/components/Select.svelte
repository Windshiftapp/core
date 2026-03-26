<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { ChevronDown, Check } from 'lucide-svelte';
  import { tick } from 'svelte';
  import { fly } from 'svelte/transition';

  let {
    value = $bindable(''),
    options = [],
    disabled = false,
    required = false,
    size = 'medium',
    id = undefined,
    class: className = '',
    placeholder = '',
    onchange = undefined,
    onfocus = undefined,
    onblur = undefined
  } = $props();

  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    forceVisible: true,
    positioning: {
      strategy: 'fixed',
      placement: 'bottom-start',
      sameWidth: true,
      gutter: 4
    },
    portal: 'body'
  });

  let triggerElement = $state(null);
  let listboxElement = $state(null);
  let highlightedIndex = $state(-1);

  // Find the selected option label
  const selectedOption = $derived(options.find(o => String(o.value) === String(value)));
  const displayText = $derived(selectedOption?.label || placeholder || '');

  // Size variants
  const sizeClasses = $derived({
    small: 'px-3 py-1.5 text-sm',
    medium: 'px-3 py-2 text-sm'
  }[size] || 'px-3 py-2 text-sm');

  // Open dropdown and focus selected/first item
  function openDropdown() {
    if (disabled) return;
    open.set(true);
    tick().then(() => {
      const selectedIdx = options.findIndex(o => String(o.value) === String(value));
      highlightedIndex = selectedIdx >= 0 ? selectedIdx : 0;
      scrollToHighlighted();
    });
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
    tick().then(() => triggerElement?.focus());
  }

  function scrollToHighlighted() {
    tick().then(() => {
      if (!listboxElement) return;
      const items = listboxElement.querySelectorAll('[role="option"]');
      const item = items[highlightedIndex];
      if (item) item.scrollIntoView({ block: 'nearest' });
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
      triggerElement?.focus();
    }
  }

  function handleListboxKeydown(e) {
    const total = options.length;
    if (total === 0) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      highlightedIndex = (highlightedIndex + 1) % total;
      scrollToHighlighted();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      highlightedIndex = highlightedIndex <= 0 ? total - 1 : highlightedIndex - 1;
      scrollToHighlighted();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      if (highlightedIndex >= 0 && highlightedIndex < total && !options[highlightedIndex].disabled) {
        selectOption(options[highlightedIndex]);
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      closeDropdown();
      triggerElement?.focus();
    } else if (e.key === 'Tab') {
      closeDropdown();
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
        listboxElement?.focus();
      });
    } else if (!$open && wasOpen) {
      // Return focus to trigger
    }
    wasOpen = $open;
  });
</script>

<div class="relative {className}">
  <!-- Trigger button -->
  <button
    bind:this={triggerElement}
    use:melt={$trigger}
    type="button"
    {id}
    {disabled}
    aria-required={required || undefined}
    aria-haspopup="listbox"
    aria-expanded={$open}
    class="w-full rounded border transition-all duration-200 flex items-center justify-between gap-2
           focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50
           disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer text-left {sizeClasses}"
    style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
    onkeydown={handleTriggerKeydown}
    onfocus={onfocus}
    onblur={onblur}
  >
    <span class="truncate flex-1 {!selectedOption ? 'opacity-60' : ''}">
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
    bind:this={listboxElement}
    role="listbox"
    tabindex="-1"
    aria-activedescendant={highlightedIndex >= 0 ? `select-option-${highlightedIndex}` : undefined}
    onkeydown={handleListboxKeydown}
    class="rounded border shadow-lg max-h-60 overflow-y-auto z-[60] focus:outline-none"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);
           box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.25), 0 10px 10px -5px rgba(0, 0, 0, 0.15);"
    transition:fly={{ duration: 150, y: -5 }}
  >
    {#each options as opt, index (opt.value ?? `opt-${index}`)}
      {@const isSelected = String(opt.value) === String(value)}
      {@const isHighlighted = highlightedIndex === index}
      <div
        id="select-option-{index}"
        role="option"
        aria-selected={isSelected}
        class="px-3 py-2 text-sm flex items-center justify-between gap-2 transition-colors duration-100
               {opt.disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}"
        style="{isSelected
          ? 'background-color: var(--ds-background-selected); color: var(--ds-text);'
          : isHighlighted && !opt.disabled
            ? 'background-color: var(--ds-background-neutral-hovered); color: var(--ds-text);'
            : 'color: var(--ds-text);'}"
        onclick={() => selectOption(opt)}
        onmouseenter={() => { if (!opt.disabled) highlightedIndex = index; }}
      >
        <span class="truncate">{opt.label}</span>
        {#if isSelected}
          <Check class="w-4 h-4 flex-shrink-0" style="color: var(--ds-interactive);" />
        {/if}
      </div>
    {/each}
  </div>
{/if}
