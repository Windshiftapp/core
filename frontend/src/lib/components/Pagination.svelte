<script>
  import { createEventDispatcher } from 'svelte';
  import { ChevronLeft, ChevronRight, MoreHorizontal } from 'lucide-svelte';
  import Button from './Button.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  
  const dispatch = createEventDispatcher();
  
  export let currentPage = 1;
  export let totalItems = 0;
  export let itemsPerPage = 50;
  export let maxItems = 100; // Maximum items to show from API
  export let showPageSizes = true;
  export let pageSizeOptions = [10, 25, 50];
  export let compact = false; // For smaller spaces
  export let hasGradient = false; // Gradient background awareness

  // Gradient-aware text classes with dark mode support
  $: textClass = hasGradient ? 'text-white' : 'text-[var(--ds-text,#111827)]';
  $: subtleTextClass = hasGradient ? 'text-white/80' : 'text-[var(--ds-text-subtle,#6b7280)]';
  $: ellipsisClass = hasGradient ? 'text-white/60' : 'text-[var(--ds-text-subtlest,#9ca3af)]';
  $: warningClass = hasGradient ? 'text-orange-300' : 'text-orange-600';

  // Gradient-aware select box styling with dark mode support
  $: selectClass = hasGradient
    ? 'bg-white/90 backdrop-blur-sm border-white/60 text-gray-900'
    : 'bg-[var(--ds-surface-raised,white)] border-[var(--ds-border,#d1d5db)] text-[var(--ds-text,#111827)]';

  // Calculate pagination values
  $: totalPages = Math.ceil(Math.min(totalItems, maxItems) / itemsPerPage);
  $: startItem = Math.min((currentPage - 1) * itemsPerPage + 1, totalItems);
  $: endItem = Math.min(currentPage * itemsPerPage, totalItems, maxItems);
  $: isFirstPage = currentPage === 1;
  $: isLastPage = currentPage === totalPages || endItem >= Math.min(totalItems, maxItems);
  
  // Generate page numbers to show
  $: visiblePages = generateVisiblePages(currentPage, totalPages);
  
  function generateVisiblePages(current, total) {
    if (total <= 7) {
      return Array.from({ length: total }, (_, i) => i + 1);
    }
    
    if (current <= 4) {
      return [1, 2, 3, 4, 5, '...', total];
    }
    
    if (current >= total - 3) {
      return [1, '...', total - 4, total - 3, total - 2, total - 1, total];
    }
    
    return [1, '...', current - 1, current, current + 1, '...', total];
  }
  
  function goToPage(page) {
    if (page >= 1 && page <= totalPages && page !== currentPage) {
      dispatch('pageChange', { page, itemsPerPage });
    }
  }
  
  function handlePageSizeChange(event) {
    const newPageSize = parseInt(event.target.value);
    const newPage = Math.min(currentPage, Math.ceil(Math.min(totalItems, maxItems) / newPageSize));
    dispatch('pageSizeChange', { page: newPage, itemsPerPage: newPageSize });
  }
</script>

{#if totalItems > 0}
  <div class="pagination-container {compact ? 'compact' : ''} {subtleTextClass}">

    <!-- Items info and page size selector -->
    <div class="flex items-center justify-between gap-4 mb-4">
      <div class="text-sm {textClass}">
        Showing {startItem}-{endItem} of {Math.min(totalItems, maxItems)}
        {#if totalItems > maxItems}
          <span class="{warningClass}">(limited to {maxItems} items)</span>
        {/if}
      </div>

      {#if showPageSizes && !compact}
        <div class="flex items-center gap-2 text-sm {textClass}">
          <label>Items per page:</label>
          <div style="min-width: 100px;">
            <BasePicker
              value={itemsPerPage}
              items={pageSizeOptions}
              placeholder="Select"
              getValue={(item) => item}
              getLabel={(item) => String(item)}
              onSelect={(item) => {
                if (item) {
                  const newPageSize = item;
                  const newPage = Math.min(currentPage, Math.ceil(Math.min(totalItems, maxItems) / newPageSize));
                  dispatch('pageSizeChange', { page: newPage, itemsPerPage: newPageSize });
                }
              }}
            />
          </div>
        </div>
      {/if}
    </div>
    
    <!-- Pagination controls -->
    {#if totalPages > 1}
      <div class="flex items-center justify-center gap-1">
        <!-- Previous button -->
        <Button
          variant="default"
          size="small"
          icon={ChevronLeft}
          onclick={() => goToPage(currentPage - 1)}
          disabled={isFirstPage}
          class="px-2"
          title="Previous page"
        />
        
        <!-- Page numbers -->
        <div class="flex items-center gap-1 mx-2">
          {#each visiblePages as page}
            {#if page === '...'}
              <span class="px-2 py-1 {ellipsisClass}">
                <MoreHorizontal class="w-4 h-4" />
              </span>
            {:else}
              <Button
                variant={page === currentPage ? 'primary' : 'default'}
                size="small"
                onclick={() => goToPage(page)}
                class="min-w-[32px] px-2"
                title="Go to page {page}"
              >
                {page}
              </Button>
            {/if}
          {/each}
        </div>
        
        <!-- Next button -->
        <Button
          variant="default"
          size="small"
          icon={ChevronRight}
          onclick={() => goToPage(currentPage + 1)}
          disabled={isLastPage}
          class="px-2"
          title="Next page"
        />
      </div>
    {/if}
  </div>
{/if}

<style>
  .pagination-container.compact {
    font-size: 0.875rem;
  }
  
  .pagination-container.compact .flex {
    gap: 0.5rem;
  }
</style>