<script>
  let { columns = '', class: className = '', style: customStyle = '', children } = $props();
</script>

<div
  class="px-4 py-3 border-b {className}"
  data-testid="list-header"
  style="background-color: var(--ds-surface); border-color: var(--ds-border); {customStyle}"
>
  <div
    class="header-grid grid gap-4 items-center text-xs font-semibold tracking-wide"
    style="grid-template-columns: {columns}; color: var(--ds-text);"
  >
    {@render children()}
  </div>
</div>

<style>
  /* Long labels must truncate inside their column instead of overlapping the next one. */
  .header-grid > :global(*) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
