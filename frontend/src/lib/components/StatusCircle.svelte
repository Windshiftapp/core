<script>
  import { Check } from 'lucide-svelte';

  let {
    status = 'pending',  // 'completed' | 'active' | 'pending'
    size = 'md',         // 'sm' | 'md' | 'lg'
    color = '#1388E7',   // Custom color override
    class: className = ''
  } = $props();

  const sizeClasses = $derived({
    sm: 'w-5 h-5',
    md: 'w-6 h-6',
    lg: 'w-7 h-7'
  }[size] || 'w-6 h-6');

  const checkSize = $derived({
    sm: 'w-3 h-3',
    md: 'w-3.5 h-3.5',
    lg: 'w-4 h-4'
  }[size] || 'w-3.5 h-3.5');

  const dotSize = $derived({
    sm: 'w-1.5 h-1.5',
    md: 'w-2 h-2',
    lg: 'w-2.5 h-2.5'
  }[size] || 'w-2 h-2');
</script>

<div class="rounded-full flex items-center justify-center {sizeClasses} {className}">
  {#if status === 'completed'}
    <div style="background-color: {color};" class="rounded-full flex items-center justify-center {sizeClasses}">
      <Check class="text-white {checkSize}" />
    </div>
  {:else if status === 'active'}
    <div 
      class="rounded-full border-2 flex items-center justify-center {sizeClasses}" 
      style="border-color: {color};"
    >
      <div class="rounded-full {dotSize}" style="background-color: {color};"></div>
    </div>
  {:else}
    <div 
      class="rounded-full border-2 {sizeClasses}" 
      style="border-color: var(--ds-border);"
    ></div>
  {/if}
</div>
