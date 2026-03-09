<script>
  import { AlertCircle, CircleCheck, Info, AlertTriangle } from 'lucide-svelte';

  let {
    variant: variantProp = 'error', // 'error', 'warning', 'info', 'success', 'neutral'
    type = undefined,               // Alias for variant
    message = '',
    showIcon = true,
    class: className = '',
    children = undefined
  } = $props();

  const variant = $derived(type || variantProp);

  const styles = $derived({
    error: {
      borderColor: 'var(--ds-border-danger)',
      iconColor: 'var(--ds-icon-danger)',
      icon: AlertCircle
    },
    warning: {
      borderColor: 'var(--ds-border-warning)',
      iconColor: 'var(--ds-icon-warning)',
      icon: AlertTriangle
    },
    info: {
      borderColor: 'var(--ds-border-info)',
      iconColor: 'var(--ds-icon-info)',
      icon: Info
    },
    success: {
      borderColor: 'var(--ds-border-success)',
      iconColor: 'var(--ds-icon-success)',
      icon: CircleCheck
    },
    neutral: {
      borderColor: 'var(--ds-border)',
      iconColor: 'var(--ds-icon)',
      icon: Info
    }
  }[variant]);

  const IconComponent = $derived(styles?.icon || AlertCircle);
</script>

{#if message || children}
  <div
    class="px-4 py-3 rounded flex items-start gap-3 {className}"
    style="background: var(--ds-surface-raised); border: 1px solid var(--ds-border); border-left: 4px solid {styles?.borderColor}; color: var(--ds-text);"
  >
    {#if showIcon}
      <IconComponent class="w-5 h-5 flex-shrink-0 mt-0.5" style="color: {styles?.iconColor};" />
    {/if}
    {#if children}
      <div class="text-sm flex-1">
        {@render children()}
      </div>
    {:else}
      <span class="text-sm">{message}</span>
    {/if}
  </div>
{/if}
