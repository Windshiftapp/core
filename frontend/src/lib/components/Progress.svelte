<script>
  let {
    value = 0,              // Current value (0-100)
    max = 100,              // Maximum value
    variant = 'bar',        // 'bar' | 'circular'
    size = 'md',            // 'sm' | 'md' | 'lg'
    color = 'primary',      // 'primary' | 'success' | 'warning' | 'danger'
    showLabel = false,      // Show percentage label
    label = '',             // Custom label text
    class: className = ''
  } = $props();

  const percentage = $derived(Math.min(100, Math.max(0, (value / max) * 100)));

  const sizeClass = $derived({
    sm: 'h-1',
    md: 'h-2',
    lg: 'h-3'
  }[size] || 'h-2');

  const colorToken = $derived({
    primary: '--ds-interactive',
    success: '--ds-success',
    warning: '--ds-warning',
    danger: '--ds-danger'
  }[color] || '--ds-interactive');

  // Circular variant dimensions
  const circleSize = $derived({
    sm: 32,
    md: 48,
    lg: 64
  }[size] || 48);

  const strokeWidth = $derived({
    sm: 3,
    md: 4,
    lg: 5
  }[size] || 4);

  const radius = $derived((circleSize - strokeWidth) / 2);
  const circumference = $derived(2 * Math.PI * radius);
  const strokeDashoffset = $derived(circumference - (percentage / 100) * circumference);
</script>

{#if variant === 'bar'}
  <div class="w-full {className}">
    {#if showLabel || label}
      <div class="flex justify-between mb-1 text-sm" style="color: var(--ds-text-subtle);">
        <span>{label || ''}</span>
        {#if showLabel}
          <span>{Math.round(percentage)}%</span>
        {/if}
      </div>
    {/if}
    <div
      class="w-full rounded-full {sizeClass}"
      style="background-color: var(--ds-background-neutral);"
    >
      <div
        class="rounded-full transition-all duration-300 {sizeClass}"
        style="
          width: {percentage}%;
          background-color: var({colorToken});
        "
      ></div>
    </div>
  </div>
{:else if variant === 'circular'}
  <div class="relative inline-flex items-center justify-center {className}">
    <svg
      width={circleSize}
      height={circleSize}
      class="transform -rotate-90"
    >
      <!-- Background circle -->
      <circle
        cx={circleSize / 2}
        cy={circleSize / 2}
        r={radius}
        fill="none"
        stroke="var(--ds-background-neutral)"
        stroke-width={strokeWidth}
      />
      <!-- Progress circle -->
      <circle
        cx={circleSize / 2}
        cy={circleSize / 2}
        r={radius}
        fill="none"
        stroke="var({colorToken})"
        stroke-width={strokeWidth}
        stroke-linecap="round"
        stroke-dasharray={circumference}
        stroke-dashoffset={strokeDashoffset}
        class="transition-all duration-300"
      />
    </svg>
    {#if showLabel}
      <span
        class="absolute text-xs font-medium"
        style="color: var(--ds-text);"
      >
        {Math.round(percentage)}%
      </span>
    {/if}
  </div>
{/if}
