<script>
  /**
   * FormSectionTitle - Section title for forms with optional description
   *
   * Use to divide forms into logical sections with clear headings.
   *
   * @example
   * <FormSectionTitle title="General Settings" />
   * <FormSectionTitle title="Advanced" description="Configure advanced options" />
   */
  let {
    title,
    description = null,
    class: className = ''
  } = $props();
</script>

<div class={className}>
  <h3 class="text-sm font-semibold uppercase tracking-wider" style="color: var(--ds-text-subtle);">
    {title}
  </h3>
  {#if description}
    <p class="text-xs mt-1" style="color: var(--ds-text-subtlest);">
      {description}
    </p>
  {/if}
</div>
