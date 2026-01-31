<script>
  import { Check } from 'lucide-svelte';

  let {
    steps = [],           // Array of step objects or labels
    currentStep = 1,      // 1-indexed current step
    showLabels = false,   // Whether to show step labels
    size = 'default',     // 'small' (7x7) or 'default' (8x8)
    getLabel = null       // Optional function (step) => label string
  } = $props();

  function getStatus(index) {
    if (index + 1 < currentStep) return 'completed';
    if (index + 1 === currentStep) return 'current';
    return 'pending';
  }

  // Size classes
  const sizeClasses = {
    small: { circle: 'w-7 h-7 text-xs', icon: 14 },
    default: { circle: 'w-8 h-8 text-sm', icon: 16 }
  };

  let sizeConfig = $derived(sizeClasses[size] || sizeClasses.default);
</script>

<div class="flex items-center justify-center gap-2">
  {#each steps as step, index}
    {@const status = getStatus(index)}
    {@const label = getLabel ? getLabel(step) : (typeof step === 'string' ? step : null)}
    <div class="flex items-center">
      <div class="flex items-center gap-2">
        <div
          class="{sizeConfig.circle} rounded-full flex items-center justify-center font-medium transition-colors"
          style="background: {status !== 'pending' ? 'var(--ds-interactive)' : 'var(--ds-background-neutral)'};
                 color: {status !== 'pending' ? 'white' : 'var(--ds-text-subtle)'};"
        >
          {#if status === 'completed'}
            <Check size={sizeConfig.icon} />
          {:else}
            {index + 1}
          {/if}
        </div>
        {#if showLabels && label}
          <span class="{size === 'small' ? 'text-xs' : 'text-sm'} font-medium whitespace-nowrap" style="color: {status === 'current' ? 'var(--ds-text)' : 'var(--ds-text-subtle)'};">
            {label}
          </span>
        {/if}
      </div>
      {#if index < steps.length - 1}
        <div
          class="w-8 h-0.5 mx-1"
          style="background: {status === 'completed' ? 'var(--ds-interactive)' : 'var(--ds-border)'};"
        ></div>
      {/if}
    </div>
  {/each}
</div>
