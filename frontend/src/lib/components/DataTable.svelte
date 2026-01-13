<script>
  import { MoreHorizontal, ChevronLeft, ChevronRight } from 'lucide-svelte';
  import DropdownMenu from '../layout/DropdownMenu.svelte';
  import EmptyState from './EmptyState.svelte';

  export let columns = []; // Array of column definitions: { key, label, width?, align?, sortable? }
  export let data = []; // Array of data objects
  export let keyField = 'id'; // Field to use as unique key
  export let emptyMessage = 'No data found';
  export let emptyDescription = ''; // Optional description for empty state
  export let emptyIcon = null; // Lucide icon component
  export let actionItems = null; // Function that takes (item) => dropdownItems array
  export let onRowClick = null; // Function to handle row clicks

  // Pagination props
  export let pagination = false;        // Enable pagination
  export let pageSize = 25;             // Items per page
  export let currentPage = 1;           // Current page (bindable)
  export let totalItems = null;         // Total items (for server-side pagination)
  export let onPageChange = null;       // Callback for page changes

  // Pagination computed values
  $: totalCount = totalItems ?? data.length;
  $: totalPages = Math.ceil(totalCount / pageSize) || 1;
  $: startItem = totalCount > 0 ? (currentPage - 1) * pageSize + 1 : 0;
  $: endItem = Math.min(currentPage * pageSize, totalCount);
  $: showPagination = pagination && totalCount > pageSize;

  // Display data - for server-side pagination data is already paginated
  $: displayData = (pagination && totalItems == null)
    ? data.slice((currentPage - 1) * pageSize, currentPage * pageSize)
    : data;

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
  let containerClass = 'rounded border shadow-sm overflow-hidden';
  let theadClass = '';
  let tbodyClass = 'divide-y';
  let trClass = 'transition-colors duration-150';
  let thClass = 'px-6 py-4 text-left text-xs font-semibold uppercase tracking-wider';
  let tdClass = 'px-6 py-4';
  
  export { containerClass as class };
  
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

<div class="{containerClass}" style="background-color: var(--ds-surface-raised); border-color: var(--ds-border);">
  {#if displayData.length === 0}
    <EmptyState
      icon={emptyIcon}
      title={emptyMessage}
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
                style="color: var(--ds-text-subtle); {getColumnWidthStyle(column)} {column.headerStyle || ''}"
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
                  {:else if column.slot === 'name'}
                    <slot name="name" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'type'}
                    <slot name="type" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'status'}
                    <slot name="status" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'usage'}
                    <slot name="usage" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'category'}
                    <slot name="category" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'color'}
                    <slot name="color" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'icon'}
                    <slot name="icon" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'hierarchy_level'}
                    <slot name="hierarchy_level" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'tests'}
                    <slot name="tests" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'days_remaining'}
                    <slot name="days_remaining" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'action'}
                    <slot name="action" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'user'}
                    <slot name="user" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'resource'}
                    <slot name="resource" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'details'}
                    <slot name="details" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'date_range'}
                    <slot name="date_range" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'scope'}
                    <slot name="scope" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'role'}
                    <slot name="role" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'date'}
                    <slot name="date" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'actions'}
                    <slot name="actions" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'level'}
                    <slot name="level" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'step_number'}
                    <slot name="step_number" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'step_action'}
                    <slot name="step_action" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'step_data'}
                    <slot name="step_data" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else if column.slot === 'step_expected'}
                    <slot name="step_expected" {item} {column}>
                      {getCellValue(item, column) || '—'}
                    </slot>
                  {:else}
                    <!-- Default cell content -->
                    {#if column.render && column.html}
                      <!-- Only render as HTML if explicitly opted-in with html:true -->
                      {@html getCellValue(item, column) || '—'}
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
          Showing {startItem}–{endItem} of {totalCount}
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
            Page {currentPage} of {totalPages}
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
