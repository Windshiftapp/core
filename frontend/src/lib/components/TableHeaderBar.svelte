<script>
  let { columns = '', class: className = '', style: customStyle = '', children } = $props();
</script>

<div
  class="px-4 py-3 border-b {className}"
  style="background-color: var(--ds-surface); border-color: var(--ds-border); {customStyle}"
>
  <div
    class="grid gap-4 items-center text-xs font-semibold tracking-wide"
    style="grid-template-columns: {columns}; color: var(--ds-text);"
  >
    {@render children()}
  </div>
</div>
