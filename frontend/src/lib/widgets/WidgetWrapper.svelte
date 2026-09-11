<script>
  import { t } from '../stores/i18n.svelte.js';
  import { getDashboardWidgetMinWidth, getDashboardWidgetDefaultWidth } from '../services/dashboardWidgetRegistry.js';
  import {
    ROW_COUNT_OPTIONS,
    resolveRowCount,
    resolveDensity,
    shouldShowRowControls,
  } from './dashboard/taskWidgetState.js';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import { useEventListener } from 'runed';
  import { ChevronDown } from '@lucide/svelte';

  let {
    title = '',
    widgetId = '',
    widgetType = '',
    isEditing = false,
    width = $bindable(getDashboardWidgetDefaultWidth(widgetType)),
    config = $bindable({}),
    gridColumns = 12,
    resizeMinWidth = null,
    resizeMaxWidth = null,
    resizeDefaultWidth = null,
    onremove = null,
    onwidthchange = null,
    onconfigchange = null,
    children,
  } = $props();

  const totalColumns = $derived(
    Number.isFinite(Number(gridColumns))
      ? Math.max(1, Math.round(Number(gridColumns)))
      : 12
  );
  const registryMinWidth = $derived(getDashboardWidgetMinWidth(widgetType) || 3);
  const minWidth = $derived(
    Math.min(totalColumns, Math.max(1, resizeMinWidth ?? registryMinWidth))
  );
  const maxWidth = $derived(
    Math.max(minWidth, Math.min(totalColumns, resizeMaxWidth ?? totalColumns))
  );
  const defaultWidth = $derived(
    Math.min(
      maxWidth,
      Math.max(
        minWidth,
        resizeDefaultWidth ?? getDashboardWidgetDefaultWidth(widgetType) ?? totalColumns
      )
    )
  );

  function handleRemove(event) {
    event.stopPropagation();
    event.preventDefault();
    onremove?.();
  }

  function setWidth(newWidth) {
    const clamped = Math.min(maxWidth, Math.max(minWidth, newWidth));
    const resolved = onwidthchange?.(clamped);
    width = Number.isFinite(resolved) ? resolved : clamped;
  }

  // --- Resize presets (WI-831) ---
  const presets = $derived([
    { label: t('widgets.widthQuarter'), value: totalColumns / 4 },
    { label: t('widgets.widthThird'), value: totalColumns / 3 },
    { label: t('widgets.widthHalf'), value: totalColumns / 2 },
    { label: t('widgets.widthTwoThirds'), value: (totalColumns * 2) / 3 },
    { label: t('widgets.widthFull'), value: totalColumns },
  ].filter((p) =>
    Number.isInteger(p.value) && p.value >= minWidth && p.value <= maxWidth
  ));

  const presetItems = $derived(presets.map((p) => ({
    title: p.label,
    testid: `widget-width-preset-${p.value}`,
    onClick: () => setWidth(p.value),
    selected: width === p.value,
  })));

  const supportsRowCount = $derived(shouldShowRowControls(widgetType, config));
  const currentRowCount = $derived(resolveRowCount(config, width));
  const currentDensity = $derived(resolveDensity(config));

  function setRowCount(value) {
    config = { ...config, rowCount: value };
    onconfigchange?.({ rowCount: value });
  }

  function setDensity(value) {
    config = { ...config, density: value };
    onconfigchange?.({ density: value });
  }

  const rowCountItems = $derived(ROW_COUNT_OPTIONS.map((n) => ({
    title: n === 'all' ? t('widgets.rowCountAll') : t(`widgets.rowCount${n}`),
    testid: `widget-row-count-${n}`,
    onClick: () => setRowCount(n),
    selected: currentRowCount === n,
  })));

  const densityItems = $derived([
    {
      title: t('widgets.densityComfortable'),
      testid: 'widget-density-comfortable',
      onClick: () => setDensity('comfortable'),
      selected: currentDensity === 'comfortable',
    },
    {
      title: t('widgets.densityCompact'),
      testid: 'widget-density-compact',
      onClick: () => setDensity('compact'),
      selected: currentDensity === 'compact',
    },
  ]);

  const menuItems = $derived([
    ...presetItems,
    ...(supportsRowCount ? [{ type: 'divider' }] : []),
    ...(supportsRowCount ? rowCountItems : []),
    ...(supportsRowCount ? [{ type: 'divider' }] : []),
    ...(supportsRowCount ? densityItems : []),
  ]);

  // --- Pointer drag resize (WI-831), follows WorkspaceNavigation.svelte ---
  let isResizing = $state(false);
  let resizeStartX = $state(0);
  let resizeStartWidth = $state(0);
  let containerEl = $state(null);
  let liveColumns = $state(null);

  function onResizeStart(e) {
    e.preventDefault();
    e.stopPropagation();
    resizeStartX = e.clientX;
    resizeStartWidth = width;
    isResizing = true;
  }

  function handleResizeMove(e) {
    if (!containerEl) return;
    const grid = containerEl.parentElement;
    if (!grid) return;
    const colWidth = grid.getBoundingClientRect().width / totalColumns;
    if (colWidth <= 0) return;
    const deltaCols = Math.round((e.clientX - resizeStartX) / colWidth);
    const next = Math.min(maxWidth, Math.max(minWidth, resizeStartWidth + deltaCols));
    liveColumns = next;
    if (next !== width) {
      const resolved = onwidthchange?.(next);
      width = Number.isFinite(resolved) ? resolved : next;
    }
  }

  function handleResizeEnd() {
    isResizing = false;
    liveColumns = null;
  }

  function onResizeHandleDblClick() {
    setWidth(defaultWidth);
  }

  useEventListener(() => (isResizing ? window : undefined), 'mousemove', handleResizeMove);
  useEventListener(() => (isResizing ? window : undefined), 'mouseup', handleResizeEnd);

  // --- Keyboard slider (WI-831) ---
  function handleSliderKeydown(e) {
    let next = width;
    switch (e.key) {
      case 'ArrowLeft':
      case 'ArrowDown':
        next = width - 1;
        break;
      case 'ArrowRight':
      case 'ArrowUp':
        next = width + 1;
        break;
      case 'Home':
        next = minWidth;
        break;
      case 'End':
        next = maxWidth;
        break;
      default:
        return;
    }
    e.preventDefault();
    setWidth(next);
  }

  const displayColumns = $derived(liveColumns ?? width);
</script>

<div
  bind:this={containerEl}
  class="widget-container rounded shadow-sm border bg-ds-surface-raised border-ds-border"
  style="--widget-cols: {displayColumns};"
  data-widget-id={widgetId}
  data-widget-wrapper
  data-widget-width={displayColumns}
>
  <!-- Header with drag handle -->
  <div class="widget-header flex items-center justify-between px-4 py-3 border-b border-ds-border">
    <div class="flex items-center gap-2 min-w-0">
      {#if isEditing}
        <button
          class="drag-handle cursor-grab hover:cursor-grabbing text-ds-text-subtlest"
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
      <h3 class="text-sm font-semibold text-ds-text truncate">{title}</h3>
    </div>

    <div class="flex items-center gap-1 flex-shrink-0">
      <!-- Width presets menu (available outside edit mode too) -->
      <DropdownMenu
        triggerIcon={ChevronDown}
        triggerIconBgColor="transparent"
        iconOnly={true}
        triggerLabel={t('widgets.resizeAriaLabel')}
        triggerClass="!p-1 text-ds-text-subtle hover:text-ds-text hover:bg-ds-surface-hover rounded"
        triggerTestid="widget-width-menu"
        placement="bottom-end"
        items={menuItems}
      />

      {#if isEditing}
        <button
          class="hover:text-red-600 p-1 text-ds-text-subtlest"
          onclick={handleRemove}
          title={t('widgets.removeWidget')}
          aria-label={t('widgets.removeWidget')}
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
      {/if}
    </div>
  </div>

  <!-- Widget content -->
  <div class="widget-content p-4">
    {@render children?.()}
  </div>

  <!-- Resize handle (right edge) — exposed only in edit mode -->
  {#if isEditing}
    <button
      class="widget-resize-handle"
      data-testid="widget-resize-handle"
      role="slider"
      tabindex="0"
      aria-label={t('widgets.resizeAriaLabel')}
      aria-valuemin={minWidth}
      aria-valuemax={maxWidth}
      aria-valuenow={displayColumns}
      aria-valuetext={t('widgets.resizeColumnsValue', { count: displayColumns })}
      onmousedown={onResizeStart}
      ondblclick={onResizeHandleDblClick}
      onkeydown={handleSliderKeydown}
    >
      <span class="widget-resize-grip" aria-hidden="true"></span>
    </button>
  {/if}

  {#if isResizing}
    <div class="widget-resize-guide" data-testid="widget-resize-guide">
      {displayColumns}
    </div>
  {/if}
</div>

<style>
  .widget-container {
    position: relative;
    grid-column: span var(--widget-cols, 12);
    transition: box-shadow 0.2s;
  }

  .widget-container:hover {
    box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
  }

  .widget-content {
    container-type: inline-size;
    container-name: widget;
  }

  .drag-handle:active {
    cursor: grabbing;
  }

  .widget-resize-handle {
    position: absolute;
    top: 0;
    right: -3px;
    bottom: 0;
    width: 6px;
    cursor: col-resize;
    background-color: transparent;
    border: none;
    padding: 0;
    z-index: 1;
    border-radius: 2px;
    transition: background-color 140ms ease-in-out;
  }

  .widget-resize-handle:hover,
  .widget-resize-handle:focus-visible {
    background-color: var(--ds-border-focused, #3b82f6);
    outline: none;
  }

  .widget-resize-guide {
    position: absolute;
    top: 0.5rem;
    right: 0.5rem;
    background-color: var(--ds-surface-raised, #fff);
    color: var(--ds-text);
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-full, 9999px);
    padding: 2px 8px;
    font-size: 0.7rem;
    font-weight: var(--font-semibold, 600);
    font-variant-numeric: tabular-nums;
    box-shadow: var(--ds-shadow-raised, 0 1px 2px rgba(0, 0, 0, 0.1));
    pointer-events: none;
    z-index: 2;
  }

  /* Small viewports collapse to full width until container queries take over. */
  @media (max-width: 768px) {
    .widget-container {
      grid-column: 1 / -1;
    }
  }
</style>
