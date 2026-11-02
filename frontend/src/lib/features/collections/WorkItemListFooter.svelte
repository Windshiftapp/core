<script>
  import Pagination from '../../components/Pagination.svelte';

  let {
    pagination = null,
    itemCount = 0,
    onpageChange = null,
    onpageSizeChange = null,
  } = $props();
</script>

{#if pagination && pagination.total_items > 0 && itemCount > 0}
  <div class="mt-6">
    <Pagination
      currentPage={pagination.page}
      totalItems={pagination.total_items}
      itemsPerPage={pagination.limit}
      maxItems={10000}
      {onpageChange}
      {onpageSizeChange}
    />
  </div>
{/if}
