<script>
  import { AlertCircle, Inbox, RefreshCw } from 'lucide-svelte';
  import Button from './Button.svelte';
  import Spinner from './Spinner.svelte';
  import { t } from '../stores/i18n.svelte.js';

  /**
   * StateDisplay - Base component for displaying application states
   *
   * @example error state
   * <StateDisplay type="error" title="Failed to load" message="Please try again" />
   *
   * @example empty state
   * <StateDisplay type="empty" title="No items" description="Add your first item" />
   *
   * @example loading state
   * <StateDisplay type="loading" message="Loading items..." />
   *
   * @example with action
   * <StateDisplay type="error" title="Error" onRetry={() => reload()}>
   *   {#snippet action()}<Button>Custom action</Button>{/snippet}
   * </StateDisplay>
   */
  let {
    type = 'empty', // 'error' | 'empty' | 'loading'
    icon: IconComponent = null,
    title = '',
    message = '',
    description = '', // Alias for message (used by EmptyState)
    onRetry = null,
    retryLabel = '',
    action = null, // Snippet for custom action
    size = 'md', // For loading spinner: 'sm' | 'md' | 'lg'
    inline = false, // For loading: horizontal layout
    hasGradient = false, // Whether displayed on a gradient background
    class: className = ''
  } = $props();

  // Computed icon color based on type and gradient
  const iconColor = $derived(
    hasGradient && type === 'empty'
      ? 'rgba(255, 255, 255, 0.6)'
      : {
          error: 'var(--ds-icon-danger)',
          empty: 'var(--ds-icon-disabled)',
          loading: 'var(--ds-icon-subtle)'
        }[type] || 'var(--ds-icon-disabled)'
  );

  // Text colors based on gradient
  const titleColor = $derived(
    hasGradient && type === 'empty'
      ? 'rgba(255, 255, 255, 0.8)'
      : type === 'error' ? 'var(--ds-text)' : 'var(--ds-text-subtle)'
  );

  const messageColor = $derived(
    hasGradient && type === 'empty'
      ? 'rgba(255, 255, 255, 0.6)'
      : 'var(--ds-text-subtle)'
  );

  // Default icons per type
  const defaultIcon = $derived({
    error: AlertCircle,
    empty: Inbox,
    loading: null // Loading uses Spinner instead
  }[type]);

  // Resolved icon (custom or default)
  const resolvedIcon = $derived(IconComponent || defaultIcon);

  // Display message (supports both 'message' and 'description' props)
  const displayMessage = $derived(message || description);

  // Padding based on type
  const padding = $derived({
    error: 'py-8',
    empty: 'py-12',
    loading: 'py-8'
  }[type] || 'py-8');
</script>

{#if type === 'loading' && inline}
  <!-- Inline loading state -->
  <div class="flex items-center gap-2 {className}">
    <Spinner {size} />
    <span class="text-sm" style="color: var(--ds-text-subtle);">
      {displayMessage || t('common.loading')}
    </span>
  </div>
{:else if type === 'loading'}
  <!-- Centered loading state -->
  <div class="flex flex-col items-center justify-center {padding} {className}">
    <Spinner {size} />
    <p class="mt-3 text-sm" style="color: var(--ds-text-subtle);">
      {displayMessage || t('common.loading')}
    </p>
  </div>
{:else}
  <!-- Error or Empty state -->
  <div class="text-center {padding} {className}">
    {#if resolvedIcon}
      <svelte:component this={resolvedIcon} class="w-10 h-10 mx-auto mb-3" style="color: {iconColor};" />
    {/if}

    {#if title}
      <h3
        class="text-lg font-medium mb-1"
        style="color: {titleColor};"
      >
        {title}
      </h3>
    {:else if type === 'error'}
      <h3 class="text-lg font-medium mb-1" style="color: {titleColor};">
        {t('components.errorState.title')}
      </h3>
    {:else if type === 'empty'}
      <h3 class="text-lg font-medium mb-1" style="color: {titleColor};">
        {t('common.noData')}
      </h3>
    {/if}

    {#if displayMessage}
      <p class="text-base mb-4" style="color: {messageColor};">{displayMessage}</p>
    {/if}

    {#if action}
      <div class="mt-4">
        {@render action()}
      </div>
    {:else if onRetry}
      <Button variant="default" onclick={onRetry}>
        <RefreshCw class="w-4 h-4 mr-2" />
        {retryLabel || t('common.retry')}
      </Button>
    {/if}
  </div>
{/if}
