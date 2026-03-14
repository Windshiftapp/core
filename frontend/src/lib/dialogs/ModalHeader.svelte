<script>
  import { X } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

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
      <icon class="w-6 h-6" style="color: var(--ds-interactive);" />
    {/if}
    <div>
      <h3 class="text-lg font-semibold" style="color: var(--ds-text);">{title}</h3>
      {#if subtitle}
        <p class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">{subtitle}</p>
      {/if}
    </div>
  </div>
  {#if showCloseButton && onClose}
    <button
      onclick={onClose}
      class="p-1.5 rounded transition-colors"
      style="color: var(--ds-text-subtle);"
      onmouseover={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
      onmouseout={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
      onfocus={(e) => e.currentTarget.style.backgroundColor = 'var(--ds-background-neutral-hovered)'}
      onblur={(e) => e.currentTarget.style.backgroundColor = 'transparent'}
      aria-label={t('aria.close')}
    >
      <X size={16} />
    </button>
  {/if}
</div>
