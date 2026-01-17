<script>
  import { t } from '../stores/i18n.svelte.js';

  let {
    size = 'md', // 'sm', 'md', 'lg'
    class: className = ''
  } = $props();

  const sizeClasses = $derived({
    sm: 'h-4 w-4',
    md: 'h-8 w-8',
    lg: 'h-12 w-12'
  }[size] || 'h-8 w-8');
</script>

<div
  class="animate-spin rounded-full border-b-2 {sizeClasses} {className}"
  style="border-color: var(--ds-interactive);"
  role="status"
  aria-label={t('common.loading')}
></div>
