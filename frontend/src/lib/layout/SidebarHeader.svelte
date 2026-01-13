<script>
  let {
    title,
    description = '',
    icon = null,
    class: className = ''
  } = $props();
</script>

<div class="px-4 py-5 border-b {className}" style="border-color: var(--ds-border);">
  <div class="flex items-center gap-3">
    {#if icon}
      <svelte:component this={icon} class="w-8 h-8 flex-shrink-0" style="color: var(--ds-interactive);" />
    {/if}
    <div class="min-w-0">
      <h1 class="text-xl font-semibold truncate" style="color: var(--ds-text);">
        {title}
      </h1>
      {#if description}
        <p class="text-sm mt-1 truncate" style="color: var(--ds-text-subtle);">
          {description}
        </p>
      {/if}
    </div>
  </div>
</div>
