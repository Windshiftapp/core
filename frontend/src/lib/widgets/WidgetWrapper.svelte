<script>
  import { t } from '../stores/i18n.svelte.js';

  let { title = "", widgetId = "", isEditing = false, width = $bindable(3), onremove = null, children } = $props();

  function handleRemove(event) {
    event.stopPropagation();
    event.preventDefault();
    onremove?.();
  }

  // Get grid column span class
  const gridColClass = $derived(`col-span-${width}`);
</script>

<div
  class="widget-container rounded shadow-sm {gridColClass}"
  style="background-color: var(--ds-surface-raised); border: 1px solid var(--ds-border);"
  data-widget-id={widgetId}
  data-widget-wrapper
>
  <!-- Header with drag handle -->
  <div class="widget-header flex items-center justify-between px-4 py-3 border-b" style="border-color: var(--ds-border);">
    <div class="flex items-center gap-2">
      {#if isEditing}
        <button
          class="drag-handle cursor-grab hover:cursor-grabbing"
          style="color: var(--ds-text-subtlest);"
          data-drag-handle
          aria-label={t('aria.dragToReorder')}
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <line x1="4" y1="6" x2="20" y2="6"></line>
            <line x1="4" y1="12" x2="20" y2="12"></line>
            <line x1="4" y1="18" x2="20" y2="18"></line>
          </svg>
        </button>
      {/if}
      <h3 class="text-sm font-semibold" style="color: var(--ds-text);">{title}</h3>
    </div>

    {#if isEditing}
      <div class="flex items-center gap-1">
        <!-- Width controls -->
        <div class="flex items-center gap-1 mr-2 rounded" style="border: 1px solid var(--ds-border);">
          <button
            class="px-2 py-1 text-xs"
            style={width === 1 ? 'background-color: var(--ds-background-selected); color: var(--ds-text-selected);' : 'color: var(--ds-text-subtle);'}
            onclick={() => (width = 1)}
            title={t('widgets.narrowWidth')}
          >
            1
          </button>
          <button
            class="px-2 py-1 text-xs"
            style={width === 2 ? 'background-color: var(--ds-background-selected); color: var(--ds-text-selected);' : 'color: var(--ds-text-subtle);'}
            onclick={() => (width = 2)}
            title={t('widgets.mediumWidth')}
          >
            2
          </button>
          <button
            class="px-2 py-1 text-xs"
            style={width === 3 ? 'background-color: var(--ds-background-selected); color: var(--ds-text-selected);' : 'color: var(--ds-text-subtle);'}
            onclick={() => (width = 3)}
            title={t('widgets.fullWidth')}
          >
            3
          </button>
        </div>

        <!-- Remove button -->
        <button
          class="hover:text-red-600 p-1"
          style="color: var(--ds-text-subtlest);"
          onclick={handleRemove}
          title={t('widgets.removeWidget')}
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <line x1="18" y1="6" x2="6" y2="18"></line>
            <line x1="6" y1="6" x2="18" y2="18"></line>
          </svg>
        </button>
      </div>
    {/if}
  </div>

  <!-- Widget content -->
  <div class="widget-content p-4">
    {@render children?.()}
  </div>
</div>

<style>
  .col-span-1 {
    grid-column: span 1 / span 1;
  }

  .col-span-2 {
    grid-column: span 2 / span 2;
  }

  .col-span-3 {
    grid-column: span 3 / span 3;
  }

  @media (max-width: 1024px) {
    /* On tablet, 2-column layout */
    .col-span-3 {
      grid-column: span 2 / span 2;
    }
  }

  @media (max-width: 768px) {
    /* On mobile, single column */
    .col-span-1,
    .col-span-2,
    .col-span-3 {
      grid-column: span 1 / span 1;
    }
  }

  .drag-handle:active {
    cursor: grabbing;
  }

  .widget-container {
    transition: box-shadow 0.2s;
  }

  .widget-container:hover {
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
  }
</style>
