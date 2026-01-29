<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { ChevronDown } from 'lucide-svelte';
  import { getTextColorForBackground } from '../utils/statusColors.js';
  import { t } from '../stores/i18n.svelte.js';
  import { tick } from 'svelte';

  export let triggerText = '';
  export let triggerIcon = null;
  export let triggerAvatar = null; // URL for avatar image
  export let triggerIconBgColor = null; // Background color for the icon (hex color)
  export let triggerBgColor = null; // Background color for entire button (hex color)
  export let triggerIconClass = 'w-4 h-4'; // Icon size class
  export let triggerGap = 'gap-2'; // Gap between icon and text
  export let items = [];
  export let maxWidth = 'max-w-3xl';
  export let placement = 'bottom'; // 'bottom', 'right', 'left', 'top'
  export let triggerClass = '';
  export let triggerStyle = '';
  export let showChevron = true;
  export let iconOnly = false; // New prop for icon-only buttons
  export let onOpen = null; // Callback when dropdown opens
  export let triggerAlignment = 'center'; // 'center', 'start', 'between'

  // Create popover (replaces createDropdownMenu to avoid typeahead focus-stealing)
  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    forceVisible: true,
    positioning: {
      placement: placement || 'bottom'
    },
    portal: 'body'
  });

  // Watch for open state changes
  let previousOpen = false;
  let triggerElement;
  let searchInputElement;

  $: hasSearchInput = items.some(i => i.type === 'search');

  $: {
    if ($open && !previousOpen) {
      // Dropdown just opened
      if (onOpen) onOpen();
      tick().then(() => {
        if (searchInputElement) {
          searchInputElement.focus();
        } else {
          // Focus first menu item for non-search dropdowns
          const container = document.querySelector('[data-menu-container]');
          if (container) {
            const firstItem = container.querySelector('button[data-menu-item]');
            if (firstItem) firstItem.focus();
          }
        }
      });
    } else if (previousOpen && !$open) {
      // Dropdown just closed, blur the trigger element
      if (triggerElement) {
        triggerElement.blur();
      }
    }
    previousOpen = $open;
  }

  $: alignmentClass = triggerAlignment === 'between'
    ? 'justify-between'
    : triggerAlignment === 'start'
      ? 'justify-start'
      : 'justify-center';

  function closeMenu() {
    if ($open) {
      open.set(false);
    }
  }

  function handleMenuKeydown(e) {
    const focusableItems = [...e.currentTarget.querySelectorAll('button[data-menu-item]')];
    const currentIndex = focusableItems.indexOf(document.activeElement);

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      const next = currentIndex + 1;
      if (next < focusableItems.length) {
        focusableItems[next].focus();
      }
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (currentIndex <= 0 && searchInputElement) {
        searchInputElement.focus();
      } else if (currentIndex > 0) {
        focusableItems[currentIndex - 1].focus();
      }
    } else if (e.key === 'Home') {
      e.preventDefault();
      if (focusableItems.length > 0) {
        focusableItems[0].focus();
      }
    } else if (e.key === 'End') {
      e.preventDefault();
      if (focusableItems.length > 0) {
        focusableItems[focusableItems.length - 1].focus();
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      open.set(false);
    }
  }

  function handleItemClick(itemData, event) {
    // Stop event from bubbling to prevent it from reaching modal overlays
    if (event) {
      event.stopPropagation();
    }

    if (itemData.type === 'checkbox' && itemData.onChange) {
      itemData.onChange(!itemData.checked);
    } else if (itemData.onClick) {
      itemData.onClick();
    }

    // Close menu after selection unless explicitly told not to
    if (itemData.closeOnSelect !== false) {
      closeMenu();
    }
  }
</script>

<!-- Trigger Button -->
<div class="relative">
  <button
    bind:this={triggerElement}
    use:melt={$trigger}
    class="{triggerAvatar ? 'p-0' : iconOnly ? '' : triggerClass ? '' : 'px-4 py-2'} rounded text-sm font-medium transition cursor-pointer flex items-center {alignmentClass} {triggerGap} flex-shrink-0 {triggerBgColor ? getTextColorForBackground(triggerBgColor) : ''} {triggerClass}"
    style="{triggerBgColor ? `background-color: ${triggerBgColor}; ${triggerStyle}` : triggerStyle}"
  >
    {#if $$slots.default}
      <!-- Custom trigger content via slot -->
      <slot />
      {#if showChevron}
        <ChevronDown class="w-3 h-3" />
      {/if}
    {:else if triggerAvatar}
      <img src={triggerAvatar} alt={t('common.profile')} class="w-8 h-8 rounded-full object-cover flex-shrink-0" />
      {#if triggerText}
        <span class="text-sm whitespace-nowrap">{triggerText}</span>
      {/if}
      {#if showChevron}
        <ChevronDown class="w-3 h-3" />
      {/if}
    {:else}
      {#if triggerIcon}
        {#if triggerBgColor}
          <!-- When full background is colored, show icon without circle -->
          <svelte:component this={triggerIcon} class="{triggerIconClass} flex-shrink-0" />
        {:else if triggerIconBgColor}
          <div
            class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
            style="background-color: {triggerIconBgColor};"
          >
            <svelte:component this={triggerIcon} class="w-3.5 h-3.5 text-white" />
          </div>
        {:else}
          <svelte:component this={triggerIcon} class="{triggerIconClass} flex-shrink-0" />
        {/if}
      {/if}
      {triggerText}
      {#if showChevron}
        <ChevronDown class="w-3 h-3" />
      {/if}
    {/if}
  </button>
</div>

<!-- Dropdown Menu -->
{#if $open}
  <div
    use:melt={$content}
    data-menu-container
    role="menu"
    onkeydown={handleMenuKeydown}
    class="{maxWidth} rounded shadow-xl border focus:outline-none z-[60]"
    style="background-color: var(--ds-surface-raised); border-color: var(--ds-border); box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.25), 0 10px 10px -5px rgba(0, 0, 0, 0.15);"
  >
    <div>
      {#each items as itemData, index (itemData.id || index)}
        {#if itemData.type === 'divider'}
          <div class="border-t mx-2" style="border-color: var(--ds-border);"></div>
        {:else if itemData.type === 'text'}
          <div class="px-4 py-3 text-sm text-center italic" style="color: var(--ds-text-subtle);">{itemData.text}</div>
        {:else if itemData.type === 'search'}
          <div class="px-3 py-2 border-b" style="border-color: var(--ds-border);">
            <input
              bind:this={searchInputElement}
              type="text"
              placeholder={itemData.placeholder || t('common.search')}
              value={itemData.value || ''}
              oninput={(e) => itemData.onInput && itemData.onInput(e.target.value)}
              onkeydown={(e) => {
                // Allow arrow down and tab to navigate to menu items
                if (e.key === 'ArrowDown' || (e.key === 'Tab' && !e.shiftKey)) {
                  e.preventDefault();
                  const container = e.target.closest('[data-menu-container]');
                  if (container) {
                    const firstItem = container.querySelector('button[data-menu-item]');
                    if (firstItem) firstItem.focus();
                  }
                  return;
                }
                // Stop propagation for other keys to prevent closing dropdown while typing
                if (e.key !== 'Escape') {
                  e.stopPropagation();
                }
              }}
              class="w-full px-3 py-2 text-sm rounded-md focus:outline-none focus:ring-2 focus:ring-[var(--ds-border-focused)] focus:border-transparent"
              style="background-color: var(--ds-background-input); border: 1px solid var(--ds-border); color: var(--ds-text);"
              onclick={(e) => e.stopPropagation()}
            />
          </div>
        {:else if itemData.type === 'group'}
          {#each itemData.items as groupItem (groupItem.id)}
            <button
              data-menu-item
              role="menuitem"
              onclick={(e) => handleItemClick(groupItem, e)}
              class="flex items-center w-full px-4 py-3 text-sm transition-all duration-200 cursor-pointer {groupItem.class || 'group'}"
              style="color: {groupItem.color || 'var(--ds-text)'};"
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-surface-pressed)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
            >
              {#if groupItem.type === 'checkbox'}
                <div class="w-4 h-4 mr-3 flex items-center justify-center">
                  <input
                    type="checkbox"
                    checked={groupItem.checked || false}
                    class="w-4 h-4 text-blue-600 rounded focus:ring-blue-500"
                    style="border-color: var(--ds-border);"
                    onclick={(e) => e.stopPropagation()}
                  />
                </div>
              {:else if groupItem.avatarUrl}
                <img src={groupItem.avatarUrl} alt="Avatar" class="w-6 h-6 mr-3 rounded object-cover" />
              {:else if groupItem.icon}
                {#if groupItem.iconColor}
                  <div class="w-6 h-6 mr-3 rounded flex items-center justify-center" style="background-color: {groupItem.iconColor};">
                    <svelte:component this={groupItem.icon} class="w-4 h-4" style="color: white;" />
                  </div>
                {:else}
                  <svelte:component this={groupItem.icon} class="w-4 h-4 mr-3 {groupItem.iconClass || 'transition-colors'}" style="color: var(--ds-icon-subtle);" />
                {/if}
              {/if}

              {#if groupItem.content}
                <!-- Custom content slot -->
                <div class="flex-1 text-left">
                  {@html groupItem.content}
                </div>
              {:else}
                <!-- Simple text content -->
                <div class="flex-1 text-left">
                  <div class="font-medium">{groupItem.title}</div>
                  {#if groupItem.subtitle}
                    <div class="text-xs line-clamp-1" style="color: var(--ds-text-subtle);">{groupItem.subtitle}</div>
                  {/if}
                </div>
              {/if}

              {#if groupItem.badge}
                <span class="text-xs px-2 py-1 rounded-full" style="color: var(--ds-text-subtlest); background-color: var(--ds-background-neutral);">{groupItem.badge}</span>
              {/if}
            </button>
          {/each}
        {:else}
          <!-- Regular item -->
          <button
            data-menu-item
            role="menuitem"
            onclick={(e) => handleItemClick(itemData, e)}
            class="flex items-center w-full px-4 py-3 text-sm transition-all duration-200 cursor-pointer {itemData.class || ''}"
            style="color: {itemData.color || 'var(--ds-text)'}; {itemData.style || ''}"
            onmouseenter={(e) => { if (!itemData.style) e.currentTarget.style.backgroundColor = 'var(--ds-surface-pressed)'; }}
            onmouseleave={(e) => { if (!itemData.style) e.currentTarget.style.backgroundColor = ''; }}
          >
            {#if itemData.type === 'checkbox'}
              <div class="w-4 h-4 mr-3 flex items-center justify-center">
                <input
                  type="checkbox"
                  checked={itemData.checked || false}
                  class="w-4 h-4 text-blue-600 rounded focus:ring-blue-500 pointer-events-none"
                  style="border-color: var(--ds-border);"
                />
              </div>
            {:else if itemData.icon}
              {#if itemData.iconColor}
                <div class="w-6 h-6 mr-3 rounded flex items-center justify-center" style="background-color: {itemData.iconColor};">
                  <svelte:component this={itemData.icon} class="w-4 h-4" style="color: white;" />
                </div>
              {:else}
                <svelte:component this={itemData.icon} class="w-4 h-4 mr-3 {itemData.iconClass || ''}" />
              {/if}
            {/if}

            <div class="flex-1 text-left">
              {#if itemData.content}
                <!-- Custom content slot -->
                {@html itemData.content}
              {:else}
                <!-- Simple text content -->
                <div class="font-medium">{itemData.title}</div>
                {#if itemData.subtitle}
                  <div class="text-xs line-clamp-1" style="color: var(--ds-text-subtle);">{itemData.subtitle}</div>
                {/if}
              {/if}
            </div>

            {#if itemData.badge}
              <span class="ml-auto text-xs {itemData.badgeClass || ''}" style="{itemData.badgeStyle || (itemData.badgeClass ? '' : 'color: var(--ds-text-subtlest);')}">{itemData.badge}</span>
            {/if}
          </button>
        {/if}
      {/each}
    </div>
  </div>
{/if}
