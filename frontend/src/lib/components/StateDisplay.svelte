<script>
  import { IconAlertCircle as AlertCircle, IconInbox as Inbox, IconRefresh as RefreshCw } from '@tabler/icons-svelte-runes';
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
  /**
   * @type {{
   *   type?: 'error' | 'empty' | 'loading',
   *   icon?: import('svelte').Component<any, any, any> | null,
   *   title?: string,
   *   message?: string,
   *   description?: string,
   *   onRetry?: (() => void) | null,
   *   retryLabel?: string,
   *   action?: import('svelte').Snippet | null,
   *   size?: 'sm' | 'md' | 'lg',
   *   inline?: boolean,
   *   class?: string,
   * }}
   */
  let {
    type = 'empty',
    icon: IconComponent = null,
    title = '',
    message = '',
    description = '',
    onRetry = null,
    retryLabel = '',
    action = null,
    size = 'md',
    inline = false,
    class: className = ''
  } = $props();

  // Computed icon color based on type (uses --ctx-* vars from parent when on gradient)
  const iconColor = $derived(
    type === 'empty'
      ? 'var(--ctx-text-subtlest, var(--ds-icon-disabled))'
      : {
          error: 'var(--ds-icon-danger)',
          loading: 'var(--ds-icon-subtle)'
        }[type] || 'var(--ds-icon-disabled)'
  );

  // Text colors (uses --ctx-* vars from parent when on gradient)
  const titleColor = $derived(
    type === 'empty'
      ? 'var(--ctx-text-subtle, var(--ds-text-subtle))'
      : type === 'error' ? 'var(--ds-text)' : 'var(--ds-text-subtle)'
  );

  const messageColor = $derived(
    type === 'empty'
      ? 'var(--ctx-text-subtlest, var(--ds-text-subtle))'
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
      {@const Icon = resolvedIcon}
      <Icon class="w-8 h-8 mx-auto mb-3" style="color: {iconColor};" />
    {/if}

    {#if title}
      <h3
        class="text-base font-medium mb-1"
        style="color: {titleColor};"
      >
        {title}
      </h3>
    {:else if type === 'error'}
      <h3 class="text-base font-medium mb-1" style="color: {titleColor};">
        {t('components.errorState.title')}
      </h3>
    {:else if type === 'empty'}
      <h3 class="text-base font-medium mb-1" style="color: {titleColor};">
        {t('common.noData')}
      </h3>
    {/if}

    {#if displayMessage}
      <p class="text-sm mb-4" style="color: {messageColor};">{displayMessage}</p>
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
