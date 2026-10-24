<script>
  let {
    title = '',
    subtitle = '',
    actions = null,
    testId = undefined,
    class: className = ''
  } = $props();
</script>

<div class="flex items-center justify-between mb-4 {className}" data-testid={testId}>
  {#if title}
    <div>
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);" data-testid={testId ? `${testId}-title` : undefined}>
        {title}
      </h3>
      {#if subtitle}
        <p class="text-sm" style="color: var(--ds-text-subtle);" data-testid={testId ? `${testId}-subtitle` : undefined}>
          {subtitle}
        </p>
      {/if}
    </div>
  {/if}
  {#if actions}
    <div class="flex items-center gap-2 ml-auto">
      {@render actions()}
    </div>
  {/if}
</div>
