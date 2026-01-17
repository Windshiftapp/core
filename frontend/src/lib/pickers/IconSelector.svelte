<script>
  import { createEventDispatcher } from 'svelte';
  import { createPopover, melt } from '@melt-ui/svelte';
  import { Search, ChevronDown, Package } from 'lucide-svelte';
  import { workspaceIconMap, workspaceIconOptions } from '../utils/icons.js';
  import Label from '../components/Label.svelte';
  import { t } from '../stores/i18n.svelte.js';

  const dispatch = createEventDispatcher();

  // Use centralized icon map for workspace icons
  const iconMap = workspaceIconMap;
  const iconOptions = workspaceIconOptions;

  // Color options with good contrast and variety
  const colorOptions = [
    '#7c3aed', '#2563eb', '#059669', '#dc2626', '#ea580c',
    '#6b7280', '#8b5cf6', '#3b82f6', '#10b981', '#ef4444',
    '#f59e0b', '#84cc16', '#06b6d4', '#ec4899', '#f97316',
    '#64748b', '#7c2d12', '#1e40af', '#065f46', '#991b1b',
    '#92400e', '#365314', '#0e7490', '#be185d', '#9a3412'
  ];

  // Props
  export let selectedIcon = 'Package';
  export let selectedColor = '#3b82f6';
  export let label = '';
  export let compact = false;  // When true, shows compact trigger that opens popover

  $: resolvedLabel = label || t('pickers.iconAndColor');

  // Search functionality
  let searchQuery = '';
  $: filteredIcons = iconOptions.filter(icon =>
    icon.toLowerCase().includes(searchQuery.toLowerCase())
  );

  // Create popover for compact mode
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
        searchQuery = '';
      }
      return next;
    }
  });

  function selectIcon(icon) {
    selectedIcon = icon;
    dispatch('change', { icon: selectedIcon, color: selectedColor });
    if (compact) {
      $open = false;
    }
  }

  function selectColor(color) {
    selectedColor = color;
    dispatch('change', { icon: selectedIcon, color: selectedColor });
    if (compact) {
      $open = false;
    }
  }

  function handleColorInputChange(event) {
    selectedColor = event.target.value;
    dispatch('change', { icon: selectedIcon, color: selectedColor });
    // Don't close popover on custom color input - user might want to adjust
  }
</script>

{#if compact}
  <!-- Compact Mode -->
  <div class="icon-selector icon-selector-compact">
    {#if resolvedLabel}
      <Label class="mb-2">{resolvedLabel}</Label>
    {/if}

    <!-- Compact Trigger Button -->
    <button
      use:melt={$trigger}
      type="button"
      class="compact-trigger"
      style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
    >
      <div class="compact-preview">
        <div class="compact-icon" style="background-color: {selectedColor}">
          <svelte:component this={iconMap[selectedIcon] || Package} size={16} color="white" />
        </div>
        <div class="compact-text">
          <span class="font-medium text-sm" style="color: var(--ds-text);">{selectedIcon}</span>
          <span class="text-xs" style="color: var(--ds-text-subtle);">{selectedColor}</span>
        </div>
      </div>
      <ChevronDown size={16} style="color: var(--ds-text-subtle);" />
    </button>

    <!-- Popover Content -->
    {#if $open}
      <div
        use:melt={$content}
        class="popover-content"
        style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
      >
        <!-- Search Field -->
        <div class="popover-search" style="border-color: var(--ds-border);">
          <Search class="popover-search-icon" size={14} />
          <input
            type="text"
            placeholder={t('pickers.searchIcons')}
            bind:value={searchQuery}
            class="popover-search-input"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
          />
        </div>

        <!-- Scrollable Icon Grid -->
        <div class="popover-icons">
          <div class="popover-section-header" style="color: var(--ds-text-subtle);">
            {t('pickers.icons')} ({filteredIcons.length})
          </div>
          <div class="popover-icon-grid">
            {#each filteredIcons as icon}
              <button
                type="button"
                class="popover-icon-option"
                class:selected={selectedIcon === icon}
                onclick={() => selectIcon(icon)}
                title={icon}
              >
                <svelte:component this={iconMap[icon]} size={14} />
              </button>
            {/each}
          </div>
        </div>

        <!-- Color Selection -->
        <div class="popover-colors" style="border-color: var(--ds-border);">
          <div class="popover-section-header" style="color: var(--ds-text-subtle);">
            {t('pickers.colors')}
          </div>
          <div class="popover-color-grid">
            {#each colorOptions as color}
              <button
                type="button"
                class="popover-color-option"
                class:selected={selectedColor === color}
                style="background-color: {color}"
                onclick={() => selectColor(color)}
                title={color}
              ></button>
            {/each}
          </div>

          <!-- Custom Color Picker -->
          <div class="custom-color-row">
            <input
              type="color"
              bind:value={selectedColor}
              oninput={handleColorInputChange}
              class="custom-color-input"
              style="border-color: var(--ds-border);"
            />
            <span class="text-xs" style="color: var(--ds-text-subtle);">{t('pickers.custom')}</span>
          </div>
        </div>
      </div>
    {/if}
  </div>
{:else}
  <!-- Full Mode (existing layout) -->
  <div class="icon-selector">
    <Label class="mb-2">{label}</Label>

    <!-- Preview -->
    <div class="preview-section mb-4">
      <div class="preview-icon" style="background-color: {selectedColor}">
        <svelte:component this={iconMap[selectedIcon] || Package} size={20} color="white" />
      </div>
      <div class="preview-text">
        <div class="font-medium" style="color: var(--ds-text);">{selectedIcon}</div>
        <div class="text-xs" style="color: var(--ds-text-subtle);">{selectedColor}</div>
      </div>
    </div>

    <!-- Icon Selection -->
    <div class="selection-section">
      <div class="section-header">
        <h4 class="text-sm font-medium" style="color: var(--ds-text);">{t('pickers.icon')}</h4>
        <div class="search-box">
          <Search class="w-4 h-4 text-gray-400" />
          <input
            type="text"
            placeholder={t('pickers.searchIcons')}
            bind:value={searchQuery}
            class="search-input"
            style="background-color: var(--ds-background-input); border-color: var(--ds-border); color: var(--ds-text);"
          />
        </div>
      </div>

      <div class="icon-grid">
        {#each filteredIcons as icon}
          <button
            type="button"
            class="icon-option"
            class:selected={selectedIcon === icon}
            onclick={() => selectIcon(icon)}
            title={icon}
          >
            <svelte:component this={iconMap[icon]} size={16} />
          </button>
        {/each}
      </div>
    </div>

    <!-- Color Selection -->
    <div class="selection-section">
      <div class="section-header">
        <h4 class="text-sm font-medium" style="color: var(--ds-text);">{t('pickers.color')}</h4>
        <input
          type="color"
          bind:value={selectedColor}
          oninput={handleColorInputChange}
          class="color-input"
        />
      </div>

      <div class="color-grid">
        {#each colorOptions as color}
          <button
            type="button"
            class="color-option"
            class:selected={selectedColor === color}
            style="background-color: {color}"
            onclick={() => selectColor(color)}
            title={color}
          ></button>
        {/each}
      </div>
    </div>
  </div>
{/if}

<style>
  .icon-selector {
    width: 100%;
  }

  .preview-section {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px;
    border: 1px solid var(--ds-border);
    border-radius: 8px;
    background-color: var(--ds-surface-raised);
  }

  .preview-icon {
    width: 40px;
    height: 40px;
    border-radius: 8px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .preview-text {
    flex: 1;
  }

  .selection-section {
    margin-bottom: 20px;
  }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
  }

  .search-box {
    position: relative;
    display: flex;
    align-items: center;
    width: 200px;
  }

  .search-box :global(svg) {
    position: absolute;
    left: 8px;
    z-index: 1;
  }

  .search-input {
    width: 100%;
    padding: 6px 12px 6px 32px;
    font-size: 12px;
    border: 1px solid;
    border-radius: 6px;
    outline: none;
    transition: border-color 0.2s;
  }

  .search-input:focus {
    border-color: #3b82f6;
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.1);
  }

  .icon-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(40px, 1fr));
    gap: 8px;
    max-height: 200px;
    overflow-y: auto;
    padding: 4px;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    background-color: var(--ds-surface-raised);
  }

  .icon-option {
    width: 40px;
    height: 40px;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    background-color: var(--ds-surface);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: all 0.2s;
    color: var(--ds-text);
  }

  .icon-option:hover {
    border-color: #3b82f6;
    background-color: #eff6ff;
  }

  .icon-option.selected {
    border-color: #3b82f6;
    background-color: #3b82f6;
    color: white;
  }

  .color-input {
    width: 40px;
    height: 32px;
    border: 1px solid var(--ds-border);
    border-radius: 6px;
    cursor: pointer;
    padding: 0;
  }

  .color-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(32px, 1fr));
    gap: 8px;
    padding: 4px;
  }

  .color-option {
    width: 32px;
    height: 32px;
    border: 2px solid transparent;
    border-radius: 6px;
    cursor: pointer;
    transition: all 0.2s;
    position: relative;
  }

  .color-option:hover {
    transform: scale(1.1);
    border-color: rgba(255, 255, 255, 0.8);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  }

  .color-option.selected {
    border-color: #374151;
    box-shadow: 0 0 0 2px #3b82f6;
  }

  /* Compact mode styles */
  .icon-selector-compact {
    width: 100%;
  }

  .compact-trigger {
    display: flex;
    align-items: center;
    justify-content: space-between;
    width: 100%;
    padding: 8px 12px;
    border-radius: 8px;
    cursor: pointer;
    transition: all 0.2s;
  }

  .compact-trigger:hover {
    border-color: #3b82f6 !important;
  }

  .compact-preview {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .compact-icon {
    width: 32px;
    height: 32px;
    border-radius: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
  }

  .compact-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  /* Popover styles */
  .popover-content {
    z-index: 50;
    border-radius: 8px;
    box-shadow: 0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05);
    overflow: hidden;
    width: 320px;
  }

  .popover-search {
    padding: 8px;
    border-bottom: 1px solid;
    position: relative;
  }

  .popover-search-input {
    width: 100%;
    padding: 8px 12px 8px 32px;
    font-size: 13px;
    border: 1px solid;
    border-radius: 6px;
    outline: none;
    transition: border-color 0.2s;
  }

  .popover-search-input:focus {
    border-color: #3b82f6;
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.1);
  }

  .popover-search :global(.popover-search-icon) {
    position: absolute;
    left: 18px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--ds-text-subtle);
  }

  .popover-icons {
    padding: 8px;
    max-height: 180px;
    overflow-y: auto;
  }

  .popover-section-header {
    font-size: 11px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    margin-bottom: 8px;
  }

  .popover-icon-grid {
    display: grid;
    grid-template-columns: repeat(8, 1fr);
    gap: 4px;
  }

  .popover-icon-option {
    width: 32px;
    height: 32px;
    border: 1px solid var(--ds-border);
    border-radius: 4px;
    background-color: var(--ds-surface);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: all 0.15s;
    color: var(--ds-text);
  }

  .popover-icon-option:hover {
    border-color: #3b82f6;
    background-color: #eff6ff;
  }

  .popover-icon-option.selected {
    border-color: #3b82f6;
    background-color: #3b82f6;
    color: white;
  }

  .popover-colors {
    padding: 8px;
    border-top: 1px solid;
  }

  .popover-color-grid {
    display: grid;
    grid-template-columns: repeat(10, 1fr);
    gap: 4px;
    margin-bottom: 8px;
  }

  .popover-color-option {
    width: 24px;
    height: 24px;
    border: 2px solid transparent;
    border-radius: 4px;
    cursor: pointer;
    transition: all 0.15s;
  }

  .popover-color-option:hover {
    transform: scale(1.1);
    border-color: rgba(255, 255, 255, 0.8);
    box-shadow: 0 2px 4px rgba(0, 0, 0, 0.15);
  }

  .popover-color-option.selected {
    border-color: #374151;
    box-shadow: 0 0 0 2px #3b82f6;
  }

  .custom-color-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-top: 8px;
    border-top: 1px solid var(--ds-border);
  }

  .custom-color-input {
    width: 32px;
    height: 24px;
    border: 1px solid;
    border-radius: 4px;
    cursor: pointer;
    padding: 0;
  }
</style>