<script>
  let {
    title = '',
    subtitle = '',
    actions = null,
    class: className = ''
  } = $props();
</script>

<div class="flex items-center justify-between mb-4 {className}">
  <div>
    <h3 class="text-lg font-semibold" style="color: var(--ds-text);">
      {title}
    </h3>
    {#if subtitle}
      <p class="text-sm" style="color: var(--ds-text-subtle);">
        {subtitle}
      </p>
    {/if}
  </div>
  {#if actions}
    <div class="flex items-center gap-2">
      {@render actions()}
    </div>
  {/if}
</div>
