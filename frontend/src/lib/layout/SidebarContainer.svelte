<script>
  let {
    width = 'w-64',
    class: className = '',
    header = null,
    footer = null,
    children
  } = $props();
</script>

<aside
  class="{width} border-r flex-shrink-0 flex flex-col {className}"
  style="border-color: var(--ds-border); background: var(--ds-surface);"
>
  {#if header}
    {@render header()}
  {/if}

  <div class="flex-1 overflow-y-auto">
    {@render children()}
  </div>

  {#if footer}
    <div class="border-t" style="border-color: var(--ds-border);">
      {@render footer()}
    </div>
  {/if}
</aside>
