<script>
  /**
   * StatCard - Dashboard stat card with gradient background and icon
   *
   * Consolidates the 4 identical gradient cards pattern from Dashboard.svelte
   */
  import Card from './Card.svelte';

  let {
    icon: Icon,
    label,
    value,
    color = 'blue', // 'blue', 'green', 'orange', 'purple'
    href = null
  } = $props();

  // Gradient background colors (subtle tint towards the accent color)
  const gradientColors = {
    blue: 'rgba(59, 130, 246, 0.02)',
    green: 'rgba(34, 197, 94, 0.02)',
    orange: 'rgba(245, 101, 101, 0.02)',
    purple: 'rgba(139, 69, 196, 0.02)'
  };

  // Icon container background colors (using design tokens that auto-switch)
  const iconBgStyles = {
    blue: 'background-color: var(--ds-accent-blue-subtle);',
    green: 'background-color: var(--ds-accent-green-subtle);',
    orange: 'background-color: var(--ds-accent-orange-subtle);',
    purple: 'background-color: var(--ds-accent-purple-subtle);'
  };

  // Icon colors (using design tokens that auto-switch)
  const iconColorStyles = {
    blue: 'color: var(--ds-accent-blue);',
    green: 'color: var(--ds-accent-green);',
    orange: 'color: var(--ds-accent-orange);',
    purple: 'color: var(--ds-accent-purple);'
  };

  const gradientStyle = $derived(
    `background: linear-gradient(135deg, var(--ds-surface-raised) 0%, ${gradientColors[color]} 100%);`
  );
</script>

<Card shadow hoverable {href} style={gradientStyle}>
  <div class="flex items-center">
    <div class="flex-shrink-0">
      <div class="w-6 h-6 rounded-md flex items-center justify-center" style={iconBgStyles[color]}>
        <Icon class="w-3.5 h-3.5" style={iconColorStyles[color]} />
      </div>
    </div>
    <div class="ml-3 w-0 flex-1">
      <dl>
        <dt class="text-xs font-medium truncate" style="color: var(--ds-text-subtle);">{label}</dt>
        <dd class="text-xl font-semibold" style="color: var(--ds-text);">{value}</dd>
      </dl>
    </div>
  </div>
</Card>
