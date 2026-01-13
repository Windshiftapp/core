<script>
  let { icon, bgColor, iconColor, label, value } = $props();
</script>

<div class="stat-card flex-1">
  <div class="flex items-center">
    <div class="flex-shrink-0">
      <div class="rounded p-2" style="background-color: {bgColor};">
        <svelte:component this={icon} class="h-5 w-5" style="color: {iconColor};" />
      </div>
    </div>
    <div class="ml-3 w-0 flex-1">
      <dl>
        <dt class="text-xs font-medium truncate" style="color: var(--ds-text-subtle);">{label}</dt>
        <dd class="text-xl font-semibold" style="color: var(--ds-text);">{value}</dd>
      </dl>
    </div>
  </div>
</div>

<style>
  .stat-card {
    transition: all 0.2s ease;
  }
</style>
