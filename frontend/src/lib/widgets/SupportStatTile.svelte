<script>
  /**
   * One tile in the support metrics summary grid.
   *
   * @type {{
   *   label: string,
   *   value: string | number,
   *   dataTestid?: string,
   * }}
   */
  let { label, value, dataTestid = undefined } = $props();
</script>

<div class="rounded-lg px-2 py-1.5" style="background: var(--ds-surface-sunken);" data-testid={dataTestid}>
  <div class="text-lg font-semibold" style="color: var(--ds-text);">{value}</div>
  <div class="text-xs" style="color: var(--ds-text-subtle);">{label}</div>
</div>
