<script>
  import Spinner from './Spinner.svelte';
  import { t } from '../stores/i18n.svelte.js';

  /**
   * LoadingState - Centered loading spinner with optional message
   *
   * @example
   * <LoadingState />
   *
   * @example with custom message
   * <LoadingState message="Loading items..." />
   *
   * @example inline
   * <LoadingState inline size="sm" message="Saving..." />
   */
  let {
    message = '',
    size = 'md', // 'sm' | 'md' | 'lg'
    inline = false, // horizontal layout
    class: className = ''
  } = $props();
</script>

{#if inline}
  <div class="flex items-center gap-2 {className}">
    <Spinner {size} />
    <span class="text-sm" style="color: var(--ds-text-subtle);">{message || t('common.loading')}</span>
  </div>
{:else}
  <div class="flex flex-col items-center justify-center py-8 {className}">
    <Spinner {size} />
    <p class="mt-3 text-sm" style="color: var(--ds-text-subtle);">{message || t('common.loading')}</p>
  </div>
{/if}
