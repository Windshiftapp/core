<script>
  let {
    header,
    body,
    footer,
    maxWidth = 'max-w-lg',
    class: className = '',
    dataTestid = undefined,
  } = $props();
</script>

<div
  class="mx-4 mb-8 flex w-full {maxWidth} flex-col rounded-lg shadow-2xl {className}"
  style="background-color: var(--ds-surface-raised);"
  data-testid={dataTestid}
>
  <div class="flex items-center gap-2 border-b px-4 py-3" style="border-color: var(--ds-border);">
    {@render header?.()}
  </div>

  <div class="px-4 py-3">
    {@render body?.()}
  </div>

  <div class="flex items-center justify-end border-t px-4 py-3" style="border-color: var(--ds-border);">
    {@render footer?.()}
  </div>
</div>
