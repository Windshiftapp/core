<script>
  import { X } from 'lucide-svelte';
    import Button from '../components/Button.svelte';

  let {
    title,
    subtitle = '',
    icon = null,
    showCloseButton = true,
    onClose = null
  } = $props();
</script>

<div class="px-6 py-4 border-b flex items-center justify-between" style="border-color: var(--ds-border);">
  <div class="flex items-center gap-3">
    {#if icon}
      <svelte:component this={icon} class="w-6 h-6" style="color: var(--ds-interactive);" />
    {/if}
    <div>
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{title}</h3>
      {#if subtitle}
        <p class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">{subtitle}</p>
      {/if}
    </div>
  </div>
  {#if showCloseButton && onClose}
    <Button
      onclick={onClose}
    >
      <X class="w-5 h-5" style="color: var(--ds-text-subtle);" />
    </Button>
  {/if}
</div>
