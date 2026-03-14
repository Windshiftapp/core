<script>
  import { MoreHorizontal, ChevronLeft, ChevronRight } from 'lucide-svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import EmptyState from './EmptyState.svelte';
  import { t } from '../stores/i18n.svelte.js';
  import { sanitizeHtml } from '../utils/sanitize.ts';

  let {
    columns = [],
    data = [],
    keyField = 'id',
    loading = false,
    emptyMessage = '',
    emptyDescription = '',
    emptyIcon = null,
    actionItems = null,
    onRowClick = null,
    pagination = false,
    pageSize = 25,
    currentPage = $bindable(1),
    totalItems = null,
    onPageChange = null,
    class: containerClass = 'rounded border shadow-sm',
    ...slotProps
  } = $props();

  let totalCount = $derived(totalItems ?? data.length);
  let totalPages = $derived(Math.ceil(totalCount / pageSize) || 1);
  let startItem = $derived(totalCount > 0 ? (currentPage - 1) * pageSize + 1 : 0);
  let endItem = $derived(Math.min(currentPage * pageSize, totalCount));
  let showPagination = $derived(pagination && totalCount > pageSize);

  let displayData = $derived((pagination && totalItems == null)
    ? data.slice((currentPage - 1) * pageSize, currentPage * pageSize)
    : data);

  function prevPage() {
    if (currentPage > 1) {
      currentPage--;
      onPageChange?.(currentPage);
    }
  }

  function nextPage() {
    if (currentPage < totalPages) {
      currentPage++;
      onPageChange?.(currentPage);
    }
  }
  
  // Default styling classes
  let tableClass = 'w-full';
  let theadClass = '';
  let tbodyClass = 'divide-y';
  let trClass = 'transition-colors duration-150';
  let thClass = 'px-6 py-4 text-left text-xs font-semibold tracking-wide';
  let tdClass = 'px-6 py-4';
  
  function getColumnWidth(column) {
    // If width contains %, px, rem, etc., return as inline style
    if (column.width && (column.width.includes('%') || column.width.includes('px') || column.width.includes('rem'))) {
      return '';  // Don't add to class, will be handled as style
    }
    if (column.width) {
      return column.width;  // Assume it's a Tailwind class like 'w-24'
    }
    if (column.key === 'actions') {
      return 'w-24';
    }
    return '';
  }

  function getColumnWidthStyle(column) {
    // Return inline style for widths with units
    if (column.width && (column.width.includes('%') || column.width.includes('px') || column.width.includes('rem'))) {
      return `width: ${column.width};`;
    }
    return '';
  }
  
  function getColumnAlign(column) {
    return column.align || 'text-left';
  }
  
  function getColumnPadding(column) {
    // Use less padding for narrow icon columns
    // Only reduce padding for pixel widths less than 60, not percentages
    if (column.key === 'icon' || (column.width && column.width.includes('px') && parseInt(column.width) < 60)) {
      return 'px-2 py-4';
    }
    return tdClass;
  }
  
  function getCellValue(item, column) {
    if (column.render) {
      return column.render(item);
    }
    
    // Handle nested properties like 'user.name'
    const keys = column.key.split('.');
    let value = item;
    for (const key of keys) {
      value = value?.[key];
    }
    return value;
  }
  
  function handleRowClick(item, event) {
    if (onRowClick && !event.target.closest('.dropdown-trigger')) {
      onRowClick(item);
    }
  }
</script>

<div class="overflow-hidden {containerClass}" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  {#if displayData.length === 0}
    <EmptyState
      icon={emptyIcon}
      title={emptyMessage || t('common.noData')}
      description={emptyDescription}
    />
  {:else}
    <div class="overflow-x-auto">
      <table class={tableClass}>
        <thead class={theadClass} style="background-color: var(--ds-surface);">
          <tr>
            {#each columns as column, colIndex}
              <th
                class="{thClass} {getColumnAlign(column)} {getColumnWidth(column)}"
                style="color: var(--ds-text); {getColumnWidthStyle(column)} {column.headerStyle || ''}"
              >
                {column.label}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody class={tbodyClass} style="--tw-divide-opacity: 1; border-color: var(--ds-border);">
          {#each displayData as item (item[keyField])}
            <tr
              class="{trClass} {onRowClick ? 'cursor-pointer' : ''}"
              style="border-color: var(--ds-border);"
              onclick={(e) => handleRowClick(item, e)}
              onmouseenter={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
              onmouseleave={(e) => e.currentTarget.style.backgroundColor = ''}
            >
              {#each columns as column, colIndex}
                <td class="{getColumnPadding(column)} {getColumnAlign(column)} {getColumnWidth(column)} {colIndex === 0 ? 'pl-4' : ''}" style="{getColumnWidthStyle(column)}">
                  {#if column.key === 'actions' && actionItems}
                    <div class="dropdown-trigger">
                      <DropdownMenu
                        triggerIcon={MoreHorizontal}
                        triggerClass="w-7 h-7 flex items-center justify-center rounded-md transition-colors"
                        triggerStyle="background-color: var(--ds-surface); color: var(--ds-text-subtle);"
                        items={actionItems(item)}
                        maxWidth="max-w-48"
                        showChevron={false}
                        iconOnly={true}
                      />
                    </div>
                  {:else if column.slot && slotProps[column.slot]}
                    {@render slotProps[column.slot](item, column)}
                  {:else}
                    <!-- Default cell content -->
                    {#if column.render && column.html}
                      <!-- Only render as HTML if explicitly opted-in with html:true -->
                      {@html sanitizeHtml(getCellValue(item, column)) || '—'}
                    {:else if column.render}
                      <!-- Render function output as text (safe by default) -->
                      {getCellValue(item, column) || '—'}
                    {:else}
                      <span style="color: {column.textColor || (column.key === 'actions' ? 'var(--ds-text)' : 'var(--ds-text)')};">
                        {getCellValue(item, column) || '—'}
                      </span>
                    {/if}
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    {#if showPagination}
      <div class="flex items-center justify-between px-4 py-3 border-t" style="border-color: var(--ds-border);">
        <span class="text-sm" style="color: var(--ds-text-subtle);">
          {t('components.dataTable.showingRange', { start: startItem, end: endItem, total: totalCount })}
        </span>
        <div class="flex items-center gap-2">
          <button
            onclick={prevPage}
            disabled={currentPage === 1}
            class="p-1.5 rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style="background: var(--ds-background-neutral); color: var(--ds-text);"
          >
            <ChevronLeft class="w-4 h-4" />
          </button>
          <span class="text-sm px-2" style="color: var(--ds-text-subtle);">
            {t('components.pagination.pageOf', { current: currentPage, total: totalPages })}
          </span>
          <button
            onclick={nextPage}
            disabled={currentPage >= totalPages}
            class="p-1.5 rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
            style="background: var(--ds-background-neutral); color: var(--ds-text);"
          >
            <ChevronRight class="w-4 h-4" />
          </button>
        </div>
      </div>
    {/if}
  {/if}
</div>
