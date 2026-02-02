<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { ChevronDown, Building2 } from 'lucide-svelte';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { onMount } from 'svelte';
  import Text from '../components/Text.svelte';
  import { t } from '../stores/i18n.svelte.js';

  // Generate unique IDs for ARIA attributes
  const listboxId = `listbox-${Math.random().toString(36).slice(2, 9)}`;
  const getOptionId = (index) => `${listboxId}-option-${index}`;

  // Props
  let {
    value = $bindable(null),
    placeholder = '',
    showUnassigned = false,
    unassignedLabel = '',
    disabled = false,
    class: className = '',
    children = null,
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || 'Select organisation');
  const resolvedUnassignedLabel = $derived(unassignedLabel || 'None');

  // Load customer organisations
  const organisations = createAsyncLoader(() => api.customerOrganisations.getAll());
  onMount(() => organisations.load());

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
        searchTerm = '';
        highlightedIndex = 0;
        setTimeout(() => inputElement?.focus(), 50);
      } else if (!wasSelectionMade) {
        onCancel();
      }
      wasSelectionMade = false;
      return next;
    }
  });

  let wasSelectionMade = false;

  // Selected organisation lookup
  let selectedOrganisation = $derived(
    (organisations.data || []).find(o => o.id === value) || null
  );

  // Filter and limit organisations
  let filteredOrganisations = $derived.by(() => {
    let result = organisations.data || [];
    if (searchTerm.trim()) {
      const term = searchTerm.toLowerCase();
      result = result.filter(o =>
        o.name?.toLowerCase().includes(term) ||
        o.email?.toLowerCase().includes(term) ||
        o.description?.toLowerCase().includes(term)
      );
    } else {
      // Show only 4 organisations by default (when no search)
      result = result.slice(0, 4);
    }
    return result;
  });

  // Handle selection
  function handleSelect(organisation) {
    wasSelectionMade = true;
    value = organisation?.id || null;
    $open = false;
    onSelect(organisation);
  }

  // Handle keyboard navigation
  function handleKeyDown(e) {
    const totalItems = (showUnassigned ? 1 : 0) + filteredOrganisations.length;

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
        if (adjustedIndex >= 0 && adjustedIndex < filteredOrganisations.length) {
          handleSelect(filteredOrganisations[adjustedIndex]);
        }
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      $open = false;
    }
  }
</script>

<!-- Trigger -->
{#if children}
  <button
    type="button"
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
  <button
    type="button"
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
  >
    <div class="flex items-center gap-2 flex-1 min-w-0">
      {#if selectedOrganisation}
        <div class="w-6 h-6 rounded-full bg-teal-100 flex items-center justify-center">
          <Building2 size={14} class="text-teal-600" />
        </div>
        <span class="truncate">{selectedOrganisation.name}</span>
      {:else}
        <span style="color: var(--ds-text-subtle);">{resolvedPlaceholder}</span>
      {/if}
    </div>
    <ChevronDown size={16} style="color: var(--ds-text-subtle);" />
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
      min-width: 280px;
      max-width: 360px;
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
        placeholder="Search organisations..."
        class="w-full px-3 py-2 rounded text-sm outline-none"
        style="
          background-color: var(--ds-background-input);
          border: 1px solid var(--ds-border);
          color: var(--ds-text);
        "
        aria-controls={listboxId}
        aria-activedescendant={getOptionId(highlightedIndex)}
        aria-autocomplete="list"
        onfocus={(e) => e.currentTarget.style.borderColor = 'var(--ds-border-focused)'}
        onblur={(e) => e.currentTarget.style.borderColor = 'var(--ds-border)'}
      />
    </div>

    <!-- Organisations List -->
    <div class="max-h-80 overflow-y-auto" role="listbox" id={listboxId} aria-label="Customer organisations">
      {#if organisations.loading}
        <div class="p-4 text-center" style="color: var(--ds-text-subtle);">
          {t('common.loading')}
        </div>
      {:else}
        <!-- Unassigned Option -->
        {#if showUnassigned}
          <button
            type="button"
            onclick={() => handleSelect(null)}
            class="w-full px-3 py-2.5 text-left text-sm transition-colors flex items-center gap-3"
            style="
              color: var(--ds-text);
              background-color: {highlightedIndex === 0 ? 'var(--ds-background-neutral-hovered)' : 'transparent'};
              border-bottom: 1px solid var(--ds-border);
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
            onfocus={(e) => {
              highlightedIndex = 0;
              e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)';
            }}
            onblur={(e) => {
              if (highlightedIndex !== 0) {
                e.currentTarget.style.backgroundColor = 'transparent';
              }
            }}
          >
            <span style="color: var(--ds-text-subtle);">{resolvedUnassignedLabel}</span>
          </button>
        {/if}

        <!-- Organisation Items -->
        {#each filteredOrganisations as organisation, index}
          {@const itemIndex = showUnassigned ? index + 1 : index}
          {@const isHighlighted = highlightedIndex === itemIndex}
          {@const isSelected = value === organisation.id}

          <button
            type="button"
            onclick={() => handleSelect(organisation)}
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
            onfocus={(e) => {
              highlightedIndex = itemIndex;
              if (!isSelected) {
                e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)';
              }
            }}
            onblur={(e) => {
              if (!isSelected && highlightedIndex !== itemIndex) {
                e.currentTarget.style.backgroundColor = 'transparent';
              }
            }}
          >
            <div class="flex items-center gap-3">
              <div class="w-8 h-8 rounded-full bg-teal-100 flex items-center justify-center flex-shrink-0">
                <Building2 size={16} class="text-teal-600" />
              </div>
              <div class="flex flex-col min-w-0 flex-1">
                <span class="font-medium truncate" style="color: var(--ds-text);">
                  {organisation.name}
                </span>
                {#if organisation.email}
                  <Text size="xs" variant="subtle" truncate>{organisation.email}</Text>
                {/if}
                {#if organisation.description}
                  <Text size="xs" variant="subtle" truncate>{organisation.description}</Text>
                {/if}
              </div>
            </div>
          </button>
        {/each}

        <!-- No Results -->
        {#if filteredOrganisations.length === 0 && !showUnassigned}
          <div class="p-4 text-center" style="color: var(--ds-text-subtle);">
            {searchTerm ? 'No organisations found' : 'No organisations available'}
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
