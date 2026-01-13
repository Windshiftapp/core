<script>
  import { BasePicker } from '.';
  import { createEventDispatcher, onMount } from 'svelte';
  import { createPopover, melt } from '@melt-ui/svelte';
  import { api } from '../api.js';

  const dispatch = createEventDispatcher();

  // Generate unique IDs for ARIA attributes
  const listboxId = `listbox-${Math.random().toString(36).slice(2, 9)}`;
  const getOptionId = (index) => `${listboxId}-option-${index}`;

  let {
    value = $bindable(null),
    placeholder = "Select milestone...",
    class: className = '',
    disabled = false,
    workspaceId = null,
    showUnassigned = true,
    unassignedLabel = 'No milestone',
    children = null  // Custom trigger snippet
  } = $props();

  let milestones = $state([]);
  let loading = $state(false);
  let error = $state(null);
  let searchTerm = $state('');
  let highlightedIndex = $state(0);
  let inputElement = $state(null);

  // Create popover for custom trigger mode
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
      }
      return next;
    }
  });

  // Filtered milestones for search
  let filteredMilestones = $derived.by(() => {
    if (!searchTerm.trim()) return milestones;
    const term = searchTerm.toLowerCase();
    return milestones.filter(m =>
      m.name?.toLowerCase().includes(term) ||
      m.description?.toLowerCase().includes(term)
    );
  });

  // Selected milestone
  let selectedMilestone = $derived(
    milestones.find(m => m.id === value) || null
  );

  // Load milestones on mount
  onMount(async () => {
    await loadMilestones();
  });

  // Reload when workspaceId changes
  $effect(() => {
    if (workspaceId !== undefined) {
      loadMilestones();
    }
  });

  async function loadMilestones() {
    loading = true;
    error = null;

    try {
      // Build filters - if workspaceId is provided, filter by workspace + global
      const filters = {};
      if (workspaceId) {
        filters.workspace_id = workspaceId;
        filters.include_global = true;
      }

      const response = await api.milestones.getAll(filters);
      milestones = response || [];
    } catch (err) {
      console.error('Failed to load milestones:', err);
      error = err.message || 'Failed to load milestones';
      milestones = [];
    } finally {
      loading = false;
    }
  }

  function handleSelect(event) {
    const milestone = event.detail;
    dispatch('select', {
      value: milestone ? milestone.id : null,
      milestone: milestone
    });
  }

  function handleCancel() {
    dispatch('cancel');
  }

  // Handle selection in custom trigger mode
  function handlePopoverSelect(milestone) {
    value = milestone?.id || null;
    $open = false;
    dispatch('select', {
      value: milestone?.id || null,
      milestone: milestone || null
    });
  }

  // Handle keyboard navigation in popover
  function handleKeyDown(e) {
    const totalItems = (showUnassigned ? 1 : 0) + filteredMilestones.length;
    if (totalItems === 0) return;

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
      highlightedIndex = (highlightedIndex - 1 + totalItems) % totalItems;
    } else if (e.key === 'Enter' || (e.key === ' ' && e.target.tagName !== 'INPUT')) {
      e.preventDefault();
      if (showUnassigned && highlightedIndex === 0) {
        handlePopoverSelect(null);
      } else {
        const index = showUnassigned ? highlightedIndex - 1 : highlightedIndex;
        if (index >= 0 && index < filteredMilestones.length) {
          handlePopoverSelect(filteredMilestones[index]);
        }
      }
    } else if (e.key === 'Escape') {
      $open = false;
    }
  }
</script>

{#if children}
  <!-- Custom trigger mode with popover -->
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

  {#if $open}
    <div
      use:melt={$content}
      class="z-[60] rounded-lg shadow-lg overflow-hidden"
      style="
        background-color: var(--ds-surface-raised);
        border: 1px solid var(--ds-border);
        min-width: 240px;
        max-width: 320px;
      "
    >
      <!-- Search input -->
      <div class="p-2 border-b" style="border-color: var(--ds-border);">
        <input
          bind:this={inputElement}
          type="text"
          bind:value={searchTerm}
          onkeydown={handleKeyDown}
          placeholder="Search..."
          class="w-full px-3 py-2 text-sm rounded border"
          style="
            background-color: var(--ds-background-input);
            border-color: var(--ds-border);
            color: var(--ds-text);
          "
          aria-controls={listboxId}
          aria-activedescendant={getOptionId(highlightedIndex)}
          aria-autocomplete="list"
        />
      </div>

      <!-- Options list -->
      <div class="max-h-60 overflow-y-auto" role="listbox" id={listboxId} aria-label="Milestones">
        {#if showUnassigned}
          <button
            type="button"
            class="w-full px-3 py-2 text-left text-sm transition-colors"
            style="
              background-color: {highlightedIndex === 0 ? 'var(--ds-background-selected)' : 'transparent'};
              color: var(--ds-text-subtle);
            "
            role="option"
            id={getOptionId(0)}
            aria-selected={value === null}
            onmouseenter={() => highlightedIndex = 0}
            onclick={() => handlePopoverSelect(null)}
          >
            {unassignedLabel}
          </button>
        {/if}

        {#each filteredMilestones as milestone, i}
          {@const index = showUnassigned ? i + 1 : i}
          <button
            type="button"
            class="w-full px-3 py-2 text-left text-sm transition-colors"
            style="
              background-color: {highlightedIndex === index ? 'var(--ds-background-selected)' : 'transparent'};
              color: var(--ds-text);
            "
            role="option"
            id={getOptionId(index)}
            aria-selected={value === milestone.id}
            onmouseenter={() => highlightedIndex = index}
            onclick={() => handlePopoverSelect(milestone)}
          >
            <div class="flex items-center gap-3">
              <div
                class="w-2 h-2 rounded-full flex-shrink-0"
                style="background-color: {milestone.category_color || '#9CA3AF'};"
              ></div>
              <span class="truncate">{milestone.name}</span>
            </div>
          </button>
        {/each}

        {#if filteredMilestones.length === 0 && !showUnassigned}
          <div class="px-3 py-2 text-sm" style="color: var(--ds-text-subtle);">
            No milestones found
          </div>
        {/if}
      </div>
    </div>
  {/if}
{:else}
  <!-- Default BasePicker mode -->
  <BasePicker
    bind:value
    items={milestones}
    {loading}
    {error}
    {placeholder}
    {disabled}
    class={className}
    allowClear={true}
    searchFields={['name', 'description']}
    getValue={(milestone) => milestone?.id}
    getLabel={(milestone) => milestone?.name ?? ''}
    on:select={handleSelect}
    on:cancel={handleCancel}
  >
    {#snippet itemSnippet({ item: milestone, isSelected })}
      <div class="flex items-center gap-3 flex-1 min-w-0">
        <!-- Category color indicator -->
        <div class="flex-shrink-0">
          <div
            class="w-2 h-2 rounded-full"
            style="background-color: {milestone.category_color || '#9CA3AF'};"
          ></div>
        </div>

        <!-- Milestone name -->
        <span class="font-medium truncate">{milestone.name}</span>
      </div>
    {/snippet}
  </BasePicker>
{/if}
