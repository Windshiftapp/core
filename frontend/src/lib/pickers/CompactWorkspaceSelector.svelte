<script>
  import { createPopover, melt } from '@melt-ui/svelte';
  import { ChevronDown, Building } from 'lucide-svelte';

  // Generate unique IDs for ARIA attributes
  const listboxId = `listbox-${Math.random().toString(36).slice(2, 9)}`;
  const getOptionId = (index) => `${listboxId}-option-${index}`;

  let {
    value = $bindable(null),
    workspaces = [],
    placeholder = 'Workspace',
    disabled = false,
    loading = false,
    onSelect = () => {}
  } = $props();

  const {
    elements: { trigger, content },
    states: { open }
  } = createPopover({
    positioning: {
      placement: 'bottom-start',
      gutter: 4,
      flip: true,
      shift: true
    },
    portal: 'body',
    forceVisible: true
  });

  let searchTerm = $state('');
  let highlightedIndex = $state(0);
  let inputElement = $state(null);

  let selectedWorkspace = $derived(
    workspaces.find(w => w.id === value) || null
  );

  let filteredWorkspaces = $derived.by(() => {
    if (!searchTerm.trim()) {
      return workspaces;
    }
    const term = searchTerm.toLowerCase();
    return workspaces.filter(w =>
      w.name?.toLowerCase().includes(term) ||
      w.key?.toLowerCase().includes(term)
    );
  });

  // Focus input when popover opens
  $effect(() => {
    if ($open) {
      searchTerm = '';
      highlightedIndex = 0;
      setTimeout(() => inputElement?.focus(), 50);
    }
  });

  function handleSelect(workspace) {
    value = workspace?.id;
    $open = false;
    onSelect(workspace);
  }

  function handleKeyDown(e) {
    const total = filteredWorkspaces.length;

    // Tab closes the dropdown without preventing default (allows focus to move)
    if (e.key === 'Tab') {
      $open = false;
      return;
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      highlightedIndex = (highlightedIndex + 1) % total;
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      highlightedIndex = highlightedIndex === 0 ? total - 1 : highlightedIndex - 1;
    } else if (e.key === 'Enter' || (e.key === ' ' && e.target.tagName !== 'INPUT')) {
      e.preventDefault();
      if (highlightedIndex >= 0 && highlightedIndex < total) {
        handleSelect(filteredWorkspaces[highlightedIndex]);
      }
    } else if (e.key === 'Escape') {
      e.preventDefault();
      $open = false;
    }
  }
</script>

<!-- Pill Trigger Button -->
<button
  use:melt={$trigger}
  {disabled}
  class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-sm font-medium transition-colors"
  style="
    background-color: var(--ds-surface);
    border: 1px solid var(--ds-border);
    color: var(--ds-text);
    opacity: {disabled ? 0.5 : 1};
    cursor: {disabled ? 'not-allowed' : 'pointer'};
  "
  role="combobox"
  aria-expanded={$open}
  aria-controls={listboxId}
  aria-haspopup="listbox"
  onmouseover={(e) => {
    if (!disabled) {
      e.currentTarget.style.backgroundColor = 'var(--ds-surface-hovered, var(--ds-background-neutral-hovered))';
    }
  }}
  onmouseout={(e) => {
    e.currentTarget.style.backgroundColor = 'var(--ds-surface)';
  }}
>
  {#if selectedWorkspace}
    {#if selectedWorkspace.avatar_url}
      <img
        src={selectedWorkspace.avatar_url}
        alt={selectedWorkspace.name}
        class="w-4 h-4 rounded flex-shrink-0"
      />
    {:else}
      <div
        class="w-4 h-4 rounded flex items-center justify-center flex-shrink-0"
        style="background-color: {selectedWorkspace.color || '#6366f1'};"
      >
        <Building size={10} style="color: #fff;" />
      </div>
    {/if}
    <span class="truncate max-w-[100px]">{selectedWorkspace.key || selectedWorkspace.name}</span>
  {:else}
    <Building size={14} style="color: var(--ds-text-subtle);" />
    <span style="color: var(--ds-text-subtle);">{placeholder}</span>
  {/if}
  <ChevronDown size={12} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
</button>

<!-- Popover Content -->
{#if $open}
  <div
    use:melt={$content}
    class="z-50 rounded-lg shadow-lg overflow-hidden"
    style="
      background-color: var(--ds-surface-raised);
      border: 1px solid var(--ds-border);
      min-width: 260px;
      max-width: 320px;
    "
  >
    <!-- Search Input -->
    <div class="p-2 border-b" style="border-color: var(--ds-border);">
      <input
        bind:this={inputElement}
        bind:value={searchTerm}
        onkeydown={handleKeyDown}
        type="text"
        placeholder="Search workspaces..."
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

    <!-- Workspaces List -->
    <div class="max-h-64 overflow-y-auto" role="listbox" id={listboxId} aria-label="Workspaces">
      {#if loading}
        <div class="p-4 text-center text-sm" style="color: var(--ds-text-subtle);">
          Loading...
        </div>
      {:else if filteredWorkspaces.length === 0}
        <div class="p-4 text-center text-sm" style="color: var(--ds-text-subtle);">
          No workspaces found
        </div>
      {:else}
        {#each filteredWorkspaces as workspace, index}
          <button
            type="button"
            class="w-full flex items-center gap-3 px-3 py-2.5 text-left transition-colors"
            style="
              background-color: {index === highlightedIndex ? 'var(--ds-background-selected)' : 'transparent'};
              color: var(--ds-text);
            "
            role="option"
            id={getOptionId(index)}
            aria-selected={workspace.id === value}
            onmouseover={(e) => {
              highlightedIndex = index;
              e.currentTarget.style.backgroundColor = 'var(--ds-background-selected)';
            }}
            onmouseout={(e) => {
              if (index !== highlightedIndex) {
                e.currentTarget.style.backgroundColor = 'transparent';
              }
            }}
            onclick={() => handleSelect(workspace)}
          >
            {#if workspace.avatar_url}
              <img
                src={workspace.avatar_url}
                alt={workspace.name}
                class="w-6 h-6 rounded flex-shrink-0"
              />
            {:else}
              <div
                class="w-6 h-6 rounded flex items-center justify-center flex-shrink-0"
                style="background-color: {workspace.color || '#6366f1'};"
              >
                <Building size={12} style="color: #fff;" />
              </div>
            {/if}
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <span class="font-medium truncate">{workspace.name}</span>
                {#if workspace.key}
                  <span
                    class="text-xs px-1.5 py-0.5 rounded flex-shrink-0"
                    style="background-color: var(--ds-background-neutral); color: var(--ds-text-subtle);"
                  >
                    {workspace.key}
                  </span>
                {/if}
              </div>
            </div>
            {#if workspace.id === value}
              <div class="w-4 h-4 rounded-full flex-shrink-0" style="background-color: var(--ds-interactive);">
                <svg class="w-4 h-4 text-white" fill="currentColor" viewBox="0 0 20 20">
                  <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                </svg>
              </div>
            {/if}
          </button>
        {/each}
      {/if}
    </div>
  </div>
{/if}
