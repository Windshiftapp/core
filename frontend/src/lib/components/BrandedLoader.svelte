<script>
  import { APP_NAME } from '../constants.js';
  import { t } from '../stores/i18n.svelte.js';

  let { label = '', detail = '', fullViewport = true } = $props();
  let displayLabel = $derived(label || t('common.loading'));
</script>

<div
  class="flex w-full items-center justify-center px-6 {fullViewport ? 'min-h-dvh' : 'h-full'}"
  style="color: var(--ds-text);"
  role="status"
  data-testid="branded-loader"
>
  <div class="text-center">
    <img
      src="windshift-3.svg"
      alt={APP_NAME}
      class="mx-auto mb-4 h-16 w-16 motion-safe:animate-pulse"
      width="64"
      height="64"
      decoding="async"
      data-testid="branded-loader-logo"
    />
    <p class="text-sm" style="color: var(--ds-text-subtle);">{displayLabel}</p>
    {#if detail}
      <p class="mt-1 text-sm" style="color: var(--ds-text-subtlest);">{detail}</p>
    {/if}
  </div>
</div>
