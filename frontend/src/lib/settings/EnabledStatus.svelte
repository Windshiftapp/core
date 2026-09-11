<script>
  import { Power, PowerOff } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  let { enabled = false } = $props();
</script>

{#if enabled}
  <div class="flex items-center gap-1">
    <Power size={14} style="color: var(--ds-icon-success);" />
    <span class="text-xs" style="color: var(--ds-text-success);">{t('common.enabled')}</span>
  </div>
{:else}
  <div class="flex items-center gap-1">
    <PowerOff size={14} style="color: var(--ds-text-subtle);" />
    <span class="text-xs" style="color: var(--ds-text-subtle);">{t('common.disabled')}</span>
  </div>
{/if}
