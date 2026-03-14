<script>
  import { ChevronLeft, ChevronRight, MoreHorizontal } from 'lucide-svelte';
  import Button from './Button.svelte';
  import BasePicker from '../pickers/BasePicker.svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    currentPage = 1,
    totalItems = 0,
    itemsPerPage = 50,
    maxItems = 10000,
    showPageSizes = true,
    pageSizeOptions = [10, 25, 50, 100],
    compact = false,
    hasGradient = false,
    onpageChange = null,
    onpageSizeChange = null
  } = $props();

  let textStyle = $derived(hasGradient ? 'color: var(--ds-text);' : 'color: var(--ds-text);');
  let subtleTextStyle = $derived(hasGradient ? 'color: var(--ds-text-subtle);' : 'color: var(--ds-text-subtle);');
  let ellipsisStyle = $derived(hasGradient ? 'color: var(--ds-text-subtlest);' : 'color: var(--ds-text-subtlest);');
  let warningStyle = $derived(hasGradient ? 'color: #ea580c;' : 'color: #ea580c;');

  let containerStyle = $derived(hasGradient
    ? 'background-color: var(--ds-glass-bg); backdrop-filter: blur(12px); border: 1px solid var(--ds-glass-border); border-radius: 0.75rem; padding: 1rem;'
    : '');

  let totalPages = $derived(Math.ceil(Math.min(totalItems, maxItems) / itemsPerPage));
  let startItem = $derived(Math.min((currentPage - 1) * itemsPerPage + 1, totalItems));
  let endItem = $derived(Math.min(currentPage * itemsPerPage, totalItems, maxItems));
  let isFirstPage = $derived(currentPage === 1);
  let isLastPage = $derived(currentPage === totalPages || endItem >= Math.min(totalItems, maxItems));

  let visiblePages = $derived(generateVisiblePages(currentPage, totalPages));
  
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
      onpageChange?.({ detail: { page, itemsPerPage } });
    }
  }

  function handlePageSizeChange(event) {
    const newPageSize = parseInt(event.target.value);
    const newPage = Math.min(currentPage, Math.ceil(Math.min(totalItems, maxItems) / newPageSize));
    onpageSizeChange?.({ detail: { page: newPage, itemsPerPage: newPageSize } });
  }
</script>

{#if totalItems > 0}
  <div class="pagination-container {compact ? 'compact' : ''}" style="{containerStyle} {subtleTextStyle}">

    <!-- Items info and page size selector -->
    <div class="flex items-center justify-between gap-4 mb-4">
      <div class="text-sm" style={textStyle}>
        {t('components.pagination.showingRange', { start: startItem, end: endItem, total: Math.min(totalItems, maxItems) })}
        {#if totalItems > maxItems}
          <span style={warningStyle}>({t('components.pagination.limitedTo', { max: maxItems })})</span>
        {/if}
      </div>

      {#if showPageSizes && !compact}
        <div class="flex items-center gap-2 text-sm" style={textStyle}>
          <span>{t('components.pagination.itemsPerPage')}</span>
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
                  onpageSizeChange?.({ detail: { page: newPage, itemsPerPage: newPageSize } });
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
          title={t('components.pagination.previousPage')}
        />
        
        <!-- Page numbers -->
        <div class="flex items-center gap-1 mx-2">
          {#each visiblePages as page}
            {#if page === '...'}
              <span class="px-2 py-1" style={ellipsisStyle}>
                <MoreHorizontal class="w-4 h-4" />
              </span>
            {:else}
              <Button
                variant={page === currentPage ? 'primary' : 'default'}
                size="small"
                onclick={() => goToPage(page)}
                class="min-w-[32px] px-2"
                title={t('components.pagination.goToPage', { page })}
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
          title={t('components.pagination.nextPage')}
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